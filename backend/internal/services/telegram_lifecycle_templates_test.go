package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRenderTelegramWelcomeMessage_Localizes(t *testing.T) {
	en := RenderTelegramWelcomeMessage("Bistro 21", "en")
	assert.Contains(t, en, "Welcome to Payverge Notifications")
	assert.Contains(t, en, "Bistro 21")
	assert.Contains(t, en, "Order notifications")
	assert.Contains(t, en, "You can customize")

	es := RenderTelegramWelcomeMessage("Bistró 21", "es")
	assert.Contains(t, es, "Bienvenido a las notificaciones de Payverge")
	assert.Contains(t, es, "Bistró 21")
	assert.Contains(t, es, "Notificaciones de pedidos")
	assert.Contains(t, es, "Puedes personalizar")
	assert.NotContains(t, es, "Podés")

	esar := RenderTelegramWelcomeMessage("Bistró 21", "es-AR")
	assert.Contains(t, esar, "Bienvenido a las notificaciones de Payverge")
	assert.Contains(t, esar, "Podés personalizar")
	assert.NotContains(t, esar, "Puedes personalizar")
}

func TestRenderTelegramWelcomeMessage_UnknownLanguageFallsBackToEnglish(t *testing.T) {
	for _, lang := range []string{"", "   ", "klingon", "fr"} {
		msg := RenderTelegramWelcomeMessage("X", lang)
		assert.Contains(t, msg, "Welcome to Payverge Notifications", "lang %q", lang)
	}
}

func TestRenderTelegramDisconnectMessage_Localizes(t *testing.T) {
	en := RenderTelegramDisconnectMessage("Bistro 21", "en")
	assert.Contains(t, en, "Telegram Disconnected")
	assert.Contains(t, en, "Bistro 21")

	es := RenderTelegramDisconnectMessage("Bistro 21", "es")
	assert.Contains(t, es, "Telegram desconectado")
	assert.Contains(t, es, "Puedes volver a conectarlo")
	assert.NotContains(t, es, "Podés")

	esar := RenderTelegramDisconnectMessage("Bistro 21", "es-AR")
	assert.Contains(t, esar, "Telegram desconectado")
	assert.Contains(t, esar, "Podés volver a conectarlo")
}

func TestRenderTelegramTestMessage_Localizes(t *testing.T) {
	at := time.Date(2026, 6, 11, 15, 4, 0, 0, time.UTC)

	en := RenderTelegramTestMessage("Bistro 21", "hello operator", "en", at)
	assert.Contains(t, en, "Bistro 21")
	assert.Contains(t, en, "Notification")
	assert.Contains(t, en, "hello operator")
	assert.Contains(t, en, "Sent at 15:04 UTC")

	es := RenderTelegramTestMessage("Bistro 21", "hola", "es", at)
	assert.Contains(t, es, "Notificación")
	assert.Contains(t, es, "hola")
	assert.Contains(t, es, "Enviado a las 15:04 UTC")
}

func TestRenderTelegramTestMessage_DefaultsBusinessName(t *testing.T) {
	at := time.Date(2026, 6, 11, 15, 4, 0, 0, time.UTC)
	assert.Contains(t, RenderTelegramTestMessage("", "hi", "en", at), "Your Business")
	assert.Contains(t, RenderTelegramTestMessage("   ", "hola", "es", at), "Tu negocio")
}

// TG-4: the lifecycle messages are sent with Markdown parse mode and embed the
// business name inside *bold*. A business name containing Markdown
// metacharacters (* _ [ `) must be escaped, or it breaks the formatting /
// injects markup. escapeTelegramMarkdown is the single helper; assert all three
// lifecycle renderers route the business name through it.
func TestEscapeTelegramMarkdown(t *testing.T) {
	assert.Equal(t, "A\\_B\\*C", escapeTelegramMarkdown("A_B*C"))
	// Legacy Markdown entity openers are escaped (* _ ` [); the closing "]" is
	// not a standalone delimiter, so it is left as-is.
	assert.Equal(t, "\\[x]\\`y\\`", escapeTelegramMarkdown("[x]`y`"))
	assert.Equal(t, "plain name", escapeTelegramMarkdown("plain name"))
}

// TG-4 (completeness): RenderTelegramTestMessage interpolates a SECOND
// operator-controlled value — the test-message body — into the same Markdown
// parse-mode string. It must be escaped too, or a body with Markdown
// metacharacters breaks Telegram entity parsing (400 → self-inflicted 500).
func TestRenderTelegramTestMessage_EscapesBodyMarkdown(t *testing.T) {
	at := time.Date(2026, 6, 11, 15, 4, 0, 0, time.UTC)
	const rawBody = "A_B*C [x]`y`"
	const escapedBody = "A\\_B\\*C \\[x]\\`y\\`"

	for _, lang := range []string{"en", "es", "es-AR"} {
		msg := RenderTelegramTestMessage("Bistro", rawBody, lang, at)
		assert.Containsf(t, msg, escapedBody, "test body (%s) must be markdown-escaped", lang)
		assert.NotContainsf(t, msg, rawBody, "test body (%s) must not appear raw", lang)
	}
}

func TestRenderTelegramLifecycleMessages_EscapeBusinessNameMarkdown(t *testing.T) {
	const raw = "A_B*C"
	const escaped = "A\\_B\\*C"
	at := time.Date(2026, 6, 11, 15, 4, 0, 0, time.UTC)

	for _, lang := range []string{"en", "es", "es-AR"} {
		welcome := RenderTelegramWelcomeMessage(raw, lang)
		assert.Containsf(t, welcome, escaped, "welcome (%s) must escape business name", lang)
		assert.NotContainsf(t, welcome, "*"+raw+"*", "welcome (%s) must not embed the raw name in bold", lang)

		disconnect := RenderTelegramDisconnectMessage(raw, lang)
		assert.Containsf(t, disconnect, escaped, "disconnect (%s) must escape business name", lang)
		assert.NotContainsf(t, disconnect, "*"+raw+"*", "disconnect (%s) must not embed the raw name in bold", lang)

		test := RenderTelegramTestMessage(raw, "body", lang, at)
		assert.Containsf(t, test, escaped, "test (%s) must escape business name", lang)
		assert.NotContainsf(t, test, "*"+raw+" -", "test (%s) must not embed the raw name in bold", lang)
	}
}
