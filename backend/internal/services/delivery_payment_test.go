package services

import (
	"errors"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Payment on an accepted prepay order starts the kitchen.
func TestHandleBillPaid_AdvancesConfirmedDelivery(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	bill, order, delivery := seedDeliveryTriple(t, svc, businessID)
	if _, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:1", database.DeliveryPaymentOnline); err != nil {
		t.Fatal(err)
	}
	// Simulate the bill flipping to paid.
	svc.db.Model(&database.Bill{}).Where("id = ?", bill.ID).Updates(map[string]interface{}{"status": database.BillStatusPaid, "paid_amount": 1700})

	if err := svc.HandleDeliveryBillPaid(bill.ID); err != nil {
		t.Fatal(err)
	}
	var d database.DeliveryOrder
	svc.db.First(&d, delivery.ID)
	if d.Status != database.DeliveryStatusPreparing {
		t.Fatalf("delivery = %s, want preparing", d.Status)
	}
	if d.PaymentExpiresAt != nil {
		t.Fatal("expiry must clear on payment")
	}
	var o database.Order
	svc.db.First(&o, order.ID)
	if o.Status != database.OrderStatusApproved {
		t.Fatalf("order = %s, want approved", o.Status)
	}
	// Idempotent + no-delivery bills are no-ops.
	if err := svc.HandleDeliveryBillPaid(bill.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.HandleDeliveryBillPaid(999999); err != nil {
		t.Fatal("bills without deliveries must be a silent no-op")
	}
}

// A partial payment must not advance the delivery — the kitchen stays idle
// until the bill is fully settled.
func TestHandleBillPaid_IgnoresPartialPayment(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	bill, order, delivery := seedDeliveryTriple(t, svc, businessID)
	if _, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:1", database.DeliveryPaymentOnline); err != nil {
		t.Fatal(err)
	}
	// Bill partially paid: 500 of 1700, status still open.
	svc.db.Model(&database.Bill{}).Where("id = ?", bill.ID).Updates(map[string]interface{}{"paid_amount": 500, "status": database.BillStatusOpen})

	if err := svc.HandleDeliveryBillPaid(bill.ID); err != nil {
		t.Fatalf("partial payment must be a silent no-op, got: %v", err)
	}
	var d database.DeliveryOrder
	svc.db.First(&d, delivery.ID)
	if d.Status != database.DeliveryStatusConfirmed {
		t.Fatalf("delivery = %s after partial payment, want confirmed", d.Status)
	}
	if d.PaymentExpiresAt == nil {
		t.Fatal("expiry must remain set — partial payment does not clear it")
	}
	var o database.Order
	svc.db.First(&o, order.ID)
	if o.Status != database.OrderStatusPending {
		t.Fatalf("order = %s after partial payment, want pending", o.Status)
	}
}

// The expiry sweep cancels unpaid expired orders and (race guard) advances
// paid ones it catches first.
func TestExpireUnpaidDeliveries(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	_, order, delivery := seedDeliveryTriple(t, svc, businessID)
	if _, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:1", database.DeliveryPaymentOnline); err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-1 * time.Minute)
	svc.db.Model(&database.DeliveryOrder{}).Where("id = ?", delivery.ID).Update("payment_expires_at", past)

	if err := svc.ExpireUnpaidDeliveries(); err != nil {
		t.Fatal(err)
	}
	var d database.DeliveryOrder
	svc.db.First(&d, delivery.ID)
	if d.Status != database.DeliveryStatusCancelled {
		t.Fatalf("unpaid expired delivery = %s, want cancelled", d.Status)
	}
	var b database.Bill
	svc.db.First(&b, d.BillID)
	if b.Status == database.BillStatusOpen {
		t.Fatal("expired order's bill must close")
	}
}

