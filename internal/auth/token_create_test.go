package auth

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// runTokenCreate builds a fresh `token create` command, wires the root
// --api-url/--token persistent flags it depends on, parses args, and runs it.
func runTokenCreate(t *testing.T, args []string) (string, error) {
	t.Helper()
	cmd := newTokenCreateCmd()
	// Mimic the persistent root flags the command inherits at runtime.
	cmd.Flags().String("api-url", "", "")
	cmd.Flags().String("token", "", "")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestTokenCreatePassthroughMintsAndPrints(t *testing.T) {
	var gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/auth/api-keys") && r.Method == http.MethodPost {
			gotAuth = r.Header.Get("Authorization")
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			// The generated client only unmarshals the 201 body when the
			// response advertises JSON; an explicit WriteHeader otherwise
			// makes Go sniff text/plain. (Matches platform/client_test.go.)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"key":   map[string]any{"id": "00000000-0000-0000-0000-000000000001", "name": "dev-gw", "prefix": "yom_pat_abc", "scopes": []string{"gateway:tunnel"}, "created_at": "2026-06-07T00:00:00Z"},
				"token": "yom_pat_secret",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	t.Setenv("YOMIRO_API_URL", srv.URL)
	t.Setenv("YOMIRO_API_TOKEN", "my.jwt.here")
	out, err := runTokenCreate(t, []string{"--name", "dev-gw", "--scopes", "gateway:tunnel"})
	if err != nil {
		t.Fatalf("token create: %v\n%s", err, out)
	}
	if gotAuth != "Bearer my.jwt.here" {
		t.Fatalf("auth header = %q, want the passthrough JWT", gotAuth)
	}
	if !strings.Contains(gotBody, `"gateway:tunnel"`) {
		t.Fatalf("request body = %q, want gateway:tunnel scope", gotBody)
	}
	if !strings.Contains(out, "yom_pat_secret") {
		t.Fatalf("output %q should print the cleartext token", out)
	}
}

func TestTokenCreateRequiresNameAndScopes(t *testing.T) {
	// No --scopes -> validation error before any network call.
	t.Setenv("YOMIRO_API_TOKEN", "my.jwt.here")
	_, err := runTokenCreate(t, []string{"--name", "x"})
	if err == nil {
		t.Fatal("expected error when --scopes missing")
	}
}

func TestTokenCreateWebPrintsPickerToken(t *testing.T) {
	// Don't launch a real browser tab to the cli-pair page during tests.
	orig := openBrowser
	openBrowser = func(string) error { return nil }
	t.Cleanup(func() { openBrowser = orig })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/cli-pair") && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "ABCD-EFGH", "expires_at": "2026-01-01T00:00:00Z"})
		case strings.Contains(r.URL.Path, "/auth/cli-pair/"):
			tok := "yom_pat_web_secret"
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "ABCD-EFGH", "api_key_token": tok})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	t.Setenv("YOMIRO_API_URL", srv.URL)
	t.Setenv("YOMIRO_FRONTEND_URL", "http://localhost:5173")
	t.Setenv("YOMIRO_API_TOKEN", "my.jwt.here")
	out, err := runTokenCreate(t, []string{"--name", "x", "--scopes", "gateway:tunnel", "--web"})
	if err != nil {
		t.Fatalf("token create --web: %v\n%s", err, out)
	}
	if !strings.Contains(out, "yom_pat_web_secret") {
		t.Fatalf("output %q should print the picker-returned token", out)
	}
}
