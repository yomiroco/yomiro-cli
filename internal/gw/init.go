package gw

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/coder/websocket"
	"github.com/yomiroco/yomiro-cli/internal/auth"
	"github.com/yomiroco/yomiro-cli/internal/config"
	"github.com/yomiroco/yomiro-cli/internal/envprofile"
	"github.com/yomiroco/yomiro-cli/internal/platform"
	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
)

func newInitCmd() *cobra.Command {
	var (
		token            string
		fromLogin        bool
		dbURL            string
		allowSchemas     []string
		allowTables      []string
		blockColumns     []string
		maxRows          int
		connectors       []string
		gatewayID        string
		platformEndpoint string
	)
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Bootstrap gw.yaml + store the gateway:tunnel token",
		RunE: func(cmd *cobra.Command, args []string) error {
			web, _ := cmd.Flags().GetBool("web")

			if token != "" && fromLogin {
				return fmt.Errorf("--token and --from-login are mutually exclusive")
			}
			if token == "" && !fromLogin {
				return fmt.Errorf("one of --token or --from-login is required")
			}

			if gatewayID == "" {
				h, _ := os.Hostname()
				if h == "" {
					h = "gw-default"
				}
				gatewayID = "gw-" + h
			}

			// gw init uses prof.WSEndpoint, which is populated for every env
			// (including the implicit prod default), so the explicit bool is
			// irrelevant here — discard it.
			prof, _, err := envprofile.Active(cmd)
			if err != nil {
				return err
			}
			platformEndpoint = resolveEndpoint(cmd, prof)

			if fromLogin {
				minted, err := mintGatewayToken(cmd, cmd.OutOrStdout(), web, gatewayID)
				if err != nil {
					return err
				}
				token = minted
			}

			if err := keyring.Set(keyringServiceGw, gatewayID, token); err != nil {
				return fmt.Errorf("keyring write (consider file fallback): %w", err)
			}

			gwYAML, err := config.GwConfigFile()
			if err != nil {
				return err
			}
			c := &config.GwConfig{
				Platform: config.PlatformConfig{
					Endpoint: platformEndpoint,
					TokenRef: fmt.Sprintf("keyring:%s/%s", keyringServiceGw, gatewayID),
				},
				Gateway: config.GatewayIdentity{ID: gatewayID, Version: "0.1.0"},
				Database: config.DatabaseConfig{
					URL:                 dbURL,
					ReadOnly:            true,
					MaxConnections:      5,
					QueryTimeoutSeconds: 30,
					AllowedSchemas:      allowSchemas,
					AllowedTables:       allowTables,
					BlockedColumns:      blockColumns,
					MaxRowsPerQuery:     maxRows,
				},
				Connectors: config.ConnectorsConfig{Enabled: connectors},
				Daemon: config.DaemonConfig{
					AutoStart:            true,
					ReconnectMaxBackoffS: 60,
					HeartbeatIntervalS:   30,
				},
				Logging: config.LoggingConfig{Level: "info"},
			}

			stateDir, err := config.StateDir()
			if err != nil {
				return err
			}
			c.Logging.AuditPath = stateDir + "/audit.log"

			if err := config.SaveGwConfig(gwYAML, c); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Wrote %s\n", gwYAML)
			fmt.Fprintf(cmd.OutOrStdout(), "✓ Stored token in keyring (%s/%s)\n", keyringServiceGw, gatewayID)

			// Probe the resolved endpoint so a wrong/undeployed URL surfaces here
			// as an actionable warning instead of as a silent 404 in the daemon
			// log later. Non-fatal: config is already written.
			probeCtx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			if ok, reason := probeWSEndpoint(probeCtx, platformEndpoint); ok {
				fmt.Fprintf(cmd.OutOrStdout(), "✓ Endpoint reachable (%s)\n", platformEndpoint)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "⚠ Endpoint check: %s (%s). Wrote config anyway — fix --endpoint / --env if the daemon can't connect. Try --env dev / --env local, or --endpoint wss://api.dev.yomiro.io/api/v1/gateway/ws\n", reason, platformEndpoint)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Run `yomiro gw up` to start the daemon.")
			return nil
		},
	}
	cmd.Flags().StringVar(&token, "token", "", "Pre-minted gateway:tunnel token (or use --from-login to mint one)")
	cmd.Flags().BoolVar(&fromLogin, "from-login", false, "Mint a gateway:tunnel token via interactive login instead of passing --token (set YOMIRO_API_TOKEN to an Auth0 JWT to mint non-interactively)")
	cmd.Flags().StringVar(&dbURL, "db-url", "", "Postgres connection URL (e.g. postgres://u:p@host/db)")
	cmd.Flags().StringSliceVar(&allowSchemas, "allow-schema", nil, "Allowlisted schema (repeatable)")
	cmd.Flags().StringSliceVar(&allowTables, "allow-table", nil, "Allowlisted table FQN (repeatable)")
	cmd.Flags().StringSliceVar(&blockColumns, "block-column", nil, "Blocked column (repeatable, e.g. users.email)")
	cmd.Flags().IntVar(&maxRows, "max-rows", 10000, "Max rows per query")
	cmd.Flags().StringSliceVar(&connectors, "connectors", []string{"postgres"}, "Connectors to enable (postgres,otel,mqtt,modbus,opcua,sonos,generic)")
	cmd.Flags().StringVar(&gatewayID, "gateway-id", "", "Gateway ID (default: gw-<hostname>)")
	cmd.Flags().StringVar(&platformEndpoint, "endpoint", "", "Platform WSS endpoint (default: derived from --env)")
	auth.AddAuthFlags(cmd)
	return cmd
}

