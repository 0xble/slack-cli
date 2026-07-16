package slack

import (
	"encoding/json"
	"testing"
)

func TestBuildRichTextBlocksCreatesNativeOrderedList(t *testing.T) {
	blocks, err := BuildRichTextBlocks("Hey <!subteam^S123>, I had questions.\n\n1. First **question**\n2. Second [link](https://example.com)")
	if err != nil {
		t.Fatalf("BuildRichTextBlocks returned error: %v", err)
	}

	var got []map[string]any
	if err := json.Unmarshal(blocks, &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(got) != 1 || got[0]["type"] != "rich_text" {
		t.Fatalf("unexpected blocks: %+v", got)
	}
	elements := got[0]["elements"].([]any)
	if len(elements) != 2 {
		t.Fatalf("expected intro and list, got %+v", elements)
	}
	intro := elements[0].(map[string]any)
	introElements := intro["elements"].([]any)
	if introElements[1].(map[string]any)["type"] != "usergroup" {
		t.Fatalf("expected native usergroup mention, got %+v", introElements)
	}
	list := elements[1].(map[string]any)
	if list["type"] != "rich_text_list" || list["style"] != "ordered" {
		t.Fatalf("expected native ordered list, got %+v", list)
	}
	items := list["elements"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected two list items, got %+v", items)
	}
	firstParts := items[0].(map[string]any)["elements"].([]any)
	if firstParts[1].(map[string]any)["style"].(map[string]any)["bold"] != true {
		t.Fatalf("expected bold span, got %+v", firstParts)
	}
	secondParts := items[1].(map[string]any)["elements"].([]any)
	if secondParts[1].(map[string]any)["type"] != "link" {
		t.Fatalf("expected native link, got %+v", secondParts)
	}
}

func TestBuildRichTextBlocksCreatesNativeBulletList(t *testing.T) {
	blocks, err := BuildRichTextBlocks("- Alpha\n- Beta")
	if err != nil {
		t.Fatalf("BuildRichTextBlocks returned error: %v", err)
	}
	var got []map[string]any
	if err := json.Unmarshal(blocks, &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	elements := got[0]["elements"].([]any)
	list := elements[0].(map[string]any)
	if list["style"] != "bullet" || len(list["elements"].([]any)) != 2 {
		t.Fatalf("unexpected bullet list: %+v", list)
	}
}

func TestBuildRichTextBlocksRejectsEmptyInput(t *testing.T) {
	if _, err := BuildRichTextBlocks("  \n"); err == nil {
		t.Fatal("expected empty input error")
	}
}
