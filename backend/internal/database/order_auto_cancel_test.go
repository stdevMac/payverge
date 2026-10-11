package database

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAutoCancelDB(t *testing.T) {
	t.Helper()
	setupOrderTestDB(t)
	require.NoError(t, db.AutoMigrate(&OperationalAlert{}, &OperationalAlertEvent{}))
}

func seedPendingOrderWithAlert(t *testing.T, business *Business, bill *Bill) (*Order, *OperationalAlert) {
	t.Helper()
	order := helperOrder(t, business, bill, []OrderItem{{
		ID: "line-1", MenuItemName: "Burger", Quantity: 1, Price: 10, Subtotal: 10,
	}})
	alert := &OperationalAlert{
		BusinessID:   business.ID,
		AlertType:    OperationalAlertTypeOrderNew,
		ResourceType: OperationalAlertResourceTypeOrder,
		ResourceID:   int64(order.ID),
		Status:       OperationalAlertStatusOpen,
		Priority:     OperationalAlertPriorityUrgent,
		Title:        "New order",
	}
	require.NoError(t, db.Create(alert).Error)
	return order, alert
}

func assertAutoCancelled(t *testing.T, orderID uint, alertID uint, billID uint) {
	t.Helper()
	var order Order
	require.NoError(t, db.First(&order, orderID).Error)
	assert.Equal(t, OrderStatusOrderCancelled, order.Status)
	assert.Equal(t, CancelReasonBillClosed, order.CancelReason)
	assert.NotEmpty(t, order.CancelledBy)
	require.NotNil(t, order.CancelledAt)

	var events []BillHistoryEvent
	require.NoError(t, db.Where("bill_id = ? AND event_type = ?", billID, BillHistoryEventOrderCanceled).Find(&events).Error)
	require.Len(t, events, 1)
	assert.Equal(t, CancelReasonBillClosed, events[0].Reason)

	var alert OperationalAlert
	require.NoError(t, db.First(&alert, alertID).Error)
	assert.Equal(t, OperationalAlertStatusResolved, alert.Status)
	require.NotNil(t, alert.ResolvedAt)
}

func TestCloseBillWithHistory_AutoCancelsPendingOrders(t *testing.T) {
	setupAutoCancelDB(t)
	business := helperBusiness(t, 0, 0)
	bill := helperBill(t, business, nil, 0)
	order, alert := seedPendingOrderWithAlert(t, business, bill)

	require.NoError(t, CloseBill(bill.ID))
	assertAutoCancelled(t, order.ID, alert.ID, bill.ID)

	err := UpdateOrderStatus(order.ID, OrderStatusApproved, "staff:1", "")
	require.Error(t, err)
}

func TestVoidBill_AutoCancelsPendingOrders(t *testing.T) {
	setupAutoCancelDB(t)
	business := helperBusiness(t, 0, 0)
	bill := helperBill(t, business, nil, 0)
	order, alert := seedPendingOrderWithAlert(t, business, bill)

	_, err := VoidBill(bill.ID, "owner:0x1", "test void")
	require.NoError(t, err)
	assertAutoCancelled(t, order.ID, alert.ID, bill.ID)
}

// TestResolveOrderAlerts_BatchesUpdatesAndInserts asserts that closing a bill
// with M=4 pending-order alerts issues exactly ONE UPDATE to operational_alerts
// and ONE INSERT to operational_alert_events (batch) regardless of M, and that
// all 4 alerts are resolved with ResolvedAt set and all 4 event rows exist.
//
// This test is the access-shape regression guard for N1-03.
func TestResolveOrderAlerts_BatchesUpdatesAndInserts(t *testing.T) {
	setupAutoCancelDB(t)
	business := helperBusiness(t, 0, 0)
	bill := helperBill(t, business, nil, 0)

	const M = 4
	alertIDs := make([]uint, M)
	for i := 0; i < M; i++ {
		_, alert := seedPendingOrderWithAlert(t, business, bill)
		alertIDs[i] = alert.ID
	}

	var alertUpdates atomic.Int64
	var eventInserts atomic.Int64

	const cbUpdate = "payverge:test:n1_03_alert_update"
	const cbInsert = "payverge:test:n1_03_event_insert"

	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(cbUpdate, func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Schema == nil {
			return
		}
		if tx.Statement.Schema.Table == "operational_alerts" {
			alertUpdates.Add(1)
		}
	}))
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(cbInsert, func(tx *gorm.DB) {
		if tx.Statement == nil || tx.Statement.Schema == nil {
			return
		}
		if tx.Statement.Schema.Table == "operational_alert_events" {
			eventInserts.Add(1)
		}
	}))
	t.Cleanup(func() {
		_ = db.Callback().Update().Remove(cbUpdate)
		_ = db.Callback().Create().Remove(cbInsert)
	})

	require.NoError(t, CloseBill(bill.ID))

	gotUpdates := alertUpdates.Load()
	gotInserts := eventInserts.Load()

	assert.Equal(t, int64(1), gotUpdates,
		"expected exactly 1 batched UPDATE to operational_alerts, got %d (N+1 regression if >1)", gotUpdates)
	assert.Equal(t, int64(1), gotInserts,
		"expected exactly 1 batched INSERT to operational_alert_events, got %d (N+1 regression if >1)", gotInserts)

	// Correctness: all alerts resolved with ResolvedAt set.
	for _, id := range alertIDs {
		var a OperationalAlert
		require.NoError(t, db.First(&a, id).Error)
		assert.Equal(t, OperationalAlertStatusResolved, a.Status,
			"alert %d should be resolved", id)
		assert.NotNil(t, a.ResolvedAt, "alert %d should have ResolvedAt set", id)
	}

	// Correctness: exactly M event rows were written by the batch insert.
	var evCount int64
	require.NoError(t, db.Model(&OperationalAlertEvent{}).
		Where("alert_id IN ?", alertIDs).Count(&evCount).Error)
	assert.Equal(t, int64(M), evCount,
		"expected %d OperationalAlertEvent rows after batch insert, got %d", M, evCount)
}

