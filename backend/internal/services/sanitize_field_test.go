package services

import (
	"strings"
	"testing"
)

func TestSanitizeField_StripsControlAndCaps(t *testing.T) {
	in := "Pizza\nMargherita\t<inject>\x07"
	got := sanitizeField(in, 12)
	if strings.ContainsAny(got, "\n\t\x07") {
		t.Fatalf("expected control chars stripped, got %q", got)
	}
	if len([]rune(got)) > 12 {
		t.Fatalf("expected cap at 12 runes, got %q (len %d)", got, len([]rune(got)))
	}
}
