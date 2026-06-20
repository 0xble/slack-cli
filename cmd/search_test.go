package cmd

import (
	"strings"
	"testing"

	"github.com/lox/slack-cli/internal/slack"
)

func TestSearchMatchToMessage_PureFields(t *testing.T) {
	match := slack.SearchMatch{
		Type:      "message",
		User:      "U1",
		Username:  "alice",
		Text:      "hello <@U2>",
		TS:        "100.000001",
		Channel:   slack.SearchChannel{ID: "C1", Name: "general"},
		Permalink: "https://example.slack.com/archives/C1/p100000001",
	}

	got := searchMatchToMessage(nil, match, false)

	if got.TS != "100.000001" || got.UserID != "U1" || got.User != "alice" {
		t.Fatalf("unexpected fields: %+v", got)
	}
	// Compact default: text_raw and type are omitted. Verbose covered below.
	if got.Text != "hello <@U2>" {
		t.Fatalf("expected text passthrough when resolver is nil, got %q", got.Text)
	}
	if got.TextRaw != "" {
		t.Fatalf("expected text_raw omitted in compact shape, got %q", got.TextRaw)
	}
	if got.Type != "" {
		t.Fatalf("expected type omitted in compact shape, got %q", got.Type)
	}
	if got.Channel == nil || got.Channel.ID != "C1" || got.Channel.Name != "general" {
		t.Fatalf("expected channel populated: %+v", got.Channel)
	}
	if got.Channel.Type != "" {
		t.Fatalf("expected empty channel.type for C-prefixed ID without resolver, got %q", got.Channel.Type)
	}
	if got.Workspace != "example.slack.com" {
		t.Fatalf("expected workspace extracted from permalink, got %q", got.Workspace)
	}
	if got.Permalink != match.Permalink {
		t.Fatalf("expected permalink passthrough, got %q", got.Permalink)
	}
}

func TestSearchMatchToMessage_VerboseRestoresTypeAndRaw(t *testing.T) {
	match := slack.SearchMatch{
		Type: "message",
		User: "U1",
		Text: "hello <@U2>",
		TS:   "100.000001",
	}
	got := searchMatchToMessage(nil, match, true)
	if got.Type != "message" {
		t.Fatalf("expected type restored under verbose, got %q", got.Type)
	}
	if got.TextRaw != "hello <@U2>" {
		t.Fatalf("expected text_raw restored under verbose, got %q", got.TextRaw)
	}
}

func TestSearchMatchToMessage_DMChannelType(t *testing.T) {
	match := slack.SearchMatch{
		User:    "U1",
		Text:    "hey",
		TS:      "100.000001",
		Channel: slack.SearchChannel{ID: "D1"},
	}
	got := searchMatchToMessage(nil, match, false)
	if got.Channel == nil || got.Channel.Type != "im" {
		t.Fatalf("expected DM match channel type 'im', got %+v", got.Channel)
	}
}

func TestSearchMatchToMessage_FallsBackToRawWhenNoResolver(t *testing.T) {
	match := slack.SearchMatch{
		Text: "see <https://example.com|link>",
		TS:   "100",
	}
	got := searchMatchToMessage(nil, match, false)
	if !strings.Contains(got.Text, "<https://example.com") {
		t.Fatalf("expected raw angle-bracket form without resolver, got %q", got.Text)
	}
}
