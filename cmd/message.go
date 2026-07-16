package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/lox/slack-cli/internal/slack"
)

const (
	maxBlocksPayloadBytes = 1 << 20
	maxMessageBlocks      = 50
)

type MessageCmd struct {
	Send   MessageSendCmd   `cmd:"" help:"Send a message to a channel or direct message"`
	Update MessageUpdateCmd `cmd:"" help:"Update a message you previously sent"`
}

type MessageSendCmd struct {
	Recipient   string `arg:"" help:"Recipient #channel, channel name, channel ID, @username, user ID, or DM ID"`
	Text        string `arg:"" optional:"" help:"Message text"`
	Stdin       bool   `help:"Read message text from stdin"`
	Thread      string `help:"Reply in a thread"`
	Mrkdwn      bool   `help:"Send text as Slack mrkdwn" xor:"render"`
	Rich        bool   `help:"Convert Markdown to native Slack rich text and lists" xor:"render"`
	Blocks      string `help:"Inline Block Kit JSON array"`
	BlocksFile  string `help:"Read Block Kit JSON array from a file" type:"existingfile"`
	BlocksStdin bool   `help:"Read Block Kit JSON array from stdin"`
	DryRun      bool   `help:"Print the exact Slack payload without sending"`
	JSON        bool   `help:"Output the send result, permalink, and returned blocks as JSON" short:"j"`
}

type MessageUpdateCmd struct {
	Channel     string `arg:"" help:"Channel name, channel ID, or DM ID"`
	Timestamp   string `arg:"" help:"Timestamp of the message to update"`
	Thread      string `help:"Parent thread timestamp when updating a reply"`
	Text        string `arg:"" optional:"" help:"Replacement message text"`
	Stdin       bool   `help:"Read replacement text from stdin"`
	Rich        bool   `help:"Convert Markdown to native Slack rich text and lists"`
	Blocks      string `help:"Inline Block Kit JSON array"`
	BlocksFile  string `help:"Read Block Kit JSON array from a file" type:"existingfile"`
	BlocksStdin bool   `help:"Read Block Kit JSON array from stdin"`
	DryRun      bool   `help:"Print the exact Slack payload without updating"`
	JSON        bool   `help:"Output the update result, permalink, and returned blocks as JSON" short:"j"`
}

func (c *MessageSendCmd) Run(ctx *Context) error {
	if err := validateMessageInputSources(c.Rich, c.Stdin, c); err != nil {
		return err
	}
	client, err := ctx.NewClient("")
	if err != nil {
		return err
	}

	text, err := c.messageText()
	if err != nil {
		return err
	}

	target, err := slack.ResolveConversationTarget(client, c.Recipient)
	if err != nil {
		err = ctx.augmentChannelNotFoundError("", err)
		err = ctx.augmentCrossWorkspaceChannelHint("", err)
		if slack.IsAPIError(err, "missing_scope") {
			return fmt.Errorf("%w. Update the Slack app scopes and rerun 'slack-cli auth login' for that workspace", err)
		}
		return err
	}

	if c.hasBlockSource() {
		blocks, err := c.loadBlocks()
		if err != nil {
			return err
		}
		payload := slack.ChatMessageRequest{
			Channel:  target.ChannelID,
			Text:     text,
			ThreadTS: c.Thread,
			Blocks:   blocks,
		}
		if c.DryRun {
			return printMessagePayload(payload)
		}
		resp, err := client.PostMessageWithBlocks(payload)
		if err != nil {
			return c.augmentSendError(ctx, err)
		}
		verified, warning := verifyMessageBlocksBestEffort(client, resp, c.Thread, blocks)
		return emitMessageWriteResult(client, "send", target, resp, verified, warning, c.JSON)
	}

	if c.Rich {
		resolvedText, err := resolveMessageUserGroups(ctx, client, text)
		if err != nil {
			return err
		}
		blocks, err := slack.BuildRichTextBlocks(resolvedText)
		if err != nil {
			return err
		}
		payload := slack.ChatMessageRequest{
			Channel:  target.ChannelID,
			Text:     resolvedText,
			ThreadTS: c.Thread,
			Blocks:   blocks,
		}
		if c.DryRun {
			return printMessagePayload(payload)
		}
		resp, err := client.PostMessageWithBlocks(payload)
		if err != nil {
			return c.augmentSendError(ctx, err)
		}
		verified, warning := verifyMessageBlocksBestEffort(client, resp, c.Thread, blocks)
		return emitMessageWriteResult(client, "send", target, resp, verified, warning, c.JSON)
	}

	if c.DryRun {
		payload := slack.ChatMessageRequest{Channel: target.ChannelID, Text: text, ThreadTS: c.Thread}
		return printMessagePayload(payload)
	}

	resp, err := client.PostMessage(target.ChannelID, text, c.Thread, c.Mrkdwn)
	if err != nil {
		return c.augmentSendError(ctx, err)
	}

	fmt.Printf("Sent message to %s (%s) at %s\n", formatMessageConversationTargetLabel(target), resp.Channel, resp.TS)
	return nil
}

