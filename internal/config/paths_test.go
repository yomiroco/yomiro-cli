package config

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestConfigDirRespectsXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/yomtest/cfg")
	got, err := ConfigDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join("/tmp/yomtest/cfg", "yomiro")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestStateDirRespectsXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/yomtest/state")
	got, err := StateDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join("/tmp/yomtest/state", "yomiro")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestConfigDirFallsBackToHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("home-dir layout differs on Windows")
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/tmp/yomtest/home")
	got, err := ConfigDir()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join("/tmp/yomtest/home", ".config", "yomiro")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestCredentialsFilePath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/yomtest/cfg")
	got, err := CredentialsFile()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join("/tmp/yomtest/cfg", "yomiro", "credentials")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
