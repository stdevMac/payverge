package emails

import (
	"fmt"
	"net/mail"
	"strings"
)

func BuildSyntheticMessage(from, to string) (EmailMessage, error) {
	fromAddress, err := mail.ParseAddress(strings.TrimSpace(from))
	if err != nil {
		return EmailMessage{}, fmt.Errorf("invalid synthetic from address: %w", err)
	}
	if strings.Contains(to, ",") || strings.Contains(to, ";") {
		return EmailMessage{}, fmt.Errorf("synthetic recipient must be one mailbox")
	}
	toAddress, err := mail.ParseAddress(strings.TrimSpace(to))
	if err != nil {
		return EmailMessage{}, fmt.Errorf("invalid synthetic recipient: %w", err)
	}
	fromValue := fromAddress.String()
	if fromAddress.Name == "" {
		// net/mail renders a nameless address as <user@example.com>. Resend
		// accepts either the bare mailbox or "Name <mailbox>", but rejects the
		// angle-only form.
		fromValue = fromAddress.Address
	}
	return EmailMessage{
		From:        fromValue,
		To:          []string{toAddress.Address},
		Subject:     "Payverge email delivery health check",
		TextBody:    "This monitored transactional email confirms Payverge delivery.",
		HTMLBody:    "<p>This monitored transactional email confirms Payverge delivery.</p>",
		Tag:         "payverge_email_healthcheck",
		MessageType: MessageTypeTransactional,
	}, nil
}
