// Package bindings wires generated platform-cmd subcommands to cobra flags
// and JSON request bodies. Two entry points:
//
//   - DefineQueryFlags walks an oapi-codegen `*<Op>Params` struct and
//     registers one cobra flag per field, supporting the pointer-of-scalar
//     shape that oapi-codegen always emits for optional query parameters.
//   - LoadJSONBody parses a `--json-body` argument (either inline JSON or
//     `@path/to/file.json`) into the operation's request-body struct.
package bindings

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// DefineQueryFlags registers one flag per exported field of *params. Fields
// must be pointer-to-scalar (or `[]string`, or `*[]string`) — anything else
// is skipped silently to keep the generated --help readable. Flag names are
// kebab-case of the JSON tag; descriptions come from the field's Go doc
// comment if present.
//
// Hidden flag policy: query params that exist only to satisfy the OpenAPI
// security scheme (`access_token`) are auto-hidden so operators don't think
// they're meant to use them — auth flows through the bearer header.
func DefineQueryFlags(cmd *cobra.Command, params any) {
	rv := reflect.ValueOf(params)
	if rv.Kind() != reflect.Pointer || rv.Elem().Kind() != reflect.Struct {
		return
	}
	rv = rv.Elem()
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if !field.IsExported() {
			continue
		}
		name := flagName(field)
		if name == "" {
			continue
		}
		usage := strings.TrimSpace(field.Tag.Get("description"))
		fieldVal := rv.Field(i).Addr()
		f := registerScalarFlag(cmd.Flags(), name, usage, fieldVal.Interface())
		if f != nil && name == "access-token" {
			f.Hidden = true
		}
	}
}

// LoadJSONBody parses `arg` into the value pointed to by dst.
//   - "@path/to/file.json" reads the file.
//   - Anything else is treated as a literal JSON string.
//   - Empty arg leaves dst at its zero value (useful for body-less POSTs
//     where the server still tolerates an empty object).
func LoadJSONBody(arg string, dst any) error {
	if arg == "" {
		return nil
	}
	var raw []byte
	if strings.HasPrefix(arg, "@") {
		path := strings.TrimPrefix(arg, "@")
		b, err := os.ReadFile(path) // #nosec G304 — operator-supplied path is the point
		if err != nil {
			return fmt.Errorf("--json-body: read %s: %w", path, err)
		}
		raw = b
	} else {
		raw = []byte(arg)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("--json-body: invalid JSON: %w", err)
	}
	return nil
}

func flagName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "" {
		tag = field.Tag.Get("form")
	}
	if tag == "" {
		return strings.ToLower(field.Name)
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "-" || name == "" {
		return ""
	}
	return strings.ReplaceAll(name, "_", "-")
}

// registerScalarFlag handles oapi-codegen's pointer-of-scalar idiom for
// optional fields. Returns the registered *pflag.Flag (for caller-side
// tweaks like Hidden), or nil if the field type isn't supported.
func registerScalarFlag(fs *pflag.FlagSet, name, usage string, ptr any) *pflag.Flag {
	switch p := ptr.(type) {
	case **string:
		fs.Var(&nullableString{p: p}, name, usage)
	case **int:
		fs.Var(&nullableInt{p: p}, name, usage)
	case **int32:
		fs.Var(&nullableInt32{p: p}, name, usage)
	case **int64:
		fs.Var(&nullableInt64{p: p}, name, usage)
	case **bool:
		fs.Var(&nullableBool{p: p}, name, usage)
	case **float32:
		fs.Var(&nullableFloat32{p: p}, name, usage)
	case **float64:
		fs.Var(&nullableFloat64{p: p}, name, usage)
	case *string:
		fs.StringVar(p, name, "", usage)
	case *int:
		fs.IntVar(p, name, 0, usage)
	case *int32:
		fs.Int32Var(p, name, 0, usage)
	case *int64:
		fs.Int64Var(p, name, 0, usage)
	case *bool:
		fs.BoolVar(p, name, false, usage)
	case *[]string:
		fs.StringSliceVar(p, name, nil, usage)
	default:
		return nil
	}
	return fs.Lookup(name)
}

// Nullable adapters: cobra/pflag has no built-in optional-pointer flags.
// Each Set call allocates the underlying value so a missing flag stays nil
// (and the server preserves the schema-defined default).

type nullableString struct{ p **string }

func (n *nullableString) String() string {
	if n.p == nil || *n.p == nil {
		return ""
	}
	return **n.p
}
func (n *nullableString) Set(v string) error { *n.p = &v; return nil }
func (n *nullableString) Type() string       { return "string" }

type nullableInt struct{ p **int }

func (n *nullableInt) String() string {
	if n.p == nil || *n.p == nil {
		return ""
	}
	return strconv.Itoa(**n.p)
}
func (n *nullableInt) Set(v string) error {
	x, err := strconv.Atoi(v)
	if err != nil {
		return err
	}
	*n.p = &x
	return nil
}
func (n *nullableInt) Type() string { return "int" }

type nullableInt32 struct{ p **int32 }

func (n *nullableInt32) String() string {
	if n.p == nil || *n.p == nil {
		return ""
	}
	return strconv.FormatInt(int64(**n.p), 10)
}
func (n *nullableInt32) Set(v string) error {
	x, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		return err
	}
	x32 := int32(x)
	*n.p = &x32
	return nil
}
func (n *nullableInt32) Type() string { return "int32" }

type nullableInt64 struct{ p **int64 }

func (n *nullableInt64) String() string {
	if n.p == nil || *n.p == nil {
		return ""
	}
	return strconv.FormatInt(**n.p, 10)
}
func (n *nullableInt64) Set(v string) error {
	x, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return err
	}
	*n.p = &x
	return nil
}
func (n *nullableInt64) Type() string { return "int64" }

type nullableBool struct{ p **bool }

func (n *nullableBool) String() string {
	if n.p == nil || *n.p == nil {
		return ""
	}
	return strconv.FormatBool(**n.p)
}
func (n *nullableBool) Set(v string) error {
	x, err := strconv.ParseBool(v)
	if err != nil {
		return err
	}
	*n.p = &x
	return nil
}
func (n *nullableBool) Type() string { return "bool" }

type nullableFloat32 struct{ p **float32 }

func (n *nullableFloat32) String() string {
	if n.p == nil || *n.p == nil {
		return ""
	}
	return strconv.FormatFloat(float64(**n.p), 'g', -1, 32)
}
func (n *nullableFloat32) Set(v string) error {
	x, err := strconv.ParseFloat(v, 32)
	if err != nil {
		return err
	}
	x32 := float32(x)
	*n.p = &x32
	return nil
}
func (n *nullableFloat32) Type() string { return "float32" }

type nullableFloat64 struct{ p **float64 }

func (n *nullableFloat64) String() string {
	if n.p == nil || *n.p == nil {
		return ""
	}
	return strconv.FormatFloat(**n.p, 'g', -1, 64)
}
func (n *nullableFloat64) Set(v string) error {
	x, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return err
	}
	*n.p = &x
	return nil
}
func (n *nullableFloat64) Type() string { return "float64" }
