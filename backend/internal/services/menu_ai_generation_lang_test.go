package services

import (
	"strings"
	"testing"
)

func TestBuildGenerationPrompt_IncludesOutputLanguage_ES(t *testing.T) {
	prompt := buildGenerationPrompt("es", "CONVERSATION HISTORY...", map[string]string{
		"business_type": "Restaurante",
		"cuisine":       "Mexicana",
	})
	if !strings.Contains(prompt, "Español") {
		t.Fatalf("expected output-language requirement naming Español, got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "All item names and descriptions MUST be written in Español") {
		t.Fatalf("expected requirements-block language line, got:\n%s", prompt)
	}
	lines := strings.Split(strings.TrimRight(prompt, "\n"), "\n")
	if !strings.Contains(lines[len(lines)-1], "Español") {
		t.Fatalf("expected final line to repeat output language, got last line: %q", lines[len(lines)-1])
	}
}

func TestBuildGenerationPrompt_DefaultsEnglish(t *testing.T) {
	prompt := buildGenerationPrompt("", "history", map[string]string{})
	if !strings.Contains(prompt, "English") {
		t.Fatalf("expected English default, got:\n%s", prompt)
	}
}
