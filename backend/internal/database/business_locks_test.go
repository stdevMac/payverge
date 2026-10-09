package database

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TestAdjustBillItem_UsesForUpdateLock asserts that the first SELECT emitted
// inside AdjustBillItem's transaction attaches `clause.Locking{Strength:
// "UPDATE"}` (i.e. FOR UPDATE) to the bill row read. Going FOR UPDATE is the
// only thing that serializes two concurrent edits; without it the later
// writer reads a stale subtotal snapshot and silently clobbers the other
// writer's recomputed totals. See ApplyConfirmedPayment for the same
// pattern on the payment side.
//
// SQLite's GORM dialector strips row-level locking at render time (SQLite
// does not support FOR UPDATE), so we verify the lock intent at the ORM
// layer by inspecting Statement.Clauses in a GORM Query callback. This
// still catches the regression the task targets: dropping the
// `clause.Locking` call on tx.First would leave the clause map empty.
func TestAdjustBillItem_UsesForUpdateLock(t *testing.T) {
	setupOrderTestDB(t)

	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, []BillItem{
		{ID: "line-fu", MenuItemID: "x", Name: "X", Price: 5, Quantity: 1, Subtotal: 5, ItemType: "menu_item"},
	}, 5)

	var firstBillQueryLocked atomic.Bool
	var firstBillQueryObserved atomic.Bool
	const cbName = "payverge:test:capture_bill_lock"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(cbName, func(tx *gorm.DB) {
		if firstBillQueryObserved.Load() {
			return
		}
		if tx.Statement == nil || tx.Statement.Schema == nil {
			return
		}
		if tx.Statement.Schema.Table != "bills" {
			return
		}
		firstBillQueryObserved.Store(true)
		if locking, ok := tx.Statement.Clauses["FOR"]; ok {
			if l, ok := locking.Expression.(clause.Locking); ok && l.Strength == "UPDATE" {
				firstBillQueryLocked.Store(true)
			}
		}
	}))
	defer func() {
		_ = db.Callback().Query().Remove(cbName)
	}()

	qty := 2
	_, _, err := AdjustBillItem(bill.ID, "line-fu", "staff", &qty, false, "")
	require.NoError(t, err)

	require.True(t, firstBillQueryObserved.Load(), "expected AdjustBillItem to issue a SELECT against bills")
	assert.True(t, firstBillQueryLocked.Load(),
		"AdjustBillItem must lock the bill row with clause.Locking{Strength: \"UPDATE\"} "+
			"so concurrent PUT /items/:id calls serialize instead of silently clobbering totals")
}

// TestAdjustBillItem_OnClosedBillReturnsSentinel ensures that once the lock
// is acquired, a non-open bill surfaces the ErrBillNotPayable sentinel (not
// an ad-hoc string). The handler maps the sentinel to a 409 and callers can
// errors.Is against it.
func TestAdjustBillItem_OnClosedBillReturnsSentinel(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, []BillItem{
		{ID: "line-lock-1", MenuItemID: "x", Name: "X", Price: 10, Quantity: 1, Subtotal: 10, ItemType: "menu_item"},
	}, 10)

	// Flip the bill to closed — AdjustBillItem must refuse to mutate it.
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", bill.ID).
		Update("status", BillStatusClosed).Error)

	qty := 2
	_, _, err := AdjustBillItem(bill.ID, "line-lock-1", "staff", &qty, false, "")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrBillNotPayable),
		"expected ErrBillNotPayable on closed bill, got %v", err)
}

