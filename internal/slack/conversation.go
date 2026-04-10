package slack

import (
	"fmt"
	"strings"
)

// ConversationTarget is a resolved Slack conversation ready for use as a
// destination (upload, message send, etc).
type ConversationTarget struct {
	Recipient   string
	ChannelID   string
	Name        string
	Username    string
	IsDM        bool
	IsPrivate   bool
	ChannelName string
	Type        string
	UserID      string
	User        *User
	Channel     *Channel
}

// ResolveConversationTarget accepts common recipient forms (channel ID, DM ID,
// user ID, @handle, #channel-name, or bare channel name) and returns a target
// whose ChannelID can be used as the Slack conversation destination.
func ResolveConversationTarget(client *Client, recipient string) (*ConversationTarget, error) {
	trimmed := strings.TrimSpace(recipient)
	if trimmed == "" {
		return nil, fmt.Errorf("recipient is required")
	}

	switch {
	case strings.HasPrefix(trimmed, "D"):
		return &ConversationTarget{
			Recipient: trimmed,
			ChannelID: trimmed,
			IsDM:      true,
			Type:      "dm",
		}, nil
	case isSlackUserID(trimmed):
		user, err := client.GetUserInfo(trimmed)
		if err != nil {
			return nil, err
		}
		return openConversationDMTarget(client, user, trimmed)
	case strings.HasPrefix(trimmed, "@"):
		user, err := findUserByUsername(client, trimmed)
		if err != nil {
			return nil, err
		}
		return openConversationDMTarget(client, user, trimmed)
	case strings.HasPrefix(trimmed, "C") || strings.HasPrefix(trimmed, "G"):
		channel, err := client.GetConversationInfo(trimmed)
		if err != nil {
			return nil, err
		}
		return targetFromChannel(trimmed, channel), nil
	default:
		return resolveChannelTargetByName(client, trimmed)
	}
}

func isSlackUserID(id string) bool {
	return strings.HasPrefix(id, "U") || strings.HasPrefix(id, "W")
}

func openConversationDMTarget(client *Client, user *User, recipient string) (*ConversationTarget, error) {
	resp, err := client.OpenConversation([]string{user.ID}, true)
	if err != nil {
		return nil, err
	}

	return &ConversationTarget{
		Recipient: recipient,
		ChannelID: resp.Channel.ID,
		Username:  user.Name,
		IsDM:      true,
		Type:      "dm",
		UserID:    user.ID,
		User:      user,
	}, nil
}

func resolveChannelTargetByName(client *Client, recipient string) (*ConversationTarget, error) {
	channelName, isChannelLike, err := parseChannelRecipient(recipient)
	if err != nil {
		return nil, err
	}
	if !isChannelLike {
		return nil, fmt.Errorf("recipient %q is not a channel", recipient)
	}

	cursor := ""
	for {
		resp, err := client.ListConversationsPage("public_channel,private_channel", 1000, cursor)
		if err != nil {
			return nil, fmt.Errorf("failed to list channels: %w", err)
		}

		for i := range resp.Channels {
			channel := &resp.Channels[i]
			if channel.Name == channelName {
				return targetFromChannel(recipient, channel), nil
			}
		}

		cursor = strings.TrimSpace(resp.ResponseMetadata.NextCursor)
		if cursor == "" {
			break
		}
	}

	return nil, &APIError{Method: "conversations.resolve", Code: "channel_not_found"}
}

func targetFromChannel(recipient string, channel *Channel) *ConversationTarget {
	targetType := "channel"
	if channel.IsPrivate || channel.IsGroup {
		targetType = "group"
	}

	return &ConversationTarget{
		Recipient:   recipient,
		ChannelID:   channel.ID,
		Name:        channel.Name,
		IsPrivate:   channel.IsPrivate,
		ChannelName: channel.Name,
		Type:        targetType,
		Channel:     channel,
	}
}

func parseChannelRecipient(recipient string) (channelName string, isChannelLike bool, err error) {
	trimmed := strings.TrimSpace(recipient)
	if trimmed == "" {
		return "", false, fmt.Errorf("recipient is required")
	}

	if strings.HasPrefix(trimmed, "#") {
		return strings.TrimPrefix(trimmed, "#"), true, nil
	}

	if strings.HasPrefix(trimmed, "@") || strings.HasPrefix(trimmed, "U") || strings.HasPrefix(trimmed, "W") || strings.HasPrefix(trimmed, "D") {
		return "", false, nil
	}

	if strings.HasPrefix(trimmed, "C") || strings.HasPrefix(trimmed, "G") {
		return "", false, nil
	}

	return trimmed, true, nil
}