// resolveEndpoint picks the gateway WSS endpoint: an explicit (changed)
// --endpoint flag wins, otherwise the active --env profile's WSEndpoint. This
// closes the bug where the endpoint defaulted to prod and silently 404'd
// against dev/local.
func resolveEndpoint(cmd *cobra.Command, prof envprofile.Profile) string {
	if f := cmd.Flag("endpoint"); f != nil && f.Changed {
		return f.Value.String()
	}
	return prof.WSEndpoint
}

// probeWSEndpoint attempts a WebSocket upgrade to the gateway endpoint to
// catch a wrong/undeployed URL at init instead of as a silent 404 in the
// daemon log later. It sends no auth — it only checks the upgrade route
// exists. Returns ok=true when the route is valid (101 upgrade, or a
// 401/403 meaning the route exists but auth happens at run time). Returns
// ok=false with a human-readable reason for a 404 (not a gateway route) or
// an unreachable host (DNS/refused/timeout).
func probeWSEndpoint(ctx context.Context, url string) (ok bool, reason string) {
	conn, resp, err := websocket.Dial(ctx, url, nil)
	if conn != nil {
		conn.Close(websocket.StatusNormalClosure, "")
	}
	if err == nil {
		// 101 upgrade succeeded — the route exists and accepted us.
		return true, ""
	}
	if resp == nil {
		// DNS failure, connection refused, timeout: no HTTP response at all.
		return false, fmt.Sprintf("unreachable: %v", err)
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		// Route exists; auth happens at run time with the real token.
		return true, ""
	case http.StatusNotFound:
		return false, "endpoint returned 404 — not a gateway WS route"
	default:
		return false, fmt.Sprintf("endpoint returned %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
}

// mintGatewayToken acquires a fresh credential (device-code, browser, or a
// YOMIRO_API_TOKEN JWT passthrough) and mints a gateway:tunnel key named
// after the gateway. Returns the cleartext token to store in the keyring.
func mintGatewayToken(cmd *cobra.Command, out io.Writer, web bool, gatewayID string) (string, error) {
	cfg, err := auth.ResolveAuthConfig(cmd)
	if err != nil {
		return "", err
	}
	// NOTE: --web mints via the browser picker, so the key's name/scopes/lifetime come from the picker, not gatewayID (unlike the device-code path below, which names the key after the gateway).
	if web {
		frontend, err := auth.ResolveFrontendURL(auth.FirstNonEmpty(os.Getenv("YOMIRO_FRONTEND_URL"), cfg.Profile.FrontendURL), cfg.APIURL)
		if err != nil {
			return "", err
		}
		return auth.AcquireViaWeb(out, platform.New(cfg.APIURL, ""), frontend, []string{"gateway:tunnel"})
	}
	jwt, err := auth.AcquireJWT(out, cfg.DC, os.Getenv("YOMIRO_API_TOKEN"))
	if err != nil {
		return "", err
	}
	created, err := platform.New(cfg.APIURL, jwt).CreateAPIKey(platform.CreateKeyRequest{
		Name:   gatewayID,
		Scopes: []string{"gateway:tunnel"},
	})
	if err != nil {
		return "", fmt.Errorf("mint gateway token: %w", err)
	}
	return created.Token, nil
}
