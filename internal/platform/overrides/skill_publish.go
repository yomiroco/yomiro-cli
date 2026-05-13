package overrides

import (
	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "publish <skill-id>",
		Short: "Publish a skill to the public registry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Printf("Publishing skill %q (override stub)\n", args[0])
			return nil
		},
	}
	Register("skill", cmd)
}
