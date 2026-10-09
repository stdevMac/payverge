package emails

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScheduledReportSenderPropagatesStableIdempotencyKey(t *testing.T) {
	provider := &captureProvider{}
	server, err := NewEmailServer(
		provider, "reports@payverge.io", "updates@payverge.io", resolveTemplatesRoot(t),
	)
	require.NoError(t, err)
	const key = "report:42:2026-08-01:2026-08-02"
	require.NoError(t, server.SendDailySummaryEmailIdempotent(
		[]string{"owner@example.test"}, "Owner", "5", "USD 10.00", "USD 2.00",
		"https://payverge.io/business/42/dashboard", "en", key,
	))
	require.Len(t, provider.sent, 1)
	require.Equal(t, key, provider.sent[0].IdempotencyKey)
}
