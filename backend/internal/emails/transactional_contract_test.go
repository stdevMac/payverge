package emails

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCriticalTransactionalEmailContractsReachProvider(t *testing.T) {
	provider := &captureProvider{}
	server, err := NewEmailServer(
		provider,
		"Payverge <noreply@payverge.io>",
		"updates@payverge.io",
		resolveTemplatesRoot(t),
	)
	require.NoError(t, err)

	require.NoError(t, server.SendEmailVerificationEmail(
		[]string{"verify@example.test"}, "Vera", "https://payverge.io/verify-email?token=verify-contract", "en",
	))
	require.NoError(t, server.SendPasswordResetEmail(
		[]string{"reset@example.test"}, "https://payverge.io/reset-password?token=reset-contract", "en",
	))
	require.NoError(t, server.SendStaffInvitationEmail(
		[]string{"invite@example.test"}, "Contract Cafe", "Inez", "manager",
		"https://payverge.io/staff/invitation?token=invite-contract", "en", 7,
	))
	require.NoError(t, server.SendPaymentReceiptEmail(
		[]string{"receipt@example.test"}, "Contract Cafe", "August 1, 2026", "Card", "receipt-contract",
		[]map[string]interface{}{{"name": "Contract Bowl", "quantity": 1, "line_total": "$12.50"}}, "$12.50", "en",
	))

	require.Len(t, provider.sent, 4)
	contracts := []struct {
		to       string
		bodyMust []string
	}{
		{"verify@example.test", []string{"Vera", "verify-contract"}},
		{"reset@example.test", []string{"reset-contract"}},
		{"invite@example.test", []string{"Contract Cafe", "Inez", "invite-contract"}},
		{"receipt@example.test", []string{"Contract Bowl", "receipt-contract", "$12.50"}},
	}
	for i, contract := range contracts {
		msg := provider.sent[i]
		require.Equal(t, []string{contract.to}, msg.To)
		require.Equal(t, "Payverge <noreply@payverge.io>", msg.From)
		require.NotEmpty(t, strings.TrimSpace(msg.Subject))
		require.Equal(t, MessageTypeTransactional, msg.MessageType)
		require.Empty(t, msg.Headers["List-Unsubscribe"])
		for _, marker := range contract.bodyMust {
			require.Contains(t, msg.HTMLBody, marker)
		}
	}
}
