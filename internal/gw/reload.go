package gw

import "github.com/spf13/cobra"

func newReloadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reload",
		Short: "Re-read gw.yaml and reconnect",
		RunE:  func(c *cobra.Command, _ []string) error { return controlAction(c, "reload") },
	}
}