// BenchmarkResolveOrderAlerts measures the CloseBill path over a bill with
// M=4 pending orders each carrying an open operational alert.  Run this
// BEFORE editing resolveOrderAlertsTx to capture the N+1 baseline, then
// again after the batch fix for the after numbers.
//
// Each iteration uses a fresh in-memory SQLite DB (unique name) so rows from
// prior iterations don't collide on the Business.BusinessId unique constraint.
func BenchmarkResolveOrderAlerts(b *testing.B) {
	const M = 4

	benchSetupDB := func(b *testing.B, name string) {
		b.Helper()
		gormDB, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
		if err != nil {
			b.Fatalf("open sqlite: %v", err)
		}
		sqlDB, err := gormDB.DB()
		if err != nil {
			b.Fatalf("sql db: %v", err)
		}
		sqlDB.SetMaxOpenConns(1)
		b.Cleanup(func() { _ = sqlDB.Close() })

		db = gormDB
		if err := db.AutoMigrate(
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
			&OperationalAlert{},
			&OperationalAlertEvent{},
		); err != nil {
			b.Fatalf("auto-migrate: %v", err)
		}
		db.Exec("DROP TABLE IF EXISTS bill_items")
		if err := db.Exec(`CREATE TABLE bill_items (
			id TEXT PRIMARY KEY, bill_id INTEGER NOT NULL, menu_item_id TEXT DEFAULT '',
			name TEXT NOT NULL, price REAL NOT NULL, quantity INTEGER NOT NULL,
			options TEXT, item_type TEXT DEFAULT 'menu_item', bundle_id INTEGER,
			parent_bundle_id INTEGER, source_offer_id INTEGER, order_id INTEGER,
			subtotal REAL NOT NULL, created_at DATETIME
		)`).Error; err != nil {
			b.Fatalf("create bill_items: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()

		// Fresh DB per iteration avoids unique-constraint collision on BusinessId.
		benchSetupDB(b, fmt.Sprintf("autocancel-bench-%d", i))

		biz := &Business{Name: "BenchBiz", BusinessId: fmt.Sprintf("biz-%d", i)}
		if err := db.Create(biz).Error; err != nil {
			b.Fatalf("create business: %v", err)
		}
		bill := &Bill{
			BusinessID:  biz.ID,
			BillNumber:  "B-bench",
			Status:      BillStatusOpen,
			Items:       "[]",
			Subtotal:    0,
			TotalAmount: 0,
		}
		if err := db.Create(bill).Error; err != nil {
			b.Fatalf("create bill: %v", err)
		}
		for j := 0; j < M; j++ {
			order := &Order{
				BillID:      bill.ID,
				BusinessID:  biz.ID,
				OrderNumber: "O-bench",
				Status:      OrderStatusPending,
				CreatedBy:   "guest",
				Items:       `[{"id":"l1","menu_item_name":"Burger","quantity":1,"price":10,"subtotal":10}]`,
			}
			if err := db.Create(order).Error; err != nil {
				b.Fatalf("create order: %v", err)
			}
			alert := &OperationalAlert{
				BusinessID:   biz.ID,
				AlertType:    OperationalAlertTypeOrderNew,
				ResourceType: OperationalAlertResourceTypeOrder,
				ResourceID:   int64(order.ID),
				Status:       OperationalAlertStatusOpen,
				Priority:     OperationalAlertPriorityUrgent,
				Title:        "New order",
			}
			if err := db.Create(alert).Error; err != nil {
				b.Fatalf("create alert: %v", err)
			}
		}

		b.StartTimer()

		if err := CloseBill(bill.ID); err != nil {
			b.Fatalf("CloseBill: %v", err)
		}
	}
}

// Fix 4: batch-resolving order alerts on bill close must hand the resolved
// rows to the registered publisher so the operational_alerts package can emit
// alert.resolved SSE frames — otherwise the urgent repeating alarm keeps
// sounding until the next dashboard poll. (The database package cannot import
// internal/events — events imports database — so the publish is a hook set by
// operational_alerts; the end-to-end hub assertion lives in that package's
// tests.)
func TestCloseBillInvokesAlertResolvedPublisher(t *testing.T) {
	setupAutoCancelDB(t)
	business := helperBusiness(t, 0, 0)
	bill := helperBill(t, business, nil, 0)
	_, alert := seedPendingOrderWithAlert(t, business, bill)

	var captured []OperationalAlert
	SetOperationalAlertResolvedPublisher(func(a OperationalAlert) {
		captured = append(captured, a)
	})
	t.Cleanup(func() { SetOperationalAlertResolvedPublisher(nil) })

	require.NoError(t, CloseBill(bill.ID))

	require.Len(t, captured, 1)
	require.Equal(t, alert.ID, captured[0].ID)
	require.Equal(t, business.ID, captured[0].BusinessID)
	require.Equal(t, OperationalAlertStatusResolved, captured[0].Status)
	require.NotNil(t, captured[0].ResolvedAt)
}
