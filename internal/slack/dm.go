package slack

import (
	"fmt"
	"strings"
)

// findUserByUsername walks users.list with cursor pagination until it finds
// a user whose Name matches handle (case-insensitive, optional leading @).
func findUserByUsername(client *Client, handle string) (*User, error) {
	needle := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(handle)), "@")

	cursor := ""
	for {
		users, err := client.ListUsersPage(1000, cursor)
		if err != nil {
			return nil, fmt.Errorf("failed to list users: %w", err)
		}

		for _, user := range users.Members {
			if strings.ToLower(strings.TrimSpace(user.Name)) == needle {
				matched := user
				return &matched, nil
			}
		}

		cursor = strings.TrimSpace(users.ResponseMetadata.NextCursor)
		if cursor == "" {
			break
		}
	}

	return nil, fmt.Errorf("user not found: %s", handle)
}
