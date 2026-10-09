package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func paidPluginBill(t *testing.T, paidCents, tipCents int64, txHash string) (*Bill, *Payment) {
	t.Helper()
	setupReversalTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, float64(paidCents)/100)
	require.NoError(t, GetDB().Model(bill).Updates(map[string]interface{}{
		"paid_amount": paidCents,
		"tip_amount":  tipCents,
		"status":      BillStatusPaid,
	}).Error)
	payment := helperPayment(t, bill, float64(paidCents)/100, float64(tipCents)/100, txHash, PaymentStatusConfirmed)
	return bill, payment
}

func reloadBillAndPayment(t *testing.T, billID uint, txHash string) (Bill, Payment) {
	t.Helper()
	var bill Bill
	require.NoError(t, GetDB().First(&bill, billID).Error)
	var payment Payment
	require.NoError(t, GetDB().Where("tx_hash = ?", txHash).First(&payment).Error)
	return bill, payment
}

// stripe-cumulative-refund: provider totals are cumulative. 30 then a
// cumulative 50 reverses 30 then 20; a replay is a no-op.
func TestApplyPluginRefundCumulative_ReversesOnlyTheDelta(t *testing.T) {
	const txHash = "plugin_ch_cumulative"
	bill, _ := paidPluginBill(t, 10000, 0, txHash)

	applied, err := ApplyPluginRefundCumulative(txHash, 3000)
	require.NoError(t, err)
	require.EqualValues(t, 3000, applied)
	b, p := reloadBillAndPayment(t, bill.ID, txHash)
	require.EqualValues(t, 7000, b.PaidAmount)
	require.EqualValues(t, 7000, p.Amount)

	applied, err = ApplyPluginRefundCumulative(txHash, 5000)
	require.NoError(t, err)
	require.EqualValues(t, 2000, applied, "cumulative 50 after 30 reverses only 20")
	b, p = reloadBillAndPayment(t, bill.ID, txHash)
	require.EqualValues(t, 5000, b.PaidAmount)
	require.Equal(t, BillStatusPartial, b.Status)
	require.EqualValues(t, 5000, p.Amount)
	require.EqualValues(t, 5000, p.ProviderRefundedCents)
	require.Equal(t, PaymentStatusConfirmed, p.Status)

	for _, replay := range []int64{5000, 3000} {
		applied, err = ApplyPluginRefundCumulative(txHash, replay)
		require.NoError(t, err)
		require.Zero(t, applied, "replayed or out-of-order cumulative %d is a no-op", replay)
	}
	b, p = reloadBillAndPayment(t, bill.ID, txHash)
	require.EqualValues(t, 5000, b.PaidAmount)
	require.EqualValues(t, 5000, p.Amount)

	applied, err = ApplyPluginRefundCumulative(txHash, 10000)
	require.NoError(t, err)
	require.EqualValues(t, 5000, applied)
	b, p = reloadBillAndPayment(t, bill.ID, txHash)
	require.Zero(t, b.PaidAmount)
	require.Equal(t, BillStatusOpen, b.Status)
	require.Equal(t, PaymentStatusRefunded, p.Status)
}

func TestApplyPluginRefundCumulative_OperatorRefundedPaymentIsLeftAlone(t *testing.T) {
	const txHash = "plugin_ch_operator_refunded"
	bill, payment := paidPluginBill(t, 10000, 0, txHash)
	require.NoError(t, GetDB().Model(payment).Update("status", PaymentStatusRefunded).Error)
	require.NoError(t, GetDB().Model(bill).Update("paid_amount", 0).Error)

	applied, err := ApplyPluginRefundCumulative(txHash, 10000)
	require.NoError(t, err)
	require.Zero(t, applied)
	b, _ := reloadBillAndPayment(t, bill.ID, txHash)
	require.Zero(t, b.PaidAmount)
}

// stripe-dispute-reversal: a 40-of-100 dispute reverses 40, not 100, and a
// reinstatement (funds_reinstated / won) restores it.
func TestSetPluginDisputedCents_PartialDisputeReversesAndReinstates(t *testing.T) {
	const txHash = "plugin_ch_dispute"
	bill, _ := paidPluginBill(t, 10000, 0, txHash)

	change, err := SetPluginDisputedCents(txHash, 4000)
	require.NoError(t, err)
	require.EqualValues(t, -4000, change)
	b, p := reloadBillAndPayment(t, bill.ID, txHash)
	require.EqualValues(t, 6000, b.PaidAmount)
	require.Equal(t, BillStatusPartial, b.Status)
	require.EqualValues(t, 6000, p.Amount)
	require.Equal(t, PaymentStatusConfirmed, p.Status, "a partial dispute does not reverse the whole payment")

	change, err = SetPluginDisputedCents(txHash, 4000)
	require.NoError(t, err)
	require.Zero(t, change, "replayed withdrawal is a no-op")

	change, err = SetPluginDisputedCents(txHash, 0)
	require.NoError(t, err)
	require.EqualValues(t, 4000, change)
	b, p = reloadBillAndPayment(t, bill.ID, txHash)
	require.EqualValues(t, 10000, b.PaidAmount)
	require.Equal(t, BillStatusPaid, b.Status)
	require.EqualValues(t, 10000, p.Amount)
	require.Zero(t, p.ProviderDisputedCents)

	change, err = SetPluginDisputedCents(txHash, 0)
	require.NoError(t, err)
	require.Zero(t, change, "replayed reinstatement is a no-op")
}

