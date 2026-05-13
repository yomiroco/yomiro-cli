package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/yomiroco/yomiro-cli/internal/credentials"
	"github.com/yomiroco/yomiro-cli/internal/platform"
)

func newLoginCmd() *cobra.Command {
	var apiURL, domain, clientID, audience string
	var scopes []string
	var debugJWT bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate via Auth0 device-code flow and mint an API key",
		Long: `Authenticate via Auth0 device-code flow and mint a long-lived API key.

The minted key carries scoped permissions; --scopes (comma-separated) lets
you override the read-only default. Existing keys can be rotated, revoked,
or replaced with a higher-privilege set in the platform UI.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			effectiveAPIURL := envOr("YOMIRO_API_URL", apiURL)

			dc := &DeviceCodeClient{
				Domain:   envOr("YOMIRO_AUTH0_DOMAIN", domain),
				ClientID: envOr("YOMIRO_AUTH0_CLIENT_ID", clientID),
				Audience: envOr("YOMIRO_AUTH0_AUDIENCE", audience),
			}
			start, err := dc.Start()
			if err != nil {
				return fmt.Errorf("start device code flow: %w", err)
			}

			fmt.Fprintln(out)
			fmt.Fprintf(out, "  Open this URL in your browser to sign in:\n\n  %s\n\n", start.VerificationURI)
			fmt.Fprintf(out, "  Verification code: %s\n\n", start.UserCode)
			fmt.Fprintln(out, "  Waiting for you to approve in the browser…")

			jwt, err := dc.PollUntilDone(start.DeviceCode, start.Interval, start.ExpiresIn)
			if err != nil {
				return fmt.Errorf("poll: %w", err)
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

			// Announce the scope set the operator is about to grant, so they
			// can Ctrl+C and re-run with --scopes before the key is minted
			// (rather than discovering 403s mid-session).
			fmt.Fprintf(out, "\n  Minting API key with scopes: %s\n", strings.Join(scopes, ", "))
			if frontend := frontendFromAPI(effectiveAPIURL); frontend != "" {
				fmt.Fprintf(out, "  (override with --scopes, or manage existing keys at %s/settings/api-keys)\n", frontend)
			} else {
				fmt.Fprintln(out, "  (override with --scopes)")
			}

			pc := platform.New(effectiveAPIURL, jwt)
			expires := time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339)
			created, err := pc.CreateAPIKey(platform.CreateKeyRequest{
				Name:      hostnameLabel(),
				Scopes:    scopes,
				ExpiresAt: &expires,
			})
			if err != nil {
				return fmt.Errorf("mint api key: %w", err)
			}

			pc2 := platform.New(effectiveAPIURL, created.Token)
			user, err := pc2.Whoami()
			if err != nil {
				return fmt.Errorf("whoami: %w", err)
			}

			store, err := credentials.New()
			if err != nil {
				return err
			}
			err = store.Save(credentials.Credentials{
				APIURL: effectiveAPIURL,
				Token:  created.Token,
				User:   user.Email,
				Tenant: user.Tenant.Name,
			})
			if err != nil {
				return fmt.Errorf("save credentials: %w", err)
			}

			fmt.Fprintf(out, "\n  ✓ Signed in as %s (tenant: %s)\n", user.Email, user.Tenant.Name)
			fmt.Fprintf(out, "  Token expires: %s\n", expires)
			return nil
		},
	}
	cmd.Flags().StringVar(&apiURL, "api-url", defaultAPIURL, "Platform API URL")
	cmd.Flags().StringVar(&domain, "auth0-domain", defaultAuth0Domain, "Auth0 tenant domain")
	cmd.Flags().StringVar(&clientID, "auth0-client-id", defaultAuth0ClientID, "Auth0 application client ID")
	cmd.Flags().StringVar(&audience, "audience", defaultAudience, "Auth0 audience claim")
	cmd.Flags().StringSliceVar(&scopes, "scopes", defaultCLIScopes, "API key scopes (comma-separated). Defaults are read-only across the wired groups.")
	cmd.Flags().BoolVar(&debugJWT, "debug-jwt", false, "Print decoded Auth0 access-token claims and exit (no key mint, no credential save)")
	return cmd
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

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func hostnameLabel() string {
	h, _ := os.Hostname()
	if h == "" {
		return "yomiro-cli"
	}
	return "cli-" + h
}
