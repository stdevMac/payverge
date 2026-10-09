package database

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupPaymentLedgerTestDB(t *testing.T) *DB {
	t.Helper()
	return setupPaymentLedgerTestDBWithLogger(t, nil)
}

func setupPaymentLedgerTestDBWithLogger(t *testing.T, gormLogger logger.Interface) *DB {
	t.Helper()

	config := &gorm.Config{}
	if gormLogger != nil {
		config.Logger = gormLogger
	}

	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), config)
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, gormDB.AutoMigrate(
		&Business{},
		&Bill{},
		&Payment{},
		&AlternativePayment{},
		&BillSplitShare{},
		&BusinessMilestoneEvent{},
		&BusinessRevenueAggregate{},
		&CashRegisterSession{},
		&CashRegisterMovement{},
	))

	SetTestDB(gormDB)
	return GetDBWrapper()
}

type recordingPaymentLedgerLogger struct {
	mu   sync.Mutex
	sqls []string
}

func (l *recordingPaymentLedgerLogger) LogMode(logger.LogLevel) logger.Interface {
	return l
}

func (l *recordingPaymentLedgerLogger) Info(context.Context, string, ...interface{}) {}

func (l *recordingPaymentLedgerLogger) Warn(context.Context, string, ...interface{}) {}

func (l *recordingPaymentLedgerLogger) Error(context.Context, string, ...interface{}) {}

func (l *recordingPaymentLedgerLogger) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sqls = append(l.sqls, sql)
}

func (l *recordingPaymentLedgerLogger) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sqls = nil
}

func (l *recordingPaymentLedgerLogger) SQL() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.sqls, "\n")
}

func createPaymentLedgerBusiness(t *testing.T, db *DB) *Business {
	t.Helper()

	suffix := fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())
	business := &Business{
		BusinessId:     "ledger-" + suffix,
		OwnerAddress:   "owner-" + suffix,
		Name:           "Ledger Test",
		SettlementAddr: "settlement-" + suffix,
		TippingAddr:    "tipping-" + suffix,
	}
	require.NoError(t, db.GetGorm().Create(business).Error)
	return business
}

func createPaymentLedgerBill(t *testing.T, db *DB, businessID uint, number string, total, paid, tip int64, status BillStatus, createdAt, updatedAt time.Time, closedAt *time.Time) *Bill {
	t.Helper()

	bill := &Bill{
		BusinessID:     businessID,
		BillNumber:     number,
		Items:          "[]",
		Subtotal:       total,
		TotalAmount:    total,
		PaidAmount:     paid,
		TipAmount:      tip,
		Status:         status,
		SettlementAddr: "settlement-" + number,
		TippingAddr:    "tipping-" + number,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
		ClosedAt:       closedAt,
	}
	require.NoError(t, db.GetGorm().Create(bill).Error)
	return bill
}

func TestRecognizedPaymentEvents_PartialPaymentsUsePaymentAmounts(t *testing.T) {
	db := setupPaymentLedgerTestDB(t)
	business := createPaymentLedgerBusiness(t, db)
	start := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	confirmedAt := start.Add(2 * time.Hour)

	bill := createPaymentLedgerBill(t, db, business.ID, "ledger-partial", 10000, 3000, 450, BillStatusPartial, start.Add(-time.Hour), confirmedAt, nil)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xguest",
		Amount:        3000,
		TipAmount:     450,
		TxHash:        "ledger_partial_payment",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     start.Add(-30 * time.Minute),
		UpdatedAt:     confirmedAt,
	}).Error)

	events, err := GetRecognizedPaymentEvents(business.ID, start, end)
	require.NoError(t, err)
	require.Len(t, events, 1)

	assert.Equal(t, bill.ID, events[0].BillID)
	assert.Equal(t, business.ID, events[0].BusinessID)
	assert.Equal(t, int64(3000), events[0].AmountCents)
	assert.NotEqual(t, int64(10000), events[0].AmountCents)
	assert.Equal(t, int64(450), events[0].TipCents)
	assert.Equal(t, "crypto", events[0].PaymentMethod)
	assert.Equal(t, "payment", events[0].Source)
	assert.Equal(t, confirmedAt, events[0].RecognizedAt)
	assert.Equal(t, bill.BillNumber, events[0].BillNumber)
	assert.Equal(t, BillStatusPartial, events[0].BillStatus)
	assert.Equal(t, int64(10000), events[0].BillTotalCents)
	assert.Equal(t, bill.CreatedAt, events[0].BillCreatedAt)
}

func TestRecognizedPaymentEvents_ExcludesPluginAlternativeTrackingAndCountsPluginPayments(t *testing.T) {
	db := setupPaymentLedgerTestDB(t)
	business := createPaymentLedgerBusiness(t, db)
	start := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	confirmedAt := start.Add(4 * time.Hour)

	bill := createPaymentLedgerBill(t, db, business.ID, "ledger-plugin", 6200, 6200, 0, BillStatusPaid, start.Add(-time.Hour), confirmedAt, nil)
	require.NoError(t, db.GetGorm().Create(&AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "cash-guest",
		Amount:          1200,
		PaymentMethod:   PaymentMethodCash,
		Status:          AltPaymentStatusConfirmed,
		ConfirmedBy:     "staff",
		CreatedAt:       confirmedAt.Add(-time.Minute),
		UpdatedAt:       confirmedAt,
		ConfirmedAt:     &confirmedAt,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "plugin-paypal-guest",
		Amount:          5000,
		PaymentMethod:   AlternativePaymentMethod("paypal"),
		Status:          AltPaymentStatusConfirmed,
		ConfirmedBy:     "plugin",
		CreatedAt:       confirmedAt.Add(-time.Minute),
		UpdatedAt:       confirmedAt,
		ConfirmedAt:     &confirmedAt,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        bill.ID,
		PayerAddr:     "plugin",
		Amount:        5000,
		TxHash:        "plugin_paypal_settlement_1",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "usd",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt.Add(-time.Minute),
		UpdatedAt:     confirmedAt,
	}).Error)

	events, err := GetRecognizedPaymentEvents(business.ID, start, end)
	require.NoError(t, err)
	require.Len(t, events, 2)

	var cashEvent, pluginEvent *RecognizedPaymentEvent
	for i := range events {
		switch events[i].PaymentMethod {
		case "cash":
			cashEvent = &events[i]
		case "plugin":
			pluginEvent = &events[i]
		default:
			require.NotEqual(t, "paypal", events[i].PaymentMethod)
		}
	}

	require.NotNil(t, cashEvent)
	assert.Equal(t, int64(1200), cashEvent.AmountCents)
	assert.Equal(t, "alternative_payment", cashEvent.Source)

	require.NotNil(t, pluginEvent)
	assert.Equal(t, int64(5000), pluginEvent.AmountCents)
	assert.Equal(t, "plugin", pluginEvent.PaymentMethod)
	assert.Equal(t, "payment", pluginEvent.Source)
}

