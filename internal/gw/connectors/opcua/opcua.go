// Package opcua implements the OPC-UA connector — namespace browse, node listing.
package opcua

import (
	"context"
	"fmt"
	"time"

	ua "github.com/gopcua/opcua"
	"github.com/gopcua/opcua/id"
	uaproto "github.com/gopcua/opcua/ua"
	c "github.com/yomiroco/yomiro-cli/internal/gw/connectors"
)

type Handler struct{}

func New() *Handler { return &Handler{} }

func (h *Handler) Inspect(ctx context.Context, host string, port int, t *c.Tracer) (*c.InspectResult, error) {
	if port == 0 {
		port = 4840
	}
	endpoint := fmt.Sprintf("opc.tcp://%s:%d", host, port)
	start := time.Now()
	cli, err := ua.NewClient(endpoint, ua.SecurityMode(uaproto.MessageSecurityModeNone))
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := cli.Connect(cctx); err != nil {
		t.Record("opcua_connect", "opc.tcp connect "+endpoint, start, "fail")
		return &c.InspectResult{Reachable: false, Host: host, Port: port}, nil
	}
	defer cli.Close(cctx)
	t.Record("opcua_connect", "opc.tcp connect "+endpoint, start, "ok")

	// Browse the root.
	browseStart := time.Now()
	req := &uaproto.BrowseRequest{
		NodesToBrowse: []*uaproto.BrowseDescription{{
			NodeID:          uaproto.NewNumericNodeID(0, id.RootFolder),
			BrowseDirection: uaproto.BrowseDirectionForward,
			ResultMask:      uint32(uaproto.BrowseResultMaskAll),
		}},
	}
	resp, err := cli.Browse(cctx, req)
	nodeCount := 0
	if err == nil && len(resp.Results) > 0 {
		nodeCount = len(resp.Results[0].References)
	}
	t.RecordCount("opcua_browse", "Browse RootFolder", browseStart, nodeCount)

	return &c.InspectResult{
		Reachable:   true,
		ServiceType: "opcua",
		Host:        host,
		Port:        port,
		CurrentConfig: map[string]any{
			"root_node_count": nodeCount,
		},
	}, nil
}

func (h *Handler) Configure(ctx context.Context, host string, port int, action string, cfg map[string]any, dryRun bool, t *c.Tracer) (*c.ConfigureResult, error) {
	return nil, fmt.Errorf("opcua connector has no configure actions yet")
}

func (h *Handler) Verify(ctx context.Context, host string, port int, expect map[string]any, t *c.Tracer) (*c.VerifyResult, error) {
	res, err := h.Inspect(ctx, host, port, t)
	if err != nil {
		return nil, err
	}
	return &c.VerifyResult{Healthy: res.Reachable, Checks: []c.HealthCheck{{Check: "opcua_reachable", Passed: res.Reachable}}}, nil
}

func (h *Handler) AvailableActions() []c.ActionDefinition { return nil }
