package slack

import (
	"strings"
	"testing"
)

func TestCanvasHTMLToText(t *testing.T) {
	got := CanvasHTMLToText(
		`<h1>Title</h1><p>Hello <a>@U123</a></p><ul><li>One</li></ul><p><a href="https://example.com">Link</a></p><control><img alt="wave"></img></control>`,
		map[string]string{"U123": "alice"},
	)

	checks := []string{
		"# Title",
		"Hello @alice",
		"- One",
		"Link (https://example.com)",
		":wave:",
	}

	for _, check := range checks {
		if !strings.Contains(got, check) {
			t.Fatalf("CanvasHTMLToText() missing %q in %q", check, got)
		}
	}
}

func TestIsCanvasFile(t *testing.T) {
	tests := []struct {
		name string
		file File
		want bool
	}{
		{name: "quip filetype", file: File{Filetype: "quip"}, want: true},
		{name: "slack docs mimetype", file: File{Mimetype: "application/vnd.slack-docs"}, want: true},
		{name: "other file", file: File{Filetype: "pdf", Mimetype: "application/pdf"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCanvasFile(tt.file); got != tt.want {
				t.Fatalf("IsCanvasFile(%+v) = %v, want %v", tt.file, got, tt.want)
			}
		})
	}
}
