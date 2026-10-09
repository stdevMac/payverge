package fiscal

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// stubDispatcher is an injectable ReceiptDispatcher recording each call so a test
// can assert the email/print fan-out without real S3/email/print transports.
type stubDispatcher struct {
	mu sync.Mutex

	uploads     int
	emailCalls  int
	printCalls  int
	lastPDF     []byte
	lastEmailTo []string
	lastLang    string

	uploadErr error
	emailErr  error
	printErr  error
}

func (s *stubDispatcher) UploadProtected(data []byte, name, folder, contentType string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uploads++
	if s.uploadErr != nil {
		return "", s.uploadErr
	}
	return "https://protected.example/" + folder + "/" + name, nil
}

func (s *stubDispatcher) SendReceiptEmail(to []string, businessName, receiptType, receiptNumber, totalDisplay, language string, pdf []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emailCalls++
	s.lastEmailTo = to
	s.lastPDF = pdf
	s.lastLang = language
	return s.emailErr
}

func (s *stubDispatcher) EnqueueReceiptPrint(ctx context.Context, businessID, billID uint, language string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.printCalls++
	return s.printErr
}

func seedBillWithCustomer(t *testing.T, db *gorm.DB, businessID, billID, customerID uint, email string, totalCents int64) {
	t.Helper()
	require.NoError(t, db.Create(&database.Business{
		ID:             businessID,
		BusinessId:     "biz-deliver",
		OwnerAddress:   "owner",
		Name:           "Test Biz",
		SettlementAddr: "settle",
		TippingAddr:    "tip",
	}).Error)
	require.NoError(t, db.Create(&database.Customer{
		ID:    customerID,
		Email: email,
	}).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID:            billID,
		BusinessID:    businessID,
		BillNumber:    "PV-deliver",
		Status:        database.BillStatusPaid,
		Items:         "[]",
		TotalAmount:   totalCents,
		PaidAmount:    totalCents,
		CRMCustomerID: &customerID,
	}).Error)
}

func newDeliveryTestWorker(t *testing.T, disp ReceiptDispatcher) (*FiscalWorker, *gorm.DB, *stubProvider) {
	t.Helper()
	db := newFiscalTestDBWithCustomer(t)
	prov := &stubProvider{}
	w := NewFiscalWorker(db, &fakeFactory{provider: prov})
	w.WorkerID = "test-worker"
	w.service.WithDelivery(disp)
	return w, db, prov
}

// runIssueThenDelivery processes fiscal issue jobs then durable delivery tasks.
// Wave 4: authorization only enqueues delivery tasks; delivery is a separate worker.
func runIssueThenDelivery(t *testing.T, w *FiscalWorker, disp ReceiptDispatcher) (jobs, deliveries int) {
	t.Helper()
	n, err := w.ProcessDue(context.Background())
	require.NoError(t, err)
	dw := NewDeliveryWorker(w.db, w.service, disp)
	dw.WorkerID = "test-delivery-worker"
	// Multiple ticks: email may retry independently of artifact/print.
	total := 0
	for i := 0; i < 5; i++ {
		// Force all pending tasks due for deterministic unit tests.
		_ = w.db.Model(&database.FiscalDeliveryTask{}).
			Where("status = ?", database.FiscalDeliveryStatusPending).
			Update("next_attempt_at", time.Now().UTC().Add(-time.Minute)).Error
		d, err := dw.ProcessDue(context.Background())
		require.NoError(t, err)
		total += d
		if d == 0 {
			break
		}
	}
	return n, total
}

func newFiscalTestDBWithCustomer(t *testing.T) *gorm.DB {
	t.Helper()
	db := newFiscalTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.Customer{}))
	return db
}

func authorizedIssueProvider() func(ctx context.Context, in IssueInput) (*ReceiptResult, error) {
	return func(ctx context.Context, in IssueInput) (*ReceiptResult, error) {
		return &ReceiptResult{
			Status:            StatusAuthorized,
			ProviderReceiptID: "71000000000001",
			AuthCode:          "71000000000001",
			ReceiptNumber:     "43",
			QRPayload:         "https://www.afip.gob.ar/fe/qr/?p=abc",
			ReceiptType:       "factura_c",
		}, nil
	}
}

