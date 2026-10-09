package print

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// mustCreateBill inserts a minimal valid Bill row and returns it.
//
// NOT NULL fields required by database.Bill:
//   - BusinessID  (index;not null)
//   - BillNumber  (uniqueIndex;not null)
//   - SettlementAddr (not null)
//   - TippingAddr    (not null)
//
// TableID must reference a real Table row because Bill.Table is preloaded
// in buildBillInputFromBill. We create a minimal Table row here.
func mustCreateBill(t *testing.T, db *gorm.DB, businessID uint) *database.Bill {
	t.Helper()

	// Create a Table row that the Bill can reference.
	tbl := &database.Table{
		BusinessID: businessID,
		TableCode:  fmt.Sprintf("tbl-%s", t.Name()),
		Name:       "T1",
	}
	if err := db.Create(tbl).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}

	b := &database.Bill{
		BusinessID:     businessID,
		TableID:        tbl.ID,
		BillNumber:     fmt.Sprintf("B-%s", t.Name()),
		SettlementAddr: "0x0000000000000000000000000000000000000010",
		TippingAddr:    "0x0000000000000000000000000000000000000011",
		Subtotal:       1000, // $10.00
		TaxAmount:      100,  // $1.00
		TotalAmount:    1100, // $11.00
	}
	if err := db.Create(b).Error; err != nil {
		t.Fatalf("create bill: %v", err)
	}
	return b
}

