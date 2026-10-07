package paidreceipt

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func setupPaidReceiptTest(t *testing.T) (*gorm.DB, *database.Bill) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared&_busy_timeout=5000"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(
		&database.Business{},
		&database.Bill{},
		&database.Printer{},
		&database.PrintJob{},
		&database.OperationalAlert{},
		&database.OperationalAlertEvent{},
	))
	business := database.Business{
		BusinessId: "receipt-" + t.Name(),
		Name:       "Receipt Test",
	}
	require.NoError(t, db.Create(&business).Error)
	bill := &database.Bill{
		BusinessID:     business.ID,
		BillNumber:     "paid-" + t.Name(),
		Status:         database.BillStatusPaid,
		TotalAmount:    1500,
		PaidAmount:     1500,
		SettlementAddr: "0xsettle",
		TippingAddr:    "0xtip",
	}
	require.NoError(t, db.Create(bill).Error)
	return db, bill
}

func TestHandleBillPaid_RecordsHonestPendingStateAndDeduplicates(t *testing.T) {
	db, bill := setupPaidReceiptTest(t)
	svc := NewService(db)

	first, err := svc.HandleBillPaid(context.Background(), bill, "cash_confirmation")
	require.NoError(t, err)
	require.Equal(t, StatePendingPrinter, first.State)
	require.NotNil(t, first.Job)

	second, err := svc.HandleBillPaid(context.Background(), bill, "duplicate_confirmation")
	require.NoError(t, err)
	require.Equal(t, first.Job.ID, second.Job.ID)

	var count int64
	require.NoError(t, db.Model(&database.PrintJob{}).
		Where("business_id = ? AND kind = ? AND source_type = ? AND source_id = ?",
			bill.BusinessID, database.PrintJobKindReceipt, "bill", bill.ID).
		Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestHandleBillPaid_RefundThenRepayCreatesOneReceiptForTheNewPaidCycle(t *testing.T) {
	db, bill := setupPaidReceiptTest(t)
	svc := NewService(db)

	first, err := svc.HandleBillPaid(context.Background(), bill, "first_settlement")
	require.NoError(t, err)
	require.NotNil(t, first.Job)

	// A full refund/reversal reopens the same bill. A later legitimate payment
	// closes it again with a new cycle boundary and needs a new final receipt.
	require.NoError(t, db.Model(&database.Bill{}).Where("id = ?", bill.ID).Updates(map[string]interface{}{
		"status":      database.BillStatusOpen,
		"paid_amount": 0,
		"closed_at":   nil,
	}).Error)
	time.Sleep(2 * time.Millisecond)
	repaidAt := time.Now().UTC()
	require.NoError(t, db.Model(&database.Bill{}).Where("id = ?", bill.ID).Updates(map[string]interface{}{
		"status":      database.BillStatusPaid,
		"paid_amount": bill.TotalAmount,
		"closed_at":   &repaidAt,
	}).Error)
	require.NoError(t, db.First(bill, bill.ID).Error)

	second, err := svc.HandleBillPaid(context.Background(), bill, "repayment")
	require.NoError(t, err)
	require.NotNil(t, second.Job)
	require.NotEqual(t, first.Job.ID, second.Job.ID,
		"repayment after reversal must not reuse the prior paid cycle's printed receipt")

	replay, err := svc.HandleBillPaid(context.Background(), bill, "repayment_duplicate")
	require.NoError(t, err)
	require.Equal(t, second.Job.ID, replay.Job.ID,
		"concurrent/replayed producers in one paid cycle must still deduplicate")

	var count int64
	require.NoError(t, db.Model(&database.PrintJob{}).
		Where("business_id = ? AND kind = ? AND source_type = ? AND source_id = ?",
			bill.BusinessID, database.PrintJobKindReceipt, "bill", bill.ID).
		Count(&count).Error)
	require.Equal(t, int64(2), count)
}

func TestHandleBillPaid_EnqueueFailureAlertsAndExposesManualRecovery(t *testing.T) {
	db, bill := setupPaidReceiptTest(t)
	require.NoError(t, db.Migrator().DropTable(&database.PrintJob{}))

	result, err := NewService(db).HandleBillPaid(context.Background(), bill, "plugin_webhook")
	require.Error(t, err)
	require.Equal(t, StateManualPrintRequired, result.State)
	require.Contains(t, result.ManualRecoveryPath, "/print/jobs")

	var alert database.OperationalAlert
	require.NoError(t, db.Where(
		"business_id = ? AND resource_type = ? AND resource_id = ?",
		bill.BusinessID, database.OperationalAlertResourceTypeBill, bill.ID,
	).First(&alert).Error)
	require.Equal(t, database.OperationalAlertTypePrintQueueStale, alert.AlertType)
	require.Contains(t, alert.Title, "manual")
}
