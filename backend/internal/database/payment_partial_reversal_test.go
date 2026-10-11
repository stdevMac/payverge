package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPartialReversePluginPayment_ReducesPaidAmount(t *testing.T) {
	setupReversalTestDB(t)

	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 1000)
	// helperBill may leave bill open unpaid — force settled state.
	require.NoError(t, GetDB().Model(bill).Updates(map[string]interface{}{
		"paid_amount": 1000,
		"status":      BillStatusPaid,
	}).Error)

	txHash := "plugin_partial_rev_1"
	payment := helperPayment(t, bill, 10.0, 0, txHash, PaymentStatusConfirmed)

	require.NoError(t, PartialReversePluginPayment(txHash, 300))

	var refreshedBill Bill
	require.NoError(t, GetDB().First(&refreshedBill, bill.ID).Error)
	require.Equal(t, int64(700), refreshedBill.PaidAmount)
	require.Equal(t, BillStatusPartial, refreshedBill.Status)

	var refreshedPayment Payment
	require.NoError(t, GetDB().Where("tx_hash = ?", payment.TxHash).First(&refreshedPayment).Error)
	require.Equal(t, int64(700), refreshedPayment.Amount)
	require.Equal(t, PaymentStatusConfirmed, refreshedPayment.Status)
}
