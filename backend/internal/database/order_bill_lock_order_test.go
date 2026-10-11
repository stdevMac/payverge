package database

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// lockSequence records, in order, each row-locking SELECT and each UPDATE of
// orders, as "lock:<table>" / "update:<table>". SQLite drops FOR UPDATE at
// render time, so the intent is read off Statement.Clauses.
func lockSequence(t *testing.T) func() []string {
	t.Helper()
	var mu sync.Mutex
	var seq []string
	lockedTable := func(tx *gorm.DB) (string, bool) {
		if tx.Statement == nil || tx.Statement.Schema == nil {
			return "", false
		}
		locking, ok := tx.Statement.Clauses["FOR"]
		if !ok {
			return "", false
		}
		if l, ok := locking.Expression.(clause.Locking); !ok || l.Strength != "UPDATE" {
			return "", false
		}
		return tx.Statement.Schema.Table, true
	}
	const queryCB = "payverge:test:lock_seq_query"
	const updateCB = "payverge:test:lock_seq_update"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(queryCB, func(tx *gorm.DB) {
		if table, ok := lockedTable(tx); ok {
			mu.Lock()
			seq = append(seq, "lock:"+table)
			mu.Unlock()
		}
	}))
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(updateCB, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "orders" {
			mu.Lock()
			seq = append(seq, "update:orders")
			mu.Unlock()
		}
	}))
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove(queryCB)
		_ = db.Callback().Update().Remove(updateCB)
	})
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), seq...)
	}
}

func firstIndex(seq []string, want ...string) int {
	for i, entry := range seq {
		for _, w := range want {
			if entry == w {
				return i
			}
		}
	}
	return -1
}

// Approve, operator cancel and guest cancel all write the bill. They must take
// the bill lock before touching the order row, the same order a line void and
// a floor merge use (bill, then the ticket); otherwise a void and a cancel of
// the same ticket deadlock.
func TestOrderBillWriters_LockBillBeforeOrder(t *testing.T) {
	setupOrderTestDB(t)
	business := helperBusiness(t, 0, 0)
	items := []OrderItem{{ID: voidLineIDA, MenuItemID: voidLineMenuA, MenuItemName: "Dish A", Quantity: 1, Price: 10, Subtotal: 10}}

	check := func(name string, run func() error) {
		t.Helper()
		seq := lockSequence(t)
		require.NoError(t, run(), name)
		got := seq()
		bill := firstIndex(got, "lock:bills")
		order := firstIndex(got, "lock:orders", "update:orders")
		require.NotEqual(t, -1, bill, "%s: bill never locked (%v)", name, got)
		require.NotEqual(t, -1, order, "%s: order never locked (%v)", name, got)
		require.Less(t, bill, order, "%s: bill must be locked before the order (%v)", name, got)
		_ = db.Callback().Query().Remove("payverge:test:lock_seq_query")
		_ = db.Callback().Update().Remove("payverge:test:lock_seq_update")
	}

	approveBill := helperBill(t, business, nil, 0)
	approveOrder := helperOrder(t, business, approveBill, items)
	check("approve", func() error {
		return UpdateOrderStatus(approveOrder.ID, OrderStatusApproved, "manager", "")
	})
	check("operator cancel", func() error {
		return UpdateOrderStatus(approveOrder.ID, OrderStatusOrderCancelled, "manager", "guest left")
	})

	guestBill := helperBill(t, business, nil, 0)
	guestOrder := helperOrder(t, business, guestBill, items)
	check("guest cancel", func() error {
		cancelled, err := CancelPendingGuestOrder(guestOrder, "changed mind", time.Now())
		require.True(t, cancelled)
		return err
	})
}

// BenchmarkUpdateOrderStatusApproveCancel measures the approve + cancel pair,
// the two transitions that take the bill lock.
func BenchmarkUpdateOrderStatusApproveCancel(b *testing.B) {
	setupOrderTestDB(b)
	business := helperBusiness(b, 0, 0)
	items := []OrderItem{{ID: voidLineIDA, MenuItemID: voidLineMenuA, MenuItemName: "Dish A", Quantity: 1, Price: 10, Subtotal: 10}}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		bill := helperBill(b, business, nil, 0)
		order := helperOrder(b, business, bill, items)
		b.StartTimer()
		if err := UpdateOrderStatus(order.ID, OrderStatusApproved, "manager", ""); err != nil {
			b.Fatal(err)
		}
		if err := UpdateOrderStatus(order.ID, OrderStatusOrderCancelled, "manager", "bench"); err != nil {
			b.Fatal(err)
		}
	}
}
