package emails

import "sort"

func managedTemplateNames() []string {
	names := []string{
		"access_removed",
		"account_closure",
		"admin_new_signup",
		"business_registered",
		"daily_summary",
		"director_digest",
		"email_verification",
		"fiscal_receipt",
		"generic_notification",
		"getting_started",
		"guest_feedback",
		"low_stock_alert",
		"milestone_first_order",
		"milestone_revenue_threshold",
		"operational_update",
		"payment_receipt",
		"password_reset",
		"payverge_update",
		// Operator-facing (intentionally NOT in guestTemplateNames below):
		// "you have an unactioned reservation request" nudge.
		"reservation_approval_reminder",
		"reservation_cancelled",
		"reservation_confirmation",
		"reservation_declined",
		"reservation_noshow",
		"reservation_pending",
		"reservation_reminder",
		"reservation_updated",
		"role_update",
		"setup_nudge",
		"staff_added",
		"staff_invitation",
		"staff_login_code",
		"staff_removed",
		"staff_updated",
		"thank_you_guest",
		"wallet_change",
		"weekly_analytics",
		"welcome",
		"welcome_ai",
	}

	sort.Strings(names)
	return names
}

// guestTemplateNames are templates sent to restaurant guests who have NO
// Payverge account. They get the guest footer (no account-settings link, no
// unsubscribe). Keyed on template name only — language-agnostic. Anything not
// listed defaults to the operator footer (fail-safe).
var guestTemplateNames = map[string]struct{}{
	"payment_receipt":          {},
	"fiscal_receipt":           {},
	"thank_you_guest":          {},
	"guest_feedback":           {},
	"reservation_confirmation": {},
	"reservation_declined":     {},
	"reservation_pending":      {},
	"reservation_cancelled":    {},
	"reservation_noshow":       {},
	"reservation_reminder":     {},
	"reservation_updated":      {},
}

func isGuestTemplate(name string) bool {
	_, ok := guestTemplateNames[name]
	return ok
}

// marketingTemplateNames are the marketing-toned operator campaigns — exactly
// the set that honors marketingEmailOptedOut in internal/services. They (plus
// anything sent as MessageTypeBroadcast) carry RFC-8058 List-Unsubscribe
// headers and the tokenized no-auth unsubscribe footer link (P2-10).
// Transactional emails are deliberately absent: they are exempt from the
// opt-out, so advertising a do-nothing unsubscribe would be dishonest and
// Gmail's one-click UI would silently "succeed" at nothing.
var marketingTemplateNames = map[string]struct{}{
	"milestone_first_order":       {},
	"milestone_revenue_threshold": {},
	"payverge_update":             {},
}

func isMarketingToned(templateName string, msgType MessageType) bool {
	if msgType == MessageTypeBroadcast {
		return true
	}
	_, ok := marketingTemplateNames[templateName]
	return ok
}

// footerVariant selects the footer for a message. Broadcast (marketing) always
// wins for CAN-SPAM compliance; guest templates get the no-account footer;
// everything else defaults to the operator footer.
func footerVariant(templateName string, msgType MessageType) string {
	switch {
	case msgType == MessageTypeBroadcast:
		return "broadcast"
	case isGuestTemplate(templateName):
		return "guest"
	default:
		return "operator"
	}
}
