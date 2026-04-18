package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lox/slack-cli/internal/output"
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
	Limit  int    `help:"Maximum number of files to list" default:"20" short:"n"`
	JSON   bool   `help:"Output as pretty JSON array" short:"j" xor:"format"`
	JSONL  bool   `help:"Output as JSON Lines, one file per line" xor:"format"`
	After  string `help:"Only list files on or after DATE (YYYY-MM-DD, UTC)" xor:"after-last,after-on"`
	Before string `help:"Only list files on or before DATE (YYYY-MM-DD, UTC)" xor:"before-on"`
	On     string `help:"Only list files on DATE (YYYY-MM-DD, UTC)" xor:"after-on,before-on,on-last"`
	Last   string `help:"Only list files from the last DURATION (e.g. 45d, 12h, 2w)" xor:"after-last,on-last"`
}

func (c *FileListCmd) Run(ctx *Context) error {
	client, err := ctx.NewClient("")
	if err != nil {
		return err
	}

	filter, err := slack.ResolveDateFilter(c.After, c.Before, c.On, c.Last, time.Now())
	if err != nil {
		return err
	}
	oldest, latest := filter.ToTimestampParams()
	resp, err := client.ListFiles(slack.ListFilesParams{Limit: c.Limit, TSFrom: oldest, TSTo: latest})
	if err != nil {
		return fmt.Errorf("failed to list files: %w", err)
	}

	if c.JSON || c.JSONL {
		records := make([]output.File, 0, len(resp.Files))
		for _, file := range resp.Files {
			records = append(records, output.ToFile(file))
		}
		if c.JSONL {
			return output.EmitJSONL(records)
		}
		return output.EmitJSON(records)
	}

	for _, file := range resp.Files {
		fmt.Println(formatFileListLine(file))
	}

	return nil
}

type FileInfoCmd struct {
	FileID string `arg:"" help:"File ID"`
	JSON   bool   `help:"Output as JSON" short:"j"`
}

func (c *FileInfoCmd) Run(ctx *Context) error {
	client, err := ctx.NewClient("")
	if err != nil {
		return err
	}

	file, err := client.GetFileInfo(c.FileID)
	if err != nil {
		err = withFilesReadScopeHint(err)
		return fmt.Errorf("failed to get file info: %w", err)
	}

	if c.JSON {
		return output.EmitJSON(output.ToFile(*file))
	}

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
		err = withFilesReadScopeHint(err)
		return fmt.Errorf("failed to get file info: %w", err)
	}

	fileURL, err := downloadableFileURL(file)
	if err != nil {
		return fmt.Errorf("file %s has no downloadable URL", c.FileID)
	}

	body, _, err := client.DownloadPrivateFile(fileURL, downloadSizeLimit(file))
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

	target, err := slack.ResolveConversationTarget(client, c.Recipient)
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
	title := strings.TrimSpace(c.Title)
	if title == "" {
		title = filename
	}

	uploadURL, err := client.GetUploadURLExternal(filename, stat.Size())
	if err != nil {
		err = withFilesWriteScopeHint(err)
		return fmt.Errorf("failed to initialize file upload: %w", err)
	}

	if err := client.UploadExternalFile(uploadURL.UploadURL, filename, fileHandle, stat.Size()); err != nil {
		return fmt.Errorf("failed to upload file bytes: %w", err)
	}

	resp, err := client.CompleteUploadExternal(uploadURL.FileID, title, target.ChannelID, c.Comment, c.Thread)
	if err != nil {
		err = withFilesWriteScopeHint(err)
		return fmt.Errorf("failed to complete file upload: %w", err)
	}

	fileID := uploadURL.FileID
	if len(resp.Files) > 0 && strings.TrimSpace(resp.Files[0].ID) != "" {
		fileID = resp.Files[0].ID
	}

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
			err = withFilesWriteScopeHint(err)
			return fmt.Errorf("failed to delete file %s: %w", fileID, err)
		}
		fmt.Printf("Deleted file %s\n", fileID)
	}

	return nil
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

func withFilesReadScopeHint(err error) error {
	if slack.IsAPIError(err, "missing_scope") {
		return fmt.Errorf("%w. Update the Slack app scopes to include files:read, then rerun 'slack-cli auth login' for that workspace", err)
	}
	return err
}

func withFilesWriteScopeHint(err error) error {
	if slack.IsAPIError(err, "missing_scope") {
		return fmt.Errorf("%w. Update the Slack app scopes to include files:write, then rerun 'slack-cli auth login' for that workspace", err)
	}
	return err
}

func downloadableFileURL(file *slack.File) (string, error) {
	if file == nil {
		return "", fmt.Errorf("file metadata is required")
	}

	fileURL := strings.TrimSpace(file.URLPrivateDownload)
	if fileURL == "" {
		fileURL = strings.TrimSpace(file.URLPrivate)
	}
	if fileURL == "" {
		return "", fmt.Errorf("no downloadable URL")
	}

	return fileURL, nil
}

func downloadSizeLimit(file *slack.File) int {
	const minDownloadSizeLimit = 1 << 20

	if file == nil || file.Size <= 0 {
		return minDownloadSizeLimit
	}
	if file.Size < minDownloadSizeLimit {
		return minDownloadSizeLimit
	}
	return file.Size
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
