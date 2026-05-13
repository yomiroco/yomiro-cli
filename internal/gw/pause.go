package gw

import "github.com/spf13/cobra"

func newPauseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pause",
		Short: "Pause tool dispatch",
		RunE:  func(c *cobra.Command, _ []string) error { return controlAction(c, "pause") },
	}
}
