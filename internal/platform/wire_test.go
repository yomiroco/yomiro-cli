package platform

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/spf13/cobra"
)

// TestAddTo_WiresIntendedGroups is the group-wiring policy acceptance test: it
// asserts AddTo wires exactly the intended set of public command groups — the
// generated allowlist (publicGroups) plus the `skill` stub — and nothing else.
// A bare root is used so only the AddTo-added groups are present (auth/gw/
// version are added in cmd/yomiro/root.go, not here). Making a group public or
// dropping one must be a deliberate edit to publicGroups, which this flags.
func TestAddTo_WiresIntendedGroups(t *testing.T) {
	root := &cobra.Command{Use: "yomiro"}
	if err := AddTo(root); err != nil {
		t.Fatalf("AddTo: %v", err)
	}

	want := []string{
		"ai-config", "capture", "dashboard", "data-source", "device",
		"device-group", "incident", "location", "skill", "user",
		"agent", "team", "alert", "ai-provider", "inspection-profile",
		"model", "ref-sheet", "otel-endpoint", "analytics", "organization",
		"entity-history", "jetson-nano",
	}
	sort.Strings(want)

	var got []string
	for _, c := range root.Commands() {
		got = append(got, c.Name())
	}
	sort.Strings(got)

	if len(got) != len(want) {
		t.Fatalf("wired groups = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("wired groups = %v, want %v", got, want)
		}
	}
}

// TestPublicGroupsCount guards the allowlist size so adding or removing an
// entry forces a conscious update to the wired-set expectation above.
func TestPublicGroupsCount(t *testing.T) {
	if len(publicGroups) != 21 {
		t.Fatalf("len(publicGroups) = %d, want 21", len(publicGroups))
	}
}

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

// TestAddTo_WiresAiConfigGroup verifies the ai-config group (which carries
// capture_config — "capture rules") is reachable, with the create-or-replace
// and update subcommands that accept a request body.
func TestAddTo_WiresAiConfigGroup(t *testing.T) {
	root := newRootForTest(t)

	group, _, err := root.Find([]string{"ai-config"})
	if err != nil || group == nil || group == root {
		t.Fatalf("ai-config group not wired: cmd=%v err=%v", group, err)
	}
	for _, sub := range []string{"create-or-replace", "update", "get", "delete"} {
		c, _, err := group.Find([]string{sub})
		if err != nil || c == nil || c == group {
			t.Fatalf("ai-config %s subcommand missing: %v", sub, err)
		}
	}
	// The body-bearing subcommands expose the standard --json-body flag.
	upd, _, _ := group.Find([]string{"update"})
	if upd.Flag("json-body") == nil {
		t.Error("ai-config update should expose --json-body")
	}
}

// TestAddTo_WiresOnboardingGroups verifies the plant-onboarding groups are
// reachable under their singular CLI names.
func TestAddTo_WiresOnboardingGroups(t *testing.T) {
	root := newRootForTest(t)
	for _, group := range []string{"location", "device-group", "data-source", "user"} {
		cmd, _, err := root.Find([]string{group})
		if err != nil || cmd == nil || cmd == root {
			t.Fatalf("%s group not wired: cmd=%v err=%v", group, cmd, err)
		}
		create, _, err := cmd.Find([]string{"create"})
		if err != nil || create == nil || create == cmd {
			t.Fatalf("%s create subcommand missing: %v", group, err)
		}
	}
}

// TestEnvURLConflict covers the --env / YOMIRO_API_URL shadowing warning.
func TestEnvURLConflict(t *testing.T) {
	const local = "http://localhost:8000"
	const dev = "https://api.dev.yomiro.io"
	tests := []struct {
		name                                    string
		envName, profileURL, envVarURL, apiFlag string
		wantWarn                                bool
	}{
		{"shadowed: env flag local but YOMIRO_API_URL=dev", "local", local, dev, "", true},
		{"no env flag set", "", local, dev, "", false},
		{"YOMIRO_API_URL unset", "local", local, "", "", false},
		{"urls agree", "local", local, local, "", false},
		{"explicit --api-url silences warning", "local", local, dev, "http://x", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := envURLConflict(tt.envName, tt.profileURL, tt.envVarURL, tt.apiFlag)
			if (got != "") != tt.wantWarn {
				t.Fatalf("envURLConflict(%q,%q,%q,%q) = %q; wantWarn=%v",
					tt.envName, tt.profileURL, tt.envVarURL, tt.apiFlag, got, tt.wantWarn)
			}
		})
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
