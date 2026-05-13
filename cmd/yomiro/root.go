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
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "yomiro",
		Short: "Yomiro CLI — gateway daemon and platform control",
		Long: `Yomiro CLI controls the customer-side gateway daemon and the platform tenant.

Run 'yomiro auth login' to authenticate, then use 'yomiro gw init/up' to
deploy a gateway, or use the platform subcommands (skill, dashboard,
capture, incident, device) to manage your tenant.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.PersistentFlags().StringVar(&flagAPIURL, "api-url", "", "Override platform API URL (default: from credentials)")
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
