package overrides

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestRegisterAndGet(t *testing.T) {
	// Clear store for test isolation
	mu.Lock()
	store = make(map[string]*cobra.Command)
	mu.Unlock()

	cmd := &cobra.Command{Use: "list"}
	Register("items", cmd)

	got, ok := Get("items", "list")
	if !ok {
		t.Errorf("Get() returned ok=false, want true")
	}
	if got != cmd {
		t.Errorf("Get() returned %v, want %v", got, cmd)
	}
}

func TestGetMissing(t *testing.T) {
	// Clear store for test isolation
	mu.Lock()
	store = make(map[string]*cobra.Command)
	mu.Unlock()

	_, ok := Get("nonexistent", "cmd")
	if ok {
		t.Errorf("Get() for missing command returned ok=true, want false")
	}
}

func TestAllInGroup(t *testing.T) {
	// Clear store for test isolation
	mu.Lock()
	store = make(map[string]*cobra.Command)
	mu.Unlock()

	// Register multiple commands in same group
	cmd1 := &cobra.Command{Use: "create"}
	cmd2 := &cobra.Command{Use: "list"}
	cmd3 := &cobra.Command{Use: "delete"}
	cmd4 := &cobra.Command{Use: "get"}

	Register("items", cmd1)
	Register("items", cmd2)
	Register("items", cmd3)
	Register("items", cmd4)

	// Also register something in a different group to verify filtering
	otherCmd := &cobra.Command{Use: "view"}
	Register("users", otherCmd)

	all := AllInGroup("items")
	if len(all) != 4 {
		t.Errorf("AllInGroup() returned %d commands, want 4", len(all))
	}

	// Verify sorting
	expectedNames := []string{"create", "delete", "get", "list"}
	for i, cmd := range all {
		if cmd.Name() != expectedNames[i] {
			t.Errorf("AllInGroup()[%d].Name() = %s, want %s", i, cmd.Name(), expectedNames[i])
		}
	}
}

func TestAllInGroupUnknown(t *testing.T) {
	// Clear store for test isolation
	mu.Lock()
	store = make(map[string]*cobra.Command)
	mu.Unlock()

	all := AllInGroup("unknown")
	if len(all) != 0 {
		t.Errorf("AllInGroup() for unknown group returned %v, want nil or empty", all)
	}
}

func TestRegisterOverwrite(t *testing.T) {
	// Clear store for test isolation
	mu.Lock()
	store = make(map[string]*cobra.Command)
	mu.Unlock()

	cmd1 := &cobra.Command{Use: "list"}
	cmd2 := &cobra.Command{Use: "list", Short: "different"}

	Register("items", cmd1)
	Register("items", cmd2)

	got, ok := Get("items", "list")
	if !ok {
		t.Errorf("Get() returned ok=false after overwrite, want true")
	}
	if got != cmd2 {
		t.Errorf("Get() returned old command after overwrite, want new command")
	}
}
