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

// setupTableStatusPostgres provisions a throwaway Postgres container with one
// business, 500 active tables (each with one open bill, every 5th also with an
// upcoming confirmed reservation), and configures the production pool
// (25 open / 10 idle). The dashboard-poll bench is serial (per-operator call),
// not parallel, but the production pool size is set to match the real server
// profile so connection-checkout cost is included.
func setupTableStatusPostgres(t testing.TB) uint {
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

	// BillItem is Postgres-native (uuid + gen_random_uuid default), so unlike
	// the SQLite fixture no handwritten DDL is needed. GetTablesWithStatus
	// aggregates bill_items for the physical-quantity column.
	require.NoError(t, db.AutoMigrate(&Business{}, &Table{}, &Bill{}, &BillItem{}, &TableReservation{}, &Order{}))

	business := &Business{
		BusinessId:     fmt.Sprintf("table-status-pg-%d", time.Now().UnixNano()),
		Name:           "Table Status PG Bench",
		OwnerAddress:   "0xTableStatusPGOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
	}
	require.NoError(t, db.Create(business).Error)

	now := time.Now().UTC()
	// Match the SQLite fixture: a realistic bill Items JSON blob (256 items).
	billItems := "[" + strings.TrimSuffix(strings.Repeat(`{"id":"item","name":"Bench","quantity":1},`, 256), ",") + "]"

	for i := 0; i < 500; i++ {
		table := &Table{
			BusinessID: business.ID,
			TableCode:  fmt.Sprintf("PG-STATUS-%03d", i),
			Name:       fmt.Sprintf("PG Status Table %03d", i),
			Capacity:   4,
			QRCode:     strings.Repeat("qr-payload", 128),
			IsActive:   true,
		}
		require.NoError(t, db.Create(table).Error)

		require.NoError(t, db.Create(&Bill{
			BusinessID:     business.ID,
			TableID:        table.ID,
			BillNumber:     fmt.Sprintf("PG-STATUS-BILL-%03d", i),
			Items:          billItems,
			Subtotal:       5000,
			TotalAmount:    5000,
			PaidAmount:     1000,
			Status:         BillStatusOpen,
			SettlementAddr: "0x1111111111111111111111111111111111111111",
			TippingAddr:    "0x2222222222222222222222222222222222222222",
			CreatedAt:      now.Add(-time.Duration(i) * time.Minute),
			UpdatedAt:      now.Add(-time.Duration(i) * time.Minute),
		}).Error)

		if i%5 == 0 {
			tableID := table.ID
			require.NoError(t, db.Create(&TableReservation{
				BusinessID:       business.ID,
				TableID:          &tableID,
				CustomerName:     fmt.Sprintf("PG Guest %03d", i),
				CustomerEmail:    fmt.Sprintf("pg-guest-%03d@example.com", i),
				PartySize:        2,
				ReservationTime:  now.Add(15 * time.Minute),
				Duration:         60,
				Status:           "confirmed",
				ConfirmationCode: fmt.Sprintf("PG-STATUS-CONF-%03d", i),
				SpecialRequests:  strings.Repeat("window ", 32),
				Notes:            strings.Repeat("internal note ", 32),
			}).Error)
		}
	}

	return business.ID
}

// BenchmarkGetTablesWithStatusPostgres measures the table-status dashboard poll
// (GetTablesWithStatus) against real Postgres with the production connection
// pool. This is the single highest-frequency authenticated poll (operators poll
// the table grid on every dashboard page load). The bench is serial — dashboard
// polls are per-operator, not concurrent — and Docker-gated via -short skip.
func BenchmarkGetTablesWithStatusPostgres(b *testing.B) {
	businessID := setupTableStatusPostgres(b)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rows, err := GetTablesWithStatus(businessID, false)
		if err != nil {
			b.Fatal(err)
		}
		if len(rows) != 500 {
			b.Fatalf("expected 500 table status rows, got %d", len(rows))
		}
	}
}
