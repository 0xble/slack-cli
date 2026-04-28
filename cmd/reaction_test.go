package cmd

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

func TestReactionAddRun(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/reactions.add":
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("failed to read request body: %v", err)
			}
			values, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("failed to parse request body: %v", err)
			}
			if values.Get("channel") != "C123" {
				t.Fatalf("expected channel C123, got %q", values.Get("channel"))
			}
			if values.Get("timestamp") != "1775772298.509159" {
				t.Fatalf("expected timestamp, got %q", values.Get("timestamp"))
			}
			if values.Get("name") != "+1" {
				t.Fatalf("expected name +1, got %q", values.Get("name"))
			}
			return dmJSONResponse(req, `{"ok":true}`)
		default:
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		err := (&ReactionAddCmd{
			URL:   "https://buildkite.slack.com/archives/C123/p1775772298509159",
			Emoji: "thumbsup",
		}).Run(ctx)
		if err != nil {
			t.Fatalf("ReactionAddCmd.Run returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Added :+1: reaction to C123 at 1775772298.509159") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestReactionAddMissingScopeHint(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/reactions.add":
			return dmJSONResponse(req, `{"ok":false,"error":"missing_scope"}`)
		default:
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
	})

	err := (&ReactionAddCmd{
		URL:   "https://buildkite.slack.com/archives/C123/p1775772298509159",
		Emoji: "+1",
	}).Run(ctx)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "reactions:write") {
		t.Fatalf("expected missing scope guidance, got %v", err)
	}
}

func TestNormalizeReactionName(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{raw: ":white_check_mark:", want: "white_check_mark"},
		{raw: "thumbsup", want: "+1"},
		{raw: "thumbs_up", want: "+1"},
		{raw: "👍", want: "+1"},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := normalizeReactionName(tt.raw)
			if err != nil {
				t.Fatalf("normalizeReactionName returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("normalizeReactionName = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReactionCommandParses(t *testing.T) {
	cli := &CLI{}
	parser, err := kong.New(cli, kong.Vars{"version": "test"})
	if err != nil {
		t.Fatalf("failed to build parser: %v", err)
	}

	_, err = parser.Parse([]string{
		"reaction",
		"add",
		"https://buildkite.slack.com/archives/C123/p1775772298509159",
		"thumbsup",
	})
	if err != nil {
		t.Fatalf("expected reaction add to parse, got %v", err)
	}
}
