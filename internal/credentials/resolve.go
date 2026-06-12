package credentials

import "os"

// DefaultAPIURL is the compiled-in fallback when no flag, env var, or stored
// credential supplies one. (Single source of truth; replaces the duplicate
// defaultAPIURL consts in platform/wire.go and auth/cmd.go.)
const DefaultAPIURL = "https://api.yomiro.io"

// Resolve picks the API URL and bearer token for an outbound request.
//
// API URL precedence, highest to lowest:
//  1. explicit flag override (flagAPIURL — empty means "unset")
//  2. YOMIRO_API_URL env var
//  3. profileAPIURL — the API URL from an active --env profile (empty means
//     "no --env"); sits ABOVE stored creds so an explicit `--env dev` beats a
//     stored prod login (e.g. during `--env dev auth login`)
//  4. stored credentials (yomiro auth login)
//  5. DefaultAPIURL
//
// Token precedence, highest to lowest (profiles carry no token):
//  1. flagToken
//  2. YOMIRO_API_TOKEN env var
//  3. stored credentials
//
// flagAPIURL/flagToken are the already-extracted *changed* flag values; pass
// "" when the flag was not set. When profileAPIURL == "" the behavior is
// identical to the no-profile case. Keeping cobra out of this signature makes
// it trivially table-testable and keeps the storage package cobra-free.
func Resolve(flagAPIURL, flagToken, profileAPIURL string) (apiURL, token string) {
	apiURL = DefaultAPIURL

	// Stored credentials are best-effort: a missing/unreadable store just
	// means "use env/flag/default", matching the prior call-site behavior.
	if store, err := New(); err == nil {
		if c, err := store.Load(); err == nil {
			if c.APIURL != "" {
				apiURL = c.APIURL
			}
			token = c.Token
		}
	}

	// A --env profile's API URL outranks stored creds but loses to an
	// explicit --api-url flag / YOMIRO_API_URL.
	if profileAPIURL != "" {
		apiURL = profileAPIURL
	}

	if v := os.Getenv("YOMIRO_API_URL"); v != "" {
		apiURL = v
	}
	if v := os.Getenv("YOMIRO_API_TOKEN"); v != "" {
		token = v
	}

	if flagAPIURL != "" {
		apiURL = flagAPIURL
	}
	if flagToken != "" {
		token = flagToken
	}

	return apiURL, token
}
