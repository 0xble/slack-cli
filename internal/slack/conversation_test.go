package slack

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestResolveConversationTarget(t *testing.T) {
	t.Run("resolves channel id", func(t *testing.T) {
		client := &Client{
			userToken: "xoxp-test-token",
			httpClient: &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					switch req.URL.Path {
					case "/api/conversations.info":
						return jsonResponse(req, `{"ok":true,"channel":{"id":"C123","name":"general","is_channel":true}}`)
					default:
						return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
					}
				}),
			},
		}

		target, err := ResolveConversationTarget(client, "C123")
		if err != nil {
			t.Fatalf("ResolveConversationTarget returned error: %v", err)
		}
		if target.ChannelID != "C123" || target.ChannelName != "general" || target.Type != "channel" {
			t.Fatalf("unexpected target: %+v", target)
		}
	})

	t.Run("resolves private channel id", func(t *testing.T) {
		client := &Client{
			userToken: "xoxp-test-token",
			httpClient: &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					switch req.URL.Path {
					case "/api/conversations.info":
						return jsonResponse(req, `{"ok":true,"channel":{"id":"G123","name":"ops","is_group":true,"is_private":true}}`)
					default:
						return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
					}
				}),
			},
		}

		target, err := ResolveConversationTarget(client, "G123")
		if err != nil {
			t.Fatalf("ResolveConversationTarget returned error: %v", err)
		}
		if target.ChannelID != "G123" || target.Type != "group" {
			t.Fatalf("unexpected target: %+v", target)
		}
	})

	t.Run("resolves channel name with leading hash", func(t *testing.T) {
		client := &Client{
			userToken: "xoxp-test-token",
			httpClient: &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					switch req.URL.Path {
					case "/api/conversations.list":
						return jsonResponse(req, `{"ok":true,"channels":[{"id":"C123","name":"general","is_channel":true}]}`)
					default:
						return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
					}
				}),
			},
		}

		target, err := ResolveConversationTarget(client, "#general")
		if err != nil {
			t.Fatalf("ResolveConversationTarget returned error: %v", err)
		}
		if target.ChannelID != "C123" || target.ChannelName != "general" {
			t.Fatalf("unexpected target: %+v", target)
		}
	})

	t.Run("resolves bare channel name", func(t *testing.T) {
		client := &Client{
			userToken: "xoxp-test-token",
			httpClient: &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					switch req.URL.Path {
					case "/api/conversations.list":
						return jsonResponse(req, `{"ok":true,"channels":[{"id":"C123","name":"general","is_channel":true}]}`)
					default:
						return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
					}
				}),
			},
		}

		target, err := ResolveConversationTarget(client, "general")
		if err != nil {
			t.Fatalf("ResolveConversationTarget returned error: %v", err)
		}
		if target.ChannelID != "C123" || target.ChannelName != "general" {
			t.Fatalf("unexpected target: %+v", target)
		}
	})

	t.Run("returns channel not found for missing channel name", func(t *testing.T) {
		client := &Client{
			userToken: "xoxp-test-token",
			httpClient: &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					switch req.URL.Path {
					case "/api/conversations.list":
						return jsonResponse(req, `{"ok":true,"channels":[]}`)
					default:
						return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
					}
				}),
			},
		}

		_, err := ResolveConversationTarget(client, "#missing")
		if err == nil {
			t.Fatalf("expected error")
		}
		if !strings.Contains(err.Error(), "slack API error: channel_not_found") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestParseChannelRecipient(t *testing.T) {
	tests := []struct {
		input      string
		wantName   string
		wantLike   bool
		shouldFail bool
	}{
		{input: "#general", wantName: "general", wantLike: true},
		{input: "general", wantName: "general", wantLike: true},
		{input: "@alice", wantLike: false},
		{input: "U123", wantLike: false},
		{input: "D123", wantLike: false},
		{input: "C123", wantLike: false},
		{input: "G123", wantLike: false},
		{input: "", shouldFail: true},
	}

	for _, tt := range tests {
		gotName, gotLike, err := parseChannelRecipient(tt.input)
		if tt.shouldFail {
			if err == nil {
				t.Fatalf("parseChannelRecipient(%q) expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Fatalf("parseChannelRecipient(%q) returned error: %v", tt.input, err)
		}
		if gotName != tt.wantName || gotLike != tt.wantLike {
			t.Fatalf("parseChannelRecipient(%q) = (%q, %v), want (%q, %v)", tt.input, gotName, gotLike, tt.wantName, tt.wantLike)
		}
	}
}
