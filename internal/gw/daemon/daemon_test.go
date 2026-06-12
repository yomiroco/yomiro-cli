package daemon

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yomiroco/yomiro-cli/internal/config"
	"github.com/zalando/go-keyring"
)

// TestReloadRereadsConfig proves `gw reload` re-reads gw.yaml into the live
// daemon (the bug was that reload was a no-op). Uses an empty DB URL so no
// pool rebuild is attempted.
func TestReloadRereadsConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gw.yaml")

	cfgA := &config.GwConfig{}
	cfgA.Platform.Endpoint = "ws://old.example/api/v1/gateway/ws"
	cfgA.Gateway.ID = "gw-test"
	if err := config.SaveGwConfig(path, cfgA); err != nil {
		t.Fatalf("save cfgA: %v", err)
	}

	d := New(cfgA, nil)
	d.ConfigPath = path

	// Operator edits gw.yaml to point at a new endpoint.
	cfgB := &config.GwConfig{}
	cfgB.Platform.Endpoint = "ws://new.example/api/v1/gateway/ws"
	cfgB.Gateway.ID = "gw-test"
	if err := config.SaveGwConfig(path, cfgB); err != nil {
		t.Fatalf("save cfgB: %v", err)
	}

	if err := d.reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := d.config().Platform.Endpoint; got != cfgB.Platform.Endpoint {
		t.Fatalf("endpoint = %q, want reloaded %q", got, cfgB.Platform.Endpoint)
	}
	// reload must signal the reconnect loop (so it redials immediately rather
	// than waiting out the backoff).
	select {
	case <-d.reloadCh:
	default:
		t.Error("reload should have signalled reloadCh for an immediate reconnect")
	}
}

// TestReloadMissingPathErrors verifies reload surfaces a clear error instead of
// silently succeeding when it has no config path.
func TestReloadMissingPathErrors(t *testing.T) {
	d := New(&config.GwConfig{}, nil)
	if err := d.reload(); err == nil {
		t.Fatal("expected error when ConfigPath is empty")
	}
}

// TestRunLogsToProvidedWriter proves the reconnect loop writes its status lines
// to Daemon.Log (so `gw run` can tee them to the file `gw logs` tails). It
// drives one failed connection against an unreachable endpoint and asserts the
// disconnect line lands in the buffer.
func TestRunLogsToProvidedWriter(t *testing.T) {
	keyring.MockInit()
	if err := keyring.Set("io.yomiro.gw", "gw-test", "tok"); err != nil {
		t.Fatalf("keyring set: %v", err)
	}
	// Isolate the control socket so we don't collide with a real daemon. Use a
	// short /tmp path — macOS caps unix-socket paths at ~104 chars, which the
	// default t.TempDir() can exceed once "/yomiro/gw.sock" is appended.
	cacheDir, err := os.MkdirTemp("/tmp", "ygw")
	if err != nil {
		t.Fatalf("mkdir cache: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(cacheDir) })
	t.Setenv("XDG_CACHE_HOME", cacheDir)

	// A server that rejects the WS handshake (404) makes the dial fail fast and
	// deterministically — the same shape as pointing at the wrong endpoint.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cfg := &config.GwConfig{}
	cfg.Platform.Endpoint = "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/v1/gateway/ws"
	cfg.Platform.TokenRef = "keyring:io.yomiro.gw/gw-test"
	cfg.Gateway.ID = "gw-test"
	cfg.Daemon.ReconnectMaxBackoffS = 1

	var buf bytes.Buffer
	d := New(cfg, nil)
	d.Log = &buf

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()

	// One dial fails fast and is logged; the loop then enters its backoff wait,
	// during which we cancel. 500ms < the 1s backoff, so it logs exactly once.
	time.Sleep(500 * time.Millisecond)
	cancel()
	runErr := <-done

	if !strings.Contains(buf.String(), "tunnel disconnected") {
		t.Fatalf("log output %q should contain the disconnect line (Run returned: %v)", buf.String(), runErr)
	}
}
