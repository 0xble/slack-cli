package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lox/slack-cli/internal/slack"
)

type FileCmd struct {
	List     FileListCmd     `cmd:"" aliases:"ls" help:"List recent files"`
	Info     FileInfoCmd     `cmd:"" help:"Show file metadata"`
	Download FileDownloadCmd `cmd:"" aliases:"dl" help:"Download a file"`
	Upload   FileUploadCmd   `cmd:"" aliases:"up" help:"Upload a file"`
	Delete   FileDeleteCmd   `cmd:"" help:"Delete file(s)"`
}

type FileListCmd struct {
	Limit int `help:"Maximum number of files to list" default:"20" short:"n"`
}

func (c *FileListCmd) Run(ctx *Context) error {
	client, err := ctx.NewClient("")
	if err != nil {
		return err
	}

	resp, err := client.ListFiles(c.Limit)
	if err != nil {
		return fmt.Errorf("failed to list files: %w", err)
	}

	for _, file := range resp.Files {
		fmt.Println(formatFileListLine(file))
	}

	return nil
}

type FileInfoCmd struct {
	FileID string `arg:"" help:"File ID"`
}

func (c *FileInfoCmd) Run(ctx *Context) error {
	client, err := ctx.NewClient("")
	if err != nil {
		return err
	}

	file, err := client.GetFileInfo(c.FileID)
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	printFileInfo(file)
	return nil
}

type FileDownloadCmd struct {
	FileID string `arg:"" help:"File ID"`
	Output string `arg:"" optional:"" help:"Output path"`
}

func (c *FileDownloadCmd) Run(ctx *Context) error {
	client, err := ctx.NewClient("")
	if err != nil {
		return err
	}

	file, err := client.GetFileInfo(c.FileID)
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	fileURL := downloadURLForFile(file)
	if fileURL == "" {
		return fmt.Errorf("file %s has no downloadable URL", c.FileID)
	}

	maxBytes := downloadLimitForFile(file)

	body, _, err := client.DownloadPrivateFile(fileURL, maxBytes)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}

	outputPath, err := resolveDownloadPath(c.Output, file.Name, file.ID)
	if err != nil {
		return err
	}
	if err := writeFileExclusive(outputPath, body); err != nil {
		return err
	}

	fmt.Printf("Downloaded file to %s\n", outputPath)
	return nil
}

type FileUploadCmd struct {
	Recipient string `arg:"" help:"Recipient #channel, channel name, channel ID, @username, user ID, or DM ID"`
	Path      string `arg:"" help:"Local file path"`
	Title     string `help:"File title"`
	Comment   string `help:"Initial comment"`
	Thread    string `help:"Reply in a thread"`
}

func (c *FileUploadCmd) Run(ctx *Context) error {
	client, err := ctx.NewClient("")
	if err != nil {
		return err
	}

	target, err := resolveFileUploadTarget(client, c.Recipient)
	if err != nil {
		err = ctx.augmentChannelNotFoundError("", err)
		err = ctx.augmentCrossWorkspaceChannelHint("", err)
		if slack.IsAPIError(err, "missing_scope") {
			return fmt.Errorf("%w. Update the Slack app scopes and rerun 'slack-cli auth login' for that workspace", err)
		}
		return err
	}

	fileHandle, stat, err := openUploadFile(c.Path)
	if err != nil {
		return err
	}
	defer fileHandle.Close() //nolint:errcheck

	filename := filepath.Base(stat.Name())
	title := resolveUploadTitle(c.Title, filename)

	uploadURL, err := client.GetUploadURLExternal(filename, stat.Size())
	if err != nil {
		if err := wrapFileWriteScopeError(err); err != nil {
			return err
		}
		return fmt.Errorf("failed to initialize file upload: %w", err)
	}

	if err := client.UploadExternalFile(uploadURL.UploadURL, filename, fileHandle, stat.Size()); err != nil {
		return fmt.Errorf("failed to upload file bytes: %w", err)
	}

	resp, err := client.CompleteUploadExternal(uploadURL.FileID, title, target.ChannelID, c.Comment, c.Thread)
	if err != nil {
		if err := wrapFileWriteScopeError(err); err != nil {
			return err
		}
		return fmt.Errorf("failed to complete file upload: %w", err)
	}

	fileID := resolveUploadedFileID(uploadURL.FileID, resp)

	fmt.Printf("Uploaded file to %s (%s): %s\n", target.Label, target.ChannelID, fileID)
	return nil
}

