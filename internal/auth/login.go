package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/yomiroco/yomiro-cli/internal/credentials"
	"github.com/yomiroco/yomiro-cli/internal/platform"
)

func newLoginCmd() *cobra.Command {
	var scopes []string
	var debugJWT bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate via Auth0 device-code flow and mint an API key",
		Long: `Authenticate via Auth0 device-code flow and mint a long-lived API key.

Two flows are available:

  Default (silent mint): mints a key immediately with the scopes from
  --scopes (or the read-only defaults). Non-interactive — best for CI and
  headless shells, where you pass --scopes explicitly (and point at a tenant
  via YOMIRO_API_URL / YOMIRO_AUTH0_* env vars).

  --web (browser scope picker): opens the platform web app so you choose
  scopes and the key's lifetime interactively before it's minted.
  Recommended for interactive shells. Needs a resolvable frontend URL: it's
  derived from a public --api-url (e.g. https://api.dev.yomiro.io), or set
  YOMIRO_FRONTEND_URL explicitly (e.g. http://localhost:5173) for local testing.

The minted key carries scoped permissions. Existing keys can be rotated,
revoked, or replaced with a higher-privilege set in the platform UI.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			web, _ := cmd.Flags().GetBool("web")

			cfg, err := ResolveAuthConfig(cmd)
			if err != nil {
				return err
			}

			jwt, err := AcquireJWT(out, cfg.DC, "")
			if err != nil {
				return err
			}

			if debugJWT {
				if claims, perr := decodeJWTClaims(jwt); perr == nil {
					fmt.Fprintf(out, "\n  --debug-jwt: Auth0 access token claims:\n%s\n", claims)
				} else {
					fmt.Fprintf(out, "\n  --debug-jwt: couldn't decode token: %v\n", perr)
				}
				// Write the raw JWT to a temp file so the operator can replay
				// validation locally without copy-pasting a sensitive token.
				path := fmt.Sprintf("/tmp/yomiro-debug-jwt-%d.txt", time.Now().Unix())
				if werr := os.WriteFile(path, []byte(jwt), 0o600); werr == nil {
					fmt.Fprintf(out, "\n  --debug-jwt: raw token written to %s (mode 0600, delete when done)\n", path)
				}
				return nil
			}

			effectiveAPIURL := cfg.APIURL
			pc := platform.New(effectiveAPIURL, jwt)

			if web {
				frontend, ferr := resolveFrontendURL(FirstNonEmpty(os.Getenv("YOMIRO_FRONTEND_URL"), cfg.Profile.FrontendURL), effectiveAPIURL)
				if ferr != nil {
					return ferr
				}
				return runWebHandshake(out, pc, effectiveAPIURL, frontend, scopes)
			}

			// Announce the scope set the operator is about to grant, so they
			// can Ctrl+C and re-run with --scopes before the key is minted
			// (rather than discovering 403s mid-session).
			fmt.Fprintf(out, "\n  Minting API key with scopes: %s\n", strings.Join(scopes, ", "))
			if frontend := frontendFromAPI(effectiveAPIURL); frontend != "" {
				fmt.Fprintf(out, "  (override with --scopes, or manage existing keys at %s/settings/api-keys)\n", frontend)
			} else {
				fmt.Fprintln(out, "  (override with --scopes)")
			}

			expires := time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339)
			created, err := pc.CreateAPIKey(platform.CreateKeyRequest{
				Name:      hostnameLabel(),
				Scopes:    scopes,
				ExpiresAt: &expires,
			})
			if err != nil {
				return fmt.Errorf("mint api key: %w", err)
			}

			user, err := saveAndWhoami(out, effectiveAPIURL, created.Token)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "\n  ✓ Signed in as %s (tenant: %s)\n", user.Email, user.Tenant.Name)
			fmt.Fprintf(out, "  Token expires: %s\n", expires)
			return nil
		},
	}
	AddAuthFlags(cmd)
	cmd.Flags().StringSliceVar(&scopes, "scopes", defaultCLIScopes, "API key scopes (comma-separated). Defaults are read-only across the wired groups.")
	cmd.Flags().BoolVar(&debugJWT, "debug-jwt", false, "Print decoded Auth0 access-token claims and exit (no key mint, no credential save)")
	return cmd
}

// webPairTimeout and webPairPollInterval bound the browser handshake. The
// timeout matches the backend's pairing TTL (PENDING_KEY_TTL_MINUTES).
const (
	webPairTimeout      = 5 * time.Minute
	webPairPollInterval = 2 * time.Second
)

// runWebHandshake drives the browser-based login: start a pairing, open the
// scope picker, wait for the operator to authorize, then save + report. The
// minted token's lifetime is chosen in the picker, so unlike the silent path
// the CLI doesn't know (or print) an expiry.
func runWebHandshake(out io.Writer, pc *platform.Client, apiURL, frontend string, scopes []string) error {
	token, err := AcquireViaWeb(out, pc, frontend, scopes)
	if err != nil {
		return err
	}
	user, err := saveAndWhoami(out, apiURL, token)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "\n  ✓ Signed in as %s (tenant: %s)\n", user.Email, user.Tenant.Name)
	return nil
}

// saveAndWhoami resolves the just-minted token to a user/tenant and persists
// the credentials. Shared by the silent and browser login paths.
func saveAndWhoami(out io.Writer, apiURL, token string) (*platform.CurrentUser, error) {
	pc := platform.New(apiURL, token)
	user, err := pc.Whoami()
	if err != nil {
		return nil, fmt.Errorf("whoami: %w", err)
	}

	store, err := credentials.New()
	if err != nil {
		return nil, err
	}
	err = store.Save(credentials.Credentials{
		APIURL: apiURL,
		Token:  token,
		User:   user.Email,
		Tenant: user.Tenant.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("save credentials: %w", err)
	}
	return user, nil
}

// decodeJWTClaims pretty-prints the payload of a JWT without verifying the
// signature. Used only by --debug-jwt to surface what Auth0 actually
// issued (audience, custom claims) when the backend rejects a token.
func decodeJWTClaims(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("not a JWT (expected 3 parts, got %d)", len(parts))
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return "", err
	}
	pretty, err := json.MarshalIndent(generic, "  ", "  ")
	if err != nil {
		return "", err
	}
	return "  " + string(pretty), nil
}

// resolveFrontendURL picks the frontend base URL the --web handshake deep-links
// into. An explicit YOMIRO_FRONTEND_URL override always wins: it's what lets
// --web target a local SPA (e.g. http://localhost:5173) that the api-host
// heuristic deliberately refuses to derive. Without an override it falls back to
// deriving the frontend from the API URL, erroring if that's not possible.
func resolveFrontendURL(override, apiURL string) (string, error) {
	if override != "" {
		return strings.TrimRight(override, "/"), nil
	}
	if frontend := frontendFromAPI(apiURL); frontend != "" {
		return frontend, nil
	}
	return "", fmt.Errorf("--web needs a resolvable frontend URL but couldn't derive one from %q; set YOMIRO_FRONTEND_URL (e.g. http://localhost:5173 for local testing), use a public --api-url (e.g. https://api.dev.yomiro.io), or omit --web to mint with --scopes", apiURL)
}

// frontendFromAPI guesses the platform frontend URL from the API URL so the
// login flow can deep-link to the API-keys management page. Returns "" for
// hosts the heuristic can't translate (e.g. localhost) so the caller can
// gracefully skip the hint.
//
// Mapping:
//
//	api.yomiro.io       -> app.yomiro.io     (prod naming has a separate app subdomain)
//	api.dev.yomiro.io   -> dev.yomiro.io     (dev/staging strip the api. prefix)
//	api.staging.foo.io  -> staging.foo.io
//	anything else       -> ""
func frontendFromAPI(apiURL string) string {
	u, err := url.Parse(apiURL)
	if err != nil || u.Host == "" {
		return ""
	}
	host := u.Host
	if !strings.HasPrefix(host, "api.") {
		return ""
	}
	rest := strings.TrimPrefix(host, "api.")
	if strings.HasPrefix(rest, "dev.") || strings.HasPrefix(rest, "staging.") {
		u.Host = rest
	} else {
		u.Host = "app." + rest
	}
	u.Path = ""
	u.RawQuery = ""
	return u.String()
}

func hostnameLabel() string {
	h, _ := os.Hostname()
	if h == "" {
		return "yomiro-cli"
	}
	return "cli-" + h
}
