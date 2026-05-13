package gw

import (
	"fmt"

	"github.com/yomiroco/yomiro-cli/internal/gw/svcwrap"
	"github.com/spf13/cobra"
)

func newDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down",
		Short: "Stop the gateway service and unregister it",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := svcwrap.New()
			if err != nil {
				return err
			}
			_ = m.Stop()
			if err := m.Uninstall(); err != nil {
				return fmt.Errorf("uninstall: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "✓ Gateway service stopped and unregistered")
			return nil
		},
	}
}
