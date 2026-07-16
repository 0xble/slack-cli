package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/lox/slack-cli/internal/slack"
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

func TestMessageSendRichCreatesNativeListAndResolvesUserGroup(t *testing.T) {
	historyCalls := 0
	userGroupCalls := 0
	var postedBlocks json.RawMessage
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/usergroups.list":
			userGroupCalls++
			return dmJSONResponse(req, `{"ok":true,"usergroups":[{"id":"S123","handle":"team","name":"Team"}]}`)
		case "/api/chat.postMessage":
			if req.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("expected JSON request, got %q", req.Header.Get("Content-Type"))
			}
			var payload map[string]json.RawMessage
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			postedBlocks = append(json.RawMessage(nil), payload["blocks"]...)
			var blocks []map[string]any
			if err := json.Unmarshal(payload["blocks"], &blocks); err != nil {
				t.Fatalf("decode blocks: %v", err)
			}
			elements := blocks[0]["elements"].([]any)
			list := elements[1].(map[string]any)
			if list["type"] != "rich_text_list" || list["style"] != "ordered" {
				t.Fatalf("expected ordered rich list, got %+v", list)
			}
			var fallback string
			if err := json.Unmarshal(payload["text"], &fallback); err != nil {
				t.Fatalf("decode fallback text: %v", err)
			}
			if fallback != "Hey <!subteam^S123>, questions:\n\n1. One\n2. Two" {
				t.Fatalf("expected resolved fallback text, got %q", fallback)
			}
			return dmJSONResponse(req, fmt.Sprintf(`{"ok":true,"channel":"D123","ts":"200.2","message":{"text":"fallback","ts":"200.2","blocks":%s}}`, postedBlocks))
		case "/api/conversations.history":
			historyCalls++
			return dmJSONResponse(req, fmt.Sprintf(`{"ok":true,"messages":[{"text":"persisted","ts":"200.2","blocks":%s}]}`, postedBlocks))
		case "/api/chat.getPermalink":
			return dmJSONResponse(req, `{"ok":true,"channel":"D123","permalink":"https://example.slack.com/archives/D123/p2002"}`)
		default:
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
	})
	auth := ctx.Config.Workspaces["default"]
	auth.UserGroups = map[string]string{"team": "S123"}
	ctx.Config.Workspaces["default"] = auth

	output := captureStdout(t, func() {
		err := (&MessageSendCmd{
			Recipient: "D123",
			Text:      "Hey @team, questions:\n\n1. One\n2. Two",
			Rich:      true,
			JSON:      true,
		}).Run(ctx)
		if err != nil {
			t.Fatalf("MessageSendCmd.Run returned error: %v", err)
		}
	})
	if userGroupCalls != 0 {
		t.Fatalf("expected configured user-group mapping to avoid API lookup, got %d calls", userGroupCalls)
	}
	if historyCalls != 1 {
		t.Fatalf("expected post-send history verification, got %d calls", historyCalls)
	}
	if !strings.Contains(output, `"verified": true`) || !strings.Contains(output, `"permalink": "https://example.slack.com/archives/D123/p2002"`) || !strings.Contains(output, `"blocks"`) {
		t.Fatalf("expected verified JSON result with permalink and blocks, got %q", output)
	}
}

func TestMessageSendDryRunDoesNotPost(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("unexpected API request %s", req.URL.Path)
	})
	output := captureStdout(t, func() {
		err := (&MessageSendCmd{Recipient: "D123", Text: "1. One\n2. Two", Rich: true, DryRun: true}).Run(ctx)
		if err != nil {
			t.Fatalf("dry run returned error: %v", err)
		}
	})
	if !strings.Contains(output, `"rich_text_list"`) || !strings.Contains(output, `"channel": "D123"`) {
		t.Fatalf("unexpected dry-run payload: %s", output)
	}
}

func TestMessageSendLoadsInlineBlocks(t *testing.T) {
	cmd := &MessageSendCmd{Blocks: `[{"type":"rich_text","elements":[]}]`}
	blocks, err := cmd.loadBlocks()
	if err != nil {
		t.Fatalf("loadBlocks returned error: %v", err)
	}
	if !strings.Contains(string(blocks), `"rich_text"`) {
		t.Fatalf("unexpected blocks: %s", blocks)
	}
}

func TestMessageSendRejectsOversizedOrExcessiveBlocks(t *testing.T) {
	t.Run("oversized payload", func(t *testing.T) {
		_, err := (&MessageSendCmd{Blocks: "[" + strings.Repeat(" ", maxBlocksPayloadBytes)}).loadBlocks()
		if err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Fatalf("expected payload-size error, got %v", err)
		}
	})
	t.Run("more than Slack message limit", func(t *testing.T) {
		blocks := make([]map[string]string, maxMessageBlocks+1)
		for i := range blocks {
			blocks[i] = map[string]string{"type": "divider"}
		}
		body, err := json.Marshal(blocks)
		if err != nil {
			t.Fatal(err)
		}
		_, err = (&MessageSendCmd{Blocks: string(body)}).loadBlocks()
		if err == nil || !strings.Contains(err.Error(), "at most 50") {
			t.Fatalf("expected block-count error, got %v", err)
		}
	})
}

func TestMessageSendRejectsMalformedBlocks(t *testing.T) {
	for _, blocks := range []string{`{"type":"section"}`, `[]`, `[{"text":"missing type"}]`} {
		if _, err := (&MessageSendCmd{Blocks: blocks}).loadBlocks(); err == nil {
			t.Fatalf("expected malformed blocks %q to fail", blocks)
		}
	}
}

