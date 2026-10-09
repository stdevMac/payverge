package services

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Tests for the DELIV-NOTIF-2 follow-up (delivery deep review 2026-07-06):
// the Telegram order.created outbox row for a guest delivery checkout must be
// written INSIDE the same database transaction that creates the bill/order/
// delivery (transactional outbox), so a crash between the checkout commit and
// a separate handler-side enqueue can never lose the operator notification.

func setupDeliveryTelegramBusiness(t *testing.T, db *gorm.DB, slug string) *database.Business {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))
	business := setupTipValidationBusiness(t, db, slug, 12)

	tgPlugin := database.Plugin{Name: "telegram", DisplayName: "Telegram", IsActive: true}
	require.NoError(t, db.Create(&tgPlugin).Error)
	require.NoError(t, db.Create(&database.BusinessPlugin{
		BusinessID: business.ID, PluginID: tgPlugin.ID, IsEnabled: true,
		Config: `{"is_connected":true,"chat_id":"55"}`,
	}).Error)
	ResetTelegramNotificationEligibilityCache()
	return business
}

func countOrderCreatedOutboxRows(t *testing.T, db *gorm.DB, businessID, orderID uint) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Model(&database.PluginNotificationDelivery{}).
		Where("business_id = ? AND plugin_name = ? AND event_type = ? AND event_id = ?",
			businessID, "telegram", PluginEventOrderCreated, fmt.Sprintf("order:%d", orderID)).
		Count(&count).Error)
	return count
}

// TestGuestDeliveryCheckoutWritesTelegramOutboxRow: a successful guest
// delivery checkout for a Telegram-connected business must leave exactly one
// pending order.created outbox row, carrying the delivery source marker.
func TestGuestDeliveryCheckoutWritesTelegramOutboxRow(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	business := setupDeliveryTelegramBusiness(t, db, "delivery-tg-outbox")
	service := NewDeliveryService(db, nil)

	checkout, err := service.GuestDeliveryCheckout(business.ID, tipCheckoutRequest(0))
	require.NoError(t, err)
	require.NotNil(t, checkout.Order)

	var row database.PluginNotificationDelivery
	require.NoError(t, db.Where("business_id = ? AND plugin_name = ? AND event_type = ? AND event_id = ?",
		business.ID, "telegram", PluginEventOrderCreated, fmt.Sprintf("order:%d", checkout.Order.ID)).
		First(&row).Error, "guest delivery checkout must enqueue the order.created outbox row")

	assert.Equal(t, database.PluginNotificationDeliveryStatusPending, row.Status)
	assert.Equal(t, "delivery", row.Payload["source"])
	assert.Equal(t, checkout.Order.OrderNumber, row.Payload["order_number"])
	assert.EqualValues(t, 1, countOrderCreatedOutboxRows(t, db, business.ID, checkout.Order.ID))
}

// TestGuestDeliveryCheckoutOutboxRowRollsBackWithCheckout: the outbox write
// must live on the checkout transaction — if the transaction rolls back, no
// orphan notification row may survive (that is what makes it a transactional
// outbox rather than a post-commit enqueue).
func TestGuestDeliveryCheckoutOutboxRowRollsBackWithCheckout(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	business := setupDeliveryTelegramBusiness(t, db, "delivery-tg-rollback")

	order := database.Order{BusinessID: business.ID, OrderNumber: "O-rollback", BillID: 1}
	bill := database.Bill{BusinessID: business.ID, BillNumber: "B-rollback"}
	forced := errors.New("forced rollback")
	err := db.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, tx.Create(&order).Error)
		require.NoError(t, enqueueDeliveryTelegramOrderCreatedTx(tx, order, bill, business, nil))
		// Row is visible inside the transaction...
		var inTx int64
		require.NoError(t, tx.Model(&database.PluginNotificationDelivery{}).
			Where("event_id = ?", fmt.Sprintf("order:%d", order.ID)).Count(&inTx).Error)
		require.EqualValues(t, 1, inTx)
		return forced
	})
	require.ErrorIs(t, err, forced)

	// ...but rolls back with the checkout: nothing survives the failed tx.
	assert.EqualValues(t, 0, countOrderCreatedOutboxRows(t, db, business.ID, order.ID),
		"a rolled-back checkout must not leave an outbox row")
}

// TestGuestDeliveryCheckoutOutboxReplayIsIdempotent: replaying the enqueue for
// the same order (e.g. a legacy handler-side fallback or a retry) must dedupe
// on (business_id, plugin_name, event_type, event_id) — ON CONFLICT DO NOTHING
// keyed on "order:<id>" — leaving exactly one row and therefore one send.
func TestGuestDeliveryCheckoutOutboxReplayIsIdempotent(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	business := setupDeliveryTelegramBusiness(t, db, "delivery-tg-replay")
	service := NewDeliveryService(db, nil)

	checkout, err := service.GuestDeliveryCheckout(business.ID, tipCheckoutRequest(0))
	require.NoError(t, err)
	require.NotNil(t, checkout.Order)
	require.NotNil(t, checkout.Bill)

	// Replay the enqueue twice more — once on a fresh transaction, once on the
	// autocommit handle (what a handler-layer fallback would do).
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return enqueueDeliveryTelegramOrderCreatedTx(tx, *checkout.Order, *checkout.Bill, business, nil)
	}))
	require.NoError(t, enqueueDeliveryTelegramOrderCreatedTx(nil, *checkout.Order, *checkout.Bill, business, nil))

	assert.EqualValues(t, 1, countOrderCreatedOutboxRows(t, db, business.ID, checkout.Order.ID),
		"replayed enqueues must dedupe to exactly one outbox row (one Telegram send)")
}

