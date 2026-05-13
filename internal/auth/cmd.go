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
// TODO: defaultAuth0ClientID is a placeholder until prod Auth0 is provisioned
// (the prod auth0-app terragrunt unit lives in infrastructure/_disabled/).
// The dev tenant's CLI client_id is `oGQgnpBtymVPMJMkHbvP433o6n8sqO4e`
// (provisioned via infrastructure/modules/auth0-app on 2026-05-04). Operators
// running against dev must currently override with --auth0-client-id /
// YOMIRO_AUTH0_CLIENT_ID; bake the real prod value here once prod auth0 lands.
//
// Note: the Auth0 tenant must also have "Device Code" enabled in Tenant
// Settings → Advanced (the auth0/auth0 provider has no resource for that
// toggle). Without it, the device-code flow returns `unauthorized_client`.
const (
	defaultAuth0Domain   = "yomiro.eu.auth0.com"
	defaultAuth0ClientID = "yomiro-cli"
	defaultAudience      = "https://api.yomiro.io"
	defaultAPIURL        = "https://api.yomiro.io"
)

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
