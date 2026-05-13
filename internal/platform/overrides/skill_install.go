package overrides

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "install <slug-or-url>",
		Short: "Install a skill from the platform registry or a URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "Installing skill %q (override stub — implement against POST /skills/install)\n", args[0])
			return nil
		},
	}
	Register("skill", cmd)
}
