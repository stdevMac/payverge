package main

import (
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/server"
)

// operatorRoutingGaps lists the operator-routing settings that are unset.
// Each one used to fall back to the upstream project's own inbox or bot;
// those defaults are gone, so a fork must configure its own or the dependent
// feature stays off.
//
//   - ADMIN_EMAILS (and its SUPPORT_EMAIL / ADMIN_EMAIL fallbacks): internal
//     alerts are skipped; contact/intake forms 503.
//   - TELEGRAM_BOT_USERNAME (only when Telegram is enabled): the operator
//     connect-link endpoint answers 503 telegram_not_configured.
func operatorRoutingGaps(telegramEnabled bool) []string {
	var gaps []string
	if !emails.HasAdminRecipients() {
		gaps = append(gaps, "ADMIN_EMAILS")
	}
	if telegramEnabled && !server.TelegramBotUsernameConfigured() {
		gaps = append(gaps, "TELEGRAM_BOT_USERNAME")
	}
	return gaps
}

// warnOperatorRoutingGaps logs one startup line per gap (see
// operatorRoutingGaps). Non-fatal: core restaurant flows do not depend on
// either setting.
func warnOperatorRoutingGaps(telegramEnabled bool) {
	for _, gap := range operatorRoutingGaps(telegramEnabled) {
		switch gap {
		case "ADMIN_EMAILS":
			emails.WarnIfAdminEmailsUnset()
		case "TELEGRAM_BOT_USERNAME":
			logger.Logger.Error("TELEGRAM_TOKEN is set but TELEGRAM_BOT_USERNAME is not: operators cannot connect Telegram (generate-token answers 503 telegram_not_configured). Set TELEGRAM_BOT_USERNAME to your bot's @username.")
		}
	}
}
