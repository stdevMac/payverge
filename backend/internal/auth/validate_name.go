package auth

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxNameLength caps a user-supplied display name. Real full names comfortably
// fit; the cap bounds abuse of the name in system-generated emails that echo it.
const maxNameLength = 100

// ValidateName enforces basic hygiene on a user-supplied display name so it
// cannot smuggle newlines, control characters, or clickable URLs into the
// verification email that greets the registrant by name. Escaping already
// neutralizes HTML/script injection; this additionally stops the name from
// carrying an auto-linkified URL (a weak phishing/abuse vector) or control
// characters. Returns a user-facing error describing the first problem found.
func ValidateName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return errors.New("name is required")
	}
	if utf8.RuneCountInString(trimmed) > maxNameLength {
		return fmt.Errorf("name must be %d characters or fewer", maxNameLength)
	}
	for _, r := range trimmed {
		if unicode.IsControl(r) {
			return errors.New("name contains invalid characters")
		}
	}
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, "://") || strings.Contains(lower, "www.") {
		return errors.New("name cannot contain a URL")
	}
	return nil
}
