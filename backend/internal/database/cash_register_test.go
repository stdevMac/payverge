package database_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCashRegisterSessionModelJSONHidesOpenExpectedTotals(t *testing.T) {
	session := database.CashRegisterSession{
		ID:                10,
		BusinessID:        20,
		Status:            database.CashRegisterSessionStatusOpen,
		OpeningFloatCents: 12500,
		ExpectedCashCents: 15000,
		CountedCashCents:  0,
		VarianceCents:     2500,
		OpenedByLabel:     "staff:7",
		OpenedAt:          time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC),
	}

	raw, err := json.Marshal(session)
	require.NoError(t, err)

	payload := decodeCashRegisterJSON(t, raw)
	require.Equal(t, float64(125), payload["opening_float"])
	requireNoJSONKey(t, payload, "opening_float_cents")
	requireNoJSONKey(t, payload, "expected_cash")
	requireNoJSONKey(t, payload, "expected_cash_cents")
	requireNoJSONKey(t, payload, "counted_cash")
	requireNoJSONKey(t, payload, "counted_cash_cents")
	requireNoJSONKey(t, payload, "variance")
	requireNoJSONKey(t, payload, "variance_cents")
	requireJSONKeyNull(t, payload, "opened_by_user_id")
	requireJSONKeyNull(t, payload, "opened_by_staff_id")
	requireJSONKeyNull(t, payload, "closed_by_user_id")
	requireJSONKeyNull(t, payload, "closed_by_staff_id")
}

func TestCashRegisterSessionModelJSONIncludesClosedSnapshot(t *testing.T) {
	closedAt := time.Date(2026, 6, 27, 18, 0, 0, 0, time.UTC)
	session := database.CashRegisterSession{
		ID:                10,
		BusinessID:        20,
		Status:            database.CashRegisterSessionStatusClosed,
		OpeningFloatCents: 12500,
		CashSalesCents:    30000,
		CashRefundsCents:  2000,
		CashInCents:       5000,
		CashOutCents:      1000,
		ExpectedCashCents: 44500,
		CountedCashCents:  44000,
		VarianceCents:     -500,
		OpenedByLabel:     "staff:7",
		ClosedByLabel:     "staff:9",
		OpenedAt:          time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC),
		ClosedAt:          &closedAt,
	}

	raw, err := json.Marshal(session)
	require.NoError(t, err)

	payload := decodeCashRegisterJSON(t, raw)
	require.Equal(t, float64(445), payload["expected_cash"])
	require.Equal(t, float64(440), payload["counted_cash"])
	require.Equal(t, float64(-5), payload["variance"])
	requireNoJSONKey(t, payload, "expected_cash_cents")
	requireNoJSONKey(t, payload, "counted_cash_cents")
	requireNoJSONKey(t, payload, "variance_cents")
	requireJSONKeyNull(t, payload, "opened_by_user_id")
	requireJSONKeyNull(t, payload, "opened_by_staff_id")
	requireJSONKeyNull(t, payload, "closed_by_user_id")
	requireJSONKeyNull(t, payload, "closed_by_staff_id")
}

func TestCashRegisterMovementModelJSONUsesDollarsAndStableNullableKeys(t *testing.T) {
	movement := database.CashRegisterMovement{
		ID:           11,
		BusinessID:   20,
		SessionID:    30,
		MovementType: database.CashRegisterMovementTypeCashOut,
		AmountCents:  12345,
		Reason:       "bank drop",
		ActorLabel:   "staff:7",
		OccurredAt:   time.Date(2026, 6, 27, 16, 0, 0, 0, time.UTC),
	}

	raw, err := json.Marshal(movement)
	require.NoError(t, err)

	payload := decodeCashRegisterJSON(t, raw)
	require.Equal(t, float64(123.45), payload["amount"])
	requireNoJSONKey(t, payload, "amount_cents")
	requireJSONKeyNull(t, payload, "alternative_payment_id")
	requireJSONKeyNull(t, payload, "bill_id")
	requireJSONKeyNull(t, payload, "actor_user_id")
	requireJSONKeyNull(t, payload, "actor_staff_id")
}

