package services

import (
	"encoding/json"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
)

// drainBusinessEvents drains all events already sitting in the subscription
// channel. Hub publishes are synchronous (buffered channel send under lock),
// so once the service call returns every emitted event is already queued.
func drainBusinessEvents(ch <-chan events.BusinessEvent) []events.BusinessEvent {
	var out []events.BusinessEvent
	for {
		select {
		case ev := <-ch:
			out = append(out, ev)
		default:
			return out
		}
	}
}

func countEventType(evs []events.BusinessEvent, eventType string) int {
	n := 0
	for _, ev := range evs {
		if ev.Type == eventType {
			n++
		}
	}
	return n
}

// TestUpdateDeliveryStatus_DeliveredEmitsOrderUpdated — MED-HIGH fix: when a
// delivery is marked delivered, the linked order flips to delivered too, and
// the Kitchen page (which listens to order.* events) must hear about it via
// exactly one order.updated. Before the fix nothing was published and the
// order sat in the "Ready" column until a manual refresh.
func TestUpdateDeliveryStatus_DeliveredEmitsOrderUpdated(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	_, order, delivery := seedDeliveryTriple(t, svc, businessID)

	// Advance the pair to a delivered-eligible state directly: delivery
	// in_transit, order ready (ready → delivered is the valid order edge).
	if err := svc.db.Model(&database.DeliveryOrder{}).Where("id = ?", delivery.ID).
		Update("status", database.DeliveryStatusInTransit).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.db.Model(&database.Order{}).Where("id = ?", order.ID).
		Update("status", database.OrderStatusOrderReady).Error; err != nil {
		t.Fatal(err)
	}

	ch, _, cancel := events.GetHub().SubscribeWithReplayTopic(businessID, 0)
	defer cancel()

	if err := svc.UpdateDeliveryStatus(delivery.ID, database.DeliveryStatusDelivered, nil, "staff:1"); err != nil {
		t.Fatal(err)
	}

	evs := drainBusinessEvents(ch)
	if got := countEventType(evs, "order.updated"); got != 1 {
		t.Fatalf("delivered must emit exactly one order.updated, got %d (events: %+v)", got, eventTypes(evs))
	}
	// The order.updated payload must carry the delivered status so the
	// Kitchen page can clear the Ready column without a reload.
	for _, ev := range evs {
		if ev.Type != "order.updated" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(ev.Data, &payload); err != nil {
			t.Fatalf("unmarshal order.updated payload: %v", err)
		}
		if payload["status"] != "delivered" {
			t.Fatalf("order.updated payload status = %v, want delivered", payload["status"])
		}
	}
}

// TestAcceptReconcile_EmitsSingleDeliveryUpdated — MED fix: the accept
// reconcile branch (delivery already preparing + order still pending from a
// prior failed approval) emitted delivery.updated twice, doubling operator
// toasts. Exactly one delivery.updated must go out, plus the order.updated
// from the healed approval.
func TestAcceptReconcile_EmitsSingleDeliveryUpdated(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	_, order, delivery := seedDeliveryTriple(t, svc, businessID)

	// Wedge state: delivery committed to preparing, order approval leg failed.
	if err := svc.db.Model(&database.DeliveryOrder{}).Where("id = ?", delivery.ID).
		Update("status", database.DeliveryStatusPreparing).Error; err != nil {
		t.Fatal(err)
	}

	ch, _, cancel := events.GetHub().SubscribeWithReplayTopic(businessID, 0)
	defer cancel()

	if _, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:1", database.DeliveryPaymentCashOnDelivery); err != nil {
		t.Fatal(err)
	}

	evs := drainBusinessEvents(ch)
	if got := countEventType(evs, "delivery.updated"); got != 1 {
		t.Fatalf("accept-reconcile must emit exactly one delivery.updated, got %d (events: %+v)", got, eventTypes(evs))
	}
	if got := countEventType(evs, "order.updated"); got != 1 {
		t.Fatalf("accept-reconcile must emit exactly one order.updated, got %d (events: %+v)", got, eventTypes(evs))
	}
}

