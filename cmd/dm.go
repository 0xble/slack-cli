package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/lox/slack-cli/internal/output"
	"github.com/lox/slack-cli/internal/slack"
)

type DMCmd struct {
	List DmListCmd `cmd:"" aliases:"ls" help:"List direct messages"`
	Read DmReadCmd `cmd:"" aliases:"history,h" help:"Read direct message history"`
}

type DmListCmd struct {
	Limit int  `help:"Maximum number of direct messages to list" default:"100"`
	JSON  bool `help:"Output as pretty JSON array" short:"j" xor:"format"`
	JSONL bool `help:"Output as JSON Lines, one DM per line" xor:"format"`
}

func (c *DmListCmd) Run(ctx *Context) error {
	client, err := ctx.NewClient("")
	if err != nil {
		return err
	}

	resp, err := client.ListConversations("im", c.Limit)
	if err != nil {
		return fmt.Errorf("failed to list direct messages: %w", err)
	}

	if c.JSON || c.JSONL {
		return c.emitStructured(client, resp.Channels)
	}

	for _, ch := range resp.Channels {
		fmt.Println(formatDMConversationLabel(client, ch))
	}

	return nil
}

func (c *DmListCmd) emitStructured(client *slack.Client, channels []slack.Channel) error {
	records := make([]output.Channel, 0, len(channels))
	for _, ch := range channels {
		rec := output.ToChannel(ch)
		if rec.UserID != "" {
			if user, err := client.GetUserInfo(rec.UserID); err == nil {
				rec.User = strings.TrimSpace(user.Name)
			}
		}
		records = append(records, rec)
	}

	if c.JSONL {
		return output.EmitJSONL(records)
	}
	return output.EmitJSON(records)
}

type DmReadCmd struct {
	Recipient string `arg:"" help:"Recipient @username, user ID, or DM ID"`
	Limit     int    `help:"Number of messages to show" default:"20"`
	Markdown  bool   `help:"Output as markdown" short:"m" xor:"format"`
	JSON      bool   `help:"Output as pretty JSON array, oldest first" short:"j" xor:"format"`
	JSONL     bool   `help:"Output as JSON Lines, oldest first" xor:"format"`
	Verbose   bool   `help:"Emit full JSON records (restore type, text_raw, and scope channel). Overrides default_json_mode." short:"V" xor:"detail"`
	Compact   bool   `help:"Emit trimmed JSON records (drop redundant fields). Overrides default_json_mode." short:"C" xor:"detail"`
	After     string `help:"Only show messages on or after DATE (YYYY-MM-DD, UTC)" xor:"after-last,after-on"`
	Before    string `help:"Only show messages on or before DATE (YYYY-MM-DD, UTC)" xor:"before-on"`
	On        string `help:"Only show messages on DATE (YYYY-MM-DD, UTC)" xor:"after-on,before-on,on-last"`
	Last      string `help:"Only show messages from the last DURATION (e.g. 45d, 12h, 2w)" xor:"after-last,on-last"`
}

func (c *DmReadCmd) Run(ctx *Context) error {
	client, err := ctx.NewClient("")
	if err != nil {
		return err
	}
	resolver := slack.NewResolver(client)

	target, err := slack.ResolveDMTarget(client, c.Recipient)
	if err != nil {
		return err
	}

	filter, err := slack.ResolveDateFilter(c.After, c.Before, c.On, c.Last, time.Now())
	if err != nil {
		return err
	}

	oldest, latest := filter.ToTimestampParams()
	history, err := client.GetConversationHistory(slack.HistoryParams{
		Channel:   target.ChannelID,
		Limit:     c.Limit,
		Oldest:    oldest,
		Latest:    latest,
		Inclusive: !filter.IsZero(),
	})
	if err != nil {
		return fmt.Errorf("failed to get DM history: %w", err)
	}

	if c.JSON || c.JSONL {
		return c.emitStructured(resolver, history.Messages, target, ctx.ResolveJSONVerbose(c.Verbose, c.Compact))
	}

	if c.Markdown {
		fmt.Print(formatMessagesAsMarkdown(history.Messages, resolver))
		return nil
	}

	for i := len(history.Messages) - 1; i >= 0; i-- {
		msg := history.Messages[i]
		user := resolver.ResolveUser(msg.User)
		fmt.Printf("[%s] %s: %s\n", msg.TS, user, resolver.FormatText(msg.Text))
	}

	return nil
}

func (c *DmReadCmd) emitStructured(resolver *slack.Resolver, messages []slack.Message, target *slack.DMTarget, verbose bool) error {
	chRef := &output.ChannelRef{
		ID:   target.ChannelID,
		Type: "im",
	}
	conv := output.MessageConverter{Resolver: resolver, Channel: chRef, Verbose: verbose}

	// Oldest first for stable timeline ordering (match text output).
	ordered := make([]slack.Message, len(messages))
	for i, m := range messages {
		ordered[len(messages)-1-i] = m
	}
	records := conv.ConvertAll(ordered)

	if c.JSONL {
		return output.EmitJSONL(records)
	}
	return output.EmitJSON(records)
}

func formatMessagesAsMarkdown(messages []slack.Message, resolver *slack.Resolver) string {
	var sb strings.Builder

	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		username := resolver.ResolveUser(msg.User)
		text := resolver.FormatText(msg.Text)

		fmt.Fprintf(&sb, "**%s** _%s_\n\n", username, msg.TS)
		fmt.Fprintf(&sb, "%s\n\n", text)
		if msg.ReplyCount > 0 {
			fmt.Fprintf(&sb, "_(%d replies)_\n\n", msg.ReplyCount)
		}
		if i > 0 {
			sb.WriteString("---\n\n")
		}
	}

	return sb.String()
}

func formatDMConversationLabel(client *slack.Client, ch slack.Channel) string {
	if ch.User == "" {
		return ch.ID
	}

	user, err := client.GetUserInfo(ch.User)
	if err != nil {
		return fmt.Sprintf("%s (%s)", ch.User, ch.ID)
	}

	username := strings.TrimSpace(user.Name)
	realName := strings.TrimSpace(user.RealName)

	if username != "" && realName != "" && username != realName {
		return fmt.Sprintf("@%s - %s (%s)", username, realName, ch.ID)
	}
	if username != "" {
		return fmt.Sprintf("@%s (%s)", username, ch.ID)
	}
	if realName != "" {
		return fmt.Sprintf("%s (%s)", realName, ch.ID)
	}
	return fmt.Sprintf("%s (%s)", ch.User, ch.ID)
}