func TestRecognizedPaymentEvents_ReversedPaymentsEmitNegativeEventsAtReversalTime(t *testing.T) {
	db := setupPaymentLedgerTestDB(t)
	business := createPaymentLedgerBusiness(t, db)
	start := time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	confirmedAt := start.Add(-time.Hour)
	reversedAt := start.Add(6 * time.Hour)

	bill := createPaymentLedgerBill(t, db, business.ID, "ledger-reversal", 5000, 0, 0, BillStatusOpen, start.Add(-time.Hour), reversedAt, nil)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xguest",
		Amount:        2000,
		TipAmount:     300,
		TxHash:        "ledger_reversed_payment",
		Status:        PaymentStatusReversed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		ReversedAt:    &reversedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     reversedAt,
	}).Error)

	events, err := GetRecognizedPaymentEvents(business.ID, start, end)
	require.NoError(t, err)
	require.Len(t, events, 1)

	assert.Equal(t, bill.ID, events[0].BillID)
	assert.Equal(t, int64(-2000), events[0].AmountCents)
	assert.Equal(t, int64(-300), events[0].TipCents)
	assert.Equal(t, "reversal", events[0].Source)
	assert.Equal(t, reversedAt, events[0].RecognizedAt)
}

func TestRecognizedPaymentEvents_ReversedPaymentKeepsOriginalRecognitionAndNetsWithReversal(t *testing.T) {
	db := setupPaymentLedgerTestDB(t)
	business := createPaymentLedgerBusiness(t, db)
	confirmationDay := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC)
	reversalDay := confirmationDay.AddDate(0, 0, 1)
	end := reversalDay.AddDate(0, 0, 1)
	confirmedAt := confirmationDay.Add(9 * time.Hour)
	reversedAt := reversalDay.Add(11 * time.Hour)

	bill := createPaymentLedgerBill(t, db, business.ID, "ledger-reversed-lifecycle", 2600, 0, 0, BillStatusOpen, confirmationDay.Add(-time.Hour), reversedAt, nil)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xreversed",
		Amount:        2600,
		TipAmount:     200,
		TxHash:        "ledger_reversed_lifecycle",
		Status:        PaymentStatusReversed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		ReversedAt:    &reversedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     reversedAt,
	}).Error)

	confirmationEvents, err := GetRecognizedPaymentEvents(business.ID, confirmationDay, reversalDay)
	require.NoError(t, err)
	require.Len(t, confirmationEvents, 1)
	assert.Equal(t, int64(2600), confirmationEvents[0].AmountCents)
	assert.Equal(t, int64(200), confirmationEvents[0].TipCents)
	assert.Equal(t, "payment", confirmationEvents[0].Source)
	assert.Equal(t, confirmedAt, confirmationEvents[0].RecognizedAt)

	reversalEvents, err := GetRecognizedPaymentEvents(business.ID, reversalDay, end)
	require.NoError(t, err)
	require.Len(t, reversalEvents, 1)
	assert.Equal(t, int64(-2600), reversalEvents[0].AmountCents)
	assert.Equal(t, int64(-200), reversalEvents[0].TipCents)
	assert.Equal(t, "reversal", reversalEvents[0].Source)
	assert.Equal(t, reversedAt, reversalEvents[0].RecognizedAt)

	allEvents, err := GetRecognizedPaymentEvents(business.ID, confirmationDay, end)
	require.NoError(t, err)
	require.Len(t, allEvents, 2)
	assert.Equal(t, int64(0), sumPaymentLedgerEventAmounts(allEvents))

	eventSummary := SummarizeRecognizedPaymentEvents(allEvents)
	sqlSummary, err := GetRecognizedPaymentSummary(business.ID, confirmationDay, end)
	require.NoError(t, err)
	assertRecognizedPaymentSummariesEqual(t, eventSummary, sqlSummary)
	assert.Equal(t, int64(0), sqlSummary.TotalRevenueCents)
	assert.Equal(t, int64(0), sqlSummary.TotalTipCents)
	assert.Equal(t, int64(2), sqlSummary.EventCount)
}

func TestRecognizedPaymentEvents_LegacyReversedPaymentUsesCreatedAtRecognitionAndNetsWithReversal(t *testing.T) {
	db := setupPaymentLedgerTestDB(t)
	business := createPaymentLedgerBusiness(t, db)
	createdDay := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	reversalDay := createdDay.AddDate(0, 0, 1)
	end := reversalDay.AddDate(0, 0, 1)
	createdAt := createdDay.Add(8 * time.Hour)
	reversedAt := reversalDay.Add(10 * time.Hour)

	bill := createPaymentLedgerBill(t, db, business.ID, "ledger-legacy-reversed-lifecycle", 3100, 0, 0, BillStatusOpen, createdDay.Add(-time.Hour), reversedAt, nil)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xlegacyreversed",
		Amount:        3100,
		TipAmount:     250,
		TxHash:        "ledger_legacy_reversed_lifecycle",
		Status:        PaymentStatusReversed,
		PaymentMethod: "crypto",
		ConfirmedAt:   nil,
		ReversedAt:    &reversedAt,
		CreatedAt:     createdAt,
		UpdatedAt:     reversedAt,
	}).Error)

	createdEvents, err := GetRecognizedPaymentEvents(business.ID, createdDay, reversalDay)
	require.NoError(t, err)
	require.Len(t, createdEvents, 1)
	assert.Equal(t, int64(3100), createdEvents[0].AmountCents)
	assert.Equal(t, int64(250), createdEvents[0].TipCents)
	assert.Equal(t, "payment", createdEvents[0].Source)
	assert.Equal(t, createdAt, createdEvents[0].RecognizedAt)

	reversalEvents, err := GetRecognizedPaymentEvents(business.ID, reversalDay, end)
	require.NoError(t, err)
	require.Len(t, reversalEvents, 1)
	assert.Equal(t, int64(-3100), reversalEvents[0].AmountCents)
	assert.Equal(t, int64(-250), reversalEvents[0].TipCents)
	assert.Equal(t, "reversal", reversalEvents[0].Source)
	assert.Equal(t, reversedAt, reversalEvents[0].RecognizedAt)

	allEvents, err := GetRecognizedPaymentEvents(business.ID, createdDay, end)
	require.NoError(t, err)
	require.Len(t, allEvents, 2)
	assert.Equal(t, int64(0), sumPaymentLedgerEventAmounts(allEvents))

	eventSummary := SummarizeRecognizedPaymentEvents(allEvents)
	sqlSummary, err := GetRecognizedPaymentSummary(business.ID, createdDay, end)
	require.NoError(t, err)
	assertRecognizedPaymentSummariesEqual(t, eventSummary, sqlSummary)
	assert.Equal(t, int64(0), sqlSummary.TotalRevenueCents)
	assert.Equal(t, int64(0), sqlSummary.TotalTipCents)
	assert.Equal(t, int64(2), sqlSummary.EventCount)
}

