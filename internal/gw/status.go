package gw

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/yomiroco/yomiro-cli/internal/config"
	"github.com/yomiroco/yomiro-cli/internal/gw/control"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the running daemon's state",
		RunE: func(cmd *cobra.Command, args []string) error {
			return controlAction(cmd, "status")
		},
	}
}

func controlAction(cmd *cobra.Command, action string) error {
	cacheDir, err := config.CacheDir()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Second)
	defer cancel()
	resp, err := control.Send(ctx, filepath.Join(cacheDir, "gw.sock"), control.Request{Action: action})
	if err != nil {
		return err
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(resp)
}
