package slack

import (
	"regexp"
	"strings"
)

var (
	canvasUserMentionRegex    = regexp.MustCompile(`<a>@(U[A-Z0-9]+)</a>`)
	canvasChannelMentionRegex = regexp.MustCompile(`<a>#([CDG][A-Z0-9]+)</a>`)
	canvasEmojiRegex          = regexp.MustCompile(`<control[^>]*><img[^>]*alt="([^"]*)"[^>]*>[^<]*</img></control>`)
	canvasLinkRegex           = regexp.MustCompile(`<a[^>]*href="([^"]*)"[^>]*>([^<]*)</a>`)
	canvasTagRegex            = regexp.MustCompile(`<[^>]+>`)
	canvasSpaceRegex          = regexp.MustCompile(`[ \t]+`)
	canvasNewlineRegex        = regexp.MustCompile(`\n{3,}`)
)

func IsCanvasFile(file File) bool {
	return file.Filetype == "quip" || file.Mimetype == "application/vnd.slack-docs"
}

func CanvasHTMLToText(html string, userNames map[string]string) string {
	html = canvasUserMentionRegex.ReplaceAllStringFunc(html, func(match string) string {
		parts := canvasUserMentionRegex.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		if display, ok := userNames[parts[1]]; ok && strings.TrimSpace(display) != "" {
			return "@" + display
		}
		return match
	})
	html = canvasChannelMentionRegex.ReplaceAllString(html, "#$1")
	html = canvasEmojiRegex.ReplaceAllString(html, ":$1:")
	html = regexp.MustCompile(`<h1[^>]*>`).ReplaceAllString(html, "\n# ")
	html = regexp.MustCompile(`</h1>`).ReplaceAllString(html, "\n")
	html = regexp.MustCompile(`<h2[^>]*>`).ReplaceAllString(html, "\n## ")
	html = regexp.MustCompile(`</h2>`).ReplaceAllString(html, "\n")
	html = regexp.MustCompile(`<h3[^>]*>`).ReplaceAllString(html, "\n### ")
	html = regexp.MustCompile(`</h3>`).ReplaceAllString(html, "\n")
	html = regexp.MustCompile(`<p[^>]*>`).ReplaceAllString(html, "\n")
	html = regexp.MustCompile(`</p>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`<br\\s*/?>`).ReplaceAllString(html, "\n")
	html = regexp.MustCompile(`<ul[^>]*>`).ReplaceAllString(html, "\n")
	html = regexp.MustCompile(`</ul>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`<li[^>]*>`).ReplaceAllString(html, "\n- ")
	html = regexp.MustCompile(`</li>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`<div[^>]*>`).ReplaceAllString(html, "\n")
	html = regexp.MustCompile(`</div>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`<b>`).ReplaceAllString(html, "**")
	html = regexp.MustCompile(`</b>`).ReplaceAllString(html, "**")
	html = regexp.MustCompile(`<i>`).ReplaceAllString(html, "_")
	html = regexp.MustCompile(`</i>`).ReplaceAllString(html, "_")
	html = canvasLinkRegex.ReplaceAllString(html, "$2 ($1)")
	html = canvasTagRegex.ReplaceAllString(html, "")
	html = strings.NewReplacer(
		"&lt;", "<",
		"&gt;", ">",
		"&amp;", "&",
		"&quot;", "\"",
		"&#39;", "'",
		"&nbsp;", " ",
	).Replace(html)
	html = canvasNewlineRegex.ReplaceAllString(html, "\n\n")
	html = canvasSpaceRegex.ReplaceAllString(html, " ")
	html = strings.ReplaceAll(html, " \n", "\n")
	html = strings.ReplaceAll(html, "\n ", "\n")
	return strings.TrimSpace(html)
}
