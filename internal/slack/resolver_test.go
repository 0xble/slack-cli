package slack

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// newTestResolver creates a Resolver with pre-populated caches (no API calls needed).
func newTestResolver(users map[string]string, channels map[string]string) *Resolver {
	if users == nil {
		users = make(map[string]string)
	}
	if channels == nil {
		channels = make(map[string]string)
	}
	return &Resolver{
		userCache:    users,
		channelCache: channels,
	}
}

func TestFormatText(t *testing.T) {
	tests := []struct {
		name     string
		users    map[string]string
		channels map[string]string
		input    string
		want     string
	}{
		{
			name:  "plain text unchanged",
			input: "hello world",
			want:  "hello world",
		},
		{
			name:  "user mention resolved",
			users: map[string]string{"U123": "alice"},
			input: "hey <@U123> check this",
			want:  "hey @alice check this",
		},
		{
			name:  "user mention with fallback name used when unknown",
			users: map[string]string{"U999": "U999"}, // simulate failed lookup (cached as raw ID)
			input: "hey <@U999|bob>",
			want:  "hey @bob",
		},
		{
			name:  "user mention with fallback ignored when resolved",
			users: map[string]string{"U123": "alice"},
			input: "hey <@U123|old-name>",
			want:  "hey @alice",
		},
		{
			name:  "display name containing <@ does not loop",
			users: map[string]string{"U123": "tricky<@name"},
			input: "hi <@U123>",
			want:  "hi @tricky<@name",
		},
		{
			name:     "channel mention with pipe",
			channels: map[string]string{"C456": "general"},
			input:    "see <#C456|general>",
			want:     "see #general",
		},
		{
			name:     "channel mention without pipe",
			channels: map[string]string{"C456": "general"},
			input:    "see <#C456>",
			want:     "see #general",
		},
		{
			name:  "URL with label",
			input: "visit <https://example.com|Example>",
			want:  "visit Example (https://example.com)",
		},
		{
			name:  "URL without label",
			input: "visit <https://example.com>",
			want:  "visit https://example.com",
		},
		{
			name:  "emoji shortcode",
			input: "great :thumbsup:",
			want:  "great 👍",
		},
		{
			name:     "multiple mentions",
			users:    map[string]string{"U1": "alice", "U2": "bob"},
			channels: map[string]string{"C1": "general"},
			input:    "<@U1> and <@U2> in <#C1|general>",
			want:     "@alice and @bob in #general",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestResolver(tt.users, tt.channels)
			got := r.FormatText(tt.input)
			if got != tt.want {
				t.Errorf("FormatText(%q)\n got: %q\nwant: %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolverPreloadChannelsPaginates(t *testing.T) {
	var cursors []string
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/api/conversations.list" {
					return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
				}
				if got := req.URL.Query().Get("types"); got != "public_channel,private_channel" {
					return nil, fmt.Errorf("unexpected types %q", got)
				}
				cursor := req.URL.Query().Get("cursor")
				cursors = append(cursors, cursor)
				switch cursor {
				case "":
					return resolverJSONResponse(req, `{"ok":true,"channels":[{"id":"C1","name":"general","is_channel":true}],"response_metadata":{"next_cursor":"page-2"}}`)
				case "page-2":
					return resolverJSONResponse(req, `{"ok":true,"channels":[{"id":"G2","name":"private","is_private":true}]}`)
				default:
					return nil, fmt.Errorf("unexpected cursor %q", cursor)
				}
			}),
		},
	}

	resolver := NewResolver(client)
	if err := resolver.PreloadChannels("public_channel,private_channel"); err != nil {
		t.Fatalf("PreloadChannels returned error: %v", err)
	}

	if strings.Join(cursors, ",") != ",page-2" {
		t.Fatalf("expected two paginated calls, got cursors %q", strings.Join(cursors, ","))
	}
	if got := resolver.ResolveChannelInfo("G2"); got == nil || got.Name != "private" || !got.IsPrivate {
		t.Fatalf("expected second page channel cached, got %+v", got)
	}
}

func TestResolverPreloadChannelsStopsOnRepeatedCursor(t *testing.T) {
	calls := 0
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if calls > 3 {
					return nil, fmt.Errorf("expected pagination to stop before call %d", calls)
				}
				return resolverJSONResponse(req, `{"ok":true,"channels":[],"response_metadata":{"next_cursor":"same-page"}}`)
			}),
		},
	}

	resolver := NewResolver(client)
	if err := resolver.PreloadChannels("public_channel"); err != nil {
		t.Fatalf("PreloadChannels returned error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected two calls before repeated cursor stop, got %d", calls)
	}
}

func TestResolverDisableChannelInfoLookupKeepsCachedOnly(t *testing.T) {
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return nil, fmt.Errorf("unexpected request to %s", req.URL.Path)
			}),
		},
	}
	resolver := NewResolver(client)
	resolver.channelInfoCache["C1"] = &Channel{ID: "C1", Name: "cached"}
	resolver.DisableChannelInfoLookup()

	if got := resolver.ResolveChannelInfo("C1"); got == nil || got.Name != "cached" {
		t.Fatalf("expected cached channel, got %+v", got)
	}
	if got := resolver.ResolveChannelInfo("C2"); got != nil {
		t.Fatalf("expected disabled lookup to return nil, got %+v", got)
	}
}

func TestListConversationsPageSendsCursor(t *testing.T) {
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				values, err := url.ParseQuery(req.URL.RawQuery)
				if err != nil {
					return nil, err
				}
				if values.Get("cursor") != "page-2" {
					return nil, fmt.Errorf("expected cursor=page-2, got %q", values.Get("cursor"))
				}
				return resolverJSONResponse(req, `{"ok":true,"channels":[{"id":"C2","name":"next"}],"response_metadata":{"next_cursor":"page-3"}}`)
			}),
		},
	}

	resp, err := client.ListConversationsPage("public_channel", 100, "page-2")
	if err != nil {
		t.Fatalf("ListConversationsPage returned error: %v", err)
	}
	if resp.ResponseMetadata.NextCursor != "page-3" {
		t.Fatalf("expected next cursor page-3, got %q", resp.ResponseMetadata.NextCursor)
	}
}

func resolverJSONResponse(req *http.Request, body string) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}
