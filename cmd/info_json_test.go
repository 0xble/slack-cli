package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/lox/slack-cli/internal/output"
)

func TestChannelInfoCmdJSON(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/api/conversations.info" {
			return dmJSONResponse(req, `{"ok":true,"channel":{"id":"C1","name":"general","num_members":7,"topic":{"value":"chatting"},"purpose":{"value":"all-hands"}}}`)
		}
		return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
	})

	out := captureStdout(t, func() {
		if err := (&ChannelInfoCmd{Channel: "C1", JSON: true}).Run(ctx); err != nil {
			t.Fatalf("ChannelInfoCmd.Run returned error: %v", err)
		}
	})

	var got output.Channel
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if got.ID != "C1" || got.Name != "general" || got.NumMembers != 7 {
		t.Fatalf("unexpected channel: %+v", got)
	}
	if got.Topic != "chatting" || got.Purpose != "all-hands" {
		t.Fatalf("expected topic/purpose populated: %+v", got)
	}
	if got.Type != "channel" {
		t.Fatalf("expected type 'channel', got %q", got.Type)
	}
}

func TestUserInfoCmdJSON(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/api/users.info" {
			return dmJSONResponse(req, `{"ok":true,"user":{"id":"U1","name":"alice","real_name":"Alice","tz":"UTC","profile":{"display_name":"al","email":"a@ex.com","title":"eng"}}}`)
		}
		return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
	})

	out := captureStdout(t, func() {
		if err := (&UserInfoCmd{User: "U1", JSON: true}).Run(ctx); err != nil {
			t.Fatalf("UserInfoCmd.Run returned error: %v", err)
		}
	})

	var got output.User
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if got.DisplayName != "al" || got.Email != "a@ex.com" || got.Title != "eng" {
		t.Fatalf("expected profile fields populated: %+v", got)
	}
}

func TestUserListCmdJSONL(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/api/users.list" {
			return dmJSONResponse(req, `{"ok":true,"members":[
				{"id":"U1","name":"alice","real_name":"Alice","profile":{}},
				{"id":"UBOT","name":"bot","is_bot":true,"profile":{}},
				{"id":"UDEL","name":"del","deleted":true,"profile":{}}
			]}`)
		}
		return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
	})

	out := captureStdout(t, func() {
		if err := (&UserListCmd{Limit: 100, JSONL: true}).Run(ctx); err != nil {
			t.Fatalf("UserListCmd.Run returned error: %v", err)
		}
	})

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected bots/deleted filtered, got %d lines: %q", len(lines), out)
	}
}

func TestFileInfoCmdJSON(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/api/files.info" {
			return dmJSONResponse(req, `{"ok":true,"file":{"id":"F1","name":"a.png","title":"Hi","mimetype":"image/png","size":123,"permalink":"https://x.slack.com/files/U1/F1/a.png","created":1700000000}}`)
		}
		return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
	})

	out := captureStdout(t, func() {
		if err := (&FileInfoCmd{FileID: "F1", JSON: true}).Run(ctx); err != nil {
			t.Fatalf("FileInfoCmd.Run returned error: %v", err)
		}
	})

	var got output.File
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if got.Name != "a.png" || got.Size != 123 || got.Permalink == "" {
		t.Fatalf("unexpected file: %+v", got)
	}
	if got.Created == "" {
		t.Fatalf("expected created timestamp, got empty")
	}
}

func TestDmListCmdJSON(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/conversations.list":
			return dmJSONResponse(req, `{"ok":true,"channels":[{"id":"D1","is_im":true,"user":"U1"}]}`)
		case "/api/users.info":
			return dmJSONResponse(req, `{"ok":true,"user":{"id":"U1","name":"alice","real_name":"Alice"}}`)
		}
		return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
	})

	out := captureStdout(t, func() {
		if err := (&DmListCmd{Limit: 20, JSON: true}).Run(ctx); err != nil {
			t.Fatalf("DmListCmd.Run returned error: %v", err)
		}
	})

	var got []output.Channel
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if len(got) != 1 || got[0].Type != "im" || got[0].User != "alice" || got[0].UserID != "U1" {
		t.Fatalf("unexpected dm list: %+v", got)
	}
}

func TestFileListCmdJSON(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/api/files.list" {
			return dmJSONResponse(req, `{"ok":true,"files":[{"id":"F1","name":"a.png","mimetype":"image/png","size":42}]}`)
		}
		return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
	})

	out := captureStdout(t, func() {
		if err := (&FileListCmd{Limit: 20, JSON: true}).Run(ctx); err != nil {
			t.Fatalf("FileListCmd.Run returned error: %v", err)
		}
	})

	var got []output.File
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if len(got) != 1 || got[0].Size != 42 {
		t.Fatalf("unexpected file list: %+v", got)
	}
}