func TestCashRegisterOneOpenSessionPerBusinessDatabaseInvariant(t *testing.T) {
	db := openCashRegisterSQLiteTestDB(t)
	now := time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC)

	first := database.CashRegisterSession{BusinessID: 1, Status: database.CashRegisterSessionStatusOpen, OpeningFloatCents: 10000, OpenedByLabel: "staff:1", OpenedAt: now}
	require.NoError(t, db.Create(&first).Error)

	second := database.CashRegisterSession{BusinessID: 1, Status: database.CashRegisterSessionStatusOpen, OpeningFloatCents: 20000, OpenedByLabel: "staff:2", OpenedAt: now}
	require.Error(t, db.Create(&second).Error)

	closed := database.CashRegisterSession{BusinessID: 1, Status: database.CashRegisterSessionStatusClosed, OpeningFloatCents: 20000, OpenedByLabel: "staff:2", OpenedAt: now, ClosedAt: &now}
	require.NoError(t, db.Create(&closed).Error)
}

func TestAttachCashRegisterMovementForAlternativePaymentCreatesCashSaleAndUpdatesOpenSessionTotals(t *testing.T) {
	db := openCashRegisterAttachSQLiteTestDB(t)
	business := createCashRegisterTestBusiness(t, db, "attach-sale")
	session := createCashRegisterTestOpenSession(t, db, business.ID, 5000)
	bill := createCashRegisterTestBill(t, db, business.ID, "attach-sale-1")
	payment := createCashRegisterTestAlternativePayment(t, db, bill.ID, database.PaymentMethodCash, database.AltPaymentStatusConfirmed, 1250)
	now := time.Date(2026, 6, 27, 13, 0, 0, 0, time.UTC)

	err := db.Transaction(func(tx *gorm.DB) error {
		return database.AttachCashRegisterMovementForAlternativePaymentTx(tx, business.ID, payment, database.CashRegisterMovementTypeCashSale, payment.Amount, database.CashRegisterActor{Label: "cashier"}, now)
	})
	require.NoError(t, err)

	var movements []database.CashRegisterMovement
	require.NoError(t, db.Find(&movements).Error)
	require.Len(t, movements, 1)
	require.Equal(t, business.ID, movements[0].BusinessID)
	require.Equal(t, session.ID, movements[0].SessionID)
	require.Equal(t, database.CashRegisterMovementTypeCashSale, movements[0].MovementType)
	require.Equal(t, int64(1250), movements[0].AmountCents)
	require.Equal(t, "cash payment", movements[0].Reason)
	require.Equal(t, payment.ID, *movements[0].AlternativePaymentID)
	require.Equal(t, bill.ID, *movements[0].BillID)
	require.Equal(t, "cashier", movements[0].ActorLabel)
	require.Equal(t, now, movements[0].OccurredAt)

	var reloaded database.CashRegisterSession
	require.NoError(t, db.First(&reloaded, session.ID).Error)
	require.Equal(t, int64(1250), reloaded.CashSalesCents)
	require.Equal(t, int64(0), reloaded.CashRefundsCents)
}

func TestAttachCashRegisterMovementForAlternativePaymentNoOpsWhenNoOpenSession(t *testing.T) {
	db := openCashRegisterAttachSQLiteTestDB(t)
	business := createCashRegisterTestBusiness(t, db, "attach-no-open")
	bill := createCashRegisterTestBill(t, db, business.ID, "attach-no-open-1")
	payment := createCashRegisterTestAlternativePayment(t, db, bill.ID, database.PaymentMethodCash, database.AltPaymentStatusConfirmed, 1250)

	err := db.Transaction(func(tx *gorm.DB) error {
		return database.AttachCashRegisterMovementForAlternativePaymentTx(tx, business.ID, payment, database.CashRegisterMovementTypeCashSale, payment.Amount, database.CashRegisterActor{Label: "cashier"}, time.Now())
	})
	require.NoError(t, err)

	var count int64
	require.NoError(t, db.Model(&database.CashRegisterMovement{}).Count(&count).Error)
	require.Equal(t, int64(0), count)
}

func TestAttachCashRegisterMovementForAlternativePaymentNoOpenSessionSkipsBillRead(t *testing.T) {
	db := openCashRegisterAttachSQLiteTestDB(t)
	business := createCashRegisterTestBusiness(t, db, "attach-no-open-missing-bill")
	payment := database.AlternativePayment{
		ID:            999,
		BillID:        999,
		Amount:        1250,
		PaymentMethod: database.PaymentMethodCash,
		Status:        database.AltPaymentStatusConfirmed,
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		return database.AttachCashRegisterMovementForAlternativePaymentTx(tx, business.ID, payment, database.CashRegisterMovementTypeCashSale, payment.Amount, database.CashRegisterActor{Label: "cashier"}, time.Now())
	})
	require.NoError(t, err)

	var count int64
	require.NoError(t, db.Model(&database.CashRegisterMovement{}).Count(&count).Error)
	require.Equal(t, int64(0), count)
}

