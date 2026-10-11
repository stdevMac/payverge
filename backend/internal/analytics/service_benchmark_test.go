package analytics

import (
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAnalyticsBenchmarkDB(b *testing.B, billCount int) (*database.DB, uint) {
	b.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", b.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		b.Fatalf("open benchmark db: %v", err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		b.Fatalf("unwrap benchmark db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	// Close the underlying connection on cleanup so the named in-memory
	// cache=shared database is destroyed. Without this, re-runs within the
	// same process (e.g. -count=N > 1) reuse the same shared DB and the
	// business INSERT collides on the unique constraint.
	b.Cleanup(func() { _ = sqlDB.Close() })

	if err := gormDB.AutoMigrate(
		&database.Business{},
		&database.Bill{},
		&database.Payment{},
		&database.AlternativePayment{},
	); err != nil {
		b.Fatalf("migrate benchmark db: %v", err)
	}

	database.SetTestDB(gormDB)
	db := database.GetDBWrapper()
	business := database.Business{Name: "Benchmark Restaurant"}
	if err := db.GetGorm().Create(&business).Error; err != nil {
		b.Fatalf("create benchmark business: %v", err)
	}

	baseTime := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	for i := 0; i < billCount; i++ {
		recognizedAt := baseTime.Add(-time.Duration(i%7) * 24 * time.Hour).Add(time.Duration(i%12) * time.Minute)
		bill := database.Bill{
			BusinessID:  business.ID,
			BillNumber:  fmt.Sprintf("BENCH-%05d", i),
			TotalAmount: 2500,
			PaidAmount:  2500,
			TipAmount:   300,
			Status:      database.BillStatusPaid,
			ClosedAt:    &recognizedAt,
			CreatedAt:   recognizedAt.Add(-30 * time.Minute),
			UpdatedAt:   recognizedAt,
		}
		if err := db.GetGorm().Create(&bill).Error; err != nil {
			b.Fatalf("create benchmark bill: %v", err)
		}
		if err := db.GetGorm().Create(&database.Payment{
			BillID:        bill.ID,
			PayerAddr:     fmt.Sprintf("0xbench%05d", i%250),
			Amount:        2500,
			TipAmount:     300,
			TxHash:        fmt.Sprintf("bench_payment_%05d", i),
			Status:        database.PaymentStatusConfirmed,
			PaymentMethod: "crypto",
			ConfirmedAt:   &recognizedAt,
			CreatedAt:     recognizedAt,
			UpdatedAt:     recognizedAt,
		}).Error; err != nil {
			b.Fatalf("create benchmark payment: %v", err)
		}
	}

	return db, business.ID
}

func BenchmarkGetPaymentWindowSummary(b *testing.B) {
	db, businessID := setupAnalyticsBenchmarkDB(b, 2500)
	service := NewAnalyticsService(db)
	start := time.Date(2026, 4, 27, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := service.GetPaymentWindowSummary(businessID, start, end); err != nil {
			b.Fatalf("window summary: %v", err)
		}
	}
}

func BenchmarkGetPeriodReport(b *testing.B) {
	db, businessID := setupAnalyticsBenchmarkDB(b, 2500)
	service := NewAnalyticsService(db).WithClock(func() time.Time {
		return time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := service.GetPeriodReport(businessID, "week", time.UTC); err != nil {
			b.Fatalf("period report: %v", err)
		}
	}
}

func BenchmarkGetDailySeries(b *testing.B) {
	// Seeds 2500 bills spread across 7 days ending 2026-05-04 (baseTime).
	// from/to are chosen to cover all seeded days so the benchmark measures
	// real aggregation work rather than an empty scan.
	db, businessID := setupAnalyticsBenchmarkDB(b, 2500)
	service := NewAnalyticsService(db)
	from := time.Date(2026, 4, 28, 0, 0, 0, 0, time.UTC) // earliest seeded day
	to := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)    // baseTime day (inclusive)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := service.GetDailySeries(businessID, from, to); err != nil {
			b.Fatal(err)
		}
	}
}

