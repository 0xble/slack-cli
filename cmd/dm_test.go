package cmd

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/lox/slack-cli/internal/config"
	"github.com/lox/slack-cli/internal/slack"
)

func TestDmListRun(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/conversations.list":
			return dmJSONResponse(req, `{"ok":true,"channels":[{"id":"D123","is_im":true,"user":"U123"}]}`)
		case "/api/users.info":
			return dmJSONResponse(req, `{"ok":true,"user":{"id":"U123","name":"alice","real_name":"Alice"}}`)
		default:
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		if err := (&DmListCmd{Limit: 20}).Run(ctx); err != nil {
			t.Fatalf("DmListCmd.Run returned error: %v", err)
		}
	})

	if !strings.Contains(output, "@alice - Alice (D123)") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestDmReadRun(t *testing.T) {
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
			default:
				return nil, fmt.Errorf("unexpected user lookup %q", req.URL.RawQuery)
			}
		default:
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		if err := (&DmReadCmd{Recipient: "D123", Limit: 20}).Run(ctx); err != nil {
			t.Fatalf("DmReadCmd.Run returned error: %v", err)
		}
	})

	if !strings.Contains(output, "[100.000001] Bob: earlier") {
		t.Fatalf("expected oldest message first, got %q", output)
	}
	if !strings.Contains(output, "[200.000001] Alice: latest") {
		t.Fatalf("expected latest message in output, got %q", output)
	}
}

func TestDmSendRun(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/users.list":
			return dmJSONResponse(req, `{"ok":true,"members":[{"id":"U123","name":"alice","real_name":"Alice"}]}`)
		case "/api/conversations.open":
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("failed to read request body: %v", err)
			}
			values, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("failed to parse request body: %v", err)
			}
			if values.Get("users") != "U123" {
				t.Fatalf("expected users=U123, got %q", values.Get("users"))
			}
			return dmJSONResponse(req, `{"ok":true,"channel":{"id":"D123","user":"U123","is_im":true}}`)
		case "/api/chat.postMessage":
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("failed to read request body: %v", err)
			}
			values, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("failed to parse request body: %v", err)
			}
			if values.Get("channel") != "D123" {
				t.Fatalf("expected channel D123, got %q", values.Get("channel"))
			}
			if values.Get("text") != "hello" {
				t.Fatalf("expected text hello, got %q", values.Get("text"))
			}
			return dmJSONResponse(req, `{"ok":true,"channel":"D123","ts":"1775772298.509159","message":{"text":"hello","ts":"1775772298.509159"}}`)
		default:
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		if err := (&DmSendCmd{Recipient: "@alice", Text: "hello"}).Run(ctx); err != nil {
			t.Fatalf("DmSendCmd.Run returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Sent DM to @alice (D123) at 1775772298.509159") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestDmSendMissingScopeHint(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/chat.postMessage":
			return dmJSONResponse(req, `{"ok":false,"error":"missing_scope"}`)
		default:
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
	})

	err := (&DmSendCmd{Recipient: "D123", Text: "hello"}).Run(ctx)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "chat:write and im:write") {
		t.Fatalf("expected missing scope guidance, got %v", err)
	}
}

func TestDmSendMessageText(t *testing.T) {
	t.Run("rejects empty text", func(t *testing.T) {
		_, err := (&DmSendCmd{}).messageText()
		if err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("reads stdin", func(t *testing.T) {
		originalStdin := os.Stdin
		reader, writer, err := os.Pipe()
		if err != nil {
			t.Fatalf("os.Pipe returned error: %v", err)
		}
		os.Stdin = reader
		defer func() {
			os.Stdin = originalStdin
		}()

		if _, err := writer.WriteString("hello from stdin"); err != nil {
			t.Fatalf("writer.WriteString returned error: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("writer.Close returned error: %v", err)
		}

		text, err := (&DmSendCmd{Stdin: true}).messageText()
		if err != nil {
			t.Fatalf("messageText returned error: %v", err)
		}
		if text != "hello from stdin" {
			t.Fatalf("messageText = %q, want %q", text, "hello from stdin")
		}
	})
}

func TestDMCommandAliasesParse(t *testing.T) {
	cli := &CLI{}
	parser, err := kong.New(cli, kong.Vars{"version": "test"})
	if err != nil {
		t.Fatalf("failed to build parser: %v", err)
	}

	if _, err := parser.Parse([]string{"dm", "ls"}); err != nil {
		t.Fatalf("expected dm ls to parse, got %v", err)
	}
	if _, err := parser.Parse([]string{"dm", "history", "D123"}); err != nil {
		t.Fatalf("expected dm history to parse, got %v", err)
	}
	if _, err := parser.Parse([]string{"dm", "h", "D123"}); err != nil {
		t.Fatalf("expected dm h to parse, got %v", err)
	}
}

func testDMContext(fn func(req *http.Request) (*http.Response, error)) *Context {
	return &Context{
		Config: &config.Config{
			CurrentWorkspace: "default",
			Workspaces: map[string]config.WorkspaceAuth{
				"default": {Token: "xoxp-test-token"},
			},
		},
		ClientFactory: func(token string) *slack.Client {
			return slack.NewClientWithHTTPClient(
				token,
				&http.Client{
					Transport: dmRoundTripFunc(fn),
				},
			)
		},
	}
}

func dmJSONResponse(req *http.Request, body string) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

type dmRoundTripFunc func(req *http.Request) (*http.Response, error)

func (f dmRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