// TestGuestDeliveryCheckoutTelegramTotalIsGuestPaidBillTotal: the order.created
// Telegram payload's total_cents is labelled "Total" and must be the guest-paid
// bill total (items + tax/service + delivery fee + driver tip + platform fee),
// not the items-only subtotal. bill.TotalAmount is already cents — do not
// scale it again.
func TestGuestDeliveryCheckoutTelegramTotalIsGuestPaidBillTotal(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	business := setupDeliveryTelegramBusiness(t, db, "delivery-tg-total")
	service := NewDeliveryService(db, nil)

	const driverTipDollars = 4.0
	checkout, err := service.GuestDeliveryCheckout(business.ID, tipCheckoutRequest(driverTipDollars))
	require.NoError(t, err)
	require.NotNil(t, checkout.Order)
	require.NotNil(t, checkout.Bill)
	require.NotNil(t, checkout.DeliveryOrder)

	require.Greater(t, checkout.DeliveryOrder.DeliveryFee, int64(0), "fixture must carry a delivery fee")
	require.Greater(t, checkout.DeliveryOrder.DriverTip, int64(0), "fixture must carry a driver tip")
	require.Greater(t, checkout.Bill.TotalAmount, checkout.Bill.Subtotal,
		"guest-paid total must exceed the items subtotal")

	var row database.PluginNotificationDelivery
	require.NoError(t, db.Where("business_id = ? AND plugin_name = ? AND event_type = ? AND event_id = ?",
		business.ID, "telegram", PluginEventOrderCreated, fmt.Sprintf("order:%d", checkout.Order.ID)).
		First(&row).Error)

	assert.Equal(t, float64(checkout.Bill.TotalAmount), row.Payload["total_cents"],
		"total_cents must be bill.TotalAmount in cents (the guest-paid total)")
	assert.NotEqual(t, float64(checkout.Bill.Subtotal), row.Payload["total_cents"],
		"total_cents must not be the items-only subtotal under a Total label")

	guestPaid := checkout.Bill.Subtotal + checkout.Bill.TaxAmount + checkout.Bill.ServiceFeeAmount +
		checkout.DeliveryOrder.DeliveryFee + checkout.DeliveryOrder.DriverTip + checkout.DeliveryOrder.PlatformFee
	assert.Equal(t, guestPaid, checkout.Bill.TotalAmount,
		"bill.TotalAmount is the source of truth for the guest-paid delivery total")
}

// TestEnqueueDeliveryTelegramOrderCreatedUsesBillTotalCents: the builder must
// prefer bill.TotalAmount (cents) even when the item list sums to a different
// (items-only) figure. Guards the money-unit contract: do not *100 the bill.
func TestEnqueueDeliveryTelegramOrderCreatedUsesBillTotalCents(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	business := setupDeliveryTelegramBusiness(t, db, "delivery-tg-bill-cents")

	order := database.Order{BusinessID: business.ID, OrderNumber: "O-total", BillID: 9}
	require.NoError(t, db.Create(&order).Error)
	bill := database.Bill{
		BusinessID:  business.ID,
		BillNumber:  "B-total",
		Subtotal:    1200,
		TotalAmount: 2516, // items + fee + tip + tax/service, already cents
	}
	items := []database.OrderItem{{Quantity: 1, Subtotal: 12.00}}

	require.NoError(t, enqueueDeliveryTelegramOrderCreatedTx(db, order, bill, business, items))

	var row database.PluginNotificationDelivery
	require.NoError(t, db.Where("event_id = ?", fmt.Sprintf("order:%d", order.ID)).First(&row).Error)
	assert.EqualValues(t, 1, row.Payload["item_count"])
	assert.Equal(t, float64(2516), row.Payload["total_cents"])
	assert.NotEqual(t, float64(1200), row.Payload["total_cents"])
}

// TestGuestDeliveryCheckoutSkipsOutboxWhenTelegramNotConnected: a business
// without a connected Telegram plugin must not accumulate outbox rows.
func TestGuestDeliveryCheckoutSkipsOutboxWhenTelegramNotConnected(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))
	business := setupTipValidationBusiness(t, db, "delivery-tg-off", 12)
	ResetTelegramNotificationEligibilityCache()
	service := NewDeliveryService(db, nil)

	checkout, err := service.GuestDeliveryCheckout(business.ID, tipCheckoutRequest(0))
	require.NoError(t, err)
	require.NotNil(t, checkout.Order)

	assert.EqualValues(t, 0, countOrderCreatedOutboxRows(t, db, business.ID, checkout.Order.ID),
		"no Telegram connection => no outbox row")
}