type FileDeleteCmd struct {
	FileIDs []string `arg:"" help:"File ID(s)"`
}

func (c *FileDeleteCmd) Run(ctx *Context) error {
	client, err := ctx.NewClient("")
	if err != nil {
		return err
	}

	for _, fileID := range c.FileIDs {
		if err := client.DeleteFile(fileID); err != nil {
			if err := wrapFileWriteScopeError(err); err != nil {
				return err
			}
			return fmt.Errorf("failed to delete file %s: %w", fileID, err)
		}
		fmt.Printf("Deleted file %s\n", fileID)
	}

	return nil
}

type fileUploadTarget struct {
	ChannelID string
	Label     string
}

func resolveFileUploadTarget(client *slack.Client, recipient string) (*fileUploadTarget, error) {
	trimmed := strings.TrimSpace(recipient)
	if trimmed == "" {
		return nil, fmt.Errorf("recipient is required")
	}

	switch {
	case strings.HasPrefix(trimmed, "D"):
		return &fileUploadTarget{ChannelID: trimmed, Label: trimmed}, nil
	case strings.HasPrefix(trimmed, "U"):
		user, err := client.GetUserInfo(trimmed)
		if err != nil {
			return nil, err
		}
		return openDMUploadTarget(client, user)
	case strings.HasPrefix(trimmed, "@"):
		user, err := lookupUploadUserByHandle(client, trimmed)
		if err != nil {
			return nil, err
		}
		return openDMUploadTarget(client, user)
	case strings.HasPrefix(trimmed, "C") || strings.HasPrefix(trimmed, "G"):
		info, err := client.GetConversationInfo(trimmed)
		if err != nil {
			return nil, err
		}
		return &fileUploadTarget{ChannelID: trimmed, Label: formatUploadChannelLabel(info, trimmed)}, nil
	default:
		return resolveChannelUploadTargetByName(client, trimmed)
	}
}

func lookupUploadUserByHandle(client *slack.Client, handle string) (*slack.User, error) {
	users, err := client.ListUsers(1000)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}

	needle := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(handle)), "@")
	for _, user := range users.Members {
		if strings.ToLower(strings.TrimSpace(user.Name)) == needle {
			matched := user
			return &matched, nil
		}
	}

	return nil, fmt.Errorf("user not found: %s", handle)
}

func openDMUploadTarget(client *slack.Client, user *slack.User) (*fileUploadTarget, error) {
	resp, err := client.OpenConversation([]string{user.ID}, true)
	if err != nil {
		return nil, err
	}

	label := "@" + user.Name
	if strings.TrimSpace(user.Name) == "" {
		label = user.ID
	}

	return &fileUploadTarget{
		ChannelID: resp.Channel.ID,
		Label:     label,
	}, nil
}

func formatUploadChannelLabel(channel *slack.Channel, fallback string) string {
	if channel != nil && strings.TrimSpace(channel.Name) != "" {
		if channel.IsPrivate {
			return channel.Name
		}
		return "#" + channel.Name
	}
	return strings.TrimSpace(fallback)
}

func resolveChannelUploadTargetByName(client *slack.Client, recipient string) (*fileUploadTarget, error) {
	channelName := strings.TrimPrefix(strings.TrimSpace(recipient), "#")
	channels, err := client.ListConversations("public_channel,private_channel", 1000)
	if err != nil {
		return nil, fmt.Errorf("failed to list channels: %w", err)
	}

	for _, channel := range channels.Channels {
		if channel.Name != channelName {
			continue
		}
		return &fileUploadTarget{
			ChannelID: channel.ID,
			Label:     formatUploadChannelLabel(&channel, recipient),
		}, nil
	}

	return nil, &slack.APIError{Method: "conversations.resolve", Code: "channel_not_found"}
}

func formatFileListLine(file slack.File) string {
	label := strings.TrimSpace(file.Title)
	if label == "" {
		label = strings.TrimSpace(file.Name)
	}
	if label == "" {
		label = file.ID
	}

	details := []string{humanSize(file.Size)}
	if strings.TrimSpace(file.PrettyType) != "" {
		details = append(details, strings.TrimSpace(file.PrettyType))
	} else if strings.TrimSpace(file.Mimetype) != "" {
		details = append(details, strings.TrimSpace(file.Mimetype))
	}

	if file.Name != "" && file.Title != "" && strings.TrimSpace(file.Name) != strings.TrimSpace(file.Title) {
		details = append(details, file.Name)
	}

	return fmt.Sprintf("%s %s (%s)", file.ID, label, strings.Join(details, ", "))
}

