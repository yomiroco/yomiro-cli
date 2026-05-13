package bindings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

// sampleParams mirrors the shape oapi-codegen emits for query-only params:
// all pointer-of-scalar, with json+form tags.
type sampleParams struct {
	Skip        *int    `form:"skip,omitempty" json:"skip,omitempty"`
	Limit       *int64  `form:"limit,omitempty" json:"limit,omitempty"`
	AccessToken *string `form:"access_token,omitempty" json:"access_token,omitempty"`
	NameFilter  *string `json:"name_filter,omitempty"`
	Active      *bool   `json:"active,omitempty"`
}

func TestDefineQueryFlags_kebabFromJSONTag(t *testing.T) {
	cmd := &cobra.Command{}
	var p sampleParams
	DefineQueryFlags(cmd, &p)
	for _, name := range []string{"skip", "limit", "access-token", "name-filter", "active"} {
		if f := cmd.Flags().Lookup(name); f == nil {
			t.Errorf("expected flag %q to be registered", name)
		}
	}
}

func TestDefineQueryFlags_accessTokenHidden(t *testing.T) {
	cmd := &cobra.Command{}
	var p sampleParams
	DefineQueryFlags(cmd, &p)
	f := cmd.Flags().Lookup("access-token")
	if f == nil {
		t.Fatal("access-token flag missing")
	}
	if !f.Hidden {
		t.Error("access-token should be Hidden")
	}
}

func TestDefineQueryFlags_setNullableInt(t *testing.T) {
	cmd := &cobra.Command{}
	var p sampleParams
	DefineQueryFlags(cmd, &p)
	if err := cmd.Flags().Parse([]string{"--skip", "42"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.Skip == nil || *p.Skip != 42 {
		t.Errorf("Skip = %v, want pointer to 42", p.Skip)
	}
	if p.Limit != nil {
		t.Errorf("Limit should be nil (unset), got pointer to %d", *p.Limit)
	}
}

func TestDefineQueryFlags_setNullableBool(t *testing.T) {
	cmd := &cobra.Command{}
	var p sampleParams
	DefineQueryFlags(cmd, &p)
	if err := cmd.Flags().Parse([]string{"--active", "true"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.Active == nil || !*p.Active {
		t.Errorf("Active = %v, want pointer to true", p.Active)
	}
}

func TestLoadJSONBody_inline(t *testing.T) {
	var dst struct {
		Name string `json:"name"`
	}
	if err := LoadJSONBody(`{"name":"foo"}`, &dst); err != nil {
		t.Fatalf("LoadJSONBody: %v", err)
	}
	if dst.Name != "foo" {
		t.Errorf("Name = %q, want %q", dst.Name, "foo")
	}
}

func TestLoadJSONBody_file(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "body.json")
	if err := os.WriteFile(path, []byte(`{"name":"bar"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var dst struct {
		Name string `json:"name"`
	}
	if err := LoadJSONBody("@"+path, &dst); err != nil {
		t.Fatalf("LoadJSONBody: %v", err)
	}
	if dst.Name != "bar" {
		t.Errorf("Name = %q, want %q", dst.Name, "bar")
	}
}

func TestLoadJSONBody_emptyIsNoOp(t *testing.T) {
	var dst struct {
		Name string `json:"name"`
	}
	if err := LoadJSONBody("", &dst); err != nil {
		t.Errorf("empty arg should be no-op, got %v", err)
	}
	if dst.Name != "" {
		t.Errorf("Name = %q, want empty", dst.Name)
	}
}

func TestLoadJSONBody_invalidJSON(t *testing.T) {
	var dst struct{}
	if err := LoadJSONBody("{not json", &dst); err == nil {
		t.Error("expected error for invalid JSON")
	}
}
