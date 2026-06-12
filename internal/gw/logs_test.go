package gw

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPrintTailWholeFileWhenSmallerThanMax verifies that when the file is
// smaller than maxBytes the full content (including the first line) is written.
func TestPrintTailWholeFileWhenSmallerThanMax(t *testing.T) {
	content := "line one\nline two\nline three\n"
	f := writeTempFile(t, content)

	var buf bytes.Buffer
	if err := printTail(f, &buf, 1024*1024); err != nil {
		t.Fatalf("printTail returned error: %v", err)
	}

	if got := buf.String(); got != content {
		t.Errorf("expected full file content\ngot:  %q\nwant: %q", got, content)
	}
}

// TestPrintTailDropsPartialFirstLine verifies that when the window starts
// mid-file the truncated partial first line is dropped and the output begins
// on a clean line boundary.
func TestPrintTailDropsPartialFirstLine(t *testing.T) {
	// Build a file that is definitely larger than our small maxBytes so the
	// window lands inside the first line.
	lines := []string{
		"alpha alpha alpha alpha alpha\n", // 30 bytes — the partial line we expect to be dropped
		"beta beta beta beta beta\n",      // 25 bytes
		"gamma gamma gamma\n",             // 18 bytes
	}
	content := strings.Join(lines, "")
	f := writeTempFile(t, content)

	// maxBytes = 50 → start = len(content)-50 = 23, which lands inside the
	// "alpha..." line (bytes 0-29). After the partial-line drop the output
	// should begin with "beta...".
	maxBytes := int64(50)
	if int64(len(content)) <= maxBytes {
		t.Fatalf("test setup error: content (%d bytes) must exceed maxBytes (%d)", len(content), maxBytes)
	}

	var buf bytes.Buffer
	if err := printTail(f, &buf, maxBytes); err != nil {
		t.Fatalf("printTail returned error: %v", err)
	}

	got := buf.String()

	// Must not contain the partial alpha line fragment.
	if strings.Contains(got, "alpha") {
		t.Errorf("output should not contain the dropped partial line; got: %q", got)
	}
	// Must contain the last full line.
	if !strings.Contains(got, "gamma gamma gamma\n") {
		t.Errorf("output should contain last line; got: %q", got)
	}
	// Must start at a line boundary — i.e. the first character is the start of
	// a complete line, not a mid-line fragment.
	if len(got) > 0 && !strings.HasPrefix(got, "beta") {
		t.Errorf("output should start with the first complete line (beta...); got: %q", got)
	}
}

// TestPrintTailEmptyFile verifies that an empty file produces no output and
// no error.
func TestPrintTailEmptyFile(t *testing.T) {
	f := writeTempFile(t, "")

	var buf bytes.Buffer
	if err := printTail(f, &buf, 64*1024); err != nil {
		t.Fatalf("printTail returned error on empty file: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected empty output for empty file; got %q", buf.String())
	}
}

// TestPrintTailLeavesOffsetAtEOF verifies that after printTail the file
// descriptor is positioned at EOF so a subsequent read returns io.EOF — i.e.
// a follow loop won't re-print the history.
func TestPrintTailLeavesOffsetAtEOF(t *testing.T) {
	content := "line one\nline two\nline three\n"
	f := writeTempFile(t, content)

	var buf bytes.Buffer
	if err := printTail(f, &buf, 64*1024); err != nil {
		t.Fatalf("printTail returned error: %v", err)
	}

	// A direct read from f should immediately return io.EOF.
	tmp := make([]byte, 1)
	_, err := f.Read(tmp)
	if err != io.EOF {
		t.Errorf("expected io.EOF after printTail; got err=%v, read=%v", err, tmp)
	}

	// Also confirm via Seek that the offset equals the file size.
	offset, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		t.Fatalf("seek current: %v", err)
	}
	if offset != int64(len(content)) {
		t.Errorf("expected offset %d (EOF); got %d", len(content), offset)
	}
}

// writeTempFile creates a temp file in t.TempDir() with the given content and
// returns it open for reading (positioned at the start).
func writeTempFile(t *testing.T, content string) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.log")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open temp file: %v", err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}
