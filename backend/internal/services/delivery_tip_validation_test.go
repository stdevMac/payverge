package services

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Tests for DEL-PAY-01: the public guest-delivery-checkout route accepted an
// unbounded float64 driver_tip. NaN/Inf survived binding, and huge values
// (e.g. 1e300) overflowed the int64 cents conversion — on amd64 producing a
// NEGATIVE bill total that the expiry sweep then treated as "paid" (free
// food). Tips must be finite, non-negative, and bounded by
// max($500, 5x order subtotal).

func setupTipValidationBusiness(t *testing.T, db *gorm.DB, slug string, burgerPrice float64) *database.Business {
	t.Helper()
	business := createTestHospitalityBusiness(t, db, slug)
	createTestHospitalityMenu(t, db, business.ID, database.MenuItem{
		ID:          "burger",
		Name:        "Burger",
		Price:       burgerPrice,
		IsAvailable: true,
	})
	createDeliverySettings(t, db, business.ID, nil)
	createDeliveryZone(t, db, business.ID, "Everywhere", func(zone *database.DeliveryZone) {
		zone.Boundaries = `{"postal_codes":["99999"]}`
		zone.MinimumOrderAmount = 0
	})
	return business
}

func tipCheckoutRequest(tip float64) GuestDeliveryCheckoutRequest {
	return GuestDeliveryCheckoutRequest{
		CustomerName:  "Tip Guest",
		CustomerPhone: "5557777777",
		CustomerEmail: "tip-bounds@example.com",
		DriverTip:     tip,
		DeliveryAddress: database.DeliveryAddress{
			Street:     "500 Market Street",
			City:       "Anywhere",
			State:      "CA",
			PostalCode: "99999",
			Country:    "US",
		},
		Items: []DeliveryCheckoutItemInput{
			{
				MenuItemName: "Burger",
				MenuItemID:   "burger",
				Quantity:     1,
				Price:        12,
			},
		},
	}
}

func TestGuestDeliveryCheckoutRejectsNonFiniteAndAbsurdTips(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := setupTipValidationBusiness(t, db, "delivery-tip-bounds", 12)

	cases := []struct {
		name string
		tip  float64
	}{
		{name: "NaN", tip: math.NaN()},
		{name: "positive infinity", tip: math.Inf(1)},
		{name: "negative infinity", tip: math.Inf(-1)},
		{name: "int64 overflow magnitude", tip: 1e300},
		{name: "just above the $500 floor cap", tip: 500.01},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.GuestDeliveryCheckout(business.ID, tipCheckoutRequest(tc.tip))
			require.Error(t, err, "tip %v must be rejected", tc.tip)
			assert.ErrorIs(t, err, ErrDeliveryValidation, "tip rejection must be a 4xx validation error")
		})
	}

	// Rejections must not leave partial rows behind.
	var billCount, deliveryCount int64
	require.NoError(t, db.Model(&database.Bill{}).Where("business_id = ?", business.ID).Count(&billCount).Error)
	require.NoError(t, db.Model(&database.DeliveryOrder{}).Where("business_id = ?", business.ID).Count(&deliveryCount).Error)
	assert.Zero(t, billCount)
	assert.Zero(t, deliveryCount)
}

// TestGuestDeliveryCheckoutAcceptsGenerousButSaneTip pins the cap rule
// max($500, 5x subtotal): a $600 tip on a $150 order (cap $750) is generous
// but allowed, and the persisted cents stay sane.
func TestGuestDeliveryCheckoutAcceptsGenerousButSaneTip(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := setupTipValidationBusiness(t, db, "delivery-tip-generous", 150)

	checkout, err := service.GuestDeliveryCheckout(business.ID, tipCheckoutRequest(600))
	require.NoError(t, err, "a $600 tip on a $150 order is within max($500, 5x subtotal)")
	require.NotNil(t, checkout.Bill)
	require.NotNil(t, checkout.DeliveryOrder)

	assert.Equal(t, int64(60000), checkout.DeliveryOrder.DriverTip, "tip must persist as exact cents")
	assert.Greater(t, checkout.Bill.TotalAmount, int64(0), "bill total must stay a sane positive cents value")
}

// TestGuestDeliveryCheckoutRejectsTipAboveFiveTimesSubtotalCap pins the other
// side of the rule: on the same $150 order, a tip above 5x subtotal ($750) is
// rejected even though larger orders could carry it.
func TestGuestDeliveryCheckoutRejectsTipAboveFiveTimesSubtotalCap(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := setupTipValidationBusiness(t, db, "delivery-tip-over-cap", 150)

	_, err := service.GuestDeliveryCheckout(business.ID, tipCheckoutRequest(750.01))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDeliveryValidation)
}
