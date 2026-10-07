package print

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func setupPrintReceiptBenchDB(b *testing.B) (*gorm.DB, uint, *uint, []database.Bill) {
	b.Helper()
	db, err := gorm.Open(sqlite.Open("file:print-receipt-bench?mode=memory&cache=shared"), &gorm.Config{})
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
		&database.Printer{},
		&database.Table{},
		&database.Payment{},
		&database.Bill{},
		&database.PrintJob{},
	); err != nil {
		b.Fatalf("auto-migrate: %v", err)
	}

	business := database.Business{
		BusinessId:      "print-bench",
		OwnerAddress:    "owner",
		Name:            "Print Bench",
		SettlementAddr:  "settlement",
		TippingAddr:     "tipping",
		DefaultCurrency: "USD",
	}
	if err := db.Create(&business).Error; err != nil {
		b.Fatalf("create business: %v", err)
	}
	table := database.Table{
		BusinessID: business.ID,
		TableCode:  "print-bench-t01",
		Name:       "T1",
		IsActive:   true,
	}
	if err := db.Create(&table).Error; err != nil {
		b.Fatalf("create table: %v", err)
	}
	locID := uint(1)
	printer := database.Printer{
		BusinessID:   business.ID,
		LocationID:   &locID,
		Name:         "front",
		Role:         "bill",
		Transport:    "browser",
		PaperWidthMM: 80,
		Enabled:      true,
	}
	if err := db.Create(&printer).Error; err != nil {
		b.Fatalf("create printer: %v", err)
	}

	bills := make([]database.Bill, b.N)
	for i := range bills {
		bills[i] = database.Bill{
			BusinessID:     business.ID,
			TableID:        table.ID,
			BillNumber:     fmt.Sprintf("PRINT-BENCH-BILL-%06d", i),
			Items:          `[{"name":"Burger","quantity":1,"subtotal":12}]`,
			Subtotal:       1200,
			TotalAmount:    1200,
			PaidAmount:     1200,
			Status:         database.BillStatusPaid,
			SettlementAddr: "settlement",
			TippingAddr:    "tipping",
		}
	}
	if err := db.CreateInBatches(&bills, 500).Error; err != nil {
		b.Fatalf("create bills: %v", err)
	}

	confirmedAt := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	payments := make([]database.Payment, len(bills))
	for i := range bills {
		payments[i] = database.Payment{
			BillID:        bills[i].ID,
			PayerAddr:     "staff_manual",
			Amount:        1200,
			TxHash:        fmt.Sprintf("print-bench-payment-%06d", i),
			Status:        database.PaymentStatusConfirmed,
			PaymentMethod: "cash",
			ConfirmedAt:   &confirmedAt,
		}
	}
	if err := db.CreateInBatches(&payments, 500).Error; err != nil {
		b.Fatalf("create payments: %v", err)
	}

	return db, business.ID, &locID, bills
}

func BenchmarkPrintEnqueueReceipt(b *testing.B) {
	db, businessID, locID, bills := setupPrintReceiptBenchDB(b)
	svc := NewService(db)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		job, err := svc.Enqueue(ctx, EnqueueParams{
			BusinessID: businessID,
			LocationID: locID,
			Kind:       database.PrintJobKindReceipt,
			SourceType: "bill",
			SourceID:   bills[i].ID,
			Language:   "en",
			CreatedBy:  "bench",
		})
		if err != nil {
			b.Fatalf("enqueue receipt: %v", err)
		}
		if job.Status != database.PrintJobStatusRouted {
			b.Fatalf("status = %s, want routed", job.Status)
		}
	}
}
