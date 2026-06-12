// Package daemon ties together the tunnel client, dbproxy, control socket,
// and reconnect loop into one runnable unit.
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// Daemon owns the runtime state of a running gateway. Config, the DB pool, and
// the connector resolver live in atomic pointers so `gw reload` can swap them
// from the control-socket goroutine while the reconnect loop reads them.
type Daemon struct {
	// Log is where the reconnect loop writes its status lines. nil ⇒ os.Stdout.
	// `gw run` points this at the daemon log file so `gw logs` can tail it.
	Log io.Writer
	// ConfigPath is the gw.yaml path; reload re-reads it. Empty ⇒ reload errors.
	ConfigPath string

	cfg      atomic.Pointer[config.GwConfig]
	pool     atomic.Pointer[pgxpool.Pool]
	resolver atomic.Pointer[connectors.TargetResolver]

	mu         sync.Mutex // guards current + connCancel
	current    *tunnel.Client
	connCancel context.CancelFunc

	// reloadCh signals the reconnect loop to redial immediately with fresh
	// config. Buffered(1) + non-blocking send so reload never blocks; it
	// interrupts both an active connection (via connCancel) and the backoff
	// sleep (via the select in Run).
	reloadCh chan struct{}
	paused   atomic.Bool
}

// New builds a Daemon from a parsed config + an open Postgres pool (pool may be
// nil when no database is configured).
func New(cfg *config.GwConfig, pool *pgxpool.Pool) *Daemon {
	d := &Daemon{reloadCh: make(chan struct{}, 1)}
	d.cfg.Store(cfg)
	if pool != nil {
		d.pool.Store(pool)
	}
	d.resolver.Store(connregistry.Build(cfg.Connectors.Enabled))
	return d
}

func (d *Daemon) config() *config.GwConfig { return d.cfg.Load() }

func (d *Daemon) logw() io.Writer {
	if d.Log != nil {
		return d.Log
	}
	return os.Stdout
}

func (d *Daemon) currentClient() *tunnel.Client {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.current
}

// Run blocks until ctx cancels, reconnecting with exponential backoff on
// transport errors. A `reload` resets the backoff so the new config applies
// immediately rather than after the current backoff window.
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
	maxBackoff := time.Duration(d.config().Daemon.ReconnectMaxBackoffS) * time.Second
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

		// A reload deliberately dropped the connection — reconnect now with the
		// fresh config instead of waiting out the backoff.
		select {
		case <-d.reloadCh:
			fmt.Fprintf(d.logw(), "config reloaded — reconnecting\n")
			backoff = time.Second
			continue
		default:
		}

		jitter := time.Duration(rand.Int63n(int64(backoff / 2)))
		sleep := backoff + jitter
		if sleep > maxBackoff {
			sleep = maxBackoff
		}
		fmt.Fprintf(d.logw(), "tunnel disconnected (%v) — retrying in %s\n", err, sleep)
		select {
		case <-ctx.Done():
			return nil
		case <-d.reloadCh:
			// Reload arrived while we were already disconnected/backing off —
			// don't wait out the window; redial immediately with the new config.
			fmt.Fprintf(d.logw(), "config reloaded — reconnecting\n")
			backoff = time.Second
			continue
		case <-time.After(sleep):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (d *Daemon) runOnce(ctx context.Context) error {
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	d.mu.Lock()
	d.connCancel = cancel
	d.mu.Unlock()

	cfg := d.config()
	token, err := d.resolveToken()
	if err != nil {
		return err
	}

	allow := &dbproxy.Allowlist{
		Tables:         cfg.Database.AllowedTables,
		BlockedColumns: cfg.Database.BlockedColumns,
		ReadOnly:       cfg.Database.ReadOnly,
	}
	pp := &dbproxy.PgProxy{
		Pool:         d.pool.Load(),
		MaxRows:      cfg.Database.MaxRowsPerQuery,
		QueryTimeout: time.Duration(cfg.Database.QueryTimeoutSeconds) * time.Second,
		Allowlist:    allow,
	}

	client := &tunnel.Client{
		URL: cfg.Platform.Endpoint,
		Auth: tunnel.AuthFrame{
			Token:     token,
			GatewayID: cfg.Gateway.ID,
			Version:   cfg.Gateway.Version,
			Manifest:  d.buildManifest(),
		},
		Handlers:          d.handlers(pp),
		HeartbeatInterval: time.Duration(cfg.Daemon.HeartbeatIntervalS) * time.Second,
	}
	d.mu.Lock()
	d.current = client
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		d.current = nil
		d.mu.Unlock()
	}()
	return client.Run(connCtx)
}

