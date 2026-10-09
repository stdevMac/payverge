//go:build whatsapp

package services

// Helpers used only by the whatsmeow-backed manager. They are pure (no
// go.mau.fi/* import) but live behind the `whatsapp` tag so the default build
// carries no unused code.

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strconv"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/pii"
)

// whatsappSenderRef returns a short stable hash of a JID for logs (never raw JID).
func whatsappSenderRef(jid string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(jid)))
	return hex.EncodeToString(sum[:6])
}

// whatsAppDailyMessageBudget caps inbound AI replies per business per UTC day,
// mirroring the HTTP waiter's AI_WAITER_DAILY_MESSAGE_BUDGET (default 2000).
func whatsAppDailyMessageBudget() int64 {
	if v := strings.TrimSpace(os.Getenv("AI_WAITER_DAILY_MESSAGE_BUDGET")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return 2000
}

// whatsAppAllergenPostCheck appends the localized "confirm with staff"
// disclaimer when the model reply mentions allergen vocabulary without already
// referencing staff confirmation — the same food-safety post-check the HTTP
// waiter applies (ai_waiter_handler.go:439-451).
func whatsAppAllergenPostCheck(locale, reply string) string {
	if DetectAllergenIntent(locale, reply) && !MentionsStaffConfirmation(locale, reply) {
		return AppendAllergenDisclaimer(locale, reply)
	}
	return reply
}

// redactWhatsAppUserText scrubs PII from a guest turn before it is persisted to
// the shared ai_waiter_messages table — matching the HTTP waiter's
// pii.Redact-on-ingest (ai_waiter_handler.go:228).
func redactWhatsAppUserText(text string) string { return pii.Redact(text) }
