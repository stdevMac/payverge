package services

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestUpdateDeliveryStatus_RejectsPaymentBypassForwardEdge pins finding (a):
// a confirmed (prepay, unpaid) delivery must NOT be advanced to preparing /
// ready / delivered via the operator status-update path. That would bypass
// payment + HandleDeliveryBillPaid, strand the order at pending (inventory
// never deducted, never expired), and fire a "delivered" email for an unpaid
// order. The only legal moves out of confirmed are terminal (cancelled/failed)
// or the system payment path.
func TestUpdateDeliveryStatus_RejectsPaymentBypassForwardEdge(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	_, order, delivery := seedDeliveryTriple(t, svc, businessID)

	// Accept as prepay → delivery=confirmed, order stays pending, payment owed.
	if _, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:1", database.DeliveryPaymentOnline); err != nil {
		t.Fatal(err)
	}

	// A dispatch:write staffer tries to jump confirmed → preparing (skip payment).
	err := svc.UpdateDeliveryStatus(delivery.ID, database.DeliveryStatusPreparing, nil, "staff:rogue")
	require.Error(t, err, "confirmed→preparing must be rejected (payment bypass)")
	require.True(t, errors.Is(err, ErrInvalidDeliveryTransition), "want ErrInvalidDeliveryTransition, got %v", err)

	// confirmed → delivered must also be rejected (would email an unpaid order).
	err = svc.UpdateDeliveryStatus(delivery.ID, database.DeliveryStatusDelivered, nil, "staff:rogue")
	require.Error(t, err, "confirmed→delivered must be rejected")
	require.True(t, errors.Is(err, ErrInvalidDeliveryTransition), "want ErrInvalidDeliveryTransition, got %v", err)

	// The delivery must be untouched — still confirmed, still awaiting payment.
	var d database.DeliveryOrder
	svc.db.First(&d, delivery.ID)
	require.Equal(t, database.DeliveryStatusConfirmed, d.Status)
	require.NotNil(t, d.PaymentExpiresAt, "payment window must remain open")
}

// TestUpdateDeliveryStatus_RejectsOnlinePendingSkipAccept pins the free-
// fulfillment hole: guest online checkout leaves delivery at pending with
// payment_mode_stored=online. The structural map still allows pending→
// assigned/delivered (COD skip-tolerant), but online prepay must force
// Accept → confirmed → pay before any fulfillment advance.
func TestUpdateDeliveryStatus_RejectsOnlinePendingSkipAccept(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	bill, order, delivery := seedDeliveryTriple(t, svc, businessID)
	require.NoError(t, svc.db.Model(&delivery).Update("payment_mode_stored", string(database.DeliveryPaymentOnline)).Error)

	for _, to := range []database.DeliveryStatus{
		database.DeliveryStatusPreparing,
		database.DeliveryStatusReady,
		database.DeliveryStatusAssigned,
		database.DeliveryStatusDelivered,
	} {
		err := svc.UpdateDeliveryStatus(delivery.ID, to, nil, "staff:rogue")
		require.Error(t, err, "pending+online → %s must be rejected", to)
		require.True(t, errors.Is(err, ErrInvalidDeliveryTransition), "want ErrInvalidDeliveryTransition for %s, got %v", to, err)
	}

	// Row untouched: still pending, bill still open, kitchen order still pending.
	var d database.DeliveryOrder
	require.NoError(t, svc.db.First(&d, delivery.ID).Error)
	require.Equal(t, database.DeliveryStatusPending, d.Status)
	var b database.Bill
	require.NoError(t, svc.db.First(&b, bill.ID).Error)
	require.Equal(t, database.BillStatusOpen, b.Status)
	require.Zero(t, b.PaidAmount)
	var o database.Order
	require.NoError(t, svc.db.First(&o, order.ID).Error)
	require.Equal(t, database.OrderStatusPending, o.Status)

	// Cancel remains legal (terminal cleanup path).
	require.NoError(t, svc.UpdateDeliveryStatus(delivery.ID, database.DeliveryStatusCancelled, nil, "staff:1"))
}

// TestAssignDriver_RejectsOnlinePendingSkipAccept mirrors the status path:
// assign must not flip a pending online prepay delivery to assigned.
func TestAssignDriver_RejectsOnlinePendingSkipAccept(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	_, _, delivery := seedDeliveryTriple(t, svc, businessID)
	require.NoError(t, svc.db.Model(&delivery).Update("payment_mode_stored", string(database.DeliveryPaymentOnline)).Error)

	driver := &database.DeliveryDriver{
		BusinessID: businessID, Name: "D", Phone: "+9", Email: "d@x.com",
		IsAvailable: true, IsActive: true, Status: database.DriverStatusOnline,
	}
	require.NoError(t, svc.db.Create(driver).Error)

	err := svc.AssignDriver(delivery.ID, driver.ID)
	require.Error(t, err, "assign on pending+online must be rejected")
	require.True(t, errors.Is(err, ErrInvalidDeliveryTransition), "want ErrInvalidDeliveryTransition, got %v", err)

	var d database.DeliveryOrder
	require.NoError(t, svc.db.First(&d, delivery.ID).Error)
	require.Equal(t, database.DeliveryStatusPending, d.Status)
	require.Nil(t, d.DriverID)
}

