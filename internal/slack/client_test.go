package slack

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestParseThreadURL(t *testing.T) {
	t.Run("message permalink uses message timestamp", func(t *testing.T) {
		channel, threadTS, err := ParseThreadURL("https://buildkite.slack.com/archives/C123/p1773973307481399")
		if err != nil {
			t.Fatalf("ParseThreadURL returned error: %v", err)
		}
		if channel != "C123" {
			t.Fatalf("expected channel C123, got %q", channel)
		}
		if threadTS != "1773973307.481399" {
			t.Fatalf("expected threadTS 1773973307.481399, got %q", threadTS)
		}
	})

	t.Run("reply permalink prefers thread_ts query parameter", func(t *testing.T) {
		channel, threadTS, err := ParseThreadURL("https://buildkite.slack.com/archives/C123/p1773999999000000?thread_ts=1773973307.481399&cid=C123")
		if err != nil {
			t.Fatalf("ParseThreadURL returned error: %v", err)
		}
		if channel != "C123" {
			t.Fatalf("expected channel C123, got %q", channel)
		}
		if threadTS != "1773973307.481399" {
			t.Fatalf("expected threadTS 1773973307.481399, got %q", threadTS)
		}
	})
}

func TestParseMessageURL(t *testing.T) {
	t.Run("message permalink", func(t *testing.T) {
		ref, err := ParseMessageURL("https://buildkite.slack.com/archives/C123/p1773973307481399")
		if err != nil {
			t.Fatalf("ParseMessageURL returned error: %v", err)
		}
		if ref.WorkspaceHost != "buildkite.slack.com" {
			t.Fatalf("expected workspace host, got %q", ref.WorkspaceHost)
		}
		if ref.ChannelID != "C123" {
			t.Fatalf("expected channel C123, got %q", ref.ChannelID)
		}
		if ref.Timestamp != "1773973307.481399" {
			t.Fatalf("expected timestamp, got %q", ref.Timestamp)
		}
	})

	t.Run("reply permalink uses reply timestamp from path", func(t *testing.T) {
		ref, err := ParseMessageURL("https://buildkite.slack.com/archives/C123/p1773999999000000?thread_ts=1773973307.481399&cid=C123")
		if err != nil {
			t.Fatalf("ParseMessageURL returned error: %v", err)
		}
		if ref.Timestamp != "1773999999.000000" {
			t.Fatalf("expected reply timestamp from path, got %q", ref.Timestamp)
		}
	})
}

func TestIsSlackHostedURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{name: "slack root", url: "https://slack.com/file.png", want: true},
		{name: "slack subdomain", url: "https://files.slack.com/file.png", want: true},
		{name: "http slack subdomain", url: "http://files.slack.com/file.png", want: false},
		{name: "uppercase host", url: "https://FILES.SLACK.COM/file.png", want: true},
		{name: "external host", url: "https://example.com/file.png", want: false},
		{name: "empty", url: "", want: false},
		{name: "invalid", url: "://bad-url", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSlackHostedURL(tt.url)
			if got != tt.want {
				t.Fatalf("isSlackHostedURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestDownloadPrivateFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("123456"))
	}))
	defer server.Close()

	client := &Client{
		userToken:  "xoxp-test-token",
		httpClient: server.Client(),
	}

	t.Run("within size limit", func(t *testing.T) {
		body, contentType, err := client.DownloadPrivateFile(server.URL, 6)
		if err != nil {
			t.Fatalf("DownloadPrivateFile returned error: %v", err)
		}
		if string(body) != "123456" {
			t.Fatalf("DownloadPrivateFile body = %q, want %q", string(body), "123456")
		}
		if contentType != "image/png" {
			t.Fatalf("DownloadPrivateFile contentType = %q, want %q", contentType, "image/png")
		}
	})

	t.Run("exceeds size limit", func(t *testing.T) {
		_, _, err := client.DownloadPrivateFile(server.URL, 5)
		if err == nil {
			t.Fatalf("DownloadPrivateFile expected error when payload exceeds size limit")
		}
		if !strings.Contains(err.Error(), "download exceeds limit") {
			t.Fatalf("DownloadPrivateFile error = %q, want contains %q", err.Error(), "download exceeds limit")
		}
	})

	t.Run("invalid limit", func(t *testing.T) {
		_, _, err := client.DownloadPrivateFile(server.URL, 0)
		if err == nil {
			t.Fatalf("DownloadPrivateFile expected error for invalid maxBytes")
		}
		if !strings.Contains(err.Error(), "maxBytes must be > 0") {
			t.Fatalf("DownloadPrivateFile error = %q, want contains %q", err.Error(), "maxBytes must be > 0")
		}
	})
}

