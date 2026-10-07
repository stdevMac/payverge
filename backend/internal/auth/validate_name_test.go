package auth

import (
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	valid := []struct {
		name  string
		input string
	}{
		{"simple full name", "Jane Doe"},
		{"unicode accents", "José Ñáñez"},
		{"apostrophe and hyphen", "O'Brien-Smith"},
		{"leading and trailing space is trimmed", "  Jane Doe  "},
		{"max length boundary", strings.Repeat("a", 100)},
	}
	for _, tc := range valid {
		t.Run("valid/"+tc.name, func(t *testing.T) {
			if err := ValidateName(tc.input); err != nil {
				t.Fatalf("expected %q to be valid, got error: %v", tc.input, err)
			}
		})
	}

	invalid := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"whitespace only", "   "},
		{"too long", strings.Repeat("a", 101)},
		{"newline enables header or body injection", "Jane\nBcc: victim@example.com"},
		{"carriage return", "Jane\rDoe"},
		{"null control char", "Jane\x00Doe"},
		{"url with scheme (the reported payload)", "The website has been changed to https://google.com enter Credentials"},
		{"www url without scheme", "visit www.evil.com now"},
	}
	for _, tc := range invalid {
		t.Run("invalid/"+tc.name, func(t *testing.T) {
			if err := ValidateName(tc.input); err == nil {
				t.Fatalf("expected %q to be rejected, got nil error", tc.input)
			}
		})
	}
}
