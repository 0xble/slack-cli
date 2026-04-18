package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/lox/slack-cli/internal/output"
	"github.com/lox/slack-cli/internal/slack"
)

type SearchCmd struct {
	Query   string `arg:"" help:"Search query (supports Slack search syntax: from:@user, in:#channel, etc.)"`
	Limit   int    `help:"Maximum number of results" default:"20"`
	JSON    bool   `help:"Output as pretty JSON array" short:"j" xor:"format"`
	JSONL   bool   `help:"Output as JSON Lines, one match per line" xor:"format"`
	Verbose bool   `help:"Emit full JSON records (restore type and text_raw)" short:"V"`
	After   string `help:"Only match messages on or after DATE (YYYY-MM-DD, UTC)" xor:"after-last,after-on"`
	Before  string `help:"Only match messages on or before DATE (YYYY-MM-DD, UTC)" xor:"before-on"`
	On      string `help:"Only match messages on DATE (YYYY-MM-DD, UTC)" xor:"after-on,before-on,on-last"`
	Last    string `help:"Only match messages from the last DURATION (e.g. 45d, 12h, 2w)" xor:"after-last,on-last"`
}

func (c *SearchCmd) Run(ctx *Context) error {
	filter, err := slack.ResolveDateFilter(c.After, c.Before, c.On, c.Last, time.Now())
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

	if c.JSON || c.JSONL {
		return c.emitStructured(resolver, resp)
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

func (c *SearchCmd) emitStructured(resolver *slack.Resolver, resp *slack.SearchResponse) error {
	records := make([]output.Message, 0, len(resp.Messages.Matches))
	for _, match := range resp.Messages.Matches {
		records = append(records, searchMatchToMessage(resolver, match, c.Verbose))
	}

	if c.JSONL {
		return output.EmitJSONL(records)
	}
	return output.EmitJSON(records)
}

func searchMatchToMessage(resolver *slack.Resolver, match slack.SearchMatch, verbose bool) output.Message {
	var workspace string
	if match.Permalink != "" {
		host, _, err := slack.ExtractWorkspaceRef(match.Permalink)
		if err == nil {
			workspace = host
		}
	}

	display := match.Username
	if display == "" && match.User != "" {
		display = resolver.ResolveUser(match.User)
	}

	rec := output.Message{
		TS:        match.TS,
		Subtype:   match.Subtype,
		User:      display,
		UserID:    match.User,
		Text:      resolver.FormatText(match.Text),
		Channel:   output.ChannelRefFromID(resolver, match.Channel.ID, match.Channel.Name),
		Workspace: workspace,
		Permalink: match.Permalink,
	}
	if verbose {
		rec.Type = match.Type
		rec.TextRaw = match.Text
	}
	return rec
}
