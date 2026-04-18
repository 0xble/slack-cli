package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/lox/slack-cli/internal/output"
	"github.com/lox/slack-cli/internal/slack"
)

type CanvasCmd struct {
	List   CanvasListCmd   `cmd:"" aliases:"ls" help:"List recent canvases"`
	Read   CanvasReadCmd   `cmd:"" help:"Read canvas content"`
	Delete CanvasDeleteCmd `cmd:"" help:"Delete canvas file(s)"`
}

type CanvasListCmd struct {
	Channel string `help:"Filter to a channel name, ID, or Slack URL"`
	Limit   int    `help:"Maximum number of canvases to list" default:"20" short:"n"`
	JSON    bool   `help:"Output as pretty JSON array" short:"j" xor:"format"`
	JSONL   bool   `help:"Output as JSON Lines, one canvas per line" xor:"format"`
	After   string `help:"Only list canvases on or after DATE (YYYY-MM-DD, UTC)" xor:"after-last,after-on"`
	Before  string `help:"Only list canvases on or before DATE (YYYY-MM-DD, UTC)" xor:"before-on"`
	On      string `help:"Only list canvases on DATE (YYYY-MM-DD, UTC)" xor:"after-on,before-on,on-last"`
	Last    string `help:"Only list canvases from the last DURATION (e.g. 45d, 12h, 2w)" xor:"after-last,on-last"`
}

func (c *CanvasListCmd) Run(ctx *Context) error {
	channelRef, urlHint, err := parseCanvasChannelReference(c.Channel)
	if err != nil {
		return err
	}

	client, err := ctx.NewClient(urlHint)
	if err != nil {
		return err
	}

	filter, err := slack.ResolveDateFilter(c.After, c.Before, c.On, c.Last, time.Now())
	if err != nil {
		return err
	}

	channelID, err := resolveCanvasChannelFilter(client, channelRef)
	if err != nil {
		return err
	}

	oldest, latest := filter.ToTimestampParams()
	resp, err := client.ListFiles(slack.ListFilesParams{
		Limit:     c.Limit,
		Types:     "canvas",
		ChannelID: channelID,
		TSFrom:    oldest,
		TSTo:      latest,
	})
	if err != nil {
		err = withFilesReadScopeHint(err)
		return fmt.Errorf("failed to list canvases: %w", err)
	}

	if c.JSON || c.JSONL {
		records := make([]output.File, 0, len(resp.Files))
		for _, file := range resp.Files {
			if slack.IsCanvasFile(file) {
				records = append(records, output.ToFile(file))
			}
		}
		if c.JSONL {
			return output.EmitJSONL(records)
		}
		return output.EmitJSON(records)
	}

	for _, file := range resp.Files {
		if slack.IsCanvasFile(file) {
			fmt.Println(formatFileListLine(file))
		}
	}

	return nil
}

type CanvasReadCmd struct {
	CanvasID string `arg:"" name:"canvas_id" help:"Canvas ID"`
	Raw      bool   `help:"Output raw HTML" xor:"format"`
	JSON     bool   `help:"Output as JSON including converted body" short:"j" xor:"format"`
}

func (c *CanvasReadCmd) Run(ctx *Context) error {
	client, err := ctx.NewClient("")
	if err != nil {
		return err
	}

	file, err := client.GetFileInfo(c.CanvasID)
	if err != nil {
		err = withFilesReadScopeHint(err)
		return fmt.Errorf("failed to get canvas info: %w", err)
	}
	if !slack.IsCanvasFile(*file) {
		return fmt.Errorf("file is not a canvas: %s", c.CanvasID)
	}

	fileURL, err := downloadableFileURL(file)
	if err != nil {
		return fmt.Errorf("canvas %s has no downloadable URL", c.CanvasID)
	}

	body, _, err := client.DownloadPrivateFile(fileURL, downloadSizeLimit(file))
	if err != nil {
		return fmt.Errorf("failed to download canvas: %w", err)
	}

	if c.Raw {
		fmt.Println(string(body))
		return nil
	}

	userNames, err := loadCanvasUserNames(client)
	if err != nil {
		return err
	}

	textBody := slack.CanvasHTMLToText(string(body), userNames)
	if c.JSON {
		return output.EmitJSON(struct {
			output.File
			Body string `json:"body,omitempty"`
		}{
			File: output.ToFile(*file),
			Body: textBody,
		})
	}

	fmt.Println(textBody)
	return nil
}

type CanvasDeleteCmd struct {
	CanvasIDs []string `arg:"" name:"canvas_id" help:"Canvas ID(s)"`
}

func (c *CanvasDeleteCmd) Run(ctx *Context) error {
	client, err := ctx.NewClient("")
	if err != nil {
		return err
	}

	for _, canvasID := range c.CanvasIDs {
		file, err := client.GetFileInfo(canvasID)
		if err != nil {
			err = withFilesReadScopeHint(err)
			return fmt.Errorf("failed to get canvas info: %w", err)
		}
		if !slack.IsCanvasFile(*file) {
			return fmt.Errorf("file is not a canvas: %s", canvasID)
		}

		if err := client.DeleteFile(canvasID); err != nil {
			err = withFilesWriteScopeHint(err)
			return fmt.Errorf("failed to delete canvas %s: %w", canvasID, err)
		}
		fmt.Printf("Deleted canvas %s\n", canvasID)
	}

	return nil
}

func parseCanvasChannelReference(channel string) (channelRef string, urlHint string, err error) {
	trimmed := strings.TrimSpace(channel)
	if trimmed == "" {
		return "", "", nil
	}

	return parseChannelReference(trimmed)
}

func resolveCanvasChannelFilter(client *slack.Client, channelRef string) (string, error) {
	if strings.TrimSpace(channelRef) == "" {
		return "", nil
	}

	if isSlackChannelID(channelRef) {
		return channelRef, nil
	}

	resp, err := client.ListConversations("public_channel,private_channel", 1000)
	if err != nil {
		return "", fmt.Errorf("failed to list channels: %w", err)
	}
	for _, ch := range resp.Channels {
		if ch.Name == channelRef {
			return ch.ID, nil
		}
	}

	return "", fmt.Errorf("channel not found: %s", channelRef)
}

func loadCanvasUserNames(client *slack.Client) (map[string]string, error) {
	resp, err := client.ListUsers(1000)
	if err != nil {
		if slack.IsAPIError(err, "missing_scope") {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("failed to list users for canvas mention resolution: %w", err)
	}

	userNames := make(map[string]string, len(resp.Members))
	for _, user := range resp.Members {
		userNames[user.ID] = canvasUserDisplayName(user)
	}

	return userNames, nil
}

func canvasUserDisplayName(user slack.User) string {
	for _, candidate := range []string{user.Profile.DisplayName, user.RealName, user.Name, user.ID} {
		if strings.TrimSpace(candidate) != "" {
			return candidate
		}
	}
	return "unknown"
}
