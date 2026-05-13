package gw

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/yomiroco/yomiro-cli/internal/config"
	"github.com/yomiroco/yomiro-cli/internal/gw/daemon"
	"github.com/yomiroco/yomiro-cli/internal/gw/dbproxy"
	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Run the gateway daemon in the foreground (no service registration)",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.GwConfigFile()
			if err != nil {
				return err
			}
			cfg, err := config.LoadGwConfig(path)
			if err != nil {
				return fmt.Errorf("load config (run `yomiro gw init` first): %w", err)
			}

			ctx, cancel := context.WithCancel(cmd.Context())
			defer cancel()

			pool, err := dbproxy.NewPgProxy(ctx, cfg.Database.URL, cfg.Database.MaxConnections)
			if err != nil {
				return fmt.Errorf("connect to customer DB: %w", err)
			}
			defer pool.Close()

			d := daemon.New(cfg, pool)

			sigs := make(chan os.Signal, 1)
			signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigs
				fmt.Fprintln(cmd.OutOrStdout(), "shutting down...")
				cancel()
			}()

			fmt.Fprintf(cmd.OutOrStdout(), "✓ Connecting to %s as %s\n", cfg.Platform.Endpoint, cfg.Gateway.ID)
			return d.Run(ctx)
		},
	}
}
