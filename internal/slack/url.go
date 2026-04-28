package slack

import (
	"fmt"
	"net/url"
	"strings"
)

type MessageURLRef struct {
	WorkspaceHost string
	TeamID        string
	ChannelID     string
	Timestamp     string
}

// ExtractWorkspaceRef returns workspace host and/or team ID from a Slack URL.
func ExtractWorkspaceRef(rawURL string) (workspaceHost string, teamID string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", fmt.Errorf("invalid URL: %w", err)
	}

	host := strings.ToLower(u.Host)
	if host != "slack.com" && !strings.HasSuffix(host, ".slack.com") {
		return "", "", fmt.Errorf("not a Slack URL")
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 2 && parts[0] == "client" && strings.HasPrefix(parts[1], "T") {
		teamID = parts[1]
	}

	if host != "slack.com" && host != "app.slack.com" {
		workspaceHost = host
	}

	return workspaceHost, teamID, nil
}

func ParseMessageURL(rawURL string) (*MessageURLRef, error) {
	workspaceHost, teamID, err := ExtractWorkspaceRef(rawURL)
	if err != nil {
		return nil, err
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, part := range parts {
		if part != "archives" || i+2 >= len(parts) {
			continue
		}

		channelID := strings.TrimSpace(parts[i+1])
		timestamp, ok := parseSlackPathTimestamp(parts[i+2])
		if channelID == "" || !ok {
			break
		}

		return &MessageURLRef{
			WorkspaceHost: workspaceHost,
			TeamID:        teamID,
			ChannelID:     channelID,
			Timestamp:     timestamp,
		}, nil
	}

	return nil, fmt.Errorf("could not parse message URL: %s", rawURL)
}

func parseSlackPathTimestamp(pathPart string) (string, bool) {
	ts := strings.TrimSpace(pathPart)
	if !strings.HasPrefix(ts, "p") {
		return "", false
	}
	ts = strings.TrimPrefix(ts, "p")
	if len(ts) < 11 {
		return "", false
	}
	return ts[:10] + "." + ts[10:], true
}
