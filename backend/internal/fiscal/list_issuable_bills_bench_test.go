package fiscal

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

func setupListIssuableBillsBenchDB(b *testing.B, n int) *Repository {
	b.Helper()
	dsn := fmt.Sprintf("file:fiscal-list-issuable-bench-%d?mode=memory&cache=shared", n)
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
		&database.Table{},
		&database.Bill{},
		&database.BusinessFiscalSettings{},
		&database.FiscalReceipt{},
	); err != nil {
		b.Fatalf("auto-migrate: %v", err)
	}

	if err := db.Create(&database.Business{
		ID: 1, BusinessId: "biz-issuable-bench", Name: "Bench Cafe",
		OwnerAddress: "0xbench", SettlementAddr: "settle", TippingAddr: "tip",
		DefaultCurrency: "ARS",
	}).Error; err != nil {
		b.Fatalf("seed business: %v", err)
	}

	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	bills := make([]database.Bill, 0, n)
	receipts := make([]database.FiscalReceipt, 0, n/4)
	for i := 0; i < n; i++ {
		closed := base.Add(time.Duration(i) * time.Minute)
		switch i % 4 {
		case 0:
			bills = append(bills, database.Bill{
				BusinessID: 1, BillNumber: fmt.Sprintf("B-PAID-%d", i),
				Status: database.BillStatusPaid, Items: "[]",
				TotalAmount: int64(1000 + i), PaidAmount: int64(1000 + i),
				ClosedAt: &closed,
			})
		case 1:
			bills = append(bills, database.Bill{
				BusinessID: 1, BillNumber: fmt.Sprintf("B-OPEN-PAID-%d", i),
				Status: database.BillStatusOpen, Items: "[]",
				TotalAmount: int64(1100 + i), PaidAmount: int64(1100 + i),
				ClosedAt: &closed,
			})
		case 2:
			bills = append(bills, database.Bill{
				BusinessID: 1, BillNumber: fmt.Sprintf("B-INVOICED-%d", i),
				Status: database.BillStatusClosed, Items: "[]",
				TotalAmount: int64(1200 + i), PaidAmount: int64(1200 + i),
				ClosedAt: &closed,
			})
		default:
			bills = append(bills, database.Bill{
				BusinessID: 1, BillNumber: fmt.Sprintf("B-NOISE-%d", i),
				Status: database.BillStatusOpen, Items: "[]",
				TotalAmount: int64(900 + i), PaidAmount: 0,
			})
		}
	}
	if err := db.CreateInBatches(&bills, 200).Error; err != nil {
		b.Fatalf("seed bills: %v", err)
	}

	var invoiced []database.Bill
	if err := db.Where("business_id = ? AND bill_number LIKE ?", 1, "B-INVOICED-%").
		Find(&invoiced).Error; err != nil {
		b.Fatalf("reload invoiced: %v", err)
	}
	for _, bill := range invoiced {
		receipts = append(receipts, database.FiscalReceipt{
			BusinessID: 1, SettingsID: 1, BillID: bill.ID,
			Country: "AR", Provider: "arca", Action: ActionIssueReceipt,
			ReceiptType: "B", TotalAmountCents: bill.TotalAmount, Currency: "ARS",
			Status: database.FiscalStatusAuthorized,
		})
	}
	if len(receipts) > 0 {
		if err := db.CreateInBatches(&receipts, 200).Error; err != nil {
			b.Fatalf("seed receipts: %v", err)
		}
	}

	return NewRepository(db)
}

// listIssuableBillsLegacyPaidAndAmount is the pre-change picker: paid AND
// paid_amount>0, plus GORM IN ? for blocking receipt statuses.
func listIssuableBillsLegacyPaidAndAmount(db *gorm.DB, businessID uint, limit int) ([]IssuableBill, error) {
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	type issuableRow struct {
		BillID            uint
		BillNumber        string
		TableLabel        string
		ClosedAt          *time.Time
		TotalCents        int64
		Currency          string
		ExistingReceiptID *uint
	}
	blocking := []database.FiscalStatus{
		database.FiscalStatusPending,
		database.FiscalStatusAuthorized,
		database.FiscalStatusCredited,
	}
	var rows []issuableRow
	err := db.Table("bills").
		Select(`
			bills.id AS bill_id,
			bills.bill_number AS bill_number,
			COALESCE(tables.name, '') AS table_label,
			bills.closed_at AS closed_at,
			bills.total_amount AS total_cents,
			COALESCE(NULLIF(TRIM(businesses.default_currency), ''), 'ARS') AS currency,
			(
				SELECT r.id FROM fiscal_receipts r
				WHERE r.bill_id = bills.id
				  AND r.action = ?
				  AND r.status IN ?
				ORDER BY r.id DESC
				LIMIT 1
			) AS existing_receipt_id
		`, ActionIssueReceipt, blocking).
		Joins("LEFT JOIN tables ON tables.id = bills.table_id AND tables.business_id = bills.business_id").
		Joins("LEFT JOIN businesses ON businesses.id = bills.business_id").
		Where("bills.business_id = ? AND bills.status = ? AND bills.paid_amount > 0",
			businessID, database.BillStatusPaid).
		Order("bills.closed_at DESC, bills.id DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]IssuableBill, len(rows))
	for i, row := range rows {
		out[i] = IssuableBill{
			BillID:            row.BillID,
			BillNumber:        row.BillNumber,
			TableLabel:        row.TableLabel,
			ClosedAt:          row.ClosedAt,
			TotalAmount:       float64(row.TotalCents) / 100.0,
			Currency:          row.Currency,
			ExistingReceiptID: row.ExistingReceiptID,
		}
	}
	return out, nil
}

func BenchmarkListIssuableBillsLegacyPaidAndAmount(b *testing.B) {
	const seedN = 400
	repo := setupListIssuableBillsBenchDB(b, seedN)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := listIssuableBillsLegacyPaidAndAmount(repo.db, 1, 20)
		if err != nil {
			b.Fatal(err)
		}
		if len(rows) != 20 {
			b.Fatalf("legacy want 20 paid rows, got %d", len(rows))
		}
		if !strings.HasPrefix(rows[0].BillNumber, "B-PAID-") {
			b.Fatalf("legacy should only return status=paid, got %q", rows[0].BillNumber)
		}
	}
}

func BenchmarkListIssuableBills(b *testing.B) {
	const seedN = 400
	repo := setupListIssuableBillsBenchDB(b, seedN)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := repo.ListIssuableBills(1, "", 20)
		if err != nil {
			b.Fatal(err)
		}
		if len(rows) != 20 {
			b.Fatalf("want 20 picker rows, got %d", len(rows))
		}
	}
}
