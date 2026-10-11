package database

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/testperf"
)

// setupOrderRacePostgres points the package-global db at a throwaway Postgres
// container. SQLite (the default test DB) runs on a single connection and
// serializes every transaction, which masks the B-1 row-lock race — these
// tests are only meaningful on Postgres.
func setupOrderRacePostgres(t *testing.T) (*Business, *Table) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping Postgres container test in -short (repo container-test convention)")
	}
	ctx := context.Background()
	pg, err := testperf.StartPostgres(ctx)
	if err != nil {
		t.Fatalf("postgres container: %v (is Docker running?)", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	prev := db
	SetTestDB(pg.DB)
	t.Cleanup(func() { SetTestDB(prev) })

	require.NoError(t, db.AutoMigrate(
		&Business{},
		&InventorySettings{},
		&InventoryItem{},
		&InventoryRecipe{},
		&InventoryMovement{},
		&Menu{},
		&Table{},
		&Bill{},
		&BillHistoryEvent{},
		&Order{},
		&DeliveryOrder{},
		&DeliveryStatusHistory{},
	))
	require.NoError(t, db.AutoMigrate(&BillItem{}))

	biz := helperBusiness(t, 0, 0)
	tbl := &Table{BusinessID: biz.ID, Name: "T1", TableCode: "t1", IsActive: true}
	require.NoError(t, db.Create(tbl).Error)
	return biz, tbl
}

// helperBillForTable creates a bill attached to the given table (Postgres FK-safe).
func helperBillForTable(t *testing.T, business *Business, tbl *Table, subtotal float64) *Bill {
	t.Helper()
	subtotalCents := int64(math.Round(subtotal * 100))
	bill := &Bill{
		BusinessID:  business.ID,
		TableID:     tbl.ID,
		BillNumber:  "B-" + time.Now().Format("150405.000"),
		Status:      BillStatusOpen,
		Items:       "[]",
		Subtotal:    subtotalCents,
		TotalAmount: subtotalCents,
	}
	require.NoError(t, db.Create(bill).Error)
	return bill
}

// TestConcurrent_DoubleApproveSameOrder_Postgres reproduces B-1: with the
// GORM-v1 `gorm:query_option FOR UPDATE` no-op, N concurrent approves of the
// SAME order all read status=pending and all run the approve side effects —
// inventory deducted N times, bill items appended N times with regenerated
// UUIDs. The existing TestConcurrent_ApproveMultipleOrders approves DIFFERENT
// orders and never exercises this.
func TestConcurrent_DoubleApproveSameOrder_Postgres(t *testing.T) {
	biz, tbl := setupOrderRacePostgres(t)

	require.NoError(t, db.Create(&InventorySettings{
		BusinessID:                biz.ID,
		InventoryEnabled:          true,
		AutoDeductOnOrderApproval: true,
		AvailabilitySyncMode:      InventoryAvailabilityModeWarn,
	}).Error)
	stock := &InventoryItem{BusinessID: biz.ID, Name: "Patty", Unit: "pcs", CurrentQuantity: 10, IsActive: true}
	require.NoError(t, db.Create(stock).Error)
	require.NoError(t, db.Create(&InventoryRecipe{
		BusinessID:       biz.ID,
		MenuItemID:       "burger-id",
		MenuItemName:     "Burger",
		InventoryItemID:  stock.ID,
		QuantityRequired: 1,
	}).Error)

	bill := helperBillForTable(t, biz, tbl, 0)
	order := helperOrder(t, biz, bill, []OrderItem{
		{ID: "race-1", MenuItemID: "burger-id", MenuItemName: "Burger", Price: 12, Quantity: 2, Subtotal: 24},
	})

	const racers = 8
	start := make(chan struct{})
	errs := make([]error, racers)
	var wg sync.WaitGroup
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			errs[idx] = UpdateOrderStatus(order.ID, OrderStatusApproved, fmt.Sprintf("racer-%d", idx), "")
		}(i)
	}
	close(start)
	wg.Wait()

	// Every call must either succeed (first writer, or post-commit idempotent
	// no-op) or fail with the conflict sentinel handlers map to 409.
	successes := 0
	for i, err := range errs {
		if err == nil {
			successes++
			continue
		}
		assert.ErrorIs(t, err, ErrInvalidStatusTransition, "racer %d returned a non-conflict error: %v", i, err)
	}
	assert.GreaterOrEqual(t, successes, 1, "at least one approve must succeed")

	// Exactly ONE inventory deduction.
	var movements []InventoryMovement
	require.NoError(t, db.Where("reference_order_id = ?", order.ID).Find(&movements).Error)
	assert.Len(t, movements, 1, "inventory must be deducted exactly once")
	var refreshed InventoryItem
	require.NoError(t, db.First(&refreshed, stock.ID).Error)
	assert.Equal(t, 8.0, refreshed.CurrentQuantity, "stock 10 - (qty 2 x required 1), deducted once")

	// Exactly ONE set of bill items — JSON snapshot AND relational rows.
	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	var billItems []BillItem
	require.NoError(t, json.Unmarshal([]byte(savedBill.Items), &billItems))
	assert.Len(t, billItems, 1, "bill JSON snapshot must carry the order's item exactly once")
	var relCount int64
	require.NoError(t, db.Model(&BillItem{}).Where("order_id = ?", order.ID).Count(&relCount).Error)
	assert.Equal(t, int64(1), relCount, "relational bill_items must carry the order's item exactly once")
	assert.Equal(t, int64(2400), savedBill.Subtotal, "subtotal must count the order once (2400 cents)")
}

