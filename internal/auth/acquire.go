package auth

import (
	"fmt"
	"io"
	"os"

	"github.com/yomiroco/yomiro-cli/internal/credentials"
	"github.com/yomiroco/yomiro-cli/internal/envprofile"
	"github.com/yomiroco/yomiro-cli/internal/platform"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

// openBrowser opens a URL in the operator's default browser. It is a package
// var so tests exercising the --web flow can stub it out — otherwise `go test`
// would launch a real browser tab to the cli-pair page.
var openBrowser = browser.OpenURL

// AuthConfig is the resolved environment for an interactive auth flow.
type AuthConfig struct {
	APIURL  string
	DC      *DeviceCodeClient
	Profile envprofile.Profile
}

// AddAuthFlags registers the device-code / browser flags shared by
// `auth login`, `auth token create`, and `gw init --from-login`. The
// `--api-url` flag is owned by the root command (persistent), so it is not
// registered here. The auth0/audience flag defaults are empty: an unset flag
// must not shadow the active --env profile, so the effective value comes from
// the profile via flagEnvOr.
func AddAuthFlags(cmd *cobra.Command) {
	cmd.Flags().String("auth0-domain", "", "Auth0 tenant domain (default: from --env)")
	cmd.Flags().String("auth0-client-id", "", "Auth0 application client ID (default: from --env)")
	cmd.Flags().String("audience", "", "Auth0 audience claim (default: from --env)")
	cmd.Flags().Bool("web", false, "Open a browser to authorize interactively instead of device-code")
}

// ResolveAuthConfig resolves the API URL and an Auth0 device-code client.
// Precedence for every field: changed flag > env var > active --env profile >
// (for API URL only) stored credential > compiled-in default. Explicit
// operator intent (a changed flag) always wins; env is the CI/shell override;
// the --env profile fills the rest; stored creds let a logged-in operator omit
// --api-url. Returns an error only for an unknown --env name.
func ResolveAuthConfig(cmd *cobra.Command) (AuthConfig, error) {
	prof, explicit, err := envprofile.Active(cmd)
	if err != nil {
		return AuthConfig{}, err
	}
	// Only hand the profile's API URL to the resolver when the env was
	// explicitly selected; an implicit prod default must not override a stored
	// login. The Auth0/audience fields below use the profile regardless.
	profileAPIURL := ""
	if explicit {
		profileAPIURL = prof.APIURL
	}
	// Token is irrelevant for the acquire path (login/token create mint their
	// own token), so only the resolved API URL is used here.
	apiURL, _ := credentials.Resolve(changedFlag(cmd, "api-url"), "", profileAPIURL)

	return AuthConfig{
		APIURL: apiURL,
		DC: &DeviceCodeClient{
			Domain:   flagEnvOr(cmd, "auth0-domain", "YOMIRO_AUTH0_DOMAIN", prof.Auth0Domain),
			ClientID: flagEnvOr(cmd, "auth0-client-id", "YOMIRO_AUTH0_CLIENT_ID", prof.Auth0ClientID),
			Audience: flagEnvOr(cmd, "audience", "YOMIRO_AUTH0_AUDIENCE", prof.Audience),
		},
		Profile: prof,
	}, nil
}

// changedFlag returns the value of a flag only if it was explicitly set on
// cmd; otherwise "". This is the "explicit flag override" input expected by
// credentials.Resolve.
func changedFlag(cmd *cobra.Command, name string) string {
	if cmd == nil {
		return ""
	}
	if f := cmd.Flag(name); f != nil && f.Changed {
		return f.Value.String()
	}
	return ""
}

// FirstNonEmpty returns a if non-empty, else b. Used to fall back from an
// explicit YOMIRO_FRONTEND_URL to the active profile's frontend URL (exported
// so the gw package can share the same helper for gw init --web).
func FirstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// flagEnvOr returns the changed flag value, else the env var, else def.
func flagEnvOr(cmd *cobra.Command, flagName, envName, def string) string {
	if v := changedFlag(cmd, flagName); v != "" {
		return v
	}
	if v := os.Getenv(envName); v != "" {
		return v
	}
	return def
}

// ResolveFrontendURL is the exported entry point to the frontend-URL resolver
// for callers outside the auth package (e.g. gw init --from-login --web).
func ResolveFrontendURL(override, apiURL string) (string, error) {
	return resolveFrontendURL(override, apiURL)
}

// AcquireJWT returns an Auth0 access token. If explicitToken is non-empty it
// is returned verbatim (the non-interactive passthrough — a JWT supplied via
// --token / YOMIRO_API_TOKEN, or replayed from `login --debug-jwt`).
// Otherwise it runs the interactive device-code flow, printing the
// verification URL/code to out.
func AcquireJWT(out io.Writer, dc *DeviceCodeClient, explicitToken string) (string, error) {
	if explicitToken != "" {
		return explicitToken, nil
	}
	start, err := dc.Start()
	if err != nil {
		return "", fmt.Errorf("start device code flow: %w", err)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "  Open this URL in your browser to sign in:\n\n  %s\n\n", start.VerificationURI)
	fmt.Fprintf(out, "  Verification code: %s\n\n", start.UserCode)
	fmt.Fprintln(out, "  Waiting for you to approve in the browser…")
	jwt, err := dc.PollUntilDone(start.DeviceCode, start.Interval, start.ExpiresIn)
	if err != nil {
		return "", fmt.Errorf("poll: %w", err)
	}
	return jwt, nil
}

// AcquireViaWeb drives the browser cli-pair handshake: start a pairing, open
// the scope-picker, and poll until the operator authorizes. It returns the
// already-minted API key token (the picker chooses scopes/lifetime, so the
// passed scopes only pre-seed it). Unlike login it does not save — the caller
// decides whether to persist or print.
func AcquireViaWeb(out io.Writer, pc *platform.Client, frontend string, scopes []string) (string, error) {
	code, err := pc.CreateCLIPairRequest(hostnameLabel(), scopes)
	if err != nil {
		return "", fmt.Errorf("start browser pairing: %w", err)
	}
	pairURL := fmt.Sprintf("%s/cli-pair/%s", frontend, code)
	fmt.Fprintf(out, "\n  Open this URL to authorize the CLI:\n  %s\n", pairURL)
	if err := openBrowser(pairURL); err != nil {
		fmt.Fprintf(out, "  (couldn't open automatically: %v)\n", err)
	}
	fmt.Fprintln(out, "  Waiting for you to authorize…")
	return pc.PollCLIPair(code, webPairTimeout, webPairPollInterval)
}