// A partially-paid prepay delivery whose window lapsed must be CANCELLED by the
// sweep — not left stuck in 'confirmed'. The sweep used to divert any bill with
// PaidAmount > 0 to HandleDeliveryBillPaid, which no-ops on a partial bill, so
// the delivery was neither advanced nor cancelled (money captured, stuck row).
func TestExpire_PartiallyPaidBill_IsCancelled(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	bill, order, delivery := seedDeliveryTriple(t, svc, businessID)
	if _, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:1", database.DeliveryPaymentOnline); err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-1 * time.Minute)
	svc.db.Model(&database.DeliveryOrder{}).Where("id = ?", delivery.ID).Update("payment_expires_at", past)
	// Bill partially paid: 500 of 1700, status still open (e.g. on-chain underpayment).
	svc.db.Model(&database.Bill{}).Where("id = ?", bill.ID).
		Updates(map[string]interface{}{"paid_amount": 500, "status": database.BillStatusOpen})

	if err := svc.ExpireUnpaidDeliveries(); err != nil {
		t.Fatal(err)
	}
	var d database.DeliveryOrder
	svc.db.First(&d, delivery.ID)
	if d.Status != database.DeliveryStatusCancelled {
		t.Fatalf("partially-paid expired delivery = %s, want cancelled (must not stay confirmed)", d.Status)
	}
	// No-refund-by-design: a bill with captured money is NOT auto-closed (it stays
	// open for operator review — see cancelDeliveryLinked, which only closes when
	// PaidAmount == 0). The fix's job is to unstick the delivery, not to force a
	// bill with money on it. The captured amount must be preserved.
	var b database.Bill
	svc.db.First(&b, d.BillID)
	if b.PaidAmount != 500 {
		t.Fatalf("captured partial amount = %d, want preserved (500)", b.PaidAmount)
	}
}

func TestExpire_SkipsPaidBills(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	bill, order, delivery := seedDeliveryTriple(t, svc, businessID)
	if _, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:1", database.DeliveryPaymentOnline); err != nil {
		t.Fatal(err)
	}
	past := time.Now().UTC().Add(-1 * time.Minute)
	svc.db.Model(&database.DeliveryOrder{}).Where("id = ?", delivery.ID).Update("payment_expires_at", past)
	svc.db.Model(&database.Bill{}).Where("id = ?", bill.ID).Updates(map[string]interface{}{"status": database.BillStatusPaid, "paid_amount": 1700})

	if err := svc.ExpireUnpaidDeliveries(); err != nil {
		t.Fatal(err)
	}
	var d database.DeliveryOrder
	svc.db.First(&d, delivery.ID)
	if d.Status != database.DeliveryStatusPreparing {
		t.Fatalf("paid-at-the-buzzer delivery must advance, got %s", d.Status)
	}
}

// TestExpire_InTxRaceGuard_PaidBillNotCancelled: calls cancelDeliveryLinked
// directly (bypassing the pre-tx paid check) on a confirmed delivery whose bill
// is already paid. This simulates the race where payment commits between the
// sweep's pre-check read and the cancel transaction lock.
// Invariant: the delivery must NOT end up cancelled.
func TestExpire_InTxRaceGuard_PaidBillNotCancelled(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	bill, order, delivery := seedDeliveryTriple(t, svc, businessID)

	// Accept as prepay: delivery → confirmed, order stays pending.
	if _, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:1", database.DeliveryPaymentOnline); err != nil {
		t.Fatal(err)
	}

	// Mark bill as paid (simulates payment committed after sweep pre-check).
	if err := svc.db.Model(&database.Bill{}).Where("id = ?", bill.ID).
		Updates(map[string]interface{}{"status": database.BillStatusPaid, "paid_amount": 1700}).Error; err != nil {
		t.Fatal(err)
	}

	// Call cancelDeliveryLinked with kind="expired" directly — bypasses the
	// sweep's pre-tx read so we exercise the in-tx guard.
	err := svc.cancelDeliveryLinked(delivery.ID, "payment window expired", "system:expiry", "expired", database.DeliveryStatusCancelled)
	// Must return errExpiredButPaid (not nil, not a different error).
	if !errors.Is(err, errExpiredButPaid) {
		t.Fatalf("expected errExpiredButPaid, got %v", err)
	}

	// The delivery must NOT be cancelled.
	var d database.DeliveryOrder
	svc.db.First(&d, delivery.ID)
	if d.Status == database.DeliveryStatusCancelled {
		t.Fatal("paid delivery must not be cancelled by expiry sweep")
	}
	if d.Status != database.DeliveryStatusConfirmed {
		t.Fatalf("delivery = %s after aborted expiry, want confirmed", d.Status)
	}
}

