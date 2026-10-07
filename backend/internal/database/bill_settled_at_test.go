package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #798: settled_at is accounting evidence that money actually settled.
// Leftover 762 carried settled_at=2026-08-15T15:45:08Z with paid $0 and
// remaining $37.82 — a settlement timestamp on a check nobody paid. No
// terminal transition that moves zero money may stamp it.

// A $0 empty walk-in close terminals as voided (#712/#704). Voided means "no
// service happened" — there is no settlement to timestamp.
func TestCloseBillWithHistory_ZeroDollarVoidDoesNotStampSettledAt(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)

	require.NoError(t, CloseBillWithHistory(bill.ID, nil))

	var reloaded Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	require.Equal(t, BillStatusVoided, reloaded.Status)
	require.NotNil(t, reloaded.ClosedAt, "void still records when the check ended")
	assert.Nil(t, reloaded.SettledAt,
		"a $0 voided walk-in settled no money — settled_at must stay NULL (#798)")
}

// The real close (paid in full) keeps stamping settled_at — that is the one
// case where money genuinely settled.
func TestCloseBillWithHistory_PaidInFullCloseStillStampsSettledAt(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 37.82)
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", bill.ID).
		Update("paid_amount", bill.TotalAmount).Error)

	require.NoError(t, CloseBillWithHistory(bill.ID, nil))

	var reloaded Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	require.Equal(t, BillStatusClosed, reloaded.Status)
	require.NotNil(t, reloaded.SettledAt, "a fully-paid close is a settlement")
}

// The delivery walk-out abandon door closes the check with $0 collected —
// abandoned_at and closed_at are stamped, settled_at must never be (#798).
func TestAbandonUnpaidOpenBill_NeverStampsSettledAt(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 37.82)
	require.NoError(t, db.Create(&Order{
		BillID:      bill.ID,
		BusinessID:  biz.ID,
		OrderNumber: "K-798-WALKOUT",
		Status:      OrderStatusOrderCancelled,
		Items:       "[]",
	}).Error)

	require.NoError(t, AbandonUnpaidOpenBill(bill.ID, "system:expiry", "delivery cancelled unpaid"))

	var reloaded Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	require.Equal(t, BillStatusAbandoned, reloaded.Status)
	require.NotNil(t, reloaded.AbandonedAt)
	require.NotNil(t, reloaded.ClosedAt)
	assert.Nil(t, reloaded.SettledAt,
		"an abandoned unpaid check settled no money — settled_at must stay NULL (#798)")
}
