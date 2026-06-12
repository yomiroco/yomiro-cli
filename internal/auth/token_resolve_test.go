package auth

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/yomiroco/yomiro-cli/internal/credentials"
)

// newTokenTestCmd returns a command with the persistent --api-url/--token flags
// (owned by the root in production) so loadClient's flag lookups resolve.
func newTokenTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.Flags().String("api-url", "", "")
	cmd.Flags().String("token", "", "")
	return cmd
}

func saveStored(t *testing.T, c credentials.Credentials) {
	t.Helper()
	store, err := credentials.New()
	if err != nil {
		t.Fatalf("credentials.New: %v", err)
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if err := store.Save(c); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

// The regression this guards: `auth token list/revoke` used to read ONLY the
// stored credentials, so YOMIRO_API_URL / --api-url were silently ignored.
func TestLoadClientEnvAPIURLBeatsStored(t *testing.T) {
	saveStored(t, credentials.Credentials{APIURL: "https://stored.example", Token: "stored-tok"})
	t.Setenv("YOMIRO_API_URL", "https://env.example")

	cmd := newTokenTestCmd()
	_ = cmd.ParseFlags(nil)
	pc, err := loadClient(cmd)
	if err != nil {
		t.Fatalf("loadClient: %v", err)
	}
	if pc.BaseURL() != "https://env.example" {
		t.Fatalf("BaseURL = %q, want env to win over stored", pc.BaseURL())
	}
}

func TestLoadClientFlagAPIURLBeatsEnvAndStored(t *testing.T) {
	saveStored(t, credentials.Credentials{APIURL: "https://stored.example", Token: "stored-tok"})
	t.Setenv("YOMIRO_API_URL", "https://env.example")

	cmd := newTokenTestCmd()
	_ = cmd.ParseFlags([]string{"--api-url", "https://flag.example"})
	pc, err := loadClient(cmd)
	if err != nil {
		t.Fatalf("loadClient: %v", err)
	}
	if pc.BaseURL() != "https://flag.example" {
		t.Fatalf("BaseURL = %q, want flag to win", pc.BaseURL())
	}
}

// "Not logged in" UX must survive: nothing stored, no env/flag token.
func TestLoadClientNotLoggedIn(t *testing.T) {
	store, err := credentials.New()
	if err != nil {
		t.Fatalf("credentials.New: %v", err)
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	t.Setenv("YOMIRO_API_URL", "")
	t.Setenv("YOMIRO_API_TOKEN", "")

	cmd := newTokenTestCmd()
	_ = cmd.ParseFlags(nil)
	if _, err := loadClient(cmd); err == nil {
		t.Fatal("expected 'not logged in' error when nothing is stored")
	}
}

// A token from env (no stored credential) is enough — must NOT report "not
// logged in", and the client targets the resolved (default) URL.
func TestLoadClientEnvTokenOnly(t *testing.T) {
	store, err := credentials.New()
	if err != nil {
		t.Fatalf("credentials.New: %v", err)
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	t.Setenv("YOMIRO_API_TOKEN", "env-tok")
	t.Setenv("YOMIRO_API_URL", "")

	cmd := newTokenTestCmd()
	_ = cmd.ParseFlags(nil)
	pc, err := loadClient(cmd)
	if err != nil {
		t.Fatalf("loadClient: %v", err)
	}
	if pc.BaseURL() != credentials.DefaultAPIURL {
		t.Fatalf("BaseURL = %q, want default", pc.BaseURL())
	}
}
