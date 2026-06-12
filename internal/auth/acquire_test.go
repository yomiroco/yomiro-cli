package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"

	"github.com/yomiroco/yomiro-cli/internal/credentials"
	"github.com/yomiroco/yomiro-cli/internal/envprofile"
)

func TestMain(m *testing.M) {
	keyring.MockInit()
	os.Exit(m.Run())
}

// newTestCmd returns a command with the auth flags registered, so flag
// lookups in ResolveAuthConfig resolve.
func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.Flags().String("api-url", "", "")
	cmd.Flags().String("env", "", "")
	AddAuthFlags(cmd)
	return cmd
}

// mustResolve runs ResolveAuthConfig and fails the test on error.
func mustResolve(t *testing.T, cmd *cobra.Command) AuthConfig {
	t.Helper()
	cfg, err := ResolveAuthConfig(cmd)
	if err != nil {
		t.Fatalf("ResolveAuthConfig: %v", err)
	}
	return cfg
}

func TestResolveAuthConfigFlagBeatsEnvForAuth0Domain(t *testing.T) {
	t.Setenv("YOMIRO_AUTH0_DOMAIN", "env.example.com")
	cmd := newTestCmd()
	_ = cmd.ParseFlags([]string{"--auth0-domain", "flag.example.com"})
	cfg := mustResolve(t, cmd)
	if cfg.DC.Domain != "flag.example.com" {
		t.Fatalf("Domain = %q, want flag to win over env", cfg.DC.Domain)
	}
}

func TestResolveAuthConfigEnvBeatsProfileForAuth0Domain(t *testing.T) {
	t.Setenv("YOMIRO_AUTH0_DOMAIN", "env.example.com")
	cmd := newTestCmd()
	_ = cmd.ParseFlags(nil)
	cfg := mustResolve(t, cmd)
	if cfg.DC.Domain != "env.example.com" {
		t.Fatalf("Domain = %q, want env to win over profile default", cfg.DC.Domain)
	}
}

func TestResolveAuthConfigDefaultAuth0DomainFromProdProfile(t *testing.T) {
	prod, _ := envprofile.Lookup("prod")
	cmd := newTestCmd()
	_ = cmd.ParseFlags(nil)
	cfg := mustResolve(t, cmd)
	if cfg.DC.Domain != prod.Auth0Domain {
		t.Fatalf("Domain = %q, want prod profile default %q", cfg.DC.Domain, prod.Auth0Domain)
	}
}

func TestResolveAuthConfigAPIURLFlagBeatsEnv(t *testing.T) {
	t.Setenv("YOMIRO_API_URL", "https://env.example")
	cmd := newTestCmd()
	_ = cmd.ParseFlags([]string{"--api-url", "https://flag.example"})
	cfg := mustResolve(t, cmd)
	if cfg.APIURL != "https://flag.example" {
		t.Fatalf("APIURL = %q, want flag to win", cfg.APIURL)
	}
}

