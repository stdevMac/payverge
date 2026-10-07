package services

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Issue #798: the abandon transition must be emitted exactly once per bill.
// Two bugs are pinned here:
//
//  1. RunOnce used to notify BEFORE abandonBill — a candidate that abandonBill
//     then refused (unpaid remaining, live kitchen, in-flight payment) got a
//     "bill.abandoned" operator notification re-published on EVERY tick while
//     the bill never actually abandoned.
//  2. A successfully abandoned bill left closed_at NULL (only abandoned_at was
//     stamped), so leftovers 1134/1139/1141 floated around half-terminal.

// The abandon transition fires once: the tick that abandons the bill notifies
// and writes one history event; the next tick is a complete no-op for that
// bill (no second notification, no second bill.abandoned history row).
func TestBillLifecycle_AbandonedOnceThenNextTickIsNoop(t *testing.T) {
	g := newBillLifecycleDB(t)
	const businessID = uint(1798)
	stale := time.Now().Add(-30 * time.Hour)
	bill := seedOpenBill(t, g, businessID, "ABANDON-798", stale, 4)

	var mu sync.Mutex
	var notifications int
	s := NewBillLifecycleSweeper(g, BillLifecycleConfig{
		DefaultThreshold: 24 * time.Hour,
		Notify: func(bizID uint, b database.Bill, age time.Duration) {
			mu.Lock()
			notifications++
			mu.Unlock()
		},
	})

	n, err := s.RunOnce(time.Now())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	var got database.Bill
	require.NoError(t, g.First(&got, bill.ID).Error)
	require.Equal(t, database.BillStatusAbandoned, got.Status)
	require.NotNil(t, got.AbandonedAt)
	require.NotNil(t, got.ClosedAt,
		"abandoned is terminal — the check must carry closed_at, not float half-dead (#798)")
	require.Nil(t, got.SettledAt,
		"the sweeper settles no money — settled_at must stay NULL (#798)")

	// Next tick: the already-abandoned bill must not be re-abandoned or
	// re-emitted.
	n, err = s.RunOnce(time.Now())
	require.NoError(t, err)
	require.Equal(t, 0, n, "an already-abandoned bill must not be re-abandoned next tick")

	mu.Lock()
	require.Equal(t, 1, notifications,
		"bill.abandoned must be emitted exactly once, not once per tick (#798)")
	mu.Unlock()

	var events int64
	require.NoError(t, g.Model(&database.BillHistoryEvent{}).
		Where("bill_id = ? AND event_type = ?", bill.ID, database.BillHistoryEventBillAbandoned).
		Count(&events).Error)
	require.Equal(t, int64(1), events, "exactly one bill.abandoned history event")

	var reloaded database.Bill
	require.NoError(t, g.First(&reloaded, bill.ID).Error)
	require.Equal(t, got.AbandonedAt.Unix(), reloaded.AbandonedAt.Unix(),
		"abandoned_at must keep the original transition time")
}

// A stale unpaid leftover ($37.82 remaining) is not sweepable — leftover money
// is settle, not write-off. It must not receive a "bill.abandoned" operator
// notification on every tick while staying open forever (#798: hourly
// bill.abandoned spam for a bill that never abandons).
func TestBillLifecycle_RefusedUnpaidCandidateNeverEmitsAbandoned(t *testing.T) {
	g := newBillLifecycleDB(t)
	const businessID = uint(1799)
	stale := time.Now().Add(-30 * time.Hour)
	bill := seedOpenBill(t, g, businessID, "UNPAID-798", stale, 6)
	require.NoError(t, g.Model(&database.Bill{}).Where("id = ?", bill.ID).
		Updates(map[string]interface{}{
			"total_amount": int64(3782),
			"subtotal":     int64(3782),
		}).Error)

	var mu sync.Mutex
	var notifications int
	s := NewBillLifecycleSweeper(g, BillLifecycleConfig{
		DefaultThreshold: 24 * time.Hour,
		Notify: func(bizID uint, b database.Bill, age time.Duration) {
			mu.Lock()
			notifications++
			mu.Unlock()
		},
	})

	for tick := 0; tick < 3; tick++ {
		n, err := s.RunOnce(time.Now())
		require.NoError(t, err)
		require.Equal(t, 0, n)
	}

	mu.Lock()
	require.Equal(t, 0, notifications,
		"a bill the sweeper refuses to abandon must not emit bill.abandoned on every tick (#798)")
	mu.Unlock()

	var got database.Bill
	require.NoError(t, g.First(&got, bill.ID).Error)
	require.Equal(t, database.BillStatusOpen, got.Status)
	require.Nil(t, got.AbandonedAt)

	var events int64
	require.NoError(t, g.Model(&database.BillHistoryEvent{}).
		Where("bill_id = ? AND event_type = ?", bill.ID, database.BillHistoryEventBillAbandoned).
		Count(&events).Error)
	require.Equal(t, int64(0), events)
}

// BenchmarkBillLifecycleRunOnce_UnpaidLeftovers measures one sweeper tick over
// a venue full of permanent unpaid leftovers (money outstanding — never
// sweepable). Before #798 each tick loaded every leftover as a candidate and
// opened a locking transaction per bill just to refuse it; after, the SQL
// pre-filter (total_amount - paid_amount <= 0) excludes them at scan time.
func BenchmarkBillLifecycleRunOnce_UnpaidLeftovers(b *testing.B) {
	g := benchBillLifecycleDB(b)
	stale := time.Now().Add(-30 * time.Hour)
	for i := 0; i < 300; i++ {
		bill := &database.Bill{
			BusinessID:  9798,
			BillNumber:  fmt.Sprintf("LEFTOVER-%d", i),
			Status:      database.BillStatusOpen,
			Subtotal:    3782,
			TotalAmount: 3782,
			TableID:     uint(i + 1),
		}
		if err := g.Create(bill).Error; err != nil {
			b.Fatal(err)
		}
		if err := g.Model(&database.Bill{}).Where("id = ?", bill.ID).
			Updates(map[string]interface{}{"created_at": stale, "updated_at": stale}).Error; err != nil {
			b.Fatal(err)
		}
	}
	s := NewBillLifecycleSweeper(g, BillLifecycleConfig{
		DefaultThreshold: 24 * time.Hour,
		Notify:           func(uint, database.Bill, time.Duration) {},
	})
	now := time.Now()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := s.RunOnce(now); err != nil {
			b.Fatal(err)
		}
	}
}

func benchBillLifecycleDB(b *testing.B) *gorm.DB {
	b.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", b.Name())
	g, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, err := g.DB()
	if err != nil {
		b.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	b.Cleanup(func() { _ = sqlDB.Close() })
	if err := g.AutoMigrate(
		&database.Business{},
		&database.Bill{},
		&database.Payment{},
		&database.AlternativePayment{},
		&database.BillHistoryEvent{},
		&database.Order{},
	); err != nil {
		b.Fatal(err)
	}
	return g
}
