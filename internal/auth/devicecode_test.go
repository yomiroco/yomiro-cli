package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDeviceCodeStartHitsCorrectEndpoint(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(deviceCodeResponse{
			DeviceCode:      "dc-1",
			UserCode:        "ABCD-1234",
			VerificationURI: "https://example.com/activate",
			ExpiresIn:       900,
			Interval:        5,
		})
	}))
	defer srv.Close()

	c := &DeviceCodeClient{Domain: strings.TrimPrefix(srv.URL, "http://"), ClientID: "cid", Audience: "aud", Scheme: "http"}
	resp, err := c.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if resp.UserCode != "ABCD-1234" {
		t.Fatalf("user code = %q", resp.UserCode)
	}
	if gotPath != "/oauth/device/code" {
		t.Fatalf("path = %q", gotPath)
	}
	if !strings.Contains(gotBody, "client_id=cid") || !strings.Contains(gotBody, "audience=aud") {
		t.Fatalf("body = %q", gotBody)
	}
}

func TestDeviceCodePollReturnsTokenOnSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "auth0-jwt",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		})
	}))
	defer srv.Close()

	c := &DeviceCodeClient{Domain: strings.TrimPrefix(srv.URL, "http://"), ClientID: "cid", Scheme: "http"}
	tok, err := c.Poll("dc-1")
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if tok != "auth0-jwt" {
		t.Fatalf("token = %q", tok)
	}
}

func TestDeviceCodePollReturnsAuthorizationPendingOnPending(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(tokenError{Error: "authorization_pending"})
	}))
	defer srv.Close()

	c := &DeviceCodeClient{Domain: strings.TrimPrefix(srv.URL, "http://"), ClientID: "cid", Scheme: "http"}
	_, err := c.Poll("dc-1")
	if err != ErrAuthorizationPending {
		t.Fatalf("got %v want ErrAuthorizationPending", err)
	}
}

func TestDeviceCodePollReturnsSlowDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(tokenError{Error: "slow_down"})
	}))
	defer srv.Close()

	c := &DeviceCodeClient{Domain: strings.TrimPrefix(srv.URL, "http://"), ClientID: "cid", Scheme: "http"}
	_, err := c.Poll("dc-1")
	if err != ErrSlowDown {
		t.Fatalf("got %v want ErrSlowDown", err)
	}
}