// TestRecognizedPaymentEvents_RefundedPaymentKeepsOriginalRecognitionAndNetsWithRefund
// locks AN-MON-1: an operator-refunded payment (status=refunded, reversed_at set)
// must behave exactly like a chain-reversed payment — the original revenue stays
// recognized in the period it was earned, and a dated negative event books the
// refund in the period it occurred. Before the fix, a refunded payment was
// excluded from every recognition branch, so the sale vanished retroactively and
// no offsetting negative was ever booked.
func TestRecognizedPaymentEvents_RefundedPaymentKeepsOriginalRecognitionAndNetsWithRefund(t *testing.T) {
	db := setupPaymentLedgerTestDB(t)
	business := createPaymentLedgerBusiness(t, db)
	confirmationDay := time.Date(2026, 5, 17, 0, 0, 0, 0, time.UTC)
	refundDay := confirmationDay.AddDate(0, 0, 1)
	end := refundDay.AddDate(0, 0, 1)
	confirmedAt := confirmationDay.Add(9 * time.Hour)
	refundedAt := refundDay.Add(11 * time.Hour)

	bill := createPaymentLedgerBill(t, db, business.ID, "ledger-refunded-lifecycle", 4000, 0, 0, BillStatusOpen, confirmationDay.Add(-time.Hour), refundedAt, nil)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xrefunded",
		Amount:        4000,
		TipAmount:     350,
		TxHash:        "ledger_refunded_lifecycle",
		Status:        PaymentStatusRefunded,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		ReversedAt:    &refundedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     refundedAt,
	}).Error)

	confirmationEvents, err := GetRecognizedPaymentEvents(business.ID, confirmationDay, refundDay)
	require.NoError(t, err)
	require.Len(t, confirmationEvents, 1)
	assert.Equal(t, int64(4000), confirmationEvents[0].AmountCents)
	assert.Equal(t, int64(350), confirmationEvents[0].TipCents)
	assert.Equal(t, "payment", confirmationEvents[0].Source)
	assert.Equal(t, confirmedAt, confirmationEvents[0].RecognizedAt)

	refundEvents, err := GetRecognizedPaymentEvents(business.ID, refundDay, end)
	require.NoError(t, err)
	require.Len(t, refundEvents, 1)
	assert.Equal(t, int64(-4000), refundEvents[0].AmountCents)
	assert.Equal(t, int64(-350), refundEvents[0].TipCents)
	assert.Equal(t, "reversal", refundEvents[0].Source)
	assert.Equal(t, refundedAt, refundEvents[0].RecognizedAt)

	allEvents, err := GetRecognizedPaymentEvents(business.ID, confirmationDay, end)
	require.NoError(t, err)
	require.Len(t, allEvents, 2)
	assert.Equal(t, int64(0), sumPaymentLedgerEventAmounts(allEvents))

	eventSummary := SummarizeRecognizedPaymentEvents(allEvents)
	sqlSummary, err := GetRecognizedPaymentSummary(business.ID, confirmationDay, end)
	require.NoError(t, err)
	assertRecognizedPaymentSummariesEqual(t, eventSummary, sqlSummary)
	assert.Equal(t, int64(0), sqlSummary.TotalRevenueCents)
	assert.Equal(t, int64(0), sqlSummary.TotalTipCents)
	assert.Equal(t, int64(2), sqlSummary.EventCount)
}

// TestRecognizedPaymentEvents_RefundedAlternativePaymentKeepsOriginalRecognitionAndNetsWithRefund
// locks AN-MON-1 for bill-managed alternative payments (cash/card/venmo/other).
// Alt-payments have no reversal timestamp and are never chain-reversed, only
// operator-refunded (status=refunded, updated_at = refund time). The original
// confirmed amount stays recognized on confirmed_at; a new negative branch books
// the refund on updated_at.
func TestRecognizedPaymentEvents_RefundedAlternativePaymentKeepsOriginalRecognitionAndNetsWithRefund(t *testing.T) {
	db := setupPaymentLedgerTestDB(t)
	business := createPaymentLedgerBusiness(t, db)
	confirmationDay := time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)
	refundDay := confirmationDay.AddDate(0, 0, 1)
	end := refundDay.AddDate(0, 0, 1)
	confirmedAt := confirmationDay.Add(8 * time.Hour)
	refundedAt := refundDay.Add(10 * time.Hour)

	bill := createPaymentLedgerBill(t, db, business.ID, "ledger-alt-refunded", 2500, 0, 0, BillStatusOpen, confirmationDay.Add(-time.Hour), refundedAt, nil)
	require.NoError(t, db.GetGorm().Create(&AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "cash-guest",
		Amount:          2500,
		BillAmountCents: 2500,
		TipAmountCents:  400,
		PaymentMethod:   PaymentMethodCash,
		Status:          AltPaymentStatusRefunded,
		ConfirmedBy:     "staff",
		CreatedAt:       confirmedAt.Add(-time.Minute),
		UpdatedAt:       refundedAt,
		ConfirmedAt:     &confirmedAt,
	}).Error)

	confirmationEvents, err := GetRecognizedPaymentEvents(business.ID, confirmationDay, refundDay)
	require.NoError(t, err)
	require.Len(t, confirmationEvents, 1)
	assert.Equal(t, int64(2500), confirmationEvents[0].AmountCents)
	assert.Equal(t, int64(400), confirmationEvents[0].TipCents)
	assert.Equal(t, "alternative_payment", confirmationEvents[0].Source)
	assert.Equal(t, confirmedAt, confirmationEvents[0].RecognizedAt)

	refundEvents, err := GetRecognizedPaymentEvents(business.ID, refundDay, end)
	require.NoError(t, err)
	require.Len(t, refundEvents, 1)
	assert.Equal(t, int64(-2500), refundEvents[0].AmountCents)
	assert.Equal(t, int64(-400), refundEvents[0].TipCents)
	assert.Equal(t, "reversal", refundEvents[0].Source)
	assert.Equal(t, refundedAt, refundEvents[0].RecognizedAt)

	allEvents, err := GetRecognizedPaymentEvents(business.ID, confirmationDay, end)
	require.NoError(t, err)
	require.Len(t, allEvents, 2)
	assert.Equal(t, int64(0), sumPaymentLedgerEventAmounts(allEvents))

	sqlSummary, err := GetRecognizedPaymentSummary(business.ID, confirmationDay, end)
	require.NoError(t, err)
	assert.Equal(t, int64(0), sqlSummary.TotalRevenueCents)
	assert.Equal(t, int64(0), sqlSummary.TotalTipCents)
	assert.Equal(t, int64(2), sqlSummary.EventCount)
}

