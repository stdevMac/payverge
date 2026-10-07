package services

import (
	"strings"
	"testing"
)

func TestExtractionPrompt_PreservesSourceLanguage(t *testing.T) {
	p := buildExtractionPrompt(2)
	if !strings.Contains(p, "Preserve the menu's original language") {
		t.Fatalf("expected source-language preservation instruction, got:\n%s", p)
	}
	if !strings.Contains(p, "Process Page 1") || !strings.Contains(p, "Process Page 2") {
		t.Fatalf("expected per-page checklist, got:\n%s", p)
	}
}
