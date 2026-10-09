package agents

import (
	"fmt"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/locales"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// ResolveOpsAssistantPrompt loads the Ops Assistant system prompt for locale.
func ResolveOpsAssistantPrompt(locale string) (string, error) {
	return resolvePersonaPrompt("ops_assistant", locale)
}

// LoadOpsPlaybook returns curated tab help markdown.
func LoadOpsPlaybook(tabKey string) (string, error) {
	key := strings.TrimSpace(strings.ToLower(tabKey))
	if key == "" {
		return "", fmt.Errorf("empty tab key")
	}
	path := fmt.Sprintf("prompts/ops_assistant/playbooks/%s.md", key)
	raw, err := services.PersonaPromptFS.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

func resolvePersonaPrompt(persona, locale string) (string, error) {
	family := promptFamily(locale)
	candidates := []string{
		fmt.Sprintf("prompts/%s/%s.md", persona, family),
		fmt.Sprintf("prompts/%s/en.md", persona),
	}
	for _, path := range candidates {
		raw, err := services.PersonaPromptFS.ReadFile(path)
		if err == nil {
			body, pending := parseReviewPending(string(raw))
			if !pending && strings.TrimSpace(body) != "" {
				return body, nil
			}
		}
	}
	return "", fmt.Errorf("agents: no prompt for persona %q locale %q", persona, locale)
}

func promptFamily(locale string) string {
	if loc, ok := locales.Lookup(locale); ok {
		switch loc.PromptFamily {
		case "es_ar":
			return "es_ar"
		case "es":
			return "es"
		}
	}
	n := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(locale), "_", "-"))
	if n == "es-ar" || n == "es_ar" {
		return "es_ar"
	}
	if strings.HasPrefix(n, "es") {
		return "es"
	}
	return "en"
}

func parseReviewPending(raw string) (string, bool) {
	s := strings.TrimLeft(raw, "\uFEFF \t\r\n")
	if !strings.HasPrefix(s, "---") {
		return strings.TrimSpace(raw), false
	}
	rest := strings.TrimPrefix(s, "---")
	end := strings.Index(rest, "---")
	if end < 0 {
		return strings.TrimSpace(raw), false
	}
	header := rest[:end]
	body := strings.TrimSpace(rest[end+3:])
	pending := strings.Contains(strings.ToLower(header), "review_pending: true")
	return body, pending
}
