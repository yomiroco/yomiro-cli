package auth

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yomiroco/yomiro-cli/internal/platform"
)

// cliPairGet is the subset of CliPairPublic the CLI poll cares about.
type cliPairGet struct {
	Code         string  `json:"code"`
	AuthorizedAt *string `json:"authorized_at"`
	APIKeyToken  *string `json:"api_key_token"`
}

func TestCreateCLIPairRequestPostsHostnameAndScopes(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		buf, _ := io.ReadAll(r.Body)
		gotBody = string(buf)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "ABCD-EFGH-JKMN", "expires_at": "2026-01-01T00:00:00Z"})
	}))
	defer srv.Close()

	pc := platform.New(srv.URL, "auth0-jwt")
	code, err := pc.CreateCLIPairRequest("cli-host", []string{"read:devices"})
	if err != nil {
		t.Fatalf("CreateCLIPairRequest: %v", err)
	}
	if code != "ABCD-EFGH-JKMN" {
		t.Fatalf("code = %q", code)
	}
	if gotPath != "/api/v1/auth/cli-pair" {
		t.Fatalf("path = %q", gotPath)
	}
	if !strings.Contains(gotBody, `"hostname":"cli-host"`) || !strings.Contains(gotBody, `"read:devices"`) {
		t.Fatalf("body = %q", gotBody)
	}
}

func TestPollCLIPairReturnsTokenAfterAuthorization(t *testing.T) {
	var calls int
	token := "yk_live_abc123"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer auth0-jwt" {
			t.Errorf("missing bearer header, got %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			// First poll: record exists but not authorized yet.
			_ = json.NewEncoder(w).Encode(cliPairGet{Code: "ABCD"})
			return
		}
		_ = json.NewEncoder(w).Encode(cliPairGet{Code: "ABCD", APIKeyToken: &token})
	}))
	defer srv.Close()

	pc := platform.New(srv.URL, "auth0-jwt")
	got, err := pc.PollCLIPair("ABCD", 5*time.Second, time.Millisecond)
	if err != nil {
		t.Fatalf("PollCLIPair: %v", err)
	}
	if got != token {
		t.Fatalf("token = %q, want %q", got, token)
	}
	if calls < 2 {
		t.Fatalf("expected at least 2 polls, got %d", calls)
	}
}

func TestPollCLIPairDeniedReturnsClearError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
		_, _ = w.Write([]byte(`{"detail":"Pairing was denied"}`))
	}))
	defer srv.Close()

	pc := platform.New(srv.URL, "auth0-jwt")
	_, err := pc.PollCLIPair("ABCD", 5*time.Second, time.Millisecond)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, platform.ErrCLIPairGone) {
		t.Fatalf("error = %v, want ErrCLIPairGone", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "denied") {
		t.Fatalf("error %q should mention denied", err)
	}
}

func TestResolveFrontendURLPrefersOverride(t *testing.T) {
	// An explicit override wins even when the API host is derivable, and any
	// trailing slash is trimmed so the "%s/cli-pair/%s" join stays clean.
	got, err := resolveFrontendURL("http://localhost:5173/", "https://api.dev.yomiro.io")
	if err != nil {
		t.Fatalf("resolveFrontendURL: %v", err)
	}
	if got != "http://localhost:5173" {
		t.Fatalf("got %q, want override with trailing slash trimmed", got)
	}
}

func TestResolveFrontendURLFallsBackToAPIDerivation(t *testing.T) {
	got, err := resolveFrontendURL("", "https://api.dev.yomiro.io")
	if err != nil {
		t.Fatalf("resolveFrontendURL: %v", err)
	}
	if got != "https://dev.yomiro.io" {
		t.Fatalf("got %q, want derived dev URL", got)
	}
}

func TestResolveFrontendURLErrorsWhenUnresolvable(t *testing.T) {
	// localhost API host can't be translated and no override is given.
	_, err := resolveFrontendURL("", "http://localhost:8000")
	if err == nil {
		t.Fatal("expected error when frontend URL is neither overridden nor derivable")
	}
	if !strings.Contains(err.Error(), "YOMIRO_FRONTEND_URL") {
		t.Fatalf("error %q should point at YOMIRO_FRONTEND_URL", err)
	}
}

func TestPollCLIPairTimesOutCleanly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always unauthorized — never returns a token.
		_ = json.NewEncoder(w).Encode(cliPairGet{Code: "ABCD"})
	}))
	defer srv.Close()

	pc := platform.New(srv.URL, "auth0-jwt")
	_, err := pc.PollCLIPair("ABCD", 100*time.Millisecond, 10*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if errors.Is(err, platform.ErrCLIPairGone) {
		t.Fatalf("timeout should not be ErrCLIPairGone, got %v", err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "timed out") {
		t.Fatalf("error %q should mention timeout", err)
	}
}
