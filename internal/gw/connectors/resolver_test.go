package connectors

import (
	"context"
	"testing"
)

type stubHandler struct{ name string }

func (s *stubHandler) Inspect(ctx context.Context, host string, port int, t *Tracer) (*InspectResult, error) {
	return &InspectResult{Reachable: true, ServiceType: s.name, Host: host, Port: port}, nil
}
func (s *stubHandler) Configure(ctx context.Context, host string, port int, action string, config map[string]any, dryRun bool, t *Tracer) (*ConfigureResult, error) {
	return &ConfigureResult{DryRun: dryRun}, nil
}
func (s *stubHandler) Verify(ctx context.Context, host string, port int, expect map[string]any, t *Tracer) (*VerifyResult, error) {
	return &VerifyResult{Healthy: true}, nil
}
func (s *stubHandler) AvailableActions() []ActionDefinition { return nil }

func TestResolverPicksByServiceType(t *testing.T) {
	r := NewResolver()
	r.Register("postgres", &stubHandler{name: "postgres"}, nil)
	r.Register("mqtt", &stubHandler{name: "mqtt"}, nil)
	got := r.Resolve("mqtt", "10.0.0.1", 1883)
	if got == nil {
		t.Fatal("nil handler")
	}
	res, _ := got.Inspect(context.Background(), "10.0.0.1", 1883, &Tracer{})
	if res.ServiceType != "mqtt" {
		t.Fatalf("got %q", res.ServiceType)
	}
}

func TestResolverFallsBackToGenericForUnknownType(t *testing.T) {
	r := NewResolver()
	r.Register("postgres", &stubHandler{name: "postgres"}, nil)
	r.SetGeneric(&stubHandler{name: "generic"})
	got := r.Resolve("", "10.0.0.1", 80)
	if got == nil {
		t.Fatal("nil handler")
	}
}
