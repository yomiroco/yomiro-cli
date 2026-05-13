package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type GwConfig struct {
	Platform   PlatformConfig   `yaml:"platform"`
	Gateway    GatewayIdentity  `yaml:"gateway"`
	Database   DatabaseConfig   `yaml:"database,omitempty"`
	Connectors ConnectorsConfig `yaml:"connectors"`
	Daemon     DaemonConfig     `yaml:"daemon"`
	Logging    LoggingConfig    `yaml:"logging"`
}

type PlatformConfig struct {
	Endpoint string `yaml:"endpoint"`
	TokenRef string `yaml:"token_ref"`
}

type GatewayIdentity struct {
	ID      string `yaml:"id"`
	Version string `yaml:"version"`
}

type DatabaseConfig struct {
	URL                 string   `yaml:"url"`
	ReadOnly            bool     `yaml:"read_only"`
	MaxConnections      int      `yaml:"max_connections"`
	QueryTimeoutSeconds int      `yaml:"query_timeout_seconds"`
	AllowedSchemas      []string `yaml:"allowed_schemas"`
	AllowedTables       []string `yaml:"allowed_tables"`
	BlockedColumns      []string `yaml:"blocked_columns"`
	MaxRowsPerQuery     int      `yaml:"max_rows_per_query"`
}

type ConnectorsConfig struct {
	Enabled []string `yaml:"enabled"`
}

type DaemonConfig struct {
	AutoStart            bool `yaml:"auto_start"`
	ReconnectMaxBackoffS int  `yaml:"reconnect_max_backoff_s"`
	HeartbeatIntervalS   int  `yaml:"heartbeat_interval_s"`
}

type LoggingConfig struct {
	Level     string `yaml:"level"`
	AuditPath string `yaml:"audit_path"`
}

func LoadGwConfig(path string) (*GwConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c GwConfig
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func SaveGwConfig(path string, c *GwConfig) error {
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}
