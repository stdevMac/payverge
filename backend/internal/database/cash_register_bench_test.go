package database

import (
	"fmt"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func BenchmarkCreateConfirmedCashAlternativePayment(b *testing.B) {
	b.Run("no_open_session", func(b *testing.B) {
		benchmarkCreateConfirmedCashAlternativePayment(b, false)
	})
	b.Run("open_session", func(b *testing.B) {
		benchmarkCreateConfirmedCashAlternativePayment(b, true)
	})
}

func benchmarkCreateConfirmedCashAlternativePayment(b *testing.B, withOpenSession bool) {
	benchDB, business := setupCashRegisterBenchmarkDB(b, withOpenSession)
	bills := seedBenchmarkBills(b, benchDB, business, b.N)
	payments := make([]AlternativePayment, b.N)
	confirmedAt := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	for i := 0; i < b.N; i++ {
		payments[i] = AlternativePayment{
			BillID:          bills[i].ID,
			ParticipantName: "cashier",
			Amount:          1000,
			PaymentMethod:   PaymentMethodCash,
			Status:          AltPaymentStatusConfirmed,
			ConfirmedBy:     "staff:1",
			ConfirmedAt:     &confirmedAt,
			IdempotencyKey:  fmt.Sprintf("bench-%d", i),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := CreateConfirmedAlternativePayment(&payments[i], nil); err != nil {
			b.Fatal(err)
		}
	}
}

func setupCashRegisterBenchmarkDB(b *testing.B, withOpenSession bool) (*gorm.DB, Business) {
	b.Helper()

	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		b.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	b.Cleanup(func() {
		_ = sqlDB.Close()
	})

	if err := gormDB.AutoMigrate(
		&Business{},
		&Bill{},
		&AlternativePayment{},
		&BillSplitShare{},
		&BusinessMilestoneEvent{},
		&BusinessRevenueAggregate{},
		&CashRegisterSession{},
		&CashRegisterMovement{},
	); err != nil {
		b.Fatal(err)
	}

	SetTestDB(gormDB)
	business := Business{
		BusinessId:     "bench-cash-register",
		Name:           "Bench Cash Register",
		SettlementAddr: "settlement-bench",
		TippingAddr:    "tipping-bench",
		IsActive:       true,
	}
	if err := gormDB.Create(&business).Error; err != nil {
		b.Fatal(err)
	}
	if !withOpenSession {
		return gormDB, business
	}
	now := time.Now().UTC()
	session := CashRegisterSession{
		BusinessID:        business.ID,
		Status:            CashRegisterSessionStatusOpen,
		OpeningFloatCents: 0,
		ExpectedCashCents: 0,
		OpenedByLabel:     "staff:1",
		OpenedByStaffID:   benchPtrUint(1),
		OpenedAt:          now,
	}
	if err := gormDB.Create(&session).Error; err != nil {
		b.Fatal(err)
	}

	return gormDB, business
}

func seedBenchmarkBills(b *testing.B, db *gorm.DB, business Business, count int) []Bill {
	b.Helper()

	bills := make([]Bill, count)
	for i := 0; i < count; i++ {
		bills[i] = Bill{
			BusinessID:     business.ID,
			BillNumber:     fmt.Sprintf("BENCH-CASH-%d", i),
			Items:          "[]",
			SettlementAddr: business.SettlementAddr,
			TippingAddr:    business.TippingAddr,
			Status:         BillStatusOpen,
			Subtotal:       1000,
			TotalAmount:    1000,
		}
	}
	if err := db.CreateInBatches(&bills, 500).Error; err != nil {
		b.Fatal(err)
	}
	return bills
}

func benchPtrUint(value uint) *uint {
	return &value
}

func BenchmarkFindLastDeclaredOpeningFloatCents(b *testing.B) {
	gormDB, business := setupCashRegisterBenchmarkDB(b, false)
	now := time.Date(2026, 6, 27, 20, 0, 0, 0, time.UTC)
	sessions := make([]CashRegisterSession, 50)
	for i := range sessions {
		closedAt := now.Add(time.Duration(i) * time.Hour)
		sessions[i] = CashRegisterSession{
			BusinessID:        business.ID,
			Status:            CashRegisterSessionStatusClosed,
			OpeningFloatCents: 20000,
			ExpectedCashCents: 147490,
			CountedCashCents:  147307,
			VarianceCents:     -183,
			OpenedByLabel:     "cashier",
			OpenedAt:          closedAt.Add(-8 * time.Hour),
			ClosedByLabel:     "cashier",
			ClosedAt:          &closedAt,
		}
	}
	if err := gormDB.CreateInBatches(&sessions, 50).Error; err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cents, err := FindLastDeclaredOpeningFloatCentsTx(gormDB, business.ID)
		if err != nil {
			b.Fatal(err)
		}
		if cents == nil || *cents != 20000 {
			b.Fatalf("expected declared float 20000, got %v", cents)
		}
	}
}

// BenchmarkListUnassignedCashAlternativePayments measures the bounded list query
// against a business with many unassigned cash tenders. It returns at most
// `limit` rows regardless of how many exist, so the payload stays O(limit).
func BenchmarkListUnassignedCashAlternativePayments(b *testing.B) {
	gormDB, business := setupCashRegisterBenchmarkDB(b, false)
	bills := seedBenchmarkBills(b, gormDB, business, 500)
	// One confirmed cash tender per bill, none assigned → all unassigned.
	payments := make([]AlternativePayment, len(bills))
	for i, bill := range bills {
		payments[i] = AlternativePayment{
			BillID:          bill.ID,
			ParticipantAddr: fmt.Sprintf("payer-%d", i),
			Amount:          1000,
			PaymentMethod:   PaymentMethodCash,
			Status:          AltPaymentStatusConfirmed,
		}
	}
	if err := gormDB.CreateInBatches(&payments, 500).Error; err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		items, total, err := ListUnassignedCashAlternativePayments(gormDB, business.ID, 50, 0)
		if err != nil {
			b.Fatal(err)
		}
		if len(items) != 50 || total != 500 {
			b.Fatalf("expected 50 of 500, got %d of %d", len(items), total)
		}
	}
}
