package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/lox/slack-cli/internal/slack"
)

type MessageCmd struct {
	Send MessageSendCmd `cmd:"" help:"Send a message to a channel or direct message"`
}

type MessageSendCmd struct {
	Recipient string `arg:"" help:"Recipient #channel, channel name, channel ID, @username, user ID, or DM ID"`
	Text      string `arg:"" optional:"" help:"Message text"`
	Stdin     bool   `help:"Read message text from stdin"`
	Thread    string `help:"Reply in a thread"`
	Mrkdwn    bool   `help:"Send text as Slack mrkdwn"`
}

func (c *MessageSendCmd) Run(ctx *Context) error {
	client, err := ctx.NewClient("")
	if err != nil {
		return err
	}

	text, err := c.messageText()
	if err != nil {
		return err
	}

	target, err := slack.ResolveConversationTarget(client, c.Recipient)
	if err != nil {
		err = ctx.augmentChannelNotFoundError("", err)
		err = ctx.augmentCrossWorkspaceChannelHint("", err)
		if slack.IsAPIError(err, "missing_scope") {
			return fmt.Errorf("%w. Update the Slack app scopes and rerun 'slack-cli auth login' for that workspace", err)
		}
		return err
	}

	resp, err := client.PostMessage(target.ChannelID, text, c.Thread, c.Mrkdwn)
	if err != nil {
		err = ctx.augmentChannelNotFoundError("", err)
		err = ctx.augmentCrossWorkspaceChannelHint("", err)
		if slack.IsAPIError(err, "missing_scope") {
			return fmt.Errorf("%w. Update the Slack app scopes and rerun 'slack-cli auth login' for that workspace", err)
		}
		return fmt.Errorf("failed to send message: %w", err)
	}

	fmt.Printf("Sent message to %s (%s) at %s\n", formatConversationTargetLabel(target), resp.Channel, resp.TS)
	return nil
}

func (c *MessageSendCmd) messageText() (string, error) {
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

func formatConversationTargetLabel(target *slack.ConversationTarget) string {
	if target == nil {
		return "recipient"
	}
	if target.Type == "dm" {
		if target.User != nil && strings.TrimSpace(target.User.Name) != "" {
			return "@" + strings.TrimSpace(target.User.Name)
		}
		if target.UserID != "" {
			return target.UserID
		}
	}
	if strings.TrimSpace(target.ChannelName) != "" {
		return "#" + strings.TrimSpace(target.ChannelName)
	}
	if target.ChannelID != "" {
		return target.ChannelID
	}
	return target.Recipient
}
