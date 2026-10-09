package services

// Regression tests for the delivery payment/cancel findings:
//
//   DEL-PAY-02 — cancel/expiry must be serialized against bill settlement:
//     cancelDeliveryLinked must decide under a bill row lock (lock order
//     bill → delivery, matching applyConfirmedPaymentTx which locks the bill
//     first), and a payment that lands on an already-terminal delivery must
//     surface a refund-review alert instead of silently no-oping.
//   DEL-PAY-03 — any cancelled/failed/rejected/expired transition of a
//     delivery whose bill has PaidAmount > 0 must raise a money-stripped
//     "refund review" operational alert (no automatic refunds).
//   DEL-GT-5  — the COD acceptance email must format the bill total in the
//     business currency with correct minor units (JPY must not show "$…").
//   DEL-SM-8  — the reconcile branch of AcceptDeliveryByOrder must never
//     issue an online-payment window: a mode=online retry of a wedged COD
//     accept must not email a pay link (which would carry no expiry).
import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// migrateOperationalAlerts adds the alert tables to the shared hospitality
// test DB (they are not part of setupHospitalityServiceTestDB's default set).
func migrateOperationalAlerts(t *testing.T, svc *DeliveryService) {
	t.Helper()
	if err := svc.db.AutoMigrate(&database.OperationalAlert{}, &database.OperationalAlertEvent{}); err != nil {
		t.Fatal(err)
	}
}

func findRefundReviewAlerts(t *testing.T, svc *DeliveryService, businessID uint) []database.OperationalAlert {
	t.Helper()
	var alerts []database.OperationalAlert
	if err := svc.db.Where("business_id = ? AND alert_type = ?", businessID, database.OperationalAlertTypePaymentRefundReview).
		Find(&alerts).Error; err != nil {
		t.Fatal(err)
	}
	return alerts
}

// DEL-PAY-03: rejecting a delivery whose bill already carries guest money must
// still cancel, must NOT close the bill, and must raise a refund-review alert
// whose metadata is money-stripped (no amounts).
func TestCancelPaidDelivery_RaisesRefundReviewAlert(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	migrateOperationalAlerts(t, svc)
	bill, order, delivery := seedDeliveryTriple(t, svc, businessID)

	// Guest paid in full before the operator rejected.
	if err := svc.db.Model(&database.Bill{}).Where("id = ?", bill.ID).
		Updates(map[string]interface{}{"paid_amount": bill.TotalAmount, "status": database.BillStatusPaid}).Error; err != nil {
		t.Fatal(err)
	}

	if err := svc.RejectDeliveryByOrder(businessID, order.ID, "staff:1", "kitchen fire"); err != nil {
		t.Fatalf("reject of a paid delivery must still succeed: %v", err)
	}

	var d database.DeliveryOrder
	svc.db.First(&d, delivery.ID)
	if d.Status != database.DeliveryStatusCancelled {
		t.Fatalf("delivery = %s, want cancelled", d.Status)
	}
	var b database.Bill
	svc.db.First(&b, bill.ID)
	if b.Status != database.BillStatusPaid {
		t.Fatalf("paid bill must not be closed/mutated by cancel; got %s", b.Status)
	}

	alerts := findRefundReviewAlerts(t, svc, businessID)
	if len(alerts) != 1 {
		t.Fatalf("want exactly 1 refund-review alert for a paid cancelled delivery, got %d", len(alerts))
	}
	a := alerts[0]
	if a.ResourceType != database.OperationalAlertResourceTypeDelivery || a.ResourceID != int64(delivery.ID) {
		t.Fatalf("alert must reference the delivery resource, got %s/%d", a.ResourceType, a.ResourceID)
	}
	meta := strings.ToLower(string(a.Metadata))
	if strings.Contains(meta, "amount") || strings.Contains(meta, "cents") {
		t.Fatalf("alert metadata must be money-stripped, got %s", meta)
	}
	if !strings.Contains(meta, "delivery_id") || !strings.Contains(meta, "bill_id") {
		t.Fatalf("alert metadata must carry delivery/bill identifiers, got %s", meta)
	}
}

