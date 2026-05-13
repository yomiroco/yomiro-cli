// Package credentials stores the operator's API token + identity hints.
//
// Two backends:
//   - keyring (preferred): OS Keychain / Secret Service / Credential Manager
//   - file fallback: ~/.config/yomiro/credentials, mode 0600
//
// The store auto-selects keyring if available and silently falls back to file.
package credentials

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/yomiroco/yomiro-cli/internal/config"
	"github.com/zalando/go-keyring"
)

const (
	keyringService = "io.yomiro.cli"
	keyringAccount = "operator"
)

// ErrNotFound is returned when no credentials are stored.
var ErrNotFound = errors.New("credentials not found")

// Credentials are the persisted operator identity + token.
type Credentials struct {
	APIURL string `json:"api_url"`
	Token  string `json:"token"`
	User   string `json:"user,omitempty"`
	Tenant string `json:"tenant,omitempty"`
}

// Store abstracts the persistence layer.
type Store interface {
	Save(Credentials) error
	Load() (Credentials, error)
	Clear() error
}

// New returns the default store: keyring if available, else file.
func New() (Store, error) {
	// Probe keyring availability with a roundtrip on a junk key.
	if err := keyring.Set(keyringService, "_probe", "ok"); err == nil {
		_ = keyring.Delete(keyringService, "_probe")
		return &keyringStore{}, nil
	}
	path, err := config.CredentialsFile()
	if err != nil {
		return nil, err
	}
	return &fileStore{path: path}, nil
}

// --- keyring backend ---

type keyringStore struct{}

func (k *keyringStore) Save(c Credentials) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return keyring.Set(keyringService, keyringAccount, string(b))
}

func (k *keyringStore) Load() (Credentials, error) {
	s, err := keyring.Get(keyringService, keyringAccount)
	if errors.Is(err, keyring.ErrNotFound) {
		return Credentials{}, ErrNotFound
	}
	if err != nil {
		return Credentials{}, err
	}
	var c Credentials
	if err := json.Unmarshal([]byte(s), &c); err != nil {
		return Credentials{}, err
	}
	return c, nil
}

func (k *keyringStore) Clear() error {
	err := keyring.Delete(keyringService, keyringAccount)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// --- file backend ---

type fileStore struct{ path string }

func (f *fileStore) Save(c Credentials) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(f.path, b, 0o600)
}

func (f *fileStore) Load() (Credentials, error) {
	b, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return Credentials{}, ErrNotFound
	}
	if err != nil {
		return Credentials{}, err
	}
	var c Credentials
	if err := json.Unmarshal(b, &c); err != nil {
		return Credentials{}, err
	}
	return c, nil
}

func (f *fileStore) Clear() error {
	err := os.Remove(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
