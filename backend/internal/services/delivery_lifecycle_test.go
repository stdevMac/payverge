package services

import (
	"errors"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func seedDeliveryTriple(t *testing.T, svc *DeliveryService, businessID uint) (database.Bill, database.Order, database.DeliveryOrder) {
	t.Helper()
	bill := database.Bill{BusinessID: businessID, BillNumber: "B-L-1", Status: database.BillStatusOpen, Subtotal: 1000, TotalAmount: 1700}
	if err := svc.db.Omit("table_id").Create(&bill).Error; err != nil {
		t.Fatal(err)
	}
	order := database.Order{BillID: bill.ID, BusinessID: businessID, OrderNumber: "O-L-1", Status: database.OrderStatusPending, CreatedBy: "guest_delivery", Items: "[]"}
	if err := svc.db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	delivery := database.DeliveryOrder{
		BusinessID: businessID, BillID: bill.ID, OrderID: &order.ID,
		DeliveryNumber: "DEL-L1", DeliveryType: database.DeliveryTypeInHouse,
		Status: database.DeliveryStatusPending, CustomerName: "g", CustomerPhone: "1",
	}
	if err := svc.db.Create(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	return bill, order, delivery
}

func TestAccept_PrepayHoldsKitchenAndSetsExpiry(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	_, order, delivery := seedDeliveryTriple(t, svc, businessID)

	res, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:1", database.DeliveryPaymentOnline)
	if err != nil {
		t.Fatal(err)
	}
	var d database.DeliveryOrder
	svc.db.First(&d, delivery.ID)
	if d.Status != database.DeliveryStatusConfirmed {
		t.Fatalf("delivery = %s, want confirmed", d.Status)
	}
	if d.PaymentExpiresAt == nil || time.Until(*d.PaymentExpiresAt) > 16*time.Minute || time.Until(*d.PaymentExpiresAt) < 14*time.Minute {
		t.Fatalf("payment_expires_at not ~15min out: %v", d.PaymentExpiresAt)
	}
	var o database.Order
	svc.db.First(&o, order.ID)
	if o.Status != database.OrderStatusPending {
		t.Fatalf("prepay accept must NOT start the kitchen; order = %s", o.Status)
	}
	if res.AwaitingPayment != true {
		t.Fatal("result must flag awaiting payment")
	}
}

func TestAccept_CODStartsKitchen(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	_, order, delivery := seedDeliveryTriple(t, svc, businessID)

	res, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:1", database.DeliveryPaymentCashOnDelivery)
	if err != nil {
		t.Fatal(err)
	}
	var d database.DeliveryOrder
	svc.db.First(&d, delivery.ID)
	if d.Status != database.DeliveryStatusPreparing {
		t.Fatalf("delivery = %s, want preparing", d.Status)
	}
	if d.PaymentExpiresAt != nil {
		t.Fatal("COD must not set a payment expiry")
	}
	var o database.Order
	svc.db.First(&o, order.ID)
	if o.Status != database.OrderStatusApproved {
		t.Fatalf("COD accept starts the kitchen; order = %s", o.Status)
	}
	if res.AwaitingPayment {
		t.Fatal("COD is never awaiting payment")
	}
}

func TestReject_CancelsAllThreeLegsAndClosesBill(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	bill, order, delivery := seedDeliveryTriple(t, svc, businessID)

	if err := svc.RejectDeliveryByOrder(businessID, order.ID, "staff:1", "out of wine"); err != nil {
		t.Fatal(err)
	}
	var d database.DeliveryOrder
	svc.db.First(&d, delivery.ID)
	if d.Status != database.DeliveryStatusCancelled || d.CancellationReason != "out of wine" {
		t.Fatalf("delivery: %s / %q", d.Status, d.CancellationReason)
	}
	var o database.Order
	svc.db.First(&o, order.ID)
	if o.Status != database.OrderStatusOrderCancelled {
		t.Fatalf("order = %s, want cancelled", o.Status)
	}
	var b database.Bill
	svc.db.First(&b, bill.ID)
	if b.Status == database.BillStatusOpen {
		t.Fatalf("bill must not stay open after reject; got %s", b.Status)
	}
}

// TestAccept_NonPendingRejected: accepting a delivery that is no longer pending
// must return an error, and the second call must not change any state.
func TestAccept_NonPendingRejected(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	_, order, delivery := seedDeliveryTriple(t, svc, businessID)

	// First accept (COD) moves delivery → preparing.
	if _, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:1", database.DeliveryPaymentCashOnDelivery); err != nil {
		t.Fatal(err)
	}

	// Second accept must fail — delivery is no longer pending.
	_, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:1", database.DeliveryPaymentCashOnDelivery)
	if err == nil {
		t.Fatal("expected an error on second accept of a non-pending delivery, got nil")
	}

	// State must be unchanged from the first accept.
	var d database.DeliveryOrder
	svc.db.First(&d, delivery.ID)
	if d.Status != database.DeliveryStatusPreparing {
		t.Fatalf("delivery status changed on second accept: got %s, want preparing", d.Status)
	}
}

// TestAccept_MissingDeliveryReturnsSentinel: accepting an orderID with no
// linked delivery row must return ErrDeliveryOrderNotFound.
func TestAccept_MissingDeliveryReturnsSentinel(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)

	// Use an order ID that has never had a delivery row.
	const nonExistentOrderID = 999999
	_, err := svc.AcceptDeliveryByOrder(businessID, nonExistentOrderID, "staff:1", database.DeliveryPaymentCashOnDelivery)
	if !errors.Is(err, ErrDeliveryOrderNotFound) {
		t.Fatalf("expected ErrDeliveryOrderNotFound, got %v", err)
	}
}

