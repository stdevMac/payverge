package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCancelOverpaidOrderCreatesRefundReviewAlert(t *testing.T) {
	setupOrderTestDB(t)
	require.NoError(t, db.AutoMigrate(&OperationalAlert{}, &OperationalAlertEvent{}))

	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)
	order := helperOrder(t, biz, bill, []OrderItem{
		{ID: "o1", MenuItemName: "Steak", Price: 20, Quantity: 1, Subtotal: 20},
	})

	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", ""))
	// Guest pays the full 2000¢ bill.
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", bill.ID).
		Updates(map[string]interface{}{"paid_amount": 2000, "status": BillStatusPaid}).Error)
	// Staff cancels the order afterwards: total drops to 0 with 2000¢ collected.
	require.NoError(t, UpdateOrderStatus(order.ID, OrderStatusOrderCancelled, "staff", "kitchen out of stock"))

	var alerts []OperationalAlert
	require.NoError(t, db.Where("business_id = ? AND resource_type = ? AND resource_id = ?",
		biz.ID, OperationalAlertResourceTypeBill, bill.ID).Find(&alerts).Error)
	require.Len(t, alerts, 1, "cancelling paid-for items must open a refund-review alert")
	require.Equal(t, OperationalAlertTypePaymentRefundReview, alerts[0].AlertType)
	require.Equal(t, OperationalAlertPriorityHigh, alerts[0].Priority)
}