func TestAttachCashRegisterMovementForAlternativePaymentNoOpsForNonCashPayment(t *testing.T) {
	db := openCashRegisterAttachSQLiteTestDB(t)
	business := createCashRegisterTestBusiness(t, db, "attach-card")
	createCashRegisterTestOpenSession(t, db, business.ID, 0)
	bill := createCashRegisterTestBill(t, db, business.ID, "attach-card-1")
	payment := createCashRegisterTestAlternativePayment(t, db, bill.ID, database.PaymentMethodCard, database.AltPaymentStatusConfirmed, 1250)

	err := db.Transaction(func(tx *gorm.DB) error {
		return database.AttachCashRegisterMovementForAlternativePaymentTx(tx, business.ID, payment, database.CashRegisterMovementTypeCashSale, payment.Amount, database.CashRegisterActor{Label: "cashier"}, time.Now())
	})
	require.NoError(t, err)

	var count int64
	require.NoError(t, db.Model(&database.CashRegisterMovement{}).Count(&count).Error)
	require.Equal(t, int64(0), count)
}

func TestAttachCashRegisterMovementForAlternativePaymentDuplicateAttachIsIdempotent(t *testing.T) {
	db := openCashRegisterAttachSQLiteTestDB(t)
	business := createCashRegisterTestBusiness(t, db, "attach-duplicate")
	session := createCashRegisterTestOpenSession(t, db, business.ID, 0)
	bill := createCashRegisterTestBill(t, db, business.ID, "attach-duplicate-1")
	payment := createCashRegisterTestAlternativePayment(t, db, bill.ID, database.PaymentMethodCash, database.AltPaymentStatusConfirmed, 1250)

	for i := 0; i < 2; i++ {
		err := db.Transaction(func(tx *gorm.DB) error {
			return database.AttachCashRegisterMovementForAlternativePaymentTx(tx, business.ID, payment, database.CashRegisterMovementTypeCashSale, payment.Amount, database.CashRegisterActor{Label: "cashier"}, time.Now())
		})
		require.NoError(t, err)
	}

	var count int64
	require.NoError(t, db.Model(&database.CashRegisterMovement{}).Count(&count).Error)
	require.Equal(t, int64(1), count)

	var reloaded database.CashRegisterSession
	require.NoError(t, db.First(&reloaded, session.ID).Error)
	require.Equal(t, int64(1250), reloaded.CashSalesCents)
}

func TestAttachCashRegisterMovementForAlternativePaymentRejectsWrongBusinessPaymentWithoutConsumingUniqueKey(t *testing.T) {
	db := openCashRegisterAttachSQLiteTestDB(t)
	businessA := createCashRegisterTestBusiness(t, db, "attach-wrong-a")
	businessB := createCashRegisterTestBusiness(t, db, "attach-wrong-b")
	sessionA := createCashRegisterTestOpenSession(t, db, businessA.ID, 0)
	createCashRegisterTestOpenSession(t, db, businessB.ID, 0)
	billA := createCashRegisterTestBill(t, db, businessA.ID, "attach-wrong-1")
	payment := createCashRegisterTestAlternativePayment(t, db, billA.ID, database.PaymentMethodCash, database.AltPaymentStatusConfirmed, 1250)

	err := db.Transaction(func(tx *gorm.DB) error {
		return database.AttachCashRegisterMovementForAlternativePaymentTx(tx, businessB.ID, payment, database.CashRegisterMovementTypeCashSale, payment.Amount, database.CashRegisterActor{Label: "cashier"}, time.Now())
	})
	require.Error(t, err)

	var count int64
	require.NoError(t, db.Model(&database.CashRegisterMovement{}).Count(&count).Error)
	require.Equal(t, int64(0), count)

	err = db.Transaction(func(tx *gorm.DB) error {
		return database.AttachCashRegisterMovementForAlternativePaymentTx(tx, businessA.ID, payment, database.CashRegisterMovementTypeCashSale, payment.Amount, database.CashRegisterActor{Label: "cashier"}, time.Now())
	})
	require.NoError(t, err)

	var movement database.CashRegisterMovement
	require.NoError(t, db.First(&movement).Error)
	require.Equal(t, businessA.ID, movement.BusinessID)
	require.Equal(t, sessionA.ID, movement.SessionID)
}

