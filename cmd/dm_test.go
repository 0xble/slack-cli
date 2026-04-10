package cmd

import (
	"fmt"
	"io"
	"net/http"
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
	if _, err := parser.Parse([]string{"dm", "send", "D123", "hello"}); err == nil {
		t.Fatalf("expected dm send parse to fail")
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