func (c *MessageUpdateCmd) Run(ctx *Context) error {
	client, err := ctx.NewClient("")
	if err != nil {
		return err
	}
	content := &MessageSendCmd{
		Text:        c.Text,
		Stdin:       c.Stdin,
		Rich:        c.Rich,
		Blocks:      c.Blocks,
		BlocksFile:  c.BlocksFile,
		BlocksStdin: c.BlocksStdin,
	}
	if err := validateMessageInputSources(c.Rich, c.Stdin, content); err != nil {
		return err
	}
	text, err := content.messageText()
	if err != nil {
		return err
	}
	if strings.TrimSpace(c.Timestamp) == "" {
		return fmt.Errorf("message timestamp is required")
	}
	target, err := slack.ResolveConversationTarget(client, c.Channel)
	if err != nil {
		return err
	}

	var blocks json.RawMessage
	hasStructuredBlocks := c.Rich || content.hasBlockSource()
	if c.Rich {
		text, err = resolveMessageUserGroups(ctx, client, text)
		if err != nil {
			return err
		}
		blocks, err = slack.BuildRichTextBlocks(text)
		if err != nil {
			return err
		}
	} else if content.hasBlockSource() {
		blocks, err = content.loadBlocks()
		if err != nil {
			return err
		}
	} else {
		blocks = json.RawMessage("[]")
	}

	payload := slack.ChatMessageRequest{Channel: target.ChannelID, TS: c.Timestamp, Text: text, Blocks: blocks}
	if c.DryRun {
		return printMessagePayload(payload)
	}
	resp, err := client.UpdateMessageWithBlocks(payload)
	if err != nil {
		return fmt.Errorf("failed to update message: %w", err)
	}
	verified := false
	warning := ""
	if hasStructuredBlocks {
		verified, warning = verifyMessageBlocksBestEffort(client, resp, c.Thread, blocks)
	}
	return emitMessageWriteResult(client, "update", target, resp, verified, warning, c.JSON)
}

func (c *MessageSendCmd) hasBlockSource() bool {
	return strings.TrimSpace(c.Blocks) != "" || strings.TrimSpace(c.BlocksFile) != "" || c.BlocksStdin
}

func validateMessageInputSources(rich, stdin bool, content *MessageSendCmd) error {
	if rich && content.hasBlockSource() {
		return fmt.Errorf("cannot combine --rich with --blocks, --blocks-file, or --blocks-stdin")
	}
	if stdin && content.BlocksStdin {
		return fmt.Errorf("cannot read both message text and blocks from stdin")
	}
	return nil
}

