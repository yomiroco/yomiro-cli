package credentials

import (
	"os"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestMain(m *testing.M) {
	keyring.MockInit()
	os.Exit(m.Run())
}

// saveStored persists a Credentials row via the default store so Resolve picks
// it up. It clears first so tests don't leak state into one another.
func saveStored(t *testing.T, c Credentials) {
	t.Helper()
	s, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if err := s.Save(c); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

func clearStored(t *testing.T) {
	t.Helper()
	s, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
}

func TestResolvePrecedence(t *testing.T) {
	tests := []struct {
		name       string
		stored     *Credentials
		envURL     string
		envToken   string
		flagURL    string
		flagToken  string
		profileURL string
		wantAPIURL string
		wantToken  string
	}{
		{
			name:       "default when nothing set",
			wantAPIURL: DefaultAPIURL,
			wantToken:  "",
		},
		{
			name:       "stored beats default",
			stored:     &Credentials{APIURL: "https://stored.example", Token: "stored-tok"},
			wantAPIURL: "https://stored.example",
			wantToken:  "stored-tok",
		},
		{
			name:       "env beats stored",
			stored:     &Credentials{APIURL: "https://stored.example", Token: "stored-tok"},
			envURL:     "https://env.example",
			envToken:   "env-tok",
			wantAPIURL: "https://env.example",
			wantToken:  "env-tok",
		},
		{
			name:       "flag beats env",
			stored:     &Credentials{APIURL: "https://stored.example", Token: "stored-tok"},
			envURL:     "https://env.example",
			envToken:   "env-tok",
			flagURL:    "https://flag.example",
			flagToken:  "flag-tok",
			wantAPIURL: "https://flag.example",
			wantToken:  "flag-tok",
		},
		{
			name:       "token precedence independent of url: flag token, env url",
			stored:     &Credentials{APIURL: "https://stored.example", Token: "stored-tok"},
			envURL:     "https://env.example",
			flagToken:  "flag-tok",
			wantAPIURL: "https://env.example",
			wantToken:  "flag-tok",
		},
		{
			name:       "empty stored url falls back to default but keeps stored token",
			stored:     &Credentials{APIURL: "", Token: "stored-tok"},
			wantAPIURL: DefaultAPIURL,
			wantToken:  "stored-tok",
		},
		{
			name:       "profile beats stored (--env dev over a stored prod login)",
			stored:     &Credentials{APIURL: "https://stored.example", Token: "stored-tok"},
			profileURL: "https://api.dev.yomiro.io",
			wantAPIURL: "https://api.dev.yomiro.io",
			wantToken:  "stored-tok",
		},
		{
			name:       "env beats profile",
			stored:     &Credentials{APIURL: "https://stored.example", Token: "stored-tok"},
			profileURL: "https://api.dev.yomiro.io",
			envURL:     "https://env.example",
			wantAPIURL: "https://env.example",
			wantToken:  "stored-tok",
		},
		{
			name:       "flag beats profile",
			stored:     &Credentials{APIURL: "https://stored.example", Token: "stored-tok"},
			profileURL: "https://api.dev.yomiro.io",
			flagURL:    "https://flag.example",
			wantAPIURL: "https://flag.example",
			wantToken:  "stored-tok",
		},
		{
			name:       "empty profile == old behavior (stored beats default)",
			stored:     &Credentials{APIURL: "https://stored.example", Token: "stored-tok"},
			profileURL: "",
			wantAPIURL: "https://stored.example",
			wantToken:  "stored-tok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.stored != nil {
				saveStored(t, *tt.stored)
			} else {
				clearStored(t)
			}
			t.Setenv("YOMIRO_API_URL", tt.envURL)
			t.Setenv("YOMIRO_API_TOKEN", tt.envToken)

			apiURL, token := Resolve(tt.flagURL, tt.flagToken, tt.profileURL)
			if apiURL != tt.wantAPIURL {
				t.Errorf("apiURL = %q, want %q", apiURL, tt.wantAPIURL)
			}
			if token != tt.wantToken {
				t.Errorf("token = %q, want %q", token, tt.wantToken)
			}
		})
	}
}