// Baseline: cancelling an UNPAID delivery raises no refund-review alert and
// still closes the bill.
func TestCancelUnpaidDelivery_NoRefundReviewAlert(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	migrateOperationalAlerts(t, svc)
	bill, order, _ := seedDeliveryTriple(t, svc, businessID)

	if err := svc.RejectDeliveryByOrder(businessID, order.ID, "staff:1", "no driver"); err != nil {
		t.Fatal(err)
	}
	if alerts := findRefundReviewAlerts(t, svc, businessID); len(alerts) != 0 {
		t.Fatalf("unpaid cancel must not raise refund-review alerts, got %d", len(alerts))
	}
	var b database.Bill
	svc.db.First(&b, bill.ID)
	if b.Status == database.BillStatusOpen {
		t.Fatalf("unpaid bill must be closed on cancel, got %s", b.Status)
	}
}

// DEL-PAY-03: a FAILED delivery with money on the bill also needs the alert
// (failed shares cancelDeliveryLinked's downstream cleanup).
func TestFailPaidDelivery_RaisesRefundReviewAlert(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	migrateOperationalAlerts(t, svc)
	bill, _, delivery := seedDeliveryTriple(t, svc, businessID)

	if err := svc.db.Model(&database.Bill{}).Where("id = ?", bill.ID).
		Updates(map[string]interface{}{"paid_amount": bill.TotalAmount, "status": database.BillStatusPaid}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.cancelDeliveryLinked(delivery.ID, "driver crashed", "staff:1", "failed", database.DeliveryStatusFailed); err != nil {
		t.Fatal(err)
	}
	if alerts := findRefundReviewAlerts(t, svc, businessID); len(alerts) != 1 {
		t.Fatalf("failed paid delivery must raise a refund-review alert, got %d", len(alerts))
	}
}

// DEL-PAY-02: the expiry sweep cancelling a PARTIALLY paid prepay delivery
// (no-refund-by-design) must flag the stranded money for refund review.
func TestExpirePartiallyPaidDelivery_CancelsAndAlerts(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	migrateOperationalAlerts(t, svc)
	bill, order, delivery := seedDeliveryTriple(t, svc, businessID)

	if _, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:1", database.DeliveryPaymentOnline); err != nil {
		t.Fatal(err)
	}
	// Guest paid part of the bill, then the window lapsed.
	past := time.Now().UTC().Add(-time.Minute)
	if err := svc.db.Model(&database.DeliveryOrder{}).Where("id = ?", delivery.ID).
		Update("payment_expires_at", past).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.db.Model(&database.Bill{}).Where("id = ?", bill.ID).
		Updates(map[string]interface{}{"paid_amount": 500, "status": database.BillStatusPartial}).Error; err != nil {
		t.Fatal(err)
	}

	if err := svc.ExpireUnpaidDeliveries(); err != nil {
		t.Fatal(err)
	}
	var d database.DeliveryOrder
	svc.db.First(&d, delivery.ID)
	if d.Status != database.DeliveryStatusCancelled {
		t.Fatalf("partially paid expired delivery must cancel; got %s", d.Status)
	}
	if alerts := findRefundReviewAlerts(t, svc, businessID); len(alerts) != 1 {
		t.Fatalf("expiring a partially paid delivery must raise a refund-review alert, got %d", len(alerts))
	}
	var b database.Bill
	svc.db.First(&b, bill.ID)
	if b.Status != database.BillStatusPartial {
		t.Fatalf("partially paid bill must not be closed by expiry, got %s", b.Status)
	}
}

