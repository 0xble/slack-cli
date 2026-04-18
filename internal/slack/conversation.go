package slack

import (
	"fmt"
	"strings"
)

// ConversationTarget is a resolved Slack conversation ready for use as a
// destination (upload, message send, etc). Name is populated for real
// channels; Username is populated for DMs.
type ConversationTarget struct {
	ChannelID string
	Name      string
	Username  string
	IsDM      bool
	IsPrivate bool
}

// ResolveConversationTarget accepts the common recipient forms (channel ID,
// DM ID, user ID, @handle, #channel-name, or bare channel name) and returns
// a ConversationTarget whose ChannelID can be used as the Slack conversation
// destination. For @handle and user ID inputs it opens a DM channel.
func ResolveConversationTarget(client *Client, recipient string) (*ConversationTarget, error) {
	trimmed := strings.TrimSpace(recipient)
	if trimmed == "" {
		return nil, fmt.Errorf("recipient is required")
	}

	switch {
	case strings.HasPrefix(trimmed, "D"):
		return &ConversationTarget{ChannelID: trimmed, IsDM: true}, nil
	case strings.HasPrefix(trimmed, "U"):
		user, err := client.GetUserInfo(trimmed)
		if err != nil {
			return nil, err
		}
		return openDMTarget(client, user)
	case strings.HasPrefix(trimmed, "@"):
		user, err := findUserByUsername(client, trimmed)
		if err != nil {
			return nil, err
		}
		return openDMTarget(client, user)
	case strings.HasPrefix(trimmed, "C") || strings.HasPrefix(trimmed, "G"):
		info, err := client.GetConversationInfo(trimmed)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(info.Name)
		return &ConversationTarget{
			ChannelID: trimmed,
			Name:      name,
			IsPrivate: info.IsPrivate,
		}, nil
	default:
		return resolveChannelTargetByName(client, trimmed)
	}
}

func openDMTarget(client *Client, user *User) (*ConversationTarget, error) {
	resp, err := client.OpenConversation([]string{user.ID}, true)
	if err != nil {
		return nil, err
	}
	return &ConversationTarget{
		ChannelID: resp.Channel.ID,
		Username:  user.Name,
		IsDM:      true,
	}, nil
}

// resolveChannelTargetByName walks conversations.list with cursor
// pagination until it finds a channel whose name matches recipient.
func resolveChannelTargetByName(client *Client, recipient string) (*ConversationTarget, error) {
	channelName := strings.TrimPrefix(strings.TrimSpace(recipient), "#")

	cursor := ""
	for {
		channels, err := client.ListConversationsPage("public_channel,private_channel", 1000, cursor)
		if err != nil {
			return nil, fmt.Errorf("failed to list channels: %w", err)
		}

		for _, channel := range channels.Channels {
			if channel.Name != channelName {
				continue
			}
			return &ConversationTarget{
				ChannelID: channel.ID,
				Name:      channel.Name,
				IsPrivate: channel.IsPrivate,
			}, nil
		}

		cursor = strings.TrimSpace(channels.ResponseMetadata.NextCursor)
		if cursor == "" {
			break
		}
	}

	return nil, &APIError{Method: "conversations.resolve", Code: "channel_not_found"}
}
