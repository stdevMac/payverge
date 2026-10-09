package fiscal

import (
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupListReceiptsPageBenchDB seeds N receipts with one delivery task each.
func setupListReceiptsPageBenchDB(b *testing.B, n int) (*Repository, *gorm.DB) {
	b.Helper()
	dsn := fmt.Sprintf("file:fiscal-list-receipts-page-bench-%d?mode=memory&cache=shared", n)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
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
		&database.BusinessFiscalSettings{},
		&database.FiscalReceipt{},
		&database.FiscalJob{},
		&database.FiscalAuditEvent{},
		&database.FiscalDeliveryTask{},
	); err != nil {
		b.Fatalf("auto-migrate: %v", err)
	}

	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	receipts := make([]database.FiscalReceipt, n)
	for i := 0; i < n; i++ {
		receipts[i] = database.FiscalReceipt{
			BusinessID:       1,
			SettingsID:       1,
			BillID:           uint(i + 1),
			Country:          "AR",
			Provider:         "arca",
			Action:           ActionIssueReceipt,
			ReceiptType:      "B",
			TotalAmountCents: int64(1000 + i),
			Currency:         "ARS",
			Status:           database.FiscalStatusAuthorized,
			CreatedAt:        base.Add(time.Duration(i) * time.Minute),
			UpdatedAt:        base.Add(time.Duration(i) * time.Minute),
		}
	}
	if err := db.CreateInBatches(&receipts, 500).Error; err != nil {
		b.Fatalf("seed receipts: %v", err)
	}

	// Reload IDs (CreateInBatches may leave zero IDs depending on dialect).
	var seeded []database.FiscalReceipt
	if err := db.Where("business_id = ?", 1).Order("id ASC").Find(&seeded).Error; err != nil {
		b.Fatalf("reload receipts: %v", err)
	}
	tasks := make([]database.FiscalDeliveryTask, 0, len(seeded)*2)
	now := base.Add(24 * time.Hour)
	for _, r := range seeded {
		tasks = append(tasks,
			database.FiscalDeliveryTask{
				BusinessID: 1, ReceiptID: r.ID,
				Channel:     database.FiscalDeliveryChannelEmail,
				Status:      database.FiscalDeliveryStatusSucceeded,
				MaxAttempts: 8, IdempotencyKey: fmt.Sprintf("r:%d:email", r.ID),
				NextAttemptAt: &now, CreatedAt: r.CreatedAt, UpdatedAt: r.CreatedAt,
			},
			database.FiscalDeliveryTask{
				BusinessID: 1, ReceiptID: r.ID,
				Channel:     database.FiscalDeliveryChannelPrint,
				Status:      database.FiscalDeliveryStatusPending,
				MaxAttempts: 8, IdempotencyKey: fmt.Sprintf("r:%d:print", r.ID),
				NextAttemptAt: &now, CreatedAt: r.CreatedAt, UpdatedAt: r.CreatedAt,
			},
		)
	}
	if err := db.CreateInBatches(&tasks, 500).Error; err != nil {
		b.Fatalf("seed delivery tasks: %v", err)
	}

	return NewRepository(db), db
}

// listReceiptsPageNPlus1 is the pre-optimization shape: page receipts then one
// delivery query per row (the FE N+1 pattern mirrored server-side for baseline).
func listReceiptsPageNPlus1(db *gorm.DB, businessID uint, page, pageSize int) (*ReceiptsPage, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	var total int64
	if err := db.Model(&database.FiscalReceipt{}).
		Where("business_id = ?", businessID).
		Count(&total).Error; err != nil {
		return nil, err
	}
	var receipts []database.FiscalReceipt
	if err := db.Where("business_id = ?", businessID).
		Order("created_at DESC, id DESC").
		Limit(pageSize).
		Offset((page - 1) * pageSize).
		Find(&receipts).Error; err != nil {
		return nil, err
	}
	out := make([]ReceiptRow, len(receipts))
	for i, r := range receipts {
		var tasks []database.FiscalDeliveryTask
		if err := db.Where("receipt_id = ?", r.ID).Find(&tasks).Error; err != nil {
			return nil, err
		}
		badges := make([]ReceiptDeliveryBadge, len(tasks))
		for j, t := range tasks {
			badges[j] = ReceiptDeliveryBadge{TaskID: t.ID, Channel: t.Channel, Status: t.Status}
		}
		out[i] = ReceiptRow{FiscalReceipt: r, Delivery: badges}
	}
	totalPages := 0
	if total > 0 {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	return &ReceiptsPage{Receipts: out, Total: total, Page: page, PageSize: pageSize, TotalPages: totalPages}, nil
}

// BenchmarkListReceiptsPageNPlus1 baselines the N+1 delivery fetch pattern
// (page of 20 receipts × per-row delivery queries) against a 500-row fixture.
func BenchmarkListReceiptsPageNPlus1(b *testing.B) {
	const seedN = 500
	_, db := setupListReceiptsPageBenchDB(b, seedN)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		page, err := listReceiptsPageNPlus1(db, 1, 1, 20)
		if err != nil {
			b.Fatal(err)
		}
		if len(page.Receipts) != 20 {
			b.Fatalf("want 20 rows, got %d", len(page.Receipts))
		}
	}
}

// BenchmarkListReceiptsPage measures the batched delivery-embed page load.
func BenchmarkListReceiptsPage(b *testing.B) {
	const seedN = 500
	repo, _ := setupListReceiptsPageBenchDB(b, seedN)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		page, err := repo.ListReceiptsPage(ListReceiptsParams{
			BusinessID: 1, Page: 1, PageSize: 20,
		})
		if err != nil {
			b.Fatal(err)
		}
		if len(page.Receipts) != 20 {
			b.Fatalf("want 20 rows, got %d", len(page.Receipts))
		}
	}
}
