package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/lox/slack-cli/internal/output"
)

const searchMatchResponse = `{
	"ok": true,
	"messages": {
		"total": 2,
		"matches": [
			{
				"type": "message",
				"user": "U1",
				"username": "alice",
				"text": "hello <@U2>",
				"ts": "100.000001",
				"channel": {"id": "C1", "name": "general"},
				"permalink": "https://example.slack.com/archives/C1/p100000001"
			},
			{
				"type": "im",
				"user": "U2",
				"username": "bob",
				"text": "just rollback",
				"ts": "200.000002",
				"channel": {"id": "D1", "name": "U1"},
				"permalink": "https://example.slack.com/archives/D1/p200000002"
			}
		]
	}
}`

func TestSearchCmdJSON(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/search.messages":
			return dmJSONResponse(req, searchMatchResponse)
		case "/api/users.info":
			return dmJSONResponse(req, `{"ok":true,"user":{"id":"U2","name":"bob","real_name":"Bob"}}`)
		default:
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
	})

	out := captureStdout(t, func() {
		if err := (&SearchCmd{Query: "q", Limit: 20, JSON: true}).Run(ctx); err != nil {
			t.Fatalf("SearchCmd.Run returned error: %v", err)
		}
	})

	var got []output.Message
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %d: %s", len(got), out)
	}

	first := got[0]
	if first.TS != "100.000001" || first.User != "alice" || first.Channel.ID != "C1" {
		t.Fatalf("unexpected first match: %+v", first)
	}
	if first.Channel.Type != "channel" {
		t.Fatalf("expected channel type 'channel', got %q", first.Channel.Type)
	}
	if first.Workspace != "example.slack.com" {
		t.Fatalf("expected workspace from permalink, got %q", first.Workspace)
	}
	if !strings.Contains(first.Text, "@Bob") {
		t.Fatalf("expected resolved mention in text, got %q", first.Text)
	}
	if !strings.Contains(first.TextRaw, "<@U2>") {
		t.Fatalf("expected raw mention preserved in text_raw, got %q", first.TextRaw)
	}

	second := got[1]
	if second.Channel.Type != "im" {
		t.Fatalf("expected DM match channel type 'im', got %q", second.Channel.Type)
	}
}

func TestSearchCmdJSONL(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/api/search.messages" {
			return dmJSONResponse(req, searchMatchResponse)
		}
		return dmJSONResponse(req, `{"ok":true,"user":{"id":"U2","name":"bob"}}`)
	})

	out := captureStdout(t, func() {
		if err := (&SearchCmd{Query: "q", Limit: 20, JSONL: true}).Run(ctx); err != nil {
			t.Fatalf("SearchCmd.Run returned error: %v", err)
		}
	})

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 JSONL lines, got %d: %q", len(lines), out)
	}
	for i, line := range lines {
		var m output.Message
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("line %d not valid JSON: %v\n%s", i, err, line)
		}
	}
}

func TestSearchCmdEmpty(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		return dmJSONResponse(req, `{"ok":true,"messages":{"total":0,"matches":[]}}`)
	})

	out := captureStdout(t, func() {
		if err := (&SearchCmd{Query: "q", Limit: 20, JSON: true}).Run(ctx); err != nil {
			t.Fatalf("SearchCmd.Run returned error: %v", err)
		}
	})

	if strings.TrimSpace(out) != "[]" {
		t.Fatalf("expected empty array, got %q", out)
	}
}