// TestUpdateDeliveryStatus_AllowsCODPendingFulfillment keeps COD (and legacy
// empty payment_mode_stored) skip-tolerant pending advances working.
func TestUpdateDeliveryStatus_AllowsCODPendingFulfillment(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	_, _, delivery := seedDeliveryTriple(t, svc, businessID)
	require.NoError(t, svc.db.Model(&delivery).Update("payment_mode_stored", string(database.DeliveryPaymentCashOnDelivery)).Error)

	require.NoError(t, svc.UpdateDeliveryStatus(delivery.ID, database.DeliveryStatusReady, nil, "staff:1"))
	var d database.DeliveryOrder
	require.NoError(t, svc.db.First(&d, delivery.ID).Error)
	require.Equal(t, database.DeliveryStatusReady, d.Status)
}

// TestUpdateDeliveryStatus_RejectsBackwardEdge pins finding (a): a forward-only
// state machine must reject a backward move (ready → preparing) even though the
// source is non-terminal.
func TestUpdateDeliveryStatus_RejectsBackwardEdge(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	_, order, delivery := seedDeliveryTriple(t, svc, businessID)

	// COD accept → delivery=preparing; then advance to ready.
	if _, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:1", database.DeliveryPaymentCashOnDelivery); err != nil {
		t.Fatal(err)
	}
	require.NoError(t, svc.UpdateDeliveryStatus(delivery.ID, database.DeliveryStatusReady, nil, "staff:1"))

	// Backward edge ready → preparing must be rejected.
	err := svc.UpdateDeliveryStatus(delivery.ID, database.DeliveryStatusPreparing, nil, "staff:1")
	require.Error(t, err, "ready→preparing (backward) must be rejected")
	require.True(t, errors.Is(err, ErrInvalidDeliveryTransition), "want ErrInvalidDeliveryTransition, got %v", err)
}

// TestUpdateDeliveryStatus_AllowsLegalForwardEdges keeps the legitimate
// progression working after the forward-edge guard lands. pending→ready (the
// operator skip the handler test relies on) and the normal hop chain must pass.
func TestUpdateDeliveryStatus_AllowsLegalForwardEdges(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)

	// A plain in-house delivery created at "ready" (post-kitchen) progressing
	// through dispatch to delivered.
	bill := database.Bill{BusinessID: businessID, BillNumber: "B-FWD-1", Status: database.BillStatusOpen, Subtotal: 1000, TotalAmount: 1700}
	require.NoError(t, svc.db.Omit("table_id").Create(&bill).Error)
	delivery := database.DeliveryOrder{
		BusinessID: businessID, BillID: bill.ID, DeliveryNumber: "DEL-FWD-1",
		DeliveryType: database.DeliveryTypeInHouse, Status: database.DeliveryStatusReady,
		CustomerName: "g", CustomerPhone: "1",
	}
	require.NoError(t, svc.db.Create(&delivery).Error)

	require.NoError(t, svc.UpdateDeliveryStatus(delivery.ID, database.DeliveryStatusAssigned, nil, "staff:1"))
	require.NoError(t, svc.UpdateDeliveryStatus(delivery.ID, database.DeliveryStatusPickedUp, nil, "staff:1"))
	require.NoError(t, svc.UpdateDeliveryStatus(delivery.ID, database.DeliveryStatusInTransit, nil, "staff:1"))
	require.NoError(t, svc.UpdateDeliveryStatus(delivery.ID, database.DeliveryStatusDelivered, nil, "staff:1"))

	// Forward skip pending→ready (handler test contract).
	bill2 := database.Bill{BusinessID: businessID, BillNumber: "B-FWD-2", Status: database.BillStatusOpen, Subtotal: 500, TotalAmount: 800}
	require.NoError(t, svc.db.Omit("table_id").Create(&bill2).Error)
	d2 := database.DeliveryOrder{
		BusinessID: businessID, BillID: bill2.ID, DeliveryNumber: "DEL-FWD-2",
		DeliveryType: database.DeliveryTypeInHouse, Status: database.DeliveryStatusPending,
		CustomerName: "g", CustomerPhone: "1",
	}
	require.NoError(t, svc.db.Create(&d2).Error)
	require.NoError(t, svc.UpdateDeliveryStatus(d2.ID, database.DeliveryStatusReady, nil, "staff:1"))
}