func (c *MessageSendCmd) loadBlocks() (json.RawMessage, error) {
	sources := 0
	if strings.TrimSpace(c.Blocks) != "" {
		sources++
	}
	if strings.TrimSpace(c.BlocksFile) != "" {
		sources++
	}
	if c.BlocksStdin {
		sources++
	}
	if sources != 1 {
		return nil, fmt.Errorf("exactly one of --blocks, --blocks-file, or --blocks-stdin is required")
	}

	var body []byte
	var err error
	switch {
	case strings.TrimSpace(c.Blocks) != "":
		body = []byte(c.Blocks)
	case strings.TrimSpace(c.BlocksFile) != "":
		file, openErr := os.Open(c.BlocksFile)
		if openErr != nil {
			return nil, fmt.Errorf("open blocks file: %w", openErr)
		}
		body, err = io.ReadAll(io.LimitReader(file, maxBlocksPayloadBytes+1))
		closeErr := file.Close()
		if err != nil {
			return nil, fmt.Errorf("read blocks file: %w", err)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close blocks file: %w", closeErr)
		}
	case c.BlocksStdin:
		body, err = io.ReadAll(io.LimitReader(os.Stdin, maxBlocksPayloadBytes+1))
		if err != nil {
			return nil, fmt.Errorf("read blocks from stdin: %w", err)
		}
	}
	if len(body) > maxBlocksPayloadBytes {
		return nil, fmt.Errorf("blocks payload exceeds %d bytes", maxBlocksPayloadBytes)
	}

	var blocks []json.RawMessage
	if err := json.Unmarshal(body, &blocks); err != nil {
		return nil, fmt.Errorf("blocks must be a JSON array: %w", err)
	}
	if len(blocks) == 0 {
		return nil, fmt.Errorf("blocks array must not be empty")
	}
	if len(blocks) > maxMessageBlocks {
		return nil, fmt.Errorf("slack messages support at most %d blocks", maxMessageBlocks)
	}
	for i, block := range blocks {
		var header struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(block, &header); err != nil {
			return nil, fmt.Errorf("block %d must be a JSON object: %w", i, err)
		}
		if strings.TrimSpace(header.Type) == "" {
			return nil, fmt.Errorf("block %d is missing type", i)
		}
	}
	return json.RawMessage(bytes.TrimSpace(body)), nil
}

func resolveMessageUserGroups(ctx *Context, client *slack.Client, text string) (string, error) {
	if ctx != nil && ctx.Config != nil {
		resolved, count := slack.ResolveUserGroupMentionsFromMap(text, ctx.Config.UserGroupMappings(ctx.Workspace))
		if count > 0 {
			return resolved, nil
		}
	}
	resolved, err := client.ResolveUserGroupMentions(text)
	if slack.IsAPIError(err, "missing_scope") {
		fmt.Fprintln(os.Stderr, "Warning: user-group handles were not resolved because this workspace lacks usergroups:read; configure workspaces.<workspace>.user_groups")
		return text, nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to resolve Slack user-group mentions: %w", err)
	}
	return resolved, nil
}

func (c *MessageSendCmd) augmentSendError(ctx *Context, err error) error {
	err = ctx.augmentChannelNotFoundError("", err)
	err = ctx.augmentCrossWorkspaceChannelHint("", err)
	if slack.IsAPIError(err, "missing_scope") {
		return fmt.Errorf("%w. Update the Slack app scopes and rerun 'slack-cli auth login' for that workspace", err)
	}
	return fmt.Errorf("failed to send message: %w", err)
}

type messageWriteResult struct {
	Operation string        `json:"operation"`
	Channel   string        `json:"channel"`
	Timestamp string        `json:"ts"`
	Permalink string        `json:"permalink,omitempty"`
	Text      string        `json:"text"`
	Blocks    []slack.Block `json:"blocks,omitempty"`
	Verified  bool          `json:"verified"`
	Warning   string        `json:"warning,omitempty"`
}

