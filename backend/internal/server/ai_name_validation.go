package server

import (
	"fmt"
	"strings"
	"unicode"
)

// MaxAiNameLen bounds the guest-visible AI waiter display name (L4-5).
// Matches director prompt sanitizer width (SanitizePromptField(..., 40)).
const MaxAiNameLen = 40

// NormalizeAndValidateAiName trims whitespace, rejects control characters, and
// enforces MaxAiNameLen. Returns the normalized name or an error for 400.
func NormalizeAndValidateAiName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", nil
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("ai_name must not contain control characters")
		}
	}
	if len([]rune(name)) > MaxAiNameLen {
		return "", fmt.Errorf("ai_name must be at most %d characters", MaxAiNameLen)
	}
	return name, nil
}
