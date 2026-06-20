package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteJSONPretty(t *testing.T) {
	var buf bytes.Buffer
	if err := writeJSON(&buf, map[string]any{"a": 1, "b": "two"}); err != nil {
		t.Fatalf("writeJSON returned error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "\n  \"a\"") {
		t.Fatalf("expected indented output, got %q", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Fatalf("expected trailing newline, got %q", out)
	}
}

func TestWriteJSONLOneLinePerItem(t *testing.T) {
	var buf bytes.Buffer
	items := []map[string]any{{"n": 1}, {"n": 2}, {"n": 3}}
	if err := writeJSONL(&buf, items); err != nil {
		t.Fatalf("writeJSONL returned error: %v", err)
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), buf.String())
	}
	for i, line := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("line %d not valid JSON: %v", i, err)
		}
	}
}

func TestWriteJSONNoHTMLEscape(t *testing.T) {
	var buf bytes.Buffer
	if err := writeJSON(&buf, map[string]string{"link": "<https://example.com>"}); err != nil {
		t.Fatalf("writeJSON returned error: %v", err)
	}
	if !strings.Contains(buf.String(), "<https://example.com>") {
		t.Fatalf("expected angle brackets preserved, got %q", buf.String())
	}
}
