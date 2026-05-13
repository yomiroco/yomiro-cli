package auth

import (
	"github.com/yomiroco/yomiro-cli/internal/credentials"
	"github.com/spf13/cobra"
)

// NewCmd returns the `yomiro auth` command tree.
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage authentication and API tokens",
	}
	cmd.AddCommand(newLoginCmd())
	cmd.AddCommand(newLogoutCmd())
	cmd.AddCommand(newWhoamiCmd())
	cmd.AddCommand(newTokenCmd())
	cmd.AddCommand(newSetTokenCmd())
	return cmd
}

// Defaults — these are baked-in for the public Auth0 tenant; can be
// overridden via env (`YOMIRO_AUTH0_DOMAIN`, `YOMIRO_AUTH0_CLIENT_ID`,
// `YOMIRO_AUTH0_AUDIENCE`, `YOMIRO_API_URL`) or flags.
//
// defaultAuth0ClientID is the prod CLI client (provisioned via
// infrastructure/live/prod/auth0-app on 2026-05-13). Operators running
// against dev override with --auth0-client-id / YOMIRO_AUTH0_CLIENT_ID
// = oGQgnpBtymVPMJMkHbvP433o6n8sqO4e (and --api-url / YOMIRO_API_URL =
// https://api.dev.yomiro.io, --audience / YOMIRO_AUTH0_AUDIENCE =
// https://api.dev.yomiro.io).
//
// Note: the Auth0 tenant must also have "Device Code" enabled in Tenant
// Settings → Advanced (the auth0/auth0 provider has no resource for that
// toggle). Without it, the device-code flow returns `unauthorized_client`.
const (
	defaultAuth0Domain   = "yomiro.eu.auth0.com"
	defaultAuth0ClientID = "sUIOMvj0aaIRFbOPDcYUZ8bM5yCbWnA5"
	defaultAudience      = "https://api.yomiro.io"
	defaultAPIURL        = "https://api.yomiro.io"
)

// defaultCLIScopes is the least-privileged set granted to a freshly-logged-in
// CLI: enough to inspect every resource the wired groups expose, nothing
// that can mutate state. Operators who need write access pass --scopes to
// `yomiro auth login` (or mint a per-purpose key in the platform UI).
//
// Mirrors the read half of backend/app/database_models/platform/api_key.py
// ApiKeyScope; keep in sync when a new resource tag lands.
var defaultCLIScopes = []string{
	"agents:read",
	"dashboards:read",
	"data:read",
	"inspection:read",
	"devices:read",
}

func newSetTokenCmd() *cobra.Command {
	var apiURL, token, user, tenant string
	cmd := &cobra.Command{
		Use:    "set-token",
		Short:  "Hidden: set credentials directly (used for ops/local dev)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := credentials.New()
			if err != nil {
				return err
			}
			return store.Save(credentials.Credentials{APIURL: apiURL, Token: token, User: user, Tenant: tenant})
		},
	}
	cmd.Flags().StringVar(&apiURL, "api-url", defaultAPIURL, "Platform API URL")
	cmd.Flags().StringVar(&token, "token", "", "API token (yom_pat_*)")
	cmd.Flags().StringVar(&user, "user", "", "User email (display only)")
	cmd.Flags().StringVar(&tenant, "tenant", "", "Tenant name (display only)")
	_ = cmd.MarkFlagRequired("token")
	return cmd
}