func TestDownloadPrivateFile_AuthorizationHeaderPolicy(t *testing.T) {
	tests := []struct {
		name     string
		fileURL  string
		wantAuth string
	}{
		{name: "https slack host sends token", fileURL: "https://files.slack.com/files-pri/T123/F123/file.png", wantAuth: "Bearer [REDACTED:slack-access-token]"},
		{name: "http slack host does not send token", fileURL: "http://files.slack.com/files-pri/T123/F123/file.png", wantAuth: ""},
		{name: "https external host does not send token", fileURL: "https://example.com/file.png", wantAuth: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotAuth string

			client := &Client{
				userToken: "[REDACTED:slack-access-token]",
				httpClient: &http.Client{
					Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
						gotAuth = req.Header.Get("Authorization")
						return &http.Response{
							StatusCode: http.StatusOK,
							Status:     "200 OK",
							Header:     http.Header{"Content-Type": []string{"image/png"}},
							Body:       io.NopCloser(strings.NewReader("ok")),
							Request:    req,
						}, nil
					}),
				},
			}

			_, _, err := client.DownloadPrivateFile(tt.fileURL, 2)
			if err != nil {
				t.Fatalf("DownloadPrivateFile() returned error: %v", err)
			}

			if gotAuth != tt.wantAuth {
				t.Fatalf("DownloadPrivateFile() authorization header = %q, want %q", gotAuth, tt.wantAuth)
			}
		})
	}
}

func TestDownloadPrivateFile_PreservesSlackAuthAcrossRedirects(t *testing.T) {
	var requests []string

	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				requests = append(requests, req.URL.String()+" auth="+req.Header.Get("Authorization"))

				switch req.URL.Host {
				case "files.slack.com":
					return &http.Response{
						StatusCode: http.StatusFound,
						Status:     "302 Found",
						Header: http.Header{
							"Location": []string{"https://workspace.slack.com/files-pri/T123/F123/report.txt"},
						},
						Body:    io.NopCloser(strings.NewReader("")),
						Request: req,
					}, nil
				case "workspace.slack.com":
					return &http.Response{
						StatusCode: http.StatusOK,
						Status:     "200 OK",
						Header:     http.Header{"Content-Type": []string{"text/plain"}},
						Body:       io.NopCloser(strings.NewReader("ok")),
						Request:    req,
					}, nil
				default:
					return nil, fmt.Errorf("unexpected host %s", req.URL.Host)
				}
			}),
		},
	}

	body, contentType, err := client.DownloadPrivateFile("https://files.slack.com/download/F123", 2)
	if err != nil {
		t.Fatalf("DownloadPrivateFile returned error: %v", err)
	}
	if string(body) != "ok" {
		t.Fatalf("DownloadPrivateFile body = %q, want %q", string(body), "ok")
	}
	if contentType != "text/plain" {
		t.Fatalf("DownloadPrivateFile contentType = %q, want %q", contentType, "text/plain")
	}
	if len(requests) != 2 {
		t.Fatalf("expected 2 requests, got %d (%v)", len(requests), requests)
	}
	for _, got := range requests {
		if !strings.Contains(got, "auth=Bearer xoxp-test-token") {
			t.Fatalf("redirect chain missing authorization header: %v", requests)
		}
	}
}