func TestResolveAuthConfigStoredAPIURLBeatsDefault(t *testing.T) {
	t.Setenv("YOMIRO_API_URL", "")
	store, err := credentials.New()
	if err != nil {
		t.Fatalf("credentials.New: %v", err)
	}
	if err := store.Save(credentials.Credentials{APIURL: "https://stored.example"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	cmd := newTestCmd()
	_ = cmd.ParseFlags(nil)
	cfg := mustResolve(t, cmd)
	if cfg.APIURL != "https://stored.example" {
		t.Fatalf("APIURL = %q, want stored value over default", cfg.APIURL)
	}
}

func TestResolveAuthConfigEnvAPIURLBeatsStored(t *testing.T) {
	store, err := credentials.New()
	if err != nil {
		t.Fatalf("credentials.New: %v", err)
	}
	if err := store.Save(credentials.Credentials{APIURL: "https://stored.example"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	t.Setenv("YOMIRO_API_URL", "https://env.example")
	cmd := newTestCmd()
	_ = cmd.ParseFlags(nil)
	cfg := mustResolve(t, cmd)
	if cfg.APIURL != "https://env.example" {
		t.Fatalf("APIURL = %q, want env over stored", cfg.APIURL)
	}
}

func TestResolveAuthConfigEnvDevYieldsDevProfile(t *testing.T) {
	// Clear stored creds and env overrides so the profile is the source.
	store, err := credentials.New()
	if err != nil {
		t.Fatalf("credentials.New: %v", err)
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	t.Setenv("YOMIRO_API_URL", "")
	t.Setenv("YOMIRO_AUTH0_AUDIENCE", "")
	t.Setenv("YOMIRO_AUTH0_CLIENT_ID", "")

	dev, _ := envprofile.Lookup("dev")
	cmd := newTestCmd()
	_ = cmd.ParseFlags([]string{"--env", "dev"})
	cfg := mustResolve(t, cmd)
	if cfg.APIURL != dev.APIURL {
		t.Errorf("APIURL = %q, want dev %q", cfg.APIURL, dev.APIURL)
	}
	if cfg.DC.Audience != dev.Audience {
		t.Errorf("Audience = %q, want dev %q", cfg.DC.Audience, dev.Audience)
	}
	if cfg.DC.ClientID != dev.Auth0ClientID {
		t.Errorf("ClientID = %q, want dev %q", cfg.DC.ClientID, dev.Auth0ClientID)
	}
}

func TestResolveAuthConfigFlagBeatsProfileAudience(t *testing.T) {
	cmd := newTestCmd()
	_ = cmd.ParseFlags([]string{"--env", "dev", "--audience", "https://flag.audience"})
	cfg := mustResolve(t, cmd)
	if cfg.DC.Audience != "https://flag.audience" {
		t.Fatalf("Audience = %q, want flag to beat dev profile", cfg.DC.Audience)
	}
}

func TestResolveAuthConfigEnvAudienceBeatsProfile(t *testing.T) {
	t.Setenv("YOMIRO_AUTH0_AUDIENCE", "https://env.audience")
	cmd := newTestCmd()
	_ = cmd.ParseFlags([]string{"--env", "dev"})
	cfg := mustResolve(t, cmd)
	if cfg.DC.Audience != "https://env.audience" {
		t.Fatalf("Audience = %q, want env to beat dev profile", cfg.DC.Audience)
	}
}

func TestResolveAuthConfigAPIURLFlagBeatsProfile(t *testing.T) {
	cmd := newTestCmd()
	_ = cmd.ParseFlags([]string{"--env", "dev", "--api-url", "https://flag.example"})
	cfg := mustResolve(t, cmd)
	if cfg.APIURL != "https://flag.example" {
		t.Fatalf("APIURL = %q, want flag to beat dev profile", cfg.APIURL)
	}
}

func TestResolveAuthConfigUnknownEnvErrors(t *testing.T) {
	cmd := newTestCmd()
	_ = cmd.ParseFlags([]string{"--env", "bogus"})
	if _, err := ResolveAuthConfig(cmd); err == nil {
		t.Fatal("ResolveAuthConfig(--env bogus) err=nil, want error")
	}
}

func TestAcquireJWTPassthroughReturnsExplicitToken(t *testing.T) {
	jwt, err := AcquireJWT(&bytes.Buffer{}, &DeviceCodeClient{}, "explicit.jwt.value")
	if err != nil {
		t.Fatalf("AcquireJWT: %v", err)
	}
	if jwt != "explicit.jwt.value" {
		t.Fatalf("jwt = %q, want passthrough", jwt)
	}
}

func TestAcquireJWTDeviceCodeFlow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth/device/code":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_code": "DEV", "user_code": "WXYZ",
				"verification_uri_complete": "https://verify", "expires_in": 5, "interval": 1,
			})
		case "/oauth/token":
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "device.jwt", "token_type": "Bearer"})
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	dc := &DeviceCodeClient{Domain: u.Host, Scheme: "http", HTTP: srv.Client()}
	jwt, err := AcquireJWT(&bytes.Buffer{}, dc, "")
	if err != nil {
		t.Fatalf("AcquireJWT: %v", err)
	}
	if jwt != "device.jwt" {
		t.Fatalf("jwt = %q, want device.jwt", jwt)
	}
}