// TestConcurrent_GuestCancelVsApprove_Postgres reproduces the B-1 overwrite:
// a guest cancel that returned success to the guest must never be flipped to
// approved by a concurrent staff approve (pre-fix, approve's unconditional
// `UPDATE orders SET status='approved'` overwrites the committed cancel).
func TestConcurrent_GuestCancelVsApprove_Postgres(t *testing.T) {
	biz, tbl := setupOrderRacePostgres(t)

	for i := 0; i < 20; i++ {
		bill := helperBillForTable(t, biz, tbl, 0)
		order := helperOrder(t, biz, bill, []OrderItem{
			{ID: fmt.Sprintf("cvap-%d", i), MenuItemName: "Soup", Price: 7, Quantity: 1, Subtotal: 7},
		})

		var approveErr error
		var cancelWon bool
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			approveErr = UpdateOrderStatus(order.ID, OrderStatusApproved, "staff", "")
		}()
		go func() {
			defer wg.Done()
			<-start
			res := db.Model(&Order{}).
				Where("id = ? AND status = ?", order.ID, OrderStatusPending).
				Updates(map[string]interface{}{
					"status":       OrderStatusOrderCancelled,
					"cancelled_by": "guest",
					"updated_at":   time.Now(),
				})
			cancelWon = res.Error == nil && res.RowsAffected > 0
		}()
		close(start)
		wg.Wait()

		var final Order
		require.NoError(t, db.First(&final, order.ID).Error)
		if cancelWon {
			assert.Equal(t, OrderStatusOrderCancelled, final.Status,
				"iteration %d: guest was told cancelled — approve (err=%v) must not overwrite it", i, approveErr)
		} else {
			assert.Equal(t, OrderStatusApproved, final.Status, "iteration %d: cancel lost — approve must have landed", i)
			assert.NoError(t, approveErr, "iteration %d", i)
		}
	}
}

// TestConcurrent_CancelSiblingVsApprove_Postgres reproduces the bill-level lost
// update: two orders share one bill. Approving order A locks the bill, appends
// A's items and recomputes totals. Concurrently cancelling order B does a
// read-modify-write of the SAME bill's items/totals/status. Before the fix, the
// cancel read the bill unlocked, so if the approve committed A's items after the
// cancel took its snapshot, the cancel's blind write overwrote the bill without
// A's items — A is approved but the guest is never charged for it, and the JSON
// snapshot diverges from the relational bill_items. The bill FOR UPDATE lock on
// the cancel path serializes the two writers, so A's items always survive.
func TestConcurrent_CancelSiblingVsApprove_Postgres(t *testing.T) {
	biz, tbl := setupOrderRacePostgres(t)

	for i := 0; i < 20; i++ {
		bill := helperBillForTable(t, biz, tbl, 0)
		orderA := helperOrder(t, biz, bill, []OrderItem{
			{ID: fmt.Sprintf("A-%d", i), MenuItemName: "Steak", Price: 30, Quantity: 1, Subtotal: 30},
		})
		orderB := helperOrder(t, biz, bill, []OrderItem{
			{ID: fmt.Sprintf("B-%d", i), MenuItemName: "Salad", Price: 12, Quantity: 1, Subtotal: 12},
		})
		// Approve B first so its items live on the bill (the row the cancel will rewrite).
		require.NoError(t, UpdateOrderStatus(orderB.ID, OrderStatusApproved, "staff", ""))

		var approveErr, cancelErr error
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			approveErr = UpdateOrderStatus(orderA.ID, OrderStatusApproved, "staff", "")
		}()
		go func() {
			defer wg.Done()
			<-start
			cancelErr = UpdateOrderStatus(orderB.ID, OrderStatusOrderCancelled, "staff", "cancel B")
		}()
		close(start)
		wg.Wait()
		require.NoError(t, approveErr, "iteration %d: approve A", i)
		require.NoError(t, cancelErr, "iteration %d: cancel B", i)

		// Final state: A approved and its items charged, B cancelled and removed.
		var savedBill Bill
		require.NoError(t, db.First(&savedBill, bill.ID).Error)
		var billItems []BillItem
		require.NoError(t, json.Unmarshal([]byte(savedBill.Items), &billItems))

		var relItems []BillItem
		require.NoError(t, db.Where("bill_id = ?", bill.ID).Find(&relItems).Error)

		// A's item must survive on the bill; B's must be gone.
		assert.Len(t, billItems, 1, "iteration %d: only order A's item should remain on the bill JSON", i)
		assert.Equal(t, len(relItems), len(billItems),
			"iteration %d: JSON snapshot and relational bill_items must agree", i)
		if assert.Len(t, relItems, 1, "iteration %d: exactly A's relational row remains", i) {
			assert.Equal(t, orderA.ID, *relItems[0].OrderID, "iteration %d: surviving item belongs to order A", i)
		}
		assert.Equal(t, int64(3000), savedBill.Subtotal,
			"iteration %d: bill must charge A's 3000-cent item, not be clobbered to B-less-A", i)
	}
}
