package handlers

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func setupPluginHandlersPrintTestDB(t *testing.T) (*gorm.DB, *database.Business, *database.Bill) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_busy_timeout=5000"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(
		&database.Business{}, &database.Table{}, &database.Bill{},
		&database.Printer{}, &database.PrintJob{}, &database.Payment{},
		&database.OperationalAlert{}, &database.OperationalAlertEvent{},
		&database.BusinessFiscalSettings{}, &database.FiscalReceipt{}, &database.FiscalJob{},
	))
	business := &database.Business{
		BusinessId: "biz-" + t.Name(), OwnerAddress: "0xowner",
		Name: "Test Bistro", SettlementAddr: "0xsettle", TippingAddr: "0xtip",
	}
	require.NoError(t, db.Create(business).Error)
	table := &database.Table{
		BusinessID: business.ID, TableCode: "tbl-" + t.Name(),
		Name: "Table 1",
	}
	require.NoError(t, db.Create(table).Error)
	bill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     "BN-" + t.Name(),
		SettlementAddr: "0xsettle",
		TippingAddr:    "0xtip",
		Status:         database.BillStatusPaid,
		Subtotal:       1000,
		TotalAmount:    1000,
		PaidAmount:     1000,
	}
	require.NoError(t, db.Create(bill).Error)
	database.SetTestDB(db)
	return db, business, bill
}

func TestEnqueueReceiptOnPaid_CreatesJob(t *testing.T) {
	db, business, bill := setupPluginHandlersPrintTestDB(t)

	locID := uint(2)
	printer := database.Printer{
		BusinessID:   business.ID,
		LocationID:   &locID,
		Name:         "front",
		Role:         "bill",
		Transport:    "browser",
		PaperWidthMM: 80,
		Enabled:      true,
	}
	require.NoError(t, db.Create(&printer).Error)

	enqueueReceiptForPaidBill(bill)

	var count int64
	db.Model(&database.PrintJob{}).
		Where("business_id = ? AND source_type = ? AND source_id = ? AND kind = ?",
			business.ID, "bill", bill.ID, database.PrintJobKindReceipt).
		Count(&count)
	require.GreaterOrEqual(t, count, int64(1), "expected at least one receipt PrintJob enqueued")
}

func TestEnqueueReceiptOnPaid_DeduplicatesRepeatedSettlementCallbacks(t *testing.T) {
	db, business, bill := setupPluginHandlersPrintTestDB(t)

	enqueueReceiptForPaidBill(bill)
	enqueueReceiptForPaidBill(bill)

	var count int64
	require.NoError(t, db.Model(&database.PrintJob{}).
		Where("business_id = ? AND source_type = ? AND source_id = ? AND kind = ?",
			business.ID, "bill", bill.ID, database.PrintJobKindReceipt).
		Count(&count).Error)
	require.Equal(t, int64(1), count, "duplicate payment confirmation must reuse the final receipt job")
}

func TestEnqueueReceiptOnPaid_PrinterQueueFailureDoesNotRollBackSettlement(t *testing.T) {
	db, business, bill := setupPluginHandlersPrintTestDB(t)
	require.NoError(t, db.Migrator().DropTable(&database.PrintJob{}))

	enqueueReceiptForPaidBill(bill)

	var reloaded database.Bill
	require.NoError(t, db.First(&reloaded, bill.ID).Error)
	require.Equal(t, database.BillStatusPaid, reloaded.Status)
	require.Equal(t, bill.PaidAmount, reloaded.PaidAmount)

	var alert database.OperationalAlert
	require.NoError(t, db.Where(
		"business_id = ? AND resource_type = ? AND resource_id = ?",
		business.ID, database.OperationalAlertResourceTypeBill, bill.ID,
	).First(&alert).Error)
	require.Equal(t, database.OperationalAlertTypePrintQueueStale, alert.AlertType)
	require.Contains(t, alert.Body, "Print it manually")
}

func TestEnqueueReceiptOnPaid_UsesBusinessDefaultLanguage(t *testing.T) {
	db, business, bill := setupPluginHandlersPrintTestDB(t)
	require.NoError(t, db.Model(&database.Business{}).Where("id = ?", business.ID).
		Update("default_language", "es-AR").Error)

	// Global printer (nil location) — auto-receipt enqueue does not pass a
	// location, so only location_id IS NULL printers are routeable.
	printer := database.Printer{
		BusinessID:   business.ID,
		Name:         "front",
		Role:         "bill",
		Transport:    "browser",
		PaperWidthMM: 80,
		Enabled:      true,
	}
	require.NoError(t, db.Create(&printer).Error)

	enqueueReceiptForPaidBill(bill)

	var job database.PrintJob
	require.NoError(t, db.Where(
		"business_id = ? AND source_type = ? AND source_id = ? AND kind = ?",
		business.ID, "bill", bill.ID, database.PrintJobKindReceipt,
	).First(&job).Error)
	require.Equal(t, "es-AR", job.Language,
		"auto-receipt must print in the business default language, not hardcoded en")
	require.NotNil(t, job.PayloadHTML)
	require.Contains(t, *job.PayloadHTML, "*** PAGADO ***",
		"rendered receipt HTML must carry Spanish labels")
	require.NotContains(t, *job.PayloadHTML, "*** PAID ***")
}

func TestEnqueueFiscalJobForPaidBill_CreatesJob(t *testing.T) {
	db, business, bill := setupPluginHandlersPrintTestDB(t)
	paymentID := uint(42)

	require.NoError(t, db.Create(&database.BusinessFiscalSettings{
		BusinessID:   business.ID,
		Country:      "AR",
		Provider:     "arca",
		Mode:         database.FiscalModeAutomaticNonBlocking,
		Environment:  "sandbox",
		SetupStatus:  "active",
		PointOfSale:  testIntPtr(1),
		TaxID:        "30711222333",
		TaxCondition: "responsable_inscripto",
	}).Error)
	enableFiscalRuntimeControl(t, db)

	enqueueFiscalJobForPaidBill(bill, &paymentID, "test")

	var job database.FiscalJob
	require.NoError(t, db.Where("business_id = ? AND bill_id = ?", business.ID, bill.ID).First(&job).Error)
	require.Equal(t, paymentID, *job.PaymentID)
	require.Equal(t, "system", job.CreatedBy)
}

func testIntPtr(value int) *int {
	return &value
}