// Track-path contract: an unset stored payment mode must resolve (never "")
// so the tracking page can decide whether to show the Pay button.
// The test business (SettlementAddr "0xsettlement") has online payments
// available, so the resolved mode must be "online".
// A business with no online method would resolve to "cash_on_delivery".
// Either way the mode is never the empty string that was returned by the
// raw GetDeliverySettings call (the bug this test pins).
func TestResolvedPaymentModeNeverEmptyOnPublicRead(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	// public=true: exercises the gate (IsActive+BusinessPageEnabled) that the
	// track handler faces. The test business passes both checks.
	dto, err := svc.GetDeliverySettingsDTO(businessID, true)
	if err != nil {
		t.Fatal(err)
	}
	if dto.PaymentMode == "" {
		t.Fatal("public settings DTO must always carry a resolved payment mode")
	}
	if dto.PaymentMode != string(database.DeliveryPaymentOnline) &&
		dto.PaymentMode != string(database.DeliveryPaymentCashOnDelivery) {
		t.Fatalf("unexpected mode %q", dto.PaymentMode)
	}
}

// ReconcilePaidDeliveries is the durable backstop when the payment-path hook
// log-and-continues: a confirmed prepay delivery with a fully paid bill must
// advance to preparing without waiting for payment_expires_at.
func TestReconcilePaidDeliveries_AdvancesStuckConfirmed(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	bill, order, delivery := seedDeliveryTriple(t, svc, businessID)
	if _, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:1", database.DeliveryPaymentOnline); err != nil {
		t.Fatal(err)
	}
	// Money landed but the pay-hook was missed (simulated by not calling HandleDeliveryBillPaid).
	if err := svc.db.Model(&database.Bill{}).Where("id = ?", bill.ID).Updates(map[string]interface{}{
		"status":      database.BillStatusPaid,
		"paid_amount": bill.TotalAmount,
	}).Error; err != nil {
		t.Fatal(err)
	}
	// Keep a far-future expiry so ExpireUnpaidDeliveries would not touch this row.
	far := time.Now().UTC().Add(24 * time.Hour)
	if err := svc.db.Model(&database.DeliveryOrder{}).Where("id = ?", delivery.ID).
		Update("payment_expires_at", far).Error; err != nil {
		t.Fatal(err)
	}

	advanced, err := svc.ReconcilePaidDeliveries(50)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if advanced != 1 {
		t.Fatalf("advanced = %d, want 1", advanced)
	}
	var d database.DeliveryOrder
	if err := svc.db.First(&d, delivery.ID).Error; err != nil {
		t.Fatal(err)
	}
	if d.Status != database.DeliveryStatusPreparing {
		t.Fatalf("delivery = %s, want preparing", d.Status)
	}
}

func TestReconcilePaidDeliveries_SkipsUnpaidConfirmed(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	bill, order, delivery := seedDeliveryTriple(t, svc, businessID)
	if _, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:1", database.DeliveryPaymentOnline); err != nil {
		t.Fatal(err)
	}
	// Unpaid — reconcile must not advance.
	_ = bill
	advanced, err := svc.ReconcilePaidDeliveries(50)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if advanced != 0 {
		t.Fatalf("advanced = %d, want 0 for unpaid", advanced)
	}
	var d database.DeliveryOrder
	svc.db.First(&d, delivery.ID)
	if d.Status != database.DeliveryStatusConfirmed {
		t.Fatalf("delivery = %s, want still confirmed", d.Status)
	}
}