func TestRecognizedPaymentEvents_ClosedUnpaidLegacyBillsAreIgnored(t *testing.T) {
	db := setupPaymentLedgerTestDB(t)
	business := createPaymentLedgerBusiness(t, db)
	start := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	closedAt := start.Add(3 * time.Hour)

	createPaymentLedgerBill(t, db, business.ID, "ledger-closed-unpaid", 7500, 0, 0, BillStatusClosed, start.Add(-time.Hour), closedAt, &closedAt)

	events, err := GetRecognizedPaymentEvents(business.ID, start, end)
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestRecognizedPaymentEvents_LegacyBillFallbackUsesUpdatedAtWithoutDuplicatingCreatedAt(t *testing.T) {
	db := setupPaymentLedgerTestDB(t)
	business := createPaymentLedgerBusiness(t, db)
	start := time.Date(2026, 5, 5, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)

	updatedOnly := createPaymentLedgerBill(t, db, business.ID, "ledger-updated-only", 4400, 4400, 250, BillStatusPaid, start.Add(-48*time.Hour), start.Add(3*time.Hour), nil)
	createdAndUpdated := createPaymentLedgerBill(t, db, business.ID, "ledger-created-and-updated", 3100, 1600, 0, BillStatusPartial, start.Add(2*time.Hour), start.Add(4*time.Hour), nil)

	events, err := GetRecognizedPaymentEvents(business.ID, start, end)
	require.NoError(t, err)

	require.Len(t, events, 2)
	assert.Equal(t, 1, countPaymentLedgerEventsForBill(events, updatedOnly.ID))
	assert.Equal(t, 1, countPaymentLedgerEventsForBill(events, createdAndUpdated.ID))

	updatedOnlyEvent := requirePaymentLedgerEventForBill(t, events, updatedOnly.ID)
	assert.Equal(t, int64(4400), updatedOnlyEvent.AmountCents)
	assert.Equal(t, int64(250), updatedOnlyEvent.TipCents)
	assert.Equal(t, start.Add(3*time.Hour), updatedOnlyEvent.RecognizedAt)
	assert.Equal(t, "legacy_bill", updatedOnlyEvent.Source)

	createdAndUpdatedEvent := requirePaymentLedgerEventForBill(t, events, createdAndUpdated.ID)
	assert.Equal(t, int64(1600), createdAndUpdatedEvent.AmountCents)
	assert.Equal(t, start.Add(2*time.Hour), createdAndUpdatedEvent.RecognizedAt)
}

func TestRecognizedPaymentEvents_ConfirmedPaymentUpdatedAtFallbackRequiresNilConfirmedAt(t *testing.T) {
	db := setupPaymentLedgerTestDB(t)
	business := createPaymentLedgerBusiness(t, db)
	start := time.Date(2026, 5, 6, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)

	fallbackBill := createPaymentLedgerBill(t, db, business.ID, "ledger-confirmed-updated-fallback", 2400, 2400, 0, BillStatusPaid, start.Add(-48*time.Hour), start.Add(5*time.Hour), nil)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        fallbackBill.ID,
		PayerAddr:     "0xlegacy",
		Amount:        2400,
		TxHash:        "ledger_confirmed_updated_fallback",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   nil,
		CreatedAt:     start.Add(-48 * time.Hour),
		UpdatedAt:     start.Add(5 * time.Hour),
	}).Error)

	outOfRangeConfirmedAt := start.Add(-2 * time.Hour)
	notRescuedBill := createPaymentLedgerBill(t, db, business.ID, "ledger-confirmed-not-rescued", 1800, 1800, 0, BillStatusPaid, start.Add(-48*time.Hour), start.Add(6*time.Hour), nil)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        notRescuedBill.ID,
		PayerAddr:     "0xnotrescued",
		Amount:        1800,
		TxHash:        "ledger_confirmed_not_rescued",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &outOfRangeConfirmedAt,
		CreatedAt:     start.Add(-48 * time.Hour),
		UpdatedAt:     start.Add(6 * time.Hour),
	}).Error)

	events, err := GetRecognizedPaymentEvents(business.ID, start, end)
	require.NoError(t, err)

	require.Len(t, events, 1)
	assert.Equal(t, fallbackBill.ID, events[0].BillID)
	assert.Equal(t, start.Add(5*time.Hour), events[0].RecognizedAt)
}

func TestRecognizedPaymentEvents_ReversalUpdatedAtFallbackRequiresNilReversedAt(t *testing.T) {
	db := setupPaymentLedgerTestDB(t)
	business := createPaymentLedgerBusiness(t, db)
	start := time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	confirmedAt := start.Add(-time.Hour)

	fallbackBill := createPaymentLedgerBill(t, db, business.ID, "ledger-reversal-updated-fallback", 2900, 0, 0, BillStatusOpen, start.Add(-48*time.Hour), start.Add(5*time.Hour), nil)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        fallbackBill.ID,
		PayerAddr:     "0xlegacyreversal",
		Amount:        2900,
		TipAmount:     125,
		TxHash:        "ledger_reversal_updated_fallback",
		Status:        PaymentStatusReversed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		ReversedAt:    nil,
		CreatedAt:     start.Add(-48 * time.Hour),
		UpdatedAt:     start.Add(5 * time.Hour),
	}).Error)

	outOfRangeReversedAt := start.Add(-time.Hour)
	notRescuedBill := createPaymentLedgerBill(t, db, business.ID, "ledger-reversal-not-rescued", 1900, 0, 0, BillStatusOpen, start.Add(-48*time.Hour), start.Add(6*time.Hour), nil)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        notRescuedBill.ID,
		PayerAddr:     "0xnotrescuedreversal",
		Amount:        1900,
		TxHash:        "ledger_reversal_not_rescued",
		Status:        PaymentStatusReversed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		ReversedAt:    &outOfRangeReversedAt,
		CreatedAt:     start.Add(-48 * time.Hour),
		UpdatedAt:     start.Add(6 * time.Hour),
	}).Error)

	noConfirmedAtBill := createPaymentLedgerBill(t, db, business.ID, "ledger-reversal-no-confirmed-at", 1700, 0, 0, BillStatusOpen, start.Add(-48*time.Hour), start.Add(7*time.Hour), nil)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        noConfirmedAtBill.ID,
		PayerAddr:     "0xnoconfirmedreversal",
		Amount:        1700,
		TxHash:        "ledger_reversal_no_confirmed_at",
		Status:        PaymentStatusReversed,
		PaymentMethod: "crypto",
		ConfirmedAt:   nil,
		ReversedAt:    &outOfRangeReversedAt,
		CreatedAt:     start.Add(-48 * time.Hour),
		UpdatedAt:     start.Add(7 * time.Hour),
	}).Error)

	events, err := GetRecognizedPaymentEvents(business.ID, start, end)
	require.NoError(t, err)

	require.Len(t, events, 1)
	assert.Equal(t, fallbackBill.ID, events[0].BillID)
	assert.Equal(t, int64(-2900), events[0].AmountCents)
	assert.Equal(t, int64(-125), events[0].TipCents)
	assert.Equal(t, "reversal", events[0].Source)
	assert.Equal(t, start.Add(5*time.Hour), events[0].RecognizedAt)
}

