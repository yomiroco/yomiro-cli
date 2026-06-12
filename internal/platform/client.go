// Package platform is a typed HTTP client for the platform's REST API.
//
// The api-key and identity wrappers (CreateAPIKey, ListAPIKeys, RevokeAPIKey,
// Whoami, CurrentOrganization) delegate to the generated oapi-codegen client
// under client/ — only request/response shaping lives here. The cli-pair
// browser-pairing flow stays hand-written: its polling loop, lenient decode,
// and free-text 410-reason parsing have no clean generated equivalent.
package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/yomiroco/yomiro-cli/internal/platform/client"
)

// ErrCLIPairGone is returned by PollCLIPair when the backend reports the
// pairing can no longer complete (HTTP 410 — denied or expired). The wrapped
// message carries the operator-facing reason.
var ErrCLIPairGone = errors.New("cli pairing gone")

// Client wraps the generated platform client with the CLI-facing method
// surface and request/response shaping the auth commands expect. The cli-pair
// flow uses the embedded base URL / token / http client directly.
type Client struct {
	gen     *client.ClientWithResponses
	baseURL string
	token   string
	http    *http.Client
}

// New returns a Client. baseURL should be the platform origin without trailing
// slash; an empty token is acceptable (help/build flows run pre-login).
func New(baseURL, token string) *Client {
	base := strings.TrimRight(baseURL, "/")
	// safe to discard: only WithRequestEditorFn is passed (which cannot error)
	// and the server string is non-empty, so the constructor cannot fail.
	gen, _ := client.NewClientWithResponses(base, client.WithRequestEditorFn(bearerEditor(token)))
	return &Client{
		gen:     gen,
		baseURL: base,
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// BaseURL returns the platform origin the client targets (no trailing slash).
// Exposed so callers/tests can assert which environment a client resolved to
// without making a request.
func (c *Client) BaseURL() string { return c.baseURL }

// bearerEditor attaches a Bearer token to outbound requests. An empty token is
// a no-op so unauthenticated flows still work.
func bearerEditor(token string) client.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		return nil
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

// apiKeyFromPublic maps a generated ApiKeyPublic into the CLI's APIKey shape.
func apiKeyFromPublic(k client.ApiKeyPublic) APIKey {
	return APIKey{
		ID:         k.Id.String(),
		Name:       k.Name,
		Prefix:     k.Prefix,
		Scopes:     k.Scopes,
		ExpiresAt:  k.ExpiresAt,
		CreatedAt:  k.CreatedAt,
		LastUsedAt: k.LastUsedAt,
		RevokedAt:  k.RevokedAt,
	}
}

// toScopes converts plain scope strings into the generated ApiKeyScope type.
func toScopes(scopes []string) []client.ApiKeyScope {
	out := make([]client.ApiKeyScope, len(scopes))
	for i, s := range scopes {
		out[i] = client.ApiKeyScope(s)
	}
	return out
}

// CreateAPIKey mints a new key. The cleartext token is in the response.
func (c *Client) CreateAPIKey(req CreateKeyRequest) (*CreatedKey, error) {
	body := client.ApiKeyCreate{Name: req.Name, Scopes: toScopes(req.Scopes)}
	if req.ExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			return nil, fmt.Errorf("parse expires_at: %w", err)
		}
		body.ExpiresAt = &t
	}
	resp, err := c.gen.AuthCreateApiKeyWithResponse(context.Background(), &client.AuthCreateApiKeyParams{}, body)
	if err != nil {
		return nil, err
	}
	if resp.JSON201 == nil {
		return nil, responseError(http.MethodPost, "/api/v1/auth/api-keys", resp.Status(), resp.Body)
	}
	return &CreatedKey{Key: apiKeyFromPublic(resp.JSON201.Key), Token: resp.JSON201.Token}, nil
}

// ListAPIKeys returns all keys for the current user.
func (c *Client) ListAPIKeys() (*APIKeyList, error) {
	resp, err := c.gen.AuthListApiKeysWithResponse(context.Background(), &client.AuthListApiKeysParams{})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, responseError(http.MethodGet, "/api/v1/auth/api-keys", resp.Status(), resp.Body)
	}
	lst := &APIKeyList{Count: resp.JSON200.Count, Data: make([]APIKey, len(resp.JSON200.Data))}
	for i, k := range resp.JSON200.Data {
		lst.Data[i] = apiKeyFromPublic(k)
	}
	return lst, nil
}

// RevokeAPIKey soft-deletes a key by ID.
func (c *Client) RevokeAPIKey(keyID string) error {
	id, err := uuid.Parse(keyID)
	if err != nil {
		return fmt.Errorf("invalid key id %q: %w", keyID, err)
	}
	resp, err := c.gen.AuthRevokeApiKeyWithResponse(context.Background(), id, &client.AuthRevokeApiKeyParams{})
	if err != nil {
		return err
	}
	if resp.StatusCode()/100 != 2 {
		return responseError(http.MethodDelete, "/api/v1/auth/api-keys/"+keyID, resp.Status(), resp.Body)
	}
	return nil
}

