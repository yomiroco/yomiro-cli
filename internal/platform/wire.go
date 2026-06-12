// Package platform wires the generated platform commands and the override
// registry into the CLI's root command.
package platform

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"github.com/yomiroco/yomiro-cli/internal/credentials"
	"github.com/yomiroco/yomiro-cli/internal/envprofile"
	"github.com/yomiroco/yomiro-cli/internal/platform/client"
	"github.com/yomiroco/yomiro-cli/internal/platform/generated"
	"github.com/yomiroco/yomiro-cli/internal/platform/overrides"
)

// AddTo registers the generated platform commands and their overrides under
// the given root command.
//
// The platform client is constructed lazily: AddTo wires a
// `PersistentPreRunE` that rebuilds it on each invocation from the parsed
// --api-url / --token flags, falling back to the credentials store. Without
// this, those root-level flags were declared but silently ignored because
// the client had already been frozen at command-tree construction time.
//
// The function passed to the generated New*Cmd constructors returns the
// current client, so a missing/empty credentials store still produces a
// usable command tree (with an empty bearer token) and `--help` works even
// before `yomiro auth login`.
func AddTo(root *cobra.Command) error {
	// Initial best-effort client so --help works pre-login.
	apiClient, err := buildClient(credentials.DefaultAPIURL, "")
	if err != nil {
		return err
	}
	current := apiClient
	getClient := func() *client.ClientWithResponses { return current }

	priorPreRunE := root.PersistentPreRunE
	root.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if priorPreRunE != nil {
			if err := priorPreRunE(cmd, args); err != nil {
				return err
			}
		}
		apiURL, token, err := resolveCredentials(cmd)
		if err != nil {
			return err
		}
		c, err := buildClient(apiURL, token)
		if err != nil {
			return err
		}
		current = c
		return nil
	}

	// Skill has no matching generated command — there's no POST /skills/install
	// in the spec (the daemon manages local skill state, not the platform
	// API). Stub a placeholder root so the override registry has a home.
	skillCmd := &cobra.Command{Use: "skill", Short: "Manage skills"}
	root.AddCommand(skillCmd)

	// Wire the public allowlist. Build each generated group, apply its
	// singular-noun Use override, and register it under root. The group names
	// for the override-registry pass below are derived from the same list (plus
	// the skill stub) so they can never drift from what's actually wired.
	groupNames := []string{skillCmd.Name()}
	for _, spec := range publicGroups {
		cmd := spec.new(getClient)
		if spec.use != "" {
			cmd.Use = spec.use
		}
		root.AddCommand(cmd)
		groupNames = append(groupNames, cmd.Name())
	}

	for _, group := range groupNames {
		groupCmd, _, err := root.Find([]string{group})
		if err != nil || groupCmd == nil || groupCmd == root {
			continue
		}
		for _, ov := range overrides.AllInGroup(group) {
			existing, _, _ := groupCmd.Find([]string{ov.Name()})
			if existing != nil && existing != groupCmd {
				groupCmd.RemoveCommand(existing)
			}
			groupCmd.AddCommand(ov)
		}
	}
	return nil
}

// groupSpec is one entry in the public-command allowlist.
type groupSpec struct {
	new func(func() *client.ClientWithResponses) *cobra.Command // generated constructor
	use string                                                  // singular-noun Use override ("" keeps the generated plural)
	why string                                                  // one-line rationale: why this group is operator-facing
}

// publicGroups is the allowlist of generated command groups exposed as
// operator-facing CLI commands. Only ~20 of the ~51 generated groups are
// public; the rest are internal/test/web-only surfaces. Making a group public
// (or removing one) is a one-line change here. Keep the singular-noun Use
// convention. The `skill` group is wired separately (it has no generated
// constructor — the daemon manages skills, there is no platform API).
var publicGroups = []groupSpec{
	{new: generated.NewDashboardsCmd, use: "dashboard", why: "operators read/author dashboards built on captured data"},
	{new: generated.NewCapturesCmd, use: "capture", why: "the captured frames/events operators triage are the core artifact"},
	{new: generated.NewIncidentsCmd, use: "incident", why: "operators review and resolve incidents raised from captures"},
	{new: generated.NewDevicesCmd, use: "device", why: "operators register and manage the edge devices that produce captures"},
	{new: generated.NewAiConfigCmd, use: "ai-config", why: "keyed per device-group, carries capture_config — the entry point for authoring what triggers captures"},
	{new: generated.NewLocationsCmd, use: "location", why: "onboarding: a plant's location hierarchy"},
	{new: generated.NewDeviceGroupsCmd, use: "device-group", why: "onboarding: groups devices + ai-config attach to"},
	{new: generated.NewDataSourcesCmd, use: "data-source", why: "onboarding: data sources, incl. gateway-proxied"},
	{new: generated.NewUsersCmd, use: "user", why: "onboarding: user records"},
	{new: generated.NewAgentsCmd, use: "agent", why: "operators author agent configs, hooks, and heartbeat tasks"},
	{new: generated.NewTeamsCmd, use: "team", why: "operators manage teams and their membership"},
	{new: generated.NewAlertsCmd, use: "alert", why: "operators create/enable/acknowledge alert rules and events"},
	{new: generated.NewAiProvidersCmd, use: "ai-provider", why: "operators configure AI providers / BYOK and discover models"},
	{new: generated.NewInspectionProfilesCmd, use: "inspection-profile", why: "domain-core: golden-image and inspection profiles"},
	{new: generated.NewModelsCmd, use: "model", why: "operators list/upload/download inspection models"},
	{new: generated.NewRefSheetsCmd, use: "ref-sheet", why: "domain: versioned inspection reference sheets"},
	{new: generated.NewOtelEndpointsCmd, use: "otel-endpoint", why: "operators configure OTel ingestion endpoints (config, not the receiver)"},
	{new: generated.NewAnalyticsCmd, use: "", why: "read-only summaries/trends/grafana links for scripted reporting"},
	{new: generated.NewOrganizationsCmd, use: "organization", why: "operators read/update their org settings (logo, mqtt config)"},
	{new: generated.NewEntityHistoryCmd, use: "", why: "recent-entity history and soft-delete restore"},
}