func TestAttachCashRegisterMovementForAlternativePaymentRejectsInvalidMovementType(t *testing.T) {
	db := openCashRegisterAttachSQLiteTestDB(t)
	business := createCashRegisterTestBusiness(t, db, "attach-invalid-type")
	createCashRegisterTestOpenSession(t, db, business.ID, 0)
	bill := createCashRegisterTestBill(t, db, business.ID, "attach-invalid-type-1")
	payment := createCashRegisterTestAlternativePayment(t, db, bill.ID, database.PaymentMethodCash, database.AltPaymentStatusConfirmed, 1250)

	err := db.Transaction(func(tx *gorm.DB) error {
		return database.AttachCashRegisterMovementForAlternativePaymentTx(tx, business.ID, payment, database.CashRegisterMovementTypeCashIn, payment.Amount, database.CashRegisterActor{Label: "cashier"}, time.Now())
	})
	require.Error(t, err)

	var count int64
	require.NoError(t, db.Model(&database.CashRegisterMovement{}).Count(&count).Error)
	require.Equal(t, int64(0), count)
}

func TestAttachCashRegisterMovementForAlternativePaymentDoesNotMutateClosedSession(t *testing.T) {
	db := openCashRegisterAttachSQLiteTestDB(t)
	business := createCashRegisterTestBusiness(t, db, "attach-closed")
	session := createCashRegisterTestClosedSession(t, db, business.ID, 1000, 1000)
	bill := createCashRegisterTestBill(t, db, business.ID, "attach-closed-1")
	payment := createCashRegisterTestAlternativePayment(t, db, bill.ID, database.PaymentMethodCash, database.AltPaymentStatusConfirmed, 1250)

	err := db.Transaction(func(tx *gorm.DB) error {
		return database.AttachCashRegisterMovementForAlternativePaymentTx(tx, business.ID, payment, database.CashRegisterMovementTypeCashSale, payment.Amount, database.CashRegisterActor{Label: "cashier"}, time.Now())
	})
	require.NoError(t, err)

	var count int64
	require.NoError(t, db.Model(&database.CashRegisterMovement{}).Count(&count).Error)
	require.Equal(t, int64(0), count)

	var reloaded database.CashRegisterSession
	require.NoError(t, db.First(&reloaded, session.ID).Error)
	require.Equal(t, int64(1000), reloaded.ExpectedCashCents)
	require.Equal(t, int64(1000), reloaded.CountedCashCents)
	require.Equal(t, int64(0), reloaded.CashSalesCents)
}