// DEL-PAY-02(c): a payment that commits AFTER the delivery went terminal
// (cancel/expiry won the race) must not be silently dropped by the payment
// hook — HandleDeliveryBillPaid must raise the refund-review alert and must
// NOT resurrect the delivery.
func TestHandleDeliveryBillPaid_TerminalDeliveryRaisesAlert(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	migrateOperationalAlerts(t, svc)
	bill, order, delivery := seedDeliveryTriple(t, svc, businessID)

	if err := svc.RejectDeliveryByOrder(businessID, order.ID, "staff:1", "closed early"); err != nil {
		t.Fatal(err)
	}
	// Payment lands after the cancel (e.g. crypto confirmation raced the reject).
	if err := svc.db.Model(&database.Bill{}).Where("id = ?", bill.ID).
		Updates(map[string]interface{}{"paid_amount": bill.TotalAmount, "status": database.BillStatusPaid}).Error; err != nil {
		t.Fatal(err)
	}

	if err := svc.HandleDeliveryBillPaid(bill.ID); err != nil {
		t.Fatalf("payment hook on a terminal delivery must not error: %v", err)
	}
	var d database.DeliveryOrder
	svc.db.First(&d, delivery.ID)
	if d.Status != database.DeliveryStatusCancelled {
		t.Fatalf("payment hook must not resurrect a cancelled delivery; got %s", d.Status)
	}
	if alerts := findRefundReviewAlerts(t, svc, businessID); len(alerts) != 1 {
		t.Fatalf("payment landing on a cancelled delivery must raise a refund-review alert, got %d", len(alerts))
	}
}

