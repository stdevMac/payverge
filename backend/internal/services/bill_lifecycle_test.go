package services

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newBillLifecycleDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	g, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := g.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	require.NoError(t, g.AutoMigrate(
		&database.Business{},
		&database.Bill{},
		&database.Payment{},
		&database.AlternativePayment{},
		&database.BillHistoryEvent{},
		&database.Order{},
	))
	return g
}

func seedOpenBill(t *testing.T, g *gorm.DB, businessID uint, number string, createdAt time.Time, tableID uint) *database.Bill {
	t.Helper()
	bill := &database.Bill{
		BusinessID:  businessID,
		BillNumber:  number,
		Status:      database.BillStatusOpen,
		TotalAmount: 0,
		TableID:     tableID,
	}
	require.NoError(t, g.Create(bill).Error)
	require.NoError(t, g.Model(&database.Bill{}).Where("id = ?", bill.ID).
		Updates(map[string]interface{}{
			"created_at": createdAt,
			"updated_at": createdAt,
		}).Error)
	require.NoError(t, g.First(bill, bill.ID).Error)
	return bill
}

// Task 16 (a): a bill open past the abandon threshold transitions to abandoned
// and leaves the open/active set.
func TestBillLifecycle_AbandonsStaleOpenBill(t *testing.T) {
	g := newBillLifecycleDB(t)
	const businessID = uint(1601)
	stale := time.Now().Add(-30 * time.Hour)
	bill := seedOpenBill(t, g, businessID, "ABANDON-1", stale, 7)

	var notified []uint
	var mu sync.Mutex
	s := NewBillLifecycleSweeper(g, BillLifecycleConfig{
		DefaultThreshold: 24 * time.Hour,
		BatchSize:        50,
		Notify: func(bizID uint, b database.Bill, age time.Duration) {
			mu.Lock()
			notified = append(notified, b.ID)
			mu.Unlock()
			require.Equal(t, businessID, bizID)
			require.GreaterOrEqual(t, age, 24*time.Hour)
		},
	})

	n, err := s.RunOnce(time.Now())
	require.NoError(t, err)
	require.Equal(t, 1, n, "exactly one bill must be abandoned")

	var got database.Bill
	require.NoError(t, g.First(&got, bill.ID).Error)
	require.Equal(t, database.BillStatusAbandoned, got.Status)
	require.NotNil(t, got.AbandonedAt)
	require.WithinDuration(t, time.Now(), *got.AbandonedAt, 5*time.Second)

	// Leaves the open set used by ActiveBillStatuses / live boards.
	var activeCount int64
	require.NoError(t, g.Model(&database.Bill{}).
		Where("business_id = ? AND status IN ?", businessID, []string{
			string(database.BillStatusOpen),
			string(database.BillStatusPartial),
		}).Count(&activeCount).Error)
	require.Equal(t, int64(0), activeCount)

	mu.Lock()
	require.Equal(t, []uint{bill.ID}, notified, "operator notification must fire before abandon")
	mu.Unlock()
}

// Never touch a bill with a payment attempt still in flight.
func TestBillLifecycle_SkipsPaymentInFlight(t *testing.T) {
	g := newBillLifecycleDB(t)
	const businessID = uint(1602)
	stale := time.Now().Add(-48 * time.Hour)
	pendingBill := seedOpenBill(t, g, businessID, "PENDING-PAY", stale, 1)
	altPendingBill := seedOpenBill(t, g, businessID, "ALT-PENDING", stale, 2)
	freshEnough := seedOpenBill(t, g, businessID, "FRESH", time.Now().Add(-2*time.Hour), 3)

	require.NoError(t, g.Create(&database.Payment{
		BillID:    pendingBill.ID,
		PayerAddr: "0xpayer",
		Amount:    1000,
		Status:    database.PaymentStatusPending,
		TxHash:    "0xpending-tx",
	}).Error)
	require.NoError(t, g.Create(&database.AlternativePayment{
		BillID:          altPendingBill.ID,
		ParticipantAddr: "cash-guest",
		Amount:          1000,
		Status:          database.AltPaymentStatusPending,
		PaymentMethod:   database.PaymentMethodCash,
	}).Error)

	s := NewBillLifecycleSweeper(g, BillLifecycleConfig{
		DefaultThreshold: 24 * time.Hour,
	})
	n, err := s.RunOnce(time.Now())
	require.NoError(t, err)
	require.Equal(t, 0, n, "in-flight payments and young bills must not be abandoned")

	for _, id := range []uint{pendingBill.ID, altPendingBill.ID, freshEnough.ID} {
		var got database.Bill
		require.NoError(t, g.First(&got, id).Error)
		require.Equal(t, database.BillStatusOpen, got.Status)
		require.Nil(t, got.AbandonedAt)
	}
}

// Business-configurable threshold: a tighter override abandons earlier.
func TestBillLifecycle_BusinessThresholdOverride(t *testing.T) {
	g := newBillLifecycleDB(t)
	const businessID = uint(1603)
	// 6 hours old — below the 24h default, above a 4h override.
	mid := time.Now().Add(-6 * time.Hour)
	bill := seedOpenBill(t, g, businessID, "OVERRIDE-1", mid, 1)

	s := NewBillLifecycleSweeper(g, BillLifecycleConfig{
		DefaultThreshold: 24 * time.Hour,
		ThresholdForBusiness: func(id uint) time.Duration {
			if id == businessID {
				return 4 * time.Hour
			}
			return 0
		},
	})
	n, err := s.RunOnce(time.Now())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	var got database.Bill
	require.NoError(t, g.First(&got, bill.ID).Error)
	require.Equal(t, database.BillStatusAbandoned, got.Status)
}

