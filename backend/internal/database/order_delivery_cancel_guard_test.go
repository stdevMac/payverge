package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedDeliveryLinkedOrder(t *testing.T) (*Business, *Bill, *Order) {
	t.Helper()
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)
	order := helperOrder(t, biz, bill, []OrderItem{
		{ID: "dlv-c-1", MenuItemName: "Pizza", Price: 15, Quantity: 1, Subtotal: 15},
	})
	require.NoError(t, db.Create(&DeliveryOrder{
		BusinessID:     biz.ID,
		BillID:         bill.ID,
		OrderID:        &order.ID,
		DeliveryNumber: "DEL-GUARD-1",
		DeliveryType:   DeliveryTypeInHouse,
		Status:         DeliveryStatusPending,
		CustomerName:   "g",
		CustomerPhone:  "1",
	}).Error)
	return biz, bill, order
}

// TestUpdateOrderStatus_RejectsDirectCancelOfDeliveryLinkedOrder is the
// B-2/X-3 DB-level guard: cancelling a delivery-linked order kitchen-side
// only would leave the delivery leg live (driver assigned, guest uninformed,
// prepaid money in limbo). Symmetric to the existing approve-path guard.
func TestUpdateOrderStatus_RejectsDirectCancelOfDeliveryLinkedOrder(t *testing.T) {
	setupOrderTestDB(t)
	_, _, order := seedDeliveryLinkedOrder(t)

	err := UpdateOrderStatus(order.ID, OrderStatusOrderCancelled, "staff", "won't make it")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDeliveryLinkedCancel)

	var fresh Order
	require.NoError(t, db.First(&fresh, order.ID).Error)
	assert.Equal(t, OrderStatusPending, fresh.Status, "guard must roll back the cancel")
}

// TestCancelDeliveryLinkedOrder_LifecycleBypassStillCancels: the delivery
// lifecycle's internal entry point bypasses the guard — mirroring how
// ApproveDeliveryLinkedOrder is the approve-side bypass.
func TestCancelDeliveryLinkedOrder_LifecycleBypassStillCancels(t *testing.T) {
	setupOrderTestDB(t)
	_, _, order := seedDeliveryLinkedOrder(t)

	require.NoError(t, CancelDeliveryLinkedOrder(order.ID, "staff:lifecycle", "delivery rejected"))

	var fresh Order
	require.NoError(t, db.First(&fresh, order.ID).Error)
	assert.Equal(t, OrderStatusOrderCancelled, fresh.Status)
	assert.Equal(t, "staff:lifecycle", fresh.CancelledBy)
	assert.Equal(t, "delivery rejected", fresh.CancelReason)
}
