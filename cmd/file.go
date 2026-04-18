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

	outputPath, err := resolveDownloadPath(c.Output, file.Name, file.ID)
	if err != nil {
		return err
	}
	if err := streamDownloadedFile(client, fileURL, outputPath); err != nil {
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

	target, err := slack.ResolveConversationTarget(client, c.Recipient)
	if err != nil {
		err = ctx.augmentChannelNotFoundError("", err)
		err = ctx.augmentCrossWorkspaceChannelHint("", err)
		if wrapped := wrapMissingScope(err, "the scopes required for this recipient"); wrapped != nil {
			return wrapped
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
		if wrapped := wrapMissingScope(err, "files:write"); wrapped != nil {
			return wrapped
		}
		return fmt.Errorf("failed to initialize file upload: %w", err)
	}

	if err := client.UploadExternalFile(uploadURL.UploadURL, filename, fileHandle, stat.Size()); err != nil {
		return fmt.Errorf("failed to upload file bytes: %w", err)
	}

	resp, err := client.CompleteUploadExternal(uploadURL.FileID, title, target.ChannelID, c.Comment, c.Thread)
	if err != nil {
		if wrapped := wrapMissingScope(err, "files:write"); wrapped != nil {
			return wrapped
		}
		return fmt.Errorf("failed to complete file upload: %w", err)
	}

	fileID := resolveUploadedFileID(uploadURL.FileID, resp)

	fmt.Printf("Uploaded file to %s (%s): %s\n", formatConversationTargetLabel(target), target.ChannelID, fileID)
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
			if wrapped := wrapMissingScope(err, "files:write"); wrapped != nil {
				return wrapped
			}
			return fmt.Errorf("failed to delete file %s: %w", fileID, err)
		}
		fmt.Printf("Deleted file %s\n", fileID)
	}

	return nil
}

// formatConversationTargetLabel returns a human-readable label for a
// resolved conversation target, suitable for CLI status lines.
func formatConversationTargetLabel(target *slack.ConversationTarget) string {
	if target == nil {
		return ""
	}
	if target.IsDM {
		if target.Username != "" {
			return "@" + target.Username
		}
		return target.ChannelID
	}
	if target.Name != "" {
		if target.IsPrivate {
			return target.Name
		}
		return "#" + target.Name
	}
	return target.ChannelID
}

func formatFileListLine(file slack.File) string {
	label := file.Title
	if label == "" {
		label = file.Name
	}
	if label == "" {
		label = file.ID
	}

	details := []string{humanSize(file.Size)}
	if file.PrettyType != "" {
		details = append(details, file.PrettyType)
	} else if file.Mimetype != "" {
		details = append(details, file.Mimetype)
	}

	if file.Name != "" && file.Title != "" && file.Name != file.Title {
		details = append(details, file.Name)
	}

	return fmt.Sprintf("%s %s (%s)", file.ID, label, strings.Join(details, ", "))
}

func printFileInfo(file *slack.File) {
	fmt.Printf("ID: %s\n", file.ID)
	if file.Name != "" {
		fmt.Printf("Name: %s\n", file.Name)
	}
	if file.Title != "" {
		fmt.Printf("Title: %s\n", file.Title)
	}
	if file.PrettyType != "" {
		fmt.Printf("Type: %s\n", file.PrettyType)
	} else if file.Mimetype != "" {
		fmt.Printf("Type: %s\n", file.Mimetype)
	}
	fmt.Printf("Size: %s\n", humanSize(file.Size))
	if file.Mode != "" {
		fmt.Printf("Mode: %s\n", file.Mode)
	}
	if file.Created > 0 {
		fmt.Printf("Created: %s\n", time.Unix(file.Created, 0).Format(time.RFC3339))
	}
	if file.User != "" {
		fmt.Printf("User: %s\n", file.User)
	}
	fmt.Printf("Public: %v\n", file.IsPublic)
	if file.FileAccess != "" {
		fmt.Printf("Access: %s\n", file.FileAccess)
	}
	if file.Permalink != "" {
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

func humanSize(size int) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := int64(size) / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	units := []string{"KB", "MB", "GB", "TB"}
	if exp >= len(units) {
		exp = len(units) - 1
	}
	return fmt.Sprintf("%.1f %s", float64(size)/float64(div), units[exp])
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

func openOutputFileExclusive(path string) (*os.File, error) {
	fileHandle, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("output file already exists: %s", path)
		}
		return nil, fmt.Errorf("failed to create output file: %w", err)
	}

	return fileHandle, nil
}

func streamDownloadedFile(client *slack.Client, fileURL, outputPath string) (err error) {
	fileHandle, err := openOutputFileExclusive(outputPath)
	if err != nil {
		return err
	}
	defer func() {
		closeErr := fileHandle.Close()
		if err == nil && closeErr != nil {
			err = fmt.Errorf("failed to close output file: %w", closeErr)
		}
		if err != nil {
			_ = os.Remove(outputPath)
		}
	}()

	if _, _, err := client.DownloadPrivateFileToWriter(fileURL, fileHandle); err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}

	return nil
}
