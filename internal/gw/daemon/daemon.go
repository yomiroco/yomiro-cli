// Package daemon ties together the tunnel client, dbproxy, control socket,
// and reconnect loop into one runnable unit.
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yomiroco/yomiro-cli/internal/config"
	"github.com/yomiroco/yomiro-cli/internal/gw/connectors"
	"github.com/yomiroco/yomiro-cli/internal/gw/connregistry"
	"github.com/yomiroco/yomiro-cli/internal/gw/control"
	"github.com/yomiroco/yomiro-cli/internal/gw/dbproxy"
	"github.com/yomiroco/yomiro-cli/internal/gw/tunnel"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zalando/go-keyring"
)

// Daemon owns the runtime state of a running gateway.
type Daemon struct {
	Cfg      *config.GwConfig
	Pool     *pgxpool.Pool
	Resolver *connectors.TargetResolver

	current *tunnel.Client
	paused  atomic.Bool
}

// New builds a Daemon from a parsed config + an open Postgres pool.
func New(cfg *config.GwConfig, pool *pgxpool.Pool) *Daemon {
	return &Daemon{Cfg: cfg, Pool: pool, Resolver: connregistry.Build(cfg.Connectors.Enabled)}
}

// Run blocks until ctx cancels, reconnecting with exponential backoff on transport errors.
func (d *Daemon) Run(ctx context.Context) error {
	cacheDir, err := config.CacheDir()
	if err != nil {
		return err
	}
	sockPath := filepath.Join(cacheDir, "gw.sock")
	srv, err := control.Listen(sockPath, d.handleControl)
	if err != nil {
		return fmt.Errorf("listen control socket: %w", err)
	}
	defer srv.Close()

	backoff := time.Second
	maxBackoff := time.Duration(d.Cfg.Daemon.ReconnectMaxBackoffS) * time.Second
	if maxBackoff == 0 {
		maxBackoff = 60 * time.Second
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		err := d.runOnce(ctx)
		if ctx.Err() != nil {
			return nil
		}
		jitter := time.Duration(rand.Int63n(int64(backoff / 2)))
		sleep := backoff + jitter
		if sleep > maxBackoff {
			sleep = maxBackoff
		}
		fmt.Printf("tunnel disconnected (%v) — retrying in %s\n", err, sleep)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(sleep):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (d *Daemon) runOnce(ctx context.Context) error {
	token, err := d.resolveToken()
	if err != nil {
		return err
	}

	allow := &dbproxy.Allowlist{
		Tables:         d.Cfg.Database.AllowedTables,
		BlockedColumns: d.Cfg.Database.BlockedColumns,
		ReadOnly:       d.Cfg.Database.ReadOnly,
	}
	pp := &dbproxy.PgProxy{
		Pool:         d.Pool,
		MaxRows:      d.Cfg.Database.MaxRowsPerQuery,
		QueryTimeout: time.Duration(d.Cfg.Database.QueryTimeoutSeconds) * time.Second,
		Allowlist:    allow,
	}

	client := &tunnel.Client{
		URL: d.Cfg.Platform.Endpoint,
		Auth: tunnel.AuthFrame{
			Token:     token,
			GatewayID: d.Cfg.Gateway.ID,
			Version:   d.Cfg.Gateway.Version,
			Manifest:  d.buildManifest(),
		},
		Handlers:          d.handlers(pp),
		HeartbeatInterval: time.Duration(d.Cfg.Daemon.HeartbeatIntervalS) * time.Second,
	}
	d.current = client
	defer func() { d.current = nil }()
	return client.Run(ctx)
}

func (d *Daemon) buildManifest() *tunnel.Manifest {
	m := &tunnel.Manifest{
		Connectors: d.Cfg.Connectors.Enabled,
		Tools:      []string{"status", "scan", "inspect", "configure", "verify", "query"},
	}
	if d.Cfg.Database.URL != "" {
		m.Database = &tunnel.DatabaseManifest{
			Type:            "postgresql",
			Schemas:         d.Cfg.Database.AllowedSchemas,
			Tables:          d.Cfg.Database.AllowedTables,
			BlockedColumns:  d.Cfg.Database.BlockedColumns,
			MaxRowsPerQuery: d.Cfg.Database.MaxRowsPerQuery,
			ReadOnly:        d.Cfg.Database.ReadOnly,
		}
	}
	return m
}

func (d *Daemon) handlers(pp *dbproxy.PgProxy) map[string]tunnel.Handler {
	return map[string]tunnel.Handler{
		"query": func(ctx context.Context, params json.RawMessage) (map[string]any, []tunnel.TraceStep, error) {
			if d.paused.Load() {
				return nil, nil, fmt.Errorf("gateway is paused")
			}
			var p tunnel.QueryParams
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, nil, err
			}
			start := time.Now()
			res, err := pp.Execute(ctx, p.SQL)
			trace := []tunnel.TraceStep{{
				Step: "pgx_query", Command: "Query " + truncate(p.SQL, 200),
				DurationMs: int(time.Since(start).Milliseconds()),
			}}
			if err != nil {
				return nil, trace, err
			}
			return map[string]any{
				"columns":           res.Columns,
				"rows":              res.Rows,
				"row_count":         res.RowCount,
				"execution_time_ms": res.ExecutionTimeMs,
			}, trace, nil
		},
		"status": func(ctx context.Context, _ json.RawMessage) (map[string]any, []tunnel.TraceStep, error) {
			uptime, queries := 0, 0
			if d.current != nil {
				uptime, queries = d.current.Stats()
			}
			return map[string]any{
				"gateway_id":     d.Cfg.Gateway.ID,
				"connected":      d.current != nil,
				"uptime_seconds": uptime,
				"queries_served": queries,
				"manifest":       d.buildManifest(),
				"paused":         d.paused.Load(),
			}, nil, nil
		},
		"scan": func(ctx context.Context, params json.RawMessage) (map[string]any, []tunnel.TraceStep, error) {
			var p struct {
				Filter       string `json:"filter"`
				NetworkRange string `json:"network_range"`
			}
			_ = json.Unmarshal(params, &p)
			tracer := &connectors.Tracer{}
			found := []connectors.ScanTarget{}
			for _, dh := range d.Resolver.DiscoveryHandlers(p.Filter) {
				targets, _ := dh.Discover(ctx, p.NetworkRange, tracer)
				found = append(found, targets...)
			}
			return map[string]any{"targets": found}, traceToTunnel(tracer), nil
		},
		"inspect": func(ctx context.Context, params json.RawMessage) (map[string]any, []tunnel.TraceStep, error) {
			var p struct {
				Host        string `json:"host"`
				Port        int    `json:"port"`
				ServiceType string `json:"service_type"`
			}
			_ = json.Unmarshal(params, &p)
			h := d.Resolver.Resolve(p.ServiceType, p.Host, p.Port)
			if h == nil {
				return nil, nil, fmt.Errorf("no handler for service_type=%q", p.ServiceType)
			}
			tracer := &connectors.Tracer{}
			res, err := h.Inspect(ctx, p.Host, p.Port, tracer)
			if err != nil {
				return nil, traceToTunnel(tracer), err
			}
			return map[string]any{
				"reachable":         res.Reachable,
				"service_type":      res.ServiceType,
				"host":              res.Host,
				"port":              res.Port,
				"current_config":    res.CurrentConfig,
				"available_actions": res.AvailableActions,
			}, traceToTunnel(tracer), nil
		},
		"configure": func(ctx context.Context, params json.RawMessage) (map[string]any, []tunnel.TraceStep, error) {
			var p struct {
				Host   string         `json:"host"`
				Port   int            `json:"port"`
				Action string         `json:"action"`
				Config map[string]any `json:"config"`
				DryRun bool           `json:"dry_run"`
			}
			_ = json.Unmarshal(params, &p)
			h := d.Resolver.Resolve("", p.Host, p.Port)
			tracer := &connectors.Tracer{}
			res, err := h.Configure(ctx, p.Host, p.Port, p.Action, p.Config, p.DryRun, tracer)
			if err != nil {
				return nil, traceToTunnel(tracer), err
			}
			return map[string]any{
				"dry_run":         res.DryRun,
				"applied":         res.Applied,
				"preview":         res.Preview,
				"changes_applied": res.ChangesApplied,
				"verify_hint":     res.VerifyHint,
			}, traceToTunnel(tracer), nil
		},
		"verify": func(ctx context.Context, params json.RawMessage) (map[string]any, []tunnel.TraceStep, error) {
			var p struct {
				Host        string         `json:"host"`
				Port        int            `json:"port"`
				ServiceType string         `json:"service_type"`
				Expect      map[string]any `json:"expect"`
			}
			_ = json.Unmarshal(params, &p)
			h := d.Resolver.Resolve(p.ServiceType, p.Host, p.Port)
			tracer := &connectors.Tracer{}
			res, err := h.Verify(ctx, p.Host, p.Port, p.Expect, tracer)
			if err != nil {
				return nil, traceToTunnel(tracer), err
			}
			checks := make([]map[string]any, 0, len(res.Checks))
			for _, c := range res.Checks {
				checks = append(checks, map[string]any{"check": c.Check, "passed": c.Passed, "detail": c.Detail})
			}
			return map[string]any{"healthy": res.Healthy, "checks": checks}, traceToTunnel(tracer), nil
		},
	}
}

func traceToTunnel(t *connectors.Tracer) []tunnel.TraceStep {
	src := t.Steps()
	out := make([]tunnel.TraceStep, len(src))
	for i, s := range src {
		out[i] = tunnel.TraceStep{Step: s.Step, Command: s.Command, DurationMs: s.DurationMs, Result: s.Result, Found: s.Found}
	}
	return out
}

func (d *Daemon) resolveToken() (string, error) {
	ref := d.Cfg.Platform.TokenRef
	const prefix = "keyring:"
	if !strings.HasPrefix(ref, prefix) {
		return "", fmt.Errorf("token_ref must be a keyring: URL")
	}
	body := strings.TrimPrefix(ref, prefix)
	parts := strings.SplitN(body, "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("malformed token_ref: %q", ref)
	}
	return keyring.Get(parts[0], parts[1])
}

func (d *Daemon) handleControl(req control.Request) control.Response {
	switch req.Action {
	case "status":
		if d.current == nil {
			return control.Response{OK: true, Data: map[string]any{
				"connected":  false,
				"gateway_id": d.Cfg.Gateway.ID,
				"paused":     d.paused.Load(),
			}}
		}
		uptime, queries := d.current.Stats()
		return control.Response{OK: true, Data: map[string]any{
			"connected":      true,
			"uptime_seconds": uptime,
			"queries_served": queries,
			"gateway_id":     d.Cfg.Gateway.ID,
			"paused":         d.paused.Load(),
		}}
	case "pause":
		d.paused.Store(true)
		return control.Response{OK: true, Detail: "paused"}
	case "resume":
		d.paused.Store(false)
		return control.Response{OK: true, Detail: "resumed"}
	case "reload":
		// Future work: re-read gw.yaml and reconnect. For now we acknowledge —
		// the next reconnect cycle picks up any keyring/config edits anyway.
		return control.Response{OK: true, Detail: "config reload requested"}
	default:
		return control.Response{OK: false, Detail: "unknown action"}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
