package services

import "strings"

// WhatsAppTerminalReason is a stable reason code for deterministic guest replies.
type WhatsAppTerminalReason string

const (
	WhatsAppTooLong            WhatsAppTerminalReason = "message_too_long"
	WhatsAppRateLimited        WhatsAppTerminalReason = "rate_limited"
	WhatsAppAIUnavailable      WhatsAppTerminalReason = "ai_unavailable"
	WhatsAppBudgetReached      WhatsAppTerminalReason = "over_budget"
	WhatsAppHumanTakeover      WhatsAppTerminalReason = "human_takeover"
	WhatsAppContextUnavailable WhatsAppTerminalReason = "context_unavailable"
	WhatsAppProviderFailed     WhatsAppTerminalReason = "provider_failed"
)

// whatsappTerminalCatalog is the English baseline. Other locales fall back to
// English when missing; registered guest locales should be extended over time.
var whatsappTerminalCatalog = map[WhatsAppTerminalReason]map[string]string{
	WhatsAppTooLong: {
		"en": "That message is a bit long for me — could you send a shorter one?",
		"es": "Ese mensaje es un poco largo — ¿puedes enviarme uno más corto?",
	},
	WhatsAppRateLimited: {
		"en": "You're sending messages quickly — give me a moment and try again.",
		"es": "Estás enviando mensajes muy rápido — dame un momento e inténtalo de nuevo.",
	},
	WhatsAppAIUnavailable: {
		"en": "Automated chat isn't available right now. A team member can help you shortly.",
		"es": "El chat automático no está disponible ahora. Un miembro del equipo puede ayudarte en breve.",
	},
	WhatsAppBudgetReached: {
		"en": "I'm at capacity for today. Please try again later or ask a staff member.",
		"es": "He llegado a mi capacidad de hoy. Inténtalo más tarde o pregunta a un miembro del personal.",
	},
	WhatsAppHumanTakeover: {
		"en": "A team member is assisting you now — they'll reply here shortly.",
		"es": "Un miembro del equipo te está atendiendo ahora — te responderá aquí en breve.",
	},
	WhatsAppContextUnavailable: {
		"en": "I can't load the menu right now. Please try again in a moment.",
		"es": "No puedo cargar el menú ahora. Inténtalo de nuevo en un momento.",
	},
	WhatsAppProviderFailed: {
		"en": "Sorry, I had trouble answering just now. Please try again.",
		"es": "Perdón, tuve un problema al responder. Inténtalo de nuevo.",
	},
}

// WhatsAppTerminalReply returns a concise localized message for a terminal state.
// Unknown locales fall back to English; missing reason falls back to provider_failed.
func WhatsAppTerminalReply(locale string, reason WhatsAppTerminalReason) string {
	locale = strings.ToLower(strings.TrimSpace(locale))
	if locale == "" {
		locale = "en"
	}
	// Normalize regional tags to base language for catalog lookup.
	if i := strings.IndexByte(locale, '-'); i > 0 {
		locale = locale[:i]
	}
	byLocale, ok := whatsappTerminalCatalog[reason]
	if !ok {
		byLocale = whatsappTerminalCatalog[WhatsAppProviderFailed]
	}
	if msg, ok := byLocale[locale]; ok && msg != "" {
		return msg
	}
	if msg, ok := byLocale["en"]; ok {
		return msg
	}
	return "Sorry, please try again in a moment."
}
