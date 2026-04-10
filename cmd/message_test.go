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
)

func TestMessageSendRunForDM(t *testing.T) {
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
		if err := (&MessageSendCmd{Recipient: "@alice", Text: "hello"}).Run(ctx); err != nil {
			t.Fatalf("MessageSendCmd.Run returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Sent message to @alice (D123) at 1775772298.509159") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestMessageSendRunForChannel(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/conversations.list":
			return dmJSONResponse(req, `{"ok":true,"channels":[{"id":"C123","name":"general","is_channel":true}]}`)
		case "/api/chat.postMessage":
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
			if values.Get("thread_ts") != "1775772000.000001" {
				t.Fatalf("expected thread_ts, got %q", values.Get("thread_ts"))
			}
			return dmJSONResponse(req, `{"ok":true,"channel":"C123","ts":"1775772298.509159","message":{"text":"hello","ts":"1775772298.509159"}}`)
		default:
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		if err := (&MessageSendCmd{
			Recipient: "#general",
			Text:      "hello",
			Thread:    "1775772000.000001",
		}).Run(ctx); err != nil {
			t.Fatalf("MessageSendCmd.Run returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Sent message to #general (C123) at 1775772298.509159") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestMessageSendMissingScopeHint(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/chat.postMessage":
			return dmJSONResponse(req, `{"ok":false,"error":"missing_scope"}`)
		default:
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
	})

	err := (&MessageSendCmd{Recipient: "D123", Text: "hello"}).Run(ctx)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "rerun 'slack-cli auth login'") {
		t.Fatalf("expected missing scope guidance, got %v", err)
	}
}

func TestMessageSendMessageText(t *testing.T) {
	t.Run("rejects empty text", func(t *testing.T) {
		_, err := (&MessageSendCmd{}).messageText()
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

		text, err := (&MessageSendCmd{Stdin: true}).messageText()
		if err != nil {
			t.Fatalf("messageText returned error: %v", err)
		}
		if text != "hello from stdin" {
			t.Fatalf("messageText = %q, want %q", text, "hello from stdin")
		}
	})
}

func TestMessageCommandParses(t *testing.T) {
	cli := &CLI{}
	parser, err := kong.New(cli, kong.Vars{"version": "test"})
	if err != nil {
		t.Fatalf("failed to build parser: %v", err)
	}

	if _, err := parser.Parse([]string{"message", "send", "#general", "hello"}); err != nil {
		t.Fatalf("expected message send to parse, got %v", err)
	}
}
