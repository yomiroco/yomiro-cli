// Package modbus implements the Modbus TCP connector — read holding registers,
// device ID via function 0x2B/0x0E.
package modbus

import (
	"context"
	"fmt"
	"net/url"
	"time"

	c "github.com/yomiroco/yomiro-cli/internal/gw/connectors"
	mb "github.com/simonvetter/modbus"
)

type Handler struct{}

func New() *Handler { return &Handler{} }

func (h *Handler) Inspect(ctx context.Context, host string, port int, t *c.Tracer) (*c.InspectResult, error) {
	if port == 0 {
		port = 502
	}
	u := &url.URL{Scheme: "tcp", Host: fmt.Sprintf("%s:%d", host, port)}
	start := time.Now()
	cli, err := mb.NewClient(&mb.ClientConfiguration{URL: u.String(), Timeout: 3 * time.Second})
	if err != nil {
		return nil, err
	}
	if err := cli.Open(); err != nil {
		t.Record("modbus_connect", "modbus tcp "+u.Host, start, "fail")
		return &c.InspectResult{Reachable: false, Host: host, Port: port}, nil
	}
	defer cli.Close()
	t.Record("modbus_connect", "modbus tcp "+u.Host, start, "open")

	regsStart := time.Now()
	regs, _ := cli.ReadRegisters(0, 3, mb.HOLDING_REGISTER)
	t.Record("read_holding", fmt.Sprintf("modbus read holding 0x0000 count=3 from %s", u.Host), regsStart, fmt.Sprintf("%v", regs))

	return &c.InspectResult{
		Reachable:   true,
		ServiceType: "modbus-tcp",
		Host:        host,
		Port:        port,
		CurrentConfig: map[string]any{
			"sample_holding_registers": regs,
		},
		AvailableActions: []c.ActionDefinition{
			{Action: "read_register", Description: "Read holding registers", RequiredParams: []string{"address", "count"}},
		},
	}, nil
}

func (h *Handler) Configure(ctx context.Context, host string, port int, action string, cfg map[string]any, dryRun bool, t *c.Tracer) (*c.ConfigureResult, error) {
	if action != "read_register" {
		return nil, fmt.Errorf("unknown action %q", action)
	}
	if dryRun {
		return &c.ConfigureResult{DryRun: true, Preview: map[string]any{"description": "read_register is read-only — dry_run is identical to apply"}}, nil
	}
	addr, _ := cfg["address"].(float64)
	count, _ := cfg["count"].(float64)
	if port == 0 {
		port = 502
	}
	cli, err := mb.NewClient(&mb.ClientConfiguration{URL: fmt.Sprintf("tcp://%s:%d", host, port), Timeout: 3 * time.Second})
	if err != nil {
		return nil, err
	}
	if err := cli.Open(); err != nil {
		return nil, err
	}
	defer cli.Close()
	regs, err := cli.ReadRegisters(uint16(addr), uint16(count), mb.HOLDING_REGISTER)
	if err != nil {
		return nil, err
	}
	return &c.ConfigureResult{DryRun: false, Applied: true, ChangesApplied: []string{fmt.Sprintf("read %d registers from 0x%04X: %v", int(count), int(addr), regs)}}, nil
}

func (h *Handler) Verify(ctx context.Context, host string, port int, expect map[string]any, t *c.Tracer) (*c.VerifyResult, error) {
	res, err := h.Inspect(ctx, host, port, t)
	if err != nil {
		return nil, err
	}
	return &c.VerifyResult{Healthy: res.Reachable, Checks: []c.HealthCheck{{Check: "modbus_reachable", Passed: res.Reachable, Detail: ""}}}, nil
}

func (h *Handler) AvailableActions() []c.ActionDefinition {
	return []c.ActionDefinition{
		{Action: "read_register", Description: "Read holding registers", RequiredParams: []string{"address", "count"}},
	}
}