func TestMessageUpdatePlainTextClearsExistingBlocks(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/chat.update":
			var payload map[string]json.RawMessage
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if string(payload["blocks"]) != "[]" {
				t.Fatalf("expected explicit empty blocks array, got %s", payload["blocks"])
			}
			return dmJSONResponse(req, `{"ok":true,"channel":"D123","ts":"300.3","message":{"text":"plain","ts":"300.3"}}`)
		case "/api/chat.getPermalink":
			return dmJSONResponse(req, `{"ok":true,"channel":"D123","permalink":"https://example.slack.com/archives/D123/p3003"}`)
		default:
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
	})
	if err := (&MessageUpdateCmd{Channel: "D123", Timestamp: "300.3", Text: "plain"}).Run(ctx); err != nil {
		t.Fatalf("MessageUpdateCmd.Run returned error: %v", err)
	}
}

func TestMessageUpdateRichCreatesNativeList(t *testing.T) {
	var updatedBlocks json.RawMessage
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/chat.update":
			var payload map[string]json.RawMessage
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			updatedBlocks = append(json.RawMessage(nil), payload["blocks"]...)
			if string(payload["ts"]) != `"200.2"` {
				t.Fatalf("unexpected timestamp: %s", payload["ts"])
			}
			if !strings.Contains(string(payload["blocks"]), `rich_text_list`) {
				t.Fatalf("expected rich list: %s", payload["blocks"])
			}
			return dmJSONResponse(req, fmt.Sprintf(`{"ok":true,"channel":"D123","ts":"200.2","message":{"text":"updated","ts":"200.2","blocks":%s}}`, updatedBlocks))
		case "/api/conversations.history":
			return dmJSONResponse(req, fmt.Sprintf(`{"ok":true,"messages":[{"text":"updated","ts":"200.2","blocks":%s}]}`, updatedBlocks))
		case "/api/chat.getPermalink":
			return dmJSONResponse(req, `{"ok":true,"channel":"D123","permalink":"https://example.slack.com/archives/D123/p2002"}`)
		default:
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
	})
	output := captureStdout(t, func() {
		err := (&MessageUpdateCmd{Channel: "D123", Timestamp: "200.2", Text: "1. First\n2. Second", Rich: true}).Run(ctx)
		if err != nil {
			t.Fatalf("MessageUpdateCmd.Run returned error: %v", err)
		}
	})
	if !strings.Contains(output, "Updated message") || !strings.Contains(output, "Verified native rich formatting") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestVerifyRichMessageResponseAcceptsArbitrarySectionBlocks(t *testing.T) {
	expected := json.RawMessage(`[{"type":"section","text":{"type":"mrkdwn","text":"hello"}}]`)
	actual := []slack.Block{{Type: "section", Text: &slack.BlockText{Type: "mrkdwn", Text: "hello"}}}
	if err := verifyRichMessageResponse(expected, actual); err != nil {
		t.Fatalf("expected section blocks to verify, got %v", err)
	}
}

func TestVerifyRichMessageResponseIgnoresSlackServerDefaults(t *testing.T) {
	expected := json.RawMessage(`[{"type":"section","text":{"type":"mrkdwn","text":"hello"}}]`)
	var actual []slack.Block
	if err := json.Unmarshal([]byte(`[{"type":"section","block_id":"b1","text":{"type":"mrkdwn","text":"hello","verbatim":false}}]`), &actual); err != nil {
		t.Fatal(err)
	}
	if err := verifyRichMessageResponse(expected, actual); err != nil {
		t.Fatalf("expected Slack defaults to normalize, got %v", err)
	}
}

func TestVerifyRichMessageResponseRejectsChangedExplicitBlockID(t *testing.T) {
	expected := json.RawMessage(`[{"type":"section","block_id":"expected","text":{"type":"mrkdwn","text":"hello"}}]`)
	var actual []slack.Block
	if err := json.Unmarshal([]byte(`[{"type":"section","block_id":"changed","text":{"type":"mrkdwn","text":"hello","verbatim":false}}]`), &actual); err != nil {
		t.Fatal(err)
	}
	if err := verifyRichMessageResponse(expected, actual); err == nil {
		t.Fatal("expected changed explicit block_id to fail verification")
	}
}

func TestVerifyRichMessageResponseRejectsChangedBlockContent(t *testing.T) {
	expected := json.RawMessage(`[{"type":"section","text":{"type":"mrkdwn","text":"hello"}}]`)
	actual := []slack.Block{{Type: "section", Text: &slack.BlockText{Type: "mrkdwn", Text: "goodbye"}}}
	if err := verifyRichMessageResponse(expected, actual); err == nil {
		t.Fatal("expected changed block content to fail verification")
	}
}

func TestVerifyRichMessageResponseRequiresExpectedBlockTypes(t *testing.T) {
	expected := json.RawMessage(`[{"type":"section","text":{"type":"mrkdwn","text":"hello"}}]`)
	actual := []slack.Block{{Type: "rich_text"}}
	if err := verifyRichMessageResponse(expected, actual); err == nil {
		t.Fatal("expected mismatched block types to fail verification")
	}
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

	cli = &CLI{}
	parser, err = kong.New(cli, kong.Vars{"version": "test"})
	if err != nil {
		t.Fatalf("failed to rebuild parser: %v", err)
	}
	if _, err := parser.Parse([]string{"message", "update", "C123", "200.2", "1. One", "--rich"}); err != nil {
		t.Fatalf("expected message update to parse, got %v", err)
	}
}
