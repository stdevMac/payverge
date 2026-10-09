package services

import (
	"embed"
	"fmt"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/locales"
)

// The AI-prompt surface contract mirrors the transactional-email one: a locale
// ships prompt assets (director_console + menu_wizard) if and only if its
// registry requiredSurfaces include "prompts". Director's Console and the Menu
// Wizard already filter selection on locales.IsPromptLocale; this test
// guarantees the embedded .md files actually exist for every prompt locale (so a
// promoted locale can't silently fall back to en.md) and that no orphan prompt
// family ships for a locale that isn't prompt-backed.
func TestPromptFamiliesMatchRegistryContract(t *testing.T) {
	categories := map[string]embed.FS{
		"director_console": directorPromptFS,
		"menu_wizard":      wizardPromptFS,
	}

	// Floor: the operator trio must stay prompt-backed (without forbidding a
	// future, deliberate expansion — the contract below stays self-maintaining).
	for _, code := range []string{"en", "es", "es-AR"} {
		if !locales.IsPromptLocale(code) {
			t.Fatalf("expected %s to be a prompt-backed locale", code)
		}
	}

	// Every prompt-required locale must have a file in every prompt category.
	for _, loc := range locales.AllLocales() {
		if !locales.IsPromptLocale(loc.Canonical) {
			continue
		}
		for cat, fsys := range categories {
			path := fmt.Sprintf("prompts/%s/%s.md", cat, loc.PromptFamily)
			if _, err := fsys.ReadFile(path); err != nil {
				t.Fatalf("prompt locale %q (family %q) is missing %s: %v", loc.Canonical, loc.PromptFamily, path, err)
			}
		}
	}

	// Conversely: no category ships a prompt family that no prompt locale points
	// at — an orphan file would never be selected and signals registry drift.
	for cat, fsys := range categories {
		entries, err := fsys.ReadDir("prompts/" + cat)
		if err != nil {
			t.Fatalf("failed to read embedded prompts/%s: %v", cat, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			family := strings.TrimSuffix(entry.Name(), ".md")
			canonical := canonicalFromPromptFamily(family)
			if !locales.IsPromptLocale(canonical) {
				t.Fatalf("orphan prompt file prompts/%s/%s.md: %q is not a prompt-backed locale", cat, family, canonical)
			}
		}
	}
}
