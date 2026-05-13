package control

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestSocketRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gw.sock")

	got := make(chan Request, 1)
	srv, err := Listen(path, func(r Request) Response {
		got <- r
		return Response{OK: true, Detail: "ack"}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := Send(ctx, path, Request{Action: "status"})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Detail != "ack" {
		t.Fatalf("resp = %+v", resp)
	}
	r := <-got
	if r.Action != "status" {
		t.Fatalf("action = %q", r.Action)
	}
}
