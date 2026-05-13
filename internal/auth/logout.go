package auth

import (
	"errors"
	"fmt"

	"github.com/yomiroco/yomiro-cli/internal/credentials"
	"github.com/yomiroco/yomiro-cli/internal/platform"
	"github.com/spf13/cobra"
)

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Revoke the local token and clear stored credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := credentials.New()
			if err != nil {
				return err
			}
			c, err := store.Load()
			if errors.Is(err, credentials.ErrNotFound) {
				fmt.Fprintln(cmd.OutOrStdout(), "Already logged out.")
				return nil
			}
			if err != nil {
				return err
			}

			// Best-effort: revoke server-side. If the network is down, we still clear locally.
			pc := platform.New(c.APIURL, c.Token)
			keys, err := pc.ListAPIKeys()
			if err == nil {
				for _, k := range keys.Data {
					if len(c.Token) >= 12 && k.Prefix == c.Token[:12] {
						_ = pc.RevokeAPIKey(k.ID)
						break
					}
				}
			}

			if err := store.Clear(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Logged out.")
			return nil
		},
	}
}