func (d *Daemon) buildManifest() *tunnel.Manifest {
	cfg := d.config()
	m := &tunnel.Manifest{
		Connectors: cfg.Connectors.Enabled,
		Tools:      []string{"status", "scan", "inspect", "configure", "verify", "query", "introspect"},
	}
	if cfg.Database.URL != "" {
		m.Database = &tunnel.DatabaseManifest{
			Type:            "postgresql",
			Schemas:         cfg.Database.AllowedSchemas,
			Tables:          cfg.Database.AllowedTables,
			BlockedColumns:  cfg.Database.BlockedColumns,
			MaxRowsPerQuery: cfg.Database.MaxRowsPerQuery,
			ReadOnly:        cfg.Database.ReadOnly,
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
			c := d.currentClient()
			if c != nil {
				uptime, queries = c.Stats()
			}
			return map[string]any{
				"gateway_id":     d.config().Gateway.ID,
				"connected":      c != nil,
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
			for _, dh := range d.resolver.Load().DiscoveryHandlers(p.Filter) {
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
			h := d.resolver.Load().Resolve(p.ServiceType, p.Host, p.Port)
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
			h := d.resolver.Load().Resolve("", p.Host, p.Port)
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
		"introspect": func(ctx context.Context, _ json.RawMessage) (map[string]any, []tunnel.TraceStep, error) {
			start := time.Now()
			schema, err := pp.Schema(ctx, d.config().Database.AllowedTables)
			trace := []tunnel.TraceStep{{
				Step: "introspect", Command: "information_schema.columns",
				DurationMs: int(time.Since(start).Milliseconds()),
			}}
			if err != nil {
				return nil, trace, err
			}
			return map[string]any{"tables": schema}, trace, nil
		},
		"verify": func(ctx context.Context, params json.RawMessage) (map[string]any, []tunnel.TraceStep, error) {
			var p struct {
				Host        string         `json:"host"`
				Port        int            `json:"port"`
				ServiceType string         `json:"service_type"`
				Expect      map[string]any `json:"expect"`
			}
			_ = json.Unmarshal(params, &p)
			h := d.resolver.Load().Resolve(p.ServiceType, p.Host, p.Port)
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
	ref := d.config().Platform.TokenRef
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

// reload re-reads gw.yaml, swaps in the new config (rebuilding the DB pool if
// the URL changed and the connector resolver if the enabled set changed), and
// drops the current tunnel so the reconnect loop dials again with the new
// config. Endpoint, allowlist, token-ref, and connector edits all take effect
// without a daemon restart.
func (d *Daemon) reload() error {
	if d.ConfigPath == "" {
		return fmt.Errorf("config path unknown")
	}
	newCfg, err := config.LoadGwConfig(d.ConfigPath)
	if err != nil {
		return err
	}
	old := d.config()

	// Build the new pool before swapping so a bad URL fails the reload cleanly
	// without disturbing the running connection. Defer closing the old pool
	// until after we've cancelled the live connection (below), so an in-flight
	// query is cancelled before its pool is torn down.
	var oldPool *pgxpool.Pool
	if newCfg.Database.URL != "" && newCfg.Database.URL != old.Database.URL {
		newPool, perr := dbproxy.NewPgProxy(context.Background(), newCfg.Database.URL, newCfg.Database.MaxConnections)
		if perr != nil {
			return fmt.Errorf("reconnect db with new url: %w", perr)
		}
		oldPool = d.pool.Swap(newPool)
	}

	d.cfg.Store(newCfg)
	d.resolver.Store(connregistry.Build(newCfg.Connectors.Enabled))

	// Force the reconnect loop to dial again with the new config immediately:
	// wake the backoff sleep (channel) and drop any live connection (cancel).
	select {
	case d.reloadCh <- struct{}{}:
	default:
	}
	d.mu.Lock()
	cancel := d.connCancel
	d.mu.Unlock()
	if cancel != nil {
		cancel()
	}

	if oldPool != nil {
		oldPool.Close()
	}
	return nil
}

func (d *Daemon) handleControl(req control.Request) control.Response {
	switch req.Action {
	case "status":
		c := d.currentClient()
		if c == nil {
			return control.Response{OK: true, Data: map[string]any{
				"connected":  false,
				"gateway_id": d.config().Gateway.ID,
				"paused":     d.paused.Load(),
			}}
		}
		uptime, queries := c.Stats()
		return control.Response{OK: true, Data: map[string]any{
			"connected":      true,
			"uptime_seconds": uptime,
			"queries_served": queries,
			"gateway_id":     d.config().Gateway.ID,
			"paused":         d.paused.Load(),
		}}
	case "pause":
		d.paused.Store(true)
		return control.Response{OK: true, Detail: "paused"}
	case "resume":
		d.paused.Store(false)
		return control.Response{OK: true, Detail: "resumed"}
	case "reload":
		if err := d.reload(); err != nil {
			return control.Response{OK: false, Detail: "reload failed: " + err.Error()}
		}
		return control.Response{OK: true, Detail: "config reloaded; reconnecting"}
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
