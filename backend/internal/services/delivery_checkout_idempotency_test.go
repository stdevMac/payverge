package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Wave 4: guest delivery checkout must be idempotent on X-Request-Id. A
// network retry (same ClientRequestID) must replay the ORIGINAL checkout —
// same delivery number, no second Bill/Order/DeliveryOrder — instead of
// double-charging the guest with two orders.
func TestGuestDeliveryCheckout_ReplaysOnDuplicateClientRequestID(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	// SQLite supports partial indexes: create the same guard the genesis
	// schema has on Postgres, so the race path is exercised too.
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_delivery_request_identity
		ON orders (business_id, client_request_id)
		WHERE created_by = 'guest_delivery' AND client_request_id IS NOT NULL`).Error)
	service := NewDeliveryService(db, nil)
	business := setupTipValidationBusiness(t, db, "delivery-idem-replay", 12)

	req := tipCheckoutRequest(0)
	req.ClientRequestID = "delivery-retry-abc123"

	first, err := service.GuestDeliveryCheckout(business.ID, req)
	require.NoError(t, err)
	require.NotNil(t, first.DeliveryOrder)
	assert.False(t, first.Duplicate)

	second, err := service.GuestDeliveryCheckout(business.ID, req)
	require.NoError(t, err)
	require.NotNil(t, second.DeliveryOrder)
	assert.True(t, second.Duplicate, "second submit with the same request id must be flagged as a replay")
	assert.Equal(t, first.DeliveryOrder.DeliveryNumber, second.DeliveryOrder.DeliveryNumber)
	assert.Equal(t, first.Bill.ID, second.Bill.ID)
	assert.Equal(t, first.Order.ID, second.Order.ID)
	assert.Equal(t, first.TrackingURL, second.TrackingURL)

	var orderCount, deliveryCount, billCount int64
	require.NoError(t, db.Model(&database.Order{}).Where("business_id = ?", business.ID).Count(&orderCount).Error)
	require.NoError(t, db.Model(&database.DeliveryOrder{}).Where("business_id = ?", business.ID).Count(&deliveryCount).Error)
	require.NoError(t, db.Model(&database.Bill{}).Where("business_id = ?", business.ID).Count(&billCount).Error)
	assert.Equal(t, int64(1), orderCount, "no second order row")
	assert.Equal(t, int64(1), deliveryCount, "no second delivery row")
	assert.Equal(t, int64(1), billCount, "no second bill row")
}

// Distinct request ids from the same guest are distinct orders — the key
// scopes the retry, not the customer.
func TestGuestDeliveryCheckout_DistinctRequestIDsCreateDistinctOrders(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := setupTipValidationBusiness(t, db, "delivery-idem-distinct", 12)

	reqA := tipCheckoutRequest(0)
	reqA.ClientRequestID = "delivery-rid-A"
	reqB := tipCheckoutRequest(0)
	reqB.ClientRequestID = "delivery-rid-B"

	a, err := service.GuestDeliveryCheckout(business.ID, reqA)
	require.NoError(t, err)
	b, err := service.GuestDeliveryCheckout(business.ID, reqB)
	require.NoError(t, err)
	assert.NotEqual(t, a.DeliveryOrder.DeliveryNumber, b.DeliveryOrder.DeliveryNumber)
}

// An empty request id (legacy clients, curl) keeps today's behavior: every
// submit is a new order, and the partial index ignores NULL ids.
func TestGuestDeliveryCheckout_NoRequestIDKeepsLegacyBehavior(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := setupTipValidationBusiness(t, db, "delivery-idem-legacy", 12)

	first, err := service.GuestDeliveryCheckout(business.ID, tipCheckoutRequest(0))
	require.NoError(t, err)
	second, err := service.GuestDeliveryCheckout(business.ID, tipCheckoutRequest(0))
	require.NoError(t, err)
	assert.NotEqual(t, first.DeliveryOrder.DeliveryNumber, second.DeliveryOrder.DeliveryNumber)
}

// BenchmarkGuestDeliveryCheckoutReplayLookup measures the added pre-check on
// the guest checkout hot path (indexed single-row lookup + two PK loads on
// replay; single not-found probe on first submit).
func BenchmarkGuestDeliveryCheckoutReplayLookup(b *testing.B) {
	t := &testing.T{}
	db := setupHospitalityServiceTestDB(t)
	service := NewDeliveryService(db, nil)
	business := setupTipValidationBusiness(t, db, "delivery-idem-bench", 12)
	req := tipCheckoutRequest(0)
	req.ClientRequestID = "bench-rid"
	if _, err := service.GuestDeliveryCheckout(business.ID, req); err != nil {
		b.Fatalf("seed checkout: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := service.findExistingGuestDeliveryCheckout(business.ID, "bench-rid"); err != nil {
			b.Fatal(err)
		}
	}
}
