package main

import (
	"os"

	"github.com/yomiroco/yomiro-cli/internal/auth"
	"github.com/yomiroco/yomiro-cli/internal/gw"
	"github.com/yomiroco/yomiro-cli/internal/platform"
	"github.com/spf13/cobra"
)

var (
	flagAPIURL string
	flagToken  string
	flagOutput string
	flagEnv    string
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "yomiro",
		Short: "Yomiro CLI — gateway daemon and platform control",
		Long: `Yomiro CLI controls the customer-side gateway daemon and the platform tenant.

Run 'yomiro auth login' to authenticate, then use 'yomiro gw init/up' to
deploy a gateway, or use the platform subcommands (skill, dashboard,
capture, incident, device) to manage your tenant.

Use --env to target an environment in one flag (default: prod). It expands to
the matching api-url, Auth0 domain/client-id/audience, gateway WS endpoint, and
frontend URL. Explicit flags and env vars still override individual fields.

  prod  (default)  api.yomiro.io          / prod Auth0 client
  staging          api.staging.yomiro.io  / staging Auth0 client
  dev              api.dev.yomiro.io      / dev Auth0 client
  local            localhost:8000         / dev Auth0 client + frontend localhost:5173`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().StringVar(&flagEnv, "env", "", "Target environment: local|dev|staging|prod (default prod; expands to api-url/auth0/audience/ws-endpoint; explicit flags/env override). Also YOMIRO_ENV.")
	cmd.PersistentFlags().StringVar(&flagAPIURL, "api-url", "", "Override platform API URL (default: from --env / credentials)")
	cmd.PersistentFlags().StringVar(&flagToken, "token", "", "Override API token for this command")
	cmd.PersistentFlags().StringVar(&flagOutput, "output", "json", "Output format: json|yaml|table")

	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(auth.NewCmd())
	cmd.AddCommand(gw.NewCmd())
	if err := platform.AddTo(cmd); err != nil {
		cmd.SetErr(os.Stderr)
	}
	return cmd
}
