package credentials

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	s := &fileStore{path: path}

	c := Credentials{APIURL: "https://api.example", Token: "yom_pat_TEST", User: "u@example.com", Tenant: "tenant-1"}
	if err := s.Save(c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != c {
		t.Fatalf("got %+v want %+v", got, c)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestFileStoreClearRemovesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "credentials")
	s := &fileStore{path: path}

	if err := s.Save(Credentials{Token: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file removed, got err=%v", err)
	}
}

func TestFileStoreLoadMissingReturnsErrNotFound(t *testing.T) {
	s := &fileStore{path: filepath.Join(t.TempDir(), "nope")}
	if _, err := s.Load(); err != ErrNotFound {
		t.Fatalf("got %v want ErrNotFound", err)
	}
}