func TestUnassignedCashAlternativePaymentSummaryIgnoresMisScopedMovement(t *testing.T) {
	db := openCashRegisterAttachSQLiteTestDB(t)
	businessA := createCashRegisterTestBusiness(t, db, "unassigned-a")
	businessB := createCashRegisterTestBusiness(t, db, "unassigned-b")
	sessionB := createCashRegisterTestOpenSession(t, db, businessB.ID, 0)
	billA := createCashRegisterTestBill(t, db, businessA.ID, "unassigned-1")
	payment := createCashRegisterTestAlternativePayment(t, db, billA.ID, database.PaymentMethodCash, database.AltPaymentStatusConfirmed, 1250)
	misScoped := database.CashRegisterMovement{
		BusinessID:           businessB.ID,
		SessionID:            sessionB.ID,
		MovementType:         database.CashRegisterMovementTypeCashSale,
		AmountCents:          1250,
		Reason:               "cash payment",
		AlternativePaymentID: &payment.ID,
		BillID:               &billA.ID,
		ActorLabel:           "cashier",
		OccurredAt:           time.Now(),
	}
	require.NoError(t, db.Create(&misScoped).Error)

	count, total, err := database.UnassignedCashAlternativePaymentSummary(db, businessA.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
	require.Equal(t, int64(1250), total)
}

func TestUnassignedCashAlternativePaymentSummaryIncludesSplitTip(t *testing.T) {
	db := openCashRegisterAttachSQLiteTestDB(t)
	business := createCashRegisterTestBusiness(t, db, "unassigned-tip")
	bill := createCashRegisterTestBill(t, db, business.ID, "unassigned-tip-1")
	payment := createCashRegisterTestAlternativePayment(t, db, bill.ID, database.PaymentMethodCash, database.AltPaymentStatusConfirmed, 4000)
	require.NoError(t, db.Model(&database.AlternativePayment{}).
		Where("id = ?", payment.ID).
		Updates(map[string]interface{}{
			"bill_amount_cents": int64(4000),
			"tip_amount_cents":  int64(650),
		}).Error)

	count, total, err := database.UnassignedCashAlternativePaymentSummary(db, business.ID)

	require.NoError(t, err)
	require.Equal(t, int64(1), count)
	require.Equal(t, int64(4650), total)
}

func TestUnassignedCashAlternativePaymentSummaryIncludesRefundMissingRefundMovement(t *testing.T) {
	db := openCashRegisterAttachSQLiteTestDB(t)
	business := createCashRegisterTestBusiness(t, db, "unassigned-refund")
	session := createCashRegisterTestClosedSession(t, db, business.ID, 1250, 1250)
	bill := createCashRegisterTestBill(t, db, business.ID, "unassigned-refund-1")
	payment := createCashRegisterTestAlternativePayment(t, db, bill.ID, database.PaymentMethodCash, database.AltPaymentStatusRefunded, 1250)
	saleMovement := database.CashRegisterMovement{
		BusinessID:           business.ID,
		SessionID:            session.ID,
		MovementType:         database.CashRegisterMovementTypeCashSale,
		AmountCents:          1250,
		Reason:               "cash payment",
		AlternativePaymentID: &payment.ID,
		BillID:               &bill.ID,
		ActorLabel:           "cashier",
		OccurredAt:           time.Now(),
	}
	require.NoError(t, db.Create(&saleMovement).Error)

	count, total, err := database.UnassignedCashAlternativePaymentSummary(db, business.ID)

	require.NoError(t, err)
	require.Equal(t, int64(1), count)
	require.Equal(t, int64(-1250), total)
}

func TestUnassignedCashAlternativePaymentSummaryKeepsRefundedSaleWhenNoMovementsExist(t *testing.T) {
	db := openCashRegisterAttachSQLiteTestDB(t)
	business := createCashRegisterTestBusiness(t, db, "unassigned-refunded-no-movements")
	bill := createCashRegisterTestBill(t, db, business.ID, "unassigned-refunded-no-movements-1")
	createCashRegisterTestAlternativePayment(t, db, bill.ID, database.PaymentMethodCash, database.AltPaymentStatusRefunded, 1250)

	count, total, err := database.UnassignedCashAlternativePaymentSummary(db, business.ID)

	require.NoError(t, err)
	require.Equal(t, int64(2), count)
	require.Equal(t, int64(0), total)
}

func TestUnassignedCashAlternativePaymentSummaryKeepsRefundedSaleWhenRefundMovementExists(t *testing.T) {
	db := openCashRegisterAttachSQLiteTestDB(t)
	business := createCashRegisterTestBusiness(t, db, "unassigned-refunded-refund-movement")
	session := createCashRegisterTestOpenSession(t, db, business.ID, 0)
	bill := createCashRegisterTestBill(t, db, business.ID, "unassigned-refunded-refund-movement-1")
	payment := createCashRegisterTestAlternativePayment(t, db, bill.ID, database.PaymentMethodCash, database.AltPaymentStatusRefunded, 1250)
	refundMovement := database.CashRegisterMovement{
		BusinessID:           business.ID,
		SessionID:            session.ID,
		MovementType:         database.CashRegisterMovementTypeCashRefund,
		AmountCents:          1250,
		Reason:               "cash refund",
		AlternativePaymentID: &payment.ID,
		BillID:               &bill.ID,
		ActorLabel:           "cashier",
		OccurredAt:           time.Now(),
	}
	require.NoError(t, db.Create(&refundMovement).Error)

	count, total, err := database.UnassignedCashAlternativePaymentSummary(db, business.ID)

	require.NoError(t, err)
	require.Equal(t, int64(1), count)
	require.Equal(t, int64(1250), total)
}

// TestListUnassignedCashAlternativePaymentsMatchesSummary asserts the list
// endpoint returns the same rows the summary counts, is bounded by limit, and
// reports the true total.
func TestListUnassignedCashAlternativePaymentsMatchesSummary(t *testing.T) {
	db := openCashRegisterAttachSQLiteTestDB(t)
	business := createCashRegisterTestBusiness(t, db, "unassigned-list")

	// Seed 3 unassigned confirmed cash tenders.
	for i := 0; i < 3; i++ {
		bill := createCashRegisterTestBill(t, db, business.ID, fmt.Sprintf("unassigned-list-%d", i))
		createCashRegisterTestAlternativePayment(t, db, bill.ID, database.PaymentMethodCash, database.AltPaymentStatusConfirmed, int64(1000+i*100))
	}

	// Summary agrees on the count.
	count, _, err := database.UnassignedCashAlternativePaymentSummary(db, business.ID)
	require.NoError(t, err)
	require.Equal(t, int64(3), count)

	// Full list.
	items, total, err := database.ListUnassignedCashAlternativePayments(db, business.ID, 50, 0)
	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Len(t, items, 3)
	require.NotEmpty(t, items[0].BillNumber)
	require.Greater(t, items[0].AmountCents, int64(0))

	// Bounded page + offset.
	page, total2, err := database.ListUnassignedCashAlternativePayments(db, business.ID, 2, 0)
	require.NoError(t, err)
	require.Equal(t, int64(3), total2, "total reflects the whole set, not the page")
	require.Len(t, page, 2, "page must be bounded by limit")

	page2, _, err := database.ListUnassignedCashAlternativePayments(db, business.ID, 2, 2)
	require.NoError(t, err)
	require.Len(t, page2, 1, "second page returns the remainder")
	require.NotEqual(t, page[0].ID, page2[0].ID)
}

// TestListUnassignedCashAlternativePaymentsExcludesAssigned asserts a tender that
// already has a cash-sale movement recorded is excluded from the list (matches
// the summary's crm.id IS NULL predicate).
func TestListUnassignedCashAlternativePaymentsExcludesAssigned(t *testing.T) {
	db := openCashRegisterAttachSQLiteTestDB(t)
	business := createCashRegisterTestBusiness(t, db, "unassigned-list-assigned")
	session := createCashRegisterTestOpenSession(t, db, business.ID, 0)
	bill := createCashRegisterTestBill(t, db, business.ID, "assigned-1")
	payment := createCashRegisterTestAlternativePayment(t, db, bill.ID, database.PaymentMethodCash, database.AltPaymentStatusConfirmed, 1500)
	// Record the cash-sale movement → this tender is now assigned.
	require.NoError(t, db.Create(&database.CashRegisterMovement{
		BusinessID:           business.ID,
		SessionID:            session.ID,
		MovementType:         database.CashRegisterMovementTypeCashSale,
		AmountCents:          1500,
		Reason:               "cash payment",
		AlternativePaymentID: &payment.ID,
		BillID:               &bill.ID,
		ActorLabel:           "cashier",
		OccurredAt:           time.Now(),
	}).Error)

	items, total, err := database.ListUnassignedCashAlternativePayments(db, business.ID, 50, 0)
	require.NoError(t, err)
	require.Equal(t, int64(0), total)
	require.Len(t, items, 0)
}

// #652: the next opening float is the last declared starting bank, never the
// counted close that already includes a shortage (1473.07 counted, short 1.83).
func TestFindLastDeclaredOpeningFloatCentsUsesOpeningBankNotCountedShortage(t *testing.T) {
	db := openCashRegisterSQLiteTestDB(t)
	const businessID uint = 41
	const otherBusinessID uint = 42

	olderClosedAt := time.Date(2026, 6, 26, 20, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&database.CashRegisterSession{
		BusinessID:        businessID,
		Status:            database.CashRegisterSessionStatusClosed,
		OpeningFloatCents: 90000,
		ExpectedCashCents: 95000,
		CountedCashCents:  95000,
		OpenedByLabel:     "cashier",
		OpenedAt:          olderClosedAt.Add(-8 * time.Hour),
		ClosedByLabel:     "cashier",
		ClosedAt:          &olderClosedAt,
	}).Error)

	shortageClosedAt := time.Date(2026, 6, 27, 20, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&database.CashRegisterSession{
		BusinessID:        businessID,
		Status:            database.CashRegisterSessionStatusClosed,
		OpeningFloatCents: 20000,
		ExpectedCashCents: 147490,
		CountedCashCents:  147307,
		VarianceCents:     -183,
		OpenedByLabel:     "cashier",
		OpenedAt:          shortageClosedAt.Add(-8 * time.Hour),
		ClosedByLabel:     "cashier",
		ClosedAt:          &shortageClosedAt,
	}).Error)

	otherClosedAt := time.Date(2026, 6, 28, 20, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&database.CashRegisterSession{
		BusinessID:        otherBusinessID,
		Status:            database.CashRegisterSessionStatusClosed,
		OpeningFloatCents: 50000,
		CountedCashCents:  50000,
		OpenedByLabel:     "cashier",
		OpenedAt:          otherClosedAt.Add(-8 * time.Hour),
		ClosedByLabel:     "cashier",
		ClosedAt:          &otherClosedAt,
	}).Error)

	cents, err := database.FindLastDeclaredOpeningFloatCentsTx(db, businessID)
	require.NoError(t, err)
	require.NotNil(t, cents)
	require.Equal(t, int64(20000), *cents)

	empty, err := database.FindLastDeclaredOpeningFloatCentsTx(db, businessID+100)
	require.NoError(t, err)
	require.Nil(t, empty)
}