// TestReconcile_PrepayingWedgedOrderHealed: seeds a COD accept that wedges
// (delivery=preparing, order=pending), then verifies a second AcceptDeliveryByOrder
// call heals the seam — order becomes approved and the call returns success.
func TestReconcile_PrepayingWedgedOrderHealed(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	_, order, delivery := seedDeliveryTriple(t, svc, businessID)

	// First accept (COD) moves delivery → preparing.
	if _, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:1", database.DeliveryPaymentCashOnDelivery); err != nil {
		t.Fatal(err)
	}
	// Verify delivery is preparing and order is approved after normal accept.
	var d database.DeliveryOrder
	svc.db.First(&d, delivery.ID)
	if d.Status != database.DeliveryStatusPreparing {
		t.Fatalf("setup: delivery = %s, want preparing", d.Status)
	}
	var o database.Order
	svc.db.First(&o, order.ID)
	if o.Status != database.OrderStatusApproved {
		t.Fatalf("setup: order = %s, want approved", o.Status)
	}

	// Simulate the wedge: reset order back to pending (as if ApproveDeliveryLinkedOrder
	// had failed during the first accept).
	if err := svc.db.Model(&database.Order{}).Where("id = ?", order.ID).Update("status", database.OrderStatusPending).Error; err != nil {
		t.Fatal(err)
	}

	// Second AcceptDeliveryByOrder call — must reconcile the wedge (not error).
	res, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:1", database.DeliveryPaymentCashOnDelivery)
	if err != nil {
		t.Fatalf("reconcile accept returned error: %v", err)
	}
	if res.AwaitingPayment {
		t.Fatal("reconciled COD delivery must not flag awaiting payment")
	}

	// Delivery must still be preparing (unchanged).
	svc.db.First(&d, delivery.ID)
	if d.Status != database.DeliveryStatusPreparing {
		t.Fatalf("reconcile: delivery = %s, want preparing", d.Status)
	}

	// Order must now be approved.
	svc.db.First(&o, order.ID)
	if o.Status != database.OrderStatusApproved {
		t.Fatalf("reconcile: order = %s, want approved", o.Status)
	}
}

// TestReconcile_CODAcceptedEmailSent: when a COD delivery is in the wedge state
// (preparing + pending order), the reconcile-only path must send the guest the
// "Order accepted" email just as a normal COD accept does.
func TestReconcile_CODAcceptedEmailSent(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	emailMock := &mockNotificationDispatcher{}
	mgr := newTestNotificationManager(emailMock)
	svc := NewDeliveryService(db, mgr)
	business := createTestHospitalityBusiness(t, db, t.Name())
	businessID := business.ID

	// Seed a delivery with a customer email so the notification path fires.
	bill := database.Bill{BusinessID: businessID, BillNumber: "B-RC-1", Status: database.BillStatusOpen, Subtotal: 1000, TotalAmount: 1700}
	if err := svc.db.Omit("table_id").Create(&bill).Error; err != nil {
		t.Fatal(err)
	}
	order := database.Order{BillID: bill.ID, BusinessID: businessID, OrderNumber: "O-RC-1", Status: database.OrderStatusPending, CreatedBy: "guest_delivery", Items: "[]"}
	if err := svc.db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	delivery := database.DeliveryOrder{
		BusinessID: businessID, BillID: bill.ID, OrderID: &order.ID,
		DeliveryNumber: "DEL-RC1", DeliveryType: database.DeliveryTypeInHouse,
		Status: database.DeliveryStatusPending, CustomerName: "rcguest", CustomerPhone: "1",
		CustomerEmail: "rcguest@example.com",
	}
	if err := svc.db.Create(&delivery).Error; err != nil {
		t.Fatal(err)
	}

	// First COD accept — moves delivery → preparing, order → approved.
	if _, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:1", database.DeliveryPaymentCashOnDelivery); err != nil {
		t.Fatal(err)
	}
	emailsBefore := len(emailMock.notifications)
	if emailsBefore == 0 {
		t.Fatal("first COD accept should have sent an email")
	}

	// Simulate wedge: reset order back to pending.
	if err := svc.db.Model(&database.Order{}).Where("id = ?", order.ID).Update("status", database.OrderStatusPending).Error; err != nil {
		t.Fatal(err)
	}

	// Reconcile accept — must send another accepted email.
	if _, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:1", database.DeliveryPaymentCashOnDelivery); err != nil {
		t.Fatalf("reconcile accept failed: %v", err)
	}

	if len(emailMock.notifications) <= emailsBefore {
		t.Fatal("reconcile-only COD accept did not send the guest 'Order accepted' email")
	}
}

// TestReconcile_ConfirmedDeliverySecondAcceptErrors: a confirmed delivery (prepay,
// payment still pending) must NOT be reconciled by AcceptDeliveryByOrder —
// the call must return an error, and state must be unchanged.
func TestReconcile_ConfirmedDeliverySecondAcceptErrors(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	_, order, delivery := seedDeliveryTriple(t, svc, businessID)

	// Accept prepay — delivery → confirmed, order stays pending.
	if _, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:1", database.DeliveryPaymentOnline); err != nil {
		t.Fatal(err)
	}

	// Second accept must still error (confirmed ≠ preparing, no reconcile applies).
	_, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:2", database.DeliveryPaymentOnline)
	if err == nil {
		t.Fatal("second accept on confirmed delivery must return an error")
	}

	// State must be unchanged.
	var d database.DeliveryOrder
	svc.db.First(&d, delivery.ID)
	if d.Status != database.DeliveryStatusConfirmed {
		t.Fatalf("second accept changed delivery status to %s, want confirmed", d.Status)
	}
}
