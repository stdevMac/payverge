package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stdevmac/payverge/backend/internal/emails"
)

func TestOperatorRoutingGaps(t *testing.T) {
	original := emails.AdminsEmails
	t.Cleanup(func() { emails.AdminsEmails = original })

	emails.AdminsEmails = nil
	t.Setenv("TELEGRAM_BOT_USERNAME", "")
	assert.Equal(t, []string{"ADMIN_EMAILS", "TELEGRAM_BOT_USERNAME"}, operatorRoutingGaps(true))
	assert.Equal(t, []string{"ADMIN_EMAILS"}, operatorRoutingGaps(false),
		"TELEGRAM_BOT_USERNAME is only required when Telegram is enabled")

	emails.AdminsEmails = []string{"ops@example.org"}
	t.Setenv("TELEGRAM_BOT_USERNAME", "@MyRestoBot")
	assert.Empty(t, operatorRoutingGaps(true))
}
