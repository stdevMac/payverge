package accounting

import (
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// BenchmarkGetSummary_CollectionGap measures GetSummary (includes collection_gap
// which unpaid-bills mirrors) for a window with outstanding bills.
func BenchmarkGetSummary_CollectionGap(b *testing.B) {
	dsn := fmt.Sprintf("file:bench_unpaid_%d?mode=memory", time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, _ := gormDB.DB()
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(gormDB)
	if err := gormDB.AutoMigrate(
		&database.Business{}, &database.Staff{}, &database.Bill{}, &database.Payment{},
		&database.AlternativePayment{}, &database.WithdrawalHistory{}, &database.ExchangeRate{},
		&database.ManualLedgerEntry{}, &database.PayrollRun{}, &database.PayrollLineItem{},
	); err != nil {
		b.Fatal(err)
	}

	biz := &database.Business{
		BusinessId: "bench-unpaid", Name: "BenchUnpaid", OwnerAddress: "0xBenchU",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD", DisplayCurrency: "USD", DefaultLanguage: "en",
		SourceLanguage: "en",
	}
	if err := gormDB.Create(biz).Error; err != nil {
		b.Fatal(err)
	}

	start := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 50; i++ {
		created := start.Add(time.Duration(i) * time.Hour)
		bill := &database.Bill{
			BusinessID:  biz.ID,
			BillNumber:  fmt.Sprintf("BU-%d", i),
			Status:      database.BillStatusOpen,
			Items:       "[]",
			TotalAmount: int64(10000 + i*100),
			PaidAmount:  int64(i * 50),
			CreatedAt:   created,
			UpdatedAt:   created,
		}
		if err := gormDB.Create(bill).Error; err != nil {
			b.Fatal(err)
		}
	}

	service := NewService(database.GetDBWrapper())
	// Warm.
	if _, err := service.GetSummary(biz.ID, start, end); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := service.GetSummary(biz.ID, start, end); err != nil {
			b.Fatal(err)
		}
	}
}
