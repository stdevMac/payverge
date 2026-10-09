package accounting

import (
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// BenchmarkListEntries is the L6-15 Backend Performance Gate microbench for the
// manual-ledger list page: the branch adds three actor preloads (selected
// columns) plus one grouped attachment COUNT per page on top of the base
// filtered page query. Run with -benchmem -count=3; compare against the
// merge-base ListEntries (no preloads, no attachment counts) by temporarily
// restoring it — numbers live in summary.md.
func BenchmarkListEntries(b *testing.B) {
	dsn := fmt.Sprintf("file:bench_list_entries_%d?mode=memory", time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, _ := gormDB.DB()
	sqlDB.SetMaxOpenConns(1)
	prev := database.GetDB()
	database.SetTestDB(gormDB)
	b.Cleanup(func() { database.SetTestDB(prev) })
	if err := gormDB.AutoMigrate(
		&database.Business{}, &database.User{}, &database.Staff{},
		&database.ManualLedgerEntry{}, &database.LedgerEntryAttachment{},
	); err != nil {
		b.Fatal(err)
	}

	biz := &database.Business{
		BusinessId: "bench-entries", Name: "BenchEntries", OwnerAddress: "0xBenchE",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD", DisplayCurrency: "USD", DefaultLanguage: "en",
		SourceLanguage: "en",
	}
	if err := gormDB.Create(biz).Error; err != nil {
		b.Fatal(err)
	}
	owner := &database.User{Email: "owner@bench.test", Name: "Bench Owner", Role: "user"}
	if err := gormDB.Create(owner).Error; err != nil {
		b.Fatal(err)
	}
	staffIDs := make([]uint, 0, 5)
	for i := 0; i < 5; i++ {
		s := &database.Staff{
			BusinessID: biz.ID, Name: fmt.Sprintf("Staff %d", i),
			Email: fmt.Sprintf("staff%d@bench.test", i), Role: "server", IsActive: true,
		}
		if err := gormDB.Create(s).Error; err != nil {
			b.Fatal(err)
		}
		staffIDs = append(staffIDs, s.ID)
	}

	start := time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 500; i++ {
		occurred := start.Add(time.Duration(i) * time.Minute)
		entry := &database.ManualLedgerEntry{
			BusinessID: biz.ID, EntryType: database.AccountingEntryTypeExpense,
			Category: "supplies", Amount: int64(1000 + i), Currency: "USD",
			OccurredAt: occurred, Description: fmt.Sprintf("Entry %d", i),
		}
		if i%2 == 0 {
			entry.CreatedByUserID = &owner.ID
		} else {
			entry.CreatedByStaffID = &staffIDs[i%5]
		}
		if err := gormDB.Create(entry).Error; err != nil {
			b.Fatal(err)
		}
		if i%3 == 0 {
			for a := 0; a < 2; a++ {
				if err := gormDB.Create(&database.LedgerEntryAttachment{
					EntryID: entry.ID, BusinessID: biz.ID,
					S3Key: fmt.Sprintf("k-%d-%d", i, a), FileName: "receipt.pdf",
				}).Error; err != nil {
					b.Fatal(err)
				}
			}
		}
	}

	svc := NewService(database.GetDBWrapper())
	input := ListEntriesInput{
		BusinessID: biz.ID, StartDate: &start, EndDate: &end,
		Page: 1, PageSize: 20,
	}
	// Warm.
	if _, err := svc.ListEntries(input); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := svc.ListEntries(input); err != nil {
			b.Fatal(err)
		}
	}
}
