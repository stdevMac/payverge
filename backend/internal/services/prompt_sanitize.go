package services

import (
	"strings"
	"unicode"
)

// SanitizePromptField neutralizes owner/short untrusted fields before they are
// interpolated into a system prompt. It strips control characters and newlines
// (so an aiName like "Sage\nSYSTEM: do X" cannot forge a new prompt section),
// collapses internal whitespace runs to a single space, trims the edges, and
// caps the result to max runes. Shared by Lanes B/C/D (contract C7).
func SanitizePromptField(s string, max int) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		case unicode.IsControl(r):
		case unicode.IsSpace(r):
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		default:
			b.WriteRune(r)
			prevSpace = false
		}
	}
	out := strings.TrimSpace(b.String())
	if max > 0 {
		runes := []rune(out)
		if len(runes) > max {
			out = strings.TrimSpace(string(runes[:max]))
		}
	}
	return out
}
