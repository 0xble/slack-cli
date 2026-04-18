package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/lox/slack-cli/internal/output"
)

func TestDmReadCmdJSON(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/conversations.history":
			return dmJSONResponse(req, `{"ok":true,"messages":[{"user":"U123","text":"latest","ts":"200.000001"},{"user":"U124","text":"earlier","ts":"100.000001"}]}`)
		case "/api/users.info":
			switch req.URL.Query().Get("user") {
			case "U123":
				return dmJSONResponse(req, `{"ok":true,"user":{"id":"U123","name":"alice","real_name":"Alice"}}`)
			case "U124":
				return dmJSONResponse(req, `{"ok":true,"user":{"id":"U124","name":"bob","real_name":"Bob"}}`)
			}
		}
		return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
	})

	out := captureStdout(t, func() {
		if err := (&DmReadCmd{Recipient: "D123", Limit: 20, JSON: true}).Run(ctx); err != nil {
			t.Fatalf("DmReadCmd.Run returned error: %v", err)
		}
	})

	var got []output.Message
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(got))
	}
	if got[0].TS != "100.000001" || got[0].User != "Bob" {
		t.Fatalf("expected oldest first, got: %+v", got[0])
	}
	if got[0].Channel == nil || got[0].Channel.ID != "D123" || got[0].Channel.Type != "im" {
		t.Fatalf("expected DM channel ref, got: %+v", got[0].Channel)
	}
}

func TestChannelReadCmdJSONL(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/conversations.history":
			return dmJSONResponse(req, `{"ok":true,"messages":[{"user":"U1","text":"b","ts":"200.000000"},{"user":"U1","text":"a","ts":"100.000000"}]}`)
		case "/api/users.info":
			return dmJSONResponse(req, `{"ok":true,"user":{"id":"U1","name":"alice","real_name":"Alice"}}`)
		case "/api/conversations.info":
			return dmJSONResponse(req, `{"ok":true,"channel":{"id":"C1","name":"general"}}`)
		}
		return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
	})

	out := captureStdout(t, func() {
		if err := (&ChannelReadCmd{Channel: "C1", Limit: 20, JSONL: true}).Run(ctx); err != nil {
			t.Fatalf("ChannelReadCmd.Run returned error: %v", err)
		}
	})

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 JSONL lines, got %d: %q", len(lines), out)
	}
	var first output.Message
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("first line not JSON: %v", err)
	}
	if first.TS != "100.000000" {
		t.Fatalf("expected oldest first in JSONL, got %q", first.TS)
	}
	if first.Channel == nil || first.Channel.Type != "channel" {
		t.Fatalf("expected channel type, got %+v", first.Channel)
	}
}

func TestThreadReadCmdJSON(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/conversations.replies":
			return dmJSONResponse(req, `{"ok":true,"messages":[{"user":"U1","text":"parent","ts":"100.000000","thread_ts":"100.000000","reply_count":1},{"user":"U2","text":"reply","ts":"150.000000","thread_ts":"100.000000"}]}`)
		case "/api/users.info":
			switch req.URL.Query().Get("user") {
			case "U1":
				return dmJSONResponse(req, `{"ok":true,"user":{"id":"U1","name":"alice","real_name":"Alice"}}`)
			case "U2":
				return dmJSONResponse(req, `{"ok":true,"user":{"id":"U2","name":"bob","real_name":"Bob"}}`)
			}
		}
		return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
	})

	out := captureStdout(t, func() {
		cmd := &ThreadReadCmd{URL: "https://example.slack.com/archives/C1/p100000000?thread_ts=100.000000", Limit: 100, JSON: true}
		if err := cmd.Run(ctx); err != nil {
			t.Fatalf("ThreadReadCmd.Run returned error: %v", err)
		}
	})

	var got []output.Message
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(got))
	}
	if got[0].TS != "100.000000" || got[1].TS != "150.000000" {
		t.Fatalf("expected parent-first order, got: %+v", got)
	}
	if got[0].Workspace != "example.slack.com" {
		t.Fatalf("expected workspace from URL, got %q", got[0].Workspace)
	}
	if got[0].ReplyCount != 1 {
		t.Fatalf("expected reply_count=1 on parent, got %d", got[0].ReplyCount)
	}
}

func TestChannelListCmdJSON(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/api/conversations.list" {
			return dmJSONResponse(req, `{"ok":true,"channels":[
				{"id":"C1","name":"general","num_members":42,"purpose":{"value":"talk"}},
				{"id":"G2","name":"private-ops","is_private":true,"num_members":3}
			]}`)
		}
		return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
	})

	out := captureStdout(t, func() {
		if err := (&ChannelListCmd{Limit: 100, JSON: true}).Run(ctx); err != nil {
			t.Fatalf("ChannelListCmd.Run returned error: %v", err)
		}
	})

	var got []output.Channel
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(got))
	}
	if got[0].Type != "channel" || got[1].Type != "private_channel" {
		t.Fatalf("unexpected types: %+v", got)
	}
	if got[0].Purpose != "talk" || got[0].NumMembers != 42 {
		t.Fatalf("unexpected first channel: %+v", got[0])
	}
}