// setupPopularItemsBenchmarkDB seeds a business with 50 distinct bill-items
// each with their own bill + payment, creating a realistic scenario where the
// dashboard only needs the top 5. The bill_items table is created inline
// because setupAnalyticsBenchmarkDB does not AutoMigrate it.
func setupPopularItemsBenchmarkDB(b *testing.B) (*database.DB, uint) {
	b.Helper()

	const itemCount = 50

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", b.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		b.Fatalf("open popular-items benchmark db: %v", err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		b.Fatalf("unwrap benchmark db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	b.Cleanup(func() { _ = sqlDB.Close() })

	if err := gormDB.AutoMigrate(
		&database.Business{},
		&database.Bill{},
		&database.Payment{},
		&database.AlternativePayment{},
	); err != nil {
		b.Fatalf("migrate: %v", err)
	}
	if err := gormDB.Exec(`
		CREATE TABLE IF NOT EXISTS bill_items (
			id TEXT PRIMARY KEY,
			bill_id INTEGER NOT NULL,
			menu_item_id TEXT DEFAULT '',
			name TEXT NOT NULL,
			price REAL NOT NULL,
			quantity INTEGER NOT NULL,
			options TEXT,
			item_type TEXT DEFAULT 'menu_item',
			bundle_id INTEGER,
			parent_bundle_id INTEGER,
			source_offer_id INTEGER,
			order_id INTEGER,
			subtotal REAL NOT NULL,
			created_at DATETIME
		)
	`).Error; err != nil {
		b.Fatalf("create bill_items: %v", err)
	}

	database.SetTestDB(gormDB)
	db := database.GetDBWrapper()

	business := database.Business{Name: "Popular Items Bench Restaurant"}
	if err := db.GetGorm().Create(&business).Error; err != nil {
		b.Fatalf("create business: %v", err)
	}

	baseTime := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	for i := 0; i < itemCount; i++ {
		confirmedAt := baseTime.Add(-time.Duration(i%7) * 24 * time.Hour)
		price := float64((itemCount - i) * 10)
		bill := database.Bill{
			BusinessID:  business.ID,
			BillNumber:  fmt.Sprintf("PITEMBENCH-%05d", i),
			TotalAmount: int64(price * 100),
			PaidAmount:  int64(price * 100),
			Status:      database.BillStatusPaid,
			CreatedAt:   confirmedAt.Add(-30 * time.Minute),
			UpdatedAt:   confirmedAt,
		}
		if err := db.GetGorm().Create(&bill).Error; err != nil {
			b.Fatalf("create bill: %v", err)
		}
		if err := db.GetGorm().Create(&database.Payment{
			BillID:        bill.ID,
			PayerAddr:     fmt.Sprintf("0xpitem%05d", i),
			Amount:        int64(price * 100),
			TxHash:        fmt.Sprintf("pitem_payment_%05d", i),
			Status:        database.PaymentStatusConfirmed,
			PaymentMethod: "crypto",
			ConfirmedAt:   &confirmedAt,
			CreatedAt:     confirmedAt,
			UpdatedAt:     confirmedAt,
		}).Error; err != nil {
			b.Fatalf("create payment: %v", err)
		}
		if err := db.GetGorm().Exec(
			`INSERT INTO bill_items (id, bill_id, name, price, quantity, subtotal, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			fmt.Sprintf("pitem-%05d", i), bill.ID,
			fmt.Sprintf("Item%d", i),
			price, 1, price, confirmedAt,
		).Error; err != nil {
			b.Fatalf("create bill_item: %v", err)
		}
	}

	return db, business.ID
}

// BenchmarkGetPopularItems_Before measures GetPopularItems with limit=0
// (fetches all 50 items and resolves names for all of them). Represents the
// unbounded path that the dashboard used to take before DUP-02 was fixed.
func BenchmarkGetPopularItems_Before(b *testing.B) {
	db, businessID := setupPopularItemsBenchmarkDB(b)
	service := NewAnalyticsService(db).WithClock(func() time.Time {
		return time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := service.GetPopularItems(businessID, 0, "week", time.UTC); err != nil {
			b.Fatalf("GetPopularItems: %v", err)
		}
	}
}

// BenchmarkGetPopularItems_After measures GetPopularItems with limit=5,
// matching the dashboard call. SQL fetches only 5 rows; name resolution is
// bounded to 5 items instead of all 50.
func BenchmarkGetPopularItems_After(b *testing.B) {
	db, businessID := setupPopularItemsBenchmarkDB(b)
	service := NewAnalyticsService(db).WithClock(func() time.Time {
		return time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := service.GetPopularItems(businessID, 5, "week", time.UTC); err != nil {
			b.Fatalf("GetPopularItems: %v", err)
		}
	}
}
