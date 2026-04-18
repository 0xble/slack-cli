package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/lox/slack-cli/internal/slack"
)

type SearchCmd struct {
	Query string `arg:"" help:"Search query (supports Slack search syntax: from:@user, in:#channel, etc.)"`
	Limit int    `help:"Maximum number of results" default:"20"`
	slack.DateFilterFlags
}

func (c *SearchCmd) Run(ctx *Context) error {
	filter, err := c.DateFilterFlags.Resolve(time.Now())
	if err != nil {
		return err
	}

	if err := slack.ValidateSearchLast(c.Last); err != nil {
		return err
	}

	query := c.Query
	if !filter.IsZero() {
		if slack.QueryHasDateOperator(query) {
			return fmt.Errorf("query already contains an after:/before:/on:/during: operator; drop it or drop the flag")
		}
		if ops := filter.ToSearchOperators(); ops != "" {
			query = strings.TrimSpace(query + " " + ops)
		}
	}

	client, err := ctx.NewClient("")
	if err != nil {
		return err
	}
	resolver := slack.NewResolver(client)
	resp, err := client.SearchMessages(query, c.Limit)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if resp.Messages.Total == 0 {
		fmt.Println("No messages found.")
		return nil
	}

	fmt.Printf("Found %d messages:\n\n", resp.Messages.Total)

	for _, match := range resp.Messages.Matches {
		channel := match.Channel.Name
		if channel == "" {
			channel = match.Channel.ID
		}
		fmt.Printf("#%s [%s]\n", channel, match.TS)
		fmt.Printf("  %s: %s\n", match.Username, resolver.FormatText(match.Text))
		if match.Permalink != "" {
			fmt.Printf("  %s\n", match.Permalink)
		}
		fmt.Println()
	}

	return nil
}
