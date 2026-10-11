package database

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/testperf"
)

// setupOrdersListPostgres provisions a throwaway Postgres with one business and
// 500 bills/orders, and configures the production pool (25 open / 10 idle) so
// the parallel bench actually exercises pool contention — unlike the SQLite
// benches which force MaxOpenConns(1).
func setupOrdersListPostgres(t testing.TB) *Business {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping Postgres container bench in -short")
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

	sqlDB, err := pg.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(10)

	// DeliveryOrder is required so the orders-list NOT EXISTS (dead-delivery
	// exclusion, audit M7) subquery can resolve the delivery_orders table.
	require.NoError(t, db.AutoMigrate(&Business{}, &Table{}, &Bill{}, &Order{}, &DeliveryOrder{}))

	business := &Business{
		BusinessId:     fmt.Sprintf("orders-list-par-%d", time.Now().UnixNano()),
		Name:           "Orders List Parallel",
		OwnerAddress:   "0xOrdersListParallelOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, db.Create(business).Error)

	table := &Table{BusinessID: business.ID, TableCode: "OLP-1", Name: "OLP", IsActive: true}
	require.NoError(t, db.Create(table).Error)

	orderItems := "[" + strings.TrimSuffix(strings.Repeat(`{"id":"i","menu_item_name":"B","quantity":1,"price":12.5,"subtotal":12.5},`, 8), ",") + "]"
	now := time.Now().UTC()
	for i := 0; i < 500; i++ {
		status := BillStatusOpen
		if i%5 == 0 {
			status = BillStatusClosed
		}
		bill := &Bill{
			BusinessID:     business.ID,
			TableID:        table.ID,
			BillNumber:     fmt.Sprintf("OLP-BILL-%03d", i),
			Items:          "[]",
			Subtotal:       1250,
			TotalAmount:    1250,
			Status:         status,
			SettlementAddr: "0x1111111111111111111111111111111111111111",
			TippingAddr:    "0x2222222222222222222222222222222222222222",
			CreatedAt:      now.Add(-time.Duration(i) * time.Minute),
			UpdatedAt:      now.Add(-time.Duration(i) * time.Minute),
		}
		require.NoError(t, db.Create(bill).Error)

		orderStatus := OrderStatusApproved
		if i%4 == 0 {
			orderStatus = OrderStatusPending
		}
		require.NoError(t, db.Create(&Order{
			BillID:      bill.ID,
			BusinessID:  business.ID,
			OrderNumber: fmt.Sprintf("OLP-%03d", i),
			Status:      orderStatus,
			CreatedBy:   "guest",
			Items:       orderItems,
			CreatedAt:   now.Add(-time.Duration(i) * time.Minute),
			UpdatedAt:   now.Add(-time.Duration(i) * time.Minute),
		}).Error)
	}
	return business
}

// BenchmarkGetOrdersByBusinessIDPaginatedParallelPostgres exercises the order
// list read concurrently over the production 25-conn pool against real
// Postgres, the only bench in the suite that measures pool contention.
func BenchmarkGetOrdersByBusinessIDPaginatedParallelPostgres(b *testing.B) {
	business := setupOrdersListPostgres(b)
	pagination := PaginationParams{Page: 1, PageSize: 100}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			result, err := GetOrdersByBusinessIDPaginated(business.ID, "pending,approved", pagination, OrderListOptions{ActiveBillsOnly: true})
			if err != nil {
				b.Fatal(err)
			}
			if len(result.Data) != 100 {
				b.Fatalf("expected 100 orders, got %d", len(result.Data))
			}
		}
	})
}