// TestAdjustBillItem_SerializesConcurrentEdits is a functional regression
// against the silent-clobber bug. Two goroutines each increment a different
// line's quantity at the same time; whichever commits second must see the
// first writer's line subtotal — *not* a stale pre-edit snapshot. Without
// the FOR UPDATE guard the second writer's updateBillTx would overwrite
// the items JSON built from a stale read, losing the first update.
//
// SQLite in-memory serializes writes via a single-connection pool (see
// setupOrderTestDB), so on this harness the scenario resolves to sequential
// execution rather than a true race. The assertion still pins the expected
// end state so a future regression that drops the transaction entirely (or
// re-fetches outside the lock) would still fail here; the SQL-shape
// assertion in TestAdjustBillItem_UsesForUpdateLock is the stronger guard
// for the Postgres production path.
func TestAdjustBillItem_SerializesConcurrentEdits(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	// Use real UUIDs because updateBillTx normalizes any non-UUID id with
	// uuid.New(), which would rewrite our test ids and make the second
	// lookup fail before the lock-assertion ever kicks in.
	idA := "11111111-1111-1111-1111-111111111111"
	idB := "22222222-2222-2222-2222-222222222222"
	bill := helperBill(t, biz, []BillItem{
		{ID: idA, MenuItemID: "a", Name: "A", Price: 10, Quantity: 1, Subtotal: 10, ItemType: "menu_item"},
		{ID: idB, MenuItemID: "b", Name: "B", Price: 20, Quantity: 1, Subtotal: 20, ItemType: "menu_item"},
	}, 30)

	var wg sync.WaitGroup
	errs := make([]error, 2)

	wg.Add(2)
	go func() {
		defer wg.Done()
		q := 3
		_, _, errs[0] = AdjustBillItem(bill.ID, idA, "staff1", &q, false, "")
	}()
	go func() {
		defer wg.Done()
		q := 2
		_, _, errs[1] = AdjustBillItem(bill.ID, idB, "staff2", &q, false, "")
	}()
	wg.Wait()

	for i, err := range errs {
		require.NoError(t, err, "adjust %d should succeed", i)
	}

	var reloaded Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	// A: 10 * 3 = 30, B: 20 * 2 = 40  => subtotal 70. If either update was
	// lost to a stale-snapshot clobber, the subtotal would be 50 (only one
	// edit landed).
	assert.InDelta(t, 7000.0, reloaded.Subtotal, 0.01,
		"both concurrent edits must land; stale-snapshot clobber would leave subtotal at 50")
}

// TestCloseBillWithHistory_RefusesNonOpenBill guards the status-race fix.
// Before the fix CloseBillWithHistory would blindly UPDATE status='closed'
// even if a concurrent payment-completion had already transitioned the row
// to 'paid', silently reverting the paid state. Now the UPDATE is scoped to
// rows still 'open' and returns ErrBillNotOpen when RowsAffected is 0.
func TestCloseBillWithHistory_RefusesNonOpenBill(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)

	// Simulate the race: payment-completion has already flipped status to
	// paid between the handler's pre-check and this UPDATE.
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", bill.ID).
		Update("status", BillStatusPaid).Error)

	err := CloseBillWithHistory(bill.ID, nil)
	require.Error(t, err, "closing a non-open bill must not silently succeed")
	assert.True(t, errors.Is(err, ErrBillNotOpen),
		"expected ErrBillNotOpen sentinel so the handler can map to 409, got %v", err)

	// Critically: the paid status must still be paid, not silently reverted
	// to closed. That was the original bug.
	var reloaded Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	assert.Equal(t, BillStatusPaid, reloaded.Status,
		"paid status must survive a racing CloseBill; previously it was clobbered to closed")
}

// TestCloseBillWithHistory_OpenBillStillCloses is a happy-path regression so
// adding the WHERE status='open' guard did not break the common case.
func TestCloseBillWithHistory_OpenBillStillCloses(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)

	require.NoError(t, CloseBillWithHistory(bill.ID, nil))

	var reloaded Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	assert.Equal(t, BillStatusVoided, reloaded.Status)
	assert.NotNil(t, reloaded.ClosedAt)
}

func TestCloseBill_UnpaidBecomesAbandoned(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 37.82)

	err := CloseBill(bill.ID)
	require.ErrorIs(t, err, ErrBillHasUnpaidRemaining)

	var reloaded Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	assert.Equal(t, BillStatusOpen, reloaded.Status, "unpaid close is settle, not abandon")
	assert.NotEqual(t, BillStatusAbandoned, reloaded.Status)
}

func TestCloseBill_LiveKitchenRefused(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)
	require.NoError(t, db.Create(&Order{
		BillID:      bill.ID,
		BusinessID:  biz.ID,
		OrderNumber: "K-1",
		Status:      OrderStatusInKitchen,
		Items:       "[]",
	}).Error)

	err := CloseBill(bill.ID)
	require.ErrorIs(t, err, ErrBillHasLiveKitchenTickets)

	var reloaded Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	assert.Equal(t, BillStatusOpen, reloaded.Status)
}

// #704 bill 762: CloseBillWithHistory used to abandon an unpaid check while
// expo still held in_kitchen tickets, then the table flipped Available.
func TestCloseBill_UnpaidWithLiveKitchenRefused(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 37.82)
	require.NoError(t, db.Create(&Order{
		BillID:      bill.ID,
		BusinessID:  biz.ID,
		OrderNumber: "K-762",
		Status:      OrderStatusInKitchen,
		Items:       "[]",
	}).Error)

	err := CloseBill(bill.ID)
	require.ErrorIs(t, err, ErrBillHasLiveKitchenTickets)

	var reloaded Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	assert.Equal(t, BillStatusOpen, reloaded.Status, "unpaid check must stay open while food is in the pass")
	assert.NotEqual(t, BillStatusAbandoned, reloaded.Status)
}

