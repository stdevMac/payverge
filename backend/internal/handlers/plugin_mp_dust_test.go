package handlers

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// P2: CLP bill 150050 cents, MP charge floors to 150000; settlement must close
// the bill and record a visible 50-cent rounding adjustment (ManualLedgerEntry
// + mercadopago_rounding payment). Two-decimal and dust ≥ 100 must not absorb.

func TestAbsorbMercadoPagoZeroDecimalDust_CLPClosesBillWithLedger(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.ManualLedgerEntry{}))

	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(business).Update("default_currency", "CLP").Error)

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-dust-clp-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 150050, // CLP 1500.50 stored as cents — not a whole major unit
		PaidAmount:  0,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	// Floored MP settlement of 150000 cents (1500 whole CLP).
	updated, applied, err := NewPluginHandlers(nil, nil).updateBillPaymentStatus(
		bill.ID, "ORD01DUSTCLP", 150000, 0, "CLP", "mercadopago", nil,
	)
	require.NoError(t, err)
	require.True(t, applied)
	require.NotNil(t, updated)

	assert.Equal(t, database.BillStatusPaid, updated.Status, "dust must close the bill")
	assert.Equal(t, int64(150050), updated.PaidAmount, "paid must equal total after dust absorb")

	// Synthetic rounding payment visible on the payment ledger.
	dustPay, err := database.GetPaymentByTxHash("plugin_mp_dust_ORD01DUSTCLP")
	require.NoError(t, err)
	assert.Equal(t, int64(50), dustPay.Amount)
	assert.Equal(t, "mercadopago_rounding", dustPay.PaymentMethod)

	// ManualLedgerEntry expense for accounting visibility.
	var entry database.ManualLedgerEntry
	require.NoError(t, database.GetDB().
		Where("business_id = ? AND reference = ?", business.ID, "mp_dust:bill:"+fmt.Sprint(bill.ID)+":pay:ORD01DUSTCLP").
		First(&entry).Error)
	assert.Equal(t, database.AccountingEntryTypeExpense, entry.EntryType)
	assert.Equal(t, "rounding", entry.Category)
	assert.Equal(t, int64(50), entry.Amount)
	assert.Equal(t, "CLP", entry.Currency)
}

func TestAbsorbMercadoPagoZeroDecimalDust_ARSNoAbsorb(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.ManualLedgerEntry{}))

	business := createTestBusiness(t)
	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-dust-ars-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 150050,
		PaidAmount:  0,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	updated, applied, err := NewPluginHandlers(nil, nil).updateBillPaymentStatus(
		bill.ID, "pay-ars-partial", 150000, 0, "ARS", "mercadopago", nil,
	)
	require.NoError(t, err)
	require.True(t, applied)
	require.NotNil(t, updated)

	assert.NotEqual(t, database.BillStatusPaid, updated.Status)
	assert.Equal(t, int64(150000), updated.PaidAmount)

	_, err = database.GetPaymentByTxHash("plugin_mp_dust_pay-ars-partial")
	assert.Error(t, err, "two-decimal ARS must not create dust payment")

	var count int64
	require.NoError(t, database.GetDB().Model(&database.ManualLedgerEntry{}).
		Where("business_id = ?", business.ID).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestAbsorbMercadoPagoZeroDecimalDust_DustAtOrAboveMajorUnitNotAbsorbed(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.ManualLedgerEntry{}))

	business := createTestBusiness(t)
	// Remaining after settlement would be 150 cents (≥ 1 major unit) — do not absorb.
	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-dust-big-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 150150,
		PaidAmount:  0,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	updated, applied, err := NewPluginHandlers(nil, nil).updateBillPaymentStatus(
		bill.ID, "ORD01DUSTBIG", 150000, 0, "CLP", "mercadopago", nil,
	)
	require.NoError(t, err)
	require.True(t, applied)
	assert.Equal(t, int64(150000), updated.PaidAmount)
	assert.NotEqual(t, database.BillStatusPaid, updated.Status)

	_, err = database.GetPaymentByTxHash("plugin_mp_dust_ORD01DUSTBIG")
	assert.Error(t, err)

	var count int64
	require.NoError(t, database.GetDB().Model(&database.ManualLedgerEntry{}).
		Where("business_id = ?", business.ID).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}

