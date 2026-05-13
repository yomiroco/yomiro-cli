// Package connectors holds the ServiceHandler abstraction and the built-in
// connectors (postgres, mqtt, modbus, opcua, sonos, otel, generic).
package connectors

import (
	"context"
	"sync"
	"time"
)

// ServiceHandler covers inspect/configure/verify for one service type.
type ServiceHandler interface {
	Inspect(ctx context.Context, host string, port int, t *Tracer) (*InspectResult, error)
	Configure(ctx context.Context, host string, port int, action string, config map[string]any, dryRun bool, t *Tracer) (*ConfigureResult, error)
	Verify(ctx context.Context, host string, port int, expect map[string]any, t *Tracer) (*VerifyResult, error)
	AvailableActions() []ActionDefinition
}

// DiscoveryHandler optionally adds LAN discovery for a connector.
type DiscoveryHandler interface {
	Discover(ctx context.Context, networkRange string, t *Tracer) ([]ScanTarget, error)
}

// ScanTarget is one device/service surfaced by a Discover call.
type ScanTarget struct {
	ScanID   string         `json:"scan_id"`
	Type     string         `json:"type"`
	Protocol string         `json:"protocol"`
	Host     string         `json:"host"`
	Port     int            `json:"port,omitempty"`
	Name     string         `json:"name,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// InspectResult is the response from Inspect.
type InspectResult struct {
	Reachable        bool               `json:"reachable"`
	ServiceType      string             `json:"service_type,omitempty"`
	Version          string             `json:"version,omitempty"`
	Host             string             `json:"host"`
	Port             int                `json:"port,omitempty"`
	CurrentConfig    map[string]any     `json:"current_config,omitempty"`
	AvailableActions []ActionDefinition `json:"available_actions,omitempty"`
}

// ActionDefinition describes a Configure action surface.
type ActionDefinition struct {
	Action         string         `json:"action"`
	Description    string         `json:"description"`
	RequiredParams []string       `json:"required_params,omitempty"`
	OptionalParams []string       `json:"optional_params,omitempty"`
	Example        map[string]any `json:"example,omitempty"`
}

// ConfigureResult is the response from Configure.
type ConfigureResult struct {
	DryRun         bool           `json:"dry_run"`
	Applied        bool           `json:"applied"`
	Preview        map[string]any `json:"preview,omitempty"`
	BackupRef      string         `json:"backup_ref,omitempty"`
	ChangesApplied []string       `json:"changes_applied,omitempty"`
	VerifyHint     string         `json:"verify_hint,omitempty"`
}

// HealthCheck is one verification step.
type HealthCheck struct {
	Check  string `json:"check"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

// VerifyResult is the response from Verify.
type VerifyResult struct {
	Healthy bool          `json:"healthy"`
	Checks  []HealthCheck `json:"checks"`
}

// TraceStep records one underlying CLI/protocol operation.
type TraceStep struct {
	Step       string `json:"step"`
	Command    string `json:"command"`
	DurationMs int    `json:"duration_ms"`
	Result     string `json:"result,omitempty"`
	Found      *int   `json:"found,omitempty"`
}

// Tracer is passed to every handler method; it captures TraceStep entries.
type Tracer struct {
	mu    sync.Mutex
	steps []TraceStep
}

// Record adds one trace step. start is the time the step started.
func (t *Tracer) Record(step, command string, start time.Time, result string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.steps = append(t.steps, TraceStep{
		Step:       step,
		Command:    command,
		DurationMs: int(time.Since(start).Milliseconds()),
		Result:     result,
	})
}

// RecordCount is the same as Record but with a found-count for discovery steps.
func (t *Tracer) RecordCount(step, command string, start time.Time, found int) {
	n := found
	t.mu.Lock()
	defer t.mu.Unlock()
	t.steps = append(t.steps, TraceStep{
		Step:       step,
		Command:    command,
		DurationMs: int(time.Since(start).Milliseconds()),
		Found:      &n,
	})
}

// Steps returns a snapshot of the recorded steps.
func (t *Tracer) Steps() []TraceStep {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]TraceStep, len(t.steps))
	copy(out, t.steps)
	return out
}
