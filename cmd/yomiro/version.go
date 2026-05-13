package main

import (
	"fmt"

	"github.com/yomiroco/yomiro-cli/internal/buildinfo"
	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version, commit, and build date",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "yomiro %s (%s) built %s\n", buildinfo.Version, buildinfo.Commit, buildinfo.Date)
			return nil
		},
	}
}
