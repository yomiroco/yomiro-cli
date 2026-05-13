package overrides

import (
	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "render <dashboard-id>",
		Short: "Render a dashboard to a file (PDF or PNG)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			out, _ := cmd.Flags().GetString("out")
			cmd.Printf("Rendering dashboard %q as %s to %s (override stub)\n", args[0], format, out)
			return nil
		},
	}
	cmd.Flags().String("format", "pdf", "pdf|png")
	cmd.Flags().String("out", "dashboard.pdf", "Output file path")
	Register("dashboard", cmd)
}
