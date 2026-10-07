package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupVoidPaymentTestDB(t *testing.T) {
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
		&Payment{},
		&BillHistoryEvent{},
		&BusinessMilestoneEvent{},
		&BusinessRevenueAggregate{},
		&CustomerBusiness{},
		&Order{},
		&OperationalAlert{},
		&OperationalAlertEvent{},
	))
}

// TestApplyConfirmedPayment_RejectsVoidedBill locks F-VOIDPAY: a voided bill is
// terminal and must not accept a late payment. VoidBill only voids unpaid bills
// (PaidAmount == 0), so a voided bill has remaining = TotalAmount > 0; without a
// Voided guard a crypto/webhook/manual payment that arrives after the void
// (e.g. a guest settling a 30-min quote token issued before the operator voided)
// would silently resurrect the bill to partial/paid.
func TestApplyConfirmedPayment_RejectsVoidedBill(t *testing.T) {
	setupVoidPaymentTestDB(t)

	business := &Business{Name: "Void Co", IsActive: true}
	require.NoError(t, db.Create(business).Error)

	bill := &Bill{
		BusinessID:  business.ID,
		BillNumber:  "VOID-PAY-1",
		Status:      BillStatusOpen,
		Items:       "[]",
		TotalAmount: 5000,
		PaidAmount:  0,
	}
	require.NoError(t, db.Create(bill).Error)

	_, err := VoidBill(bill.ID, "staff", "guest left without paying")
	require.NoError(t, err)

	_, applied, err := ApplyConfirmedPayment(ConfirmedPaymentInput{
		BillID:        bill.ID,
		PayerAddr:     "crypto_guest",
		Amount:        5000,
		TxHash:        "tx-late-after-void",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "crypto",
	}, nil)

	require.ErrorIs(t, err, ErrBillNotPayable, "a voided bill must reject new payments")
	assert.False(t, applied, "no payment should be applied to a voided bill")

	var reloaded Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	assert.Equal(t, BillStatusVoided, reloaded.Status, "the bill must stay voided")
	assert.EqualValues(t, 0, reloaded.PaidAmount, "no money should be credited to a voided bill")
}
