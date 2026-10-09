package emails

import (
	"html"
	"regexp"
	"strings"
)

// Plain-text derivation for HTML-only messages. Templates render HTML only;
// the SMTP provider needs a text/plain alternative (spam filters penalise
// HTML-only mail) and the log provider needs a readable preview. This is a
// deliberately small, dependency-free converter — good enough for our own
// templates, not a general-purpose HTML renderer.
var (
	htmlCommentRe = regexp.MustCompile(`(?s)<!--.*?-->`)
	htmlDropRes   = []*regexp.Regexp{
		regexp.MustCompile(`(?is)<head\b[^>]*>.*?</head\s*>`),
		regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style\s*>`),
		regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script\s*>`),
		regexp.MustCompile(`(?is)<title\b[^>]*>.*?</title\s*>`),
	}
	htmlAnchorRe   = regexp.MustCompile(`(?is)<a\b[^>]*?\bhref\s*=\s*["']([^"']*)["'][^>]*>(.*?)</a\s*>`)
	htmlBreakRe    = regexp.MustCompile(`(?i)<\s*(br|hr)\b[^>]*>|<\s*/\s*(p|div|tr|table|h[1-6]|li|ul|ol|blockquote|section|header|footer)\s*>`)
	htmlTagRe      = regexp.MustCompile(`(?s)<[^>]*>`)
	hrefAttrRe     = regexp.MustCompile(`(?i)\bhref\s*=\s*["']([^"']+)["']`)
	bareURLRe      = regexp.MustCompile(`https?://[^\s"'<>()]+`)
	horizontalWSRe = regexp.MustCompile(`[ \t\f\v\x{00a0}]+`)
	blankRunRe     = regexp.MustCompile(`\n{3,}`)
)

// htmlToPlainText renders an HTML email body as readable plain text. Anchor
// targets are kept as "label (url)" so verification / reset links survive.
func htmlToPlainText(body string) string {
	s := htmlCommentRe.ReplaceAllString(body, "")
	for _, re := range htmlDropRes {
		s = re.ReplaceAllString(s, "")
	}
	s = htmlAnchorRe.ReplaceAllStringFunc(s, func(m string) string {
		parts := htmlAnchorRe.FindStringSubmatch(m)
		href := strings.TrimSpace(html.UnescapeString(parts[1]))
		label := strings.TrimSpace(html.UnescapeString(htmlTagRe.ReplaceAllString(parts[2], "")))
		label = horizontalWSRe.ReplaceAllString(label, " ")
		switch {
		case href == "" || strings.HasPrefix(strings.ToLower(href), "mailto:"):
			return label
		case label == "" || label == href:
			return href
		default:
			return label + " (" + href + ")"
		}
	})
	s = htmlBreakRe.ReplaceAllString(s, "\n")
	s = htmlTagRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")

	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(horizontalWSRe.ReplaceAllString(line, " "))
	}
	s = strings.Join(lines, "\n")
	s = blankRunRe.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

// plainTextBody returns the message's text body, deriving one from the HTML
// body when the caller supplied only HTML.
func plainTextBody(msg EmailMessage) string {
	if strings.TrimSpace(msg.TextBody) != "" {
		return msg.TextBody
	}
	if strings.TrimSpace(msg.HTMLBody) == "" {
		return ""
	}
	return htmlToPlainText(msg.HTMLBody)
}

// extractLinks returns the distinct http(s) links in the message (HTML hrefs
// first, then bare URLs in the text body), capped at max entries.
func extractLinks(msg EmailMessage, max int) []string {
	seen := map[string]bool{}
	var links []string
	add := func(raw string) {
		link := strings.TrimSpace(html.UnescapeString(raw))
		lower := strings.ToLower(link)
		if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
			return
		}
		if seen[link] || len(links) >= max {
			return
		}
		seen[link] = true
		links = append(links, link)
	}
	for _, m := range hrefAttrRe.FindAllStringSubmatch(msg.HTMLBody, -1) {
		add(m[1])
	}
	for _, m := range bareURLRe.FindAllString(msg.TextBody, -1) {
		add(strings.TrimRight(m, ".,;:!?"))
	}
	return links
}

// truncateRunes shortens s to at most max runes, marking the cut.
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…[truncated]"
}
