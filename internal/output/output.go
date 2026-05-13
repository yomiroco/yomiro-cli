// Package output renders platform API responses for the CLI in the format
// selected by the persistent --output flag (json, yaml). Falls back to raw
// bytes when no typed payload matches the status code (e.g. 204).
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// formatFromCmd resolves --output by walking up the command tree (the flag is
// declared as persistent on root). Defaults to "json" when unset or unparseable.
func formatFromCmd(cmd *cobra.Command) string {
	if cmd == nil {
		return "json"
	}
	if f := cmd.Flag("output"); f != nil {
		if v := f.Value.String(); v != "" {
			return v
		}
	}
	return "json"
}

// RenderResponse extracts the typed payload from an oapi-codegen response
// struct (looks for JSON<status> via reflection) and writes it in the
// selected format. Non-2xx statuses return an error with the body content,
// so cobra surfaces them with exit code 1.
func RenderResponse(cmd *cobra.Command, resp any) error {
	rv := reflect.ValueOf(resp)
	if rv.Kind() == reflect.Pointer {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("output: response is not a struct (got %s)", rv.Kind())
	}

	httpField := rv.FieldByName("HTTPResponse")
	if !httpField.IsValid() {
		return fmt.Errorf("output: response struct missing HTTPResponse field")
	}
	httpResp, _ := httpField.Interface().(*http.Response)
	if httpResp == nil {
		return fmt.Errorf("output: HTTPResponse is nil")
	}
	status := httpResp.StatusCode

	w := cmd.OutOrStdout()
	format := formatFromCmd(cmd)

	jsonField := rv.FieldByName(fmt.Sprintf("JSON%d", status))
	if jsonField.IsValid() && !jsonField.IsNil() {
		payload := jsonField.Elem().Interface()
		if status >= 400 {
			return renderError(cmd.ErrOrStderr(), format, status, payload)
		}
		return Render(w, format, payload)
	}

	body := rv.FieldByName("Body")
	var raw []byte
	if body.IsValid() && body.Kind() == reflect.Slice && body.Type().Elem().Kind() == reflect.Uint8 {
		raw = body.Bytes()
	}

	if status >= 400 {
		var parsed any
		if len(raw) > 0 && json.Unmarshal(raw, &parsed) == nil {
			return renderError(cmd.ErrOrStderr(), format, status, parsed)
		}
		return fmt.Errorf("%s: %s", httpResp.Status, string(raw))
	}

	if len(raw) == 0 {
		return nil
	}
	var parsed any
	if json.Unmarshal(raw, &parsed) == nil {
		return Render(w, format, parsed)
	}
	_, err := w.Write(raw)
	return err
}

// Render writes v in the requested format. Unknown formats fall back to JSON
// with a one-line stderr warning so an operator typo doesn't kill the call.
//
// YAML output round-trips through JSON so the json tags on oapi-codegen
// structs propagate as YAML keys — without this the encoder uses Go field
// names (`AnnotationSyncVersion` → `annotationsyncversion`) which is
// useless for operators piping yaml back into other tools.
func Render(w io.Writer, format string, v any) error {
	switch format {
	case "yaml", "yml":
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		var generic any
		if err := json.Unmarshal(raw, &generic); err != nil {
			return err
		}
		enc := yaml.NewEncoder(w)
		enc.SetIndent(2)
		defer enc.Close()
		return enc.Encode(generic)
	case "table":
		// Table rendering is a Stage 3 ergonomics task. Fall back to JSON
		// today so `--output table` never errors out — operators see the
		// data, just not pretty-printed yet.
		fallthrough
	case "", "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	default:
		fmt.Fprintf(w, "warning: unknown --output %q, defaulting to json\n", format)
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}
}

// renderError prints the failure payload to stderr in the selected format and
// returns an error that cobra will surface (with the chosen exit code).
func renderError(stderr io.Writer, format string, status int, payload any) error {
	_ = Render(stderr, format, payload)
	return fmt.Errorf("request failed with HTTP %d", status)
}
