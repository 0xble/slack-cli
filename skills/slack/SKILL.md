---
name: slack
description: Read Slack messages, threads, channels, users, files, canvases, and direct messages via CLI. Use when asked to view Slack URLs, search Slack, look up users, work with Slack files or canvases, or read/send DMs.
allowed-tools: Bash(slack-cli:*)
---

# Slack CLI

A CLI for reading Slack content, searching messages, browsing users, working with files, and basic direct message workflows.
A CLI for reading Slack content, searching messages, browsing users, working with files and canvases, and basic message send workflows.

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
slack-cli canvas list         # List recent canvases
slack-cli canvas read         # Read canvas content by ID
slack-cli canvas delete       # Delete canvases by ID
slack-cli dm list             # List direct messages
slack-cli dm read             # Read a direct message by @username, user ID, or DM ID
slack-cli message send        # Send a message to a channel or direct message
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
slack-cli channel read "#general" --limit 50
slack-cli channel read "https://workspace.slack.com/archives/C123" --markdown
```

### Upload a file

```bash
slack-cli file upload "#general" ./report.txt
slack-cli file upload @alice ./report.txt --comment "latest version"
```

### Filter by date

`search` supports calendar filters: `--after`, `--before`, and `--on`.
`channel read` and `thread read` also support rolling windows with `--last`.
Dates are interpreted in UTC and accept unambiguous calendar-day formats such
as `YYYY-MM-DD`, `YYYY/MM/DD`, `18 Apr 2026`, and `Apr 18 2026`. Timestamps,
partial dates, and other inputs with times are rejected.

```bash
slack-cli search "deploy" --after 2026-04-01 --before 2026-04-30
slack-cli search "incident" --on "Apr 18 2026"
slack-cli channel read "#general" --last 2w
slack-cli thread read "$URL" --on "18 Apr 2026" --json
```

### Machine-readable output (--json / --jsonl)

These commands support `--json` (pretty array or object) and `--jsonl` (one
record per line): `search`, `channel read`, `channel list`, `channel info`,
`file list`, `file info`, `thread read`, `user list`, `user info`.

Message records emit a full normalized shape for machines: `ts`,
`thread_ts` (when Slack provides it), `type`, `subtype` (when set, e.g.
`bot_message`, `channel_join`, `channel_archive`, `huddle_thread`),
`user`, `user_id`, `text` (resolver-formatted), `text_raw`, `channel`,
`workspace`, `permalink`, `reply_count`, and `files` when those fields are
available. When Slack channel metadata is available, `channel.type` is one
of `channel`, `private_channel`, `im`, or `mpim`.

```bash
slack-cli search "deploy" --limit 20 --jsonl | jq -c 'select(.channel.type == "channel")'
slack-cli channel read "#general" --limit 50 --json
slack-cli file list --limit 20 --jsonl
slack-cli thread read "$URL" --json
slack-cli channel list --json
slack-cli user list --json
slack-cli channel info C123 --json
```

### Read or send a DM

```bash
slack-cli dm read @username
slack-cli message send @username "hello"
```

## Discovering Options

To see available subcommands and flags, run `--help` on any command:

```bash
slack-cli --help
slack-cli view --help
slack-cli search --help
```

## Notes

- Use `--markdown` with `view`, `thread read`, or `channel read` when you need structured terminal output
- `file upload` accepts channel names, conversation IDs, `@username`, or `U123`
- Use `--json` / `--jsonl` for agent consumption; `--jsonl` pipes cleanly into `jq -c`
- Thread URLs with `thread_ts` parameter are automatically detected
- Channel names can include or omit the `#` prefix
- If you see `channel_not_found` and multiple workspaces are configured, retry with `--workspace <workspace>`
- User lookup accepts both user IDs (U123ABC) and email addresses
