package cmd

import (
	"fmt"
	"strings"

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

	history, err := client.GetConversationHistory(target.ChannelID, c.Limit)
	if err != nil {
		return fmt.Errorf("failed to get DM history: %w", err)
	}

	if c.JSON || c.JSONL {
		return c.emitStructured(resolver, history.Messages, target)
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

func (c *DmReadCmd) emitStructured(resolver *slack.Resolver, messages []slack.Message, target *slack.DMTarget) error {
	chRef := &output.ChannelRef{
		ID:   target.ChannelID,
		Type: "im",
	}
	conv := output.MessageConverter{Resolver: resolver, Channel: chRef}

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
