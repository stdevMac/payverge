package emails

import (
	"errors"
	"net/mail"
	"os"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/stdevmac/payverge/backend/internal/config"
)

// ErrNoRecipients is returned by every send path when the message has no
// non-blank recipient. Dispatching such a message would only park a dead
// outbox row (or burn a provider call that is rejected anyway).
var ErrNoRecipients = errors.New("email has no recipients")

// parseAdminEmails splits a comma-separated ADMIN_EMAILS value, dropping blank
// entries. An unset or blank value yields nil — never a hard-coded inbox.
func parseAdminEmails(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// adminEmailsFromEnv resolves the operator inbox: the comma-separated
// ADMIN_EMAILS, else defaultAdminEmails.
func adminEmailsFromEnv() []string {
	if out := parseAdminEmails(os.Getenv("ADMIN_EMAILS")); len(out) > 0 {
		return out
	}
	return defaultAdminEmails()
}

// defaultAdminEmails is where operator alerts go when ADMIN_EMAILS is unset:
// SUPPORT_EMAIL (config.SupportEmail), else the bootstrap ADMIN_EMAIL, else
// nobody. Each must be a bare address; anything else is ignored. It never
// falls back to an upstream mailbox — a self-hosted instance must not mail its
// contact-form traffic to a third party.
func defaultAdminEmails() []string {
	if v := config.SupportEmail(); v != "" {
		return []string{v}
	}
	if v := bareEmailEnv("ADMIN_EMAIL"); v != "" {
		return []string{v}
	}
	return nil
}

// bareEmailEnv returns the env value when it is a single bare address
// ("ops@example.org", not "Ops <ops@example.org>" or a list), else "".
func bareEmailEnv(key string) string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return ""
	}
	addr, err := mail.ParseAddress(raw)
	if err != nil || addr.Name != "" || addr.Address != raw {
		return ""
	}
	return raw
}

// initAdminEmails re-resolves AdminsEmails from the environment.
func initAdminEmails() {
	AdminsEmails = adminEmailsFromEnv()
}

// HasAdminRecipients reports whether an operator inbox is configured.
func HasAdminRecipients() bool {
	return hasRecipient(AdminsEmails)
}

// WarnIfAdminEmailsUnset logs one startup warning when no operator inbox is
// configured and reports whether that is the case. Internal alerts are then
// skipped and public contact/intake forms answer 503 instead of mailing anyone.
func WarnIfAdminEmailsUnset() bool {
	if HasAdminRecipients() {
		return false
	}
	logrus.Warn("No operator inbox: ADMIN_EMAILS, SUPPORT_EMAIL and ADMIN_EMAIL are all unset, so operator alerts (new signups, stuck checkouts, escalations) are skipped. Set ADMIN_EMAILS=ops@your-domain to receive them.")
	return true
}

// sendAdminAlert sends an English internal-alert template to the operator
// inbox, or skips it (nil error) when none is configured: a missing alert
// inbox must never fail the signup/billing flow that triggered the alert.
func (e *EmailServer) sendAdminAlert(templateName string, templateBody map[string]interface{}) error {
	if !HasAdminRecipients() {
		logrus.WithField("template_name", templateName).Warn("admin alert skipped: no operator inbox configured")
		return nil
	}
	return e.SendTransactionalEmail(AdminsEmails, templateName, templateBody, LanguageEnglish)
}

func hasRecipient(to []string) bool {
	for _, addr := range to {
		if strings.TrimSpace(addr) != "" {
			return true
		}
	}
	return false
}
