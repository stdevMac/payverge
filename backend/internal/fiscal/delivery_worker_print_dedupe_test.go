package fiscal

import (
	"context"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// printOnlyDispatcher counts EnqueueReceiptPrint calls; other channels are
// unused by execPrint.
type printOnlyDispatcher struct {
	ReceiptDispatcher
	prints int
}

func (d *printOnlyDispatcher) EnqueueReceiptPrint(ctx context.Context, businessID, billID uint, language string) error {
	d.prints++
	return nil
}

// A courtesy receipt print job created by the payment flow must NOT suppress
// the fiscal print: courtesy HTML is rendered at enqueue time, BEFORE the CAE
// exists, so it carries no fiscal data. Only a prior fiscal-delivery print
// counts as "already printed".
func TestExecPrintNotSuppressedByCourtesyReceiptJob(t *testing.T) {
	db := newFiscalTestDB(t)
	if err := db.AutoMigrate(&database.PrintJob{}); err != nil {
		t.Fatal(err)
	}

	bill := database.Bill{BusinessID: 1, BillNumber: "DEDUP-1", Status: database.BillStatusPaid,
		SettlementAddr: "0xs", TippingAddr: "0xt"}
	if err := db.Create(&bill).Error; err != nil {
		t.Fatal(err)
	}
	// Courtesy job from the payment-confirmation flow (NOT fiscal-delivery).
	if err := db.Create(&database.PrintJob{
		BusinessID: bill.BusinessID, Kind: database.PrintJobKindReceipt,
		SourceType: "bill", SourceID: bill.ID,
		Status: database.PrintJobStatusPending, CreatedBy: "system",
	}).Error; err != nil {
		t.Fatal(err)
	}

	disp := &printOnlyDispatcher{}
	w := NewDeliveryWorker(db, nil, disp)
	jobCtx := &JobContext{Bill: bill, Settings: database.BusinessFiscalSettings{Country: "AR"}}
	receipt := &database.FiscalReceipt{BillID: bill.ID, BusinessID: bill.BusinessID}

	if err := w.execPrint(context.Background(), receipt, jobCtx, database.FiscalDeliveryTask{}); err != nil {
		t.Fatal(err)
	}
	if disp.prints != 1 {
		t.Fatalf("fiscal print suppressed by courtesy job: prints = %d, want 1", disp.prints)
	}
}

// A prior fiscal-delivery print job still dedupes (retry/lease-expiry re-run
// must not print a second fiscal ticket).
func TestExecPrintStillDedupesFiscalDeliveryJobs(t *testing.T) {
	db := newFiscalTestDB(t)
	if err := db.AutoMigrate(&database.PrintJob{}); err != nil {
		t.Fatal(err)
	}
	bill := database.Bill{BusinessID: 1, BillNumber: "DEDUP-2", Status: database.BillStatusPaid,
		SettlementAddr: "0xs", TippingAddr: "0xt"}
	if err := db.Create(&bill).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.PrintJob{
		BusinessID: bill.BusinessID, Kind: database.PrintJobKindReceipt,
		SourceType: "bill", SourceID: bill.ID,
		Status: database.PrintJobStatusPending, CreatedBy: "fiscal-delivery",
	}).Error; err != nil {
		t.Fatal(err)
	}

	disp := &printOnlyDispatcher{}
	w := NewDeliveryWorker(db, nil, disp)
	jobCtx := &JobContext{Bill: bill, Settings: database.BusinessFiscalSettings{Country: "AR"}}
	receipt := &database.FiscalReceipt{BillID: bill.ID, BusinessID: bill.BusinessID}

	if err := w.execPrint(context.Background(), receipt, jobCtx, database.FiscalDeliveryTask{}); err != nil {
		t.Fatal(err)
	}
	if disp.prints != 0 {
		t.Fatalf("fiscal-delivery dedupe broken: prints = %d, want 0", disp.prints)
	}
}
