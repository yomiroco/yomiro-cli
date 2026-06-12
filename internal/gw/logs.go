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

// tailBytes is the maximum number of bytes of existing log content printed
// before the follow loop begins. 64 KiB is enough for several hundred lines
// without flooding the terminal on a large log file.
const tailBytes int64 = 64 * 1024

// printTail writes the last up-to maxBytes of f to w, dropping a partial
// first line when the window starts mid-file, and leaves f positioned at
// EOF so a follow loop can continue from there.
func printTail(f *os.File, w io.Writer, maxBytes int64) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	size := info.Size()

	start := int64(0)
	if size > maxBytes {
		start = size - maxBytes
	}

	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return err
	}

	reader := bufio.NewReader(f)

	// When we start mid-file, discard up to and including the first newline so
	// we never emit a truncated partial line.
	if start > 0 {
		if _, err := reader.ReadString('\n'); err != nil && !errors.Is(err, io.EOF) {
			return err
		}
	}

	_, err = io.Copy(w, reader)
	return err
}

func newLogsCmd() *cobra.Command {
	var audit bool
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show recent log history then follow new output (--audit for the query audit log)",
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
			if err := printTail(f, cmd.OutOrStdout(), tailBytes); err != nil {
				return err
			}
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