// resolveCredentials chooses the API URL and bearer token for the next
// request by delegating to credentials.Resolve. Precedence, highest to lowest:
//
//  1. --api-url / --token flags (explicit operator intent)
//  2. YOMIRO_API_URL / YOMIRO_API_TOKEN env vars (so the same binary can
//     be pointed at dev/local/prod from a shell or CI without re-auth)
//  3. The active --env / YOMIRO_ENV profile's API URL (api-url only)
//  4. Credentials store (yomiro auth login)
//  5. credentials.DefaultAPIURL
//
// Empty token is acceptable — help and the gateway subcommands don't need
// it, and missing-auth errors surface from the server. An unknown --env name
// is the only error this returns.
func resolveCredentials(cmd *cobra.Command) (apiURL, token string, err error) {
	prof, explicit, err := envprofile.Active(cmd)
	if err != nil {
		return "", "", err
	}
	profileAPIURL := ""
	if explicit {
		profileAPIURL = prof.APIURL
	}
	// Surface the footgun where an explicit --env is silently shadowed by a
	// YOMIRO_API_URL exported in the shell (env var outranks the profile by
	// design — see the precedence doc above). Without this warning the request
	// quietly goes to the wrong backend and fails with an opaque auth error.
	if msg := envURLConflict(changedFlag(cmd, "env"), prof.APIURL, os.Getenv("YOMIRO_API_URL"), changedFlag(cmd, "api-url")); msg != "" {
		fmt.Fprintln(os.Stderr, msg)
	}
	apiURL, token = credentials.Resolve(changedFlag(cmd, "api-url"), changedFlag(cmd, "token"), profileAPIURL)
	return apiURL, token, nil
}

// envURLConflict returns a warning when an explicit --env flag's target API URL
// is being overridden by a differing YOMIRO_API_URL, or "" when there is nothing
// to warn about. It is a pure function of its inputs so it can be table-tested.
//
// No warning fires when: --env was not set via the flag (envName == ""), an
// explicit --api-url is present (the operator already steered the URL directly),
// YOMIRO_API_URL is unset, or the two URLs agree.
func envURLConflict(envName, profileURL, envVarURL, apiURLFlag string) string {
	if envName == "" || apiURLFlag != "" || envVarURL == "" || envVarURL == profileURL {
		return ""
	}
	return fmt.Sprintf(
		"warning: --env %s targets %s, but YOMIRO_API_URL=%s takes precedence (env var outranks --env). "+
			"Unset YOMIRO_API_URL or pass --api-url to override.",
		envName, profileURL, envVarURL,
	)
}

// changedFlag returns the value of a flag only if it was explicitly set on
// cmd; otherwise "". This is the "explicit flag override" input expected by
// credentials.Resolve.
func changedFlag(cmd *cobra.Command, name string) string {
	if cmd == nil {
		return ""
	}
	if f := cmd.Flag(name); f != nil && f.Changed {
		return f.Value.String()
	}
	return ""
}

func buildClient(apiURL, token string) (*client.ClientWithResponses, error) {
	c, err := client.NewClientWithResponses(apiURL, client.WithRequestEditorFn(bearer(token)))
	if err != nil {
		return nil, fmt.Errorf("build platform client: %w", err)
	}
	return c, nil
}

// bearer returns a RequestEditorFn that attaches a Bearer token to outbound
// requests. An empty token is a no-op so help/build flows work pre-login.
func bearer(token string) client.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		return nil
	}
}