// Bounded batch: one RunOnce abandons at most BatchSize bills.
func TestBillLifecycle_BoundedBatch(t *testing.T) {
	g := newBillLifecycleDB(t)
	const businessID = uint(1604)
	stale := time.Now().Add(-40 * time.Hour)
	for i := 0; i < 5; i++ {
		seedOpenBill(t, g, businessID, fmt.Sprintf("BATCH-%d", i), stale.Add(-time.Duration(i)*time.Minute), uint(i+1))
	}
	s := NewBillLifecycleSweeper(g, BillLifecycleConfig{
		DefaultThreshold: 24 * time.Hour,
		BatchSize:        2,
	})
	n, err := s.RunOnce(time.Now())
	require.NoError(t, err)
	require.Equal(t, 2, n)

	var abandoned int64
	require.NoError(t, g.Model(&database.Bill{}).
		Where("business_id = ? AND status = ?", businessID, database.BillStatusAbandoned).
		Count(&abandoned).Error)
	require.Equal(t, int64(2), abandoned)
}

// #704: abandoning a stale check while expo still holds in_kitchen tickets
// is how T2/T3 showed Available with tickets 1124/1127 still cooking.
func TestBillLifecycle_SkipsLiveKitchenTickets(t *testing.T) {
	g := newBillLifecycleDB(t)
	const businessID = uint(1606)
	stale := time.Now().Add(-30 * time.Hour)
	cooking := seedOpenBill(t, g, businessID, "COOK-1", stale, 5)
	pending := seedOpenBill(t, g, businessID, "PEND-1", stale, 1)
	idle := seedOpenBill(t, g, businessID, "IDLE-1", stale, 2)

	require.NoError(t, g.Create(&database.Order{
		BillID:      cooking.ID,
		BusinessID:  businessID,
		OrderNumber: "G86-1124",
		Status:      database.OrderStatusInKitchen,
		Items:       "[]",
	}).Error)
	require.NoError(t, g.Create(&database.Order{
		BillID:      pending.ID,
		BusinessID:  businessID,
		OrderNumber: "G86-1129",
		Status:      database.OrderStatusPending,
		Items:       "[]",
	}).Error)

	s := NewBillLifecycleSweeper(g, BillLifecycleConfig{
		DefaultThreshold: 24 * time.Hour,
	})
	n, err := s.RunOnce(time.Now())
	require.NoError(t, err)
	require.Equal(t, 1, n, "only the idle stale check may be abandoned")

	var gotCooking, gotPending, gotIdle database.Bill
	require.NoError(t, g.First(&gotCooking, cooking.ID).Error)
	require.NoError(t, g.First(&gotPending, pending.ID).Error)
	require.NoError(t, g.First(&gotIdle, idle.ID).Error)
	require.Equal(t, database.BillStatusOpen, gotCooking.Status)
	require.Equal(t, database.BillStatusOpen, gotPending.Status)
	require.Equal(t, database.BillStatusAbandoned, gotIdle.Status)
}

// After expo delivers, leftover unpaid is still settle — the sweeper must not
// abandon $37.82 the way CloseBill used to (#704 / 762). Delivery walk-out
// keeps AbandonUnpaidOpenBill; Liberar does not.
func TestBillLifecycle_SkipsUnpaidAfterOrderDelivered(t *testing.T) {
	g := newBillLifecycleDB(t)
	const businessID = uint(1607)
	stale := time.Now().Add(-30 * time.Hour)
	unpaid := seedOpenBill(t, g, businessID, "UNPAID-762", stale, 9)
	require.NoError(t, g.Model(&database.Bill{}).Where("id = ?", unpaid.ID).
		Updates(map[string]interface{}{
			"total_amount": int64(3782),
			"subtotal":     int64(3782),
		}).Error)
	require.NoError(t, g.Create(&database.Order{
		BillID:      unpaid.ID,
		BusinessID:  businessID,
		OrderNumber: "G86-762-DONE",
		Status:      database.OrderStatusOrderDelivered,
		Items:       "[]",
	}).Error)

	s := NewBillLifecycleSweeper(g, BillLifecycleConfig{
		DefaultThreshold: 24 * time.Hour,
	})
	n, err := s.RunOnce(time.Now())
	require.NoError(t, err)
	require.Equal(t, 0, n, "unpaid remaining after delivered must stay open")

	var got database.Bill
	require.NoError(t, g.First(&got, unpaid.ID).Error)
	require.Equal(t, database.BillStatusOpen, got.Status)
	require.Nil(t, got.AbandonedAt)
}

// SSE notification channel is used when no custom Notify is provided.
func TestBillLifecycle_DefaultNotifyPublishesSSE(t *testing.T) {
	g := newBillLifecycleDB(t)
	const businessID = uint(1605)
	stale := time.Now().Add(-30 * time.Hour)
	bill := seedOpenBill(t, g, businessID, "SSE-1", stale, 1)

	ch, _, cancel := events.GetHub().SubscribeWithReplayTopic(businessID, 0, "bill.abandoned")
	defer cancel()

	s := NewBillLifecycleSweeper(g, BillLifecycleConfig{
		DefaultThreshold: 24 * time.Hour,
	})
	n, err := s.RunOnce(time.Now())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	select {
	case evt := <-ch:
		require.NotNil(t, evt)
		_ = bill
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected bill.abandoned SSE event")
	}
}