func TestService_EnqueueBillRendersHTMLAndRoutes(t *testing.T) {
	db := setupTestDB(t)
	business := mustCreateBusiness(t, db)
	bill := mustCreateBill(t, db, business.ID)

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
		t.Fatal(err)
	}

	svc := NewService(db)
	job, err := svc.Enqueue(context.Background(), EnqueueParams{
		BusinessID: business.ID,
		LocationID: &locID,
		Kind:       database.PrintJobKindBill,
		SourceType: "bill",
		SourceID:   bill.ID,
		Language:   "en",
		CreatedBy:  "system",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if job.Status != database.PrintJobStatusRouted {
		t.Fatalf("expected status routed, got %q", job.Status)
	}
	if job.PrinterID == nil || *job.PrinterID != printer.ID {
		t.Fatalf("expected printer_id %d, got %v", printer.ID, job.PrinterID)
	}
	if job.PayloadHTML == nil || *job.PayloadHTML == "" {
		t.Fatal("expected payload_html to be populated")
	}
}

func TestService_EnqueueStaysPending_WhenNoPrinter(t *testing.T) {
	db := setupTestDB(t)
	business := mustCreateBusiness(t, db)
	bill := mustCreateBill(t, db, business.ID)

	svc := NewService(db)
	job, err := svc.Enqueue(context.Background(), EnqueueParams{
		BusinessID: business.ID,
		Kind:       database.PrintJobKindBill,
		SourceType: "bill",
		SourceID:   bill.ID,
		Language:   "en",
		CreatedBy:  "system",
	})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if job.Status != database.PrintJobStatusPending {
		t.Fatalf("expected pending, got %q", job.Status)
	}
}

func TestService_RerouteAssignsPrinterToPendingJob(t *testing.T) {
	db := setupTestDB(t)
	business := mustCreateBusiness(t, db)
	bill := mustCreateBill(t, db, business.ID)
	svc := NewService(db)

	// Bill kind (not receipt) so renderHTML does not require a paid bill.
	pending, err := svc.Enqueue(context.Background(), EnqueueParams{
		BusinessID: business.ID,
		Kind:       database.PrintJobKindBill,
		SourceType: "bill",
		SourceID:   bill.ID,
		Language:   "en",
		CreatedBy:  "system",
	})
	requireNoErr(t, err)
	if pending.Status != database.PrintJobStatusPending || pending.PrinterID != nil {
		t.Fatalf("expected unassigned pending job, got status=%q printer=%v", pending.Status, pending.PrinterID)
	}

	printer := database.Printer{
		BusinessID: business.ID, Name: "front", Role: "bill",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	requireNoErr(t, db.Create(&printer).Error)

	routed, err := svc.Reroute(context.Background(), pending.ID, "operator")
	requireNoErr(t, err)
	if routed.Status != database.PrintJobStatusRouted {
		t.Fatalf("expected routed, got %q", routed.Status)
	}
	if routed.PrinterID == nil || *routed.PrinterID != printer.ID {
		t.Fatalf("expected printer %d, got %v", printer.ID, routed.PrinterID)
	}
}

func TestService_MarkPrintedTransitionsStatus(t *testing.T) {
	db := setupTestDB(t)
	business := mustCreateBusiness(t, db)
	job := database.PrintJob{
		BusinessID: business.ID,
		Kind:       database.PrintJobKindBill,
		SourceType: "bill",
		SourceID:   1,
		Status:     database.PrintJobStatusPrinting,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewService(db)
	if err := svc.MarkPrinted(context.Background(), job.ID, "0xabc"); err != nil {
		t.Fatalf("MarkPrinted: %v", err)
	}
	var reloaded database.PrintJob
	if err := db.First(&reloaded, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != database.PrintJobStatusPrinted {
		t.Fatalf("expected printed, got %q", reloaded.Status)
	}
	if reloaded.PrintedAt == nil {
		t.Fatal("expected PrintedAt to be set")
	}
}

// TestBuildBillInput_RejectsCrossTenantBill verifies defense-in-depth:
// buildBillInputFromBill includes a WHERE business_id = ? clause. Feed it
// a bill owned by bizB but pass bizA's businessID — it must fail to load
// the bill. Removing the WHERE clause causes this test to FAIL (the bill
// loads and the victim business name appears in the output). (PRINT-XTENANT-SOURCEID-1)
func TestBuildBillInput_RejectsCrossTenantBill(t *testing.T) {
	db := setupTestDB(t)
	bizA := mustCreateBusiness(t, db)

	bizB := &database.Business{
		BusinessId:     "bizB-" + t.Name(),
		OwnerAddress:   "0x00000000000000000000000000000000000000B0",
		Name:           "BizB",
		SettlementAddr: "0x00000000000000000000000000000000000000B1",
		TippingAddr:    "0x00000000000000000000000000000000000000B2",
	}
	requireNoErr(t, db.Create(bizB).Error)

	billB := &database.Bill{
		BusinessID:     bizB.ID,
		TableID:        1,
		BillNumber:     "B-cross-tenant",
		SettlementAddr: "0x0000000000000000000000000000000000000010",
		TippingAddr:    "0x0000000000000000000000000000000000000011",
		Subtotal:       2000,
		TotalAmount:    2000,
	}
	requireNoErr(t, db.Create(billB).Error)

	// Try to load billB using bizA's businessID — must fail.
	_, err := buildBillInputFromBill(context.Background(), db, billB.ID, bizA.ID, "en")
	assertErr(t, err, "cross-tenant bill load must fail with business_id scope")
}

// TestBuildReceiptInput_RejectsCrossTenantBill is the receipt counterpart.
func TestBuildReceiptInput_RejectsCrossTenantBill(t *testing.T) {
	db := setupTestDB(t)
	bizA := mustCreateBusiness(t, db)

	bizB := &database.Business{
		BusinessId:     "bizB-" + t.Name(),
		OwnerAddress:   "0x00000000000000000000000000000000000000B0",
		Name:           "BizB",
		SettlementAddr: "0x00000000000000000000000000000000000000B1",
		TippingAddr:    "0x00000000000000000000000000000000000000B2",
	}
	requireNoErr(t, db.Create(bizB).Error)

	billB := &database.Bill{
		BusinessID:     bizB.ID,
		TableID:        1,
		BillNumber:     "B-cross-rcpt",
		SettlementAddr: "0x0000000000000000000000000000000000000010",
		TippingAddr:    "0x0000000000000000000000000000000000000011",
		Subtotal:       2000,
		TotalAmount:    2000,
	}
	requireNoErr(t, db.Create(billB).Error)

	_, err := buildReceiptInputFromBill(context.Background(), db, billB.ID, bizA.ID, "en")
	assertErr(t, err, "cross-tenant receipt load must fail with business_id scope")
}

// assertErr fails the test when err is nil.
func assertErr(t *testing.T, err error, msg string) {
	t.Helper()
	if err == nil {
		t.Fatal(msg)
	}
}

// requireNoErr fails the test when err is non-nil.
func requireNoErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestService_Reprint_ForbiddenForKitchenKinds(t *testing.T) {
	db := setupTestDB(t)
	business := mustCreateBusiness(t, db)
	job := database.PrintJob{
		BusinessID: business.ID,
		Kind:       database.PrintJobKindKitchen,
		SourceType: "order",
		SourceID:   1,
		Status:     database.PrintJobStatusPrinted,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewService(db)
	if _, err := svc.Reprint(context.Background(), job.ID, "0xabc"); err == nil {
		t.Fatal("expected error for reprint of kitchen kind")
	}
}

func TestService_Reprint_ReceiptCreatesDistinctJobs(t *testing.T) {
	db := setupTestDB(t)
	business := mustCreateBusiness(t, db)
	bill := mustCreateBill(t, db, business.ID)
	closedAt := time.Now().UTC()
	requireNoErr(t, db.Model(bill).Updates(map[string]interface{}{
		"status":      database.BillStatusPaid,
		"paid_amount": bill.TotalAmount,
		"closed_at":   &closedAt,
	}).Error)
	original := database.PrintJob{
		BusinessID: business.ID,
		Kind:       database.PrintJobKindReceipt,
		SourceType: "bill",
		SourceID:   bill.ID,
		Status:     database.PrintJobStatusFailedPermanent,
	}
	requireNoErr(t, db.Create(&original).Error)

	svc := NewService(db)
	first, err := svc.Reprint(context.Background(), original.ID, "operator")
	requireNoErr(t, err)
	second, err := svc.Reprint(context.Background(), original.ID, "operator")
	requireNoErr(t, err)

	if first.ID == original.ID || second.ID == original.ID || first.ID == second.ID {
		t.Fatalf(
			"manual reprints must create distinct jobs: original=%d first=%d second=%d",
			original.ID,
			first.ID,
			second.ID,
		)
	}
	if first.SourceType != "reprint" || second.SourceType != "reprint" {
		t.Fatalf(
			"manual reprints must bypass auto-receipt dedupe: first=%q second=%q",
			first.SourceType,
			second.SourceType,
		)
	}
	if first.SourceID != original.SourceID || second.SourceID != original.SourceID {
		t.Fatalf("manual reprints must preserve the source bill id")
	}
}

func TestService_Reprint_RejectsNonterminalReceiptJobs(t *testing.T) {
	for _, status := range []database.PrintJobStatus{
		database.PrintJobStatusPending,
		database.PrintJobStatusRouted,
		database.PrintJobStatusPrinting,
		database.PrintJobStatusFailedRetryable,
	} {
		t.Run(string(status), func(t *testing.T) {
			db := setupTestDB(t)
			business := mustCreateBusiness(t, db)
			job := database.PrintJob{
				BusinessID: business.ID,
				Kind:       database.PrintJobKindReceipt,
				SourceType: "bill",
				SourceID:   42,
				Status:     status,
			}
			requireNoErr(t, db.Create(&job).Error)

			_, err := NewService(db).Reprint(context.Background(), job.ID, "operator")
			if err == nil {
				t.Fatalf("status %q: expected nonterminal reprint rejection", status)
			}
		})
	}
}

// TestService_EnqueueKitchenTicket_DedupesPerOrder guards the at-most-one
// kitchen ticket per order invariant. The approve-time producer and the orphan
// sweep both check-then-insert; a second enqueue for the same order must no-op
// back the existing job instead of creating a duplicate ticket.
func TestService_EnqueueKitchenTicket_DedupesPerOrder(t *testing.T) {
	db := setupTestDB(t)
	business := mustCreateBusiness(t, db)
	orderID := uint(4242)
	svc := NewService(db)

	params := EnqueueParams{
		BusinessID: business.ID,
		Kind:       database.PrintJobKindKitchen,
		SourceType: "order",
		SourceID:   orderID,
		OrderID:    &orderID,
		CreatedBy:  "order_approve",
	}

	first, err := svc.Enqueue(context.Background(), params)
	if err != nil {
		t.Fatalf("first enqueue: %v", err)
	}

	// Second producer (e.g. orphan sweep) racing on the same order.
	params.CreatedBy = "orphan_sweep"
	second, err := svc.Enqueue(context.Background(), params)
	if err != nil {
		t.Fatalf("second enqueue: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("second enqueue created a NEW job id=%d (first id=%d); expected the existing one", second.ID, first.ID)
	}

	var count int64
	if err := db.Model(&database.PrintJob{}).
		Where("order_id = ? AND kind IN ?", orderID, kitchenTicketKinds).
		Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 kitchen ticket for the order, got %d", count)
	}
}

// TestService_MarkPrinted_RejectsTerminalJob guards the transition-source guard:
// a job that already cancelled must not be flipped to printed (re-stamping
// printed_at), and the caller receives ErrIllegalTransition to surface a 409.
func TestService_MarkPrinted_RejectsTerminalJob(t *testing.T) {
	db := setupTestDB(t)
	business := mustCreateBusiness(t, db)
	job := database.PrintJob{
		BusinessID: business.ID,
		Kind:       database.PrintJobKindBill,
		SourceType: "bill",
		SourceID:   1,
		Status:     database.PrintJobStatusCancelled,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewService(db)
	if err := svc.MarkPrinted(context.Background(), job.ID, "0xabc"); !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("expected ErrIllegalTransition marking a cancelled job printed, got %v", err)
	}
	var reloaded database.PrintJob
	if err := db.First(&reloaded, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != database.PrintJobStatusCancelled {
		t.Fatalf("cancelled job status changed to %q", reloaded.Status)
	}
	if reloaded.PrintedAt != nil {
		t.Fatal("printed_at should not be stamped on a cancelled job")
	}
}

// TestService_Cancel_RejectsPrintedJob guards the reverse: an already-printed
// job cannot be cancelled out from under its terminal state.
func TestService_Cancel_RejectsPrintedJob(t *testing.T) {
	db := setupTestDB(t)
	business := mustCreateBusiness(t, db)
	job := database.PrintJob{
		BusinessID: business.ID,
		Kind:       database.PrintJobKindBill,
		SourceType: "bill",
		SourceID:   1,
		Status:     database.PrintJobStatusPrinted,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewService(db)
	if err := svc.Cancel(context.Background(), job.ID, "0xabc"); !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("expected ErrIllegalTransition cancelling a printed job, got %v", err)
	}
	var reloaded database.PrintJob
	if err := db.First(&reloaded, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if reloaded.Status != database.PrintJobStatusPrinted {
		t.Fatalf("printed job status changed to %q", reloaded.Status)
	}
}
