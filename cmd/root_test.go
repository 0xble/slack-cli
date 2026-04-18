package cmd

import (
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/lox/slack-cli/internal/config"
)

func TestResolveTokenFallsBackToDefaultForUnmappedURLWorkspace(t *testing.T) {
	ctx := &Context{
		Config: &config.Config{
			CurrentWorkspace: "default",
			Workspaces: map[string]config.WorkspaceAuth{
				"default": {Token: "legacy-token"},
			},
		},
	}

	token, err := ctx.resolveToken("https://buildkite.slack.com/archives/C123/p1234567890123456")
	if err != nil {
		t.Fatalf("resolveToken returned error: %v", err)
	}
	if token != "legacy-token" {
		t.Fatalf("expected legacy-token fallback, got %q", token)
	}
}

func TestResolveTokenExplicitWorkspaceStillErrors(t *testing.T) {
	ctx := &Context{
		Workspace: "missing.slack.com",
		Config: &config.Config{
			CurrentWorkspace: "default",
			Workspaces: map[string]config.WorkspaceAuth{
				"default": {Token: "legacy-token"},
			},
		},
	}

	_, err := ctx.resolveToken("")
	if err == nil {
		t.Fatalf("expected error for unknown explicit workspace")
	}
	if !strings.Contains(err.Error(), "Run 'slack-cli auth login' for that workspace") {
		t.Fatalf("expected slack-cli auth hint, got %q", err.Error())
	}
}

func TestResolveTokenUnmappedURLWorkspaceDoesNotFallbackWhenMultipleWorkspacesConfigured(t *testing.T) {
	ctx := &Context{
		Config: &config.Config{
			CurrentWorkspace: "buildkite.slack.com",
			Workspaces: map[string]config.WorkspaceAuth{
				"buildkite.slack.com":      {Token: "buildkite-token"},
				"buildkite-corp.slack.com": {Token: "corp-token"},
			},
		},
	}

	_, err := ctx.resolveToken("https://missing.slack.com/archives/C123/p1234567890123456")
	if err == nil {
		t.Fatalf("expected error for unmapped URL workspace")
	}
	if !strings.Contains(err.Error(), "Run 'slack-cli auth login' for that workspace") {
		t.Fatalf("expected workspace auth hint, got %q", err.Error())
	}
}

func TestThreadReadMarkdownFlagParses(t *testing.T) {
	cli := &CLI{}
	parser, err := kong.New(cli, kong.Vars{"version": "test"})
	if err != nil {
		t.Fatalf("failed to build parser: %v", err)
	}

	_, err = parser.Parse([]string{"thread", "read", "https://buildkite.slack.com/archives/C123/p1773973307481399", "--markdown"})
	if err != nil {
		t.Fatalf("expected --markdown to parse for thread read, got %v", err)
	}
}

func TestResolveJSONVerboseFlagsWin(t *testing.T) {
	ctx := &Context{Config: &config.Config{DefaultJSONMode: "verbose"}}

	if !ctx.ResolveJSONVerbose(true, false) {
		t.Fatalf("--verbose must force verbose even when config says verbose")
	}
	if ctx.ResolveJSONVerbose(false, true) {
		t.Fatalf("--compact must force compact even when config says verbose")
	}
}

func TestResolveJSONVerboseFallsBackToConfig(t *testing.T) {
	verbose := &Context{Config: &config.Config{DefaultJSONMode: "verbose"}}
	if !verbose.ResolveJSONVerbose(false, false) {
		t.Fatalf("expected verbose from config default")
	}

	compact := &Context{Config: &config.Config{DefaultJSONMode: "compact"}}
	if compact.ResolveJSONVerbose(false, false) {
		t.Fatalf("expected compact from config default")
	}

	empty := &Context{Config: &config.Config{}}
	if empty.ResolveJSONVerbose(false, false) {
		t.Fatalf("expected compact when config default is empty")
	}

	nilCfg := &Context{}
	if nilCfg.ResolveJSONVerbose(false, false) {
		t.Fatalf("expected compact when config is nil")
	}
}
