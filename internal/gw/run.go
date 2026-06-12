package gw

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
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

			// Tee daemon output to the log file `gw logs` tails (the service
			// manager captures stdout elsewhere, e.g. launchd's StandardOutPath).
			out := io.Writer(cmd.OutOrStdout())
			if stateDir, derr := config.StateDir(); derr == nil {
				logFile := filepath.Join(stateDir, "daemon.log")
				if f, ferr := os.OpenFile(logFile,
					os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); ferr == nil {
					defer f.Close()
					out = io.MultiWriter(cmd.OutOrStdout(), f)
				} else {
					fmt.Fprintf(os.Stderr, "warning: could not open %s (%v); `yomiro gw logs` will be empty\n", logFile, ferr)
				}
			}

			pool, err := dbproxy.NewPgProxy(ctx, cfg.Database.URL, cfg.Database.MaxConnections)
			if err != nil {
				return fmt.Errorf("connect to customer DB: %w", err)
			}
			defer pool.Close()

			d := daemon.New(cfg, pool)
			d.Log = out
			d.ConfigPath = path

			sigs := make(chan os.Signal, 1)
			signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigs
				fmt.Fprintln(out, "shutting down...")
				cancel()
			}()

			fmt.Fprintf(out, "✓ Connecting to %s as %s\n", cfg.Platform.Endpoint, cfg.Gateway.ID)
			return d.Run(ctx)
		},
	}
}
