package slack

import (
	"fmt"
	"strings"
)

type DMTarget struct {
	Recipient string
	UserID    string
	ChannelID string
	User      *User
}

func ResolveDMTarget(client *Client, recipient string) (*DMTarget, error) {
	trimmed := strings.TrimSpace(recipient)
	if trimmed == "" {
		return nil, fmt.Errorf("recipient is required")
	}

	if strings.HasPrefix(trimmed, "#") {
		return nil, fmt.Errorf("recipient %q looks like a channel; use dm commands with @username, U123, or D123", recipient)
	}

	if strings.HasPrefix(trimmed, "D") {
		return &DMTarget{
			Recipient: trimmed,
			ChannelID: trimmed,
		}, nil
	}

	if strings.HasPrefix(trimmed, "U") {
		user, err := client.GetUserInfo(trimmed)
		if err != nil {
			return nil, fmt.Errorf("failed to look up user %s: %w", trimmed, err)
		}
		return openDMForUser(client, user, trimmed)
	}

	if strings.HasPrefix(trimmed, "@") {
		username := strings.TrimPrefix(trimmed, "@")
		user, err := findUserByUsername(client, username)
		if err != nil {
			return nil, err
		}
		return openDMForUser(client, user, trimmed)
	}

	return nil, fmt.Errorf("recipient %q must be @username, U123, or D123", recipient)
}

func openDMForUser(client *Client, user *User, recipient string) (*DMTarget, error) {
	resp, err := client.OpenConversation([]string{user.ID}, true)
	if err != nil {
		return nil, fmt.Errorf("failed to open DM for %s: %w", recipient, err)
	}

	return &DMTarget{
		Recipient: recipient,
		UserID:    user.ID,
		ChannelID: resp.Channel.ID,
		User:      user,
	}, nil
}

func findUserByUsername(client *Client, username string) (*User, error) {
	normalized := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(username, "@")))
	if normalized == "" {
		return nil, fmt.Errorf("username is required")
	}

	resp, err := client.ListUsers(1000)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	for _, user := range resp.Members {
		if user.Deleted || user.IsBot {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(user.Name), normalized) {
			matched := user
			return &matched, nil
		}
	}

	return nil, fmt.Errorf("user @%s not found", normalized)
}
