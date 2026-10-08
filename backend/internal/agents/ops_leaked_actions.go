package agents

import (
	"encoding/json"
	"regexp"
	"strings"
)

// leakedKVBlockRe matches an action-metadata key-value run leaked into answer
// prose, e.g. "href: /business/2/dashboard?tab=menu kind: primary disabled:
// false disabled_reason: null". The run starts at an "href:" key and captures
// the whole span through an optional trailing disabled_reason value. Keys are
// whitespace- or newline-separated.
var leakedKVBlockRe = regexp.MustCompile(
	`href:\s*(\S+)` +
		`(?:\s+kind:\s*(\S+))?` +
		`(?:\s+disabled:\s*(true|false))?` +
		`(?:\s+disabled_reason:\s*(null|"[^"]*"|[^\n]+))?`,
)

// leakedJSONObjRe matches a serialized JSON action object embedded in prose:
// a brace-delimited object that contains an "href" key. Non-greedy (no inner
// braces) so adjacent objects are matched separately.
var leakedJSONObjRe = regexp.MustCompile(`\{[^{}]*"href"[^{}]*\}`)

var multiSpaceRe = regexp.MustCompile(`[ \t]{2,}`)

// isSafeHref reports whether an action href is safe to render or navigate to:
// an internal absolute path ("/..." but not protocol-relative "//host"), or an
// http(s) URL. Everything else — javascript:, data:, mailto:, etc. — is rejected
// so semi-trusted model output cannot produce an executable or off-origin link.
func isSafeHref(href string) bool {
	h := strings.TrimSpace(href)
	if h == "" || strings.HasPrefix(h, "//") {
		return false
	}
	if strings.HasPrefix(h, "/") {
		return true
	}
	lower := strings.ToLower(h)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

// extractLeakedActions detects action metadata the model leaked into the answer
// string — key-value runs (href:/kind:/disabled:/disabled_reason:) and embedded
// JSON action objects — moves each into a proper ActionLink, and returns the
// answer with those blocks stripped. Label falls back to the text immediately
// preceding the block, else the last href path segment. Kind is normalized via
// normalizeKind. Empty-href candidates are dropped. Dedupe is left to the
// caller (NormalizeOpsResponse) which merges these with resp.Actions.
func extractLeakedActions(answer string, businessID uint) (string, []ActionLink) {
	extracted := make([]ActionLink, 0)
	clean := answer

	// 1) Embedded JSON action objects first (they may themselves contain the
	//    key names the KV regex looks for; parsing them structurally is safer).
	for _, loc := range leakedJSONObjRe.FindAllStringIndex(clean, -1) {
		raw := clean[loc[0]:loc[1]]
		var obj struct {
			Label          string          `json:"label"`
			Href           string          `json:"href"`
			Kind           string          `json:"kind"`
			Disabled       bool            `json:"disabled"`
			DisabledReason json.RawMessage `json:"disabled_reason"`
		}
		if err := json.Unmarshal([]byte(raw), &obj); err != nil {
			continue
		}
		href := strings.TrimRight(strings.TrimSpace(obj.Href), ".,;:!?")
		if href == "" || !isSafeHref(href) {
			continue
		}
		label := strings.TrimSpace(obj.Label)
		if label == "" {
			label = labelFromContext("", href)
		}
		extracted = append(extracted, ActionLink{
			Label:          label,
			Href:           href,
			Kind:           normalizeKind(obj.Kind, href),
			Disabled:       obj.Disabled,
			DisabledReason: rawJSONString(obj.DisabledReason),
		})
	}
	clean = leakedJSONObjRe.ReplaceAllString(clean, "")

	// 2) Key-value runs. Capture the preceding text (for label fallback) before
	//    stripping. Iterate over matches so multiple leaked actions are handled.
	matches := leakedKVBlockRe.FindAllStringSubmatchIndex(clean, -1)
	var b strings.Builder
	last := 0
	for _, m := range matches {
		start, end := m[0], m[1]
		preceding := clean[last:start]
		b.WriteString(preceding)
		last = end

		sub := leakedKVBlockRe.FindStringSubmatch(clean[start:end])
		href := strings.TrimRight(strings.TrimSpace(sub[1]), ".,;:!?")
		if href == "" || !isSafeHref(href) {
			// Not a safe/real action link (e.g. javascript:/data: schemes, or
			// prose that merely mentions "href:"). Keep the matched text in the
			// clean output verbatim instead of stripping it, and emit nothing.
			b.WriteString(clean[start:end])
			continue
		}
		disabled := strings.EqualFold(strings.TrimSpace(sub[3]), "true")
		reason := normalizeReason(sub[4])
		extracted = append(extracted, ActionLink{
			Label:          labelFromContext(preceding, href),
			Href:           href,
			Kind:           normalizeKind(sub[2], href),
			Disabled:       disabled,
			DisabledReason: reason,
		})
	}
	b.WriteString(clean[last:])
	clean = b.String()

	clean = strings.TrimSpace(collapseWhitespace(clean))
	return clean, extracted
}

// normalizeKind maps an unknown/blank kind to the enum. Internal "/" href ->
// navigate; http(s) href -> external. Known kinds (navigate/external/handoff)
// pass through unchanged.
func normalizeKind(kind, href string) string {
	k := strings.ToLower(strings.TrimSpace(kind))
	switch k {
	case "navigate", "external", "handoff":
		return k
	}
	h := strings.TrimSpace(href)
	if strings.HasPrefix(h, "http://") || strings.HasPrefix(h, "https://") {
		return "external"
	}
	return "navigate"
}

// labelFromContext derives a button label from the text preceding the leaked
// block, else the last non-empty path segment of the href. Falls back to "Open".
func labelFromContext(preceding, href string) string {
	trimmed := strings.TrimSpace(preceding)
	trimmed = strings.Trim(trimmed, ".:;,-— \t\n")
	if trimmed != "" {
		lines := strings.Split(trimmed, "\n")
		lastLine := strings.TrimSpace(lines[len(lines)-1])
		if lastLine != "" && len(lastLine) <= 60 {
			return lastLine
		}
	}
	if seg := lastPathSegment(href); seg != "" {
		return seg
	}
	return "Open"
}

// lastPathSegment returns a label-ish segment of an internal href: the tab=
// value when present, else the final path element (query stripped).
func lastPathSegment(href string) string {
	h := strings.TrimSpace(href)
	if h == "" {
		return ""
	}
	path := h
	query := ""
	if i := strings.IndexByte(h, '?'); i >= 0 {
		path = h[:i]
		query = h[i+1:]
	}
	if query != "" {
		for _, part := range strings.Split(query, "&") {
			if strings.HasPrefix(part, "tab=") {
				return strings.TrimPrefix(part, "tab=")
			}
		}
	}
	segs := strings.Split(strings.Trim(path, "/"), "/")
	for i := len(segs) - 1; i >= 0; i-- {
		if segs[i] != "" {
			return segs[i]
		}
	}
	return ""
}

// normalizeReason converts a captured disabled_reason token to a string:
// "null" -> "", a quoted value -> unquoted, else the raw token.
func normalizeReason(tok string) string {
	t := strings.TrimSpace(tok)
	if t == "" || strings.EqualFold(t, "null") {
		return ""
	}
	if len(t) >= 2 && t[0] == '"' && t[len(t)-1] == '"' {
		return t[1 : len(t)-1]
	}
	return t
}

// rawJSONString decodes a JSON disabled_reason value (string, null, or absent)
// to a Go string. null/absent -> "".
func rawJSONString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return ""
}

// collapseWhitespace squeezes runs of spaces/tabs left by stripped blocks and
// drops trailing blank lines, without touching single spaces.
func collapseWhitespace(s string) string {
	s = multiSpaceRe.ReplaceAllString(s, " ")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		out = append(out, strings.TrimRight(ln, " \t"))
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}