func TestAbsorbMercadoPagoZeroDecimalDust_NonMPPluginNoAbsorb(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.ManualLedgerEntry{}))

	business := createTestBusiness(t)
	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-dust-stripe-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 150050,
		PaidAmount:  0,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	updated, applied, err := NewPluginHandlers(nil, nil).updateBillPaymentStatus(
		bill.ID, "stripe_pi_1", 150000, 0, "CLP", "stripe", nil,
	)
	require.NoError(t, err)
	require.True(t, applied)
	assert.Equal(t, int64(150000), updated.PaidAmount)
	assert.NotEqual(t, database.BillStatusPaid, updated.Status)
}

// Q2: ledger insert failure must roll back the synthetic rounding payment so
// the bill stays partial (recoverable) rather than closed-paid without books.
func TestAbsorbMercadoPagoZeroDecimalDust_LedgerFailureRollsBackPayment(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.ManualLedgerEntry{}))

	orig := mpDustLedgerCreate
	t.Cleanup(func() { mpDustLedgerCreate = orig })
	mpDustLedgerCreate = func(tx *gorm.DB, entry *database.ManualLedgerEntry) error {
		return fmt.Errorf("forced ledger insert failure")
	}

	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(business).Update("default_currency", "CLP").Error)

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-dust-atomic-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 150050,
		PaidAmount:  0,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	// Direct absorb after a partial paid state (MP settlement without dust).
	bill.PaidAmount = 150000
	require.NoError(t, database.GetDB().Save(bill).Error)

	absorbed, absErr := absorbMercadoPagoZeroDecimalDust(bill, "CLP", "ORD01DUSTATOMIC")
	require.Error(t, absErr)
	assert.Nil(t, absorbed)
	assert.Contains(t, absErr.Error(), "ledger")

	_, err := database.GetPaymentByTxHash("plugin_mp_dust_ORD01DUSTATOMIC")
	assert.Error(t, err, "rounding payment must not persist when ledger fails")

	var count int64
	require.NoError(t, database.GetDB().Model(&database.ManualLedgerEntry{}).
		Where("business_id = ? AND reference LIKE ?", business.ID, "mp_dust:%").Count(&count).Error)
	assert.Equal(t, int64(0), count)

	var reloaded database.Bill
	require.NoError(t, database.GetDB().First(&reloaded, bill.ID).Error)
	assert.Equal(t, int64(150000), reloaded.PaidAmount, "MP settlement amount unchanged")
	assert.NotEqual(t, database.BillStatusPaid, reloaded.Status, "bill must stay partial/recoverable")
}

// Q2 end-to-end: settlement path logs absorb error and leaves no dust payment.
func TestUpdateBillPaymentStatus_DustLedgerFailureLeavesPartialBill(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.ManualLedgerEntry{}))

	orig := mpDustLedgerCreate
	t.Cleanup(func() { mpDustLedgerCreate = orig })
	mpDustLedgerCreate = func(tx *gorm.DB, entry *database.ManualLedgerEntry) error {
		return fmt.Errorf("forced ledger insert failure")
	}

	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Model(business).Update("default_currency", "CLP").Error)

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  fmt.Sprintf("B-dust-settle-fail-%d", time.Now().UnixNano()),
		Status:      database.BillStatusOpen,
		Items:       "[]",
		TotalAmount: 150050,
		PaidAmount:  0,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	// Settlement itself must succeed; absorb failure is logged and non-fatal.
	updated, applied, err := NewPluginHandlers(nil, nil).updateBillPaymentStatus(
		bill.ID, "ORD01DUSTSETTLEFAIL", 150000, 0, "CLP", "mercadopago", nil,
	)
	require.NoError(t, err)
	require.True(t, applied)
	require.NotNil(t, updated)

	assert.Equal(t, int64(150000), updated.PaidAmount)
	assert.NotEqual(t, database.BillStatusPaid, updated.Status)

	_, err = database.GetPaymentByTxHash("plugin_mp_dust_ORD01DUSTSETTLEFAIL")
	assert.Error(t, err)

	var count int64
	require.NoError(t, database.GetDB().Model(&database.ManualLedgerEntry{}).
		Where("business_id = ?", business.ID).Count(&count).Error)
	assert.Equal(t, int64(0), count)
}
