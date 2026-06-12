package platform

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateAPIKeySendsBodyAndAuth(t *testing.T) {
	var gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(CreatedKey{
			Key:   APIKey{ID: "33333333-3333-3333-3333-333333333333", Name: "test", Prefix: "yom_pat_AAAA", Scopes: []string{"agents:read"}},
			Token: "yom_pat_AAAAAAAAAAAA",
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "auth0-jwt")
	got, err := c.CreateAPIKey(CreateKeyRequest{Name: "test", Scopes: []string{"agents:read"}})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if got.Token != "yom_pat_AAAAAAAAAAAA" {
		t.Fatalf("token = %q", got.Token)
	}
	if gotAuth != "Bearer auth0-jwt" {
		t.Fatalf("auth header = %q", gotAuth)
	}
	if !strings.Contains(gotBody, `"name":"test"`) {
		t.Fatalf("body = %q", gotBody)
	}
}

func TestCreateAPIKeyReturnsServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"detail":"You cannot grant these scopes: ['tenant:admin']"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	_, err := c.CreateAPIKey(CreateKeyRequest{Name: "x", Scopes: []string{"tenant:admin"}})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "tenant:admin") {
		t.Fatalf("err = %v", err)
	}
}

func TestListAPIKeysParsesResponse(t *testing.T) {
	keyID := "11111111-1111-1111-1111-111111111111"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":  []APIKey{{ID: keyID, Name: "k", Prefix: "yom_pat_X"}},
			"count": 1,
		})
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	got, err := c.ListAPIKeys()
	if err != nil {
		t.Fatal(err)
	}
	if got.Count != 1 || len(got.Data) != 1 {
		t.Fatalf("got %+v", got)
	}
}

// TestGetMethodsReturnServerError locks in the status-check-before-deref
// pattern for the migrated GET wrappers: on a non-2xx response the typed
// JSONxxx body is nil, so each method must surface the responseError-formatted
// error (method + path + status + body) rather than nil-deref the body.
func TestGetMethodsReturnServerError(t *testing.T) {
	cases := []struct {
		name string
		call func(c *Client) error
	}{
		{
			name: "ListAPIKeys",
			call: func(c *Client) error { _, err := c.ListAPIKeys(); return err },
		},
		{
			name: "Whoami",
			call: func(c *Client) error { _, err := c.Whoami(); return err },
		},
		{
			name: "CurrentOrganization",
			call: func(c *Client) error { _, err := c.CurrentOrganization(); return err },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"detail":"insufficient scope"}`))
			}))
			defer srv.Close()

			c := New(srv.URL, "tok")
			err := tc.call(c)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), "insufficient scope") {
				t.Fatalf("err = %v, want server detail body", err)
			}
			if !strings.Contains(err.Error(), "403") {
				t.Fatalf("err = %v, want status in message", err)
			}
		})
	}
}

func TestRevokeAPIKey(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}))
	defer srv.Close()

	keyID := "22222222-2222-2222-2222-222222222222"
	c := New(srv.URL, "tok")
	if err := c.RevokeAPIKey(keyID); err != nil {
		t.Fatal(err)
	}
	if gotMethod != "DELETE" {
		t.Fatalf("method = %q", gotMethod)
	}
	if !strings.HasSuffix(gotPath, "/api/v1/auth/api-keys/"+keyID) {
		t.Fatalf("path = %q", gotPath)
	}
}
