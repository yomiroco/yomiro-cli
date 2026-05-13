package platform

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
)

// newRootForTest builds a fresh root command with the same persistent flags
// the real CLI registers, then attaches AddTo. Each test gets its own root
// so PersistentPreRunE state doesn't leak between cases.
func newRootForTest(t *testing.T) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "yomiro"}
	root.PersistentFlags().String("api-url", "", "")
	root.PersistentFlags().String("token", "", "")
	root.PersistentFlags().String("output", "json", "")
	if err := AddTo(root); err != nil {
		t.Fatalf("AddTo: %v", err)
	}
	root.SetOut(&discard{})
	root.SetErr(&discard{})
	return root
}

// TestAddTo_FlagOverridesAPIURL verifies the regression fix for the
// pre-Stage-3 wire.go behaviour where --api-url silently fell back to the
// credentials store. With PersistentPreRunE rebuilding the client per
// invocation, the flag value reaches the actual outbound HTTP request.
func TestAddTo_FlagOverridesAPIURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"count":0,"data":[]}`))
	}))
	defer srv.Close()

	root := newRootForTest(t)
	root.SetArgs([]string{"device", "list", "--api-url", srv.URL})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotPath == "" {
		t.Fatal("test server was not reached; --api-url override didn't take effect")
	}
}

// TestAddTo_EnvOverridesAPIURL covers YOMIRO_API_URL — the env var has to
// dominate the credentials store (so a saved login doesn't pin the binary
// to a single environment) but still yield to an explicit --api-url flag.
func TestAddTo_EnvOverridesAPIURL(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"count":0,"data":[]}`))
	}))
	defer srv.Close()

	t.Setenv("YOMIRO_API_URL", srv.URL)
	t.Setenv("YOMIRO_API_TOKEN", "envtoken-123")

	root := newRootForTest(t)
	root.SetArgs([]string{"device", "list"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if gotAuth != "Bearer envtoken-123" {
		t.Errorf("Authorization = %q, want Bearer envtoken-123", gotAuth)
	}
}

// TestAddTo_FlagBeatsEnv asserts the documented precedence: explicit flag
// wins over env var. An operator who passes --api-url is overriding their
// shell-level setup intentionally.
func TestAddTo_FlagBeatsEnv(t *testing.T) {
	flagHit := false
	flagSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flagHit = true
		_, _ = w.Write([]byte(`{"count":0,"data":[]}`))
	}))
	defer flagSrv.Close()
	envHit := false
	envSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envHit = true
		_, _ = w.Write([]byte(`{"count":0,"data":[]}`))
	}))
	defer envSrv.Close()

	t.Setenv("YOMIRO_API_URL", envSrv.URL)

	root := newRootForTest(t)
	root.SetArgs([]string{"device", "list", "--api-url", flagSrv.URL})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !flagHit {
		t.Error("--api-url should have routed the request")
	}
	if envHit {
		t.Error("YOMIRO_API_URL should NOT win when --api-url is explicit")
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