func TestRecognizedPaymentEvents_AlternativePaymentCreatedAtFallbackRequiresNilConfirmedAt(t *testing.T) {
	db := setupPaymentLedgerTestDB(t)
	business := createPaymentLedgerBusiness(t, db)
	start := time.Date(2026, 5, 8, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)

	fallbackBill := createPaymentLedgerBill(t, db, business.ID, "ledger-alt-created-fallback", 2100, 2100, 0, BillStatusPaid, start.Add(-48*time.Hour), start.Add(2*time.Hour), nil)
	require.NoError(t, db.GetGorm().Create(&AlternativePayment{
		BillID:          fallbackBill.ID,
		ParticipantAddr: "cash-fallback",
		Amount:          2100,
		PaymentMethod:   PaymentMethodCash,
		Status:          AltPaymentStatusConfirmed,
		CreatedAt:       start.Add(2 * time.Hour),
		UpdatedAt:       start.Add(2 * time.Hour),
		ConfirmedAt:     nil,
	}).Error)

	outOfRangeConfirmedAt := start.Add(-time.Hour)
	notRescuedBill := createPaymentLedgerBill(t, db, business.ID, "ledger-alt-not-rescued", 2300, 2300, 0, BillStatusPaid, start.Add(-48*time.Hour), start.Add(3*time.Hour), nil)
	require.NoError(t, db.GetGorm().Create(&AlternativePayment{
		BillID:          notRescuedBill.ID,
		ParticipantAddr: "cash-not-rescued",
		Amount:          2300,
		PaymentMethod:   PaymentMethodCash,
		Status:          AltPaymentStatusConfirmed,
		CreatedAt:       start.Add(3 * time.Hour),
		UpdatedAt:       start.Add(3 * time.Hour),
		ConfirmedAt:     &outOfRangeConfirmedAt,
	}).Error)

	events, err := GetRecognizedPaymentEvents(business.ID, start, end)
	require.NoError(t, err)

	require.Len(t, events, 1)
	assert.Equal(t, fallbackBill.ID, events[0].BillID)
	assert.Equal(t, int64(2100), events[0].AmountCents)
	assert.Equal(t, "alternative_payment", events[0].Source)
	assert.Equal(t, start.Add(2*time.Hour), events[0].RecognizedAt)
}

func TestRecognizedPaymentEventsForAllBusinessesIncludesMultipleBusinesses(t *testing.T) {
	db := setupPaymentLedgerTestDB(t)
	firstBusiness := createPaymentLedgerBusiness(t, db)
	secondBusiness := createPaymentLedgerBusiness(t, db)
	start := time.Date(2026, 5, 9, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	firstConfirmedAt := start.Add(2 * time.Hour)
	secondConfirmedAt := start.Add(4 * time.Hour)

	firstBill := createPaymentLedgerBill(t, db, firstBusiness.ID, "ledger-all-business-first", 1100, 1100, 0, BillStatusPaid, start.Add(-time.Hour), firstConfirmedAt, nil)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        firstBill.ID,
		PayerAddr:     "0xfirst",
		Amount:        1100,
		TxHash:        "ledger_all_business_first",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &firstConfirmedAt,
		CreatedAt:     firstConfirmedAt,
		UpdatedAt:     firstConfirmedAt,
	}).Error)

	secondBill := createPaymentLedgerBill(t, db, secondBusiness.ID, "ledger-all-business-second", 2200, 2200, 0, BillStatusPaid, start.Add(-time.Hour), secondConfirmedAt, nil)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        secondBill.ID,
		PayerAddr:     "0xsecond",
		Amount:        2200,
		TxHash:        "ledger_all_business_second",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &secondConfirmedAt,
		CreatedAt:     secondConfirmedAt,
		UpdatedAt:     secondConfirmedAt,
	}).Error)

	firstEvents, err := GetRecognizedPaymentEvents(firstBusiness.ID, start, end)
	require.NoError(t, err)
	secondEvents, err := GetRecognizedPaymentEvents(secondBusiness.ID, start, end)
	require.NoError(t, err)
	events := append(append([]RecognizedPaymentEvent{}, firstEvents...), secondEvents...)

	require.Len(t, events, 2)
	assert.Equal(t, int64(3300), sumPaymentLedgerEventAmounts(events))
	assert.Contains(t, paymentLedgerEventBusinessIDs(events), firstBusiness.ID)
	assert.Contains(t, paymentLedgerEventBusinessIDs(events), secondBusiness.ID)

	summary, err := GetRecognizedPaymentSummaryForAllBusinesses(start, end)
	require.NoError(t, err)
	assert.Equal(t, int64(3300), summary.TotalRevenueCents)
	assert.Equal(t, int64(2), summary.EventCount)
	assert.Equal(t, int64(3300), summary.RevenueByMethod["crypto"])
}

