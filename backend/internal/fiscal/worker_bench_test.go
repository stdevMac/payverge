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

// noopAuthorizedProvider returns an authorized result with no network or work, so
// the benchmark measures the worker's claim → load → persist → audit cost rather
// than provider latency.
type noopAuthorizedProvider struct{}

func (noopAuthorizedProvider) Country() string { return "AR" }
func (noopAuthorizedProvider) Name() string    { return "arca" }
func (noopAuthorizedProvider) ValidateSettings(context.Context, Settings) error {
	return nil
}
func (noopAuthorizedProvider) IssueReceipt(context.Context, IssueInput) (*ReceiptResult, error) {
	return &ReceiptResult{
		Status:            StatusAuthorized,
		ProviderReceiptID: "71000000000001",
		AuthCode:          "71000000000001",
		ReceiptNumber:     "1",
		QRPayload:         "https://www.afip.gob.ar/fe/qr/?p=abc",
		ReceiptType:       "factura_c",
	}, nil
}
func (noopAuthorizedProvider) IssueCreditNote(context.Context, CreditNoteInput) (*ReceiptResult, error) {
	return &ReceiptResult{Status: StatusAuthorized}, nil
}
func (noopAuthorizedProvider) GetStatus(context.Context, string) (*ReceiptStatus, error) {
	return &ReceiptStatus{Status: StatusAuthorized}, nil
}

func setupFiscalWorkerBenchDB(b *testing.B) *gorm.DB {
	b.Helper()
	db, err := gorm.Open(sqlite.Open("file:fiscal-worker-bench?mode=memory&cache=shared"), &gorm.Config{
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
	return db
}

// seedWorkerBenchJobs creates count paid bills + pending issue jobs for one
// business with ready fiscal settings.
func seedWorkerBenchJobs(b *testing.B, db *gorm.DB, count int) {
	b.Helper()
	business := database.Business{
		BusinessId:     "fiscal-worker-bench",
		OwnerAddress:   "owner",
		Name:           "Fiscal Worker Bench",
		SettlementAddr: "settlement",
		TippingAddr:    "tipping",
	}
	if err := db.Create(&business).Error; err != nil {
		b.Fatalf("create business: %v", err)
	}
	pos := 3
	settings := database.BusinessFiscalSettings{
		BusinessID:   business.ID,
		Country:      "AR",
		Provider:     "arca",
		Mode:         database.FiscalModeAutomaticNonBlocking,
		Environment:  "sandbox",
		TaxID:        "20123456789",
		TaxCondition: "monotributo",
		PointOfSale:  &pos,
		SetupStatus:  "ready",
	}
	if err := db.Create(&settings).Error; err != nil {
		b.Fatalf("create settings: %v", err)
	}

	bills := make([]database.Bill, count)
	for i := range bills {
		bills[i] = database.Bill{
			BusinessID:  business.ID,
			BillNumber:  fmt.Sprintf("FISCAL-WORKER-BENCH-%06d", i),
			Status:      database.BillStatusPaid,
			Items:       "[]",
			TotalAmount: 12100,
			PaidAmount:  12100,
		}
	}
	if err := db.CreateInBatches(&bills, 500).Error; err != nil {
		b.Fatalf("create bills: %v", err)
	}

	jobs := make([]database.FiscalJob, count)
	for i := range jobs {
		jobs[i] = database.FiscalJob{
			BusinessID:     business.ID,
			SettingsID:     settings.ID,
			BillID:         bills[i].ID,
			Action:         ActionIssueReceipt,
			IdempotencyKey: fmt.Sprintf("bench:issue:%06d", i),
			Status:         database.FiscalStatusPending,
			MaxAttempts:    5,
			CreatedBy:      "system",
		}
	}
	if err := db.CreateInBatches(&jobs, 500).Error; err != nil {
		b.Fatalf("create jobs: %v", err)
	}
}

func BenchmarkFiscalWorkerProcessDue(b *testing.B) {
	db := setupFiscalWorkerBenchDB(b)
	seedWorkerBenchJobs(b, db, b.N)

	w := NewFiscalWorker(db, &fakeFactory{provider: noopAuthorizedProvider{}})
	w.WorkerID = "bench-worker"
	// Claim the entire backlog in a single sweep so the per-op cost reflects one
	// claimed job's full load → persist → audit path.
	w.BatchSize = b.N + 1
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()

	processed := 0
	for processed < b.N {
		n, err := w.ProcessDue(ctx)
		if err != nil {
			b.Fatalf("process due: %v", err)
		}
		if n == 0 {
			b.Fatalf("no jobs processed at %d/%d", processed, b.N)
		}
		processed += n
	}
}
