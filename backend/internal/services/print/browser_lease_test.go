package print

import (
	"context"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestQueue_ClaimBrowserJob_OnlyBrowserEnabledOldestFirst(t *testing.T) {
	db := setupTestDB(t)
	biz := mustCreateBusiness(t, db)
	q := NewQueue(db)
	ctx := context.Background()
	now := time.Now()

	// Disabled browser printer — jobs on it must not be claimable.
	disabled := database.Printer{
		BusinessID: biz.ID, Name: "off", Role: "bill",
		Transport: "browser", PaperWidthMM: 80, Enabled: false,
	}
	if err := db.Create(&disabled).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&disabled).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	// Enabled cloudprnt — must not be claimed by browser agent.
	cloud := database.Printer{
		BusinessID: biz.ID, Name: "cloud", Role: "bill",
		Transport: "cloudprnt", PaperWidthMM: 80, Enabled: true,
	}
	if err := db.Create(&cloud).Error; err != nil {
		t.Fatal(err)
	}
	// Enabled browser — claimable.
	browser := database.Printer{
		BusinessID: biz.ID, Name: "browser", Role: "bill",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	if err := db.Create(&browser).Error; err != nil {
		t.Fatal(err)
	}

	html := "<html>receipt</html>"
	older := database.PrintJob{
		BusinessID: biz.ID, PrinterID: &browser.ID, Kind: database.PrintJobKindReceipt,
		SourceType: "bill", SourceID: 1, Status: database.PrintJobStatusRouted,
		PayloadHTML: &html, MaxAttempts: 6,
	}
	if err := db.Create(&older).Error; err != nil {
		t.Fatal(err)
	}
	// Backdate so oldest-first is deterministic.
	if err := db.Model(&older).Update("created_at", now.Add(-2*time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	newer := database.PrintJob{
		BusinessID: biz.ID, PrinterID: &browser.ID, Kind: database.PrintJobKindReceipt,
		SourceType: "bill", SourceID: 2, Status: database.PrintJobStatusRouted,
		PayloadHTML: &html, MaxAttempts: 6,
	}
	if err := db.Create(&newer).Error; err != nil {
		t.Fatal(err)
	}
	// Noise jobs: disabled printer + cloudprnt.
	noise1 := database.PrintJob{
		BusinessID: biz.ID, PrinterID: &disabled.ID, Kind: database.PrintJobKindReceipt,
		SourceType: "bill", SourceID: 3, Status: database.PrintJobStatusRouted,
		PayloadHTML: &html, MaxAttempts: 6,
	}
	if err := db.Create(&noise1).Error; err != nil {
		t.Fatal(err)
	}
	noise2 := database.PrintJob{
		BusinessID: biz.ID, PrinterID: &cloud.ID, Kind: database.PrintJobKindReceipt,
		SourceType: "bill", SourceID: 4, Status: database.PrintJobStatusRouted,
		PayloadHTML: &html, MaxAttempts: 6,
	}
	if err := db.Create(&noise2).Error; err != nil {
		t.Fatal(err)
	}

	job, err := q.ClaimBrowserJob(ctx, biz.ID, "client-a", now, 30*time.Second)
	if err != nil {
		t.Fatalf("ClaimBrowserJob: %v", err)
	}
	if job == nil {
		t.Fatal("expected a claimed job")
	}
	if job.ID != older.ID {
		t.Fatalf("expected oldest browser job %d, got %d", older.ID, job.ID)
	}
	if job.Status != database.PrintJobStatusPrinting {
		t.Fatalf("expected printing, got %q", job.Status)
	}
	if job.ClaimedBy == nil || *job.ClaimedBy != "client-a" {
		t.Fatalf("expected claimed_by=client-a, got %v", job.ClaimedBy)
	}
	if job.LeaseExpiresAt == nil || !job.LeaseExpiresAt.After(now) {
		t.Fatalf("expected lease_expires_at in the future, got %v", job.LeaseExpiresAt)
	}
	if job.PayloadHTML == nil || *job.PayloadHTML == "" {
		t.Fatal("claimed job must include rendered HTML")
	}
}

func TestQueue_ClaimBrowserJob_TwoClientsOnlyOneWins(t *testing.T) {
	db := setupTestDB(t)
	biz := mustCreateBusiness(t, db)
	q := NewQueue(db)
	ctx := context.Background()
	now := time.Now()

	browser := database.Printer{
		BusinessID: biz.ID, Name: "browser", Role: "bill",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	if err := db.Create(&browser).Error; err != nil {
		t.Fatal(err)
	}
	html := "<html>x</html>"
	job := database.PrintJob{
		BusinessID: biz.ID, PrinterID: &browser.ID, Kind: database.PrintJobKindBill,
		SourceType: "bill", SourceID: 1, Status: database.PrintJobStatusRouted,
		PayloadHTML: &html, MaxAttempts: 6,
	}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	a, err := q.ClaimBrowserJob(ctx, biz.ID, "client-a", now, time.Minute)
	if err != nil {
		t.Fatalf("claim a: %v", err)
	}
	if a == nil {
		t.Fatal("client-a should claim")
	}
	b, err := q.ClaimBrowserJob(ctx, biz.ID, "client-b", now, time.Minute)
	if err != nil {
		t.Fatalf("claim b: %v", err)
	}
	if b != nil {
		t.Fatalf("client-b must not claim an already-leased job, got job %d", b.ID)
	}
}

func TestQueue_ClaimBrowserJobForPrinter_OnlyClaimsSelectedStation(t *testing.T) {
	db := setupTestDB(t)
	biz := mustCreateBusiness(t, db)
	q := NewQueue(db)
	now := time.Now()

	front := database.Printer{BusinessID: biz.ID, Name: "front", Role: "bill", Transport: "browser", PaperWidthMM: 80, Enabled: true}
	kitchen := database.Printer{BusinessID: biz.ID, Name: "kitchen", Role: "kitchen", Transport: "browser", PaperWidthMM: 80, Enabled: true}
	if err := db.Create(&front).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&kitchen).Error; err != nil {
		t.Fatal(err)
	}
	html := "<html>x</html>"
	frontJob := database.PrintJob{BusinessID: biz.ID, PrinterID: &front.ID, Kind: database.PrintJobKindBill, SourceType: "bill", SourceID: 1, Status: database.PrintJobStatusRouted, PayloadHTML: &html, MaxAttempts: 6}
	kitchenJob := database.PrintJob{BusinessID: biz.ID, PrinterID: &kitchen.ID, Kind: database.PrintJobKindKitchen, SourceType: "order", SourceID: 2, Status: database.PrintJobStatusRouted, PayloadHTML: &html, MaxAttempts: 6}
	if err := db.Create(&frontJob).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&kitchenJob).Error; err != nil {
		t.Fatal(err)
	}

	claimed, err := q.ClaimBrowserJobForPrinter(context.Background(), biz.ID, kitchen.ID, "kitchen-station", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.ID != kitchenJob.ID {
		t.Fatalf("selected kitchen station must claim job %d, got %#v", kitchenJob.ID, claimed)
	}
	var untouched database.PrintJob
	if err := db.First(&untouched, frontJob.ID).Error; err != nil {
		t.Fatal(err)
	}
	if untouched.Status != database.PrintJobStatusRouted {
		t.Fatalf("front job must remain routed, got %s", untouched.Status)
	}
}

func TestQueue_ClaimAuthorizedBrowserJobForPrinter_SkipsUnauthorizedKinds(t *testing.T) {
	db := setupTestDB(t)
	biz := mustCreateBusiness(t, db)
	q := NewQueue(db)
	printer := database.Printer{BusinessID: biz.ID, Name: "front", Role: "bill", Transport: "browser", PaperWidthMM: 80, Enabled: true}
	if err := db.Create(&printer).Error; err != nil {
		t.Fatal(err)
	}
	html := "<html>x</html>"
	receipt := database.PrintJob{BusinessID: biz.ID, PrinterID: &printer.ID, Kind: database.PrintJobKindReceipt, SourceType: "bill", SourceID: 1, Status: database.PrintJobStatusRouted, PayloadHTML: &html, MaxAttempts: 6}
	bill := database.PrintJob{BusinessID: biz.ID, PrinterID: &printer.ID, Kind: database.PrintJobKindBill, SourceType: "bill", SourceID: 2, Status: database.PrintJobStatusRouted, PayloadHTML: &html, MaxAttempts: 6}
	if err := db.Create(&receipt).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&bill).Error; err != nil {
		t.Fatal(err)
	}

	claimed, err := q.ClaimAuthorizedBrowserJobForPrinter(context.Background(), biz.ID, printer.ID, []database.PrintJobKind{database.PrintJobKindBill}, "bill-only", time.Now(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.ID != bill.ID {
		t.Fatalf("bill-only operator must skip receipt %d and claim bill %d, got %#v", receipt.ID, bill.ID, claimed)
	}
	var untouched database.PrintJob
	if err := db.First(&untouched, receipt.ID).Error; err != nil {
		t.Fatal(err)
	}
	if untouched.Status != database.PrintJobStatusRouted {
		t.Fatalf("unauthorized receipt must remain routed, got %s", untouched.Status)
	}
}

func TestQueue_ClaimBrowserJobForPrinter_RejectsInvalidStation(t *testing.T) {
	db := setupTestDB(t)
	biz := mustCreateBusiness(t, db)
	q := NewQueue(db)
	disabled := database.Printer{BusinessID: biz.ID, Name: "off", Role: "bill", Transport: "browser", PaperWidthMM: 80, Enabled: false}
	if err := db.Create(&disabled).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&disabled).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}

	_, err := q.ClaimBrowserJobForPrinter(context.Background(), biz.ID, disabled.ID, "station", time.Now(), time.Minute)
	if err != ErrInvalidBrowserPrinter {
		t.Fatalf("want ErrInvalidBrowserPrinter, got %v", err)
	}
}

func TestQueue_ClaimBrowserJob_CrossBusinessIsolation(t *testing.T) {
	db := setupTestDB(t)
	bizA := mustCreateBusiness(t, db)
	// Second business needs a distinct BusinessId for uniqueness.
	bizB := &database.Business{
		BusinessId:     "biz-b-" + t.Name(),
		OwnerAddress:   "0x00000000000000000000000000000000000000b1",
		Name:           "Other Bistro",
		SettlementAddr: "0x00000000000000000000000000000000000000b2",
		TippingAddr:    "0x00000000000000000000000000000000000000b3",
	}
	if err := db.Create(bizB).Error; err != nil {
		t.Fatal(err)
	}
	q := NewQueue(db)
	ctx := context.Background()
	now := time.Now()

	printerA := database.Printer{
		BusinessID: bizA.ID, Name: "a", Role: "bill",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	if err := db.Create(&printerA).Error; err != nil {
		t.Fatal(err)
	}
	html := "<html>a</html>"
	jobA := database.PrintJob{
		BusinessID: bizA.ID, PrinterID: &printerA.ID, Kind: database.PrintJobKindBill,
		SourceType: "bill", SourceID: 1, Status: database.PrintJobStatusRouted,
		PayloadHTML: &html, MaxAttempts: 6,
	}
	if err := db.Create(&jobA).Error; err != nil {
		t.Fatal(err)
	}

	claimed, err := q.ClaimBrowserJob(ctx, bizB.ID, "client-b", now, time.Minute)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed != nil {
		t.Fatalf("biz B must not see biz A job, got %d", claimed.ID)
	}
}

func TestQueue_BrowserLease_RenewPresentedConfirmOwnership(t *testing.T) {
	db := setupTestDB(t)
	biz := mustCreateBusiness(t, db)
	q := NewQueue(db)
	ctx := context.Background()
	now := time.Now()

	browser := database.Printer{
		BusinessID: biz.ID, Name: "browser", Role: "bill",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	if err := db.Create(&browser).Error; err != nil {
		t.Fatal(err)
	}
	html := "<html>x</html>"
	seed := database.PrintJob{
		BusinessID: biz.ID, PrinterID: &browser.ID, Kind: database.PrintJobKindBill,
		SourceType: "bill", SourceID: 1, Status: database.PrintJobStatusRouted,
		PayloadHTML: &html, MaxAttempts: 6,
	}
	if err := db.Create(&seed).Error; err != nil {
		t.Fatal(err)
	}

	job, err := q.ClaimBrowserJob(ctx, biz.ID, "owner", now, 30*time.Second)
	if err != nil || job == nil {
		t.Fatalf("claim: job=%v err=%v", job, err)
	}

	// Non-owner mutations must fail.
	if err := q.RenewBrowserLease(ctx, job.ID, "other", now, time.Minute); err != ErrNotLeaseOwner {
		t.Fatalf("renew other: want ErrNotLeaseOwner, got %v", err)
	}
	if err := q.MarkPresented(ctx, job.ID, "other", now); err != ErrNotLeaseOwner {
		t.Fatalf("present other: want ErrNotLeaseOwner, got %v", err)
	}
	if err := q.ConfirmPrinted(ctx, job.ID, "other", now); err != ErrNotLeaseOwner {
		t.Fatalf("confirm other: want ErrNotLeaseOwner, got %v", err)
	}

	// Owner renew + present + confirm.
	if err := q.RenewBrowserLease(ctx, job.ID, "owner", now, time.Minute); err != nil {
		t.Fatalf("renew: %v", err)
	}
	if err := q.MarkPresented(ctx, job.ID, "owner", now); err != nil {
		t.Fatalf("present: %v", err)
	}
	var afterPresent database.PrintJob
	if err := db.First(&afterPresent, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if afterPresent.PresentedAt == nil {
		t.Fatal("presented_at must be set after MarkPresented")
	}
	if afterPresent.Status != database.PrintJobStatusPrinting {
		t.Fatalf("present must NOT flip to printed; got %q", afterPresent.Status)
	}
	if afterPresent.PrintedAt != nil {
		t.Fatal("present must not set printed_at")
	}

	if err := q.ConfirmPrinted(ctx, job.ID, "owner", now); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	var afterConfirm database.PrintJob
	if err := db.First(&afterConfirm, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if afterConfirm.Status != database.PrintJobStatusPrinted {
		t.Fatalf("expected printed, got %q", afterConfirm.Status)
	}
	if afterConfirm.PrintedAt == nil {
		t.Fatal("printed_at must be set on confirm")
	}

	// Duplicate confirmation is a conflict, not a silent double-print.
	if err := q.ConfirmPrinted(ctx, job.ID, "owner", now); err != ErrIllegalTransition && err != ErrNotLeaseOwner {
		t.Fatalf("second confirm: want illegal transition or not owner, got %v", err)
	}
}

func TestQueue_BrowserLease_ExpiredReclaim(t *testing.T) {
	db := setupTestDB(t)
	biz := mustCreateBusiness(t, db)
	q := NewQueue(db)
	ctx := context.Background()
	now := time.Now()

	browser := database.Printer{
		BusinessID: biz.ID, Name: "browser", Role: "bill",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	if err := db.Create(&browser).Error; err != nil {
		t.Fatal(err)
	}
	html := "<html>x</html>"
	seed := database.PrintJob{
		BusinessID: biz.ID, PrinterID: &browser.ID, Kind: database.PrintJobKindBill,
		SourceType: "bill", SourceID: 1, Status: database.PrintJobStatusRouted,
		PayloadHTML: &html, MaxAttempts: 6,
	}
	if err := db.Create(&seed).Error; err != nil {
		t.Fatal(err)
	}

	job, err := q.ClaimBrowserJob(ctx, biz.ID, "crashed-tab", now, time.Second)
	if err != nil || job == nil {
		t.Fatalf("claim: %v %v", job, err)
	}

	// Crash: lease expires without confirm.
	expiredAt := now.Add(2 * time.Second)
	n, err := q.ReclaimExpiredBrowserLeases(ctx, expiredAt)
	if err != nil {
		t.Fatalf("reclaim: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 reclaimed, got %d", n)
	}

	// Second client claims the recovered job.
	recovered, err := q.ClaimBrowserJob(ctx, biz.ID, "survivor", expiredAt, time.Minute)
	if err != nil {
		t.Fatalf("reclaim claim: %v", err)
	}
	if recovered == nil || recovered.ID != job.ID {
		t.Fatalf("expected recovered job %d, got %+v", job.ID, recovered)
	}
	if recovered.ClaimedBy == nil || *recovered.ClaimedBy != "survivor" {
		t.Fatalf("expected claimed_by=survivor, got %v", recovered.ClaimedBy)
	}
}

func TestQueue_BrowserLease_RetryAndFail(t *testing.T) {
	db := setupTestDB(t)
	biz := mustCreateBusiness(t, db)
	q := NewQueue(db)
	ctx := context.Background()
	now := time.Now()

	browser := database.Printer{
		BusinessID: biz.ID, Name: "browser", Role: "bill",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	if err := db.Create(&browser).Error; err != nil {
		t.Fatal(err)
	}
	html := "<html>x</html>"
	seed := database.PrintJob{
		BusinessID: biz.ID, PrinterID: &browser.ID, Kind: database.PrintJobKindBill,
		SourceType: "bill", SourceID: 1, Status: database.PrintJobStatusRouted,
		PayloadHTML: &html, MaxAttempts: 6, AttemptCount: 0,
	}
	if err := db.Create(&seed).Error; err != nil {
		t.Fatal(err)
	}

	job, err := q.ClaimBrowserJob(ctx, biz.ID, "owner", now, time.Minute)
	if err != nil || job == nil {
		t.Fatalf("claim: %v", err)
	}
	if err := q.MarkPresented(ctx, job.ID, "owner", now); err != nil {
		t.Fatalf("present: %v", err)
	}
	if err := q.RetryBrowserJob(ctx, job.ID, "owner", now); err != nil {
		t.Fatalf("retry: %v", err)
	}
	var afterRetry database.PrintJob
	if err := db.First(&afterRetry, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if afterRetry.Status != database.PrintJobStatusRouted {
		t.Fatalf("retry should return job to routed, got %q", afterRetry.Status)
	}
	if afterRetry.ClaimedBy != nil {
		t.Fatal("retry must clear claimed_by")
	}
	if afterRetry.PresentedAt != nil {
		t.Fatal("retry must clear presented_at")
	}

	// Re-claim and fail permanently.
	job2, err := q.ClaimBrowserJob(ctx, biz.ID, "owner", now, time.Minute)
	if err != nil || job2 == nil {
		t.Fatalf("reclaim after retry: %v", err)
	}
	if err := q.FailBrowserJob(ctx, job2.ID, "owner", now, "operator cancelled station"); err != nil {
		t.Fatalf("fail: %v", err)
	}
	var afterFail database.PrintJob
	if err := db.First(&afterFail, job2.ID).Error; err != nil {
		t.Fatal(err)
	}
	if afterFail.Status != database.PrintJobStatusFailedPermanent {
		t.Fatalf("expected failed_permanent, got %q", afterFail.Status)
	}
}

func TestService_ConfirmPrinted_DoesNotAutoMarkFromPresent(t *testing.T) {
	db := setupTestDB(t)
	biz := mustCreateBusiness(t, db)
	svc := NewService(db)
	ctx := context.Background()
	now := time.Now()

	browser := database.Printer{
		BusinessID: biz.ID, Name: "browser", Role: "bill",
		Transport: "browser", PaperWidthMM: 80, Enabled: true,
	}
	if err := db.Create(&browser).Error; err != nil {
		t.Fatal(err)
	}
	html := "<html>x</html>"
	seed := database.PrintJob{
		BusinessID: biz.ID, PrinterID: &browser.ID, Kind: database.PrintJobKindBill,
		SourceType: "bill", SourceID: 1, Status: database.PrintJobStatusRouted,
		PayloadHTML: &html, MaxAttempts: 6,
	}
	if err := db.Create(&seed).Error; err != nil {
		t.Fatal(err)
	}

	job, err := svc.ClaimBrowserJob(ctx, biz.ID, "client-1", now, time.Minute)
	if err != nil || job == nil {
		t.Fatalf("claim: %v", err)
	}
	if err := svc.MarkPresented(ctx, job.ID, "client-1", now, "actor"); err != nil {
		t.Fatalf("present: %v", err)
	}
	var mid database.PrintJob
	if err := db.First(&mid, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if mid.Status == database.PrintJobStatusPrinted {
		t.Fatal("MarkPresented must not auto-confirm printed")
	}
	if err := svc.ConfirmPrinted(ctx, job.ID, "client-1", now, "actor"); err != nil {
		t.Fatalf("confirm: %v", err)
	}
}