func TestRecognizedPaymentSummaryAggregatesInSQLAndNormalizesPluginMethods(t *testing.T) {
	recorder := &recordingPaymentLedgerLogger{}
	db := setupPaymentLedgerTestDBWithLogger(t, recorder)
	business := createPaymentLedgerBusiness(t, db)
	start := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	confirmedAt := start.Add(2 * time.Hour)

	bill := createPaymentLedgerBill(t, db, business.ID, "ledger-summary-sql", 10500, 10500, 0, BillStatusPaid, start.Add(-time.Hour), confirmedAt, nil)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        bill.ID,
		PayerAddr:     "plugin",
		Amount:        3000,
		TipAmount:     150,
		TxHash:        "processor_reference",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "usd",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xpluginmethod",
		Amount:        700,
		TxHash:        "ledger_plugin_method",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "plugin",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xplugintx",
		Amount:        800,
		TxHash:        "plugin_summary_reference",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "usd",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xblankmethod",
		Amount:        2000,
		TxHash:        "ledger_blank_method",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "cash-summary",
		Amount:          4000,
		PaymentMethod:   PaymentMethodCash,
		Status:          AltPaymentStatusConfirmed,
		CreatedAt:       confirmedAt,
		UpdatedAt:       confirmedAt,
		ConfirmedAt:     &confirmedAt,
	}).Error)

	recorder.Reset()
	summary, err := GetRecognizedPaymentSummary(business.ID, start, end)
	require.NoError(t, err)

	assert.Equal(t, int64(10500), summary.TotalRevenueCents)
	assert.Equal(t, int64(150), summary.TotalTipCents)
	assert.Equal(t, int64(5), summary.EventCount)
	assert.Equal(t, int64(4500), summary.RevenueByMethod["plugin"])
	assert.Equal(t, int64(2000), summary.RevenueByMethod["crypto"])
	assert.Equal(t, int64(4000), summary.RevenueByMethod["cash"])
	assert.Equal(t, int64(3), summary.CountByMethod["plugin"])

	summarySQL := strings.ToLower(recorder.SQL())
	assert.Contains(t, summarySQL, "sum(")
	assert.NotContains(t, summarySQL, "bill_number")
	assert.NotContains(t, summarySQL, "recognized_at")
}

func TestRecognizedPaymentSummaryNoLegacyRowsScansZeroLegacyContribution(t *testing.T) {
	recorder := &recordingPaymentLedgerLogger{}
	db := setupPaymentLedgerTestDBWithLogger(t, recorder)
	business := createPaymentLedgerBusiness(t, db)
	start := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)

	recorder.Reset()
	summary, err := GetRecognizedPaymentSummary(business.ID, start, end)
	require.NoError(t, err)

	assert.Equal(t, int64(0), summary.TotalRevenueCents)
	assert.Equal(t, int64(0), summary.TotalTipCents)
	assert.Equal(t, int64(0), summary.EventCount)
	assert.NotContains(t, summary.RevenueByMethod, "legacy_bill")

	summarySQL := strings.ToLower(recorder.SQL())
	assert.Contains(t, summarySQL, "'legacy_bill' as payment_method")
	assert.Contains(t, summarySQL, "coalesce(sum(")
}

func TestRecognizedPaymentSummaryPluginTxHashRequiresLiteralUnderscorePrefix(t *testing.T) {
	db := setupPaymentLedgerTestDB(t)
	business := createPaymentLedgerBusiness(t, db)
	start := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	confirmedAt := start.Add(2 * time.Hour)

	bill := createPaymentLedgerBill(t, db, business.ID, "ledger-plugin-prefix", 3500, 3500, 0, BillStatusPaid, start.Add(-time.Hour), confirmedAt, nil)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xpluginx",
		Amount:        1200,
		TxHash:        "pluginX_not_a_plugin_prefix",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "usd",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        bill.ID,
		PayerAddr:     "0xplugin",
		Amount:        2300,
		TxHash:        "plugin_real_plugin_prefix",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "usd",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)

	summary, err := GetRecognizedPaymentSummary(business.ID, start, end)
	require.NoError(t, err)

	assert.Equal(t, int64(3500), summary.TotalRevenueCents)
	assert.Equal(t, int64(1200), summary.RevenueByMethod["usd"])
	assert.Equal(t, int64(2300), summary.RevenueByMethod["plugin"])
	assert.Equal(t, int64(1), summary.CountByMethod["usd"])
	assert.Equal(t, int64(1), summary.CountByMethod["plugin"])
}

func TestRecognizedPaymentEventsRejectsTooLargeRange(t *testing.T) {
	db := setupPaymentLedgerTestDB(t)
	business := createPaymentLedgerBusiness(t, db)
	start := time.Date(2026, 5, 13, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 32)

	_, err := GetRecognizedPaymentEvents(business.ID, start, end)
	assert.ErrorContains(t, err, "recognized payment event range too large")

	_, err = GetRecognizedPaymentSummary(business.ID, start, end)
	assert.NoError(t, err)
}

func TestRecognizedPaymentEventsRejectsTooManyRows(t *testing.T) {
	db := setupPaymentLedgerTestDB(t)
	business := createPaymentLedgerBusiness(t, db)
	start := time.Date(2026, 5, 14, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	confirmedAt := start.Add(3 * time.Hour)
	bill := createPaymentLedgerBill(t, db, business.ID, "ledger-row-limit", 20000, 0, 0, BillStatusPartial, start.Add(-time.Hour), confirmedAt, nil)

	payments := make([]Payment, 10001)
	for i := range payments {
		payments[i] = Payment{
			BillID:        bill.ID,
			PayerAddr:     "0xrowlimit",
			Amount:        1,
			TxHash:        fmt.Sprintf("ledger_row_limit_%05d", i),
			Status:        PaymentStatusConfirmed,
			PaymentMethod: "crypto",
			ConfirmedAt:   &confirmedAt,
			CreatedAt:     confirmedAt,
			UpdatedAt:     confirmedAt,
		}
	}
	require.NoError(t, db.GetGorm().CreateInBatches(payments, 500).Error)

	_, err := GetRecognizedPaymentEvents(business.ID, start, end)
	assert.ErrorContains(t, err, "recognized payment event limit exceeded")

	summary, err := GetRecognizedPaymentSummary(business.ID, start, end)
	require.NoError(t, err)
	assert.Equal(t, int64(10001), summary.TotalRevenueCents)
	assert.Equal(t, int64(10001), summary.EventCount)
}

func TestRecognizedPaymentEventsUsesNullSafePaymentProjection(t *testing.T) {
	recorder := &recordingPaymentLedgerLogger{}
	db := setupPaymentLedgerTestDBWithLogger(t, recorder)
	business := createPaymentLedgerBusiness(t, db)
	start := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)

	recorder.Reset()
	_, err := GetRecognizedPaymentEvents(business.ID, start, end)
	require.NoError(t, err)

	sql := strings.ToLower(recorder.SQL())
	assert.Contains(t, sql, "coalesce(payments.amount, 0)")
	assert.Contains(t, sql, "coalesce(payments.tip_amount, 0)")
	assert.Contains(t, sql, "coalesce(payments.payment_method, '')")
	assert.Contains(t, sql, "coalesce(alternative_payments.amount, 0)")
	assert.Contains(t, sql, "coalesce(alternative_payments.payment_method, '')")
}

