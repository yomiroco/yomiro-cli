package gw

import (
	"fmt"
	"strings"

	"github.com/yomiroco/yomiro-cli/internal/gw/svcwrap"
	"github.com/kardianos/service"
	"github.com/spf13/cobra"
)

func newUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Install + start the gateway as a system service",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := svcwrap.New()
			if err != nil {
				return err
			}
			st, err := m.Status()
			if err == nil && st == service.StatusRunning {
				fmt.Fprintln(cmd.OutOrStdout(), "Gateway is already running.")
				return nil
			}
			if err := m.Install(); err != nil && !isAlreadyInstalled(err) {
				return fmt.Errorf("install: %w", err)
			}
			if err := m.Start(); err != nil {
				return fmt.Errorf("start: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "✓ Gateway service started")
			return nil
		},
	}
}

func isAlreadyInstalled(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already exists") || strings.Contains(msg, "already installed")
}
