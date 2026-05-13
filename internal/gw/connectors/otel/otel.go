// Package otel implements the OTEL collector connector — gRPC health + config R/W.
package otel

import (
	"context"
	"fmt"
	"os"
	"time"

	c "github.com/yomiroco/yomiro-cli/internal/gw/connectors"
	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

type Handler struct {
	ConfigPath string // default /etc/otelcol/config.yaml
}

func New() *Handler { return &Handler{ConfigPath: "/etc/otelcol/config.yaml"} }

func (h *Handler) Inspect(ctx context.Context, host string, port int, t *c.Tracer) (*c.InspectResult, error) {
	if port == 0 {
		port = 4317
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	start := time.Now()
	conn, err := grpc.DialContext(ctx, addr, grpc.WithInsecure(), grpc.WithBlock(), grpc.WithTimeout(3*time.Second)) //nolint:staticcheck // grpc.NewClient migration tracked separately
	if err != nil {
		t.Record("grpc_dial", "grpc "+addr, start, "fail")
		return &c.InspectResult{Reachable: false, Host: host, Port: port}, nil
	}
	defer conn.Close()
	t.Record("grpc_dial", "grpc "+addr, start, "ok")

	healthStart := time.Now()
	hc := healthpb.NewHealthClient(conn)
	resp, err := hc.Check(ctx, &healthpb.HealthCheckRequest{})
	healthy := err == nil && resp.GetStatus() == healthpb.HealthCheckResponse_SERVING
	t.Record("grpc_health", "grpc_health_probe -addr="+addr, healthStart, fmt.Sprintf("%v", resp.GetStatus()))

	res := &c.InspectResult{Reachable: true, ServiceType: "otel-collector", Host: host, Port: port, CurrentConfig: map[string]any{}}
	if b, err := os.ReadFile(h.ConfigPath); err == nil {
		res.CurrentConfig["config_path"] = h.ConfigPath
		res.CurrentConfig["config_size"] = len(b)
	}
	res.AvailableActions = []c.ActionDefinition{
		{Action: "add_receiver", Description: "Add a receiver to otel config", RequiredParams: []string{"receiver_type", "receiver_config"}},
		{Action: "restart", Description: "Restart the otelcol service"},
	}
	if !healthy {
		res.Reachable = false
	}
	return res, nil
}

func (h *Handler) Configure(ctx context.Context, host string, port int, action string, cfg map[string]any, dryRun bool, t *c.Tracer) (*c.ConfigureResult, error) {
	// For v1, config edit is dry-run only — apply requires sudo + service restart, defer until customer demand.
	if !dryRun {
		return nil, fmt.Errorf("otel configure is dry-run only in v1 — apply requires manual edit + systemctl restart otelcol")
	}
	return &c.ConfigureResult{DryRun: true, Preview: map[string]any{"description": fmt.Sprintf("would %s on %s:%d (config at %s)", action, host, port, h.ConfigPath)}}, nil
}

func (h *Handler) Verify(ctx context.Context, host string, port int, expect map[string]any, t *c.Tracer) (*c.VerifyResult, error) {
	res, err := h.Inspect(ctx, host, port, t)
	if err != nil {
		return nil, err
	}
	return &c.VerifyResult{Healthy: res.Reachable, Checks: []c.HealthCheck{{Check: "grpc_serving", Passed: res.Reachable}}}, nil
}

func (h *Handler) AvailableActions() []c.ActionDefinition {
	return []c.ActionDefinition{
		{Action: "add_receiver"}, {Action: "restart"},
	}
}
