package output

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// fakeResp mirrors the oapi-codegen response struct shape: Body, HTTPResponse,
// and one JSON<status> field per documented response.
type fakeResp struct {
	Body         []byte
	HTTPResponse *http.Response
	JSON200      *map[string]any
	JSON404      *map[string]any
}

func newCmd(format string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	cmd := &cobra.Command{}
	cmd.PersistentFlags().String("output", format, "")
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	return cmd, &out, &errBuf
}

func TestRenderResponse_json200(t *testing.T) {
	cmd, out, _ := newCmd("json")
	payload := map[string]any{"id": "abc", "name": "x"}
	resp := &fakeResp{
		HTTPResponse: &http.Response{StatusCode: 200, Status: "200 OK"},
		JSON200:      &payload,
	}
	if err := RenderResponse(cmd, resp); err != nil {
		t.Fatalf("RenderResponse: %v", err)
	}
	if !strings.Contains(out.String(), `"id": "abc"`) {
		t.Errorf("expected json output, got %s", out.String())
	}
}

func TestRenderResponse_yaml200_preservesKeys(t *testing.T) {
	cmd, out, _ := newCmd("yaml")
	payload := map[string]any{"name_filter": "x", "skip_count": 5}
	resp := &fakeResp{
		HTTPResponse: &http.Response{StatusCode: 200, Status: "200 OK"},
		JSON200:      &payload,
	}
	if err := RenderResponse(cmd, resp); err != nil {
		t.Fatalf("RenderResponse: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "name_filter:") {
		t.Errorf("yaml should preserve snake_case keys, got %s", s)
	}
}

func TestRenderResponse_404_writesToStderrAndErrors(t *testing.T) {
	cmd, out, errBuf := newCmd("json")
	payload := map[string]any{"detail": "not found"}
	resp := &fakeResp{
		HTTPResponse: &http.Response{StatusCode: 404, Status: "404 Not Found"},
		JSON404:      &payload,
	}
	err := RenderResponse(cmd, resp)
	if err == nil {
		t.Fatal("RenderResponse should return error on non-2xx")
	}
	if out.Len() != 0 {
		t.Errorf("error responses should not write to stdout, got %s", out.String())
	}
	if !strings.Contains(errBuf.String(), "not found") {
		t.Errorf("stderr should carry error payload, got %s", errBuf.String())
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention status, got %q", err.Error())
	}
}

func TestRenderResponse_emptyBody204(t *testing.T) {
	cmd, out, _ := newCmd("json")
	resp := &fakeResp{
		HTTPResponse: &http.Response{StatusCode: 204, Status: "204 No Content"},
		Body:         nil,
	}
	if err := RenderResponse(cmd, resp); err != nil {
		t.Fatalf("RenderResponse: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("204 should produce no output, got %s", out.String())
	}
}

func TestRenderResponse_fallsBackToRawBody(t *testing.T) {
	cmd, out, _ := newCmd("json")
	resp := &fakeResp{
		HTTPResponse: &http.Response{StatusCode: 200, Status: "200 OK"},
		Body:         []byte(`{"k":"v"}`),
		// no JSON200 set — renderer should parse Body
	}
	if err := RenderResponse(cmd, resp); err != nil {
		t.Fatalf("RenderResponse: %v", err)
	}
	if !strings.Contains(out.String(), `"k": "v"`) {
		t.Errorf("expected fallback rendering, got %s", out.String())
	}
}

func TestRender_unknownFormatFallsBackToJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, "xml", map[string]any{"a": 1}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(buf.String(), `"a": 1`) {
		t.Errorf("unknown format should fall back to json, got %s", buf.String())
	}
}
