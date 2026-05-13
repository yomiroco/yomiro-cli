// Package platform is a typed HTTP client for the platform's REST API.
//
// This file holds hand-written wrappers for endpoints the CLI uses directly.
// Auto-generated wrappers (oapi-codegen output) live alongside under client/.
package platform

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a thin wrapper around net/http for the platform API.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// New returns a Client. baseURL should be the platform origin without trailing slash.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}
}

// APIKey mirrors backend ApiKeyPublic.
type APIKey struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Prefix     string   `json:"prefix"`
	Scopes     []string `json:"scopes"`
	ExpiresAt  *string  `json:"expires_at"`
	CreatedAt  string   `json:"created_at"`
	LastUsedAt *string  `json:"last_used_at"`
	RevokedAt  *string  `json:"revoked_at"`
}

// APIKeyList mirrors ApiKeysPublic.
type APIKeyList struct {
	Data  []APIKey `json:"data"`
	Count int      `json:"count"`
}

// CreatedKey mirrors ApiKeyCreatedResponse — Token is shown only here.
type CreatedKey struct {
	Key   APIKey `json:"key"`
	Token string `json:"token"`
}

// CreateKeyRequest is the POST body for /api-keys.
type CreateKeyRequest struct {
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	ExpiresAt *string  `json:"expires_at,omitempty"`
}

// CurrentUser mirrors a subset of UserPublic — used by `whoami`. The
// backend's UserPublic schema returns `tenant_id` (flat UUID), not a nested
// tenant object; the display-friendly `Tenant.Name` is fetched separately
// via CurrentOrganization (GET /api/v1/organizations/me).
type CurrentUser struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	TenantID string `json:"tenant_id"`
	Tenant   struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"-"`
}

// CurrentOrganization is a minimal mirror of TenantPublic — only what the
// CLI surfaces in post-login output today.
type CurrentOrganization struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	InternalID string `json:"internal_id"`
}

func (c *Client) do(req *http.Request, out any) error {
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s %s -> %s: %s", req.Method, req.URL.Path, resp.Status, string(body))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// CreateAPIKey mints a new key. The cleartext token is in the response.
func (c *Client) CreateAPIKey(req CreateKeyRequest) (*CreatedKey, error) {
	body, _ := json.Marshal(req)
	r, _ := http.NewRequest(http.MethodPost, c.BaseURL+"/api/v1/auth/api-keys", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	var ck CreatedKey
	if err := c.do(r, &ck); err != nil {
		return nil, err
	}
	return &ck, nil
}

// ListAPIKeys returns all keys for the current user.
func (c *Client) ListAPIKeys() (*APIKeyList, error) {
	r, _ := http.NewRequest(http.MethodGet, c.BaseURL+"/api/v1/auth/api-keys", nil)
	var lst APIKeyList
	if err := c.do(r, &lst); err != nil {
		return nil, err
	}
	return &lst, nil
}

// RevokeAPIKey soft-deletes a key by ID.
func (c *Client) RevokeAPIKey(keyID string) error {
	r, _ := http.NewRequest(http.MethodDelete, c.BaseURL+"/api/v1/auth/api-keys/"+keyID, nil)
	return c.do(r, nil)
}

// Whoami returns the current user, with the tenant display name resolved
// via a follow-up GET /organizations/me. The latter is best-effort: if the
// caller's token doesn't have org scope or the user has no tenant_id, the
// Tenant fields stay at their zero values rather than the call erroring.
func (c *Client) Whoami() (*CurrentUser, error) {
	r, _ := http.NewRequest(http.MethodGet, c.BaseURL+"/api/v1/users/me", nil)
	var u CurrentUser
	if err := c.do(r, &u); err != nil {
		return nil, err
	}
	if org, err := c.CurrentOrganization(); err == nil && org != nil {
		u.Tenant.ID = org.ID
		u.Tenant.Name = org.Name
	}
	return &u, nil
}

// CurrentOrganization returns the tenant the bearer token resolves to.
func (c *Client) CurrentOrganization() (*CurrentOrganization, error) {
	r, _ := http.NewRequest(http.MethodGet, c.BaseURL+"/api/v1/organizations/me", nil)
	var o CurrentOrganization
	if err := c.do(r, &o); err != nil {
		return nil, err
	}
	return &o, nil
}
