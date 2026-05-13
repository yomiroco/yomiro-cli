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
	"github.com/yomiroco/yomiro-cli/internal/platform/client"
	"github.com/yomiroco/yomiro-cli/internal/platform/generated"
	"github.com/yomiroco/yomiro-cli/internal/platform/overrides"
)

// Default API URL used when no flag, env var, or saved credential is set.
// Operators target other environments via YOMIRO_API_URL (e.g. dev backend
// at https://api.dev.yomiro.io, local at http://localhost:8000) without
// having to rebuild or re-auth.
const defaultAPIURL = "https://api.yomiro.io"

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
	apiClient, err := buildClient(defaultAPIURL, "")
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

	dash := generated.NewDashboardsCmd(getClient)
	dash.Use = "dashboard"
	root.AddCommand(dash)

	capCmd := generated.NewCapturesCmd(getClient)
	capCmd.Use = "capture"
	root.AddCommand(capCmd)

	inc := generated.NewIncidentsCmd(getClient)
	inc.Use = "incident"
	root.AddCommand(inc)

	dev := generated.NewDevicesCmd(getClient)
	dev.Use = "device"
	root.AddCommand(dev)

	for _, group := range []string{"skill", "dashboard", "capture", "incident", "device"} {
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

// resolveCredentials chooses the API URL and bearer token for the next
// request. Precedence, highest to lowest:
//
//  1. --api-url / --token flags (explicit operator intent)
//  2. YOMIRO_API_URL / YOMIRO_API_TOKEN env vars (so the same binary can
//     be pointed at dev/local/prod from a shell or CI without re-auth)
//  3. Credentials store (yomiro auth login)
//  4. defaultAPIURL constant
//
// Empty token is acceptable — help and the gateway subcommands don't need
// it, and missing-auth errors surface from the server.
func resolveCredentials(cmd *cobra.Command) (apiURL, token string, err error) {
	apiURL = defaultAPIURL
	if store, err := credentials.New(); err == nil {
		if c, err := store.Load(); err == nil {
			if c.APIURL != "" {
				apiURL = c.APIURL
			}
			token = c.Token
		}
	}
	if v := os.Getenv("YOMIRO_API_URL"); v != "" {
		apiURL = v
	}
	if v := os.Getenv("YOMIRO_API_TOKEN"); v != "" {
		token = v
	}
	if cmd != nil {
		if f := cmd.Flag("api-url"); f != nil && f.Changed {
			apiURL = f.Value.String()
		}
		if f := cmd.Flag("token"); f != nil && f.Changed {
			token = f.Value.String()
		}
	}
	return apiURL, token, nil
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
