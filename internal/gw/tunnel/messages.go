// Package tunnel implements the gateway-side WSS protocol client.
package tunnel

import "encoding/json"

// AuthFrame is the first message the gateway sends after connect.
type AuthFrame struct {
	Type      string    `json:"type"` // "auth"
	Token     string    `json:"token"`
	GatewayID string    `json:"gateway_id"`
	Version   string    `json:"version"`
	Manifest  *Manifest `json:"manifest,omitempty"`
}

type Manifest struct {
	Database   *DatabaseManifest `json:"database,omitempty"`
	Connectors []string          `json:"connectors"`
	Tools      []string          `json:"tools_supported"`
}

type DatabaseManifest struct {
	Type            string   `json:"type"`
	Schemas         []string `json:"schemas"`
	Tables          []string `json:"tables"`
	BlockedColumns  []string `json:"blocked_columns,omitempty"`
	MaxRowsPerQuery int      `json:"max_rows_per_query"`
	ReadOnly        bool     `json:"read_only"`
}

// ToolRequest is the platform-→-gateway message shape.
type ToolRequest struct {
	Type      string          `json:"type"` // "tool_request"
	RequestID string          `json:"request_id"`
	Tool      string          `json:"tool"`
	Params    json.RawMessage `json:"params"`
}

// ToolResponse is gateway-→-platform success.
type ToolResponse struct {
	Type      string         `json:"type"` // "tool_response"
	RequestID string         `json:"request_id"`
	Result    map[string]any `json:"result"`
	Trace     []TraceStep    `json:"trace,omitempty"`
}

// ToolError is gateway-→-platform failure.
type ToolError struct {
	Type      string      `json:"type"` // "tool_error"
	RequestID string      `json:"request_id"`
	Error     string      `json:"error"`
	Trace     []TraceStep `json:"trace,omitempty"`
}

// TraceStep mirrors backend.app.schemas.gateway.TraceStep.
type TraceStep struct {
	Step       string `json:"step"`
	Command    string `json:"command"`
	DurationMs int    `json:"duration_ms"`
	Result     string `json:"result,omitempty"`
	Found      *int   `json:"found,omitempty"`
}

// Heartbeat is sent by the gateway every N seconds.
type Heartbeat struct {
	Type    string `json:"type"` // "heartbeat"
	Status  string `json:"status"`
	Uptime  int    `json:"uptime_seconds"`
	Queries int    `json:"queries_served"`
}

// QueryParams is the params shape for tool="query".
type QueryParams struct {
	SQL       string `json:"sql"`
	TimeoutMs int    `json:"timeout_ms,omitempty"`
}
