package output

import (
	"strings"
	"time"

	"github.com/lox/slack-cli/internal/slack"
)

// Shared record shapes for --json / --jsonl output. Deliberately independent
// from the Slack wire types so the CLI's public JSON contract does not shift
// when upstream API fields change.

// Message is the common shape emitted by search, channel read, dm read, and
// thread read. Callers populate whichever fields apply to their source.
type Message struct {
	TS         string       `json:"ts"`
	ThreadTS   string       `json:"thread_ts,omitempty"`
	Type       string       `json:"type,omitempty"`
	User       string       `json:"user,omitempty"`
	UserID     string       `json:"user_id,omitempty"`
	Text       string       `json:"text"`
	TextRaw    string       `json:"text_raw,omitempty"`
	Channel    *ChannelRef  `json:"channel,omitempty"`
	Workspace  string       `json:"workspace,omitempty"`
	Permalink  string       `json:"permalink,omitempty"`
	ReplyCount int          `json:"reply_count,omitempty"`
	Files      []FileRef    `json:"files,omitempty"`
}

// ChannelRef is a short reference embedded in Message results. Full channel
// records use the Channel type below.
type ChannelRef struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
}

// FileRef is a compact file reference embedded on messages. Full file records
// use the File type below.
type FileRef struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	Title     string `json:"title,omitempty"`
	Mimetype  string `json:"mimetype,omitempty"`
	Permalink string `json:"permalink,omitempty"`
}

// Channel is the shape emitted by channel list, channel info, dm list.
type Channel struct {
	ID         string `json:"id"`
	Name       string `json:"name,omitempty"`
	Type       string `json:"type"`
	IsPrivate  bool   `json:"is_private"`
	IsArchived bool   `json:"is_archived,omitempty"`
	NumMembers int    `json:"num_members,omitempty"`
	Topic      string `json:"topic,omitempty"`
	Purpose    string `json:"purpose,omitempty"`
	UserID     string `json:"user_id,omitempty"`
	User       string `json:"user,omitempty"`
}

