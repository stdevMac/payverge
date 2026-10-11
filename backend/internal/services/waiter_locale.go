package services

import (
	"fmt"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/llmeval/langid"
	"github.com/stdevmac/payverge/backend/internal/locales"
)

// ResolveWaiterConversationLocale picks the language for a WhatsApp (or similar)
// turn. High-confidence detection of guest text updates the locale; short or
// low-confidence text retains the stored conversation locale; empty history
// falls back to English.
func ResolveWaiterConversationLocale(text, stored string) string {
	text = strings.TrimSpace(text)
	stored = canonicalGuestLocale(stored)

	if text != "" {
		detected, confidence := langid.Detect(text)
		canon := canonicalGuestLocale(detected)
		if confidence >= 0.34 && isGuestLocale(canon) {
			return canon
		}
	}
	if isGuestLocale(stored) {
		return stored
	}
	return "en"
}

func canonicalGuestLocale(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" || code == "auto" {
		return ""
	}
	// Map regional tags to base when catalog is language-level.
	if i := strings.IndexByte(code, '-'); i > 0 {
		base := code[:i]
		// Keep es-ar as a distinct guest locale when present.
		if code == "es-ar" || code == "es_ar" {
			return "es-AR"
		}
		return base
	}
	return code
}

// isGuestLocale reports whether code is a supported guest UI language.
// Mirrors the frontend guest-messages set without importing the i18n package.
func isGuestLocale(code string) bool {
	switch strings.ToLower(code) {
	case "en", "es", "es-ar", "pt", "fr", "de", "it", "nl", "pl", "ru",
		"ar", "he", "tr", "ja", "ko", "zh", "th", "vi", "hi", "sv", "no", "da":
		return true
	default:
		return false
	}
}

func displayGuestLocale(code string) string {
	if loc, ok := locales.Lookup(strings.TrimSpace(code)); ok {
		if name := strings.TrimSpace(loc.NativeName); name != "" {
			return name
		}
		return loc.Canonical
	}
	if strings.TrimSpace(code) == "" {
		return "English"
	}
	return strings.TrimSpace(code)
}

// WaiterLanguageTransition is the Live Monitor system line when a retained
// conversation changes guest locale without starting a new session.
func WaiterLanguageTransition(from, to string) string {
	return fmt.Sprintf("Language switched from %s to %s.", displayGuestLocale(from), displayGuestLocale(to))
}

// WaiterNewConversationTransition is persisted on the closed predecessor so
// owners can see why an English thread stopped receiving turns.
func WaiterNewConversationTransition(to string) string {
	return fmt.Sprintf("Guest started a new conversation in %s.", displayGuestLocale(to))
}