func emitMessageWriteResult(client *slack.Client, operation string, target *slack.ConversationTarget, resp *slack.PostMessageResponse, verified bool, warning string, jsonOutput bool) error {
	result := messageWriteResult{
		Operation: operation,
		Channel:   resp.Channel,
		Timestamp: resp.TS,
		Text:      resp.Message.Text,
		Blocks:    resp.Message.Blocks,
		Verified:  verified,
		Warning:   warning,
	}
	permalink, err := client.GetMessagePermalink(resp.Channel, resp.TS)
	if err != nil {
		permalinkWarning := "permalink lookup failed: " + err.Error()
		if result.Warning == "" {
			result.Warning = permalinkWarning
		} else {
			result.Warning += "; " + permalinkWarning
		}
	} else {
		result.Permalink = permalink
	}
	if jsonOutput {
		body, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to encode message result: %w", err)
		}
		fmt.Println(string(body))
		return nil
	}

	verb := "Sent"
	if operation == "update" {
		verb = "Updated"
	}
	fmt.Printf("%s message in %s (%s) at %s\n", verb, formatMessageConversationTargetLabel(target), resp.Channel, resp.TS)
	if result.Permalink != "" {
		fmt.Printf("Permalink: %s\n", result.Permalink)
	}
	if result.Warning != "" {
		fmt.Printf("Warning: %s\n", result.Warning)
	}
	if verified {
		fmt.Println("Verified native rich formatting in Slack response")
	}
	return nil
}

func printMessagePayload(payload slack.ChatMessageRequest) error {
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode dry-run payload: %w", err)
	}
	fmt.Println(string(body))
	return nil
}

func verifyMessageBlocksBestEffort(client *slack.Client, resp *slack.PostMessageResponse, threadTS string, expected json.RawMessage) (bool, string) {
	immediateErr := verifyRichMessageResponse(expected, resp.Message.Blocks)
	persistedErr := verifyPersistedRichMessage(client, resp, threadTS, expected)
	if persistedErr == nil {
		return true, ""
	}
	parts := []string{"persisted block verification failed: " + persistedErr.Error()}
	if immediateErr != nil {
		parts = append(parts, "write response verification failed: "+immediateErr.Error())
	}
	return false, strings.Join(parts, "; ")
}

func verifyPersistedRichMessage(client *slack.Client, resp *slack.PostMessageResponse, threadTS string, expected json.RawMessage) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		message, err := client.GetMessageByTimestamp(resp.Channel, resp.TS, threadTS)
		if err == nil {
			err = verifyRichMessageResponse(expected, message.Blocks)
		}
		if err == nil {
			resp.Message = *message
			return nil
		}
		lastErr = err
		if attempt < 2 {
			time.Sleep(250 * time.Millisecond)
		}
	}
	return lastErr
}

func verifyRichMessageResponse(expected json.RawMessage, actual []slack.Block) error {
	if len(actual) == 0 {
		return fmt.Errorf("slack response contained no blocks")
	}
	var expectedBlocks []struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(expected, &expectedBlocks); err != nil {
		return fmt.Errorf("decode expected blocks: %w", err)
	}
	if len(expectedBlocks) != len(actual) {
		return fmt.Errorf("slack returned %d blocks, expected %d", len(actual), len(expectedBlocks))
	}
	for i := range expectedBlocks {
		if expectedBlocks[i].Type != actual[i].Type {
			return fmt.Errorf("slack block %d has type %q, expected %q", i, actual[i].Type, expectedBlocks[i].Type)
		}
	}

	actualJSON, err := json.Marshal(actual)
	if err != nil {
		return fmt.Errorf("encode returned blocks: %w", err)
	}
	for _, style := range []string{"ordered", "bullet"} {
		if hasRichTextListStyle(expected, style) && !hasRichTextListStyle(actualJSON, style) {
			return fmt.Errorf("slack flattened the requested %s rich_text_list", style)
		}
	}
	expectedNormalized, actualNormalized, err := normalizeSlackBlockPair(expected, actualJSON)
	if err != nil {
		return fmt.Errorf("normalize blocks: %w", err)
	}
	if !bytes.Equal(expectedNormalized, actualNormalized) {
		return fmt.Errorf("slack persisted blocks differ from the requested payload")
	}
	return nil
}

