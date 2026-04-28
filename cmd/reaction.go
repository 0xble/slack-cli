package cmd

import (
	"fmt"
	"strings"

	"github.com/lox/slack-cli/internal/slack"
)

type ReactionCmd struct {
	Add ReactionAddCmd `cmd:"" help:"Add a reaction to a message"`
}

type ReactionAddCmd struct {
	URL   string `arg:"" help:"Slack message URL"`
	Emoji string `arg:"" help:"Emoji name, with or without surrounding colons"`
}

func (c *ReactionAddCmd) Run(ctx *Context) error {
	ref, err := slack.ParseMessageURL(c.URL)
	if err != nil {
		return err
	}

	emojiName, err := normalizeReactionName(c.Emoji)
	if err != nil {
		return err
	}

	client, err := ctx.NewClient(c.URL)
	if err != nil {
		return err
	}

	if _, err := client.AddReaction(ref.ChannelID, ref.Timestamp, emojiName); err != nil {
		if slack.IsAPIError(err, "missing_scope") {
			return fmt.Errorf("%w. Add the reactions:write Slack app scope and rerun 'slack-cli auth login' for that workspace", err)
		}
		return fmt.Errorf("failed to add reaction: %w", err)
	}

	fmt.Printf("Added :%s: reaction to %s at %s\n", emojiName, ref.ChannelID, ref.Timestamp)
	return nil
}

func normalizeReactionName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	name = strings.TrimPrefix(name, ":")
	name = strings.TrimSuffix(name, ":")
	name = strings.TrimSpace(strings.ToLower(name))

	switch name {
	case "":
		return "", fmt.Errorf("emoji name is required")
	case "👍", "thumbsup", "thumbs_up", "thumbs-up":
		return "+1", nil
	default:
		return name, nil
	}
}
