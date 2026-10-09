package services

import (
	"fmt"
	"strings"
	"time"
)

// Telegram connection lifecycle copy (welcome on connect, disconnect on
// cleanup, and the operator "test message" wrapper). These are operator-facing
// and sent with Markdown parse mode, separate from the HTML notification
// templates, but they resolve language through the same locale registry path
// (resolveTelegramLocale) so the whole Telegram tier stays in one place.

// telegramLifecycleCopy holds the localized strings for the connection
// lifecycle messages. welcome and disconnect carry a single %s for the
// business name.
type telegramLifecycleCopy struct {
	welcome         string
	disconnect      string
	testTitle       string
	testSentAtLabel string
	businessName    string // fallback when the business name is empty
}

// RenderTelegramWelcomeMessage renders the connection welcome message in the
// business's language. Unknown/empty languages fall back to English.
func RenderTelegramWelcomeMessage(businessName string, language string) string {
	c := telegramLifecycleCopyFor(language)
	return fmt.Sprintf(c.welcome, telegramLifecycleBusinessName(businessName, c))
}

// RenderTelegramDisconnectMessage renders the disconnection message in the
// business's language. Unknown/empty languages fall back to English.
func RenderTelegramDisconnectMessage(businessName string, language string) string {
	c := telegramLifecycleCopyFor(language)
	return fmt.Sprintf(c.disconnect, telegramLifecycleBusinessName(businessName, c))
}

// RenderTelegramTestMessage renders the operator "test notification" wrapper in
// the business's language. The body is the operator-supplied message; sentAt is
// rendered in the locale-neutral numeric "15:04 MST" form (only the surrounding
// words are translated), matching the notification templates' timestamp policy.
// Both operator-controlled values — the business name AND the body — are escaped
// for Markdown parse mode (the send path sets ParseMode="Markdown"), so a body
// with metacharacters can't break entity parsing or inject markup.
func RenderTelegramTestMessage(businessName string, body string, language string, sentAt time.Time) string {
	c := telegramLifecycleCopyFor(language)
	name := telegramLifecycleBusinessName(businessName, c)
	message := fmt.Sprintf("🔔 *%s - %s*\n\n%s", name, c.testTitle, escapeTelegramMarkdown(body))
	message += fmt.Sprintf("\n\n_%s %s_", c.testSentAtLabel, sentAt.Format("15:04 MST"))
	return message
}

// telegramLifecycleBusinessName resolves the business name (falling back to the
// localized default when empty) and escapes it for Telegram's legacy Markdown
// parse mode. The lifecycle messages embed this value inside *bold*, so an
// unescaped Markdown metacharacter in the operator-controlled business name
// would otherwise break formatting or inject markup.
func telegramLifecycleBusinessName(businessName string, c telegramLifecycleCopy) string {
	if strings.TrimSpace(businessName) == "" {
		return escapeTelegramMarkdown(c.businessName)
	}
	return escapeTelegramMarkdown(businessName)
}

// escapeTelegramMarkdown escapes the metacharacters that delimit entities in
// Telegram's legacy "Markdown" parse mode (* _ ` [), which is the parse mode the
// lifecycle messages are sent with. Backslash is escaped first so we never
// double-escape an escape we just inserted. This is intentionally the legacy
// Markdown set, not the broader MarkdownV2 set, to match the send path's
// ParseMode = "Markdown".
func escapeTelegramMarkdown(text string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"*", "\\*",
		"_", "\\_",
		"`", "\\`",
		"[", "\\[",
	)
	return replacer.Replace(text)
}

func telegramLifecycleCopyFor(language string) telegramLifecycleCopy {
	if c, ok := telegramLifecycleCopyByLocale[resolveTelegramLocale(language)]; ok {
		return c
	}
	return telegramLifecycleCopyByLocale[telegramLocaleEN]
}

var telegramLifecycleCopyByLocale = map[telegramLocale]telegramLifecycleCopy{
	telegramLocaleEN: {
		welcome: "🎉 *Welcome to Payverge Notifications!*\n\n" +
			"Your business *%s* is now connected to receive real-time notifications.\n\n" +
			"📱 *What you'll receive:*\n" +
			"• Order notifications\n" +
			"• Payment confirmations\n" +
			"• Daily business summaries\n" +
			"• Important alerts\n\n" +
			"⚙️ You can customize your notification preferences in the Payverge dashboard.\n\n" +
			"_Ready to stay connected with your business!_",
		disconnect: "👋 *Telegram Disconnected*\n\n" +
			"Your business *%s* has been disconnected from Telegram notifications.\n\n" +
			"You can reconnect anytime through the Payverge dashboard.\n\n" +
			"Thank you for using Payverge!",
		testTitle:       "Notification",
		testSentAtLabel: "Sent at",
		businessName:    "Your Business",
	},
	telegramLocaleES:   spanishTelegramLifecycleCopy(false),
	telegramLocaleESAR: spanishTelegramLifecycleCopy(true),
}

// spanishTelegramLifecycleCopy builds the Spanish lifecycle copy. Rioplatense
// (es-AR) differs only in the present-tense second person ("Puedes" → "Podés"),
// the same voseo split the rest of the es_ar tier makes; everything else is
// shared, so the two tables come from one builder.
func spanishTelegramLifecycleCopy(rioplatense bool) telegramLifecycleCopy {
	canCustomize := "Puedes personalizar"
	canReconnect := "Puedes volver a conectarlo"
	if rioplatense {
		canCustomize = "Podés personalizar"
		canReconnect = "Podés volver a conectarlo"
	}
	return telegramLifecycleCopy{
		welcome: "🎉 *¡Bienvenido a las notificaciones de Payverge!*\n\n" +
			"Tu negocio *%s* ya está conectado para recibir notificaciones en tiempo real.\n\n" +
			"📱 *Qué recibirás:*\n" +
			"• Notificaciones de pedidos\n" +
			"• Confirmaciones de pago\n" +
			"• Resúmenes diarios del negocio\n" +
			"• Alertas importantes\n\n" +
			"⚙️ " + canCustomize + " tus preferencias de notificación en el panel de Payverge.\n\n" +
			"_¡Listo para mantenerte conectado con tu negocio!_",
		disconnect: "👋 *Telegram desconectado*\n\n" +
			"Tu negocio *%s* se ha desconectado de las notificaciones de Telegram.\n\n" +
			canReconnect + " en cualquier momento desde el panel de Payverge.\n\n" +
			"¡Gracias por usar Payverge!",
		testTitle:       "Notificación",
		testSentAtLabel: "Enviado a las",
		businessName:    "Tu negocio",
	}
}
