package auth

import (
	"fmt"
	"os"
	"time"

	"github.com/yomiroco/yomiro-cli/internal/credentials"
	"github.com/yomiroco/yomiro-cli/internal/platform"
	"github.com/spf13/cobra"
)

func newLoginCmd() *cobra.Command {
	var apiURL, domain, clientID, audience string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate via Auth0 device-code flow and mint an API key",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

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

			// Mint a long-lived API key against the platform.
			pc := platform.New(envOr("YOMIRO_API_URL", apiURL), jwt)
			expires := time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339)
			created, err := pc.CreateAPIKey(platform.CreateKeyRequest{
				Name:      hostnameLabel(),
				Scopes:    []string{"agents:read", "dashboards:read", "data:read", "inspection:read", "devices:read"},
				ExpiresAt: &expires,
			})
			if err != nil {
				return fmt.Errorf("mint api key: %w", err)
			}

			// Resolve identity for display.
			pc2 := platform.New(envOr("YOMIRO_API_URL", apiURL), created.Token)
			user, err := pc2.Whoami()
			if err != nil {
				return fmt.Errorf("whoami: %w", err)
			}

			store, err := credentials.New()
			if err != nil {
				return err
			}
			err = store.Save(credentials.Credentials{
				APIURL: envOr("YOMIRO_API_URL", apiURL),
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
	return cmd
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
