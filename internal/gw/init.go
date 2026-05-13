package gw

import (
	"fmt"
	"os"

	"github.com/yomiroco/yomiro-cli/internal/config"
	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
)

func newInitCmd() *cobra.Command {
	var (
		token            string
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
			if token == "" {
				return fmt.Errorf("--token is required")
			}
			if gatewayID == "" {
				h, _ := os.Hostname()
				if h == "" {
					h = "gw-default"
				}
				gatewayID = "gw-" + h
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
			fmt.Fprintln(cmd.OutOrStdout(), "Run `yomiro gw up` to start the daemon.")
			return nil
		},
	}
	cmd.Flags().StringVar(&token, "token", "", "API token with gateway:tunnel scope (required)")
	cmd.Flags().StringVar(&dbURL, "db-url", "", "Postgres connection URL (e.g. postgres://u:p@host/db)")
	cmd.Flags().StringSliceVar(&allowSchemas, "allow-schema", nil, "Allowlisted schema (repeatable)")
	cmd.Flags().StringSliceVar(&allowTables, "allow-table", nil, "Allowlisted table FQN (repeatable)")
	cmd.Flags().StringSliceVar(&blockColumns, "block-column", nil, "Blocked column (repeatable, e.g. users.email)")
	cmd.Flags().IntVar(&maxRows, "max-rows", 10000, "Max rows per query")
	cmd.Flags().StringSliceVar(&connectors, "connectors", []string{"postgres"}, "Connectors to enable (postgres,otel,mqtt,modbus,opcua,sonos,generic)")
	cmd.Flags().StringVar(&gatewayID, "gateway-id", "", "Gateway ID (default: gw-<hostname>)")
	cmd.Flags().StringVar(&platformEndpoint, "endpoint", "wss://api.yomiro.io/api/v1/gateway/ws", "Platform WSS endpoint")
	_ = cmd.MarkFlagRequired("token")
	return cmd
}