// TestAccept_CODEmitsSingleDeliveryUpdated pins the normal (non-reconcile)
// accept path to a single delivery.updated emit.
func TestAccept_CODEmitsSingleDeliveryUpdated(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	_, order, _ := seedDeliveryTriple(t, svc, businessID)

	ch, _, cancel := events.GetHub().SubscribeWithReplayTopic(businessID, 0)
	defer cancel()

	if _, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:1", database.DeliveryPaymentCashOnDelivery); err != nil {
		t.Fatal(err)
	}

	evs := drainBusinessEvents(ch)
	if got := countEventType(evs, "delivery.updated"); got != 1 {
		t.Fatalf("accept must emit exactly one delivery.updated, got %d (events: %+v)", got, eventTypes(evs))
	}
}

// TestCancelDeliveryOrder_ServiceEmitsSingleCancelled pins the lifecycle
// cancel path (the one emit that must survive the handler dedup) to exactly
// one delivery.cancelled.
func TestCancelDeliveryOrder_ServiceEmitsSingleCancelled(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	_, _, delivery := seedDeliveryTriple(t, svc, businessID)

	ch, _, cancel := events.GetHub().SubscribeWithReplayTopic(businessID, 0)
	defer cancel()

	if err := svc.CancelDeliveryOrder(delivery.ID, "customer asked", "staff:1"); err != nil {
		t.Fatal(err)
	}

	evs := drainBusinessEvents(ch)
	if got := countEventType(evs, "delivery.cancelled"); got != 1 {
		t.Fatalf("cancel must emit exactly one delivery.cancelled, got %d (events: %+v)", got, eventTypes(evs))
	}
}

// TestFailedDelivery_CancelledPayloadCarriesFailedStatus — LOW fix contract:
// a FAILED delivery rides the delivery.cancelled topic (no dedicated topic),
// so the payload's status field is the only way consumers can distinguish
// failed from cancelled. Pin that the emitted payload says "failed".
func TestFailedDelivery_CancelledPayloadCarriesFailedStatus(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	_, _, delivery := seedDeliveryTriple(t, svc, businessID)

	if err := svc.db.Model(&database.DeliveryOrder{}).Where("id = ?", delivery.ID).
		Update("status", database.DeliveryStatusInTransit).Error; err != nil {
		t.Fatal(err)
	}

	ch, _, cancel := events.GetHub().SubscribeWithReplayTopic(businessID, 0)
	defer cancel()

	if err := svc.UpdateDeliveryStatus(delivery.ID, database.DeliveryStatusFailed, nil, "staff:1"); err != nil {
		t.Fatal(err)
	}

	evs := drainBusinessEvents(ch)
	if got := countEventType(evs, "delivery.cancelled"); got != 1 {
		t.Fatalf("failed delivery must emit exactly one delivery.cancelled, got %d (events: %+v)", got, eventTypes(evs))
	}
	for _, ev := range evs {
		if ev.Type != "delivery.cancelled" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal(ev.Data, &payload); err != nil {
			t.Fatalf("unmarshal delivery.cancelled payload: %v", err)
		}
		if payload["status"] != "failed" {
			t.Fatalf("delivery.cancelled payload status = %v, want failed", payload["status"])
		}
	}
	// The delivery row itself must end at failed, not cancelled.
	var d database.DeliveryOrder
	if err := svc.db.First(&d, delivery.ID).Error; err != nil {
		t.Fatal(err)
	}
	if d.Status != database.DeliveryStatusFailed {
		t.Fatalf("delivery status = %s, want failed", d.Status)
	}
}

func eventTypes(evs []events.BusinessEvent) []string {
	out := make([]string, 0, len(evs))
	for _, ev := range evs {
		out = append(out, ev.Type)
	}
	return out
}
