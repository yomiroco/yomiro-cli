package gw

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/yomiroco/yomiro-cli/internal/config"
	"github.com/spf13/cobra"
)

func newLogsCmd() *cobra.Command {
	var audit bool
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Tail the daemon log (or audit log with --audit)",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := logPath(audit)
			if err != nil {
				return err
			}
			f, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("open log %s: %w", path, err)
			}
			defer f.Close()
			_, _ = f.Seek(0, io.SeekEnd)
			r := bufio.NewReader(f)
			for {
				line, err := r.ReadString('\n')
				if errors.Is(err, io.EOF) {
					time.Sleep(500 * time.Millisecond)
					continue
				}
				if err != nil {
					return err
				}
				fmt.Fprint(cmd.OutOrStdout(), line)
			}
		},
	}
	cmd.Flags().BoolVar(&audit, "audit", false, "Tail the per-query audit log instead of the daemon log")
	return cmd
}

func logPath(audit bool) (string, error) {
	stateDir, err := config.StateDir()
	if err != nil {
		return "", err
	}
	if audit {
		return stateDir + "/audit.log", nil
	}
	return stateDir + "/daemon.log", nil
}
