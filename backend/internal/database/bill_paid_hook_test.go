package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestApplyConfirmedPayment_InvokesBillPaidHookInTx: when a confirmed payment
// transitions the bill to paid, the injected hook must run INSIDE the
// settlement transaction (Wave 4 fiscal outbox). The hook here writes a
// sentinel row through the tx handle; if the hook ran outside the tx, or not
// at all, the sentinel is absent.
func TestApplyConfirmedPayment_InvokesBillPaidHookInTx(t *testing.T) {
	setupReversalTestDB(t)
	biz := helperBusiness(t, 0, 0)

	bill := Bill{BusinessID: biz.ID, BillNumber: "B-hook-1",
		TotalAmount: 1000, Status: BillStatusOpen}
	require.NoError(t, db.Omit("table_id").Create(&bill).Error)

	var hookBillID uint
	var hookPaymentID *uint
	var sawPaidStatus bool
	var sawSettlementTime bool
	SetBillPaidInTxHook(func(tx *gorm.DB, b *Bill, paymentID, altPaymentID *uint) error {
		hookBillID = b.ID
		hookPaymentID = paymentID
		sawPaidStatus = b.Status == BillStatusPaid
		sawSettlementTime = b.SettledAt != nil
		return nil
	})
	t.Cleanup(func() { SetBillPaidInTxHook(nil) })

	_, applied, err := ApplyConfirmedPayment(ConfirmedPaymentInput{
		BillID: bill.ID, PayerAddr: "hook_test", Amount: 1000,
		TxHash: "0xhook1", Status: PaymentStatusConfirmed, PaymentMethod: "crypto",
	}, nil)
	require.NoError(t, err)
	require.True(t, applied)
	require.Equal(t, bill.ID, hookBillID, "hook must receive the paid bill")
	require.True(t, sawPaidStatus, "hook must see the bill in paid status")
	require.True(t, sawSettlementTime, "hook must see the canonical settlement time")
	require.NotNil(t, hookPaymentID, "hook must receive the settling payment id")
	var settled Bill
	require.NoError(t, db.First(&settled, bill.ID).Error)
	require.NotNil(t, settled.SettledAt, "payment completion must stamp canonical settlement time")
	require.NotNil(t, settled.ClosedAt)
	require.WithinDuration(t, *settled.ClosedAt, *settled.SettledAt, time.Millisecond)
}

// TestApplyConfirmedPayment_PartialPaymentSkipsHook: the hook only fires on
// the transition to paid, never on partial settlements.
func TestApplyConfirmedPayment_PartialPaymentSkipsHook(t *testing.T) {
	setupReversalTestDB(t)
	biz := helperBusiness(t, 0, 0)

	bill := Bill{BusinessID: biz.ID, BillNumber: "B-hook-2",
		TotalAmount: 1000, Status: BillStatusOpen}
	require.NoError(t, db.Omit("table_id").Create(&bill).Error)

	fired := false
	SetBillPaidInTxHook(func(tx *gorm.DB, b *Bill, paymentID, altPaymentID *uint) error {
		fired = true
		return nil
	})
	t.Cleanup(func() { SetBillPaidInTxHook(nil) })

	_, applied, err := ApplyConfirmedPayment(ConfirmedPaymentInput{
		BillID: bill.ID, PayerAddr: "hook_test", Amount: 400,
		TxHash: "0xhook2", Status: PaymentStatusConfirmed, PaymentMethod: "crypto",
	}, nil)
	require.NoError(t, err)
	require.True(t, applied)
	require.False(t, fired, "hook must not fire for a partial payment")
	var partial Bill
	require.NoError(t, db.First(&partial, bill.ID).Error)
	require.Nil(t, partial.SettledAt, "partial collection is not terminal settlement")
}

// TestSettleBillSplitShare_CashFullSettleInvokesBillPaidHookInTx: the
// cash/alt-payment split branch flips the bill to paid via
// applyBillPaymentAmounts directly (bill_split.go), bypassing
// applyConfirmedPaymentTx — so it must invoke the bill-paid hook itself, or a
// bill fully settled by cash splits never gets a factura (W4T2b gap).
func TestSettleBillSplitShare_CashFullSettleInvokesBillPaidHookInTx(t *testing.T) {
	sdb := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBill(t, sdb, "split-hook-full", 4000)

	var hookBillID uint
	var hookAltPaymentID *uint
	var sawPaidStatus bool
	hookFires := 0
	SetBillPaidInTxHook(func(tx *gorm.DB, b *Bill, paymentID, altPaymentID *uint) error {
		hookFires++
		hookBillID = b.ID
		hookAltPaymentID = altPaymentID
		sawPaidStatus = b.Status == BillStatusPaid
		return nil
	})
	t.Cleanup(func() { SetBillPaidInTxHook(nil) })

	share, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-hook-full",
		DisplayName:    "Hook Guest",
		Mode:           BillSplitModeCustom,
		AmountCents:    4000,
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)

	settled, paidBill, applied, err := SettleBillSplitShare(SettleBillSplitShareInput{
		ShareID:        share.ID,
		GuestSessionID: "guest-hook-full",
		IdempotencyKey: "hook-full-settle",
		Tender:         "card",
		PayerAddr:      "cashier",
		Now:            now.Add(time.Minute),
	})
	require.NoError(t, err)
	require.True(t, applied)
	require.Equal(t, BillStatusPaid, paidBill.Status)

	require.Equal(t, 1, hookFires, "hook must fire exactly once for the paying settle")
	require.Equal(t, bill.ID, hookBillID)
	require.True(t, sawPaidStatus, "hook must see the bill in paid status")
	require.NotNil(t, hookAltPaymentID, "hook must receive the settling alternative payment id")
	require.NotNil(t, settled.AlternativePaymentID)
	require.Equal(t, *settled.AlternativePaymentID, *hookAltPaymentID)
}

// TestSettleBillSplitShare_CashPartialSettleSkipsHook: a split settle that
// leaves the bill partially paid must not fire the bill-paid hook.
func TestSettleBillSplitShare_CashPartialSettleSkipsHook(t *testing.T) {
	sdb := setupBillSplitTestDB(t, nil)
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	bill := createBillSplitBill(t, sdb, "split-hook-partial", 10000)

	fired := false
	SetBillPaidInTxHook(func(tx *gorm.DB, b *Bill, paymentID, altPaymentID *uint) error {
		fired = true
		return nil
	})
	t.Cleanup(func() { SetBillPaidInTxHook(nil) })

	share, _, err := HoldBillSplitShare(HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-hook-partial",
		DisplayName:    "Hook Guest",
		Mode:           BillSplitModeCustom,
		AmountCents:    4000,
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)

	_, paidBill, applied, err := SettleBillSplitShare(SettleBillSplitShareInput{
		ShareID:        share.ID,
		GuestSessionID: "guest-hook-partial",
		IdempotencyKey: "hook-partial-settle",
		Tender:         "card",
		PayerAddr:      "cashier",
		Now:            now.Add(time.Minute),
	})
	require.NoError(t, err)
	require.True(t, applied)
	require.Equal(t, BillStatusPartial, paidBill.Status)
	require.False(t, fired, "hook must not fire while the bill is only partially paid")
}
