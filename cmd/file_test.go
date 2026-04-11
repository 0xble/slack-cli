package cmd

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/lox/slack-cli/internal/config"
	"github.com/lox/slack-cli/internal/slack"
)

func TestFileListRun(t *testing.T) {
	ctx := testFileContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/files.list":
			return fileJSONResponse(req, `{"ok":true,"files":[{"id":"F123","name":"report.txt","title":"Weekly Report","size":1536,"pretty_type":"Plain Text"}]}`)
		default:
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		if err := (&FileListCmd{Limit: 20}).Run(ctx); err != nil {
			t.Fatalf("FileListCmd.Run returned error: %v", err)
		}
	})

	if !strings.Contains(output, "F123 Weekly Report (1.5 KB, Plain Text, report.txt)") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestFileInfoRun(t *testing.T) {
	ctx := testFileContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/files.info":
			return fileJSONResponse(req, `{"ok":true,"file":{"id":"F123","name":"report.txt","title":"Weekly Report","size":1536,"pretty_type":"Plain Text","mode":"hosted","created":1775840563,"user":"U123","is_public":false,"file_access":"visible","permalink":"https://example.slack.com/files/F123"}}`)
		default:
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		if err := (&FileInfoCmd{FileID: "F123"}).Run(ctx); err != nil {
			t.Fatalf("FileInfoCmd.Run returned error: %v", err)
		}
	})

	if !strings.Contains(output, "ID: F123") || !strings.Contains(output, "Type: Plain Text") || !strings.Contains(output, "Permalink: https://example.slack.com/files/F123") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestFileDownloadRun(t *testing.T) {
	tempDir := t.TempDir()
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd returned error: %v", err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("os.Chdir returned error: %v", err)
	}
	defer func() {
		_ = os.Chdir(originalWD)
	}()

	ctx := testFileContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/files.info":
			return fileJSONResponse(req, `{"ok":true,"file":{"id":"F123","name":"report.txt","size":5,"url_private_download":"https://files.slack.com/download/F123"}}`)
		case "/download/F123":
			if got := req.Header.Get("Authorization"); got != "Bearer xoxp-test-token" {
				t.Fatalf("unexpected Authorization header %q", got)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"text/plain"}},
				Body:       io.NopCloser(strings.NewReader("hello!")),
				Request:    req,
			}, nil
		default:
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		if err := (&FileDownloadCmd{FileID: "F123"}).Run(ctx); err != nil {
			t.Fatalf("FileDownloadCmd.Run returned error: %v", err)
		}
	})

	body, err := os.ReadFile(filepath.Join(tempDir, "report.txt"))
	if err != nil {
		t.Fatalf("os.ReadFile returned error: %v", err)
	}
	if string(body) != "hello!" {
		t.Fatalf("downloaded body = %q, want %q", string(body), "hello!")
	}
	if !strings.Contains(output, "Downloaded file to") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestFileUploadRun(t *testing.T) {
	tempDir := t.TempDir()
	uploadPath := filepath.Join(tempDir, "report.txt")
	if err := os.WriteFile(uploadPath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	ctx := testFileContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/conversations.list":
			return fileJSONResponse(req, `{"ok":true,"channels":[{"id":"C123","name":"general","is_channel":true}]}`)
		case "/api/files.getUploadURLExternal":
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("failed to read request body: %v", err)
			}
			values, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("failed to parse request body: %v", err)
			}
			if values.Get("filename") != "report.txt" || values.Get("length") != "5" {
				t.Fatalf("unexpected upload init payload: %q", string(body))
			}
			return fileJSONResponse(req, `{"ok":true,"upload_url":"https://upload.example/F123","file_id":"F123"}`)
		case "/api/files.completeUploadExternal":
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
			if values.Get("initial_comment") != "hello there" {
				t.Fatalf("expected initial_comment, got %q", values.Get("initial_comment"))
			}
			if values.Get("thread_ts") != "123.456" {
				t.Fatalf("expected thread_ts, got %q", values.Get("thread_ts"))
			}
			if values.Get("files") != `[{"id":"F123","title":"Weekly Report"}]` {
				t.Fatalf("unexpected files payload %q", values.Get("files"))
			}
			return fileJSONResponse(req, `{"ok":true,"files":[{"id":"F123","name":"report.txt","title":"Weekly Report"}]}`)
		case "/F123":
			if req.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", req.Method)
			}
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("failed to read upload body: %v", err)
			}
			if string(body) != "hello" {
				t.Fatalf("unexpected upload body %q", string(body))
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"text/plain"}},
				Body:       io.NopCloser(strings.NewReader("OK - 5")),
				Request:    req,
			}, nil
		default:
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		if err := (&FileUploadCmd{
			Recipient: "#general",
			Path:      uploadPath,
			Title:     "Weekly Report",
			Comment:   "hello there",
			Thread:    "123.456",
		}).Run(ctx); err != nil {
			t.Fatalf("FileUploadCmd.Run returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Uploaded file to #general (C123): F123") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestFileUploadMissingScopeHint(t *testing.T) {
	ctx := testFileContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/files.getUploadURLExternal":
			return fileJSONResponse(req, `{"ok":false,"error":"missing_scope"}`)
		default:
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
	})

	tempDir := t.TempDir()
	uploadPath := filepath.Join(tempDir, "report.txt")
	if err := os.WriteFile(uploadPath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}

	err := (&FileUploadCmd{Recipient: "D123", Path: uploadPath}).Run(ctx)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "files:write") {
		t.Fatalf("expected files:write guidance, got %v", err)
	}
}

func TestFileDeleteRun(t *testing.T) {
	ctx := testFileContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/files.delete":
			return fileJSONResponse(req, `{"ok":true}`)
		default:
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		if err := (&FileDeleteCmd{FileIDs: []string{"F123", "F456"}}).Run(ctx); err != nil {
			t.Fatalf("FileDeleteCmd.Run returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Deleted file F123") || !strings.Contains(output, "Deleted file F456") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestFileCommandAliasesParse(t *testing.T) {
	cli := &CLI{}
	parser, err := kong.New(cli, kong.Vars{"version": "test"})
	if err != nil {
		t.Fatalf("failed to build parser: %v", err)
	}

	if _, err := parser.Parse([]string{"file", "ls"}); err != nil {
		t.Fatalf("expected file ls to parse, got %v", err)
	}
	if _, err := parser.Parse([]string{"file", "dl", "F123"}); err != nil {
		t.Fatalf("expected file dl to parse, got %v", err)
	}
	if _, err := parser.Parse([]string{"file", "up", "#general", "report.txt"}); err != nil {
		t.Fatalf("expected file up to parse, got %v", err)
	}
}

func testFileContext(fn func(req *http.Request) (*http.Response, error)) *Context {
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
					Transport: fileRoundTripFunc(fn),
				},
			)
		},
	}
}

func fileJSONResponse(req *http.Request, body string) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

type fileRoundTripFunc func(req *http.Request) (*http.Response, error)

func (f fileRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
