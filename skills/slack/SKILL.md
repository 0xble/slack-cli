---
name: slack
description: Read Slack messages, threads, channels, files, canvases, and direct messages via CLI. Use when asked to view Slack URLs, search Slack, look up Slack users, read/send DMs, or manage Slack files.
allowed-tools: Bash(slack-cli:*)
---

# Slack CLI

A CLI for reading Slack content plus basic message, file, and canvas workflows.

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
slack-cli dm list             # List direct messages
slack-cli dm read             # Read a direct message by @username, user ID, or DM ID
slack-cli file list           # List recent files
slack-cli file info           # Show file metadata
slack-cli file download       # Download a file by ID
slack-cli file upload         # Upload a file to a channel or direct message
slack-cli file delete         # Delete files by ID
slack-cli canvas list         # List recent canvases
slack-cli canvas read         # Read canvas content by ID
slack-cli canvas delete       # Delete canvases by ID
slack-cli message send        # Send a message to a channel or direct message
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

### Read or send a DM

```bash
slack-cli dm read @username
slack-cli message send @username "hello"
```

### Work with files and canvases

```bash
slack-cli file upload #general ./report.txt
slack-cli file download F123
slack-cli canvas list --channel #general
slack-cli canvas read F123
slack-cli canvas read F123 --raw
```

## Discovering Options

To see available subcommands and flags, run `--help` on any command:

```bash
slack-cli --help
slack-cli view --help
slack-cli search --help
```

### Machine-readable output (--json / --jsonl)

Most read, search, list, and info commands support `--json` (pretty array or
object) and `--jsonl` (one record per line). Message records share a common
shape with `ts`, `user`, `user_id`, `text` (formatted), `text_raw` (unresolved),
`channel.{id,name,type}`, `workspace`, and `permalink` (when available).

```bash
slack-cli search "deploy" --limit 20 --jsonl | jq -c 'select(.channel.type == "channel")'
slack-cli channel read #general --limit 50 --json
slack-cli thread read "$URL" --json
slack-cli channel list --json
slack-cli user list --json
slack-cli channel info C123 --json
```

## Notes

- Use `--markdown` with `view`, `thread read`, or `channel read` when you need structured terminal output
- Use `--json` / `--jsonl` for agent consumption; `--jsonl` pipes cleanly into `jq -c`
- Thread URLs with `thread_ts` parameter are automatically detected
- Channel names can include or omit the `#` prefix
- If you see `channel_not_found` and multiple workspaces are configured, retry with `--workspace <workspace>`
- User lookup accepts both user IDs (U123ABC) and email addresses
