package cmd

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/lox/slack-cli/internal/config"
	"github.com/lox/slack-cli/internal/slack"
)

func TestCanvasListRun(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/conversations.list":
			values := req.URL.Query()
			if values.Get("types") != "public_channel,private_channel" {
				t.Fatalf("expected channel list types, got %q", values.Get("types"))
			}
			return dmJSONResponse(req, `{"ok":true,"channels":[{"id":"C123","name":"general","is_channel":true}]}`)
		case "/api/files.list":
			values := req.URL.Query()
			if values.Get("types") != "canvas" {
				t.Fatalf("expected types=canvas, got %q", values.Get("types"))
			}
			if values.Get("channel") != "C123" {
				t.Fatalf("expected channel=C123, got %q", values.Get("channel"))
			}
			return dmJSONResponse(req, `{"ok":true,"files":[{"id":"F123","title":"Project Plan","size":1024,"filetype":"quip"}]}`)
		default:
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		if err := (&CanvasListCmd{Channel: "#general", Limit: 10}).Run(ctx); err != nil {
			t.Fatalf("CanvasListCmd.Run returned error: %v", err)
		}
	})

	if !strings.Contains(output, "F123 Project Plan (1.0 KB)") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestCanvasReadRun(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/files.info":
			return dmJSONResponse(req, `{"ok":true,"file":{"id":"F123","title":"Project Plan","size":54,"filetype":"quip","url_private_download":"https://files.slack.com/download/F123"}}`)
		case "/download/F123":
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"text/html"}},
				Body:       io.NopCloser(strings.NewReader(`<h1>Plan</h1><p>Hello <a>@U123</a></p>`)),
				Request:    req,
			}, nil
		case "/api/users.list":
			return dmJSONResponse(req, `{"ok":true,"members":[{"id":"U123","name":"alice","real_name":"Alice"}]}`)
		default:
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		if err := (&CanvasReadCmd{CanvasID: "F123"}).Run(ctx); err != nil {
			t.Fatalf("CanvasReadCmd.Run returned error: %v", err)
		}
	})

	if !strings.Contains(output, "# Plan") || !strings.Contains(output, "Hello @Alice") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestCanvasReadRawRun(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/files.info":
			return dmJSONResponse(req, `{"ok":true,"file":{"id":"F123","size":17,"filetype":"quip","url_private_download":"https://files.slack.com/download/F123"}}`)
		case "/download/F123":
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{"Content-Type": []string{"text/html"}},
				Body:       io.NopCloser(strings.NewReader(`<p>raw html</p>`)),
				Request:    req,
			}, nil
		default:
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		if err := (&CanvasReadCmd{CanvasID: "F123", Raw: true}).Run(ctx); err != nil {
			t.Fatalf("CanvasReadCmd.Run returned error: %v", err)
		}
	})

	if !strings.Contains(output, "<p>raw html</p>") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestCanvasReadRejectsNonCanvasFile(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/files.info":
			return dmJSONResponse(req, `{"ok":true,"file":{"id":"F123","filetype":"pdf","mimetype":"application/pdf"}}`)
		default:
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
	})

	err := (&CanvasReadCmd{CanvasID: "F123"}).Run(ctx)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "file is not a canvas") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCanvasDeleteRun(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/files.info":
			return dmJSONResponse(req, fmt.Sprintf(`{"ok":true,"file":{"id":"%s","filetype":"quip"}}`, req.URL.Query().Get("file")))
		case "/api/files.delete":
			body, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("failed to read request body: %v", err)
			}
			values, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("failed to parse request body: %v", err)
			}
			if values.Get("file") == "" {
				t.Fatalf("expected file param, got %q", string(body))
			}
			return dmJSONResponse(req, `{"ok":true}`)
		default:
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
	})

	output := captureStdout(t, func() {
		if err := (&CanvasDeleteCmd{CanvasIDs: []string{"F123", "F456"}}).Run(ctx); err != nil {
			t.Fatalf("CanvasDeleteCmd.Run returned error: %v", err)
		}
	})

	if !strings.Contains(output, "Deleted canvas F123") || !strings.Contains(output, "Deleted canvas F456") {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestCanvasDeleteRejectsNonCanvasFile(t *testing.T) {
	ctx := testDMContext(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/api/files.info":
			return dmJSONResponse(req, `{"ok":true,"file":{"id":"F123","filetype":"pdf","mimetype":"application/pdf"}}`)
		default:
			return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
		}
	})

	err := (&CanvasDeleteCmd{CanvasIDs: []string{"F123"}}).Run(ctx)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "file is not a canvas") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCanvasCommandParses(t *testing.T) {
	cli := &CLI{}
	parser, err := kong.New(cli, kong.Vars{"version": "test"})
	if err != nil {
		t.Fatalf("failed to build parser: %v", err)
	}

	if _, err := parser.Parse([]string{"canvas", "ls"}); err != nil {
		t.Fatalf("expected canvas ls to parse, got %v", err)
	}
	if _, err := parser.Parse([]string{"canvas", "read", "F123", "--raw"}); err != nil {
		t.Fatalf("expected canvas read --raw to parse, got %v", err)
	}
	if _, err := parser.Parse([]string{"canvas", "delete", "F123", "F456"}); err != nil {
		t.Fatalf("expected canvas delete to parse, got %v", err)
	}
}

func TestCanvasListUsesWorkspaceFromChannelURL(t *testing.T) {
	var usedToken string

	ctx := &Context{
		Config: &config.Config{
			CurrentWorkspace: "brianle.slack.com",
			Workspaces: map[string]config.WorkspaceAuth{
				"brianle.slack.com":             {Token: "brian-token"},
				"hostandhomecleaners.slack.com": {Token: "hhh-token"},
			},
		},
		ClientFactory: func(token string) *slack.Client {
			usedToken = token
			return slack.NewClientWithHTTPClient(
				token,
				&http.Client{
					Transport: dmRoundTripFunc(func(req *http.Request) (*http.Response, error) {
						switch req.URL.Path {
						case "/api/files.list":
							return dmJSONResponse(req, `{"ok":true,"files":[]}`)
						default:
							return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
						}
					}),
				},
			)
		},
	}

	if err := (&CanvasListCmd{
		Channel: "https://hostandhomecleaners.slack.com/archives/C06QF3A0TJM",
		Limit:   1,
	}).Run(ctx); err != nil {
		t.Fatalf("CanvasListCmd.Run returned error: %v", err)
	}

	if usedToken != "hhh-token" {
		t.Fatalf("expected workspace token from URL, got %q", usedToken)
	}
}
