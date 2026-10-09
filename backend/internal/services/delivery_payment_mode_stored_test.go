package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Tests for the payment_mode_stored follow-up (delivery deep review
// 2026-07-06, DEL-OP-2): delivery orders must persist the EFFECTIVE payment
// mode that was in force when the order was created, so the operator UI can
// truthfully flag in-flight orders whose mode differs from the current
// settings. Existing rows (created before the column) stay empty — empty
// means "unknown" and must not produce a mismatch signal.

// TestGuestDeliveryCheckoutPersistsEffectivePaymentModeOnline: business has a
// settlement address, so online payment is available and the resolved default
// mode is "online" — the created delivery order must carry it.
func TestGuestDeliveryCheckoutPersistsEffectivePaymentModeOnline(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := setupTipValidationBusiness(t, db, "delivery-mode-online", 12)

	checkout, err := service.GuestDeliveryCheckout(business.ID, tipCheckoutRequest(0))
	require.NoError(t, err)
	require.NotNil(t, checkout.DeliveryOrder)

	assert.Equal(t, string(database.DeliveryPaymentOnline), checkout.DeliveryOrder.PaymentModeStored,
		"guest checkout must persist the effective payment mode at creation time")

	// And the persisted row (what the operator list endpoint serializes)
	// must carry it too, not just the in-memory DTO.
	var row database.DeliveryOrder
	require.NoError(t, db.First(&row, checkout.DeliveryOrder.ID).Error)
	assert.Equal(t, string(database.DeliveryPaymentOnline), row.PaymentModeStored)
}

// TestGuestDeliveryCheckoutPersistsEffectivePaymentModeCOD: no settlement
// address and no payment plugins → online unavailable → the effective mode
// resolves to cash_on_delivery and must be stored as such.
func TestGuestDeliveryCheckoutPersistsEffectivePaymentModeCOD(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := setupTipValidationBusiness(t, db, "delivery-mode-cod", 12)
	require.NoError(t, db.Model(&database.Business{}).
		Where("id = ?", business.ID).
		Update("settlement_addr", "").Error)

	checkout, err := service.GuestDeliveryCheckout(business.ID, tipCheckoutRequest(0))
	require.NoError(t, err)
	require.NotNil(t, checkout.DeliveryOrder)

	assert.Equal(t, string(database.DeliveryPaymentCashOnDelivery), checkout.DeliveryOrder.PaymentModeStored)
}

// TestStaffCreateDeliveryOrderPersistsEffectivePaymentMode: the staff/operator
// creation path (CreateDeliveryOrder) must stamp the same effective mode.
func TestStaffCreateDeliveryOrderPersistsEffectivePaymentMode(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := setupTipValidationBusiness(t, db, "delivery-mode-staff", 12)

	bill := &database.Bill{
		BusinessID:  business.ID,
		BillNumber:  "B-staff-mode",
		TotalAmount: 1200,
		Status:      database.BillStatusOpen,
	}
	require.NoError(t, db.Omit("table_id").Create(bill).Error)

	order, err := service.CreateDeliveryOrder(CreateDeliveryOrderRequest{
		BusinessID:    business.ID,
		BillID:        bill.ID,
		CustomerName:  "Staff Guest",
		CustomerPhone: "5550000000",
		DeliveryAddress: database.DeliveryAddress{
			Street: "1 Main St", City: "Anywhere", Country: "US",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, string(database.DeliveryPaymentOnline), order.PaymentModeStored,
		"staff-created deliveries must persist the effective payment mode too")
}

// TestLegacyDeliveryOrderPaymentModeStoredStaysEmpty: rows created before the
// column exists carry the empty string ("unknown") — the backfill contract is
// explicitly "no backfill", so nothing may rewrite it after the fact.
func TestLegacyDeliveryOrderPaymentModeStoredStaysEmpty(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	business := createTestHospitalityBusiness(t, db, "delivery-mode-legacy")

	legacy := &database.DeliveryOrder{
		BusinessID:     business.ID,
		BillID:         1,
		DeliveryNumber: "DEL-legacy-mode",
		DeliveryType:   database.DeliveryTypeInHouse,
		Status:         database.DeliveryStatusPending,
		CustomerName:   "Legacy",
		CustomerPhone:  "5551111111",
		QuoteMetadata:  database.JSONRawMessage(`{}`),
	}
	require.NoError(t, db.Create(legacy).Error)

	var row database.DeliveryOrder
	require.NoError(t, db.First(&row, legacy.ID).Error)
	assert.Empty(t, row.PaymentModeStored, "legacy rows must read back as empty = unknown")
}