func TestFindLastDeclaredOpeningFloatCentsQueryShapeOmitsCountedCash(t *testing.T) {
	db := openCashRegisterSQLiteTestDB(t)
	const businessID uint = 43
	closedAt := time.Date(2026, 6, 27, 20, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&database.CashRegisterSession{
		BusinessID:        businessID,
		Status:            database.CashRegisterSessionStatusClosed,
		OpeningFloatCents: 20000,
		ExpectedCashCents: 147490,
		CountedCashCents:  147307,
		VarianceCents:     -183,
		OpenedByLabel:     "cashier",
		OpenedAt:          closedAt.Add(-8 * time.Hour),
		ClosedByLabel:     "cashier",
		ClosedAt:          &closedAt,
	}).Error)

	var sqls []string
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("caja652:capture_sql", func(tx *gorm.DB) {
		if tx.Statement != nil {
			sqls = append(sqls, strings.ToLower(tx.Statement.SQL.String()))
		}
	}))
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove("caja652:capture_sql")
	})

	cents, err := database.FindLastDeclaredOpeningFloatCentsTx(db, businessID)
	require.NoError(t, err)
	require.NotNil(t, cents)
	require.Equal(t, int64(20000), *cents)
	require.NotEmpty(t, sqls)

	joined := strings.Join(sqls, " ")
	require.Contains(t, joined, "opening_float_cents")
	require.NotContains(t, joined, "counted_cash_cents")
	require.NotContains(t, joined, "expected_cash_cents")
	require.NotContains(t, joined, "select *")
}