// Whoami returns the current user, with the tenant display name resolved
// via a follow-up GET /organizations/me. The latter is best-effort: if the
// caller's token doesn't have org scope or the user has no tenant_id, the
// Tenant fields stay at their zero values rather than the call erroring.
func (c *Client) Whoami() (*CurrentUser, error) {
	resp, err := c.gen.UsersReadUserMeWithResponse(context.Background(), &client.UsersReadUserMeParams{})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, responseError(http.MethodGet, "/api/v1/users/me", resp.Status(), resp.Body)
	}
	u := &CurrentUser{
		ID:       resp.JSON200.Id.String(),
		Email:    string(resp.JSON200.Email),
		TenantID: resp.JSON200.TenantId.String(),
	}
	if org, err := c.CurrentOrganization(); err == nil && org != nil {
		u.Tenant.ID = org.ID
		u.Tenant.Name = org.Name
	}
	return u, nil
}

// CurrentOrganization returns the tenant the bearer token resolves to.
func (c *Client) CurrentOrganization() (*CurrentOrganization, error) {
	resp, err := c.gen.OrganizationsReadOrganizationMeWithResponse(context.Background(), &client.OrganizationsReadOrganizationMeParams{})
	if err != nil {
		return nil, err
	}
	if resp.JSON200 == nil {
		return nil, responseError(http.MethodGet, "/api/v1/organizations/me", resp.Status(), resp.Body)
	}
	return &CurrentOrganization{
		ID:         resp.JSON200.Id.String(),
		Name:       resp.JSON200.Name,
		InternalID: resp.JSON200.InternalId,
	}, nil
}

// CreateCLIPairRequest is the POST body for /cli-pair — starts a browser
// pairing tied to the calling operator's Auth0 token.
type CreateCLIPairRequest struct {
	Hostname      string   `json:"hostname"`
	DefaultScopes []string `json:"default_scopes"`
}

// cliPairCreatedResponse mirrors backend CliPairCreatedResponse.
type cliPairCreatedResponse struct {
	Code      string `json:"code"`
	ExpiresAt string `json:"expires_at"`
}

// cliPairPublic mirrors the fields of backend CliPairPublic the CLI polls for.
// APIKeyToken is populated exactly once — on the first poll after the operator
// authorizes in the browser.
type cliPairPublic struct {
	Code           string   `json:"code"`
	SelectedScopes []string `json:"selected_scopes"`
	AuthorizedAt   *string  `json:"authorized_at"`
	APIKeyToken    *string  `json:"api_key_token"`
}

// CreateCLIPairRequest starts a browser-based pairing using the Auth0 JWT and
// returns the short code the operator opens in the web app to pick scopes.
//
// Kept hand-written rather than delegating to the generated client: it shares
// the lenient decode with the custom PollCLIPair flow below.
func (c *Client) CreateCLIPairRequest(hostname string, defaultScopes []string) (string, error) {
	body, _ := json.Marshal(CreateCLIPairRequest{Hostname: hostname, DefaultScopes: defaultScopes})
	r, _ := http.NewRequest(http.MethodPost, c.baseURL+"/api/v1/auth/cli-pair", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	var created cliPairCreatedResponse
	if err := c.do(r, &created); err != nil {
		return "", err
	}
	return created.Code, nil
}

// do issues a request with the bearer token attached and decodes a 2xx JSON
// body into out. Used only by the hand-written cli-pair pairing calls.
func (c *Client) do(req *http.Request, out any) error {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
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

// PollCLIPair polls the pairing record until the operator authorizes it in the
// browser and the single-use API key token appears. It returns:
//   - the cleartext token once api_key_token is populated,
//   - ErrCLIPairGone (with a denied/expired reason) on HTTP 410,
//   - a timeout error after max elapses while the record is still unauthorized.
func (c *Client) PollCLIPair(code string, max time.Duration, interval time.Duration) (string, error) {
	deadline := time.Now().Add(max)
	for {
		token, err := c.pollCLIPairOnce(code)
		if err != nil {
			return "", err
		}
		if token != "" {
			return token, nil
		}
		if !time.Now().Add(interval).Before(deadline) {
			return "", fmt.Errorf("timed out waiting for browser authorization after %s; re-run `yomiro auth login --web` to try again", max)
		}
		time.Sleep(interval)
	}
}

// pollCLIPairOnce performs a single GET. It returns the token if present (and
// thus picked up), an empty string if the record exists but isn't authorized
// yet, or ErrCLIPairGone on 410.
func (c *Client) pollCLIPairOnce(code string) (string, error) {
	r, _ := http.NewRequest(http.MethodGet, c.baseURL+"/api/v1/auth/cli-pair/"+code, nil)
	if c.token != "" {
		r.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(r)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusGone {
		body, _ := io.ReadAll(resp.Body)
		reason := strings.TrimSpace(string(body))
		// Intentionally coupled to the backend's free-text 410 detail string
		// ("Pairing was denied" vs "Pairing code expired"). The proper fix is
		// a machine-readable reason field on the 410 response (follow-up).
		if strings.Contains(strings.ToLower(reason), "deni") {
			return "", fmt.Errorf("%w: authorization denied by operator", ErrCLIPairGone)
		}
		return "", fmt.Errorf("%w: pairing code expired", ErrCLIPairGone)
	}
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("GET %s -> %s: %s", r.URL.Path, resp.Status, string(body))
	}

	var record cliPairPublic
	if err := json.NewDecoder(resp.Body).Decode(&record); err != nil {
		return "", err
	}
	if record.APIKeyToken != nil && *record.APIKeyToken != "" {
		return *record.APIKeyToken, nil
	}
	return "", nil
}

// responseError builds the error returned for a non-success generated-client
// response, preserving the request method/path, status, and raw body so
// callers (and operators) see the server's detail message.
func responseError(method, path string, status string, body []byte) error {
	return fmt.Errorf("%s %s -> %s: %s", method, path, status, string(body))
}