// TestDeliverReceiptHappyPath: an authorized issue job enqueues delivery tasks;
// the delivery worker then uploads PDF/QR, emails once, prints once, and stamps
// DeliveredAt when all channels succeed.
func TestDeliverReceiptHappyPath(t *testing.T) {
	disp := &stubDispatcher{}
	w, db, prov := newDeliveryTestWorker(t, disp)
	prov.IssueFn = authorizedIssueProvider()

	seedBillWithCustomer(t, db, 1, 1, 100, "guest@example.com", 12100)
	seedReadySettings(t, db, 1)
	enqueueIssueJob(t, db, 1, 1)

	jobs, deliveries := runIssueThenDelivery(t, w, disp)
	require.Equal(t, 1, jobs)
	require.GreaterOrEqual(t, deliveries, 3)

	require.Equal(t, 1, disp.emailCalls, "exactly one email must be enqueued")
	require.Equal(t, 1, disp.printCalls, "exactly one print must be enqueued")
	require.GreaterOrEqual(t, disp.uploads, 1, "the PDF (and QR) must be uploaded")
	require.Equal(t, []string{"guest@example.com"}, disp.lastEmailTo)
	require.NotEmpty(t, disp.lastPDF, "email must carry the rendered PDF bytes")
	require.Equal(t, "%PDF", string(disp.lastPDF[:4]), "delivered attachment must be a PDF")

	var r database.FiscalReceipt
	require.NoError(t, db.Where("bill_id = ?", 1).First(&r).Error)
	require.Equal(t, database.FiscalStatusAuthorized, r.Status)
	require.NotNil(t, r.DeliveredAt, "an authorized delivered receipt must have DeliveredAt set")
	require.NotNil(t, r.PDFPath, "PDFPath must be set after a successful upload")

	var taskCount int64
	require.NoError(t, db.Model(&database.FiscalDeliveryTask{}).
		Where("receipt_id = ? AND status = ?", r.ID, database.FiscalDeliveryStatusSucceeded).
		Count(&taskCount).Error)
	require.Equal(t, int64(3), taskCount)
}

// TestDeliverReceiptIdempotent: re-running the delivery worker on already-
// succeeded tasks must NOT re-deliver.
func TestDeliverReceiptIdempotent(t *testing.T) {
	disp := &stubDispatcher{}
	w, db, prov := newDeliveryTestWorker(t, disp)
	prov.IssueFn = authorizedIssueProvider()

	seedBillWithCustomer(t, db, 1, 1, 100, "guest@example.com", 12100)
	seedReadySettings(t, db, 1)
	enqueueIssueJob(t, db, 1, 1)

	_, _ = runIssueThenDelivery(t, w, disp)
	require.Equal(t, 1, disp.emailCalls)

	var r database.FiscalReceipt
	require.NoError(t, db.Where("bill_id = ?", 1).First(&r).Error)
	require.NotNil(t, r.DeliveredAt)

	// Second delivery sweep: succeeded tasks are not reclaimable.
	dw := NewDeliveryWorker(db, w.service, disp)
	dw.WorkerID = "test-delivery-worker-2"
	n2, err := dw.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, n2)

	require.Equal(t, 1, disp.emailCalls, "a delivered receipt must not be re-emailed")
	require.Equal(t, 1, disp.printCalls, "a delivered receipt must not be re-printed")
}

// TestDeliverReceiptEmailFailureLeavesUndelivered: a transient email failure must
// NOT set DeliveredAt (email task stays pending/retry) and must NOT fail the job.
func TestDeliverReceiptEmailFailureLeavesUndelivered(t *testing.T) {
	disp := &stubDispatcher{emailErr: errors.New("postmark temporarily unavailable")}
	w, db, prov := newDeliveryTestWorker(t, disp)
	prov.IssueFn = authorizedIssueProvider()

	seedBillWithCustomer(t, db, 1, 1, 100, "guest@example.com", 12100)
	seedReadySettings(t, db, 1)
	enqueueIssueJob(t, db, 1, 1)

	jobs, _ := runIssueThenDelivery(t, w, disp)
	require.Equal(t, 1, jobs)

	// The fiscal JOB must still be authorized — delivery failure is best-effort.
	var job database.FiscalJob
	require.NoError(t, db.Where("bill_id = ?", 1).First(&job).Error)
	require.Equal(t, database.FiscalStatusAuthorized, job.Status, "delivery failure must NOT fail the fiscal job")

	var r database.FiscalReceipt
	require.NoError(t, db.Where("bill_id = ?", 1).First(&r).Error)
	require.Equal(t, database.FiscalStatusAuthorized, r.Status)
	require.Nil(t, r.DeliveredAt, "an email failure must leave DeliveredAt nil so delivery retries")

	var emailTask database.FiscalDeliveryTask
	require.NoError(t, db.Where("receipt_id = ? AND channel = ?", r.ID, database.FiscalDeliveryChannelEmail).
		First(&emailTask).Error)
	require.NotEqual(t, database.FiscalDeliveryStatusSucceeded, emailTask.Status)
}

// TestDeliverReceiptCustomerEmailUsesNarrowProjection asserts the recipient
// lookup reads only the email column from customers — never SELECT * over the
// wide customer row — so the delivery path stays cheap on the worker.
func TestDeliverReceiptCustomerEmailUsesNarrowProjection(t *testing.T) {
	disp := &stubDispatcher{}
	w, db, prov := newDeliveryTestWorker(t, disp)
	prov.IssueFn = authorizedIssueProvider()

	var sawCustomerLoad, sawSelectStar bool
	// The recipient lookup Scans into a primitive (string email), which GORM
	// routes through the Row callback, not the Query callback.
	capture := func(tx *gorm.DB) {
		sql := strings.ToLower(tx.Statement.SQL.String())
		if strings.Contains(sql, "from `customers`") || strings.Contains(sql, "from \"customers\"") {
			sawCustomerLoad = true
			if strings.Contains(sql, "select *") {
				sawSelectStar = true
			}
		}
	}
	require.NoError(t, db.Callback().Row().After("gorm:row").Register("test:capture_customer_row", capture))
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:capture_customer_sql", capture))
	t.Cleanup(func() {
		_ = db.Callback().Row().Remove("test:capture_customer_row")
		_ = db.Callback().Query().Remove("test:capture_customer_sql")
	})

	seedBillWithCustomer(t, db, 1, 1, 100, "guest@example.com", 12100)
	seedReadySettings(t, db, 1)
	enqueueIssueJob(t, db, 1, 1)

	jobs, _ := runIssueThenDelivery(t, w, disp)
	require.Equal(t, 1, jobs)

	require.True(t, sawCustomerLoad, "delivery must read the customer email")
	require.False(t, sawSelectStar, "customer email lookup must use a narrow projection, not SELECT *")
}

