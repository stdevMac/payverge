package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAltRefundTestDB(t *testing.T) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	db = gormDB
	require.NoError(t, db.AutoMigrate(
		&Business{},
		&Bill{},
		&AlternativePayment{},
		&BillSplitShare{},
		&BillHistoryEvent{},
		&BusinessMilestoneEvent{},
		&BusinessRevenueAggregate{},
		&CustomerBusiness{},
		&Order{},
		&OperationalAlert{},
		&OperationalAlertEvent{},
		&CashRegisterSession{},
		&CashRegisterMovement{},
	))
}

// TestRefundBillAlternativePayment_ReversesNonSplitTip locks R3-BP-1: refunding
// a plain (non-split) manual payment that carried a tip must reverse that tip
// from the parent bill AND the revenue aggregate. Before the fix, refundTip was
// only sourced from a linked split share, so a non-split cash tip stayed booked
// on bill.TipAmount and the aggregate's net tip while the cash drawer already
// logged amount+tip going out — books vs drawer diverged forever.
func TestRefundBillAlternativePayment_ReversesNonSplitTip(t *testing.T) {
	setupAltRefundTestDB(t)

	business := &Business{Name: "Tip Co", IsActive: true}
	require.NoError(t, db.Create(business).Error)

	// Seed the revenue aggregate as if this payment had already been recognized:
	// net revenue = bill amount, net tip = tip amount, one recognized bill.
	require.NoError(t, db.Create(&BusinessRevenueAggregate{
		BusinessID:          business.ID,
		NetRevenueCents:     5000,
		NetTipCents:         800,
		GrossRevenueCents:   5000,
		GrossTipCents:       800,
		PositiveEventCount:  1,
		RecognizedBillCount: 1,
	}).Error)

	// Bill fully paid: total 5000, paid 5800 (bill + tip), tip 800.
	bill := &Bill{
		BusinessID:  business.ID,
		BillNumber:  "ALT-TIP-1",
		Status:      BillStatusPaid,
		Items:       "[]",
		TotalAmount: 5000,
		PaidAmount:  5800,
		TipAmount:   800,
	}
	require.NoError(t, db.Create(bill).Error)

	// A confirmed, non-split cash payment carrying a $8 tip. Amount is the total
	// cash taken (bill + tip); BillAmountCents / TipAmountCents split it.
	confirmedBy := "staff"
	payment := &AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "guest",
		Amount:          5800,
		BillAmountCents: 5000,
		TipAmountCents:  800,
		PaymentMethod:   PaymentMethodCash,
		Status:          AltPaymentStatusConfirmed,
		ConfirmedBy:     confirmedBy,
	}
	require.NoError(t, db.Create(payment).Error)

	refreshedBill, refreshedPayment, err := RefundBillAlternativePayment(
		bill.ID, payment.ID, "staff", "guest changed their mind",
	)
	require.NoError(t, err)
	require.NotNil(t, refreshedBill)
	require.NotNil(t, refreshedPayment)

	// Bill tip must return to zero — the whole tip belonged to this payment.
	assert.EqualValues(t, 0, refreshedBill.TipAmount, "bill tip must be reversed on refund")
	assert.EqualValues(t, 0, refreshedBill.PaidAmount, "paid amount must drop by the full tender")
	assert.Equal(t, AltPaymentStatusRefunded, refreshedPayment.Status)

	// Revenue aggregate net tip must also drop back to zero (delta of -800).
	var aggregate BusinessRevenueAggregate
	require.NoError(t, db.Where("business_id = ?", business.ID).First(&aggregate).Error)
	assert.EqualValues(t, 0, aggregate.NetTipCents, "aggregate net tip must be reversed on refund")
	assert.EqualValues(t, 0, aggregate.NetRevenueCents, "aggregate net revenue must be reversed on refund")
}
