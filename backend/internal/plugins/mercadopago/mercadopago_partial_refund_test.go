package mercadopago

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/plugins"
)

// TestMercadoPagoRefundedCents locks F-MPREFUND: the refunded amount is read
// from MercadoPago's transaction_amount_refunded so a partial refund/chargeback
// is detected from data rather than assumed to be a full reversal.
func TestMercadoPagoRefundedCents(t *testing.T) {
	require.Nil(t, mercadoPagoRefundedCents(nil))
	require.Nil(t, mercadoPagoRefundedCents(&PaymentInfo{TransactionAmountRefunded: 0}))

	// A partial refund of $12.50 surfaces 1250 cents.
	got := mercadoPagoRefundedCents(&PaymentInfo{TransactionAmountRefunded: 12.50})
	require.NotNil(t, got)
	require.EqualValues(t, 1250, *got)

	// A full refund equal to the capture surfaces the full amount (so the
	// downstream partial check r < capture is false -> full reversal).
	full := mercadoPagoRefundedCents(&PaymentInfo{TransactionAmount: 30.00, TransactionAmountRefunded: 30.00})
	require.NotNil(t, full)
	require.EqualValues(t, 3000, *full)
}

// mp-partial-refund-dropped: an approved payment with a partial refund
// declares the cumulative refunded total so the handler can reverse it.
func TestApplyMercadoPagoCumulativeRefund(t *testing.T) {
	resp := &plugins.WebhookResponse{Status: "completed"}
	applyMercadoPagoCumulativeRefund(resp, &PaymentInfo{Status: "approved", TransactionAmount: 100, TransactionAmountRefunded: 30})
	require.NotNil(t, resp.RefundedCumulativeCents)
	require.EqualValues(t, 3000, *resp.RefundedCumulativeCents)

	resp = &plugins.WebhookResponse{Status: "completed"}
	applyMercadoPagoCumulativeRefund(resp, &PaymentInfo{Status: "approved", TransactionAmount: 100})
	require.Nil(t, resp.RefundedCumulativeCents, "nothing refunded yet")

	resp = &plugins.WebhookResponse{Status: "disputed"}
	applyMercadoPagoCumulativeRefund(resp, &PaymentInfo{Status: "in_mediation", TransactionAmountRefunded: 30})
	require.Nil(t, resp.RefundedCumulativeCents)
}
