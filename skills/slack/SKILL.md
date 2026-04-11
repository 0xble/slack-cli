---
name: slack
description: Read Slack messages, threads, and channels via CLI. Use when asked to view Slack URLs, search Slack, or look up Slack users.
allowed-tools: Bash(slack-cli:*)
---

# Slack CLI

A CLI for reading Slack content, searching messages, browsing users, and working with files.

## Installation

If `slack-cli` is not on PATH, install it:

```bash
brew install --cask lox/tap/slack-cli
```

Or: `go install github.com/lox/slack-cli@latest`

See https://github.com/lox/slack-cli for setup instructions (Slack app creation and OAuth).

## Available Commands

```
slack-cli view <url>          # View any Slack URL (message, thread, or channel)
slack-cli search <query>      # Search messages
slack-cli channel list        # List channels you're a member of
slack-cli channel read        # Read recent messages from a channel name, ID, or URL
slack-cli channel info        # Show channel information by name, ID, or URL
slack-cli file list           # List recent files
slack-cli file info           # Show file metadata
slack-cli file download       # Download a file by ID
slack-cli file upload         # Upload and share a file
slack-cli file delete         # Delete a file by ID
slack-cli thread read         # Read a thread by URL or channel+timestamp (supports --markdown)
slack-cli user list           # List users in the workspace
slack-cli user info           # Show user information
slack-cli auth login          # Authenticate with Slack via OAuth
slack-cli auth status         # Show authentication status
```

## Common Patterns

### View a Slack URL the user shared

```bash
slack-cli view "https://workspace.slack.com/archives/C123/p1234567890" --markdown
```

### Search for messages

```bash
slack-cli search "from:@username keyword"
slack-cli search "in:#channel-name keyword"
```

### Read a channel

```bash
slack-cli channel read #general --limit 50
slack-cli channel read "https://workspace.slack.com/archives/C123" --markdown
```

### Upload a file

```bash
slack-cli file upload #general ./report.txt
slack-cli file upload @alice ./report.txt --comment "latest version"
```

## Discovering Options

To see available subcommands and flags, run `--help` on any command:

```bash
slack-cli --help
slack-cli view --help
slack-cli search --help
```

## Notes

- Use `--markdown` with `view`, `thread read`, or `channel read` when you need structured output
- `file upload` accepts channel names, conversation IDs, `@username`, or `U123`
- Thread URLs with `thread_ts` parameter are automatically detected
- Channel names can include or omit the `#` prefix
- If you see `channel_not_found` and multiple workspaces are configured, retry with `--workspace <workspace>`
- User lookup accepts both user IDs (U123ABC) and email addresses