// User is the shape emitted by user list and user info.
type User struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	RealName    string `json:"real_name,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Email       string `json:"email,omitempty"`
	Title       string `json:"title,omitempty"`
	TZ          string `json:"tz,omitempty"`
	IsBot       bool   `json:"is_bot"`
	Deleted     bool   `json:"deleted"`
}

// File is the shape emitted by file list, file info, canvas list, canvas read.
type File struct {
	ID              string `json:"id"`
	Name            string `json:"name,omitempty"`
	Title           string `json:"title,omitempty"`
	Mimetype        string `json:"mimetype,omitempty"`
	Filetype        string `json:"filetype,omitempty"`
	PrettyType      string `json:"pretty_type,omitempty"`
	Mode            string `json:"mode,omitempty"`
	Size            int    `json:"size,omitempty"`
	User            string `json:"user,omitempty"`
	IsPublic        bool   `json:"is_public,omitempty"`
	URLPrivate      string `json:"url_private,omitempty"`
	Permalink       string `json:"permalink,omitempty"`
	PermalinkPublic string `json:"permalink_public,omitempty"`
	FileAccess      string `json:"file_access,omitempty"`
	Created         string `json:"created,omitempty"`
}

// ChannelTypeFor maps Slack's channel flags to a single-word type tag that
// downstream consumers can branch on cheaply.
func ChannelTypeFor(ch slack.Channel) string {
	switch {
	case ch.IsIM:
		return "im"
	case ch.IsMPIM:
		return "mpim"
	case ch.IsPrivate, ch.IsGroup:
		return "private_channel"
	default:
		return "channel"
	}
}

// ChannelTypeFromID infers a type tag from a raw Slack channel/DM ID prefix.
// Useful for contexts like search where only the ID is known.
func ChannelTypeFromID(id string) string {
	if len(id) == 0 {
		return ""
	}
	switch id[0] {
	case 'D':
		return "im"
	case 'G':
		return "mpim"
	case 'C':
		return "channel"
	default:
		return ""
	}
}

// ToChannel converts a slack.Channel wire type into the public Channel record.
func ToChannel(ch slack.Channel) Channel {
	return Channel{
		ID:         ch.ID,
		Name:       ch.Name,
		Type:       ChannelTypeFor(ch),
		IsPrivate:  ch.IsPrivate,
		IsArchived: ch.IsArchived,
		NumMembers: ch.NumMembers,
		Topic:      ch.Topic.Value,
		Purpose:    ch.Purpose.Value,
		UserID:     ch.User,
	}
}

// ToChannelRef returns a compact reference suitable for embedding on Message.
func ToChannelRef(ch slack.Channel) *ChannelRef {
	return &ChannelRef{
		ID:   ch.ID,
		Name: ch.Name,
		Type: ChannelTypeFor(ch),
	}
}

// ToUser converts a slack.User wire type into the public User record.
func ToUser(u slack.User) User {
	return User{
		ID:          u.ID,
		Name:        u.Name,
		RealName:    u.RealName,
		DisplayName: u.Profile.DisplayName,
		Email:       u.Profile.Email,
		Title:       u.Profile.Title,
		TZ:          u.TZ,
		IsBot:       u.IsBot,
		Deleted:     u.Deleted,
	}
}

// ToFile converts a slack.File wire type into the public File record.
func ToFile(f slack.File) File {
	var created string
	if f.Created > 0 {
		created = time.Unix(f.Created, 0).UTC().Format(time.RFC3339)
	}
	return File{
		ID:              f.ID,
		Name:            f.Name,
		Title:           f.Title,
		Mimetype:        f.Mimetype,
		Filetype:        f.Filetype,
		PrettyType:      f.PrettyType,
		Mode:            f.Mode,
		Size:            f.Size,
		User:            f.User,
		IsPublic:        f.IsPublic,
		URLPrivate:      f.URLPrivate,
		Permalink:       f.Permalink,
		PermalinkPublic: f.PermalinkPublic,
		FileAccess:      f.FileAccess,
		Created:         created,
	}
}

// ToFileRef returns a compact file reference suitable for embedding on Message.
func ToFileRef(f slack.File) FileRef {
	return FileRef{
		ID:        f.ID,
		Name:      f.Name,
		Title:     f.Title,
		Mimetype:  f.Mimetype,
		Permalink: f.Permalink,
	}
}

// MessageConverter converts a slack.Message to the public Message record. It
// needs a resolver for user display names and formatted text, plus optional
// channel context for embedded references and workspace host for citations.
type MessageConverter struct {
	Resolver  *slack.Resolver
	Channel   *ChannelRef
	Workspace string
}

// Convert turns one slack.Message into the public Message record.
func (mc MessageConverter) Convert(m slack.Message) Message {
	var display string
	var userID string
	if strings.TrimSpace(m.User) != "" {
		userID = m.User
		if mc.Resolver != nil {
			display = mc.Resolver.ResolveUser(m.User)
		}
	}

	text := m.Text
	if mc.Resolver != nil {
		text = mc.Resolver.FormatText(m.Text)
	}

	var files []FileRef
	if len(m.Files) > 0 {
		files = make([]FileRef, 0, len(m.Files))
		for _, f := range m.Files {
			files = append(files, ToFileRef(f))
		}
	}

	ch := mc.Channel
	if m.Channel != nil && (ch == nil || ch.ID == "") {
		ch = &ChannelRef{ID: m.Channel.ID, Name: m.Channel.Name}
	}

	return Message{
		TS:         m.TS,
		ThreadTS:   m.ThreadTS,
		Type:       m.Type,
		User:       display,
		UserID:     userID,
		Text:       text,
		TextRaw:    m.Text,
		Channel:    ch,
		Workspace:  mc.Workspace,
		Permalink:  m.Permalink,
		ReplyCount: m.ReplyCount,
		Files:      files,
	}
}

// ConvertAll converts a slice of slack.Message records in order.
func (mc MessageConverter) ConvertAll(msgs []slack.Message) []Message {
	out := make([]Message, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, mc.Convert(m))
	}
	return out
}
