// Package envprofile maps a target-environment name (local|dev|staging|prod) to a full
// set of environment values: API URL, Auth0 domain/client-id/audience, gateway
// WS endpoint, and frontend URL. It exists so a single `--env dev` flag (or
// YOMIRO_ENV) can expand to the whole bundle, while explicit per-field flags and
// env vars still override individual values upstream.
//
// This package is leaf-level on purpose: it imports only stdlib, cobra, and
// internal/credentials (for the shared DefaultAPIURL constant). It must NOT
// import auth, platform, or gw — those packages depend on this one, and the
// reverse would create an import cycle.
package envprofile

import (
	"fmt"
	"os"

	"github.com/yomiroco/yomiro-cli/internal/credentials"
	"github.com/spf13/cobra"
)

// Profile is the expanded set of environment values for a named environment.
// FrontendURL is empty for prod/dev (it is derived from the API host by the
// auth package's frontendFromAPI heuristic); only local needs it spelled out
// because localhost can't be derived from the API URL.
type Profile struct {
	Name          string
	APIURL        string
	Auth0Domain   string
	Auth0ClientID string
	Audience      string
	WSEndpoint    string
	FrontendURL   string
}

const (
	auth0Domain  = "yomiro.eu.auth0.com"
	prodClientID = "sUIOMvj0aaIRFbOPDcYUZ8bM5yCbWnA5"
	devClientID  = "oGQgnpBtymVPMJMkHbvP433o6n8sqO4e"
	// TODO(staging): replace with the real staging Auth0 client ID. Staging has
	// its own Auth0 application; until this is filled in, `--env staging` login
	// will fail loudly against Auth0 rather than silently hit the wrong tenant.
	stagingClientID   = "REPLACE_ME_STAGING_AUTH0_CLIENT_ID"
	devAPIURL         = "https://api.dev.yomiro.io"
	stagingAPIURL     = "https://api.staging.yomiro.io"
	localAPIURL       = "http://localhost:8000"
	prodWSEndpoint    = "wss://api.yomiro.io/api/v1/gateway/ws"
	devWSEndpoint     = "wss://api.dev.yomiro.io/api/v1/gateway/ws"
	stagingWSEndpoint = "wss://api.staging.yomiro.io/api/v1/gateway/ws"
	localWSEndpoint   = "ws://localhost:8000/api/v1/gateway/ws"
)

// profiles holds the authoritative env tables. prod's APIURL/Audience reuse
// credentials.DefaultAPIURL to keep one source of truth for that constant.
var profiles = map[string]Profile{
	"prod": {
		Name:          "prod",
		APIURL:        credentials.DefaultAPIURL,
		Auth0Domain:   auth0Domain,
		Auth0ClientID: prodClientID,
		Audience:      credentials.DefaultAPIURL,
		WSEndpoint:    prodWSEndpoint,
		FrontendURL:   "", // derived → app.yomiro.io
	},
	"dev": {
		Name:          "dev",
		APIURL:        devAPIURL,
		Auth0Domain:   auth0Domain,
		Auth0ClientID: devClientID,
		Audience:      devAPIURL,
		WSEndpoint:    devWSEndpoint,
		FrontendURL:   "", // derived → dev.yomiro.io
	},
	"staging": {
		Name:          "staging",
		APIURL:        stagingAPIURL,
		Auth0Domain:   auth0Domain,
		Auth0ClientID: stagingClientID,
		Audience:      stagingAPIURL,
		WSEndpoint:    stagingWSEndpoint,
		FrontendURL:   "", // derived → staging.yomiro.io
	},
	"local": {
		// local deliberately reuses the DEV Auth0 client + DEV audience: this
		// is the documented working local setup. It requires the local backend
		// to run with AUTH0_API_IDENTIFIER=https://api.dev.yomiro.io so the
		// token's audience matches what the backend validates.
		Name:          "local",
		APIURL:        localAPIURL,
		Auth0Domain:   auth0Domain,
		Auth0ClientID: devClientID,
		Audience:      devAPIURL,
		WSEndpoint:    localWSEndpoint,
		FrontendURL:   "http://localhost:5173",
	},
}

// Lookup returns the named profile. ok is false for an unknown name.
func Lookup(name string) (Profile, bool) {
	p, ok := profiles[name]
	return p, ok
}

// Active resolves the active profile from (highest precedence first):
//  1. the --env flag, if present and explicitly changed on cmd
//  2. the YOMIRO_ENV env var
//  3. the "prod" default
//
// It tolerates cmd == nil or a cmd without an "env" flag (it just falls through
// to the env var / default), so hermetic tests and pre-flag-parse callers don't
// panic. An unknown env name returns an error.
//
// The returned profile is always fully populated with the real per-env values
// (it is never mutated). The explicit bool reports whether the operator
// actively selected an env (a changed --env flag OR a set YOMIRO_ENV) versus
// falling through to the implicit prod default. Callers feeding
// credentials.Resolve gate the profile APIURL on explicit: an implicit prod
// default must NOT let the compiled prod APIURL override a stored login (that
// would regress the Task-2 "stored beats default" semantics), while an
// explicit --env dev legitimately beats a stored prod login. Fields other than
// APIURL (Auth0*, WSEndpoint, FrontendURL) are used regardless of explicit, so
// implicit prod still yields the prod WS endpoint / auth0 defaults.
func Active(cmd *cobra.Command) (Profile, bool, error) {
	name := ""
	explicit := false
	if cmd != nil {
		if f := cmd.Flag("env"); f != nil && f.Changed {
			name = f.Value.String()
			explicit = true
		}
	}
	if name == "" {
		if v := os.Getenv("YOMIRO_ENV"); v != "" {
			name = v
			explicit = true
		}
	}
	if name == "" {
		name = "prod"
	}
	p, ok := Lookup(name)
	if !ok {
		return Profile{}, false, fmt.Errorf("unknown --env %q: expected one of local, dev, staging, prod", name)
	}
	return p, explicit, nil
}
