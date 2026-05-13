package gw

import "github.com/spf13/cobra"

func newResumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume",
		Short: "Resume tool dispatch",
		RunE:  func(c *cobra.Command, _ []string) error { return controlAction(c, "resume") },
	}
}