func normalizeSlackBlockPair(expectedRaw, actualRaw []byte) ([]byte, []byte, error) {
	var expected any
	if err := json.Unmarshal(expectedRaw, &expected); err != nil {
		return nil, nil, err
	}
	var actual any
	if err := json.Unmarshal(actualRaw, &actual); err != nil {
		return nil, nil, err
	}
	var stripServerDefaults func(any, any)
	stripServerDefaults = func(expectedNode, actualNode any) {
		switch expectedTyped := expectedNode.(type) {
		case map[string]any:
			actualTyped, ok := actualNode.(map[string]any)
			if !ok {
				return
			}
			if _, specified := expectedTyped["block_id"]; !specified {
				delete(actualTyped, "block_id")
			}
			if _, specified := expectedTyped["verbatim"]; !specified {
				if verbatim, ok := actualTyped["verbatim"].(bool); ok && !verbatim {
					delete(actualTyped, "verbatim")
				}
			}
			if _, specified := expectedTyped["emoji"]; !specified {
				if emoji, ok := actualTyped["emoji"].(bool); ok && emoji {
					delete(actualTyped, "emoji")
				}
			}
			for key, expectedChild := range expectedTyped {
				if actualChild, ok := actualTyped[key]; ok {
					stripServerDefaults(expectedChild, actualChild)
				}
			}
		case []any:
			actualTyped, ok := actualNode.([]any)
			if !ok {
				return
			}
			for i := 0; i < len(expectedTyped) && i < len(actualTyped); i++ {
				stripServerDefaults(expectedTyped[i], actualTyped[i])
			}
		}
	}
	stripServerDefaults(expected, actual)
	expectedNormalized, err := json.Marshal(expected)
	if err != nil {
		return nil, nil, err
	}
	actualNormalized, err := json.Marshal(actual)
	if err != nil {
		return nil, nil, err
	}
	return expectedNormalized, actualNormalized, nil
}

func hasRichTextListStyle(raw []byte, style string) bool {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	var walk func(any) bool
	walk = func(node any) bool {
		switch typed := node.(type) {
		case map[string]any:
			if typed["type"] == "rich_text_list" && typed["style"] == style {
				return true
			}
			for _, child := range typed {
				if walk(child) {
					return true
				}
			}
		case []any:
			for _, child := range typed {
				if walk(child) {
					return true
				}
			}
		}
		return false
	}
	return walk(value)
}

func (c *MessageSendCmd) messageText() (string, error) {
	if c.Stdin && c.Text != "" {
		return "", fmt.Errorf("cannot use both message text argument and --stdin")
	}

	if c.Stdin {
		body, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read stdin: %w", err)
		}
		text := string(body)
		if strings.TrimSpace(text) == "" {
			return "", fmt.Errorf("message text is required")
		}
		return text, nil
	}

	if strings.TrimSpace(c.Text) == "" {
		return "", fmt.Errorf("message text is required")
	}

	return c.Text, nil
}

func formatMessageConversationTargetLabel(target *slack.ConversationTarget) string {
	if target == nil {
		return "recipient"
	}
	if target.Type == "dm" {
		if target.User != nil && strings.TrimSpace(target.User.Name) != "" {
			return "@" + strings.TrimSpace(target.User.Name)
		}
		if target.UserID != "" {
			return target.UserID
		}
	}
	if strings.TrimSpace(target.ChannelName) != "" {
		return "#" + strings.TrimSpace(target.ChannelName)
	}
	if target.ChannelID != "" {
		return target.ChannelID
	}
	return target.Recipient
}