func TestBusinessUsesDemoHouseRailTx(t *testing.T) {
	db := openCashRegisterAttachSQLiteTestDB(t)
	realBiz := createCashRegisterTestBusiness(t, db, "real-house-rail")
	demoFlag := createCashRegisterTestBusiness(t, db, "demo-flag-house-rail")
	require.NoError(t, db.Model(&demoFlag).Updates(map[string]any{
		"is_demo": true,
		"kind":    database.BusinessKindReal,
	}).Error)
	demoKind := createCashRegisterTestBusiness(t, db, "demo-kind-house-rail")
	require.NoError(t, db.Model(&demoKind).Update("kind", database.BusinessKindDemo).Error)

	uses, err := database.BusinessUsesDemoHouseRailTx(db, realBiz.ID)
	require.NoError(t, err)
	require.False(t, uses)

	uses, err = database.BusinessUsesDemoHouseRailTx(db, demoFlag.ID)
	require.NoError(t, err)
	require.False(t, uses, "sticky is_demo=true on kind=real must not open the house rail")

	uses, err = database.BusinessUsesDemoHouseRailTx(db, demoKind.ID)
	require.NoError(t, err)
	require.True(t, uses)

	uses, err = database.BusinessUsesDemoHouseRailTx(db, 999999)
	require.NoError(t, err)
	require.False(t, uses)
}

func TestHasHumanClosedCashRegisterSessionTx(t *testing.T) {
	db := openCashRegisterAttachSQLiteTestDB(t)
	business := createCashRegisterTestBusiness(t, db, "human-close-marker")
	seedClosedAt := time.Date(2026, 8, 21, 4, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&database.CashRegisterSession{
		BusinessID:        business.ID,
		Status:            database.CashRegisterSessionStatusClosed,
		OpeningFloatCents: 10000,
		OpenedByLabel:     "demo-manager",
		OpenedAt:          seedClosedAt.Add(-2 * time.Hour),
		ClosedByLabel:     "demo-manager",
		ClosedAt:          &seedClosedAt,
	}).Error)

	has, err := database.HasHumanClosedCashRegisterSessionTx(db, business.ID)
	require.NoError(t, err)
	require.False(t, has)

	userID := uint(42)
	humanClosedAt := time.Date(2026, 8, 21, 16, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&database.CashRegisterSession{
		BusinessID:        business.ID,
		Status:            database.CashRegisterSessionStatusClosed,
		OpeningFloatCents: 10000,
		OpenedByLabel:     "manager",
		OpenedAt:          humanClosedAt.Add(-8 * time.Hour),
		ClosedByUserID:    &userID,
		ClosedByLabel:     "manager",
		ClosedAt:          &humanClosedAt,
	}).Error)

	has, err = database.HasHumanClosedCashRegisterSessionTx(db, business.ID)
	require.NoError(t, err)
	require.True(t, has)
}

func openCashRegisterSQLiteTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:"+dsnName+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	require.NoError(t, db.AutoMigrate(&database.CashRegisterSession{}))
	assertSQLiteIndexExists(t, db, "idx_cash_register_sessions_one_open_per_business")

	return db
}

func openCashRegisterAttachSQLiteTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	db, err := gorm.Open(sqlite.Open("file:"+dsnName+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	require.NoError(t, db.AutoMigrate(
		&database.Business{},
		&database.Bill{},
		&database.AlternativePayment{},
		&database.CashRegisterSession{},
		&database.CashRegisterMovement{},
	))
	return db
}

func createCashRegisterTestBusiness(t *testing.T, db *gorm.DB, businessID string) database.Business {
	t.Helper()

	business := database.Business{
		BusinessId:     businessID,
		OwnerAddress:   "owner-" + businessID,
		Name:           businessID,
		SettlementAddr: "settlement-" + businessID,
		TippingAddr:    "tipping-" + businessID,
	}
	require.NoError(t, db.Create(&business).Error)
	return business
}

func createCashRegisterTestOpenSession(t *testing.T, db *gorm.DB, businessID uint, openingFloatCents int64) database.CashRegisterSession {
	t.Helper()

	session := database.CashRegisterSession{
		BusinessID:        businessID,
		Status:            database.CashRegisterSessionStatusOpen,
		OpeningFloatCents: openingFloatCents,
		OpenedByLabel:     "cashier",
		OpenedAt:          time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC),
	}
	require.NoError(t, db.Create(&session).Error)
	return session
}

func createCashRegisterTestClosedSession(t *testing.T, db *gorm.DB, businessID uint, expectedCashCents int64, countedCashCents int64) database.CashRegisterSession {
	t.Helper()

	openedAt := time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC)
	closedAt := openedAt.Add(8 * time.Hour)
	session := database.CashRegisterSession{
		BusinessID:        businessID,
		Status:            database.CashRegisterSessionStatusClosed,
		OpeningFloatCents: expectedCashCents,
		ExpectedCashCents: expectedCashCents,
		CountedCashCents:  countedCashCents,
		VarianceCents:     countedCashCents - expectedCashCents,
		OpenedByLabel:     "cashier",
		OpenedAt:          openedAt,
		ClosedByLabel:     "cashier",
		ClosedAt:          &closedAt,
	}
	require.NoError(t, db.Create(&session).Error)
	return session
}

func createCashRegisterTestBill(t *testing.T, db *gorm.DB, businessID uint, billNumber string) database.Bill {
	t.Helper()

	bill := database.Bill{
		BusinessID:     businessID,
		BillNumber:     billNumber,
		SettlementAddr: "settlement",
		TippingAddr:    "tipping",
		Status:         database.BillStatusPaid,
		TotalAmount:    10000,
		PaidAmount:     10000,
	}
	require.NoError(t, db.Create(&bill).Error)
	return bill
}

func createCashRegisterTestAlternativePayment(t *testing.T, db *gorm.DB, billID uint, method database.AlternativePaymentMethod, status database.AlternativePaymentStatus, amountCents int64) database.AlternativePayment {
	t.Helper()

	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	payment := database.AlternativePayment{
		BillID:          billID,
		ParticipantAddr: "cashier",
		ParticipantName: "Cashier",
		Amount:          amountCents,
		BillAmountCents: amountCents,
		PaymentMethod:   method,
		Status:          status,
		ConfirmedBy:     "cashier",
		ConfirmedAt:     &now,
	}
	require.NoError(t, db.Create(&payment).Error)
	return payment
}

func decodeCashRegisterJSON(t *testing.T, raw []byte) map[string]interface{} {
	t.Helper()

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &payload))
	return payload
}

func requireNoJSONKey(t *testing.T, payload map[string]interface{}, key string) {
	t.Helper()

	_, ok := payload[key]
	require.False(t, ok, "JSON key %q must be absent", key)
}

func requireJSONKeyNull(t *testing.T, payload map[string]interface{}, key string) {
	t.Helper()

	value, ok := payload[key]
	require.True(t, ok, "JSON key %q must be present", key)
	require.Nil(t, value, "JSON key %q must be null", key)
}

func assertSQLiteIndexExists(t *testing.T, db *gorm.DB, name string) {
	t.Helper()

	var count int64
	require.NoError(t, db.Raw(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, name).Scan(&count).Error)
	require.Equal(t, int64(1), count, "SQLite AutoMigrate must create %s", name)
}
