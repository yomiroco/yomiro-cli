// Package sonos discovers Sonos speakers via UPnP SSDP.
package sonos

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	c "github.com/yomiroco/yomiro-cli/internal/gw/connectors"
	"github.com/huin/goupnp"
)

type Handler struct{}

func New() *Handler { return &Handler{} }

func (h *Handler) Discover(ctx context.Context, networkRange string, t *c.Tracer) ([]c.ScanTarget, error) {
	start := time.Now()
	devices, err := goupnp.DiscoverDevicesCtx(ctx, "urn:schemas-upnp-org:device:ZonePlayer:1")
	if err != nil {
		t.RecordCount("ssdp_search", "UPnP M-SEARCH ssdp:all", start, 0)
		return nil, nil
	}
	out := make([]c.ScanTarget, 0, len(devices))
	for _, d := range devices {
		if d.Err != nil {
			continue
		}
		host := strings.Split(strings.TrimPrefix(strings.TrimPrefix(d.Location.Host, "http://"), "https://"), ":")[0]
		out = append(out, c.ScanTarget{
			ScanID:   "scan-sonos-" + d.Root.Device.UDN,
			Type:     "sonos_speaker",
			Protocol: "sonos",
			Host:     host,
			Port:     1400,
			Name:     d.Root.Device.FriendlyName,
			Metadata: map[string]any{
				"model":    d.Root.Device.ModelName,
				"udn":      d.Root.Device.UDN,
				"location": d.Location.String(),
			},
		})
	}
	t.RecordCount("ssdp_search", "UPnP M-SEARCH urn:schemas-upnp-org:device:ZonePlayer:1", start, len(out))
	return out, nil
}

func (h *Handler) Inspect(ctx context.Context, host string, port int, t *c.Tracer) (*c.InspectResult, error) {
	if port == 0 {
		port = 1400
	}
	url := fmt.Sprintf("http://%s:%d/xml/device_description.xml", host, port)
	start := time.Now()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Record("sonos_descr", url, start, "fail")
		return &c.InspectResult{Reachable: false, Host: host, Port: port}, nil
	}
	defer resp.Body.Close()
	t.Record("sonos_descr", url, start, resp.Status)
	return &c.InspectResult{
		Reachable: resp.StatusCode/100 == 2, ServiceType: "sonos_speaker",
		Host: host, Port: port,
	}, nil
}

func (h *Handler) Configure(ctx context.Context, host string, port int, action string, cfg map[string]any, dryRun bool, t *c.Tracer) (*c.ConfigureResult, error) {
	return nil, fmt.Errorf("sonos connector has no configure actions yet")
}

func (h *Handler) Verify(ctx context.Context, host string, port int, expect map[string]any, t *c.Tracer) (*c.VerifyResult, error) {
	res, err := h.Inspect(ctx, host, port, t)
	if err != nil {
		return nil, err
	}
	return &c.VerifyResult{Healthy: res.Reachable, Checks: []c.HealthCheck{{Check: "sonos_descr", Passed: res.Reachable}}}, nil
}

func (h *Handler) AvailableActions() []c.ActionDefinition { return nil }
