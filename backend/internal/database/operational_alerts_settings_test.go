package database

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// Fix 5: payment_refund_review must be part of the wire settings map so the
// FE can enable sound/browser-notification for refund-review alerts and the
// settings UI can render the row. Defaults mirror the FE fallback in
// NotificationPreferencesTab: enabled, non-repeating, high priority.
func TestDefaultBusinessAlertSettingsIncludePaymentRefundReview(t *testing.T) {
	settings := DefaultBusinessAlertSettings(1)

	raw, err := json.Marshal(settings.EventSettings)
	require.NoError(t, err)

	var wire map[string]AlertTypeSetting
	require.NoError(t, json.Unmarshal(raw, &wire))
	refund, ok := wire["payment_refund_review"]
	require.True(t, ok, "settings JSON must contain payment_refund_review")
	require.True(t, refund.Enabled)
	require.False(t, refund.Repeating)
	require.Equal(t, OperationalAlertPriorityHigh, refund.Priority)
}