func printFileInfo(file *slack.File) {
	fmt.Printf("ID: %s\n", file.ID)
	if strings.TrimSpace(file.Name) != "" {
		fmt.Printf("Name: %s\n", file.Name)
	}
	if strings.TrimSpace(file.Title) != "" {
		fmt.Printf("Title: %s\n", file.Title)
	}
	if strings.TrimSpace(file.PrettyType) != "" {
		fmt.Printf("Type: %s\n", file.PrettyType)
	} else if strings.TrimSpace(file.Mimetype) != "" {
		fmt.Printf("Type: %s\n", file.Mimetype)
	}
	fmt.Printf("Size: %s\n", humanSize(file.Size))
	if strings.TrimSpace(file.Mode) != "" {
		fmt.Printf("Mode: %s\n", file.Mode)
	}
	if file.Created > 0 {
		fmt.Printf("Created: %s\n", time.Unix(file.Created, 0).Format(time.RFC3339))
	}
	if strings.TrimSpace(file.User) != "" {
		fmt.Printf("User: %s\n", file.User)
	}
	fmt.Printf("Public: %v\n", file.IsPublic)
	if strings.TrimSpace(file.FileAccess) != "" {
		fmt.Printf("Access: %s\n", file.FileAccess)
	}
	if strings.TrimSpace(file.Permalink) != "" {
		fmt.Printf("Permalink: %s\n", file.Permalink)
	}
}

func downloadURLForFile(file *slack.File) string {
	fileURL := strings.TrimSpace(file.URLPrivateDownload)
	if fileURL != "" {
		return fileURL
	}
	return strings.TrimSpace(file.URLPrivate)
}

func downloadLimitForFile(file *slack.File) int {
	if file == nil || file.Size <= 0 {
		return 1024
	}

	// Slack-hosted downloads can drift slightly from the reported size.
	return file.Size + 1024
}

func resolveUploadTitle(rawTitle, filename string) string {
	title := strings.TrimSpace(rawTitle)
	if title != "" {
		return title
	}
	return filename
}

func resolveUploadedFileID(fallback string, response *slack.CompleteUploadExternalResponse) string {
	if response != nil && len(response.Files) > 0 {
		fileID := strings.TrimSpace(response.Files[0].ID)
		if fileID != "" {
			return fileID
		}
	}
	return fallback
}

func wrapFileWriteScopeError(err error) error {
	if slack.IsAPIError(err, "missing_scope") {
		return fmt.Errorf("%w. Update the Slack app scopes to include files:write, then rerun 'slack-cli auth login' for that workspace", err)
	}
	return nil
}

func humanSize(size int) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	if size < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(size)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(size)/(1024*1024))
}

func openUploadFile(path string) (*os.File, os.FileInfo, error) {
	fileHandle, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open file: %w", err)
	}

	stat, err := fileHandle.Stat()
	if err != nil {
		fileHandle.Close() //nolint:errcheck
		return nil, nil, fmt.Errorf("failed to stat file: %w", err)
	}
	if stat.IsDir() {
		fileHandle.Close() //nolint:errcheck
		return nil, nil, fmt.Errorf("upload path must be a file")
	}

	return fileHandle, stat, nil
}

func resolveDownloadPath(output, name, fileID string) (string, error) {
	filename := strings.TrimSpace(name)
	if filename == "" {
		filename = fileID
	}
	filename = filepath.Base(filename)

	if strings.TrimSpace(output) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to determine current directory: %w", err)
		}
		return filepath.Join(cwd, filename), nil
	}

	info, err := os.Stat(output)
	if err == nil && info.IsDir() {
		return filepath.Join(output, filename), nil
	}
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("failed to inspect output path: %w", err)
	}

	return output, nil
}

func writeFileExclusive(path string, body []byte) error {
	fileHandle, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("output file already exists: %s", path)
		}
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer fileHandle.Close() //nolint:errcheck

	if _, err := fileHandle.Write(body); err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}

	return nil
}
