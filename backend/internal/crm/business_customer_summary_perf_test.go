package crm

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupSummaryPerfDB seeds a SQLite in-memory database with:
//   - one target business with `targetRows` CustomerBusiness rows
//   - one noise business with 50 rows (ensures the WHERE clause
//     on business_id is exercised and the aggregate is not over-counting).
//
// Tiers are distributed: first 20% Gold, rest Bronze.
// Recent visits: first 30% have last_visit_at within 30 days.
func setupSummaryPerfDB(b *testing.B, targetRows int) (*Service, uint) {
	b.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_", ".", "_").Replace(b.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		b.Fatalf("open sqlite: %v", err)
	}

	sqlDB, err := gormDB.DB()
	if err != nil {
		b.Fatalf("get sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	b.Cleanup(func() { _ = sqlDB.Close() })

	database.SetTestDB(gormDB)
	if err := gormDB.AutoMigrate(
		&database.Customer{},
		&database.CustomerPreferences{},
		&database.Business{},
		&database.CustomerBusiness{},
	); err != nil {
		b.Fatalf("automigrate: %v", err)
	}

	mkBusiness := func(tag string) *database.Business {
		biz := &database.Business{
			BusinessId:      fmt.Sprintf("biz-%s-%d", tag, time.Now().UnixNano()),
			Name:            fmt.Sprintf("Biz %s", tag),
			OwnerAddress:    fmt.Sprintf("0xOwner%s", tag),
			SettlementAddr:  "0x1111111111111111111111111111111111111111",
			TippingAddr:     "0x2222222222222222222222222222222222222222",
			DefaultCurrency: "USD",
			IsActive:        true,
		}
		if err := gormDB.Create(biz).Error; err != nil {
			b.Fatalf("create business: %v", err)
		}
		return biz
	}

	target := mkBusiness("target")
	noise := mkBusiness("noise")

	now := time.Now().UTC()
	recent := now.Add(-10 * 24 * time.Hour) // within 30-day window

	seedConnections := func(bizID uint, n int) {
		for i := 0; i < n; i++ {
			c := &database.Customer{
				Email:        fmt.Sprintf("cust-%d-%d-%d@example.com", bizID, i, time.Now().UnixNano()),
				PasswordHash: "hash",
				Name:         fmt.Sprintf("Customer %d", i),
				IsActive:     true,
			}
			if err := gormDB.Create(c).Error; err != nil {
				b.Fatalf("create customer: %v", err)
			}

			tier := "Bronze"
			if float64(i) < float64(n)*0.20 {
				tier = "Gold"
			}

			var lastVisit *time.Time
			if float64(i) < float64(n)*0.30 {
				v := recent
				lastVisit = &v
			}

			cb := &database.CustomerBusiness{
				CustomerID:   c.ID,
				BusinessID:   bizID,
				LoyaltyTier:  tier,
				TotalSpent:   float64(50 + i),
				VisitCount:   i % 30,
				LastVisitAt:  lastVisit,
				FirstVisitAt: now.Add(-time.Duration(i+1) * 24 * time.Hour),
				IsActive:     true,
			}
			if err := gormDB.Create(cb).Error; err != nil {
				b.Fatalf("create customer_business: %v", err)
			}
		}
	}

	seedConnections(target.ID, targetRows)
	seedConnections(noise.ID, 50)

	return NewService(database.GetDB()), target.ID
}

// BenchmarkGetBusinessCustomerSummary measures the single SQL aggregate
// pass introduced in commit a241b699. Seeded with 1 000 rows for the target
// business plus 50 noise rows for a second business.
func BenchmarkGetBusinessCustomerSummary(b *testing.B) {
	service, businessID := setupSummaryPerfDB(b, 1000)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		summary, err := service.GetBusinessCustomerSummary(businessID, "", "", "")
		if err != nil {
			b.Fatal(err)
		}
		if summary.TotalCustomers != 1000 {
			b.Fatalf("expected 1000 total customers, got %d", summary.TotalCustomers)
		}
	}
}

// BenchmarkListBusinessCustomersWithSummary measures the combined handler-visible
// cost: one SQL aggregate (GetBusinessCustomerSummary) + one paginated list query
// (GetBusinessCustomers, first page of 20). This models a typical dashboard load.
func BenchmarkListBusinessCustomersWithSummary(b *testing.B) {
	service, businessID := setupSummaryPerfDB(b, 1000)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		summary, err := service.GetBusinessCustomerSummary(businessID, "", "", "")
		if err != nil {
			b.Fatal(err)
		}
		if summary.TotalCustomers != 1000 {
			b.Fatalf("expected 1000 total customers, got %d", summary.TotalCustomers)
		}

		customers, total, err := service.GetBusinessCustomers(businessID, 1, 20, "", "")
		if err != nil {
			b.Fatal(err)
		}
		if total != 1000 {
			b.Fatalf("expected list total 1000, got %d", total)
		}
		if len(customers) != 20 {
			b.Fatalf("expected 20 customers on page 1, got %d", len(customers))
		}
	}
}
