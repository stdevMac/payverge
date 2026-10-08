package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// pluginBillOnTable seeds a bill on tableID in status with a confirmed plugin
// payment of paidCents that covers the whole total.
func pluginBillOnTable(t *testing.T, biz *Business, tableID uint, status BillStatus, paidCents int64, txHash string) (*Bill, *Payment) {
	t.Helper()
	bill := helperBill(t, biz, nil, float64(paidCents)/100)
	updates := map[string]interface{}{
		"paid_amount": paidCents,
		"status":      status,
		"table_id":    tableID,
	}
	if status == BillStatusClosed || status == BillStatusVoided {
		updates["closed_at"] = time.Now().Add(-10 * time.Minute)
	}
	require.NoError(t, GetDB().Model(bill).Updates(updates).Error)
	payment := helperPayment(t, bill, float64(paidCents)/100, 0, txHash, PaymentStatusConfirmed)
	return bill, payment
}

func openCheckOnTable(t *testing.T, biz *Business, tableID uint) *Bill {
	t.Helper()
	next := helperBill(t, biz, nil, 25)
	require.NoError(t, GetDB().Model(next).Updates(map[string]interface{}{
		"table_id": tableID,
		"status":   BillStatusOpen,
	}).Error)
	return next
}

// A provider refund, dispute withdrawal or full reversal against a closed
// check must not reopen it: the table already seats a new check, and a second
// open/partial bill there would trip idx_bills_active_per_table and fail the
// webhook on every redelivery.
func TestPluginProviderReversal_ClosedBillStaysClosedWithNewCheckOnTable(t *testing.T) {
	setupReversalTestDB(t)
	biz := helperBusiness(t, 0, 0)
	const tableID uint = 7

	refundBill, _ := pluginBillOnTable(t, biz, tableID, BillStatusClosed, 10000, "plugin_closed_refund")
	disputeBill, _ := pluginBillOnTable(t, biz, tableID, BillStatusClosed, 10000, "plugin_closed_dispute")
	reverseBill, _ := pluginBillOnTable(t, biz, tableID, BillStatusClosed, 10000, "plugin_closed_reverse")
	partialBill, _ := pluginBillOnTable(t, biz, tableID, BillStatusClosed, 10000, "plugin_closed_partial")
	openCheckOnTable(t, biz, tableID)

	applied, err := ApplyPluginRefundCumulative("plugin_closed_refund", 3000)
	require.NoError(t, err)
	require.EqualValues(t, 3000, applied)

	change, err := SetPluginDisputedCents("plugin_closed_dispute", 4000)
	require.NoError(t, err)
	require.EqualValues(t, -4000, change)

	require.NoError(t, ReversePluginPayment("plugin_closed_reverse"))
	require.NoError(t, PartialReversePluginPayment("plugin_closed_partial", 2500))

	for _, tc := range []struct {
		bill *Bill
		paid int64
	}{
		{refundBill, 7000},
		{disputeBill, 6000},
		{reverseBill, 0},
		{partialBill, 7500},
	} {
		var reloaded Bill
		require.NoError(t, GetDB().First(&reloaded, tc.bill.ID).Error)
		require.Equal(t, BillStatusClosed, reloaded.Status, "bill %d", tc.bill.ID)
		require.NotNil(t, reloaded.ClosedAt, "bill %d keeps closed_at", tc.bill.ID)
		require.EqualValues(t, tc.paid, reloaded.PaidAmount, "bill %d money still moves", tc.bill.ID)
	}

	// Reinstating the dispute restores the money and still leaves it closed.
	change, err = SetPluginDisputedCents("plugin_closed_dispute", 0)
	require.NoError(t, err)
	require.EqualValues(t, 4000, change)
	var reinstated Bill
	require.NoError(t, GetDB().First(&reinstated, disputeBill.ID).Error)
	require.Equal(t, BillStatusClosed, reinstated.Status)
	require.EqualValues(t, 10000, reinstated.PaidAmount)
}

// A paid check still reopens on a provider reversal so the money owed shows,
// but not when another check already holds its table: then it stays paid and
// only the money moves.
func TestPluginProviderReversal_PaidBillReopensOnlyWhenTableIsFree(t *testing.T) {
	setupReversalTestDB(t)
	biz := helperBusiness(t, 0, 0)

	freeBill, _ := pluginBillOnTable(t, biz, 11, BillStatusPaid, 10000, "plugin_paid_free_table")
	require.NoError(t, ReversePluginPayment("plugin_paid_free_table"))
	var reopened Bill
	require.NoError(t, GetDB().First(&reopened, freeBill.ID).Error)
	require.Equal(t, BillStatusOpen, reopened.Status)
	require.Zero(t, reopened.PaidAmount)

	busyBill, _ := pluginBillOnTable(t, biz, 12, BillStatusPaid, 10000, "plugin_paid_busy_table")
	openCheckOnTable(t, biz, 12)
	applied, err := ApplyPluginRefundCumulative("plugin_paid_busy_table", 3000)
	require.NoError(t, err)
	require.EqualValues(t, 3000, applied)
	var kept Bill
	require.NoError(t, GetDB().First(&kept, busyBill.ID).Error)
	require.Equal(t, BillStatusPaid, kept.Status)
	require.EqualValues(t, 7000, kept.PaidAmount)

	active, err := HasActiveBillForTableID(12)
	require.NoError(t, err)
	require.True(t, active)
	var activeCount int64
	require.NoError(t, GetDB().Model(&Bill{}).
		Where("table_id = ? AND status IN ?", 12, []BillStatus{BillStatusOpen, BillStatusPartial}).
		Count(&activeCount).Error)
	require.EqualValues(t, 1, activeCount, "the reversal must not add a second active check")
}
