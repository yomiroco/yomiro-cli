package gw

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/zalando/go-keyring"

	"github.com/yomiroco/yomiro-cli/internal/envprofile"
	"github.com/spf13/cobra"
)

// wsURL converts an httptest server's http(s) URL to a ws(s) URL with path.
func wsURL(srvURL, path string) string {
	return "ws://" + strings.TrimPrefix(srvURL, "http://") + path
}

func TestProbeWSEndpointReachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		c.Close(websocket.StatusNormalClosure, "")
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ok, reason := probeWSEndpoint(ctx, wsURL(srv.URL, "/api/v1/gateway/ws"))
	if !ok {
		t.Fatalf("probeWSEndpoint ok=false reason=%q, want ok=true", reason)
	}
}

func TestProbeWSEndpoint404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ok, reason := probeWSEndpoint(ctx, wsURL(srv.URL, "/api/v1/gateway/ws"))
	if ok {
		t.Fatalf("probeWSEndpoint ok=true, want ok=false for 404")
	}
	if !strings.Contains(reason, "404") {
		t.Fatalf("reason = %q, want mention of 404", reason)
	}
}

func TestProbeWSEndpointUnreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Port 1 is privileged and not listening: connection refused, fast.
	ok, reason := probeWSEndpoint(ctx, "ws://127.0.0.1:1/api/v1/gateway/ws")
	if ok {
		t.Fatalf("probeWSEndpoint ok=true, want ok=false for unreachable host")
	}
	if !strings.Contains(reason, "unreachable") {
		t.Fatalf("reason = %q, want mention of unreachable", reason)
	}
}

// newEndpointCmd returns a command with just the --endpoint flag registered,
// so resolveEndpoint can be exercised without building the full init tree.
func newEndpointCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().String("endpoint", "", "")
	return cmd
}

func TestResolveEndpointDefaultsToProfileWSEndpoint(t *testing.T) {
	dev, _ := envprofile.Lookup("dev")
	cmd := newEndpointCmd()
	_ = cmd.ParseFlags(nil)
	if got := resolveEndpoint(cmd, dev); got != dev.WSEndpoint {
		t.Fatalf("resolveEndpoint = %q, want dev WS endpoint %q", got, dev.WSEndpoint)
	}
}

func TestResolveEndpointFlagBeatsProfile(t *testing.T) {
	prod, _ := envprofile.Lookup("prod")
	cmd := newEndpointCmd()
	_ = cmd.ParseFlags([]string{"--endpoint", "wss://custom.example/ws"})
	if got := resolveEndpoint(cmd, prod); got != "wss://custom.example/ws" {
		t.Fatalf("resolveEndpoint = %q, want flag value", got)
	}
}

// runInit builds the init command with the root --api-url/--token persistent
// flags it relies on, then executes it with args.
func runInit(t *testing.T, args []string) (string, error) {
	t.Helper()
	cmd := newInitCmd()
	cmd.Flags().String("api-url", "", "")
	cmd.Flags().String("env", "", "")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestGwInitFromLoginMintsGatewayTunnelAndWritesConfig(t *testing.T) {
	keyring.MockInit()
	// Isolate config + state to a temp dir (GwConfigFile/StateDir honour these).
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_STATE_HOME", tmp)
	t.Setenv("HOME", tmp)

	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/auth/api-keys") && r.Method == http.MethodPost {
			b, _ := readAll(r)
			gotBody = b
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"key":   map[string]any{"id": "00000000-0000-0000-0000-000000000002", "name": "gw-test", "prefix": "yom_pat_x", "scopes": []string{"gateway:tunnel"}, "created_at": "2026-06-07T00:00:00Z"},
				"token": "yom_pat_gateway_secret",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	t.Setenv("YOMIRO_API_URL", srv.URL)
	t.Setenv("YOMIRO_API_TOKEN", "operator.jwt") // passthrough -> no device-code

	// Point the endpoint at the local test server (which 404s on the WS path)
	// so the init probe stays hermetic — no real-network dial — and exercises
	// the non-fatal warning path.
	wsEndpoint := wsURL(srv.URL, "/api/v1/gateway/ws")
	out, err := runInit(t, []string{"--from-login", "--gateway-id", "gw-test", "--db-url", "postgres://u:p@h/db", "--endpoint", wsEndpoint})
	if err != nil {
		t.Fatalf("gw init --from-login: %v\n%s", err, out)
	}
	if !strings.Contains(gotBody, `"gateway:tunnel"`) {
		t.Fatalf("mint body = %q, want gateway:tunnel", gotBody)
	}
	if !strings.Contains(out, "⚠ Endpoint check:") || !strings.Contains(out, "404") {
		t.Fatalf("output missing actionable endpoint warning, got:\n%s", out)
	}
	stored, err := keyring.Get(keyringServiceGw, "gw-test")
	if err != nil || stored != "yom_pat_gateway_secret" {
		t.Fatalf("keyring token = %q err=%v, want minted token", stored, err)
	}
	var found bool
	_ = filepath.Walk(tmp, func(p string, info os.FileInfo, _ error) error {
		if info != nil && info.Name() == "gw.yaml" {
			found = true
		}
		return nil
	})
	if !found {
		t.Fatal("expected gw.yaml to be written")
	}
}

func TestGwInitRejectsBothTokenAndFromLogin(t *testing.T) {
	_, err := runInit(t, []string{"--token", "yom_pat_x", "--from-login"})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("err = %v, want mutually-exclusive error", err)
	}
}

func TestGwInitRejectsNeitherTokenNorFromLogin(t *testing.T) {
	_, err := runInit(t, []string{})
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("err = %v, want one-of-required error", err)
	}
}

// readAll drains a request body to a string.
func readAll(r *http.Request) (string, error) {
	buf := new(bytes.Buffer)
	_, err := buf.ReadFrom(r.Body)
	return buf.String(), err
}
