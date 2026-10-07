package services

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed prompts/ai_waiter/greetings/*.md
var waiterGreetingFS embed.FS

//go:embed prompts/ai_waiter/greetings/closed/*.md
var waiterClosedGreetingFS embed.FS

//go:embed prompts/ai_waiter/greetings/concierge/*.md
var waiterConciergeGreetingFS embed.FS

//go:embed prompts/ai_waiter/greetings/scripted/*.md
var waiterScriptedGreetingFS embed.FS

// WaiterGreeting returns the localized opening message including the EU AI Act
// Art. 50 disclosure. Greeting assets are the single source of truth (the
// 7-locale GetGreeting Go map is deleted; contract C5).
//
// When browseOnly is true (venue closed or guest ordering disabled), the
// closed-template family is used so the assistant does not offer "help
// ordering" after hours. Concierge mode already never takes orders, so the
// open template is fine either way.
func WaiterGreeting(code, aiName, businessName, mode string, browseOnly bool) string {
	if aiName == "" {
		aiName = "Sage"
	}
	loc := resolveWaiterLocale(code)
	if resolveWaiterMode(mode) == "concierge" {
		raw, err := loadConciergeGreeting(loc.Canonical)
		if err != nil {
			raw, err = loadConciergeGreeting("en")
		}
		if err == nil {
			return strings.NewReplacer("{{AI_NAME}}", aiName, "{{BUSINESS_NAME}}", businessName).Replace(strings.TrimSpace(string(raw)))
		}
	}
	useClosed := browseOnly && resolveWaiterMode(mode) == "ordering"
	raw, err := loadWaiterGreeting(loc.Canonical, useClosed)
	if err != nil {
		if useClosed {
			// Prefer English closed over open-locale if a locale is missing.
			raw, err = loadWaiterGreeting("en", true)
		}
		if err != nil {
			raw, err = loadWaiterGreeting("en", false)
		}
		if err != nil {
			if useClosed {
				return fmt.Sprintf(
					"Welcome to %s! I'm %s, your AI assistant. You're chatting with an AI. Ordering is paused right now — ask me anything about the menu.",
					businessName, aiName,
				)
			}
			return fmt.Sprintf("Welcome to %s! I'm %s, your AI assistant. You're chatting with an AI.", businessName, aiName)
		}
	}
	return strings.NewReplacer("{{AI_NAME}}", aiName, "{{BUSINESS_NAME}}", businessName).Replace(strings.TrimSpace(string(raw)))
}

func loadWaiterGreeting(canonical string, closed bool) ([]byte, error) {
	if closed {
		return waiterClosedGreetingFS.ReadFile(fmt.Sprintf("prompts/ai_waiter/greetings/closed/%s.md", canonical))
	}
	return waiterGreetingFS.ReadFile(fmt.Sprintf("prompts/ai_waiter/greetings/%s.md", canonical))
}

func loadConciergeGreeting(canonical string) ([]byte, error) {
	return waiterConciergeGreetingFS.ReadFile(fmt.Sprintf("prompts/ai_waiter/greetings/concierge/%s.md", canonical))
}

// WaiterScriptedGreeting is the opening message when no LLM provider is
// configured and the waiter answers from the menu snapshot with set replies
// (ai_waiter_mode=basic). It must not present the helper as an AI, and it
// offers no ordering help, so one family covers open, closed and concierge.
func WaiterScriptedGreeting(code, aiName, businessName string) string {
	if aiName == "" {
		aiName = "Sage"
	}
	loc := resolveWaiterLocale(code)
	raw, err := waiterScriptedGreetingFS.ReadFile(fmt.Sprintf("prompts/ai_waiter/greetings/scripted/%s.md", loc.Canonical))
	if err != nil {
		raw, err = waiterScriptedGreetingFS.ReadFile("prompts/ai_waiter/greetings/scripted/en.md")
	}
	if err != nil {
		return fmt.Sprintf("Welcome to %s! I'm %s, the menu helper here. I'm not an AI: I answer with set replies built from the menu.", businessName, aiName)
	}
	return strings.NewReplacer("{{AI_NAME}}", aiName, "{{BUSINESS_NAME}}", businessName).Replace(strings.TrimSpace(string(raw)))
}
