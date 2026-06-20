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
	slack.DateFilterFlags
	Markdown bool `help:"Output as markdown" short:"m" xor:"format"`
	JSON     bool `help:"Output as pretty JSON array, parent first" short:"j" xor:"format"`
	JSONL    bool `help:"Output as JSON Lines, parent first" xor:"format"`
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

	filter, err := c.Resolve(time.Now())
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
		chRef := output.ChannelRefFromID(resolver, channelID, "")
		conv := output.MessageConverter{Resolver: resolver, Channel: chRef, Workspace: workspace}
		if c.JSONL {
			i := 0
			return output.EmitJSONLStream(func() (output.Message, bool, error) {
				if i >= len(replies.Messages) {
					return output.Message{}, false, nil
				}
				m := conv.Convert(replies.Messages[i])
				i++
				return m, true, nil
			})
		}
		records := make([]output.Message, 0, len(replies.Messages))
		for _, m := range replies.Messages {
			records = append(records, conv.Convert(m))
		}
		return output.EmitJSON(records)
	}

	if c.Markdown {
		fmt.Print(c.formatRepliesAsMarkdown(replies.Messages, resolver, threadTS))
		return nil
	}

	for _, msg := range replies.Messages {
		user := resolver.ResolveUser(msg.User)
		fmt.Printf("[%s] %s: %s\n", msg.TS, user, resolver.FormatText(msg.BodyText()))
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

	// If the first returned message is the thread parent, render it as the
	// root and the rest as quoted replies. Slack accepts reply timestamps for
	// conversations.replies and may still return the parent first, so this
	// checks the returned message shape rather than only the requested ts.
	hasParent := len(messages) > 0 && isThreadParentMessage(messages[0], threadTS)

	start := 0
	if hasParent {
		msg := messages[0]
		username := resolver.ResolveUser(msg.User)
		text := resolver.FormatText(msg.BodyText())
		fmt.Fprintf(&sb, "**%s** _%s_\n\n", username, msg.TS)
		fmt.Fprintf(&sb, "%s\n\n", text)
		if len(messages) > 1 {
			fmt.Fprintf(&sb, "---\n\n**%d replies**\n\n", len(messages)-1)
		}
		start = 1
	} else if len(messages) > 0 {
		fmt.Fprintf(&sb, "_Thread parent not returned; showing %d matching replies._\n\n", len(messages))
	}

	for _, msg := range messages[start:] {
		username := resolver.ResolveUser(msg.User)
		text := resolver.FormatText(msg.BodyText())

		fmt.Fprintf(&sb, "> **%s** _%s_\n>\n", username, msg.TS)
		for _, line := range strings.Split(text, "\n") {
			fmt.Fprintf(&sb, "> %s\n", line)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func isThreadParentMessage(msg slack.Message, requestedTS string) bool {
	if msg.TS == "" {
		return false
	}
	if msg.ThreadTS != "" {
		return msg.ThreadTS == msg.TS
	}
	if msg.ReplyCount > 0 {
		return true
	}
	return msg.TS == requestedTS
}
