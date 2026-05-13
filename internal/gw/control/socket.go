// Package control implements a tiny request/response protocol over a Unix
// socket so `yomiro gw status/pause/resume/reload` can talk to a running daemon.
package control

import (
	"context"
	"encoding/json"
	"net"
	"os"
)

// Request is a single command from the CLI.
type Request struct {
	Action string         `json:"action"` // "status", "pause", "resume", "reload"
	Args   map[string]any `json:"args,omitempty"`
}

// Response is the daemon's reply.
type Response struct {
	OK     bool           `json:"ok"`
	Detail string         `json:"detail,omitempty"`
	Data   map[string]any `json:"data,omitempty"`
}

// Server holds a listening socket.
type Server struct {
	ln net.Listener
}

// Close closes the listener and removes the socket file.
func (s *Server) Close() error {
	addr := s.ln.Addr().String()
	err := s.ln.Close()
	_ = os.Remove(addr)
	return err
}

// Listen starts a Unix-socket listener and dispatches Requests to handler.
func Listen(path string, handler func(Request) Response) (*Server, error) {
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		return nil, err
	}
	srv := &Server{ln: ln}

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				var req Request
				if err := json.NewDecoder(conn).Decode(&req); err != nil {
					return
				}
				resp := handler(req)
				_ = json.NewEncoder(conn).Encode(resp)
			}(c)
		}
	}()
	return srv, nil
}

// Send connects to the socket, sends one Request, reads one Response.
func Send(ctx context.Context, path string, req Request) (*Response, error) {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "unix", path)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, err
	}
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
