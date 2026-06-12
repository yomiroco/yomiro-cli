package envprofile

import (
	"testing"

	"github.com/yomiroco/yomiro-cli/internal/credentials"
	"github.com/spf13/cobra"
)

func TestLookupTableValues(t *testing.T) {
	tests := []struct {
		name string
		want Profile
	}{
		{
			name: "prod",
			want: Profile{
				Name:          "prod",
				APIURL:        credentials.DefaultAPIURL,
				Auth0Domain:   "yomiro.eu.auth0.com",
				Auth0ClientID: "sUIOMvj0aaIRFbOPDcYUZ8bM5yCbWnA5",
				Audience:      credentials.DefaultAPIURL,
				WSEndpoint:    "wss://api.yomiro.io/api/v1/gateway/ws",
				FrontendURL:   "",
			},
		},
		{
			name: "dev",
			want: Profile{
				Name:          "dev",
				APIURL:        "https://api.dev.yomiro.io",
				Auth0Domain:   "yomiro.eu.auth0.com",
				Auth0ClientID: "oGQgnpBtymVPMJMkHbvP433o6n8sqO4e",
				Audience:      "https://api.dev.yomiro.io",
				WSEndpoint:    "wss://api.dev.yomiro.io/api/v1/gateway/ws",
				FrontendURL:   "",
			},
		},
		{
			name: "staging",
			want: Profile{
				Name:          "staging",
				APIURL:        "https://api.staging.yomiro.io",
				Auth0Domain:   "yomiro.eu.auth0.com",
				Auth0ClientID: "REPLACE_ME_STAGING_AUTH0_CLIENT_ID",
				Audience:      "https://api.staging.yomiro.io",
				WSEndpoint:    "wss://api.staging.yomiro.io/api/v1/gateway/ws",
				FrontendURL:   "",
			},
		},
		{
			name: "local",
			want: Profile{
				Name:          "local",
				APIURL:        "http://localhost:8000",
				Auth0Domain:   "yomiro.eu.auth0.com",
				Auth0ClientID: "oGQgnpBtymVPMJMkHbvP433o6n8sqO4e",
				Audience:      "https://api.dev.yomiro.io",
				WSEndpoint:    "ws://localhost:8000/api/v1/gateway/ws",
				FrontendURL:   "http://localhost:5173",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Lookup(tt.name)
			if !ok {
				t.Fatalf("Lookup(%q) ok=false, want true", tt.name)
			}
			if got != tt.want {
				t.Fatalf("Lookup(%q) = %+v, want %+v", tt.name, got, tt.want)
			}
		})
	}
}

func TestLookupUnknown(t *testing.T) {
	if _, ok := Lookup("bogus"); ok {
		t.Fatal("Lookup(bogus) ok=true, want false")
	}
}

// newEnvCmd returns a command with the --env flag registered so Active(cmd) can
// read it.
func newEnvCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().String("env", "", "")
	return cmd
}

func TestActiveDefaultsToProd(t *testing.T) {
	t.Setenv("YOMIRO_ENV", "")
	got, explicit, err := Active(newEnvCmd())
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if got.Name != "prod" {
		t.Fatalf("Active = %q, want prod", got.Name)
	}
	if explicit {
		t.Fatalf("explicit = true, want false for implicit prod default")
	}
}

