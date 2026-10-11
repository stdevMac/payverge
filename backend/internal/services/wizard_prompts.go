package services

import (
	"embed"
	"fmt"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/locales"
)

//go:embed prompts/menu_wizard/*.md
var wizardPromptFS embed.FS

// GetWizardPrompt returns the system prompt for the specified language.
func GetWizardPrompt(language string) string {
	locale := resolvePromptLocale(language)
	contentLocale := locale
	content, err := wizardPromptFS.ReadFile(fmt.Sprintf("prompts/menu_wizard/%s.md", locale.PromptFamily))
	if err != nil {
		defaultContent, defaultErr := wizardPromptFS.ReadFile("prompts/menu_wizard/en.md")
		if defaultErr != nil {
			return "You are a Payverge AI menu wizard. Respond with valid JSON."
		}
		content = defaultContent
		contentLocale = locales.Default()
	}

	instructions := strings.TrimSpace(string(content))
	return fmt.Sprintf("%s\n\nCRITICAL: You MUST respond in %s.\n\n%s",
		instructions, contentLocale.NativeName, wizardTurnInstruction(contentLocale.Canonical))
}

// wizardTurnInstruction returns the per-turn response-format reminder in the
// user's prompt family. It replaces the old hardcoded English "[Instructions]"
// suffix (menu_ai_service.go) so localized conversations stay localized.
func wizardTurnInstruction(language string) string {
	switch resolvePromptLocale(language).PromptFamily {
	case "es":
		return "Respondé como el asistente en formato JSON con: message, is_complete, extracted_config, suggested_options."
	case "es_ar":
		return "Respondé como el asistente en formato JSON con: message, is_complete, extracted_config, suggested_options. (Español de Argentina)"
	default:
		return "Respond as the assistant in JSON format containing: message, is_complete, extracted_config, suggested_options."
	}
}

func canonicalFromPromptFamily(family string) string {
	parts := strings.Split(family, "_")
	if len(parts) == 2 {
		return fmt.Sprintf("%s-%s", parts[0], strings.ToUpper(parts[1]))
	}

	return family
}

func resolvePromptLocale(language string) locales.Locale {
	if locale, ok := locales.Lookup(language); ok && locales.IsPromptLocale(locale.Canonical) {
		return locale
	}

	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(language), "_", "-"))
	for _, locale := range locales.AllLocales() {
		if locales.IsPromptLocale(locale.Canonical) && (normalized == strings.ToLower(locale.Canonical) || normalized == locale.PathSegment) {
			return locale
		}
	}

	return locales.Default()
}
