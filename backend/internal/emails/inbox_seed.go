package emails

import (
	"fmt"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/config"
)

// SendCriticalInboxSeed renders and sends the four launch-critical templates
// to each monitored mailbox. Provider acceptance is not inbox proof; the
// scheduled workflow records the seed ID for external inbox/header review.
func SendCriticalInboxSeed(server *EmailServer, recipients []string, seedID string) error {
	if server == nil {
		return fmt.Errorf("email server is required")
	}
	seedID = strings.TrimSpace(seedID)
	if seedID == "" {
		return fmt.Errorf("inbox seed id is required")
	}
	for _, recipient := range recipients {
		to, err := canonicalEmail(recipient)
		if err != nil {
			return fmt.Errorf("invalid inbox seed recipient: %w", err)
		}
		if err := server.SendEmailVerificationEmail([]string{to}, "Inbox Seed", config.PublicURL()+"/verify-email?token="+seedID, "en"); err != nil {
			return fmt.Errorf("verification seed: %w", err)
		}
		if err := server.SendPasswordResetEmail([]string{to}, config.PublicURL()+"/reset-password?token="+seedID, "en"); err != nil {
			return fmt.Errorf("password-reset seed: %w", err)
		}
		if err := server.SendStaffInvitationEmail([]string{to}, "Payverge Inbox Seed", "Inbox Seed", "manager", config.PublicURL()+"/staff/invitation?token="+seedID, "en", 7); err != nil {
			return fmt.Errorf("invitation seed: %w", err)
		}
		if err := server.SendPaymentReceiptEmail(
			[]string{to}, "Payverge Inbox Seed", "August 1, 2026", "Test", seedID,
			[]map[string]interface{}{{"name": "Inbox seed", "quantity": 1, "line_total": "$0.01"}}, "$0.01", "en",
		); err != nil {
			return fmt.Errorf("receipt seed: %w", err)
		}
	}
	return nil
}
