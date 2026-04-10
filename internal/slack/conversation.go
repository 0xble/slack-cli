package slack

import (
	"fmt"
	"strings"
)

type ConversationTarget struct {
	Recipient   string
	ChannelID   string
	ChannelName string
	Type        string
	UserID      string
	User        *User
	Channel     *Channel
}

func ResolveConversationTarget(client *Client, recipient string) (*ConversationTarget, error) {
	target, err := ResolveDMTarget(client, recipient)
	if err == nil {
		return &ConversationTarget{
			Recipient: recipient,
			ChannelID: target.ChannelID,
			Type:      "dm",
			UserID:    target.UserID,
			User:      target.User,
		}, nil
	}

	channelName, isChannelLike, parseErr := parseChannelRecipient(recipient)
	if parseErr != nil {
		return nil, parseErr
	}
	if isChannelLike {
		return resolveChannelTargetByName(client, recipient, channelName)
	}

	channel, err := client.GetConversationInfo(recipient)
	if err != nil {
		return nil, err
	}

	targetType := "channel"
	if channel.IsPrivate {
		targetType = "group"
	}

	return &ConversationTarget{
		Recipient:   recipient,
		ChannelID:   channel.ID,
		ChannelName: channel.Name,
		Type:        targetType,
		Channel:     channel,
	}, nil
}

func resolveChannelTargetByName(client *Client, recipient, channelName string) (*ConversationTarget, error) {
	resp, err := client.ListConversations("public_channel,private_channel", 1000)
	if err != nil {
		return nil, fmt.Errorf("failed to list channels: %w", err)
	}

	for _, ch := range resp.Channels {
		if ch.Name != channelName {
			continue
		}

		targetType := "channel"
		if ch.IsPrivate {
			targetType = "group"
		}

		matched := ch
		return &ConversationTarget{
			Recipient:   recipient,
			ChannelID:   ch.ID,
			ChannelName: ch.Name,
			Type:        targetType,
			Channel:     &matched,
		}, nil
	}

	return nil, &APIError{Method: "conversations.resolve", Code: "channel_not_found"}
}

func parseChannelRecipient(recipient string) (channelName string, isChannelLike bool, err error) {
	trimmed := strings.TrimSpace(recipient)
	if trimmed == "" {
		return "", false, fmt.Errorf("recipient is required")
	}

	if strings.HasPrefix(trimmed, "#") {
		return strings.TrimPrefix(trimmed, "#"), true, nil
	}

	if strings.HasPrefix(trimmed, "@") || strings.HasPrefix(trimmed, "U") || strings.HasPrefix(trimmed, "D") {
		return "", false, nil
	}

	if strings.HasPrefix(trimmed, "C") || strings.HasPrefix(trimmed, "G") {
		return "", false, nil
	}

	return trimmed, true, nil
}
