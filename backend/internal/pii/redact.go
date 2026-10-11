// Package pii provides log-/storage-safe redaction of personally identifiable
// information (emails, phone numbers) from free-form text.
//
// Contract C3 (AI excellence campaign): Redact MUST NOT mangle ISO dates, date
// ranges, space/comma-grouped quantities, currency amounts, "#"-prefixed IDs, or
// percentages. Phone redaction requires both a >=9-digit run AND a phone-context
// signal (leading "+"/"(", or a phone keyword matched AS A WHOLE WORD within 16
// chars before the match — never a substring, so words like "was"/"software"/
// "award" that merely contain a keyword fragment do not trigger redaction).
package pii

import (
	"regexp"
	"strings"
)

const (
	redactedEmail = "[redacted-email]"
	redactedPhone = "[redacted-phone]"
)

var (
	emailRegex = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)

	phoneCandidateRegex = regexp.MustCompile(`[+(]?\d[\d().\-\s]{7,}\d`)

	isoDateRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

	parenAreaCodeRegex = regexp.MustCompile(`^\(\d{2,4}\)`)

	phoneKeywordRegex = regexp.MustCompile(
		`(?i)(^|[^\p{L}])(tel|phone|call|whatsapp|mobile|cell|tel[eé]fono|llamar|llam[aá]|m[oó]vil|celular)([^\p{L}]|$)`,
	)
)

// Redact replaces emails and phone numbers with [redacted-email]/[redacted-phone].
func Redact(s string) string {
	if s == "" {
		return s
	}
	s = emailRegex.ReplaceAllString(s, redactedEmail)
	return redactPhones(s)
}

func redactPhones(s string) string {
	locs := phoneCandidateRegex.FindAllStringIndex(s, -1)
	if locs == nil {
		return s
	}
	var b strings.Builder
	prev := 0
	for _, loc := range locs {
		start, end := loc[0], loc[1]
		match := s[start:end]
		if !isRedactablePhone(s, start, match) {
			continue
		}
		b.WriteString(s[prev:start])
		b.WriteString(redactedPhone)
		prev = end
	}
	if prev == 0 {
		return s
	}
	b.WriteString(s[prev:])
	return b.String()
}

func isRedactablePhone(full string, start int, match string) bool {
	trimmed := strings.TrimSpace(match)
	if isoDateRegex.MatchString(trimmed) {
		return false
	}
	if countDigits(trimmed) < 9 {
		return false
	}
	if start > 0 && full[start-1] == '#' {
		return false
	}
	if strings.HasPrefix(trimmed, "+") {
		return true
	}
	if strings.HasPrefix(trimmed, "(") && parenAreaCodeRegex.MatchString(trimmed) {
		return true
	}
	return hasPhoneKeywordContext(full, start)
}

func hasPhoneKeywordContext(full string, start int) bool {
	from := start - 16
	if from < 0 {
		from = 0
	}
	prefix := full[from:start]
	return phoneKeywordRegex.MatchString(prefix)
}

func countDigits(s string) int {
	n := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n++
		}
	}
	return n
}
