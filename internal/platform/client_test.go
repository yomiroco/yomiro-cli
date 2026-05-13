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
		_ = json.NewEncoder(w).Encode(CreatedKey{
			Key:   APIKey{ID: "k1", Name: "test", Prefix: "yom_pat_AAAA", Scopes: []string{"agents:read"}},
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":  []APIKey{{ID: "k1", Name: "k", Prefix: "yom_pat_X"}},
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

func TestRevokeAPIKey(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	if err := c.RevokeAPIKey("abc-123"); err != nil {
		t.Fatal(err)
	}
	if gotMethod != "DELETE" {
		t.Fatalf("method = %q", gotMethod)
	}
	if !strings.HasSuffix(gotPath, "/api/v1/auth/api-keys/abc-123") {
		t.Fatalf("path = %q", gotPath)
	}
}
