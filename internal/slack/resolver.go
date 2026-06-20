package slack

import (
	"strings"

	"github.com/enescakir/emoji"
)

// Resolver resolves Slack user IDs and channel IDs to human-readable names,
// and formats message text by replacing mentions and emoji shortcodes.
// Results are cached for the lifetime of the Resolver.
type Resolver struct {
	client                    *Client
	userCache                 map[string]string
	channelCache              map[string]string
	channelInfoCache          map[string]*Channel // nil value = negative cache
	channelInfoLookupDisabled bool
}

// NewResolver creates a Resolver that uses the given client for API lookups.
func NewResolver(client *Client) *Resolver {
	return &Resolver{
		client:           client,
		userCache:        make(map[string]string),
		channelCache:     make(map[string]string),
		channelInfoCache: make(map[string]*Channel),
	}
}

// PreloadChannels fetches the full conversations list and
// populates the channel info cache so later ResolveChannelInfo /
// ResolveChannel / ChannelRefFromID calls do not trigger per-channel API
// lookups. Callers use this on hot paths (e.g. search results spanning
// many channels) to avoid an N+1 conversations.info pattern.
func (r *Resolver) PreloadChannels(types string) error {
	if r.client == nil {
		return nil
	}

	cursor := ""
	seenCursors := make(map[string]bool)
	for {
		resp, err := r.client.ListConversationsPage(types, 1000, cursor)
		if err != nil {
			return err
		}
		for i := range resp.Channels {
			ch := resp.Channels[i]
			r.channelInfoCache[ch.ID] = &ch
			if ch.Name != "" {
				r.channelCache[ch.ID] = ch.Name
			}
		}

		cursor = strings.TrimSpace(resp.ResponseMetadata.NextCursor)
		if cursor == "" {
			return nil
		}
		if seenCursors[cursor] {
			return nil
		}
		seenCursors[cursor] = true
	}
}

// DisableChannelInfoLookup prevents lazy conversations.info lookups while
// still allowing already-cached channel metadata to be used.
func (r *Resolver) DisableChannelInfoLookup() {
	r.channelInfoLookupDisabled = true
}

// ResolveChannelInfo returns the full Channel record for the given ID,
// cached for the Resolver's lifetime. Returns nil if the lookup fails
// (for example because of missing scope or an unknown ID), which lets
// callers fall back to best-effort inference.
func (r *Resolver) ResolveChannelInfo(channelID string) *Channel {
	if channelID == "" {
		return nil
	}
	if info, ok := r.channelInfoCache[channelID]; ok {
		return info
	}
	if r.channelInfoLookupDisabled || r.client == nil {
		return nil
	}

	channel, err := r.client.GetConversationInfo(channelID)
	if err != nil {
		r.channelInfoCache[channelID] = nil
		return nil
	}

	r.channelInfoCache[channelID] = channel
	if channel.Name != "" {
		r.channelCache[channelID] = channel.Name
	}
	return channel
}

// ResolveUser returns a display name for the given user ID.
// It prefers DisplayName, then RealName, then Username.
// Returns "bot" for empty IDs and falls back to the raw ID on error.
func (r *Resolver) ResolveUser(userID string) string {
	if userID == "" {
		return "bot"
	}

	if name, ok := r.userCache[userID]; ok {
		return name
	}

	user, err := r.client.GetUserInfo(userID)
	if err != nil {
		r.userCache[userID] = userID
		return userID
	}

	name := user.Profile.DisplayName
	if name == "" {
		name = user.RealName
	}
	if name == "" {
		name = user.Name
	}

	r.userCache[userID] = name
	return name
}

// ResolveChannel returns a channel name for the given channel ID.
// Falls back to the raw ID on error.
func (r *Resolver) ResolveChannel(channelID string) string {
	if name, ok := r.channelCache[channelID]; ok {
		return name
	}

	channel, err := r.client.GetConversationInfo(channelID)
	if err != nil {
		r.channelCache[channelID] = channelID
		return channelID
	}

	r.channelCache[channelID] = channel.Name
	return channel.Name
}

// FormatText replaces user mentions (<@U123>), channel mentions (<#C123|name>),
// URL links (<http://...|label>), and emoji shortcodes in message text.
func (r *Resolver) FormatText(text string) string {
	result := text

	// Resolve user mentions: <@U123ABC> -> @username
	pos := 0
	for pos < len(result) {
		start := strings.Index(result[pos:], "<@")
		if start == -1 {
			break
		}
		start += pos
		end := strings.Index(result[start:], ">")
		if end == -1 {
			break
		}
		end += start

		userID := result[start+2 : end]

		// Handle <@U123|name> format — keep fallback name
		var fallbackName string
		if pipeIdx := strings.Index(userID, "|"); pipeIdx != -1 {
			fallbackName = userID[pipeIdx+1:]
			userID = userID[:pipeIdx]
		}

		displayName := r.ResolveUser(userID)
		if displayName == userID && fallbackName != "" {
			displayName = fallbackName
		}
		replacement := "@" + displayName
		result = result[:start] + replacement + result[end+1:]
		pos = start + len(replacement)
	}

	// Resolve channel mentions: <#C123ABC|channel-name> -> #channel-name
	pos = 0
	for pos < len(result) {
		start := strings.Index(result[pos:], "<#")
		if start == -1 {
			break
		}
		start += pos
		end := strings.Index(result[start:], ">")
		if end == -1 {
			break
		}
		end += start

		channelPart := result[start+2 : end]

		var channelName string
		if pipeIdx := strings.Index(channelPart, "|"); pipeIdx != -1 {
			channelName = channelPart[pipeIdx+1:]
		} else {
			channelName = r.ResolveChannel(channelPart)
		}

		replacement := "#" + channelName
		result = result[:start] + replacement + result[end+1:]
		pos = start + len(replacement)
	}

	// Resolve URL links: <http://example.com|label> -> label (http://example.com)
	pos = 0
	for pos < len(result) {
		start := strings.Index(result[pos:], "<http")
		if start == -1 {
			break
		}
		start += pos
		end := strings.Index(result[start:], ">")
		if end == -1 {
			break
		}
		end += start

		linkContent := result[start+1 : end]

		var replacement string
		if pipeIdx := strings.Index(linkContent, "|"); pipeIdx != -1 {
			linkURL := linkContent[:pipeIdx]
			label := linkContent[pipeIdx+1:]
			replacement = label + " (" + linkURL + ")"
		} else {
			replacement = linkContent
		}
		result = result[:start] + replacement + result[end+1:]
		pos = start + len(replacement)
	}

	// Convert emoji shortcodes: :smile: -> 😊
	result = emoji.Parse(result)

	return result
}