// After expo bumps the ticket, leftover $37.82 is still a settle door — Close
// must not abandon the unpaid check as a side effect of "kitchen done."
func TestCloseBill_UnpaidAfterKitchenBumpRefusesSettle(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 37.82)
	require.NoError(t, db.Create(&Order{
		BillID:      bill.ID,
		BusinessID:  biz.ID,
		OrderNumber: "K-762-DONE",
		Status:      OrderStatusOrderDelivered,
		Items:       "[]",
	}).Error)

	err := CloseBill(bill.ID)
	require.ErrorIs(t, err, ErrBillHasUnpaidRemaining)
	require.NotErrorIs(t, err, ErrBillHasLiveKitchenTickets)

	var reloaded Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	assert.Equal(t, BillStatusOpen, reloaded.Status)
	assert.NotEqual(t, BillStatusAbandoned, reloaded.Status)
	assert.Nil(t, reloaded.AbandonedAt)
}

// Delivery-only abandon door: refuse while expo still owns the check.
func TestAbandonUnpaidOpenBill_RefusesLiveKitchen(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 18.04)
	require.NoError(t, db.Create(&Order{
		BillID:      bill.ID,
		BusinessID:  biz.ID,
		OrderNumber: "K-DELIVER-KITCHEN",
		Status:      OrderStatusInKitchen,
		Items:       "[]",
	}).Error)

	err := AbandonUnpaidOpenBill(bill.ID, "driver", "delivery cancelled unpaid")
	require.ErrorIs(t, err, ErrBillHasLiveKitchenTickets)

	var reloaded Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	assert.Equal(t, BillStatusOpen, reloaded.Status)
	assert.Nil(t, reloaded.AbandonedAt)
}

// Remaining guard: a $0 open check is not an unpaid walk-out.
func TestAbandonUnpaidOpenBill_RefusesZeroRemaining(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)

	err := AbandonUnpaidOpenBill(bill.ID, "driver", "delivery cancelled unpaid")
	require.ErrorIs(t, err, ErrBillHasUnpaidRemaining)

	var reloaded Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	assert.Equal(t, BillStatusOpen, reloaded.Status)
}

// Delivery walk-out still works when remaining is unpaid and kitchen is idle.
func TestAbandonUnpaidOpenBill_AbandonsUnpaidDelivery(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 22.00)
	require.NoError(t, db.Create(&Order{
		BillID:      bill.ID,
		BusinessID:  biz.ID,
		OrderNumber: "K-DELIVER-DONE",
		Status:      OrderStatusOrderCancelled,
		Items:       "[]",
	}).Error)

	require.NoError(t, AbandonUnpaidOpenBill(bill.ID, "driver", "delivery cancelled unpaid"))

	var reloaded Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	assert.Equal(t, BillStatusAbandoned, reloaded.Status)
	assert.NotNil(t, reloaded.AbandonedAt)
	assert.NotNil(t, reloaded.ClosedAt)
}

// #772: if a leftover is reopened without clearing closed_at (hourly demo
// re-arm used to do this), the next abandon must keep the original death time.
func TestAbandonUnpaidOpenBill_DoesNotRestampExistingClosedAt(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 37.82)
	require.NoError(t, db.Create(&Order{
		BillID:      bill.ID,
		BusinessID:  biz.ID,
		OrderNumber: "K-772-REARM",
		Status:      OrderStatusOrderCancelled,
		Items:       "[]",
	}).Error)

	require.NoError(t, AbandonUnpaidOpenBill(bill.ID, "system:expiry", "delivery cancelled unpaid"))

	diedAt := time.Date(2026, 8, 21, 16, 15, 50, 0, time.UTC)
	require.NoError(t, db.Model(&Bill{}).Where("id = ?", bill.ID).Updates(map[string]interface{}{
		"status":       BillStatusOpen,
		"closed_at":    diedAt,
		"abandoned_at": diedAt,
	}).Error)
	require.NoError(t, AbandonUnpaidOpenBill(bill.ID, "system:expiry", "delivery cancelled unpaid"))

	var second Bill
	require.NoError(t, db.First(&second, bill.ID).Error)
	require.Equal(t, BillStatusAbandoned, second.Status)
	require.NotNil(t, second.ClosedAt)
	require.NotNil(t, second.AbandonedAt)
	assert.Equal(t, diedAt.Unix(), second.ClosedAt.Unix(), "closed_at must stay the first expiry, not now()")
	assert.Equal(t, diedAt.Unix(), second.AbandonedAt.Unix(), "abandoned_at must stay the first expiry")
}
