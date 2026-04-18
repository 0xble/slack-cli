package cmd

import (
	"fmt"

	"github.com/lox/slack-cli/internal/output"
	"github.com/lox/slack-cli/internal/slack"
)

type SearchCmd struct {
	Query   string `arg:"" help:"Search query (supports Slack search syntax: from:@user, in:#channel, etc.)"`
	Limit   int    `help:"Maximum number of results" default:"20"`
	JSON    bool   `help:"Output as pretty JSON array" short:"j" xor:"format"`
	JSONL   bool   `help:"Output as JSON Lines, one match per line" xor:"format"`
	Verbose bool   `help:"Emit full JSON records (restore type and text_raw). Overrides default_json_mode." short:"V" xor:"detail"`
	Compact bool   `help:"Emit trimmed JSON records (drop redundant fields). Overrides default_json_mode." short:"C" xor:"detail"`
}

func (c *SearchCmd) Run(ctx *Context) error {
	client, err := ctx.NewClient("")
	if err != nil {
		return err
	}
	resolver := slack.NewResolver(client)
	// Preload the channel list in one call so per-match channel.type
	// population on the JSON path does not fan out to N+1
	// conversations.info requests across N distinct result channels.
	if c.JSON || c.JSONL {
		resolver.PreloadChannels("public_channel,private_channel,mpim,im")
	}
	resp, err := client.SearchMessages(c.Query, c.Limit)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if c.JSON || c.JSONL {
		return c.emitStructured(resolver, resp, ctx.ResolveJSONVerbose(c.Verbose, c.Compact))
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

func (c *SearchCmd) emitStructured(resolver *slack.Resolver, resp *slack.SearchResponse, verbose bool) error {
	if c.JSONL {
		i := 0
		return output.EmitJSONLStream(func() (output.Message, bool, error) {
			if i >= len(resp.Messages.Matches) {
				return output.Message{}, false, nil
			}
			m := searchMatchToMessage(resolver, resp.Messages.Matches[i], verbose)
			i++
			return m, true, nil
		})
	}

	records := make([]output.Message, 0, len(resp.Messages.Matches))
	for _, match := range resp.Messages.Matches {
		records = append(records, searchMatchToMessage(resolver, match, verbose))
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
	if display == "" && match.User != "" && resolver != nil {
		display = resolver.ResolveUser(match.User)
	}

	text := match.Text
	if resolver != nil {
		text = resolver.FormatText(match.Text)
	}

	rec := output.Message{
		TS:        match.TS,
		Subtype:   match.Subtype,
		User:      display,
		UserID:    match.User,
		Text:      text,
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