func TestSetPluginDisputedCents_FullDisputeWithTipRestoresTipAndStatus(t *testing.T) {
	const txHash = "plugin_ch_dispute_full"
	bill, _ := paidPluginBill(t, 10000, 1000, txHash)

	change, err := SetPluginDisputedCents(txHash, 11000)
	require.NoError(t, err)
	require.EqualValues(t, -11000, change)
	b, p := reloadBillAndPayment(t, bill.ID, txHash)
	require.Zero(t, b.PaidAmount)
	require.Zero(t, b.TipAmount)
	require.Equal(t, PaymentStatusReversed, p.Status)
	require.EqualValues(t, 1000, p.ProviderDisputedTipCents)

	change, err = SetPluginDisputedCents(txHash, 0)
	require.NoError(t, err)
	require.EqualValues(t, 11000, change)
	b, p = reloadBillAndPayment(t, bill.ID, txHash)
	require.EqualValues(t, 10000, b.PaidAmount)
	require.EqualValues(t, 1000, b.TipAmount)
	require.Equal(t, BillStatusPaid, b.Status)
	require.Equal(t, PaymentStatusConfirmed, p.Status)
	require.Nil(t, p.ReversedAt)
	require.EqualValues(t, 10000, p.Amount)
	require.EqualValues(t, 1000, p.TipAmount)
}

// A partial dispute, then a provider refund of the undisputed remainder, then
// a won dispute: the refund zeroed the row (status refunded) but the withdrawn
// 40 comes back, so it must be restored rather than ignored.
func TestSetPluginDisputedCents_ReinstatesAfterRefundOfRemainder(t *testing.T) {
	const txHash = "plugin_ch_dispute_then_refund"
	bill, _ := paidPluginBill(t, 10000, 0, txHash)

	change, err := SetPluginDisputedCents(txHash, 4000)
	require.NoError(t, err)
	require.EqualValues(t, -4000, change)

	applied, err := ApplyPluginRefundCumulative(txHash, 6000)
	require.NoError(t, err)
	require.EqualValues(t, 6000, applied)
	b, p := reloadBillAndPayment(t, bill.ID, txHash)
	require.Zero(t, b.PaidAmount)
	require.Equal(t, PaymentStatusRefunded, p.Status)

	change, err = SetPluginDisputedCents(txHash, 0)
	require.NoError(t, err)
	require.EqualValues(t, 4000, change, "the won dispute amount returns to the ledger")
	b, p = reloadBillAndPayment(t, bill.ID, txHash)
	require.EqualValues(t, 4000, b.PaidAmount)
	require.EqualValues(t, 4000, p.Amount)
	require.Zero(t, p.ProviderDisputedCents)
	require.EqualValues(t, 6000, p.ProviderRefundedCents)
	require.Equal(t, PaymentStatusConfirmed, p.Status)

	change, err = SetPluginDisputedCents(txHash, 0)
	require.NoError(t, err)
	require.Zero(t, change, "a repeated reinstatement is a no-op")
}

// An operator-refunded payment that was never disputed is not touched by a
// dispute reinstatement.
func TestSetPluginDisputedCents_UndisputedRefundedPaymentIsLeftAlone(t *testing.T) {
	const txHash = "plugin_ch_refunded_no_dispute"
	bill, payment := paidPluginBill(t, 10000, 0, txHash)
	require.NoError(t, GetDB().Model(payment).Updates(map[string]interface{}{"status": PaymentStatusRefunded, "amount": 0}).Error)
	require.NoError(t, GetDB().Model(bill).Update("paid_amount", 0).Error)

	change, err := SetPluginDisputedCents(txHash, 0)
	require.NoError(t, err)
	require.Zero(t, change)
	b, p := reloadBillAndPayment(t, bill.ID, txHash)
	require.Zero(t, b.PaidAmount)
	require.Equal(t, PaymentStatusRefunded, p.Status)
}