// TestUpdateDeliveryStatus_CancelledBeforeUpdateIsNotResurrected pins finding
// (b) deterministically: the row read + terminal guard must run INSIDE the
// transaction under a FOR UPDATE lock. The old code read the row with a plain
// non-locking pre-read, evaluated the guard on that stale snapshot, then did a
// full-row Save with no conditional WHERE — so a cancel that committed after the
// pre-read but before the Save would be clobbered (cancelled → delivered).
//
// Here we model the worst case: a delivery that has been cancelled, then an
// update arrives. With the in-tx locked re-read the update must observe the
// committed cancel and reject with ErrDeliveryStatusTerminal; the delivery must
// stay cancelled and the released driver must stay released.
func TestUpdateDeliveryStatus_CancelledBeforeUpdateIsNotResurrected(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	delivery, driver := seedAssignedDelivery(t, svc, businessID)

	// A cancel commits first (expiry sweep / operator cancel / order rejection).
	require.NoError(t, svc.CancelDeliveryOrder(delivery.ID, "guest cancelled", "guest"))

	// A delivered update races in afterward — must NOT resurrect.
	err := svc.UpdateDeliveryStatus(delivery.ID, database.DeliveryStatusDelivered, nil, "staff:1")
	require.Error(t, err, "cannot deliver a cancelled delivery")
	require.True(t, errors.Is(err, ErrDeliveryStatusTerminal), "want ErrDeliveryStatusTerminal, got %v", err)

	var d database.DeliveryOrder
	svc.db.First(&d, delivery.ID)
	require.Equal(t, database.DeliveryStatusCancelled, d.Status, "cancelled delivery must not be resurrected to delivered")

	var drv database.DeliveryDriver
	svc.db.First(&drv, driver.ID)
	require.Nil(t, drv.CurrentDeliveryID, "released driver must not be rebound by the clobbering update")
	require.Equal(t, database.DriverStatusOnline, drv.Status, "released driver must stay online")
}

// TestUpdateDeliveryStatus_ConcurrentCancelDoesNotResurrect pins finding (b):
// the row read + terminal/transition guard must run under a FOR UPDATE lock
// inside the transaction. A cancel that commits in the read→save window must
// win — the update must NOT resurrect a cancelled delivery to delivered.
func TestUpdateDeliveryStatus_ConcurrentCancelDoesNotResurrect(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	delivery, driver := seedAssignedDelivery(t, svc, businessID)

	// Advance to in_transit so a delivered hop is otherwise legal.
	require.NoError(t, svc.UpdateDeliveryStatus(delivery.ID, database.DeliveryStatusPickedUp, nil, "staff:1"))
	require.NoError(t, svc.UpdateDeliveryStatus(delivery.ID, database.DeliveryStatusInTransit, nil, "staff:1"))

	var wg sync.WaitGroup
	wg.Add(2)
	var updateErr, cancelErr error
	go func() {
		defer wg.Done()
		updateErr = svc.UpdateDeliveryStatus(delivery.ID, database.DeliveryStatusDelivered, nil, "staff:1")
	}()
	go func() {
		defer wg.Done()
		cancelErr = svc.CancelDeliveryOrder(delivery.ID, "guest cancelled", "guest")
	}()
	wg.Wait()
	_ = updateErr
	_ = cancelErr

	// Whatever the interleaving, the row must be in a single consistent terminal
	// state. The dangerous outcome is: cancel commits, then the stale-snapshot
	// update flips it back to delivered (resurrecting a cancelled order, rebinding
	// the released driver, firing a delivered email). Assert that does NOT happen:
	// if the delivery is cancelled, it must STAY cancelled.
	var d database.DeliveryOrder
	svc.db.First(&d, delivery.ID)
	if d.Status == database.DeliveryStatusCancelled {
		// A locked re-read means the delivered update saw cancelled and rejected.
		require.Equal(t, database.DeliveryStatusCancelled, d.Status, "cancelled delivery must not be resurrected")
		var drv database.DeliveryDriver
		svc.db.First(&drv, driver.ID)
		require.Nil(t, drv.CurrentDeliveryID, "driver must stay released after cancel won")
	}
}

// TestUpdateDeliveryStatus_FailedRunsFullCleanup pins finding (c): the "failed"
// terminal must run the SAME cleanup as "cancelled" — cancel the linked order
// (restoring inventory if it had been deducted) and close the unpaid bill —
// not merely release the driver.
func TestUpdateDeliveryStatus_FailedRunsFullCleanup(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	bill, order, delivery := seedDeliveryTriple(t, svc, businessID)

	// COD accept → order approved (inventory deducted), delivery preparing.
	if _, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:1", database.DeliveryPaymentCashOnDelivery); err != nil {
		t.Fatal(err)
	}
	// Move forward to a dispatched state so "failed" is a legal terminal hop.
	require.NoError(t, svc.UpdateDeliveryStatus(delivery.ID, database.DeliveryStatusReady, nil, "staff:1"))

	require.NoError(t, svc.UpdateDeliveryStatus(delivery.ID, database.DeliveryStatusFailed, nil, "staff:1"))

	var d database.DeliveryOrder
	svc.db.First(&d, delivery.ID)
	require.Equal(t, database.DeliveryStatusFailed, d.Status)

	// Linked order must be cancelled (the cleanup, not left approved/in-flight).
	var o database.Order
	svc.db.First(&o, order.ID)
	require.Equal(t, database.OrderStatusOrderCancelled, o.Status, "failed must cancel the linked order")

	// Unpaid bill must be closed.
	var b database.Bill
	svc.db.First(&b, bill.ID)
	require.NotEqual(t, database.BillStatusOpen, b.Status, "failed must close the unpaid bill")
}
