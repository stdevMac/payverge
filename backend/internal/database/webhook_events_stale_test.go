package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestWebhookEventReprocessable locks F-WEBHOOKSTUCK: a webhook-event row left
// in "processing" by a crash (before any Mark* call) must become reprocessable
// once it is stale, so the provider's legitimate retry isn't swallowed forever.
// A genuinely in-flight (fresh) processing row stays non-reprocessable.
func TestWebhookEventReprocessable(t *testing.T) {
	base := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)

	require.False(t, WebhookEventReprocessable(nil, base))
	require.True(t, WebhookEventReprocessable(&WebhookEvent{Status: "failed", ReceivedAt: base}, base),
		"a failed event is reprocessable")
	require.False(t, WebhookEventReprocessable(&WebhookEvent{Status: "processed", ReceivedAt: base.Add(-time.Hour)}, base),
		"a processed event is never reprocessable")
	require.False(t, WebhookEventReprocessable(&WebhookEvent{Status: "processing", ReceivedAt: base.Add(-time.Minute)}, base),
		"a fresh processing event is genuinely in flight — not reprocessable")
	require.True(t, WebhookEventReprocessable(&WebhookEvent{Status: "processing", ReceivedAt: base.Add(-StaleWebhookProcessingAfter - time.Minute)}, base),
		"a stale processing event (crashed mid-flight) is reprocessable")
}
