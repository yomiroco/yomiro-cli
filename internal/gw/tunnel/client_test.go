package tunnel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestClientHandshakesAndRespondsToToolRequest(t *testing.T) {
	receivedAuth := make(chan AuthFrame, 1)
	receivedResp := make(chan ToolResponse, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")

		ctx := r.Context()
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Errorf("read auth: %v", err)
			return
		}
		var auth AuthFrame
		_ = json.Unmarshal(data, &auth)
		receivedAuth <- auth

		req := ToolRequest{Type: "tool_request", RequestID: "r-1", Tool: "ping", Params: json.RawMessage(`{}`)}
		b, _ := json.Marshal(req)
		_ = conn.Write(ctx, websocket.MessageText, b)

		_, data, err = conn.Read(ctx)
		if err != nil {
			t.Errorf("read response: %v", err)
			return
		}
		var resp ToolResponse
		_ = json.Unmarshal(data, &resp)
		receivedResp <- resp
	}))
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	c := &Client{
		URL:  url,
		Auth: AuthFrame{Token: "yom_pat_TEST", GatewayID: "gw-test", Version: "0.1.0"},
		Handlers: map[string]Handler{
			"ping": func(ctx context.Context, _ json.RawMessage) (map[string]any, []TraceStep, error) {
				return map[string]any{"pong": true}, nil, nil
			},
		},
		HeartbeatInterval: time.Hour,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() { _ = c.Run(ctx) }()

	auth := <-receivedAuth
	if auth.Token != "yom_pat_TEST" || auth.GatewayID != "gw-test" {
		t.Fatalf("auth = %+v", auth)
	}
	resp := <-receivedResp
	if resp.RequestID != "r-1" || resp.Result["pong"] != true {
		t.Fatalf("resp = %+v", resp)
	}
}
