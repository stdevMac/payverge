package telegram

import (
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stdevMac/shares/internal/emails"
	"github.com/stdevMac/shares/internal/structs"
)

const (
	LangEnglish = "en"
	LangSpanish = "es"
)

// getLanguage returns the user's preferred language or defaults to English
func getLanguage(user structs.User) string {
	if user.LanguageSelected == "" {
		return LangEnglish
	}
	switch strings.ToLower(user.LanguageSelected) {
	case LangSpanish:
		return LangSpanish
	default:
		return LangEnglish
	}
}

// TelegramNotificationDispatcher is a dispatcher that sends notifications via Telegram
type TelegramNotificationDispatcher struct {
	bot *tgbotapi.BotAPI
}

// NewTelegramNotificationDispatcher creates a new Telegram notification dispatcher
func NewTelegramNotificationDispatcher(bot *tgbotapi.BotAPI) *TelegramNotificationDispatcher {
	return &TelegramNotificationDispatcher{
		bot: bot,
	}
}

// translateMonth translates month names from English to Spanish
func translateMonth(month string) string {
	monthTranslations := map[string]string{
		"January":   "Enero",
		"February":  "Febrero",
		"March":     "Marzo",
		"April":     "Abril",
		"May":       "Mayo",
		"June":      "Junio",
		"July":      "Julio",
		"August":    "Agosto",
		"September": "Septiembre",
		"October":   "Octubre",
		"November":  "Noviembre",
		"December":  "Diciembre",
	}

	if translation, ok := monthTranslations[month]; ok {
		return translation
	}
	return month
}

// formatDateSpanish formats a date in Spanish style (e.g., "2 de Enero de 2006")
func formatDateSpanish(t time.Time) string {
	month := translateMonth(t.Format("January"))
	return fmt.Sprintf("%d de %s de %d", t.Day(), month, t.Year())
}

