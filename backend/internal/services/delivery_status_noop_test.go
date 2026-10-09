package services

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestUpdateDeliveryStatus_NoopIsIdempotent pins the advance-double-fire fix:
// a repeated status write (from == to) — an operator double-click or a retried
// request — must short-circuit BEFORE the Save/history/notify block. The old
// code re-Saved the row (rewriting ActualPickupTime/ActualDeliveryTime to
// "now"), appended a spurious "X → X" history row, and re-sent the
// status-change notification. The no-op must return nil, leave timestamps
// untouched, and add no history rows.
func TestUpdateDeliveryStatus_NoopIsIdempotent(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)

	bill := database.Bill{BusinessID: businessID, BillNumber: "B-NOOP-1", Status: database.BillStatusOpen, Subtotal: 1000, TotalAmount: 1700}
	require.NoError(t, svc.db.Omit("table_id").Create(&bill).Error)
	delivery := database.DeliveryOrder{
		BusinessID: businessID, BillID: bill.ID, DeliveryNumber: "DEL-NOOP-1",
		DeliveryType: database.DeliveryTypeInHouse, Status: database.DeliveryStatusReady,
		CustomerName: "g", CustomerPhone: "1",
	}
	require.NoError(t, svc.db.Create(&delivery).Error)

	// First real advance to picked_up stamps ActualPickupTime and writes history.
	require.NoError(t, svc.UpdateDeliveryStatus(delivery.ID, database.DeliveryStatusPickedUp, nil, "staff:1"))

	var afterFirst database.DeliveryOrder
	require.NoError(t, svc.db.First(&afterFirst, delivery.ID).Error)
	require.NotNil(t, afterFirst.ActualPickupTime, "first picked_up advance must stamp the pickup time")
	firstPickup := *afterFirst.ActualPickupTime

	var historyBefore int64
	require.NoError(t, svc.db.Model(&database.DeliveryStatusHistory{}).
		Where("delivery_order_id = ?", delivery.ID).Count(&historyBefore).Error)

	// Repeat the SAME status: must be a no-op returning nil.
	require.NoError(t, svc.UpdateDeliveryStatus(delivery.ID, database.DeliveryStatusPickedUp, nil, "staff:1"),
		"a redundant same-status write must be treated as an idempotent success")

	var afterNoop database.DeliveryOrder
	require.NoError(t, svc.db.First(&afterNoop, delivery.ID).Error)
	require.NotNil(t, afterNoop.ActualPickupTime)
	require.True(t, afterNoop.ActualPickupTime.Equal(firstPickup),
		"a no-op must NOT rewrite ActualPickupTime")

	var historyAfter int64
	require.NoError(t, svc.db.Model(&database.DeliveryStatusHistory{}).
		Where("delivery_order_id = ?", delivery.ID).Count(&historyAfter).Error)
	require.Equal(t, historyBefore, historyAfter,
		"a no-op must NOT append a spurious X → X history row")
}
