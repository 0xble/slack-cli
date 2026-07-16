package slack

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	orderedListLine = regexp.MustCompile(`^\s*\d+[.)]\s+(.+)$`)
	bulletListLine  = regexp.MustCompile(`^\s*[-+*]\s+(.+)$`)
)

// BuildRichTextBlocks converts the practical Markdown subset used in Slack
// drafts into Slack's native rich_text block structure. Ordered and bulleted
// Markdown lists become rich_text_list elements rather than flat text lines.
func BuildRichTextBlocks(markdown string) (json.RawMessage, error) {
	markdown = strings.ReplaceAll(markdown, "\r\n", "\n")
	if strings.TrimSpace(markdown) == "" {
		return nil, fmt.Errorf("rich message text is required")
	}

	lines := strings.Split(markdown, "\n")
	elements := make([]any, 0)
	for i := 0; i < len(lines); {
		if strings.TrimSpace(lines[i]) == "" {
			i++
			continue
		}

		if style, item, ok := parseListLine(lines[i]); ok {
			items := make([]any, 0)
			for i < len(lines) {
				itemStyle, itemText, itemOK := parseListLine(lines[i])
				if !itemOK || itemStyle != style {
					break
				}
				items = append(items, map[string]any{
					"type":     "rich_text_section",
					"elements": parseInlineRichText(itemText),
				})
				i++
			}
			elements = append(elements, map[string]any{
				"type":     "rich_text_list",
				"style":    style,
				"indent":   0,
				"border":   0,
				"elements": items,
			})
			_ = item
			continue
		}

		paragraph := make([]string, 0)
		for i < len(lines) && strings.TrimSpace(lines[i]) != "" {
			if _, _, isList := parseListLine(lines[i]); isList {
				break
			}
			paragraph = append(paragraph, lines[i])
			i++
		}
		text := strings.Join(paragraph, "\n")
		if hasNonBlankLineAfter(lines, i) {
			text += "\n"
		}
		elements = append(elements, map[string]any{
			"type":     "rich_text_section",
			"elements": parseInlineRichText(text),
		})
	}

	blocks, err := json.Marshal([]any{map[string]any{
		"type":     "rich_text",
		"elements": elements,
	}})
	if err != nil {
		return nil, fmt.Errorf("encode rich text blocks: %w", err)
	}
	return blocks, nil
}

func parseListLine(line string) (style, item string, ok bool) {
	if match := orderedListLine.FindStringSubmatch(line); len(match) == 2 {
		return "ordered", match[1], true
	}
	if match := bulletListLine.FindStringSubmatch(line); len(match) == 2 {
		return "bullet", match[1], true
	}
	return "", "", false
}

func hasNonBlankLineAfter(lines []string, index int) bool {
	for ; index < len(lines); index++ {
		if strings.TrimSpace(lines[index]) != "" {
			return true
		}
	}
	return false
}

func parseInlineRichText(text string) []any {
	result := make([]any, 0)
	for len(text) > 0 {
		switch {
		case strings.HasPrefix(text, "**"):
			if end := strings.Index(text[2:], "**"); end >= 0 {
				content := text[2 : 2+end]
				result = append(result, richTextSpan(content, map[string]any{"bold": true}))
				text = text[2+end+2:]
				continue
			}
		case strings.HasPrefix(text, "`"):
			if end := strings.Index(text[1:], "`"); end >= 0 {
				content := text[1 : 1+end]
				result = append(result, richTextSpan(content, map[string]any{"code": true}))
				text = text[1+end+1:]
				continue
			}
		case strings.HasPrefix(text, "["):
			if closeLabel := strings.Index(text, "]("); closeLabel > 0 {
				if closeURL := strings.Index(text[closeLabel+2:], ")"); closeURL >= 0 {
					label := text[1:closeLabel]
					url := text[closeLabel+2 : closeLabel+2+closeURL]
					result = append(result, map[string]any{"type": "link", "url": url, "text": label})
					text = text[closeLabel+2+closeURL+1:]
					continue
				}
			}
		case strings.HasPrefix(text, "<"):
			if end := strings.Index(text, ">"); end > 0 {
				if element, ok := parseSlackInlineToken(text[1:end]); ok {
					result = append(result, element)
					text = text[end+1:]
					continue
				}
			}
		case strings.HasPrefix(text, "*"):
			if end := strings.Index(text[1:], "*"); end >= 0 {
				content := text[1 : 1+end]
				result = append(result, richTextSpan(content, map[string]any{"italic": true}))
				text = text[1+end+1:]
				continue
			}
		}

		next := nextInlineMarker(text[1:])
		if next < 0 {
			result = appendPlainText(result, text)
			break
		}
		next++
		result = appendPlainText(result, text[:next])
		text = text[next:]
	}
	return result
}

func parseSlackInlineToken(token string) (map[string]any, bool) {
	switch {
	case strings.HasPrefix(token, "!subteam^"):
		id := strings.SplitN(strings.TrimPrefix(token, "!subteam^"), "|", 2)[0]
		if id != "" {
			return map[string]any{"type": "usergroup", "usergroup_id": id}, true
		}
	case strings.HasPrefix(token, "@"):
		id := strings.SplitN(strings.TrimPrefix(token, "@"), "|", 2)[0]
		if id != "" {
			return map[string]any{"type": "user", "user_id": id}, true
		}
	case strings.HasPrefix(token, "#"):
		id := strings.SplitN(strings.TrimPrefix(token, "#"), "|", 2)[0]
		if id != "" {
			return map[string]any{"type": "channel", "channel_id": id}, true
		}
	case strings.HasPrefix(token, "http://"), strings.HasPrefix(token, "https://"):
		parts := strings.SplitN(token, "|", 2)
		element := map[string]any{"type": "link", "url": parts[0]}
		if len(parts) == 2 {
			element["text"] = parts[1]
		}
		return element, true
	}
	return nil, false
}

func richTextSpan(text string, style map[string]any) map[string]any {
	return map[string]any{"type": "text", "text": text, "style": style}
}

func appendPlainText(elements []any, text string) []any {
	if text == "" {
		return elements
	}
	return append(elements, map[string]any{"type": "text", "text": text})
}

func nextInlineMarker(text string) int {
	best := -1
	for _, marker := range []string{"*", "`", "[", "<"} {
		if index := strings.Index(text, marker); index >= 0 && (best < 0 || index < best) {
			best = index
		}
	}
	return best
}