// Regression guard: the normal confirmed→preparing advance on full payment is
// unchanged, and a DELIVERED delivery receiving its (normal) payment must NOT
// raise a refund-review alert.
func TestHandleDeliveryBillPaid_ConfirmedStillAdvances(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	migrateOperationalAlerts(t, svc)
	bill, order, delivery := seedDeliveryTriple(t, svc, businessID)

	if _, err := svc.AcceptDeliveryByOrder(businessID, order.ID, "staff:1", database.DeliveryPaymentOnline); err != nil {
		t.Fatal(err)
	}
	if err := svc.db.Model(&database.Bill{}).Where("id = ?", bill.ID).
		Updates(map[string]interface{}{"paid_amount": bill.TotalAmount, "status": database.BillStatusPaid}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.HandleDeliveryBillPaid(bill.ID); err != nil {
		t.Fatal(err)
	}
	var d database.DeliveryOrder
	svc.db.First(&d, delivery.ID)
	if d.Status != database.DeliveryStatusPreparing {
		t.Fatalf("paid confirmed delivery must advance to preparing; got %s", d.Status)
	}
	if d.PaymentExpiresAt != nil {
		t.Fatal("payment expiry must clear on advance")
	}
	if alerts := findRefundReviewAlerts(t, svc, businessID); len(alerts) != 0 {
		t.Fatalf("normal payment advance must not raise refund-review alerts, got %d", len(alerts))
	}
}

func TestHandleDeliveryBillPaid_DeliveredDeliveryNoAlert(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	migrateOperationalAlerts(t, svc)
	bill, _, delivery := seedDeliveryTriple(t, svc, businessID)

	if err := svc.db.Model(&database.DeliveryOrder{}).Where("id = ?", delivery.ID).
		Update("status", database.DeliveryStatusDelivered).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.db.Model(&database.Bill{}).Where("id = ?", bill.ID).
		Updates(map[string]interface{}{"paid_amount": bill.TotalAmount, "status": database.BillStatusPaid}).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.HandleDeliveryBillPaid(bill.ID); err != nil {
		t.Fatal(err)
	}
	if alerts := findRefundReviewAlerts(t, svc, businessID); len(alerts) != 0 {
		t.Fatalf("payment on a DELIVERED delivery is normal money, not refund review; got %d alerts", len(alerts))
	}
}

// seedDeliveryTripleWithEmail mirrors seedDeliveryTriple but attaches a guest
// email so notification paths fire, and lets the caller pick the bill total.
func seedDeliveryTripleWithEmail(t *testing.T, svc *DeliveryService, businessID uint, totalCents int64) (database.Bill, database.Order, database.DeliveryOrder) {
	t.Helper()
	bill := database.Bill{BusinessID: businessID, BillNumber: "B-CUR-1", Status: database.BillStatusOpen, Subtotal: totalCents, TotalAmount: totalCents}
	if err := svc.db.Omit("table_id").Create(&bill).Error; err != nil {
		t.Fatal(err)
	}
	order := database.Order{BillID: bill.ID, BusinessID: businessID, OrderNumber: "O-CUR-1", Status: database.OrderStatusPending, CreatedBy: "guest_delivery", Items: "[]"}
	if err := svc.db.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	delivery := database.DeliveryOrder{
		BusinessID: businessID, BillID: bill.ID, OrderID: &order.ID,
		DeliveryNumber: "DEL-CUR1", DeliveryType: database.DeliveryTypeInHouse,
		Status: database.DeliveryStatusPending, CustomerName: "g", CustomerPhone: "1",
		CustomerEmail: "guest@example.com",
	}
	if err := svc.db.Create(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	return bill, order, delivery
}

// DEL-GT-5: the COD acceptance email must render the bill total in the
// business currency with correct minor units — a JPY business must see
// "JPY 1700", never "$1700.00" (zero-decimal currencies have no cents).
func TestNotifyDeliveryAccepted_CODUsesBusinessCurrency(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	emailMock := &mockNotificationDispatcher{}
	svc := NewDeliveryService(db, newTestNotificationManager(emailMock))
	business := createTestHospitalityBusiness(t, db, t.Name())
	if err := db.Model(&database.Business{}).Where("id = ?", business.ID).
		Update("default_currency", "JPY").Error; err != nil {
		t.Fatal(err)
	}

	// ¥1700 stored as 170000 platform cents (major×100 for all currencies).
	_, order, _ := seedDeliveryTripleWithEmail(t, svc, business.ID, 170000)
	if _, err := svc.AcceptDeliveryByOrder(business.ID, order.ID, "staff:1", database.DeliveryPaymentCashOnDelivery); err != nil {
		t.Fatal(err)
	}

	if len(emailMock.notifications) == 0 {
		t.Fatal("COD accept must send the guest email")
	}
	body := emailMock.notifications[len(emailMock.notifications)-1].Description
	if strings.Contains(body, "$") {
		t.Fatalf("COD email must not hardcode '$' for a JPY business; body: %q", body)
	}
	if !strings.Contains(body, "JPY 1700") || strings.Contains(body, "JPY 1700.00") {
		t.Fatalf("COD email must show the zero-decimal JPY total ('JPY 1700'); body: %q", body)
	}
}

// DEL-SM-8: retrying a wedged COD accept with mode=online must NOT issue an
// online-payment window — no pay link, no awaiting-payment result, and no
// payment expiry may appear (the primary path always pairs link+expiry).
func TestReconcile_OnlineModeRetryDoesNotIssuePayLink(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	emailMock := &mockNotificationDispatcher{}
	svc := NewDeliveryService(db, newTestNotificationManager(emailMock))
	business := createTestHospitalityBusiness(t, db, t.Name())
	_, order, delivery := seedDeliveryTripleWithEmail(t, svc, business.ID, 1700)

	// COD accept, then wedge: order back to pending (approval leg "failed").
	if _, err := svc.AcceptDeliveryByOrder(business.ID, order.ID, "staff:1", database.DeliveryPaymentCashOnDelivery); err != nil {
		t.Fatal(err)
	}
	if err := svc.db.Model(&database.Order{}).Where("id = ?", order.ID).
		Update("status", database.OrderStatusPending).Error; err != nil {
		t.Fatal(err)
	}
	emailsBefore := len(emailMock.notifications)

	// Retry with the WRONG mode (online) — reconcile must derive COD semantics.
	res, err := svc.AcceptDeliveryByOrder(business.ID, order.ID, "staff:1", database.DeliveryPaymentOnline)
	if err != nil {
		t.Fatalf("reconcile retry failed: %v", err)
	}
	if res.AwaitingPayment {
		t.Fatal("reconcile of a preparing delivery must never report awaiting payment")
	}
	if res.PaymentMode != database.DeliveryPaymentCashOnDelivery {
		t.Fatalf("reconcile must derive the mode from delivery state (COD), got %s", res.PaymentMode)
	}
	var d database.DeliveryOrder
	svc.db.First(&d, delivery.ID)
	if d.PaymentExpiresAt != nil {
		t.Fatal("reconcile must not set a payment expiry")
	}
	if len(emailMock.notifications) <= emailsBefore {
		t.Fatal("reconcile of an unpaid COD wedge must still send the accepted email")
	}
	body := emailMock.notifications[len(emailMock.notifications)-1].Description
	if strings.Contains(body, "/pay") {
		t.Fatalf("reconcile email must not contain a pay link (no expiry backs it); body: %q", body)
	}
}

// DEL-SM-8 companion: a prepay wedge (bill already settled, order approval leg
// failed) must not email COD copy to a guest who already paid — no acceptance
// email at all on reconcile of a paid bill.
func TestReconcile_PaidBillSkipsAcceptedEmail(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	emailMock := &mockNotificationDispatcher{}
	svc := NewDeliveryService(db, newTestNotificationManager(emailMock))
	business := createTestHospitalityBusiness(t, db, t.Name())
	bill, order, delivery := seedDeliveryTripleWithEmail(t, svc, business.ID, 1700)

	// Simulate a paid prepay delivery wedged at the order-approval leg.
	if err := svc.db.Model(&database.DeliveryOrder{}).Where("id = ?", delivery.ID).
		Update("status", database.DeliveryStatusPreparing).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.db.Model(&database.Bill{}).Where("id = ?", bill.ID).
		Updates(map[string]interface{}{"paid_amount": bill.TotalAmount, "status": database.BillStatusPaid}).Error; err != nil {
		t.Fatal(err)
	}
	emailsBefore := len(emailMock.notifications)

	res, err := svc.AcceptDeliveryByOrder(business.ID, order.ID, "staff:1", database.DeliveryPaymentOnline)
	if err != nil {
		t.Fatalf("reconcile retry failed: %v", err)
	}
	if res.AwaitingPayment {
		t.Fatal("settled bill: reconcile must not report awaiting payment")
	}
	var o database.Order
	svc.db.First(&o, order.ID)
	if o.Status != database.OrderStatusApproved {
		t.Fatalf("reconcile must approve the wedged order; got %s", o.Status)
	}
	if len(emailMock.notifications) != emailsBefore {
		t.Fatalf("reconcile of a PAID bill must not send an acceptance email (guest already paid); got %d new", len(emailMock.notifications)-emailsBefore)
	}
}

// --- DEL-PAY-02(a) access-shape: bill is read (locked on Postgres) BEFORE the
// delivery row inside cancelDeliveryLinked's transaction, matching the
// bill-first lock order of applyConfirmedPaymentTx. SQLite drops the FOR
// UPDATE clause, so the pinned shape is the statement ORDER: the bills read
// (with paid_amount projected) must precede the full delivery-row read.

type sqlRecorder struct {
	mu   sync.Mutex
	sqls []string
}

func (r *sqlRecorder) LogMode(logger.LogLevel) logger.Interface      { return r }
func (r *sqlRecorder) Info(context.Context, string, ...interface{})  {}
func (r *sqlRecorder) Warn(context.Context, string, ...interface{})  {}
func (r *sqlRecorder) Error(context.Context, string, ...interface{}) {}
func (r *sqlRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.mu.Lock()
	r.sqls = append(r.sqls, sql)
	r.mu.Unlock()
}

func TestCancelDeliveryLinked_LocksBillBeforeDelivery(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	migrateOperationalAlerts(t, svc)
	_, _, delivery := seedDeliveryTriple(t, svc, businessID)

	rec := &sqlRecorder{}
	recSvc := &DeliveryService{db: svc.db.Session(&gorm.Session{Logger: rec})}
	if err := recSvc.cancelDeliveryLinked(delivery.ID, "shape check", "staff:1", "cancelled", database.DeliveryStatusCancelled); err != nil {
		t.Fatal(err)
	}

	billIdx, deliveryLockIdx := -1, -1
	for i, q := range rec.sqls {
		u := strings.ToLower(q)
		if billIdx == -1 && strings.Contains(u, "from `bills`") && strings.Contains(u, "paid_amount") {
			billIdx = i
		}
		if deliveryLockIdx == -1 && strings.Contains(u, "select * from `delivery_orders`") {
			deliveryLockIdx = i
		}
	}
	if billIdx == -1 {
		t.Fatalf("cancel must read the bill (with paid_amount) inside the tx; queries: %v", rec.sqls)
	}
	if deliveryLockIdx == -1 {
		t.Fatalf("cancel must take the full delivery row; queries: %v", rec.sqls)
	}
	if billIdx > deliveryLockIdx {
		t.Fatalf("lock-order regression: bill read (idx %d) must precede the delivery locking read (idx %d) to match applyConfirmedPaymentTx's bill-first order; queries: %v", billIdx, deliveryLockIdx, rec.sqls)
	}
}
