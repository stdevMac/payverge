package stripe

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func stripeLifecycle(t *testing.T, eventType, object string) (status string, refunded, disputed *int64) {
	t.Helper()
	payload := fmt.Sprintf(`{"type":%q,"data":{"object":%s}}`, eventType, object)
	response, err := (&StripePlugin{}).HandleWebhook(7, []byte(payload), nil)
	require.NoError(t, err)
	require.True(t, response.Success, response.Message)
	return response.Status, response.RefundedCumulativeCents, response.DisputedCents
}

// charge.amount_refunded is cumulative; declare it as such so the handler
// applies only the delta.
func TestStripeChargeRefunded_DeclaresCumulativeRefund(t *testing.T) {
	status, refunded, disputed := stripeLifecycle(t, "charge.refunded",
		`{"id":"ch_1","payment_intent":"pi_1","amount":10000,"amount_refunded":5000,"currency":"usd"}`)
	require.Equal(t, "refunded", status)
	require.NotNil(t, refunded)
	require.EqualValues(t, 5000, *refunded)
	require.Nil(t, disputed)

	_, refunded, _ = stripeLifecycle(t, "charge.refunded",
		`{"id":"ch_1","payment_intent":"pi_1","amount_refunded":500,"currency":"jpy"}`)
	require.EqualValues(t, 50000, *refunded, "zero-decimal minor units convert to cents")
}

func TestStripeDisputeEvents_DeclareDisputedAmount(t *testing.T) {
	dispute := func(status string) string {
		return fmt.Sprintf(`{"id":"dp_1","payment_intent":"pi_1","amount":4000,"currency":"usd","status":%q}`, status)
	}

	status, _, disputed := stripeLifecycle(t, "charge.dispute.funds_withdrawn", dispute("needs_response"))
	require.Equal(t, "reversed", status)
	require.EqualValues(t, 4000, *disputed, "only the disputed amount is withdrawn")

	status, _, disputed = stripeLifecycle(t, "charge.dispute.funds_reinstated", dispute("won"))
	require.Equal(t, "dispute_reinstated", status)
	require.EqualValues(t, 0, *disputed)

	status, _, disputed = stripeLifecycle(t, "charge.dispute.closed", dispute("won"))
	require.Equal(t, "dispute_reinstated", status)
	require.EqualValues(t, 0, *disputed)

	status, _, disputed = stripeLifecycle(t, "charge.dispute.closed", dispute("lost"))
	require.Equal(t, "reversed", status)
	require.EqualValues(t, 4000, *disputed)

	status, _, disputed = stripeLifecycle(t, "charge.dispute.closed",
		`{"id":"dp_1","payment_intent":"pi_1","currency":"usd","status":"lost"}`)
	require.Equal(t, "unsupported", status, "a lost close without an amount must not fully reverse")
	require.Nil(t, disputed)

	status, _, disputed = stripeLifecycle(t, "charge.dispute.created", dispute("needs_response"))
	require.Equal(t, "disputed", status)
	require.Nil(t, disputed, "an opened dispute stays alert-only")
}
