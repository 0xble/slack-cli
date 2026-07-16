package slack

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

type UserGroup struct {
	ID     string `json:"id"`
	Handle string `json:"handle"`
	Name   string `json:"name"`
}

type UserGroupsResponse struct {
	OK         bool        `json:"ok"`
	UserGroups []UserGroup `json:"usergroups"`
}

var userGroupHandlePattern = regexp.MustCompile(`(^|[\s(\[])@([A-Za-z0-9._-]+)\b`)

func (c *Client) ListUserGroups() (*UserGroupsResponse, error) {
	body, err := c.request("usergroups.list", nil)
	if err != nil {
		return nil, err
	}
	var result UserGroupsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse usergroups.list response: %w", err)
	}
	return &result, nil
}

// ResolveUserGroupMentions replaces known @handle references with Slack's
// canonical subteam token. Unknown handles and email-address fragments remain
// unchanged so message text is not silently corrupted.
func (c *Client) ResolveUserGroupMentions(text string) (string, error) {
	if !userGroupHandlePattern.MatchString(text) {
		return text, nil
	}
	groups, err := c.ListUserGroups()
	if err != nil {
		return "", err
	}
	byHandle := make(map[string]string, len(groups.UserGroups))
	for _, group := range groups.UserGroups {
		if group.ID != "" && group.Handle != "" {
			byHandle[strings.ToLower(group.Handle)] = group.ID
		}
	}
	resolved, _ := ResolveUserGroupMentionsFromMap(text, byHandle)
	return resolved, nil
}

func ResolveUserGroupMentionsFromMap(text string, byHandle map[string]string) (string, int) {
	count := 0
	resolved := userGroupHandlePattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := userGroupHandlePattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		id := byHandle[strings.ToLower(parts[2])]
		if id == "" {
			return match
		}
		count++
		return parts[1] + "<!subteam^" + id + ">"
	})
	return resolved, count
}