// escapeMarkdown escapes special characters for Telegram MarkdownV2 format
func escapeMarkdown(text string) string {
	// These characters must be escaped in MarkdownV2:
	// '_', '*', '[', ']', '(', ')', '~', '`', '>', '#', '+', '-', '=', '|', '{', '}', '.', '!'
	escaped := text
	// First escape the backslash itself
	escaped = strings.ReplaceAll(escaped, "\\", "\\\\")
	// Then escape the rest of the special characters
	escaped = strings.ReplaceAll(escaped, "_", "\\_")
	escaped = strings.ReplaceAll(escaped, "*", "\\*")
	escaped = strings.ReplaceAll(escaped, "[", "\\[")
	escaped = strings.ReplaceAll(escaped, "]", "\\]")
	escaped = strings.ReplaceAll(escaped, "(", "\\(")
	escaped = strings.ReplaceAll(escaped, ")", "\\)")
	escaped = strings.ReplaceAll(escaped, "~", "\\~")
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

// formatFleetReadyToInvestMessage formats a message for new investment opportunities
func (t *TelegramNotificationDispatcher) formatFleetReadyToInvestMessage(data map[string]interface{}, user structs.User, carID string) string {
	fleetName := escapeMarkdown(data["CarBrand"].(string))
	fleetLink := data["CarLink"].(string)
	lang := getLanguage(user)

	var message strings.Builder
	switch lang {
	case LangSpanish:
		message.WriteString("🚀 *Nueva Oportunidad de Inversión*\n\n")
		message.WriteString(fmt.Sprintf("Flota: *%s*\n\n", fleetName))

		if amountToFill, ok := data["amount_to_fill"].(float64); ok {
			message.WriteString("💰 *Detalles de la Inversión*\n")
			message.WriteString(fmt.Sprintf("Monto Objetivo: $%s\n", escapeMarkdown(fmt.Sprintf("%.2f", amountToFill))))
			if apr, ok := data["roi"].(float64); ok {
				message.WriteString(fmt.Sprintf("APR Esperado: %s%%\n", escapeMarkdown(fmt.Sprintf("%.1f", apr))))
			}
		}

		message.WriteString("\n🎯 \\¡Sé de los primeros en invertir en esta emocionante oportunidad\\!")
		message.WriteString(fmt.Sprintf("\n\n🔗 %s", formatLink("Ver Detalles de la Flota", fleetLink)))

	default: // English
		message.WriteString("🚀 *New Investment Opportunity*\n\n")
		message.WriteString(fmt.Sprintf("Fleet: *%s*\n\n", fleetName))

		if amountToFill, ok := data["amount_to_fill"].(float64); ok {
			message.WriteString("💰 *Investment Details*\n")
			message.WriteString(fmt.Sprintf("Target Amount: $%s\n", escapeMarkdown(fmt.Sprintf("%.2f", amountToFill))))
			if apr, ok := data["roi"].(float64); ok {
				message.WriteString(fmt.Sprintf("Expected APR: %s%%\n", escapeMarkdown(fmt.Sprintf("%.1f", apr))))
			}
		}

		message.WriteString("\n🎯 " + escapeMarkdown("Be among the first to invest in this exciting opportunity!"))
		message.WriteString(fmt.Sprintf("\n\n🔗 %s", formatLink("View Fleet Details", fleetLink)))
	}

	return message.String()
}

// formatFleetAlmostFundedMessage formats a message for fleets that are close to being fully funded
func (t *TelegramNotificationDispatcher) formatFleetAlmostFundedMessage(data map[string]interface{}, user structs.User, carID string) string {
	fleetName := escapeMarkdown(data["CarBrand"].(string))
	fleetLink := data["CarLink"].(string)
	lang := getLanguage(user)

	var message strings.Builder
	switch lang {
	case LangSpanish:
		message.WriteString(fmt.Sprintf("⚡ *¡Flota Casi Financiada!*\n\n"))
		message.WriteString(fmt.Sprintf("Flota: *%s*\n\n", fleetName))

		if fundedPercent, ok := data["funded_percent"].(float64); ok {
			message.WriteString(fmt.Sprintf("Ya financiado: %s%%\n", escapeMarkdown(fmt.Sprintf("%.1f", fundedPercent))))
		}
		if amountToFill, ok := data["amount_to_fill"].(float64); ok {
			message.WriteString(fmt.Sprintf("Monto restante: $%s\n", escapeMarkdown(fmt.Sprintf("%.2f", amountToFill))))
		}
		if apr, ok := data["roi"].(float64); ok {
			message.WriteString(fmt.Sprintf("APR Esperado: %s%%\n", escapeMarkdown(fmt.Sprintf("%.1f", apr))))
		}

		message.WriteString("\n🏃 *¡No te lo pierdas\\!* Cupos limitados\\.")
		message.WriteString(fmt.Sprintf("\n\n🔗 %s", formatLink("Ver Detalles de la Flota", fleetLink)))

	default: // English
		message.WriteString(fmt.Sprintf("⚡ *Fleet Almost Funded*\n\n"))
		message.WriteString(fmt.Sprintf("Fleet: *%s*\n\n", fleetName))

		if fundedPercent, ok := data["funded_percent"].(float64); ok {
			message.WriteString(fmt.Sprintf("Already funded: %s%%\n", escapeMarkdown(fmt.Sprintf("%.1f", fundedPercent))))
		}
		if amountToFill, ok := data["amount_to_fill"].(float64); ok {
			message.WriteString(fmt.Sprintf("Remaining amount: $%s\n", escapeMarkdown(fmt.Sprintf("%.2f", amountToFill))))
		}
		if apr, ok := data["roi"].(float64); ok {
			message.WriteString(fmt.Sprintf("Expected APR: %s%%\n", escapeMarkdown(fmt.Sprintf("%.1f", apr))))
		}

		message.WriteString("\n🏃 *Don't miss out\\!* Limited spots remaining\\.")
		message.WriteString(fmt.Sprintf("\n\n🔗 %s", formatLink("View Fleet Details", fleetLink)))
	}

	return message.String()
}

// formatFleetFundedMessage formats a message for fully funded fleets
func (t *TelegramNotificationDispatcher) formatFleetFundedMessage(data map[string]interface{}, user structs.User, carID string) string {
	fleetName := escapeMarkdown(data["CarBrand"].(string))
	fleetLink := data["CarLink"].(string)
	lang := getLanguage(user)

	var message strings.Builder
	switch lang {
	case LangSpanish:
		message.WriteString(fmt.Sprintf("🎉 *\\¡Felicitaciones\\!*\n"))
		message.WriteString(fmt.Sprintf("\\¡La flota *%s* está completamente financiada\\!\n\n", fleetName))

		if totalValue, ok := data["total_value"].(float64); ok {
			message.WriteString("💰 *Resumen de la Inversión*\n")
			message.WriteString(fmt.Sprintf("Valor Total de la Flota: $%s\n", escapeMarkdown(fmt.Sprintf("%.2f", totalValue))))
			if apr, ok := data["roi"].(float64); ok {
				message.WriteString(fmt.Sprintf("APR Esperado: %s%%\n", escapeMarkdown(fmt.Sprintf("%.1f", apr))))

				// Add personalized investment info if user has invested
				if user.Address != "" {
					userAmount := data["user_amount"].(float64)
					if userAmount > 0 {
						ownershipPercentage := (userAmount * 100) / totalValue
						expectedUserProfit := (totalValue * (apr / 100)) * (ownershipPercentage / 100)

						message.WriteString(fmt.Sprintf("\n💎 *Tu Inversión*\n"))
						message.WriteString(fmt.Sprintf("Tu Participación: %s%%\n", escapeMarkdown(fmt.Sprintf("%.2f", ownershipPercentage))))
						message.WriteString(fmt.Sprintf("Monto Invertido: $%s\n", escapeMarkdown(fmt.Sprintf("%.2f", userAmount))))
						message.WriteString(fmt.Sprintf("Ganancias Esperadas: $%s\n", escapeMarkdown(fmt.Sprintf("%.2f", expectedUserProfit))))
					}
				}
			}
		}

		message.WriteString("\n🚀 *Próximos Pasos*\n")
		message.WriteString("1\\. Comienza la adquisición de vehículos\n")
		message.WriteString("2\\. Actualizaciones regulares sobre las compras\n")
		message.WriteString("3\\. Comienzan los ingresos por alquiler\n")
		message.WriteString("4\\. Seguimiento del rendimiento en el panel\n")

		// Add timestamp for funding completion
		message.WriteString(fmt.Sprintf("\n📅 Financiado el: %s", escapeMarkdown(formatDateSpanish(time.Now()))))

		message.WriteString(fmt.Sprintf("\n\n🔗 %s", formatLink("Ver Detalles de la Flota", fleetLink)))
		message.WriteString("\n\n💡 _\\¡Sigue el progreso de tu inversión en tu panel\\!_")

		if support, ok := data["SupportEmail"].(string); ok {
			message.WriteString(fmt.Sprintf("\n\n📧 \\¿Preguntas\\? Contáctanos: %s", escapeMarkdown(support)))
		}

	default: // English
		message.WriteString(fmt.Sprintf("🎉 *Congratulations\\!*\n"))
		message.WriteString(fmt.Sprintf("The *%s* fleet is now fully funded\\!\n\n", fleetName))

		if totalValue, ok := data["total_value"].(float64); ok {
			message.WriteString("💰 *Investment Overview*\n")
			message.WriteString(fmt.Sprintf("Total Fleet Value: $%s\n", escapeMarkdown(fmt.Sprintf("%.2f", totalValue))))
			if apr, ok := data["roi"].(float64); ok {
				message.WriteString(fmt.Sprintf("Expected APR: %s%%\n", escapeMarkdown(fmt.Sprintf("%.1f", apr))))

				// Add personalized investment info if user has invested
				if user.Address != "" {
					userAmount := data["user_amount"].(float64)
					if userAmount > 0 {
						ownershipPercentage := (userAmount * 100) / totalValue
						expectedUserProfit := (totalValue * (apr / 100)) * (ownershipPercentage / 100)

						message.WriteString(fmt.Sprintf("\n💎 *Your Investment*\n"))
						message.WriteString(fmt.Sprintf("Your Share: %s%%\n", escapeMarkdown(fmt.Sprintf("%.2f", ownershipPercentage))))
						message.WriteString(fmt.Sprintf("Amount Invested: $%s\n", escapeMarkdown(fmt.Sprintf("%.2f", userAmount))))
						message.WriteString(fmt.Sprintf("Expected Profits: $%s\n", escapeMarkdown(fmt.Sprintf("%.2f", expectedUserProfit))))
					}
				}
			}
		}

		message.WriteString("\n🚀 *Next Steps*\n")
		message.WriteString("1\\. Vehicle acquisition begins\n")
		message.WriteString("2\\. Regular updates on purchases\n")
		message.WriteString("3\\. Rental income starts flowing\n")
		message.WriteString("4\\. Track performance in dashboard\n")

		// Add timestamp for funding completion
		message.WriteString(fmt.Sprintf("\n📅 Funded on: %s", escapeMarkdown(time.Now().Format("January 2, 2006"))))

		message.WriteString(fmt.Sprintf("\n\n🔗 %s", formatLink("View Fleet Details", fleetLink)))
		message.WriteString("\n\n💡 _Track your investment progress in your dashboard\\!_")

		if support, ok := data["SupportEmail"].(string); ok {
			message.WriteString(fmt.Sprintf("\n\n📧 Questions\\? Contact us: %s", escapeMarkdown(support)))
		}
	}

	return message.String()
}

// formatFleetNotRealizedMessage formats a message for fleets that were not realized
func (t *TelegramNotificationDispatcher) formatFleetNotRealizedMessage(data map[string]interface{}, user structs.User, id string) string {
	var message strings.Builder
	redeemLink := data["RedeemLink"].(string)
	fleetName := data["FleetName"].(string)
	lang := getLanguage(user)

	switch lang {
	case LangSpanish:
		message.WriteString("\n\n🚨 *Aviso Importante: Cancelación de Inversión en Flota*")
		message.WriteString("\n\nEstimado Inversor,")
		message.WriteString(fmt.Sprintf("\n\nLamentamos informarte que la compra de la flota '*%s*' no pudo "+
			"ser completada\\.", escapeMarkdown(fleetName)))
		message.WriteString("\nComo resultado, estamos iniciando el reembolso de tu inversión\\.")
		message.WriteString("\n\nPara reclamar tu reembolso, por favor haz clic en el enlace a continuación:")
		message.WriteString(fmt.Sprintf("\n🔗 [*Reclamar tu Reembolso*](%s)", escapeMarkdown(redeemLink)))
		message.WriteString("\n\nSi tienes alguna pregunta, no dudes en contactar a nuestro equipo de soporte\\.")
		message.WriteString("\n\nGracias por tu comprensión y confianza continua en nuestra plataforma\\.")

	default: // English
		message.WriteString("\n\n🚨 *Important Notice: Fleet Investment Cancellation*")
		message.WriteString("\n\nDear Investor,")
		message.WriteString(fmt.Sprintf("\n\nWe regret to inform you that the purchase of the fleet '*%s*' could not "+
			"be completed\\.", escapeMarkdown(fleetName)))
		message.WriteString("\nAs a result, we are initiating a refund of your investment\\.")
		message.WriteString("\n\nTo claim your refund, please click on the link below:")
		message.WriteString(fmt.Sprintf("\n🔗 [*Claim Your Refund*](%s)", escapeMarkdown(redeemLink)))
		message.WriteString("\n\nIf you have any questions, please don't hesitate to contact our support team\\.")
		message.WriteString("\n\nThank you for your understanding and continued trust in our platform\\.")
	}

	return message.String()
}

// formatCarStatusMessage formats a message for car status notifications
func (t *TelegramNotificationDispatcher) formatCarStatusMessage(data map[string]interface{}, user structs.User, carID string) string {
	var message strings.Builder
	// lang := getLanguage(user)

	return message.String()
}

// formatDailyReportMessage formats the daily report message
func (t *TelegramNotificationDispatcher) formatDailyReportMessage(data map[string]any, user structs.User) string {
	var message strings.Builder
	// lang := getLanguage(user)

	return message.String()
}

// getInlineKeyboard creates inline keyboard with relevant buttons
func (t *TelegramNotificationDispatcher) getInlineKeyboard(user structs.User) *tgbotapi.InlineKeyboardMarkup {
	var buttons [][]tgbotapi.InlineKeyboardButton
	lang := getLanguage(user)

	// Always add dashboard button
	dashboardText := "Check Profile"
	if lang == LangSpanish {
		dashboardText = "Ver Perfil"
	}
	dashboardButton := tgbotapi.NewInlineKeyboardButtonURL(
		dashboardText,
		"https://app.tokenfleet.io/profile",
	)
	buttons = append(buttons, []tgbotapi.InlineKeyboardButton{dashboardButton})

	// Add referral button if user has a referral code
	if user.ReferralCode != "" {
		referralText := "Share & Earn More"
		if lang == LangSpanish {
			referralText = "Comparte y Gana Más"
		}
		referralButton := tgbotapi.NewInlineKeyboardButtonURL(
			referralText,
			fmt.Sprintf("https://app.tokenfleet.io/referee/%s", user.ReferralCode),
		)
		buttons = append(buttons, []tgbotapi.InlineKeyboardButton{referralButton})
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(buttons...)
	return &keyboard
}

// DispatchNotification sends the notification via Telegram
func (t *TelegramNotificationDispatcher) DispatchNotification(notification structs.Notification, user structs.User, carID string) {
	if user.TGChatID == 0 {
		fmt.Printf("User %s has no Telegram chat ID\n", user.Address)
		return
	}

	var messageText string
	data := notification.TemplateData

	// Format message based on notification type
	switch notification.TemplateID {
	case emails.TemplateFleetReadyToInvest:
		messageText = t.formatFleetReadyToInvestMessage(data, user, carID)
	case emails.TemplateFleetFunded:
		messageText = t.formatFleetFundedMessage(data, user, carID)
	case emails.TemplateFleetAlmostFunded:
		messageText = t.formatFleetAlmostFundedMessage(data, user, carID)
	case emails.TemplateFleetNotRealized:
		messageText = t.formatFleetNotRealizedMessage(data, user, carID)
	case emails.TemplateCarBought:
		data["status"] = "bought"
		messageText = t.formatCarStatusMessage(data, user, carID)
	case emails.TemplateCarSold:
		data["status"] = "sold"
		messageText = t.formatCarStatusMessage(data, user, carID)
	case emails.TemplateCarRented:
		data["status"] = "rented"
		messageText = t.formatCarStatusMessage(data, user, carID)
	case emails.TemplateCarMaintenance:
		data["status"] = "maintenance"
		messageText = t.formatCarStatusMessage(data, user, carID)
	case emails.TemplateCarAvailable:
		data["status"] = "available"
		messageText = t.formatCarStatusMessage(data, user, carID)
	case emails.TemplateDailyReport:
		messageText = t.formatDailyReportMessage(data, user)
	default:
		if lang := getLanguage(user); lang == LangSpanish {
			// For now, use the same title and description for both languages
			// TODO: Add support for Spanish titles and descriptions in the Notification struct
			messageText = fmt.Sprintf("*%s*\n\n%s", escapeMarkdown(notification.Title), escapeMarkdown(notification.Description))
		} else {
			messageText = fmt.Sprintf("*%s*\n\n%s", escapeMarkdown(notification.Title), escapeMarkdown(notification.Description))
		}
	}

	msg := tgbotapi.NewMessage(user.TGChatID, messageText)
	msg.ParseMode = "MarkdownV2"      // Using V2 for better formatting support
	msg.DisableWebPagePreview = false // Enable link previews

	// Add inline keyboard with relevant buttons
	keyboard := t.getInlineKeyboard(user)
	msg.ReplyMarkup = keyboard

	_, err := t.bot.Send(msg)
	if err != nil {
		fmt.Printf("Error sending Telegram message to user %s: %v\n", user.Address, err)
	}
}
