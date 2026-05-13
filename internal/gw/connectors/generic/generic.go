// Package generic is the fallback connector: TCP connect, banner grab,
// TLS cert read. Used when no specific connector matches.
package generic

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	c "github.com/yomiroco/yomiro-cli/internal/gw/connectors"
)

type Handler struct{}

func New() *Handler { return &Handler{} }

func (h *Handler) Inspect(ctx context.Context, host string, port int, t *c.Tracer) (*c.InspectResult, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	start := time.Now()
	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		t.Record("tcp_connect", "nc -z "+addr, start, "refused")
		return &c.InspectResult{Reachable: false, Host: host, Port: port}, nil
	}
	t.Record("tcp_connect", "nc -z "+addr, start, "open")

	res := &c.InspectResult{Reachable: true, Host: host, Port: port, ServiceType: "unknown", CurrentConfig: map[string]any{}}

	// Banner grab.
	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err == nil || err == io.EOF {
		banner := strings.TrimSpace(string(buf[:n]))
		if banner != "" {
			res.CurrentConfig["banner"] = banner
		}
	}
	conn.Close()

	// TLS probe.
	tlsStart := time.Now()
	td := tls.Dialer{NetDialer: &net.Dialer{Timeout: 2 * time.Second}, Config: &tls.Config{InsecureSkipVerify: true}}
	tconn, err := td.DialContext(ctx, "tcp", addr)
	if err == nil {
		state := tconn.(*tls.Conn).ConnectionState()
		if len(state.PeerCertificates) > 0 {
			cert := state.PeerCertificates[0]
			res.CurrentConfig["tls_subject"] = cert.Subject.String()
			res.CurrentConfig["tls_issuer"] = cert.Issuer.String()
		}
		tconn.Close()
		t.Record("tls_probe", "openssl s_client -connect "+addr, tlsStart, "ok")
	}

	return res, nil
}

func (h *Handler) Configure(ctx context.Context, host string, port int, action string, config map[string]any, dryRun bool, t *c.Tracer) (*c.ConfigureResult, error) {
	return nil, fmt.Errorf("generic connector has no configure actions")
}

func (h *Handler) Verify(ctx context.Context, host string, port int, expect map[string]any, t *c.Tracer) (*c.VerifyResult, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	start := time.Now()
	d := net.Dialer{Timeout: 3 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		t.Record("tcp_connect", "nc -z "+addr, start, "refused")
		return &c.VerifyResult{Healthy: false, Checks: []c.HealthCheck{{Check: "reachable", Passed: false, Detail: err.Error()}}}, nil
	}
	conn.Close()
	t.Record("tcp_connect", "nc -z "+addr, start, "open")
	return &c.VerifyResult{Healthy: true, Checks: []c.HealthCheck{{Check: "reachable", Passed: true, Detail: "TCP connect OK"}}}, nil
}

func (h *Handler) AvailableActions() []c.ActionDefinition { return nil }
