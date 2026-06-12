package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yomiroco/yomiro-cli/internal/credentials"
	"github.com/yomiroco/yomiro-cli/internal/envprofile"
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

// loadClient builds a platform client for the token list/revoke commands using
// the unified credential resolution (flag > env > stored > default). It
// preserves the "not logged in" UX: if no token is resolvable from any source
// AND nothing is stored, it returns the friendly login hint instead of letting
// an unauthenticated request fail server-side.
func loadClient(cmd *cobra.Command) (*platform.Client, error) {
	prof, explicit, err := envprofile.Active(cmd)
	if err != nil {
		return nil, err
	}
	profileAPIURL := ""
	if explicit {
		profileAPIURL = prof.APIURL
	}
	apiURL, token := credentials.Resolve(changedFlag(cmd, "api-url"), changedFlag(cmd, "token"), profileAPIURL)
	if token == "" {
		if _, stored := storedCredential(); !stored {
			return nil, fmt.Errorf("not logged in — run `yomiro auth login`")
		}
	}
	return platform.New(apiURL, token), nil
}

// storedCredential reports whether a credential is persisted (best-effort).
func storedCredential() (credentials.Credentials, bool) {
	store, err := credentials.New()
	if err != nil {
		return credentials.Credentials{}, false
	}
	c, err := store.Load()
	if errors.Is(err, credentials.ErrNotFound) || err != nil {
		return credentials.Credentials{}, false
	}
	return c, true
}

func newTokenCreateCmd() *cobra.Command {
	var name, scopes, ttl string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Mint a new API key. Cleartext is shown ONCE.",
		Long: `Mint a new API key for the current user.

Minting requires interactive auth (the backend deliberately forbids a
long-lived key from minting more keys), so this runs a fresh Auth0
device-code login by default. Pass --web to pick scopes in the browser, or
set YOMIRO_API_TOKEN / --token to an Auth0 JWT to mint non-interactively
(e.g. in CI, or replaying a token from 'yomiro auth login --debug-jwt').
With --web the browser picker chooses scopes and lifetime, so --name/--scopes/--ttl only pre-seed it.

The minted key is printed once and is NOT saved as your login credential.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			scopeList := splitScopes(scopes)
			if len(scopeList) == 0 {
				return fmt.Errorf("--scopes is required (comma-separated)")
			}

			cfg, err := ResolveAuthConfig(cmd)
			if err != nil {
				return err
			}
			web, _ := cmd.Flags().GetBool("web")

			var token string
			if web {
				frontend, ferr := resolveFrontendURL(FirstNonEmpty(os.Getenv("YOMIRO_FRONTEND_URL"), cfg.Profile.FrontendURL), cfg.APIURL)
				if ferr != nil {
					return ferr
				}
				pc := platform.New(cfg.APIURL, "")
				t, err := AcquireViaWeb(out, pc, frontend, scopeList)
				if err != nil {
					return err
				}
				token = t
			} else {
				jwt, err := AcquireJWT(out, cfg.DC, explicitToken(cmd))
				if err != nil {
					return err
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
				pc := platform.New(cfg.APIURL, jwt)
				created, err := pc.CreateAPIKey(req)
				if err != nil {
					return fmt.Errorf("mint api key: %w", err)
				}
				token = created.Token
			}

			fmt.Fprintf(out, "\n  ✓ API key minted\n\n")
			fmt.Fprintf(out, "  Token (copy now — not shown again):\n\n  %s\n\n", token)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Human-readable name for the key (required)")
	cmd.Flags().StringVar(&scopes, "scopes", "", "Comma-separated scopes, e.g. 'gateway:tunnel' or 'agents:read,dashboards:read'")
	cmd.Flags().StringVar(&ttl, "ttl", "0", "Lifetime as Go duration (e.g. 720h for 30d), or 0 for never")
	AddAuthFlags(cmd)
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

// splitScopes parses a comma-separated scope string, trimming blanks.
func splitScopes(scopes string) []string {
	out := []string{}
	for _, s := range strings.Split(scopes, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// explicitToken returns a JWT supplied out-of-band to skip device-code: the
// --token flag if changed, else YOMIRO_API_TOKEN. Empty means "run device-code".
func explicitToken(cmd *cobra.Command) string {
	if f := cmd.Flag("token"); f != nil && f.Changed {
		return f.Value.String()
	}
	return os.Getenv("YOMIRO_API_TOKEN")
}

func newTokenListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List API keys for the current user",
		RunE: func(cmd *cobra.Command, args []string) error {
			pc, err := loadClient(cmd)
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
			pc, err := loadClient(cmd)
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
