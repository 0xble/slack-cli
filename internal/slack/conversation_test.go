package slack

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
)

func TestResolveConversationTargetEnterpriseUserID(t *testing.T) {
	var sawUserInfo bool
	var sawOpenConversation bool
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/api/users.info":
					sawUserInfo = true
					if req.URL.Query().Get("user") != "W123" {
						t.Fatalf("expected user=W123, got %q", req.URL.Query().Get("user"))
					}
					return jsonResponse(req, `{"ok":true,"user":{"id":"W123","name":"alice"}}`)
				case "/api/conversations.open":
					sawOpenConversation = true
					body, err := io.ReadAll(req.Body)
					if err != nil {
						t.Fatalf("failed to read request body: %v", err)
					}
					values, err := url.ParseQuery(string(body))
					if err != nil {
						t.Fatalf("failed to parse request body: %v", err)
					}
					if values.Get("users") != "W123" {
						t.Fatalf("expected users=W123, got %q", values.Get("users"))
					}
					return jsonResponse(req, `{"ok":true,"channel":{"id":"D123","user":"W123","is_im":true}}`)
				default:
					return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
				}
			}),
		},
	}

	target, err := ResolveConversationTarget(client, "W123")
	if err != nil {
		t.Fatalf("ResolveConversationTarget returned error: %v", err)
	}
	if target.ChannelID != "D123" || !target.IsDM || target.Username != "alice" {
		t.Fatalf("unexpected target: %+v", target)
	}
	if !sawUserInfo || !sawOpenConversation {
		t.Fatalf("expected users.info and conversations.open to be called")
	}
}
