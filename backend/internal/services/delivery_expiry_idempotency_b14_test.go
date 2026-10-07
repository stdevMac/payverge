package services

// B-14 (audit L3-41): the delivery payment-window expiry sweeper must not
// append a second terminal transition to an already-terminal delivery-linked
// order. Production once accumulated nine "Cancelado · system:expiry"
// bill-timeline entries on one order over three weeks when the sweeper
// re-cancelled a terminal row.
//
// Guards under test:
//   - cancelDeliveryLinked: IsTerminal early-exit (no DeliveryStatusHistory
//     write, no order/bill propagation, no second emails)
//   - ExpireUnpaidDeliveries: selects only status=confirmed with a past
//     payment_expires_at (so a cancelled row is never re-selected)
//   - updateOrderStatus / CancelDeliveryLinkedOrder: same-status no-op so a
//     second cancel never appends another BillHistoryEvent order.cancelled

import (
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// countDeliveryTerminalHistory counts DeliveryStatusHistory rows for a
// delivery that record a terminal cancel/fail transition authored by actor.
func countDeliveryTerminalHistory(t *testing.T, svc *DeliveryService, deliveryID uint, actor string) int64 {
	t.Helper()
	var n int64
	if err := svc.db.Model(&database.DeliveryStatusHistory{}).
		Where("delivery_order_id = ? AND changed_by = ? AND status IN ?",
			deliveryID, actor,
			[]database.DeliveryStatus{database.DeliveryStatusCancelled, database.DeliveryStatusFailed}).
		Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

// countOrderCancelTimeline counts bill-history order.cancelled events for an
// order authored by actor — this is the operator "Cancelado · <actor>" timeline
// entry rendered on the bill (audit L3-41).
func countOrderCancelTimeline(t *testing.T, svc *DeliveryService, orderID uint, actor string) int64 {
	t.Helper()
	var n int64
	if err := svc.db.Model(&database.BillHistoryEvent{}).
		Where("order_id = ? AND event_type = ? AND actor = ?",
			orderID, database.BillHistoryEventOrderCanceled, actor).
		Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

// seedExpiredUnpaidPrepayAccept builds a delivery-linked order that the expiry
// sweeper will pick up: accepted as online/prepay (confirmed + payment window),
// unpaid open bill, payment_expires_at in the past.
func seedExpiredUnpaidPrepayAccept(t *testing.T, svc *DeliveryService, businessID uint) (database.Bill, database.Order, database.DeliveryOrder) {
	t.Helper()
	bill, order, delivery := seedDeliveryTriple(t, svc, businessID)
	if _, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:1", database.DeliveryPaymentOnline); err != nil {
		t.Fatalf("accept prepay: %v", err)
	}
	past := time.Now().UTC().Add(-2 * time.Minute)
	if err := svc.db.Model(&database.DeliveryOrder{}).Where("id = ?", delivery.ID).
		Update("payment_expires_at", past).Error; err != nil {
		t.Fatal(err)
	}
	// Reload so callers see post-accept status.
	if err := svc.db.First(&delivery, delivery.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.db.First(&order, order.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.db.First(&bill, bill.ID).Error; err != nil {
		t.Fatal(err)
	}
	if delivery.Status != database.DeliveryStatusConfirmed {
		t.Fatalf("pre-expire fixture: delivery = %s, want confirmed", delivery.Status)
	}
	if bill.Status != database.BillStatusOpen || bill.PaidAmount != 0 {
		t.Fatalf("pre-expire fixture: bill must be unpaid open; status=%s paid=%d", bill.Status, bill.PaidAmount)
	}
	return bill, order, delivery
}

// TestB14_ExpireUnpaidDeliveries_NoDuplicateTerminalTransition pins L3-41:
// one expire cancels once (exactly one terminal DeliveryStatusHistory row and
// exactly one order.cancelled bill-timeline entry with actor system:expiry);
// a second ExpireUnpaidDeliveries pass plus a direct second cancelDeliveryLinked
// (kind=expired) must not append further terminal history, must not error with
// anything other than a documented already-terminal no-op (nil), and must leave
// delivery/order statuses unchanged.
func TestB14_ExpireUnpaidDeliveries_NoDuplicateTerminalTransition(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	_, order, delivery := seedExpiredUnpaidPrepayAccept(t, svc, businessID)
	const expiryActor = "system:expiry"

	// --- First sweep: must cancel exactly once ---
	if err := svc.ExpireUnpaidDeliveries(); err != nil {
		t.Fatalf("first ExpireUnpaidDeliveries: %v", err)
	}

	var dAfterFirst database.DeliveryOrder
	if err := svc.db.First(&dAfterFirst, delivery.ID).Error; err != nil {
		t.Fatal(err)
	}
	if dAfterFirst.Status != database.DeliveryStatusCancelled {
		t.Fatalf("after first expire: delivery = %s, want cancelled", dAfterFirst.Status)
	}
	if dAfterFirst.PaymentExpiresAt != nil {
		t.Fatalf("after first expire: payment_expires_at must be cleared, got %v", dAfterFirst.PaymentExpiresAt)
	}

	var oAfterFirst database.Order
	if err := svc.db.First(&oAfterFirst, order.ID).Error; err != nil {
		t.Fatal(err)
	}
	if oAfterFirst.Status != database.OrderStatusOrderCancelled {
		t.Fatalf("after first expire: order = %s, want cancelled", oAfterFirst.Status)
	}

	histAfterFirst := countDeliveryTerminalHistory(t, svc, delivery.ID, expiryActor)
	if histAfterFirst != 1 {
		t.Fatalf("after first expire: DeliveryStatusHistory terminal rows by %s = %d, want 1", expiryActor, histAfterFirst)
	}
	timelineAfterFirst := countOrderCancelTimeline(t, svc, order.ID, expiryActor)
	if timelineAfterFirst != 1 {
		t.Fatalf("after first expire: order.cancelled bill timeline by %s = %d, want 1", expiryActor, timelineAfterFirst)
	}

	// --- Second sweep: status filter + cleared expiry must select nothing ---
	if err := svc.ExpireUnpaidDeliveries(); err != nil {
		t.Fatalf("second ExpireUnpaidDeliveries: %v", err)
	}

	// --- Direct re-cancel (kind=expired): IsTerminal early-exit must no-op ---
	// Even if a future sweep regression re-selects the row, cancelDeliveryLinked
	// itself must refuse a second terminal write.
	if err := svc.cancelDeliveryLinked(delivery.ID, "payment window expired", expiryActor, "expired", database.DeliveryStatusCancelled); err != nil {
		// Documented already-terminal behaviour is a quiet nil return (not an error
		// sentinel). Any non-nil error here is a regression.
		t.Fatalf("second cancelDeliveryLinked(kind=expired) must be a quiet no-op, got: %v", err)
	}

	var dAfterSecond database.DeliveryOrder
	if err := svc.db.First(&dAfterSecond, delivery.ID).Error; err != nil {
		t.Fatal(err)
	}
	if dAfterSecond.Status != database.DeliveryStatusCancelled {
		t.Fatalf("after re-cancel: delivery = %s, want still cancelled", dAfterSecond.Status)
	}
	if dAfterSecond.CancelledBy != dAfterFirst.CancelledBy {
		t.Fatalf("after re-cancel: cancelled_by mutated %q → %q", dAfterFirst.CancelledBy, dAfterSecond.CancelledBy)
	}
	if dAfterSecond.CancellationReason != dAfterFirst.CancellationReason {
		t.Fatalf("after re-cancel: cancellation_reason mutated %q → %q", dAfterFirst.CancellationReason, dAfterSecond.CancellationReason)
	}

	var oAfterSecond database.Order
	if err := svc.db.First(&oAfterSecond, order.ID).Error; err != nil {
		t.Fatal(err)
	}
	if oAfterSecond.Status != database.OrderStatusOrderCancelled {
		t.Fatalf("after re-cancel: order = %s, want still cancelled", oAfterSecond.Status)
	}

	histAfterSecond := countDeliveryTerminalHistory(t, svc, delivery.ID, expiryActor)
	if histAfterSecond != 1 {
		t.Fatalf("B-14: second terminal DeliveryStatusHistory row appended (got %d, want 1) — expiry sweeper is not idempotent", histAfterSecond)
	}
	timelineAfterSecond := countOrderCancelTimeline(t, svc, order.ID, expiryActor)
	if timelineAfterSecond != 1 {
		t.Fatalf("B-14: second order.cancelled timeline entry appended (got %d, want 1) — L3-41 regression (nine Cancelado·system:expiry entries)", timelineAfterSecond)
	}
}
