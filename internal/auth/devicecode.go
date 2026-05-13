// Package auth implements the Auth0 device-code OAuth flow and the operator
// auth subcommands.
package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Auth0 device-code well-known errors per RFC 8628 § 3.5.
var (
	ErrAuthorizationPending = errors.New("authorization pending")
	ErrSlowDown             = errors.New("slow down")
	ErrAccessDenied         = errors.New("access denied")
	ErrExpiredToken         = errors.New("device code expired")
)

// DeviceCodeClient talks to an Auth0 tenant using the device-authorization
// flow (RFC 8628). Scheme is "https" in production; tests inject "http".
type DeviceCodeClient struct {
	Domain   string
	ClientID string
	Audience string
	Scheme   string
	HTTP     *http.Client
}

func (c *DeviceCodeClient) scheme() string {
	if c.Scheme == "" {
		return "https"
	}
	return c.Scheme
}

func (c *DeviceCodeClient) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri_complete"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

type tokenError struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// Start initiates a device authorization request and returns the user-facing
// code, verification URL, and polling parameters.
func (c *DeviceCodeClient) Start() (*deviceCodeResponse, error) {
	body := url.Values{
		"client_id": {c.ClientID},
		"scope":     {"openid profile email offline_access"},
	}
	if c.Audience != "" {
		body.Set("audience", c.Audience)
	}
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s://%s/oauth/device/code", c.scheme(), c.Domain), strings.NewReader(body.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("device code request failed: %s", resp.Status)
	}

	var dcr deviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&dcr); err != nil {
		return nil, err
	}
	if dcr.Interval == 0 {
		dcr.Interval = 5
	}
	return &dcr, nil
}

// Poll exchanges the device code for an access token. Caller is responsible
// for the polling loop and respecting the interval / slow_down hint.
func (c *DeviceCodeClient) Poll(deviceCode string) (string, error) {
	body := url.Values{
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
		"device_code": {deviceCode},
		"client_id":   {c.ClientID},
	}
	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s://%s/oauth/token", c.scheme(), c.Domain), strings.NewReader(body.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 == 2 {
		var tr tokenResponse
		if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
			return "", err
		}
		return tr.AccessToken, nil
	}

	var te tokenError
	_ = json.NewDecoder(resp.Body).Decode(&te)
	switch te.Error {
	case "authorization_pending":
		return "", ErrAuthorizationPending
	case "slow_down":
		return "", ErrSlowDown
	case "access_denied":
		return "", ErrAccessDenied
	case "expired_token":
		return "", ErrExpiredToken
	default:
		return "", fmt.Errorf("token endpoint returned %s: %s", te.Error, te.ErrorDescription)
	}
}

// PollUntilDone runs Poll on a ticker, respecting authorization_pending and
// slow_down. Returns the access token or an error.
func (c *DeviceCodeClient) PollUntilDone(deviceCode string, interval int, expiresIn int) (string, error) {
	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)
	delay := time.Duration(interval) * time.Second
	for time.Now().Before(deadline) {
		time.Sleep(delay)
		tok, err := c.Poll(deviceCode)
		switch err {
		case nil:
			return tok, nil
		case ErrAuthorizationPending:
			continue
		case ErrSlowDown:
			delay += 5 * time.Second
			continue
		default:
			return "", err
		}
	}
	return "", ErrExpiredToken
}
