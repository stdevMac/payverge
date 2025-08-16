package telegram

import (
	"fmt"
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"web3-boilerplate/internal/emails"
	"web3-boilerplate/internal/structs"
)

const (
	LangEnglish = "en"
	LangSpanish = "es"
)

// TelegramNotificationDispatcher handles Telegram notifications
type TelegramNotificationDispatcher struct {
	bot *tgbotapi.BotAPI
}

// NewTelegramNotificationDispatcher creates a new Telegram notification dispatcher
func NewTelegramNotificationDispatcher(bot *tgbotapi.BotAPI) *TelegramNotificationDispatcher {
	return &TelegramNotificationDispatcher{
		bot: bot,
	}
}

// getLanguage returns the user's preferred language
func getLanguage(user structs.User) string {
	if user.LanguageSelected == "es" {
		return LangSpanish
	}
	return LangEnglish
}

// escapeMarkdown escapes special characters for Telegram MarkdownV2
func escapeMarkdown(text string) string {
	escaped := text
	escaped = strings.ReplaceAll(escaped, "_", "\\_")
	escaped = strings.ReplaceAll(escaped, "*", "\\*")
	escaped = strings.ReplaceAll(escaped, "[", "\\[")
	escaped = strings.ReplaceAll(escaped, "]", "\\]")
	escaped = strings.ReplaceAll(escaped, "(", "\\(")
	escaped = strings.ReplaceAll(escaped, ")", "\\)")
	escaped = strings.ReplaceAll(escaped, "~", "\\~")
	escaped = strings.ReplaceAll(escaped, "`", "\\`")
	escaped = strings.ReplaceAll(escaped, ">", "\\>")
	escaped = strings.ReplaceAll(escaped, "#", "\\#")
	escaped = strings.ReplaceAll(escaped, "+", "\\+")
	escaped = strings.ReplaceAll(escaped, "-", "\\-")
	escaped = strings.ReplaceAll(escaped, "=", "\\=")
	escaped = strings.ReplaceAll(escaped, "|", "\\|")
	escaped = strings.ReplaceAll(escaped, "{", "\\{")
	escaped = strings.ReplaceAll(escaped, "}", "\\}")
	escaped = strings.ReplaceAll(escaped, ".", "\\.")
	escaped = strings.ReplaceAll(escaped, "!", "\\!")
	return escaped
}

// formatLink formats a URL link for Telegram MarkdownV2
func formatLink(text, url string) string {
	return fmt.Sprintf("[%s](%s)", escapeMarkdown(text), url)
}

// formatGenericMessage formats a generic notification message
func (t *TelegramNotificationDispatcher) formatGenericMessage(data map[string]interface{}, user structs.User, id string) string {
	title := escapeMarkdown(data["title"].(string))
	description := escapeMarkdown(data["description"].(string))
	lang := getLanguage(user)

	var message strings.Builder
	switch lang {
	case LangSpanish:
		message.WriteString(fmt.Sprintf("📢 *%s*\n\n", title))
		message.WriteString(fmt.Sprintf("%s\n", description))

	default: // English
		message.WriteString(fmt.Sprintf("📢 *%s*\n\n", title))
		message.WriteString(fmt.Sprintf("%s\n", description))
	}

	return message.String()
}

// formatWelcomeMessage formats a welcome message for new users
func (t *TelegramNotificationDispatcher) formatWelcomeMessage(data map[string]interface{}, user structs.User) string {
	lang := getLanguage(user)
	var message strings.Builder

	switch lang {
	case LangSpanish:
		message.WriteString("🎉 *¡Bienvenido a Web3 Boilerplate!*\n\n")
		message.WriteString("Gracias por unirte a nuestra plataforma\\. ")
		message.WriteString("Ahora puedes acceder a todas las funcionalidades disponibles\\.")

	default: // English
		message.WriteString("🎉 *Welcome to Web3 Boilerplate!*\n\n")
		message.WriteString("Thank you for joining our platform\\. ")
		message.WriteString("You now have access to all available features\\.")
	}

	return message.String()
}

// formatAdminMessage formats an admin notification message
func (t *TelegramNotificationDispatcher) formatAdminMessage(data map[string]interface{}, user structs.User) string {
	title := escapeMarkdown(data["title"].(string))
	description := escapeMarkdown(data["description"].(string))
	lang := getLanguage(user)

	var message strings.Builder
	switch lang {
	case LangSpanish:
		message.WriteString("🔧 *Notificación de Administrador*\n\n")
		message.WriteString(fmt.Sprintf("**%s**\n\n", title))
		message.WriteString(fmt.Sprintf("%s\n", description))

	default: // English
		message.WriteString("🔧 *Admin Notification*\n\n")
		message.WriteString(fmt.Sprintf("**%s**\n\n", title))
		message.WriteString(fmt.Sprintf("%s\n", description))
	}

	return message.String()
}

// getInlineKeyboard returns an inline keyboard for the user
func (t *TelegramNotificationDispatcher) getInlineKeyboard(user structs.User) tgbotapi.InlineKeyboardMarkup {
	lang := getLanguage(user)
	
	var dashboardText, settingsText string
	if lang == LangSpanish {
		dashboardText = "📊 Panel de Control"
		settingsText = "⚙️ Configuración"
	} else {
		dashboardText = "📊 Dashboard"
		settingsText = "⚙️ Settings"
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL(dashboardText, "https://yourapp.com/dashboard"),
			tgbotapi.NewInlineKeyboardButtonURL(settingsText, "https://yourapp.com/settings"),
		),
	)

	return keyboard
}

// DispatchNotification sends the notification via Telegram
func (t *TelegramNotificationDispatcher) DispatchNotification(notification structs.Notification, user structs.User, id string) {
	if user.TGChatID == 0 {
		fmt.Printf("User %s has no Telegram chat ID\n", user.Address)
		return
	}

	var messageText string
	data := notification.TemplateData

	// Format message based on notification type
	switch notification.TemplateID {
	case emails.TemplateGenericNotification:
		messageText = t.formatGenericMessage(data, user, id)
	case emails.TemplateWelcomeEmail:
		messageText = t.formatWelcomeMessage(data, user)
	case emails.TemplateAdminNotification:
		messageText = t.formatAdminMessage(data, user)
	default:
		messageText = fmt.Sprintf("*%s*\n\n%s", escapeMarkdown(notification.Title), escapeMarkdown(notification.Description))
	}

	msg := tgbotapi.NewMessage(user.TGChatID, messageText)
	msg.ParseMode = "MarkdownV2"
	msg.DisableWebPagePreview = false

	// Add inline keyboard with relevant buttons
	keyboard := t.getInlineKeyboard(user)
	msg.ReplyMarkup = keyboard

	if _, err := t.bot.Send(msg); err != nil {
		log.Printf("Error sending Telegram message to user %s: %v", user.Address, err)
	}
}