func TestDownloadPrivateFileToWriter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("streamed"))
	}))
	defer server.Close()

	client := &Client{
		userToken:  "xoxp-test-token",
		httpClient: server.Client(),
	}

	var builder strings.Builder
	contentType, written, err := client.DownloadPrivateFileToWriter(server.URL, &builder)
	if err != nil {
		t.Fatalf("DownloadPrivateFileToWriter returned error: %v", err)
	}
	if contentType != "text/plain" {
		t.Fatalf("unexpected contentType %q", contentType)
	}
	if written != int64(len("streamed")) {
		t.Fatalf("unexpected bytes written %d", written)
	}
	if builder.String() != "streamed" {
		t.Fatalf("unexpected body %q", builder.String())
	}
}

func TestTransferHTTPClient_RemovesOverallTimeout(t *testing.T) {
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	transferClient := client.transferHTTPClient()
	if transferClient == client.httpClient {
		t.Fatalf("transferHTTPClient should return a distinct client copy")
	}
	if transferClient.Timeout != 0 {
		t.Fatalf("transferHTTPClient timeout = %v, want 0", transferClient.Timeout)
	}
	if client.httpClient.Timeout != 30*time.Second {
		t.Fatalf("base client timeout mutated to %v", client.httpClient.Timeout)
	}
}

func TestOpenConversation_UsesPOSTFormEncoding(t *testing.T) {
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodPost {
					t.Fatalf("expected POST, got %s", req.Method)
				}
				if req.URL.Path != "/api/conversations.open" {
					t.Fatalf("expected /api/conversations.open, got %s", req.URL.Path)
				}
				if got := req.Header.Get("Authorization"); got != "Bearer xoxp-test-token" {
					t.Fatalf("unexpected Authorization header %q", got)
				}

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
				if values.Get("return_im") != "true" {
					t.Fatalf("expected return_im=true, got %q", values.Get("return_im"))
				}

				return jsonResponse(req, `{"ok":true,"channel":{"id":"D123","user":"U123","is_im":true}}`)
			}),
		},
	}

	resp, err := client.OpenConversation([]string{"U123"}, true)
	if err != nil {
		t.Fatalf("OpenConversation returned error: %v", err)
	}
	if resp.Channel.ID != "D123" {
		t.Fatalf("expected channel D123, got %q", resp.Channel.ID)
	}
}

func TestListFiles(t *testing.T) {
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet {
					t.Fatalf("expected GET, got %s", req.Method)
				}
				if req.URL.Path != "/api/files.list" {
					t.Fatalf("expected /api/files.list, got %s", req.URL.Path)
				}
				if req.URL.Query().Get("count") != "20" {
					t.Fatalf("expected count=20, got %q", req.URL.Query().Get("count"))
				}
				return jsonResponse(req, `{"ok":true,"files":[{"id":"F123","name":"report.txt","title":"Report","size":42}]}`)
			}),
		},
	}

	resp, err := client.ListFiles(ListFilesParams{Limit: 20})
	if err != nil {
		t.Fatalf("ListFiles returned error: %v", err)
	}
	if len(resp.Files) != 1 || resp.Files[0].ID != "F123" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestListConversationsPage(t *testing.T) {
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/api/conversations.list" {
					t.Fatalf("expected /api/conversations.list, got %s", req.URL.Path)
				}
				if req.URL.Query().Get("cursor") != "page-2" {
					t.Fatalf("expected cursor=page-2, got %q", req.URL.Query().Get("cursor"))
				}
				return jsonResponse(req, `{"ok":true,"channels":[{"id":"C123","name":"general"}],"response_metadata":{"next_cursor":"page-3"}}`)
			}),
		},
	}

	resp, err := client.ListConversationsPage("public_channel,private_channel", 1000, "page-2")
	if err != nil {
		t.Fatalf("ListConversationsPage returned error: %v", err)
	}
	if resp.ResponseMetadata.NextCursor != "page-3" {
		t.Fatalf("unexpected next cursor %q", resp.ResponseMetadata.NextCursor)
	}
}

