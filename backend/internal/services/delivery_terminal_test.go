package services

import (
	"fmt"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
)

var terminalDeliverySeq uint

func seedAssignedDelivery(t *testing.T, svc *DeliveryService, businessID uint) (database.DeliveryOrder, database.DeliveryDriver) {
	t.Helper()
	terminalDeliverySeq++
	seq := terminalDeliverySeq

	bill := database.Bill{
		BusinessID:  businessID,
		BillNumber:  fmt.Sprintf("B-T-%d", seq),
		Status:      database.BillStatusOpen,
		Subtotal:    1000,
		TotalAmount: 1700,
	}
	if err := svc.db.Omit("table_id").Create(&bill).Error; err != nil {
		t.Fatal(err)
	}

	driver := database.DeliveryDriver{BusinessID: businessID, Name: "D", Phone: "1", Status: database.DriverStatusOnline, IsAvailable: true, IsActive: true}
	if err := svc.db.Create(&driver).Error; err != nil {
		t.Fatal(err)
	}
	delivery := database.DeliveryOrder{
		BusinessID: businessID, BillID: bill.ID, DeliveryNumber: fmt.Sprintf("DEL-T%d", seq),
		DeliveryType: database.DeliveryTypeInHouse, Status: database.DeliveryStatusReady,
		CustomerName: "g", CustomerPhone: "1",
	}
	if err := svc.db.Create(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.AssignDriver(delivery.ID, driver.ID); err != nil {
		t.Fatal(err)
	}
	return delivery, driver
}

// R3: cancelling an assigned delivery used to end at status "ready" with
// cancelled_at set (UnassignDriver overwrote the terminal status).
func TestCancel_WithDriverStaysCancelled(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	delivery, driver := seedAssignedDelivery(t, svc, businessID)

	if err := svc.CancelDeliveryOrder(delivery.ID, "guest no-show", "staff:1"); err != nil {
		t.Fatal(err)
	}
	var d database.DeliveryOrder
	svc.db.First(&d, delivery.ID)
	if d.Status != database.DeliveryStatusCancelled {
		t.Fatalf("cancel resurrected to %s", d.Status)
	}
	var drv database.DeliveryDriver
	svc.db.First(&drv, driver.ID)
	if drv.Status != database.DriverStatusOnline || drv.CurrentDeliveryID != nil {
		t.Fatalf("driver not released: status=%s current=%v", drv.Status, drv.CurrentDeliveryID)
	}
}

// R4: delivered used to leave the driver busy forever.
func TestDelivered_ReleasesDriver(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	delivery, driver := seedAssignedDelivery(t, svc, businessID)

	if err := svc.UpdateDeliveryStatus(delivery.ID, database.DeliveryStatusDelivered, nil, "staff:1"); err != nil {
		t.Fatal(err)
	}
	var drv database.DeliveryDriver
	svc.db.First(&drv, driver.ID)
	if drv.Status != database.DriverStatusOnline || drv.CurrentDeliveryID != nil {
		t.Fatalf("driver not released on delivered: status=%s current=%v", drv.Status, drv.CurrentDeliveryID)
	}
	// Second assignment must now work.
	bill2 := database.Bill{BusinessID: businessID, BillNumber: "B-T-R4", Status: database.BillStatusOpen, Subtotal: 500, TotalAmount: 800}
	if err := svc.db.Omit("table_id").Create(&bill2).Error; err != nil {
		t.Fatal(err)
	}
	d2 := database.DeliveryOrder{BusinessID: businessID, BillID: bill2.ID, DeliveryNumber: "DEL-T-R4", DeliveryType: database.DeliveryTypeInHouse, Status: database.DeliveryStatusReady, CustomerName: "g", CustomerPhone: "1"}
	if err := svc.db.Create(&d2).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.AssignDriver(d2.ID, driver.ID); err != nil {
		t.Fatalf("driver should be assignable again: %v", err)
	}
}

// Carry-forward (Task 6 review): cancelling an already-cancelled delivery must
// be a quiet no-op — no re-emission, no duplicate emails, no state mutation.
func TestCancel_AlreadyCancelledIsQuietNoop(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	delivery, _ := seedAssignedDelivery(t, svc, businessID)

	// First cancel — sets reason and actor.
	if err := svc.CancelDeliveryOrder(delivery.ID, "first-reason", "staff:1"); err != nil {
		t.Fatal(err)
	}

	var before database.DeliveryOrder
	svc.db.First(&before, delivery.ID)
	if before.Status != database.DeliveryStatusCancelled {
		t.Fatalf("first cancel: got %s, want cancelled", before.Status)
	}
	if before.CancellationReason != "first-reason" {
		t.Fatalf("first cancel: reason = %q, want %q", before.CancellationReason, "first-reason")
	}

	// Second cancel — must return nil and must NOT overwrite the first cancellation data.
	if err := svc.CancelDeliveryOrder(delivery.ID, "second-reason", "staff:2"); err != nil {
		t.Fatalf("second cancel returned non-nil error: %v", err)
	}

	var after database.DeliveryOrder
	svc.db.First(&after, delivery.ID)
	if after.CancellationReason != "first-reason" {
		t.Fatalf("second cancel mutated cancellation_reason: got %q, want %q", after.CancellationReason, "first-reason")
	}
	if after.CancelledBy != before.CancelledBy {
		t.Fatalf("second cancel mutated cancelled_by: got %q, want %q", after.CancelledBy, before.CancelledBy)
	}
}
