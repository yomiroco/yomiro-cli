package auth

import "testing"

func TestFrontendFromAPI(t *testing.T) {
	cases := []struct{ apiURL, want string }{
		{"https://api.yomiro.io", "https://app.yomiro.io"},
		{"https://api.dev.yomiro.io", "https://dev.yomiro.io"},
		{"https://api.staging.yomiro.io", "https://staging.yomiro.io"},
		{"https://api.yomiro.io/api/v1", "https://app.yomiro.io"},
		{"http://localhost:8000", ""},
		{"https://example.com", ""},
		{"not a url", ""},
		{"", ""},
	}
	for _, tc := range cases {
		got := frontendFromAPI(tc.apiURL)
		if got != tc.want {
			t.Errorf("frontendFromAPI(%q) = %q, want %q", tc.apiURL, got, tc.want)
		}
	}
}