// TestDeliverReceiptDeliveryOrderEmailFallback (DELIV-FISC-1 regression):
// delivery checkout creates the bill WITHOUT a CRMCustomerID and stores the
// guest email on DeliveryOrder.CustomerEmail. The fiscal receipt must fall back
// to the linked delivery order's email so delivery guests still receive their
// AFIP factura.
func TestDeliverReceiptDeliveryOrderEmailFallback(t *testing.T) {
	disp := &stubDispatcher{}
	w, db, prov := newDeliveryTestWorker(t, disp)
	prov.IssueFn = authorizedIssueProvider()

	require.NoError(t, db.AutoMigrate(&database.DeliveryOrder{}))

	require.NoError(t, db.Create(&database.Business{
		ID:             1,
		BusinessId:     "biz-delivery-fallback",
		OwnerAddress:   "owner",
		Name:           "Test Biz",
		SettlementAddr: "settle",
		TippingAddr:    "tip",
	}).Error)
	// Delivery bill: NO CRMCustomerID (delivery checkout never sets it).
	require.NoError(t, db.Create(&database.Bill{
		ID:          1,
		BusinessID:  1,
		BillNumber:  "PV-delivery",
		Status:      database.BillStatusPaid,
		Items:       "[]",
		TotalAmount: 12100,
		PaidAmount:  12100,
	}).Error)
	// The guest email lives on the linked delivery order.
	require.NoError(t, db.Create(&database.DeliveryOrder{
		ID:             1,
		BusinessID:     1,
		BillID:         1,
		DeliveryNumber: "DEL-1",
		DeliveryType:   database.DeliveryTypeInHouse,
		Status:         database.DeliveryStatusPending,
		CustomerName:   "Delivery Guest",
		CustomerPhone:  "+5491100000000",
		CustomerEmail:  "delivery-guest@example.com",
	}).Error)
	seedReadySettings(t, db, 1)
	enqueueIssueJob(t, db, 1, 1)

	jobs, _ := runIssueThenDelivery(t, w, disp)
	require.Equal(t, 1, jobs)

	require.Equal(t, 1, disp.emailCalls, "delivery guest must receive the fiscal receipt email")
	require.Equal(t, []string{"delivery-guest@example.com"}, disp.lastEmailTo, "recipient must fall back to DeliveryOrder.CustomerEmail")
	require.NotEmpty(t, disp.lastPDF, "email must carry the rendered PDF bytes")

	var r database.FiscalReceipt
	require.NoError(t, db.Where("bill_id = ?", 1).First(&r).Error)
	require.NotNil(t, r.DeliveredAt, "receipt must be stamped delivered after the fallback email send")
}

// TestDeliverReceiptNoCustomerEmailStillDeliversAndPrints: a Consumidor Final
// bill with no customer email must NOT block delivery — it uploads, best-effort
// prints, and stamps DeliveredAt (no recipient to email).
func TestDeliverReceiptNoCustomerEmail(t *testing.T) {
	disp := &stubDispatcher{}
	w, db, prov := newDeliveryTestWorker(t, disp)
	prov.IssueFn = authorizedIssueProvider()

	// No CRMCustomerID → Consumidor Final.
	require.NoError(t, db.Create(&database.Bill{
		ID:          1,
		BusinessID:  1,
		BillNumber:  "PV-cf",
		Status:      database.BillStatusPaid,
		Items:       "[]",
		TotalAmount: 12100,
		PaidAmount:  12100,
	}).Error)
	require.NoError(t, db.Create(&database.Business{
		ID:             1,
		BusinessId:     "biz-cf",
		OwnerAddress:   "owner",
		Name:           "Test Biz",
		SettlementAddr: "settle",
		TippingAddr:    "tip",
	}).Error)
	seedReadySettings(t, db, 1)
	enqueueIssueJob(t, db, 1, 1)

	jobs, _ := runIssueThenDelivery(t, w, disp)
	require.Equal(t, 1, jobs)

	require.Equal(t, 0, disp.emailCalls, "no customer email → no email")
	require.Equal(t, 1, disp.printCalls, "print should still fire best-effort")

	var r database.FiscalReceipt
	require.NoError(t, db.Where("bill_id = ?", 1).First(&r).Error)
	require.NotNil(t, r.DeliveredAt, "with no recipient to email, delivery is considered complete and stamped")
}