func TestGetFileInfo(t *testing.T) {
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/api/files.info" {
					t.Fatalf("expected /api/files.info, got %s", req.URL.Path)
				}
				if req.URL.Query().Get("file") != "F123" {
					t.Fatalf("expected file=F123, got %q", req.URL.Query().Get("file"))
				}
				return jsonResponse(req, `{"ok":true,"file":{"id":"F123","name":"report.txt","title":"Report","size":42}}`)
			}),
		},
	}

	file, err := client.GetFileInfo("F123")
	if err != nil {
		t.Fatalf("GetFileInfo returned error: %v", err)
	}
	if file.ID != "F123" || file.Name != "report.txt" {
		t.Fatalf("unexpected file: %+v", file)
	}
}

func TestDeleteFile(t *testing.T) {
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodPost {
					t.Fatalf("expected POST, got %s", req.Method)
				}
				if req.URL.Path != "/api/files.delete" {
					t.Fatalf("expected /api/files.delete, got %s", req.URL.Path)
				}
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("failed to read request body: %v", err)
				}
				values, err := url.ParseQuery(string(body))
				if err != nil {
					t.Fatalf("failed to parse request body: %v", err)
				}
				if values.Get("file") != "F123" {
					t.Fatalf("expected file=F123, got %q", values.Get("file"))
				}
				return jsonResponse(req, `{"ok":true}`)
			}),
		},
	}

	if err := client.DeleteFile("F123"); err != nil {
		t.Fatalf("DeleteFile returned error: %v", err)
	}
}

func TestGetUploadURLExternal(t *testing.T) {
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodPost {
					t.Fatalf("expected POST, got %s", req.Method)
				}
				if req.URL.Path != "/api/files.getUploadURLExternal" {
					t.Fatalf("expected /api/files.getUploadURLExternal, got %s", req.URL.Path)
				}
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("failed to read request body: %v", err)
				}
				values, err := url.ParseQuery(string(body))
				if err != nil {
					t.Fatalf("failed to parse request body: %v", err)
				}
				if values.Get("filename") != "report.txt" {
					t.Fatalf("expected filename=report.txt, got %q", values.Get("filename"))
				}
				if values.Get("length") != "42" {
					t.Fatalf("expected length=42, got %q", values.Get("length"))
				}
				return jsonResponse(req, `{"ok":true,"upload_url":"https://files.slack.com/upload/v1/abc","file_id":"F123"}`)
			}),
		},
	}

	resp, err := client.GetUploadURLExternal("report.txt", 42)
	if err != nil {
		t.Fatalf("GetUploadURLExternal returned error: %v", err)
	}
	if resp.UploadURL != "https://files.slack.com/upload/v1/abc" || resp.FileID != "F123" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestAddReaction_UsesPOSTFormEncoding(t *testing.T) {
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodPost {
					t.Fatalf("expected POST, got %s", req.Method)
				}
				if req.URL.Path != "/api/reactions.add" {
					t.Fatalf("expected /api/reactions.add, got %s", req.URL.Path)
				}

				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("failed to read request body: %v", err)
				}
				values, err := url.ParseQuery(string(body))
				if err != nil {
					t.Fatalf("failed to parse request body: %v", err)
				}
				if values.Get("channel") != "C123" {
					t.Fatalf("expected channel=C123, got %q", values.Get("channel"))
				}
				if values.Get("timestamp") != "1775772298.509159" {
					t.Fatalf("expected timestamp=1775772298.509159, got %q", values.Get("timestamp"))
				}
				if values.Get("name") != "+1" {
					t.Fatalf("expected name=+1, got %q", values.Get("name"))
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
					Request:    req,
				}, nil
			}),
		},
	}

	resp, err := client.AddReaction("C123", "1775772298.509159", "+1")
	if err != nil {
		t.Fatalf("AddReaction returned error: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected ok response")
	}
}