func TestRecognizedPaymentEventsOrdersSameTimestampBillAndSourceBySourceRowID(t *testing.T) {
	db := setupPaymentLedgerTestDB(t)
	business := createPaymentLedgerBusiness(t, db)
	start := time.Date(2026, 5, 16, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	confirmedAt := start.Add(3 * time.Hour)
	bill := createPaymentLedgerBill(t, db, business.ID, "ledger-ordering", 3000, 3000, 0, BillStatusPaid, start.Add(-time.Hour), confirmedAt, nil)

	require.NoError(t, db.GetGorm().Create(&Payment{
		ID:            200,
		BillID:        bill.ID,
		PayerAddr:     "0xsecond",
		Amount:        2000,
		TxHash:        "ledger_ordering_second",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&Payment{
		ID:            100,
		BillID:        bill.ID,
		PayerAddr:     "0xfirst",
		Amount:        1000,
		TxHash:        "ledger_ordering_first",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)

	events, err := GetRecognizedPaymentEvents(business.ID, start, end)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, int64(1000), events[0].AmountCents)
	assert.Equal(t, int64(2000), events[1].AmountCents)
}

func TestRecognizedPaymentSummaryMatchesEventsForMixedLedgerDataset(t *testing.T) {
	db := setupPaymentLedgerTestDB(t)
	business := createPaymentLedgerBusiness(t, db)
	start := time.Date(2026, 5, 17, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 3)
	confirmedAt := start.Add(2 * time.Hour)
	reversedConfirmedAt := start.Add(3 * time.Hour)
	reversedAt := start.AddDate(0, 0, 1).Add(4 * time.Hour)
	alternativeConfirmedAt := start.Add(5 * time.Hour)

	confirmedBill := createPaymentLedgerBill(t, db, business.ID, "ledger-drift-confirmed", 1500, 1500, 0, BillStatusPaid, start.Add(-time.Hour), confirmedAt, nil)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        confirmedBill.ID,
		PayerAddr:     "plugin",
		Amount:        1500,
		TipAmount:     100,
		TxHash:        "processor_drift_confirmed",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "usd",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)

	reversedBill := createPaymentLedgerBill(t, db, business.ID, "ledger-drift-reversed", 800, 0, 0, BillStatusOpen, start.Add(-time.Hour), reversedAt, nil)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        reversedBill.ID,
		PayerAddr:     "0xreverseddrift",
		Amount:        800,
		TipAmount:     80,
		TxHash:        "ledger_drift_reversed",
		Status:        PaymentStatusReversed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &reversedConfirmedAt,
		ReversedAt:    &reversedAt,
		CreatedAt:     reversedConfirmedAt,
		UpdatedAt:     reversedAt,
	}).Error)

	alternativeBill := createPaymentLedgerBill(t, db, business.ID, "ledger-drift-alt", 900, 900, 0, BillStatusPaid, start.Add(-time.Hour), alternativeConfirmedAt, nil)
	require.NoError(t, db.GetGorm().Create(&AlternativePayment{
		BillID:          alternativeBill.ID,
		ParticipantAddr: "cash-drift",
		Amount:          900,
		PaymentMethod:   PaymentMethodCash,
		Status:          AltPaymentStatusConfirmed,
		CreatedAt:       alternativeConfirmedAt,
		UpdatedAt:       alternativeConfirmedAt,
		ConfirmedAt:     &alternativeConfirmedAt,
	}).Error)

	legacyUpdatedAt := start.AddDate(0, 0, 2).Add(2 * time.Hour)
	createPaymentLedgerBill(t, db, business.ID, "ledger-drift-legacy", 700, 700, 70, BillStatusPaid, start.AddDate(0, 0, -3), legacyUpdatedAt, nil)

	events, err := GetRecognizedPaymentEvents(business.ID, start, end)
	require.NoError(t, err)
	eventSummary := SummarizeRecognizedPaymentEvents(events)

	sqlSummary, err := GetRecognizedPaymentSummary(business.ID, start, end)
	require.NoError(t, err)
	assertRecognizedPaymentSummariesEqual(t, eventSummary, sqlSummary)

	allBusinessSQLSummary, err := GetRecognizedPaymentSummaryForAllBusinesses(start, end)
	require.NoError(t, err)
	assertRecognizedPaymentSummariesEqual(t, eventSummary, allBusinessSQLSummary)
}

func TestRecognizedPaymentMonthlySummaryAggregatesAllBusinessesInSingleRange(t *testing.T) {
	db := setupPaymentLedgerTestDB(t)
	firstBusiness := createPaymentLedgerBusiness(t, db)
	secondBusiness := createPaymentLedgerBusiness(t, db)
	aprilPaymentAt := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	mayPaymentAt := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	reversedAt := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)

	confirmedBill := createPaymentLedgerBill(t, db, firstBusiness.ID, "ledger-month-confirmed", 1000, 1000, 100, BillStatusPaid, mayPaymentAt.Add(-time.Hour), mayPaymentAt, nil)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        confirmedBill.ID,
		PayerAddr:     "0xledger_month_confirmed",
		Amount:        1000,
		TipAmount:     100,
		TxHash:        "ledger_month_confirmed",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &mayPaymentAt,
		CreatedAt:     mayPaymentAt,
		UpdatedAt:     mayPaymentAt,
	}).Error)

	alternativeBill := createPaymentLedgerBill(t, db, secondBusiness.ID, "ledger-month-alt", 500, 500, 0, BillStatusPaid, mayPaymentAt.Add(-time.Hour), mayPaymentAt, nil)
	require.NoError(t, db.GetGorm().Create(&AlternativePayment{
		BillID:          alternativeBill.ID,
		ParticipantAddr: "cashier",
		Amount:          500,
		PaymentMethod:   PaymentMethodCash,
		Status:          AltPaymentStatusConfirmed,
		ConfirmedAt:     &mayPaymentAt,
		CreatedAt:       mayPaymentAt,
		UpdatedAt:       mayPaymentAt,
	}).Error)

	reversedBill := createPaymentLedgerBill(t, db, firstBusiness.ID, "ledger-month-reversed", 700, 0, 0, BillStatusOpen, aprilPaymentAt.Add(-time.Hour), reversedAt, nil)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        reversedBill.ID,
		PayerAddr:     "0xledger_month_reversed",
		Amount:        700,
		TipAmount:     70,
		TxHash:        "ledger_month_reversed",
		Status:        PaymentStatusReversed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &aprilPaymentAt,
		ReversedAt:    &reversedAt,
		CreatedAt:     aprilPaymentAt,
		UpdatedAt:     reversedAt,
	}).Error)

	summaries, err := GetRecognizedPaymentMonthlySummaryForAllBusinesses(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)

	april := summaries["2026-04"]
	assert.Equal(t, int64(700), april.TotalRevenueCents)
	assert.Equal(t, int64(70), april.TotalTipCents)
	assert.Equal(t, int64(700), april.GrossRevenueCents)
	assert.Equal(t, int64(70), april.GrossTipCents)
	assert.Equal(t, int64(1), april.PositiveEventCount)
	assert.Equal(t, int64(0), april.ReversalEventCount)

	may := summaries["2026-05"]
	assert.Equal(t, int64(800), may.TotalRevenueCents)
	assert.Equal(t, int64(30), may.TotalTipCents)
	assert.Equal(t, int64(1500), may.GrossRevenueCents)
	assert.Equal(t, int64(100), may.GrossTipCents)
	assert.Equal(t, int64(2), may.PositiveEventCount)
	assert.Equal(t, int64(1), may.ReversalEventCount)
}

