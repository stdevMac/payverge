package accounting

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// seedPayrollDetailBench builds one payroll run with nLines staff payees and
// created/paid/voided actor staff for GetPayrollRun + JSON microbenchmarks.
func seedPayrollDetailBench(tb testing.TB, nLines int) (businessID, runID uint) {
	tb.Helper()

	dsn := fmt.Sprintf("file:bench_payroll_detail_%d_%d?mode=memory", nLines, time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		tb.Fatal(err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		tb.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(gormDB)
	if err := gormDB.AutoMigrate(
		&database.Business{},
		&database.Staff{},
		&database.PayrollRun{},
		&database.PayrollLineItem{},
	); err != nil {
		tb.Fatal(err)
	}

	biz := &database.Business{
		BusinessId:      fmt.Sprintf("bench-pr-detail-%d", nLines),
		Name:            "BenchPayrollDetail",
		OwnerAddress:    fmt.Sprintf("0xBenchPR%d", nLines),
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
		DisplayCurrency: "USD",
		DefaultLanguage: "en",
		SourceLanguage:  "en",
	}
	if err := gormDB.Create(biz).Error; err != nil {
		tb.Fatal(err)
	}

	mkStaff := func(email, name string) *database.Staff {
		s := &database.Staff{
			BusinessID: biz.ID,
			Email:      email,
			Name:       name,
			Role:       database.StaffRoleManager,
			IsActive:   true,
			InvitedBy:  "bench",
		}
		if err := gormDB.Create(s).Error; err != nil {
			tb.Fatal(err)
		}
		return s
	}
	createdBy := mkStaff("creator@bench.test", "Creator")
	paidBy := mkStaff("paid@bench.test", "Payer")
	voidedBy := mkStaff("void@bench.test", "Voider")

	paidAt := time.Date(2026, time.March, 16, 12, 0, 0, 0, time.UTC)
	voidedAt := time.Date(2026, time.March, 17, 9, 0, 0, 0, time.UTC)
	run := &database.PayrollRun{
		BusinessID:       biz.ID,
		PeriodStart:      time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:        time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC),
		Status:           database.PayrollRunStatusVoid,
		Currency:         "USD",
		GrossTotal:       int64(10000 * nLines),
		BonusTotal:       int64(500 * nLines),
		DeductionTotal:   int64(200 * nLines),
		NetTotal:         int64(10300 * nLines),
		CreatedByStaffID: &createdBy.ID,
		PaidAt:           &paidAt,
		PaidByStaffID:    &paidBy.ID,
		VoidedAt:         &voidedAt,
		VoidedByStaffID:  &voidedBy.ID,
	}
	if err := gormDB.Create(run).Error; err != nil {
		tb.Fatal(err)
	}

	for i := 0; i < nLines; i++ {
		payee := mkStaff(fmt.Sprintf("payee%d@bench.test", i), fmt.Sprintf("Payee %d", i))
		sid := payee.ID
		if err := gormDB.Create(&database.PayrollLineItem{
			PayrollRunID:    run.ID,
			BusinessID:      biz.ID,
			PayeeType:       database.PayrollPayeeTypeStaff,
			StaffID:         &sid,
			PayeeName:       payee.Name,
			GrossAmount:     10000,
			BonusAmount:     500,
			DeductionAmount: 200,
			NetAmount:       10300,
		}).Error; err != nil {
			tb.Fatal(err)
		}
	}

	return biz.ID, run.ID
}

// TestGetPayrollRunDetailBodySizeProbe records service-level JSON body sizes for
// 5-line and 50-line runs (summary.md evidence). Not a regression gate — B-10 is.
func TestGetPayrollRunDetailBodySizeProbe(t *testing.T) {
	for _, n := range []int{5, 50} {
		n := n
		t.Run(fmt.Sprintf("%d_lines", n), func(t *testing.T) {
			businessID, runID := seedPayrollDetailBench(t, n)
			service := NewService(database.GetDBWrapper())
			run, err := service.GetPayrollRun(businessID, runID)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(run)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("GetPayrollRun JSON body size (%d lines): %d bytes", n, len(raw))
		})
	}
}

// BenchmarkGetPayrollRunDetail_5Lines measures GetPayrollRun + JSON marshal for
// a 5-line run (audit drawer shape). L6-16 baseline ~32.6 KB body.
// Run: go test ./internal/accounting/ -bench BenchmarkGetPayrollRunDetail -benchmem -count=3 -run '^$'
func BenchmarkGetPayrollRunDetail_5Lines(b *testing.B) {
	businessID, runID := seedPayrollDetailBench(b, 5)
	service := NewService(database.GetDBWrapper())

	// Warm + capture body size once outside the timed loop.
	run, err := service.GetPayrollRun(businessID, runID)
	if err != nil {
		b.Fatal(err)
	}
	raw, err := json.Marshal(run)
	if err != nil {
		b.Fatal(err)
	}
	bodyBytes := float64(len(raw))

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		run, err := service.GetPayrollRun(businessID, runID)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := json.Marshal(run); err != nil {
			b.Fatal(err)
		}
		b.ReportMetric(bodyBytes, "body_B")
	}
}

// BenchmarkGetPayrollRunDetail_50Lines measures GetPayrollRun + JSON marshal for
// a 50-line run (dense payroll). Body scales with empty relation recursion pre-fix.
func BenchmarkGetPayrollRunDetail_50Lines(b *testing.B) {
	businessID, runID := seedPayrollDetailBench(b, 50)
	service := NewService(database.GetDBWrapper())

	run, err := service.GetPayrollRun(businessID, runID)
	if err != nil {
		b.Fatal(err)
	}
	raw, err := json.Marshal(run)
	if err != nil {
		b.Fatal(err)
	}
	bodyBytes := float64(len(raw))

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		run, err := service.GetPayrollRun(businessID, runID)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := json.Marshal(run); err != nil {
			b.Fatal(err)
		}
		b.ReportMetric(bodyBytes, "body_B")
	}
}
