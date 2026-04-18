package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/lox/slack-cli/internal/output"
	"github.com/lox/slack-cli/internal/slack"
)

type ThreadCmd struct {
	Read ThreadReadCmd `cmd:"" help:"Read a thread by URL or channel+timestamp"`
}

type ThreadReadCmd struct {
	URL       string `arg:"" optional:"" help:"Thread URL (e.g., https://workspace.slack.com/archives/C123/p1234567890)"`
	Channel   string `help:"Channel ID" short:"c"`
	Timestamp string `help:"Thread timestamp" short:"t"`
	Limit     int    `help:"Maximum number of replies" default:"100"`
	Markdown  bool   `help:"Output as markdown" short:"m" xor:"format"`
	JSON      bool   `help:"Output as pretty JSON array, parent first" short:"j" xor:"format"`
	JSONL     bool   `help:"Output as JSON Lines, parent first" xor:"format"`
	Verbose   bool   `help:"Emit full JSON records (restore type, text_raw, scope channel, and scope thread_ts). Overrides default_json_mode." short:"V" xor:"detail"`
	Compact   bool   `help:"Emit trimmed JSON records (drop redundant fields). Overrides default_json_mode." short:"C" xor:"detail"`
	After     string `help:"Only show replies on or after DATE (YYYY-MM-DD, UTC)" xor:"after-last,after-on"`
	Before    string `help:"Only show replies on or before DATE (YYYY-MM-DD, UTC)" xor:"before-on"`
	On        string `help:"Only show replies on DATE (YYYY-MM-DD, UTC)" xor:"after-on,before-on,on-last"`
	Last      string `help:"Only show replies from the last DURATION (e.g. 45d, 12h, 2w)" xor:"after-last,on-last"`
}

func (c *ThreadReadCmd) Run(ctx *Context) error {
	var channelID, threadTS string
	var err error

	if c.URL != "" {
		channelID, threadTS, err = slack.ParseThreadURL(c.URL)
		if err != nil {
			return fmt.Errorf("failed to parse thread URL: %w", err)
		}
	} else if c.Channel != "" && c.Timestamp != "" {
		channelID = c.Channel
		threadTS = c.Timestamp
	} else {
		return fmt.Errorf("provide either a thread URL or --channel and --timestamp")
	}

	filter, err := slack.ResolveDateFilter(c.After, c.Before, c.On, c.Last, time.Now())
	if err != nil {
		return err
	}

	client, err := ctx.NewClient(c.URL)
	if err != nil {
		return err
	}
	resolver := slack.NewResolver(client)

	oldest, latest := filter.ToTimestampParams()
	replies, err := client.GetConversationReplies(slack.RepliesParams{
		Channel:   channelID,
		ThreadTS:  threadTS,
		Limit:     c.Limit,
		Oldest:    oldest,
		Latest:    latest,
		Inclusive: !filter.IsZero(),
	})
	if err != nil {
		err = c.augmentReadError(ctx, err)
		return fmt.Errorf("failed to get thread: %w", err)
	}

	if c.JSON || c.JSONL {
		var workspace string
		if c.URL != "" {
			if host, _, herr := slack.ExtractWorkspaceRef(c.URL); herr == nil {
				workspace = host
			}
		}
		verbose := ctx.ResolveJSONVerbose(c.Verbose, c.Compact)
		chRef := output.ChannelRefFromID(resolver, channelID, "")
		conv := output.MessageConverter{Resolver: resolver, Channel: chRef, Workspace: workspace, Verbose: verbose}
		records := conv.ConvertAll(replies.Messages)
		if !verbose {
			// thread_ts on every record just restates the command scope; drop it
			// so compact output stays focused on per-reply signal.
			for i := range records {
				records[i].ThreadTS = ""
			}
		}
		if c.JSONL {
			return output.EmitJSONL(records)
		}
		return output.EmitJSON(records)
	}

	if c.Markdown {
		fmt.Print(c.formatRepliesAsMarkdown(replies.Messages, resolver, threadTS))
		return nil
	}

	for _, msg := range replies.Messages {
		user := resolver.ResolveUser(msg.User)
		fmt.Printf("[%s] %s: %s\n", msg.TS, user, resolver.FormatText(msg.Text))
	}

	return nil
}

func (c *ThreadReadCmd) augmentReadError(ctx *Context, err error) error {
	err = ctx.augmentChannelNotFoundError(c.URL, err)
	err = ctx.augmentCrossWorkspaceChannelHint(c.URL, err)
	return err
}

func (c *ThreadReadCmd) formatRepliesAsMarkdown(messages []slack.Message, resolver *slack.Resolver, threadTS string) string {
	var sb strings.Builder

	// If the first message is the thread parent, render it as the root
	// and the rest as quoted replies. If a date filter excluded the
	// parent (messages[0].TS != threadTS), render everything as replies
	// with a note so the count and block labels stay accurate.
	hasParent := len(messages) > 0 && messages[0].TS == threadTS

	start := 0
	if hasParent {
		msg := messages[0]
		username := resolver.ResolveUser(msg.User)
		text := resolver.FormatText(msg.Text)
		fmt.Fprintf(&sb, "**%s** _%s_\n\n", username, msg.TS)
		fmt.Fprintf(&sb, "%s\n\n", text)
		if len(messages) > 1 {
			fmt.Fprintf(&sb, "---\n\n**%d replies**\n\n", len(messages)-1)
		}
		start = 1
	} else if len(messages) > 0 {
		fmt.Fprintf(&sb, "_Thread parent filtered out; showing %d matching replies._\n\n", len(messages))
	}

	for _, msg := range messages[start:] {
		username := resolver.ResolveUser(msg.User)
		text := resolver.FormatText(msg.Text)

		fmt.Fprintf(&sb, "> **%s** _%s_\n>\n", username, msg.TS)
		for _, line := range strings.Split(text, "\n") {
			fmt.Fprintf(&sb, "> %s\n", line)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