// TestActiveExplicitFlag verifies the explicit bool tracks whether the operator
// actively selected an env, while the returned profile always carries the real
// per-env values (never mutated/blanked).
func TestActiveExplicitFlag(t *testing.T) {
	tests := []struct {
		name         string
		envVar       string
		flagArgs     []string
		wantName     string
		wantExplicit bool
	}{
		{name: "bare implicit prod", wantName: "prod", wantExplicit: false},
		{name: "explicit --env prod", flagArgs: []string{"--env", "prod"}, wantName: "prod", wantExplicit: true},
		{name: "YOMIRO_ENV=prod is explicit", envVar: "prod", wantName: "prod", wantExplicit: true},
		{name: "explicit --env dev", flagArgs: []string{"--env", "dev"}, wantName: "dev", wantExplicit: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("YOMIRO_ENV", tt.envVar)
			cmd := newEnvCmd()
			if err := cmd.ParseFlags(tt.flagArgs); err != nil {
				t.Fatalf("ParseFlags: %v", err)
			}
			got, explicit, err := Active(cmd)
			if err != nil {
				t.Fatalf("Active: %v", err)
			}
			if got.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tt.wantName)
			}
			if explicit != tt.wantExplicit {
				t.Errorf("explicit = %v, want %v", explicit, tt.wantExplicit)
			}
			// Profile is never blanked: APIURL always carries the real value.
			want, _ := Lookup(tt.wantName)
			if got.APIURL != want.APIURL || got.APIURL == "" {
				t.Errorf("APIURL = %q, want real value %q", got.APIURL, want.APIURL)
			}
		})
	}
}

func TestActiveEnvVar(t *testing.T) {
	t.Setenv("YOMIRO_ENV", "dev")
	got, explicit, err := Active(newEnvCmd())
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if got.Name != "dev" {
		t.Fatalf("Active = %q, want dev", got.Name)
	}
	if !explicit {
		t.Fatalf("explicit = false, want true for YOMIRO_ENV=dev")
	}
}

func TestActiveFlagBeatsEnv(t *testing.T) {
	t.Setenv("YOMIRO_ENV", "dev")
	cmd := newEnvCmd()
	if err := cmd.ParseFlags([]string{"--env", "local"}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	got, _, err := Active(cmd)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	if got.Name != "local" {
		t.Fatalf("Active = %q, want local (flag beats env)", got.Name)
	}
}

func TestActiveUnknownErrors(t *testing.T) {
	t.Setenv("YOMIRO_ENV", "")
	cmd := newEnvCmd()
	if err := cmd.ParseFlags([]string{"--env", "bogus"}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if _, _, err := Active(cmd); err == nil {
		t.Fatal("Active(bogus) err=nil, want error")
	}
}

func TestActiveToleratesNilAndMissingFlag(t *testing.T) {
	t.Setenv("YOMIRO_ENV", "")
	if got, explicit, err := Active(nil); err != nil || got.Name != "prod" || explicit {
		t.Fatalf("Active(nil) = %q, explicit=%v, %v; want prod, false, nil", got.Name, explicit, err)
	}
	// cmd without an "env" flag should not panic.
	bare := &cobra.Command{Use: "x"}
	if got, explicit, err := Active(bare); err != nil || got.Name != "prod" || explicit {
		t.Fatalf("Active(bare) = %q, explicit=%v, %v; want prod, false, nil", got.Name, explicit, err)
	}
}

// TestActiveProfileFeedsResolve documents the end-to-end precedence that the
// explicit bool guards: a bare invocation lets stored creds beat the compiled
// default (profileAPIURL=""), while an explicit --env dev beats stored.
func TestActiveProfileFeedsResolve(t *testing.T) {
	// Bare: implicit prod, explicit=false → caller passes "" → stored wins.
	t.Setenv("YOMIRO_ENV", "")
	_, explicit, err := Active(newEnvCmd())
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	bareProfileAPIURL := ""
	if explicit {
		bareProfileAPIURL = "<would-be-prod>"
	}
	if bareProfileAPIURL != "" {
		t.Fatalf("bare invocation must yield empty profileAPIURL, got %q", bareProfileAPIURL)
	}

	// Explicit --env dev: explicit=true → caller passes dev APIURL.
	cmd := newEnvCmd()
	if err := cmd.ParseFlags([]string{"--env", "dev"}); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	prof, explicit, err := Active(cmd)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	devProfileAPIURL := ""
	if explicit {
		devProfileAPIURL = prof.APIURL
	}
	dev, _ := Lookup("dev")
	if devProfileAPIURL != dev.APIURL {
		t.Fatalf("explicit --env dev must yield dev APIURL, got %q", devProfileAPIURL)
	}
}
