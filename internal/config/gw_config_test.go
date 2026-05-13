package config

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleYAML = `
platform:
  endpoint: wss://api.example/api/v1/gateway/ws
  token_ref: keyring:io.yomiro.gw/gw-test
gateway:
  id: gw-test
  version: 0.1.0
database:
  url: postgres://u:p@localhost:5432/db
  read_only: true
  max_connections: 5
  query_timeout_seconds: 30
  allowed_schemas: [public, inspections]
  allowed_tables: [public.defects, inspections.runs]
  blocked_columns: ["users.email"]
  max_rows_per_query: 5000
connectors:
  enabled: [postgres, modbus]
daemon:
  auto_start: true
  reconnect_max_backoff_s: 60
  heartbeat_interval_s: 30
logging:
  level: info
  audit_path: /tmp/audit.log
`

func TestLoadGwConfig(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gw.yaml")
	if err := os.WriteFile(p, []byte(sampleYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadGwConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Platform.Endpoint != "wss://api.example/api/v1/gateway/ws" {
		t.Fatalf("endpoint = %q", c.Platform.Endpoint)
	}
	if c.Gateway.ID != "gw-test" {
		t.Fatalf("gateway id = %q", c.Gateway.ID)
	}
	if !c.Database.ReadOnly {
		t.Fatal("expected read_only=true")
	}
	if len(c.Database.AllowedTables) != 2 {
		t.Fatalf("allowed_tables = %v", c.Database.AllowedTables)
	}
	if len(c.Connectors.Enabled) != 2 {
		t.Fatalf("connectors = %v", c.Connectors.Enabled)
	}
}

func TestSaveGwConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gw.yaml")
	in := &GwConfig{
		Platform: PlatformConfig{Endpoint: "wss://x/ws", TokenRef: "keyring:io.yomiro.gw/x"},
		Gateway:  GatewayIdentity{ID: "x", Version: "0.1.0"},
	}
	if err := SaveGwConfig(p, in); err != nil {
		t.Fatal(err)
	}
	out, err := LoadGwConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if out.Gateway.ID != "x" {
		t.Fatalf("round-trip lost id: %+v", out)
	}
	info, _ := os.Stat(p)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
}
