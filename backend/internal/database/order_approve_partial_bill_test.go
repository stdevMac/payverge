package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Approving a still-pending order onto a bill a guest has already partly paid
// must succeed. The collected share stays put, the total grows by the order
// subtotal, and the bill stays partial while that share does not cover it.
func TestUpdateOrderStatus_ApprovesPendingOrderOnPartialBill(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)

	existing := BillItem{
		ID:       "11111111-1111-4111-8111-111111111111",
		Name:     "Steak",
		Price:    10,
		Quantity: 1,
		Subtotal: 10,
		ItemType: "menu_item",
	}
	bill := helperBill(t, biz, []BillItem{existing}, 10)
	require.NoError(t, GetDB().Model(bill).Updates(map[string]interface{}{
		"paid_amount": int64(400),
		"status":      BillStatusPartial,
	}).Error)

	const orderSubtotalDollars = 6.0
	order := helperOrder(t, biz, bill, []OrderItem{
		{ID: "ord-partial-1", MenuItemName: "Wine", Price: orderSubtotalDollars, Quantity: 1, Subtotal: orderSubtotalDollars},
	})

	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", ""))

	var saved Bill
	require.NoError(t, GetDB().First(&saved, bill.ID).Error)
	assert.Equal(t, BillStatusPartial, saved.Status)
	assert.Equal(t, int64(1000)+int64(orderSubtotalDollars*100), saved.TotalAmount)
	assert.Equal(t, int64(400), saved.PaidAmount)

	var updated Order
	require.NoError(t, GetDB().First(&updated, order.ID).Error)
	assert.Equal(t, OrderStatusApproved, updated.Status)
}

func TestUpdateOrderStatus_RejectsApprovalWhenBillIsClosed(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 10)
	require.NoError(t, GetDB().Model(bill).Updates(map[string]interface{}{
		"status": BillStatusClosed,
	}).Error)
	order := helperOrder(t, biz, bill, []OrderItem{
		{ID: "ord-closed-1", MenuItemName: "Soup", Price: 7, Quantity: 1, Subtotal: 7},
	})

	err := UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bill is not open for approved order items")

	var updated Order
	require.NoError(t, GetDB().First(&updated, order.ID).Error)
	assert.Equal(t, OrderStatusPending, updated.Status)
}
