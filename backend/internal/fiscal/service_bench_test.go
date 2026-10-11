package fiscal

import (
	"context"
	"fmt"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupFiscalIssueBenchDB(b *testing.B) (*gorm.DB, uint, []database.Bill) {
	b.Helper()
	db, err := gorm.Open(sqlite.Open("file:fiscal-issue-bench?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		b.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		b.Fatalf("sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	b.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(
		&database.Business{},
		&database.Bill{},
		&database.Payment{},
		&database.AlternativePayment{},
		&database.BusinessFiscalSettings{},
		&database.FiscalReceipt{},
		&database.FiscalJob{},
		&database.FiscalAuditEvent{},
	); err != nil {
		b.Fatalf("auto-migrate: %v", err)
	}

	business := database.Business{
		BusinessId:     "fiscal-bench",
		OwnerAddress:   "owner",
		Name:           "Fiscal Bench",
		SettlementAddr: "settlement",
		TippingAddr:    "tipping",
	}
	if err := db.Create(&business).Error; err != nil {
		b.Fatalf("create business: %v", err)
	}
	settings := database.BusinessFiscalSettings{
		BusinessID:  business.ID,
		Country:     "AR",
		Provider:    "arca",
		Mode:        database.FiscalModeManual,
		Environment: "sandbox",
		SetupStatus: "valid",
	}
	if err := db.Create(&settings).Error; err != nil {
		b.Fatalf("create settings: %v", err)
	}

	bills := make([]database.Bill, b.N)
	for i := range bills {
		bills[i] = database.Bill{
			BusinessID:  business.ID,
			BillNumber:  fmt.Sprintf("FISCAL-BENCH-BILL-%06d", i),
			Status:      database.BillStatusPaid,
			Items:       "[]",
			TotalAmount: 1200,
			PaidAmount:  1200,
		}
	}
	if err := db.CreateInBatches(&bills, 500).Error; err != nil {
		b.Fatalf("create bills: %v", err)
	}

	return db, business.ID, bills
}

func BenchmarkFiscalIssueReceiptEnqueue(b *testing.B) {
	db, businessID, bills := setupFiscalIssueBenchDB(b)
	svc := NewService(db, NewProviderRegistry())
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := svc.IssueReceipt(ctx, businessID, bills[i].ID, "bench"); err != nil {
			b.Fatalf("issue receipt: %v", err)
		}
	}
}
