package tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// Handler executes a tool request and returns its result. Each tool
// (query, status, scan, ...) is dispatched by name; the daemon registers
// handlers via Register.
type Handler func(ctx context.Context, params json.RawMessage) (result map[string]any, trace []TraceStep, err error)

// Client owns one WSS connection.
type Client struct {
	URL      string
	Auth     AuthFrame
	Handlers map[string]Handler

	HeartbeatInterval time.Duration
	WorkerLimit       int

	conn    *websocket.Conn
	mu      sync.Mutex
	queries int
	started time.Time
}

// Run blocks until ctx cancels or the connection drops permanently. The
// caller is responsible for the reconnect loop — Run returns on transport
// error or context cancellation.
func (c *Client) Run(ctx context.Context) error {
	if c.HeartbeatInterval == 0 {
		c.HeartbeatInterval = 30 * time.Second
	}
	if c.WorkerLimit == 0 {
		c.WorkerLimit = 8
	}

	conn, _, err := websocket.Dial(ctx, c.URL, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	c.conn = conn
	defer conn.Close(websocket.StatusNormalClosure, "")

	c.Auth.Type = "auth"
	if err := writeJSON(ctx, conn, c.Auth); err != nil {
		return fmt.Errorf("send auth: %w", err)
	}

	c.started = time.Now()
	hbCtx, hbCancel := context.WithCancel(ctx)
	defer hbCancel()
	go c.heartbeatLoop(hbCtx)

	jobs := make(chan ToolRequest, 64)
	var wg sync.WaitGroup
	for i := 0; i < c.WorkerLimit; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for req := range jobs {
				c.dispatch(ctx, req)
			}
		}()
	}

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			close(jobs)
			wg.Wait()
			return err
		}
		var msg map[string]any
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		switch msg["type"] {
		case "tool_request":
			var req ToolRequest
			_ = json.Unmarshal(data, &req)
			jobs <- req
		case "control":
			// pause/resume from platform side — daemon handles via control socket too,
			// but if platform sends, we acknowledge.
		}
	}
}

func (c *Client) dispatch(ctx context.Context, req ToolRequest) {
	h, ok := c.Handlers[req.Tool]
	if !ok {
		_ = c.send(ctx, ToolError{Type: "tool_error", RequestID: req.RequestID, Error: "unknown tool: " + req.Tool})
		return
	}
	result, trace, err := h(ctx, req.Params)
	c.mu.Lock()
	c.queries++
	c.mu.Unlock()
	if err != nil {
		_ = c.send(ctx, ToolError{Type: "tool_error", RequestID: req.RequestID, Error: err.Error(), Trace: trace})
		return
	}
	_ = c.send(ctx, ToolResponse{Type: "tool_response", RequestID: req.RequestID, Result: result, Trace: trace})
}

func (c *Client) heartbeatLoop(ctx context.Context) {
	t := time.NewTicker(c.HeartbeatInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.mu.Lock()
			q := c.queries
			c.mu.Unlock()
			_ = c.send(ctx, Heartbeat{
				Type:   "heartbeat",
				Status: "ok",
				Uptime: int(time.Since(c.started).Seconds()), Queries: q,
			})
		}
	}
}

func (c *Client) send(ctx context.Context, v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return writeJSON(ctx, c.conn, v)
}

func writeJSON(ctx context.Context, conn *websocket.Conn, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, b)
}

// Stats returns counters for `gw status`.
func (c *Client) Stats() (uptimeS int, queries int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return int(time.Since(c.started).Seconds()), c.queries
}
