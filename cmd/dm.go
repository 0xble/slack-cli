package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lox/slack-cli/internal/slack"
)

type DMCmd struct {
	List DmListCmd `cmd:"" aliases:"ls" help:"List direct messages"`
	Read DmReadCmd `cmd:"" aliases:"history,h" help:"Read direct message history"`
	Send DmSendCmd `cmd:"" help:"Send a direct message"`
}

type DmListCmd struct {
	Limit int `help:"Maximum number of direct messages to list" default:"100"`
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

	for _, ch := range resp.Channels {
		fmt.Println(formatDMConversationLabel(client, ch))
	}

	return nil
}

type DmReadCmd struct {
	Recipient string `arg:"" help:"Recipient @username, user ID, or DM ID"`
	Limit     int    `help:"Number of messages to show" default:"20"`
	Markdown  bool   `help:"Output as markdown" short:"m"`
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

type DmSendCmd struct {
	Recipient string `arg:"" help:"Recipient @username, user ID, or DM ID"`
	Text      string `arg:"" optional:"" help:"Message text"`
	Stdin     bool   `help:"Read message text from stdin"`
	Thread    string `help:"Reply in a thread"`
	Mrkdwn    bool   `help:"Send text as Slack mrkdwn"`
}

func (c *DmSendCmd) Run(ctx *Context) error {
	client, err := ctx.NewClient("")
	if err != nil {
		return err
	}

	text, err := c.messageText()
	if err != nil {
		return err
	}

	target, err := slack.ResolveDMTarget(client, c.Recipient)
	if err != nil {
		if slack.IsAPIError(err, "missing_scope") {
			return fmt.Errorf("%w. Update the Slack app scopes and rerun 'slack-cli auth login' for that workspace", err)
		}
		return err
	}

	resp, err := client.PostMessage(target.ChannelID, text, c.Thread, c.Mrkdwn)
	if err != nil {
		if slack.IsAPIError(err, "missing_scope") {
			return fmt.Errorf("%w. Update the Slack app scopes to include chat:write and im:write, then rerun 'slack-cli auth login' for that workspace", err)
		}
		return fmt.Errorf("failed to send DM: %w", err)
	}

	fmt.Printf("Sent DM to %s (%s) at %s\n", formatDMTargetLabel(target), resp.Channel, resp.TS)
	return nil
}

func (c *DmSendCmd) messageText() (string, error) {
	if c.Stdin && c.Text != "" {
		return "", fmt.Errorf("cannot use both message text argument and --stdin")
	}

	if c.Stdin {
		body, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read stdin: %w", err)
		}
		text := string(body)
		if strings.TrimSpace(text) == "" {
			return "", fmt.Errorf("message text is required")
		}
		return text, nil
	}

	if strings.TrimSpace(c.Text) == "" {
		return "", fmt.Errorf("message text is required")
	}

	return c.Text, nil
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

func formatDMTargetLabel(target *slack.DMTarget) string {
	if target == nil {
		return "recipient"
	}
	if target.User != nil && strings.TrimSpace(target.User.Name) != "" {
		return "@" + strings.TrimSpace(target.User.Name)
	}
	if target.UserID != "" {
		return target.UserID
	}
	if target.ChannelID != "" {
		return target.ChannelID
	}
	return target.Recipient
}
