package slack

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestResolveDMTarget(t *testing.T) {
	t.Run("accepts direct message channel id", func(t *testing.T) {
		target, err := ResolveDMTarget(&Client{}, "D123")
		if err != nil {
			t.Fatalf("ResolveDMTarget returned error: %v", err)
		}
		if target.ChannelID != "D123" {
			t.Fatalf("expected D123, got %q", target.ChannelID)
		}
	})

	t.Run("opens dm for user id", func(t *testing.T) {
		client := &Client{
			userToken: "xoxp-test-token",
			httpClient: &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					switch req.URL.Path {
					case "/api/users.info":
						return jsonResponse(req, `{"ok":true,"user":{"id":"U123","name":"alice","real_name":"Alice"}}`)
					case "/api/conversations.open":
						if req.Method != http.MethodPost {
							t.Fatalf("expected POST for conversations.open, got %s", req.Method)
						}
						body, err := io.ReadAll(req.Body)
						if err != nil {
							t.Fatalf("failed to read request body: %v", err)
						}
						values, err := url.ParseQuery(string(body))
						if err != nil {
							t.Fatalf("failed to parse form body: %v", err)
						}
						if values.Get("users") != "U123" {
							t.Fatalf("expected users=U123, got %q", values.Get("users"))
						}
						return jsonResponse(req, `{"ok":true,"channel":{"id":"D123","user":"U123","is_im":true}}`)
					default:
						return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
					}
				}),
			},
		}

		target, err := ResolveDMTarget(client, "U123")
		if err != nil {
			t.Fatalf("ResolveDMTarget returned error: %v", err)
		}
		if target.ChannelID != "D123" || target.UserID != "U123" {
			t.Fatalf("unexpected target: %+v", target)
		}
	})

	t.Run("opens dm for username", func(t *testing.T) {
		client := &Client{
			userToken: "xoxp-test-token",
			httpClient: &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					switch req.URL.Path {
					case "/api/users.list":
						return jsonResponse(req, `{"ok":true,"members":[{"id":"U123","name":"alice","real_name":"Alice"}]}`)
					case "/api/conversations.open":
						return jsonResponse(req, `{"ok":true,"channel":{"id":"D123","user":"U123","is_im":true}}`)
					default:
						return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
					}
				}),
			},
		}

		target, err := ResolveDMTarget(client, "@Alice")
		if err != nil {
			t.Fatalf("ResolveDMTarget returned error: %v", err)
		}
		if target.ChannelID != "D123" || target.User == nil || target.User.Name != "alice" {
			t.Fatalf("unexpected target: %+v", target)
		}
	})

	t.Run("rejects channel-like recipient", func(t *testing.T) {
		_, err := ResolveDMTarget(&Client{}, "#general")
		if err == nil {
			t.Fatalf("expected error for channel-like recipient")
		}
		if !strings.Contains(err.Error(), "looks like a channel") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func jsonResponse(req *http.Request, body string) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}