func TestAddReactionAlreadyReactedIsSuccess(t *testing.T) {
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"ok":false,"error":"already_reacted"}`)),
					Request:    req,
				}, nil
			}),
		},
	}

	resp, err := client.AddReaction("C123", "1775772298.509159", "+1")
	if err != nil {
		t.Fatalf("AddReaction returned error: %v", err)
	}
	if !resp.OK {
		t.Fatalf("expected already_reacted to return ok response")
	}
}

func TestUploadExternalFile(t *testing.T) {
	var gotContentType string
	var gotLength int64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		gotContentType = r.Header.Get("Content-Type")
		gotLength = r.ContentLength
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read upload body: %v", err)
		}
		if string(body) != "hello" {
			t.Fatalf("unexpected upload body %q", string(body))
		}
		_, _ = w.Write([]byte("OK - 5"))
	}))
	defer server.Close()

	client := &Client{httpClient: server.Client()}
	if err := client.UploadExternalFile(server.URL, "report.txt", strings.NewReader("hello"), 5); err != nil {
		t.Fatalf("UploadExternalFile returned error: %v", err)
	}
	if gotContentType != "text/plain; charset=utf-8" {
		t.Fatalf("unexpected Content-Type %q", gotContentType)
	}
	if gotLength != 5 {
		t.Fatalf("unexpected Content-Length %d", gotLength)
	}
}

func TestCompleteUploadExternal(t *testing.T) {
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodPost {
					t.Fatalf("expected POST, got %s", req.Method)
				}
				if req.URL.Path != "/api/files.completeUploadExternal" {
					t.Fatalf("expected /api/files.completeUploadExternal, got %s", req.URL.Path)
				}
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("failed to read request body: %v", err)
				}
				values, err := url.ParseQuery(string(body))
				if err != nil {
					t.Fatalf("failed to parse request body: %v", err)
				}
				if values.Get("channel_id") != "C123" {
					t.Fatalf("expected channel_id=C123, got %q", values.Get("channel_id"))
				}
				if values.Get("initial_comment") != "hello" {
					t.Fatalf("expected initial_comment=hello, got %q", values.Get("initial_comment"))
				}
				if values.Get("thread_ts") != "123.456" {
					t.Fatalf("expected thread_ts=123.456, got %q", values.Get("thread_ts"))
				}
				if values.Get("files") != `[{"id":"F123","title":"Report"}]` {
					t.Fatalf("unexpected files payload %q", values.Get("files"))
				}
				return jsonResponse(req, `{"ok":true,"files":[{"id":"F123","name":"report.txt","title":"Report"}]}`)
			}),
		},
	}

	resp, err := client.CompleteUploadExternal("F123", "Report", "C123", "hello", "123.456")
	if err != nil {
		t.Fatalf("CompleteUploadExternal returned error: %v", err)
	}
	if len(resp.Files) != 1 || resp.Files[0].ID != "F123" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestListUsersPage(t *testing.T) {
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/api/users.list" {
					t.Fatalf("expected /api/users.list, got %s", req.URL.Path)
				}
				if req.URL.Query().Get("cursor") != "page-2" {
					t.Fatalf("expected cursor=page-2, got %q", req.URL.Query().Get("cursor"))
				}
				return jsonResponse(req, `{"ok":true,"members":[{"id":"U123","name":"alice"}],"response_metadata":{"next_cursor":"page-3"}}`)
			}),
		},
	}

	resp, err := client.ListUsersPage(1000, "page-2")
	if err != nil {
		t.Fatalf("ListUsersPage returned error: %v", err)
	}
	if resp.ResponseMetadata.NextCursor != "page-3" {
		t.Fatalf("unexpected next cursor %q", resp.ResponseMetadata.NextCursor)
	}
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

func TestListFiles_UsesQueryParams(t *testing.T) {
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet {
					t.Fatalf("expected GET, got %s", req.Method)
				}
				if req.URL.Path != "/api/files.list" {
					t.Fatalf("expected /api/files.list, got %s", req.URL.Path)
				}
				values := req.URL.Query()
				if values.Get("count") != "10" {
					t.Fatalf("expected count=10, got %q", values.Get("count"))
				}
				if values.Get("types") != "canvas" {
					t.Fatalf("expected types=canvas, got %q", values.Get("types"))
				}
				if values.Get("channel") != "C123" {
					t.Fatalf("expected channel=C123, got %q", values.Get("channel"))
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"ok":true,"files":[{"id":"F123","filetype":"quip"}]}`)),
					Request:    req,
				}, nil
			}),
		},
	}

	resp, err := client.ListFiles(ListFilesParams{Limit: 10, Types: "canvas", ChannelID: "C123"})
	if err != nil {
		t.Fatalf("ListFiles returned error: %v", err)
	}
	if len(resp.Files) != 1 || resp.Files[0].ID != "F123" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestGetUploadURLExternal_UsesPOSTFormEncoding(t *testing.T) {
	client := &Client{
		userToken: "xoxp-test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodPost {
					t.Fatalf("expected POST, got %s", req.Method)
				}
				if req.URL.Path != "/api/files.getUploadURLExternal" {
					t.Fatalf("expected /api/files.getUploadURLExternal, got %s", req.URL.Path)
				}
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("failed to read request body: %v", err)
				}
				values, err := url.ParseQuery(string(body))
				if err != nil {
					t.Fatalf("failed to parse request body: %v", err)
				}
				if values.Get("filename") != "report.txt" || values.Get("length") != "5" {
					t.Fatalf("unexpected body: %q", string(body))
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"ok":true,"upload_url":"https://upload.example/F123","file_id":"F123"}`)),
					Request:    req,
				}, nil
			}),
		},
	}

	resp, err := client.GetUploadURLExternal("report.txt", 5)
	if err != nil {
		t.Fatalf("GetUploadURLExternal returned error: %v", err)
	}
	if resp.FileID != "F123" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestPostMessageWithBlocksUsesJSON(t *testing.T) {
	client := &Client{
		userToken: "test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/api/chat.postMessage" {
					t.Fatalf("expected chat.postMessage, got %s", req.URL.Path)
				}
				if got := req.Header.Get("Content-Type"); got != "application/json" {
					t.Fatalf("expected application/json, got %q", got)
				}
				var payload ChatMessageRequest
				if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				if payload.Channel != "C123" || payload.Text != "fallback" || payload.ThreadTS != "100.1" {
					t.Fatalf("unexpected payload: %+v", payload)
				}
				var blocks []map[string]any
				if err := json.Unmarshal(payload.Blocks, &blocks); err != nil {
					t.Fatalf("decode blocks: %v", err)
				}
				if len(blocks) != 1 || blocks[0]["type"] != "rich_text" {
					t.Fatalf("unexpected blocks: %+v", blocks)
				}
				return jsonResponse(req, `{"ok":true,"channel":"C123","ts":"200.2","message":{"text":"fallback","ts":"200.2","blocks":[{"type":"rich_text","elements":[]}]}}`)
			}),
		},
	}

	blocks := json.RawMessage(`[{"type":"rich_text","elements":[]}]`)
	resp, err := client.PostMessageWithBlocks(ChatMessageRequest{
		Channel:  "C123",
		Text:     "fallback",
		ThreadTS: "100.1",
		Blocks:   blocks,
	})
	if err != nil {
		t.Fatalf("PostMessageWithBlocks returned error: %v", err)
	}
	if resp.TS != "200.2" || len(resp.Message.Blocks) != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestUpdateMessageWithBlocksUsesJSON(t *testing.T) {
	client := &Client{
		userToken: "test-token",
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/api/chat.update" || req.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("unexpected request: %s %s", req.URL.Path, req.Header.Get("Content-Type"))
			}
			var payload ChatMessageRequest
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if payload.Channel != "C123" || payload.TS != "200.2" || len(payload.Blocks) == 0 {
				t.Fatalf("unexpected payload: %+v", payload)
			}
			return jsonResponse(req, `{"ok":true,"channel":"C123","ts":"200.2","message":{"text":"updated","ts":"200.2","blocks":[{"type":"rich_text","elements":[]}]}}`)
		})},
	}
	resp, err := client.UpdateMessageWithBlocks(ChatMessageRequest{Channel: "C123", TS: "200.2", Text: "updated", Blocks: json.RawMessage(`[{"type":"rich_text","elements":[]}]`)})
	if err != nil {
		t.Fatalf("UpdateMessageWithBlocks returned error: %v", err)
	}
	if resp.TS != "200.2" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestGetMessagePermalink(t *testing.T) {
	client := &Client{
		userToken: "test-token",
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/api/chat.getPermalink" || req.URL.Query().Get("channel") != "C123" || req.URL.Query().Get("message_ts") != "200.2" {
				t.Fatalf("unexpected request: %s?%s", req.URL.Path, req.URL.RawQuery)
			}
			return jsonResponse(req, `{"ok":true,"channel":"C123","permalink":"https://example.slack.com/archives/C123/p2002"}`)
		})},
	}
	permalink, err := client.GetMessagePermalink("C123", "200.2")
	if err != nil {
		t.Fatalf("GetMessagePermalink returned error: %v", err)
	}
	if permalink != "https://example.slack.com/archives/C123/p2002" {
		t.Fatalf("unexpected permalink %q", permalink)
	}
}

func TestGetMessageByTimestamp(t *testing.T) {
	client := &Client{
		userToken: "test-token",
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/api/conversations.history" || req.URL.Query().Get("oldest") != "200.2" || req.URL.Query().Get("inclusive") != "true" {
				t.Fatalf("unexpected request: %s?%s", req.URL.Path, req.URL.RawQuery)
			}
			return jsonResponse(req, `{"ok":true,"messages":[{"text":"other","ts":"300.3"},{"text":"target","ts":"200.2","blocks":[{"type":"rich_text","elements":[]}]}]}`)
		})},
	}
	message, err := client.GetMessageByTimestamp("C123", "200.2", "")
	if err != nil {
		t.Fatalf("GetMessageByTimestamp returned error: %v", err)
	}
	if message.Text != "target" || len(message.Blocks) != 1 {
		t.Fatalf("unexpected message: %+v", message)
	}
}

func TestGetMessageByTimestampInThread(t *testing.T) {
	client := &Client{
		userToken: "test-token",
		httpClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/api/conversations.replies" || req.URL.Query().Get("ts") != "100.1" {
				t.Fatalf("unexpected request: %s?%s", req.URL.Path, req.URL.RawQuery)
			}
			return jsonResponse(req, `{"ok":true,"messages":[{"text":"root","ts":"100.1"},{"text":"reply","ts":"200.2","blocks":[{"type":"rich_text","elements":[]}]}]}`)
		})},
	}
	message, err := client.GetMessageByTimestamp("C123", "200.2", "100.1")
	if err != nil {
		t.Fatalf("GetMessageByTimestamp returned error: %v", err)
	}
	if message.Text != "reply" || len(message.Blocks) != 1 {
		t.Fatalf("unexpected message: %+v", message)
	}
}

func TestResolveUserGroupMentions(t *testing.T) {
	client := &Client{
		userToken: "test-token",
		httpClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/api/usergroups.list" {
					t.Fatalf("expected usergroups.list, got %s", req.URL.Path)
				}
				return jsonResponse(req, `{"ok":true,"usergroups":[{"id":"S123","handle":"team","name":"Team"}]}`)
			}),
		},
	}

	got, err := client.ResolveUserGroupMentions("Hey @team, ask @unknown. Email a@team.com")
	if err != nil {
		t.Fatalf("ResolveUserGroupMentions returned error: %v", err)
	}
	want := "Hey <!subteam^S123>, ask @unknown. Email a@team.com"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