func TestRecognizedPaymentBillCountForAllBusinessesNetsReversals(t *testing.T) {
	db := setupPaymentLedgerTestDB(t)
	firstBusiness := createPaymentLedgerBusiness(t, db)
	secondBusiness := createPaymentLedgerBusiness(t, db)
	confirmedAt := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	reversedAt := confirmedAt.Add(2 * time.Hour)

	partialBill := createPaymentLedgerBill(t, db, firstBusiness.ID, "ledger-all-business-partial", 10000, 1000, 0, BillStatusPartial, confirmedAt.Add(-time.Hour), confirmedAt, nil)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        partialBill.ID,
		PayerAddr:     "0xledger_all_partial",
		Amount:        1000,
		TxHash:        "ledger_all_partial",
		Status:        PaymentStatusConfirmed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}).Error)

	legacyBill := createPaymentLedgerBill(t, db, secondBusiness.ID, "ledger-all-business-legacy", 2000, 2000, 0, BillStatusPaid, confirmedAt.Add(-time.Hour), confirmedAt, nil)
	legacyBill.ClosedAt = &confirmedAt
	require.NoError(t, db.GetGorm().Save(legacyBill).Error)

	reversedBill := createPaymentLedgerBill(t, db, secondBusiness.ID, "ledger-all-business-reversed", 10000, 10000, 0, BillStatusPartial, confirmedAt.Add(-time.Hour), reversedAt, nil)
	require.NoError(t, db.GetGorm().Create(&Payment{
		BillID:        reversedBill.ID,
		PayerAddr:     "0xledger_all_reversed",
		Amount:        10000,
		TxHash:        "ledger_all_reversed",
		Status:        PaymentStatusReversed,
		PaymentMethod: "crypto",
		ConfirmedAt:   &confirmedAt,
		ReversedAt:    &reversedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     reversedAt,
	}).Error)

	count, err := GetRecognizedPaymentBillCountForAllBusinesses(confirmedAt.Add(-24*time.Hour), reversedAt.Add(24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

func TestSummarizeRecognizedPaymentEvents_NetsAmountsAndGroupsByMethod(t *testing.T) {
	events := []RecognizedPaymentEvent{
		{AmountCents: 1000, TipCents: 100, PaymentMethod: "cash", Source: "alternative_payment"},
		{AmountCents: 2500, TipCents: 0, PaymentMethod: "plugin", Source: "payment"},
		{AmountCents: -500, TipCents: -50, PaymentMethod: "plugin", Source: "reversal"},
	}

	summary := SummarizeRecognizedPaymentEvents(events)

	assert.Equal(t, int64(3000), summary.TotalRevenueCents)
	assert.Equal(t, int64(50), summary.TotalTipCents)
	assert.Equal(t, int64(3), summary.EventCount)
	assert.Equal(t, int64(1000), summary.RevenueByMethod["cash"])
	assert.Equal(t, int64(2000), summary.RevenueByMethod["plugin"])
	assert.Equal(t, int64(-50), summary.TipByMethod["plugin"])
	assert.Equal(t, int64(2), summary.CountByMethod["plugin"])
}

func countPaymentLedgerEventsForBill(events []RecognizedPaymentEvent, billID uint) int {
	count := 0
	for _, event := range events {
		if event.BillID == billID {
			count++
		}
	}
	return count
}

func requirePaymentLedgerEventForBill(t *testing.T, events []RecognizedPaymentEvent, billID uint) RecognizedPaymentEvent {
	t.Helper()

	for _, event := range events {
		if event.BillID == billID {
			return event
		}
	}

	require.Failf(t, "missing payment ledger event", "bill %d did not have a recognized payment event", billID)
	return RecognizedPaymentEvent{}
}

func sumPaymentLedgerEventAmounts(events []RecognizedPaymentEvent) int64 {
	var total int64
	for _, event := range events {
		total += event.AmountCents
	}
	return total
}

func paymentLedgerEventBusinessIDs(events []RecognizedPaymentEvent) []uint {
	ids := make([]uint, 0, len(events))
	for _, event := range events {
		ids = append(ids, event.BusinessID)
	}
	return ids
}

func assertRecognizedPaymentSummariesEqual(t *testing.T, expected, actual RecognizedPaymentSummary) {
	t.Helper()

	assert.Equal(t, expected.TotalRevenueCents, actual.TotalRevenueCents)
	assert.Equal(t, expected.TotalTipCents, actual.TotalTipCents)
	assert.Equal(t, expected.GrossRevenueCents, actual.GrossRevenueCents)
	assert.Equal(t, expected.GrossTipCents, actual.GrossTipCents)
	assert.Equal(t, expected.EventCount, actual.EventCount)
	assert.Equal(t, expected.PositiveEventCount, actual.PositiveEventCount)
	assert.Equal(t, expected.ReversalEventCount, actual.ReversalEventCount)
	assert.Equal(t, expected.RevenueByMethod, actual.RevenueByMethod)
	assert.Equal(t, expected.TipByMethod, actual.TipByMethod)
	assert.Equal(t, expected.CountByMethod, actual.CountByMethod)
}
