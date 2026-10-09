package services

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
)

// TestEmitDeliveryCancelled_OrderCancelledCarriesBothIDKeys is the X-7
// contract test for the delivery-lifecycle emitter: its order.cancelled
// payload must match the server emitter's shape additively (`id` AND
// `order_id`, plus bill_id/status/reason/cancelled_by).
func TestEmitDeliveryCancelled_OrderCancelledCarriesBothIDKeys(t *testing.T) {
	svc := NewDeliveryService(nil, nil) // emit path touches no DB
	orderID := uint(321)
	delivery := &database.DeliveryOrder{
		ID:                 9,
		BusinessID:         88899,
		BillID:             77,
		OrderID:            &orderID,
		Status:             database.DeliveryStatusCancelled,
		CancellationReason: "kitchen closed",
		CancelledBy:        "staff:4",
	}

	ch, _, cancel := events.GetHub().SubscribeWithReplayTopic(delivery.BusinessID, 0)
	defer cancel()

	svc.emitDeliveryCancelled(delivery)

	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-ch:
			if ev.Type != "order.cancelled" {
				continue // delivery.cancelled arrives first
			}
			var payload map[string]any
			if err := json.Unmarshal(ev.Data, &payload); err != nil {
				t.Fatalf("unmarshal payload: %v", err)
			}
			if payload["order_id"] != float64(orderID) || payload["id"] != float64(orderID) {
				t.Fatalf("payload must carry both id keys, got %v", payload)
			}
			if payload["bill_id"] != float64(77) || payload["status"] != "cancelled" {
				t.Fatalf("payload missing bill_id/status: %v", payload)
			}
			if payload["reason"] != "kitchen closed" || payload["cancelled_by"] != "staff:4" {
				t.Fatalf("payload missing reason/cancelled_by: %v", payload)
			}
			return
		case <-deadline:
			t.Fatal("order.cancelled event not received")
		}
	}
}
