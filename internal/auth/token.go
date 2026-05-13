package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yomiroco/yomiro-cli/internal/credentials"
	"github.com/yomiroco/yomiro-cli/internal/platform"
	"github.com/spf13/cobra"
)

func newTokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Manage API keys for the current user",
	}
	cmd.AddCommand(newTokenCreateCmd())
	cmd.AddCommand(newTokenListCmd())
	cmd.AddCommand(newTokenRevokeCmd())
	return cmd
}

func loadClient() (*platform.Client, error) {
	store, err := credentials.New()
	if err != nil {
		return nil, err
	}
	c, err := store.Load()
	if errors.Is(err, credentials.ErrNotFound) {
		return nil, fmt.Errorf("not logged in — run `yomiro auth login`")
	}
	if err != nil {
		return nil, err
	}
	return platform.New(c.APIURL, c.Token), nil
}

func newTokenCreateCmd() *cobra.Command {
	var name, scopes, ttl string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Mint a new API key. Cleartext is shown ONCE.",
		RunE: func(cmd *cobra.Command, args []string) error {
			pc, err := loadClient()
			if err != nil {
				return err
			}
			scopeList := []string{}
			for _, s := range strings.Split(scopes, ",") {
				if s = strings.TrimSpace(s); s != "" {
					scopeList = append(scopeList, s)
				}
			}
			if len(scopeList) == 0 {
				return fmt.Errorf("--scopes is required (comma-separated)")
			}

			req := platform.CreateKeyRequest{Name: name, Scopes: scopeList}
			if ttl != "" && ttl != "0" {
				d, err := time.ParseDuration(ttl)
				if err != nil {
					return fmt.Errorf("--ttl: %w", err)
				}
				exp := time.Now().Add(d).UTC().Format(time.RFC3339)
				req.ExpiresAt = &exp
			}

			created, err := pc.CreateAPIKey(req)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "\n  ✓ API key %s created\n\n", created.Key.Name)
			fmt.Fprintf(out, "  Token (copy now — not shown again):\n\n  %s\n\n", created.Token)
			fmt.Fprintf(out, "  Prefix: %s\n", created.Key.Prefix)
			fmt.Fprintf(out, "  Scopes: %s\n", strings.Join(created.Key.Scopes, ", "))
			if created.Key.ExpiresAt != nil {
				fmt.Fprintf(out, "  Expires: %s\n", *created.Key.ExpiresAt)
			} else {
				fmt.Fprintln(out, "  Expires: never")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Human-readable name for the key (required)")
	cmd.Flags().StringVar(&scopes, "scopes", "", "Comma-separated scopes, e.g. 'gateway:tunnel' or 'agents:read,dashboards:read'")
	cmd.Flags().StringVar(&ttl, "ttl", "0", "Lifetime as Go duration (e.g. 720h for 30d), or 0 for never")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newTokenListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List API keys for the current user",
		RunE: func(cmd *cobra.Command, args []string) error {
			pc, err := loadClient()
			if err != nil {
				return err
			}
			lst, err := pc.ListAPIKeys()
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(lst)
		},
	}
}

func newTokenRevokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revoke <key-id>",
		Short: "Revoke an API key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pc, err := loadClient()
			if err != nil {
				return err
			}
			if err := pc.RevokeAPIKey(args[0]); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Revoked.")
			return nil
		},
	}
}
