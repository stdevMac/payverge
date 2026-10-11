package database

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	recognizedPaymentSourcePayment            = "payment"
	recognizedPaymentSourceAlternative        = "alternative_payment"
	recognizedPaymentSourceReversal           = "reversal"
	recognizedPaymentSourceLegacyBillFallback = "legacy_bill"

	maxRecognizedPaymentEventRange = 31 * 24 * time.Hour
	maxRecognizedPaymentEventRows  = 10000
)

var (
	ErrRecognizedPaymentEventRangeTooLarge = errors.New("recognized payment event range too large")
	ErrRecognizedPaymentEventLimitExceeded = errors.New("recognized payment event limit exceeded")
)

// RecognizedPaymentEvent represents revenue recognized from an immutable
// payment source at the time the payment, reversal, or legacy fallback took
// effect.
type RecognizedPaymentEvent struct {
	BillID         uint       `json:"bill_id"`
	BusinessID     uint       `json:"business_id"`
	AmountCents    int64      `json:"amount_cents"`
	TipCents       int64      `json:"tip_cents"`
	PaymentMethod  string     `json:"payment_method"`
	Source         string     `json:"source"`
	RecognizedAt   time.Time  `json:"recognized_at"`
	BillNumber     string     `json:"bill_number"`
	BillStatus     BillStatus `json:"bill_status"`
	BillTotalCents int64      `json:"bill_total_cents"`
	BillCreatedAt  time.Time  `json:"bill_created_at"`
}

// RecognizedPaymentSummary is a net summary of recognized payment events.
type RecognizedPaymentSummary struct {
	TotalRevenueCents  int64            `json:"total_revenue_cents"`
	TotalTipCents      int64            `json:"total_tip_cents"`
	GrossRevenueCents  int64            `json:"gross_revenue_cents"`
	GrossTipCents      int64            `json:"gross_tip_cents"`
	EventCount         int64            `json:"event_count"`
	PositiveEventCount int64            `json:"positive_event_count"`
	ReversalEventCount int64            `json:"reversal_event_count"`
	RevenueByMethod    map[string]int64 `json:"revenue_by_method"`
	TipByMethod        map[string]int64 `json:"tip_by_method"`
	CountByMethod      map[string]int64 `json:"count_by_method"`
}

var billManagedAlternativePaymentMethodStrings = []string{
	string(PaymentMethodCash),
	string(PaymentMethodCard),
	string(PaymentMethodVenmo),
	string(PaymentMethodOther),
}

func BillManagedAlternativePaymentMethodStrings() []string {
	methods := make([]string, len(billManagedAlternativePaymentMethodStrings))
	copy(methods, billManagedAlternativePaymentMethodStrings)
	return methods
}

// Recognition status sets are the single source of truth for "which payment
// statuses count toward recognized revenue" across every encoding of the ledger:
// the GORM event/summary/monthly builders here, the raw bill-count SQL below, the
// analytics service CTE (internal/analytics/service.go), the today-by-staff query
// (internal/handlers/analytics.go), and the milestone / business-revenue-aggregate
// rollups (milestone_events.go, business_revenue_aggregate.go). Keep them here so
// adding a new terminal status can never be forgotten in one copy.
var (
	// RecognizedPaymentStatuses keep their original positive recognition in the
	// period the payment was earned. Reversed (chain reorg) and refunded
	// (operator refund) payments stay recognized and are netted by a dated
	// negative event at reversal/refund time — they are NOT silently erased.
	RecognizedPaymentStatuses = []PaymentStatus{PaymentStatusConfirmed, PaymentStatusReversed, PaymentStatusRefunded}

	// ReversedOrRefundedPaymentStatuses represent a payment that was later undone.
	// They emit the dated negative event (and, for legacy rows with no
	// confirmed_at, also carry the positive recognition at created_at).
	ReversedOrRefundedPaymentStatuses = []PaymentStatus{PaymentStatusReversed, PaymentStatusRefunded}

	// RecognizedAltPaymentStatuses mirror RecognizedPaymentStatuses for
	// bill-managed alternative payments, which are never chain-reversed — only
	// operator-refunded (status=refunded, netted on updated_at).
	RecognizedAltPaymentStatuses = []AlternativePaymentStatus{AltPaymentStatusConfirmed, AltPaymentStatusRefunded}
)

type paymentLedgerQueryRow struct {
	BillID         uint
	BusinessID     uint
	AmountCents    int64
	TipCents       int64
	PaymentMethod  string
	Source         string
	RecognizedAt   time.Time
	BillNumber     string
	BillStatus     BillStatus
	BillTotalCents int64
	BillCreatedAt  time.Time
	PayerAddr      string
	TxHash         string
	SourceRowID    uint
}

type paymentLedgerSummaryRow struct {
	PaymentMethod      string `gorm:"column:payment_method"`
	RevenueCents       int64  `gorm:"column:revenue_cents"`
	TipCents           int64  `gorm:"column:tip_cents"`
	GrossRevenueCents  int64  `gorm:"column:gross_revenue_cents"`
	GrossTipCents      int64  `gorm:"column:gross_tip_cents"`
	EventCount         int64  `gorm:"column:event_count"`
	PositiveEventCount int64  `gorm:"column:positive_event_count"`
	ReversalEventCount int64  `gorm:"column:reversal_event_count"`
}

type paymentLedgerMonthlySummaryRow struct {
	MonthKey           string `gorm:"column:month_key"`
	PaymentMethod      string `gorm:"column:payment_method"`
	RevenueCents       int64  `gorm:"column:revenue_cents"`
	TipCents           int64  `gorm:"column:tip_cents"`
	GrossRevenueCents  int64  `gorm:"column:gross_revenue_cents"`
	GrossTipCents      int64  `gorm:"column:gross_tip_cents"`
	EventCount         int64  `gorm:"column:event_count"`
	PositiveEventCount int64  `gorm:"column:positive_event_count"`
	ReversalEventCount int64  `gorm:"column:reversal_event_count"`
}

func (row paymentLedgerQueryRow) event() RecognizedPaymentEvent {
	return RecognizedPaymentEvent{
		BillID:         row.BillID,
		BusinessID:     row.BusinessID,
		AmountCents:    row.AmountCents,
		TipCents:       row.TipCents,
		PaymentMethod:  NormalizeRecognizedPaymentMethod(row.PaymentMethod, row.PayerAddr, row.TxHash),
		Source:         row.Source,
		RecognizedAt:   row.RecognizedAt,
		BillNumber:     row.BillNumber,
		BillStatus:     row.BillStatus,
		BillTotalCents: row.BillTotalCents,
		BillCreatedAt:  row.BillCreatedAt,
	}
}

// NormalizeRecognizedPaymentMethod collapses plugin settlement rows to a
// single method while preserving ordinary payment-method names.
func NormalizeRecognizedPaymentMethod(method, payerAddr, txHash string) string {
	normalizedMethod := strings.ToLower(strings.TrimSpace(method))
	normalizedPayer := strings.ToLower(strings.TrimSpace(payerAddr))
	normalizedTxHash := strings.ToLower(strings.TrimSpace(txHash))

	if normalizedMethod == "plugin" || normalizedPayer == "plugin" || strings.HasPrefix(normalizedTxHash, "plugin_") {
		return "plugin"
	}
	if normalizedMethod == "" {
		return "crypto"
	}
	return normalizedMethod
}

func GetRecognizedPaymentEvents(businessID uint, startDate, endDate time.Time) ([]RecognizedPaymentEvent, error) {
	return getRecognizedPaymentEvents(&businessID, startDate, endDate)
}

func getRecognizedPaymentEvents(businessID *uint, startDate, endDate time.Time) ([]RecognizedPaymentEvent, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if endDate.Sub(startDate) > maxRecognizedPaymentEventRange {
		return nil, ErrRecognizedPaymentEventRangeTooLarge
	}

	rows := make([]paymentLedgerQueryRow, 0)
	appenders := []func(*[]paymentLedgerQueryRow, *uint, time.Time, time.Time) error{
		appendConfirmedPaymentEvents,
		appendLegacyConfirmedPaymentEvents,
		appendLegacyReversedPaymentEvents,
		appendReversalPaymentEvents,
		appendLegacyReversalPaymentEvents,
		appendAlternativePaymentEvents,
		appendLegacyAlternativePaymentEvents,
		appendAlternativeRefundReversalEvents,
		appendLegacyBillFallbackEvents,
	}

	for _, appendEvents := range appenders {
		if err := appendEvents(&rows, businessID, startDate, endDate); err != nil {
			return nil, err
		}
		if len(rows) > maxRecognizedPaymentEventRows {
			return nil, ErrRecognizedPaymentEventLimitExceeded
		}
	}

	sort.SliceStable(rows, func(i, j int) bool {
		if !rows[i].RecognizedAt.Equal(rows[j].RecognizedAt) {
			return rows[i].RecognizedAt.Before(rows[j].RecognizedAt)
		}
		if rows[i].BillID != rows[j].BillID {
			return rows[i].BillID < rows[j].BillID
		}
		if rows[i].Source != rows[j].Source {
			return rows[i].Source < rows[j].Source
		}
		return rows[i].SourceRowID < rows[j].SourceRowID
	})

	events := make([]RecognizedPaymentEvent, 0, len(rows))
	for _, row := range rows {
		events = append(events, row.event())
	}
	return events, nil
}

func appendConfirmedPaymentEvents(rows *[]paymentLedgerQueryRow, businessID *uint, startDate, endDate time.Time) error {
	query := paymentEventBaseQuery(businessID).
		Select(paymentEventSelect("payments.confirmed_at", recognizedPaymentSourcePayment, false)).
		Where("payments.status IN ?", RecognizedPaymentStatuses).
		Where("payments.confirmed_at IS NOT NULL").
		Where("payments.confirmed_at >= ? AND payments.confirmed_at < ?", startDate, endDate)

	return appendPaymentLedgerRows(rows, query)
}

func appendLegacyConfirmedPaymentEvents(rows *[]paymentLedgerQueryRow, businessID *uint, startDate, endDate time.Time) error {
	query := paymentEventBaseQuery(businessID).
		Select(paymentEventSelect("payments.updated_at", recognizedPaymentSourcePayment, false)).
		Where("payments.status = ?", PaymentStatusConfirmed).
		Where("payments.confirmed_at IS NULL").
		Where("payments.updated_at >= ? AND payments.updated_at < ?", startDate, endDate)

	return appendPaymentLedgerRows(rows, query)
}

func appendLegacyReversedPaymentEvents(rows *[]paymentLedgerQueryRow, businessID *uint, startDate, endDate time.Time) error {
	query := paymentEventBaseQuery(businessID).
		Select(paymentEventSelect("payments.created_at", recognizedPaymentSourcePayment, false)).
		Where("payments.status IN ?", ReversedOrRefundedPaymentStatuses).
		Where("payments.confirmed_at IS NULL").
		Where("payments.created_at >= ? AND payments.created_at < ?", startDate, endDate)

	return appendPaymentLedgerRows(rows, query)
}

func appendReversalPaymentEvents(rows *[]paymentLedgerQueryRow, businessID *uint, startDate, endDate time.Time) error {
	query := paymentEventBaseQuery(businessID).
		Select(paymentEventSelect("payments.reversed_at", recognizedPaymentSourceReversal, true)).
		Where("payments.status IN ?", ReversedOrRefundedPaymentStatuses).
		Where("payments.reversed_at IS NOT NULL").
		Where("payments.reversed_at >= ? AND payments.reversed_at < ?", startDate, endDate)

	return appendPaymentLedgerRows(rows, query)
}

func appendLegacyReversalPaymentEvents(rows *[]paymentLedgerQueryRow, businessID *uint, startDate, endDate time.Time) error {
	query := paymentEventBaseQuery(businessID).
		Select(paymentEventSelect("payments.updated_at", recognizedPaymentSourceReversal, true)).
		Where("payments.status IN ?", ReversedOrRefundedPaymentStatuses).
		Where("payments.reversed_at IS NULL").
		Where("payments.updated_at >= ? AND payments.updated_at < ?", startDate, endDate)

	return appendPaymentLedgerRows(rows, query)
}

func appendAlternativePaymentEvents(rows *[]paymentLedgerQueryRow, businessID *uint, startDate, endDate time.Time) error {
	query := alternativePaymentEventBaseQuery(businessID).
		Select(alternativePaymentEventSelect("alternative_payments.confirmed_at", false)).
		Where("alternative_payments.status IN ?", RecognizedAltPaymentStatuses).
		Where("alternative_payments.payment_method IN ?", billManagedAlternativePaymentMethodStrings).
		Where("alternative_payments.confirmed_at IS NOT NULL").
		Where("alternative_payments.confirmed_at >= ? AND alternative_payments.confirmed_at < ?", startDate, endDate)

	return appendPaymentLedgerRows(rows, query)
}

func appendLegacyAlternativePaymentEvents(rows *[]paymentLedgerQueryRow, businessID *uint, startDate, endDate time.Time) error {
	query := alternativePaymentEventBaseQuery(businessID).
		Select(alternativePaymentEventSelect("alternative_payments.created_at", false)).
		Where("alternative_payments.status IN ?", RecognizedAltPaymentStatuses).
		Where("alternative_payments.payment_method IN ?", billManagedAlternativePaymentMethodStrings).
		Where("alternative_payments.confirmed_at IS NULL").
		Where("alternative_payments.created_at >= ? AND alternative_payments.created_at < ?", startDate, endDate)

	return appendPaymentLedgerRows(rows, query)
}

// appendAlternativeRefundReversalEvents books the dated negative event for an
// operator-refunded bill-managed alternative payment. Alt-payments carry no
// reversal timestamp, so the refund is recognized at updated_at (the refund
// time). This mirrors the payment reversal branch and nets the original positive
// recognition to zero over all time.
func appendAlternativeRefundReversalEvents(rows *[]paymentLedgerQueryRow, businessID *uint, startDate, endDate time.Time) error {
	query := alternativePaymentEventBaseQuery(businessID).
		Select(alternativePaymentEventSelect("alternative_payments.updated_at", true)).
		Where("alternative_payments.status = ?", AltPaymentStatusRefunded).
		Where("alternative_payments.payment_method IN ?", billManagedAlternativePaymentMethodStrings).
		Where("alternative_payments.updated_at >= ? AND alternative_payments.updated_at < ?", startDate, endDate)

	return appendPaymentLedgerRows(rows, query)
}

func appendLegacyBillFallbackEvents(rows *[]paymentLedgerQueryRow, businessID *uint, startDate, endDate time.Time) error {
	// Keep direct-column SELECTs rather than COALESCE here: SQLite returns a
	// coalesced timestamp as text, which cannot be scanned into time.Time. The
	// mutually exclusive predicates encode the same canonical fallback order.
	branches := []struct {
		column string
		where  string
	}{
		{"bills.settled_at", "bills.settled_at IS NOT NULL"},
		{"bills.closed_at", "bills.settled_at IS NULL AND bills.closed_at IS NOT NULL"},
		{"bills.updated_at", "bills.settled_at IS NULL AND bills.closed_at IS NULL AND bills.status IN ('paid', 'closed') AND bills.updated_at IS NOT NULL"},
		{"bills.created_at", "bills.settled_at IS NULL AND bills.closed_at IS NULL AND bills.status NOT IN ('paid', 'closed') AND bills.created_at IS NOT NULL"},
	}
	for _, branch := range branches {
		query := legacyBillFallbackBaseQuery(businessID).
			Select(legacyBillFallbackEventSelect(branch.column)).
			Where(branch.where).
			Where(branch.column+" >= ? AND "+branch.column+" < ?", startDate, endDate)
		if err := appendPaymentLedgerRows(rows, query); err != nil {
			return err
		}
	}
	return nil
}

// RecognizedBillSettlementTimeSQL is the canonical legacy time expression used
// by revenue and tip SQL reports. Partial legacy bills never reached terminal
// settlement, so their last defensible fallback remains their open time.
func RecognizedBillSettlementTimeSQL() string {
	return "COALESCE(bills.settled_at, bills.closed_at, CASE WHEN bills.status IN ('paid', 'closed') THEN bills.updated_at ELSE bills.created_at END, bills.updated_at, bills.created_at)"
}

func paymentEventBaseQuery(businessID *uint) *gorm.DB {
	query := db.Model(&Payment{}).Joins("JOIN bills ON payments.bill_id = bills.id")
	return withOptionalPaymentLedgerBusinessScope(query, businessID)
}

func alternativePaymentEventBaseQuery(businessID *uint) *gorm.DB {
	query := db.Model(&AlternativePayment{}).Joins("JOIN bills ON alternative_payments.bill_id = bills.id")
	return withOptionalPaymentLedgerBusinessScope(query, businessID)
}

func legacyBillFallbackBaseQuery(businessID *uint) *gorm.DB {
	query := db.Model(&Bill{}).
		Where("(bills.paid_amount > ? OR (bills.status = ? AND bills.total_amount > ?))", 0, BillStatusPaid, 0).
		Where(
			"NOT EXISTS (SELECT 1 FROM payments recognized_payments WHERE recognized_payments.bill_id = bills.id AND recognized_payments.status IN ?)",
			RecognizedPaymentStatuses,
		).
		Where(
			"NOT EXISTS (SELECT 1 FROM alternative_payments recognized_alternative_payments WHERE recognized_alternative_payments.bill_id = bills.id AND recognized_alternative_payments.status IN ? AND recognized_alternative_payments.payment_method IN ?)",
			RecognizedAltPaymentStatuses,
			billManagedAlternativePaymentMethodStrings,
		)
	return withOptionalPaymentLedgerBusinessScope(query, businessID)
}

func withOptionalPaymentLedgerBusinessScope(query *gorm.DB, businessID *uint) *gorm.DB {
	if businessID == nil {
		return query
	}
	return query.Where("bills.business_id = ?", *businessID)
}

func appendPaymentLedgerRows(rows *[]paymentLedgerQueryRow, query *gorm.DB) error {
	var scannedRows []paymentLedgerQueryRow
	if err := query.Limit(maxRecognizedPaymentEventRows + 1).Scan(&scannedRows).Error; err != nil {
		return err
	}
	if len(scannedRows) > maxRecognizedPaymentEventRows {
		return ErrRecognizedPaymentEventLimitExceeded
	}

	*rows = append(*rows, scannedRows...)
	if len(*rows) > maxRecognizedPaymentEventRows {
		return ErrRecognizedPaymentEventLimitExceeded
	}
	return nil
}

func paymentEventSelect(recognizedAtColumn, source string, reverse bool) string {
	amountColumn := "COALESCE(payments.amount, 0)"
	tipColumn := "COALESCE(payments.tip_amount, 0)"
	if reverse {
		amountColumn = "(-COALESCE(payments.amount, 0))"
		tipColumn = "(-COALESCE(payments.tip_amount, 0))"
	}

	return fmt.Sprintf(`
		payments.bill_id AS bill_id,
		bills.business_id AS business_id,
		%s AS amount_cents,
		%s AS tip_cents,
		COALESCE(payments.payment_method, '') AS payment_method,
		'%s' AS source,
		%s AS recognized_at,
		COALESCE(bills.bill_number, '') AS bill_number,
		bills.status AS bill_status,
		COALESCE(bills.total_amount, 0) AS bill_total_cents,
		bills.created_at AS bill_created_at,
		COALESCE(payments.payer_addr, '') AS payer_addr,
		COALESCE(payments.tx_hash, '') AS tx_hash,
		payments.id AS source_row_id
	`, amountColumn, tipColumn, source, recognizedAtColumn)
}

func alternativePaymentEventSelect(recognizedAtColumn string, reverse bool) string {
	amountColumn := "COALESCE(alternative_payments.amount, 0)"
	tipColumn := "COALESCE(alternative_payments.tip_amount_cents, 0)"
	source := recognizedPaymentSourceAlternative
	if reverse {
		amountColumn = "(-COALESCE(alternative_payments.amount, 0))"
		tipColumn = "(-COALESCE(alternative_payments.tip_amount_cents, 0))"
		source = recognizedPaymentSourceReversal
	}

	return fmt.Sprintf(`
		alternative_payments.bill_id AS bill_id,
		bills.business_id AS business_id,
		%s AS amount_cents,
		%s AS tip_cents,
		COALESCE(alternative_payments.payment_method, '') AS payment_method,
		'%s' AS source,
		%s AS recognized_at,
		COALESCE(bills.bill_number, '') AS bill_number,
		bills.status AS bill_status,
		COALESCE(bills.total_amount, 0) AS bill_total_cents,
		bills.created_at AS bill_created_at,
		'' AS payer_addr,
		'' AS tx_hash,
		alternative_payments.id AS source_row_id
	`, amountColumn, tipColumn, source, recognizedAtColumn)
}

func legacyBillFallbackEventSelect(recognizedAtColumn string) string {
	return fmt.Sprintf(`
		bills.id AS bill_id,
		bills.business_id AS business_id,
		CASE
			WHEN COALESCE(bills.paid_amount, 0) > 0 THEN COALESCE(bills.paid_amount, 0)
			ELSE COALESCE(bills.total_amount, 0)
		END AS amount_cents,
		COALESCE(bills.tip_amount, 0) AS tip_cents,
		'legacy_bill' AS payment_method,
		'%s' AS source,
		%s AS recognized_at,
		COALESCE(bills.bill_number, '') AS bill_number,
		bills.status AS bill_status,
		COALESCE(bills.total_amount, 0) AS bill_total_cents,
		bills.created_at AS bill_created_at,
		'' AS payer_addr,
		'' AS tx_hash,
		bills.id AS source_row_id
	`, recognizedPaymentSourceLegacyBillFallback, recognizedAtColumn)
}

func SummarizeRecognizedPaymentEvents(events []RecognizedPaymentEvent) RecognizedPaymentSummary {
	summary := RecognizedPaymentSummary{
		RevenueByMethod: make(map[string]int64),
		TipByMethod:     make(map[string]int64),
		CountByMethod:   make(map[string]int64),
	}

	for _, event := range events {
		method := NormalizeRecognizedPaymentMethod(event.PaymentMethod, "", "")
		summary.TotalRevenueCents += event.AmountCents
		summary.TotalTipCents += event.TipCents
		summary.EventCount++
		if event.AmountCents > 0 || event.TipCents > 0 {
			summary.GrossRevenueCents += event.AmountCents
			summary.GrossTipCents += event.TipCents
			summary.PositiveEventCount++
		} else if event.AmountCents < 0 || event.TipCents < 0 {
			summary.ReversalEventCount++
		}
		summary.RevenueByMethod[method] += event.AmountCents
		summary.TipByMethod[method] += event.TipCents
		summary.CountByMethod[method]++
	}

	return summary
}

func GetRecognizedPaymentSummary(businessID uint, startDate, endDate time.Time) (RecognizedPaymentSummary, error) {
	return getRecognizedPaymentSummary(&businessID, startDate, endDate)
}

func GetRecognizedPaymentSummaryForAllBusinesses(startDate, endDate time.Time) (RecognizedPaymentSummary, error) {
	return getRecognizedPaymentSummary(nil, startDate, endDate)
}

func GetRecognizedPaymentMonthlySummaryForAllBusinesses(startDate, endDate time.Time) (map[string]RecognizedPaymentSummary, error) {
	return getRecognizedPaymentMonthlySummary(nil, startDate, endDate)
}

func GetRecognizedPaymentBillCountForAllBusinesses(startDate, endDate time.Time) (int64, error) {
	if db == nil {
		return 0, gorm.ErrInvalidDB
	}

	var count int64
	err := db.Raw(`
		SELECT COUNT(*) FROM (
			SELECT bill_id
			FROM (
				SELECT payments.bill_id AS bill_id, COALESCE(payments.amount, 0) AS amount_cents
				FROM payments
				JOIN bills ON payments.bill_id = bills.id
				WHERE payments.status IN ?
					AND payments.confirmed_at IS NOT NULL
					AND payments.confirmed_at >= ? AND payments.confirmed_at < ?
			UNION ALL
				SELECT payments.bill_id AS bill_id, COALESCE(payments.amount, 0) AS amount_cents
				FROM payments
				JOIN bills ON payments.bill_id = bills.id
				WHERE payments.status = ?
					AND payments.confirmed_at IS NULL
					AND payments.updated_at >= ? AND payments.updated_at < ?
			UNION ALL
				SELECT payments.bill_id AS bill_id, COALESCE(payments.amount, 0) AS amount_cents
				FROM payments
				JOIN bills ON payments.bill_id = bills.id
				WHERE payments.status IN ?
					AND payments.confirmed_at IS NULL
					AND payments.created_at >= ? AND payments.created_at < ?
			UNION ALL
				SELECT payments.bill_id AS bill_id, (-COALESCE(payments.amount, 0)) AS amount_cents
				FROM payments
				JOIN bills ON payments.bill_id = bills.id
				WHERE payments.status IN ?
					AND payments.reversed_at IS NOT NULL
					AND payments.reversed_at >= ? AND payments.reversed_at < ?
			UNION ALL
				SELECT payments.bill_id AS bill_id, (-COALESCE(payments.amount, 0)) AS amount_cents
				FROM payments
				JOIN bills ON payments.bill_id = bills.id
				WHERE payments.status IN ?
					AND payments.reversed_at IS NULL
					AND payments.updated_at >= ? AND payments.updated_at < ?
			UNION ALL
				SELECT alternative_payments.bill_id AS bill_id, COALESCE(alternative_payments.amount, 0) AS amount_cents
				FROM alternative_payments
				JOIN bills ON alternative_payments.bill_id = bills.id
				WHERE alternative_payments.status IN ?
					AND alternative_payments.payment_method IN ?
					AND alternative_payments.confirmed_at IS NOT NULL
					AND alternative_payments.confirmed_at >= ? AND alternative_payments.confirmed_at < ?
			UNION ALL
				SELECT alternative_payments.bill_id AS bill_id, COALESCE(alternative_payments.amount, 0) AS amount_cents
				FROM alternative_payments
				JOIN bills ON alternative_payments.bill_id = bills.id
				WHERE alternative_payments.status IN ?
					AND alternative_payments.payment_method IN ?
					AND alternative_payments.confirmed_at IS NULL
					AND alternative_payments.created_at >= ? AND alternative_payments.created_at < ?
			UNION ALL
				SELECT alternative_payments.bill_id AS bill_id, (-COALESCE(alternative_payments.amount, 0)) AS amount_cents
				FROM alternative_payments
				JOIN bills ON alternative_payments.bill_id = bills.id
				WHERE alternative_payments.status = ?
					AND alternative_payments.payment_method IN ?
					AND alternative_payments.updated_at >= ? AND alternative_payments.updated_at < ?
			UNION ALL
				SELECT bills.id AS bill_id, CASE
					WHEN COALESCE(bills.paid_amount, 0) > 0 THEN COALESCE(bills.paid_amount, 0)
					ELSE COALESCE(bills.total_amount, 0)
				END AS amount_cents
				FROM bills
				WHERE (bills.paid_amount > ? OR (bills.status = ? AND bills.total_amount > ?))
					AND NOT EXISTS (SELECT 1 FROM payments recognized_payments WHERE recognized_payments.bill_id = bills.id AND recognized_payments.status IN ?)
					AND NOT EXISTS (SELECT 1 FROM alternative_payments recognized_alternative_payments WHERE recognized_alternative_payments.bill_id = bills.id AND recognized_alternative_payments.status IN ? AND recognized_alternative_payments.payment_method IN ?)
					AND bills.closed_at IS NOT NULL
					AND bills.closed_at >= ? AND bills.closed_at < ?
			UNION ALL
				SELECT bills.id AS bill_id, CASE
					WHEN COALESCE(bills.paid_amount, 0) > 0 THEN COALESCE(bills.paid_amount, 0)
					ELSE COALESCE(bills.total_amount, 0)
				END AS amount_cents
				FROM bills
				WHERE (bills.paid_amount > ? OR (bills.status = ? AND bills.total_amount > ?))
					AND NOT EXISTS (SELECT 1 FROM payments recognized_payments WHERE recognized_payments.bill_id = bills.id AND recognized_payments.status IN ?)
					AND NOT EXISTS (SELECT 1 FROM alternative_payments recognized_alternative_payments WHERE recognized_alternative_payments.bill_id = bills.id AND recognized_alternative_payments.status IN ? AND recognized_alternative_payments.payment_method IN ?)
					AND bills.closed_at IS NULL
					AND bills.created_at >= ? AND bills.created_at < ?
			UNION ALL
				SELECT bills.id AS bill_id, CASE
					WHEN COALESCE(bills.paid_amount, 0) > 0 THEN COALESCE(bills.paid_amount, 0)
					ELSE COALESCE(bills.total_amount, 0)
				END AS amount_cents
				FROM bills
				WHERE (bills.paid_amount > ? OR (bills.status = ? AND bills.total_amount > ?))
					AND NOT EXISTS (SELECT 1 FROM payments recognized_payments WHERE recognized_payments.bill_id = bills.id AND recognized_payments.status IN ?)
					AND NOT EXISTS (SELECT 1 FROM alternative_payments recognized_alternative_payments WHERE recognized_alternative_payments.bill_id = bills.id AND recognized_alternative_payments.status IN ? AND recognized_alternative_payments.payment_method IN ?)
					AND bills.closed_at IS NULL
					AND (bills.created_at < ? OR bills.created_at >= ?)
					AND bills.updated_at >= ? AND bills.updated_at < ?
			) recognized_events
			GROUP BY bill_id
			HAVING SUM(amount_cents) > 0
		) recognized_bills
	`,
		RecognizedPaymentStatuses, startDate, endDate,
		PaymentStatusConfirmed, startDate, endDate,
		ReversedOrRefundedPaymentStatuses, startDate, endDate,
		ReversedOrRefundedPaymentStatuses, startDate, endDate,
		ReversedOrRefundedPaymentStatuses, startDate, endDate,
		RecognizedAltPaymentStatuses, billManagedAlternativePaymentMethodStrings, startDate, endDate,
		RecognizedAltPaymentStatuses, billManagedAlternativePaymentMethodStrings, startDate, endDate,
		AltPaymentStatusRefunded, billManagedAlternativePaymentMethodStrings, startDate, endDate,
		0, BillStatusPaid, 0, RecognizedPaymentStatuses, RecognizedAltPaymentStatuses, billManagedAlternativePaymentMethodStrings, startDate, endDate,
		0, BillStatusPaid, 0, RecognizedPaymentStatuses, RecognizedAltPaymentStatuses, billManagedAlternativePaymentMethodStrings, startDate, endDate,
		0, BillStatusPaid, 0, RecognizedPaymentStatuses, RecognizedAltPaymentStatuses, billManagedAlternativePaymentMethodStrings, startDate, endDate, startDate, endDate,
	).Scan(&count).Error
	return count, err
}

func getRecognizedPaymentSummary(businessID *uint, startDate, endDate time.Time) (RecognizedPaymentSummary, error) {
	if db == nil {
		return RecognizedPaymentSummary{}, gorm.ErrInvalidDB
	}

	summary := newRecognizedPaymentSummary()
	appenders := []func(*RecognizedPaymentSummary, *uint, time.Time, time.Time) error{
		appendConfirmedPaymentSummary,
		appendLegacyConfirmedPaymentSummary,
		appendLegacyReversedPaymentSummary,
		appendReversalPaymentSummary,
		appendLegacyReversalPaymentSummary,
		appendAlternativePaymentSummary,
		appendLegacyAlternativePaymentSummary,
		appendAlternativeRefundReversalSummary,
		appendLegacyBillFallbackSummary,
	}

	for _, appendSummary := range appenders {
		if err := appendSummary(&summary, businessID, startDate, endDate); err != nil {
			return RecognizedPaymentSummary{}, err
		}
	}

	return summary, nil
}

func newRecognizedPaymentSummary() RecognizedPaymentSummary {
	return RecognizedPaymentSummary{
		RevenueByMethod: make(map[string]int64),
		TipByMethod:     make(map[string]int64),
		CountByMethod:   make(map[string]int64),
	}
}

func getRecognizedPaymentMonthlySummary(businessID *uint, startDate, endDate time.Time) (map[string]RecognizedPaymentSummary, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}

	summaries := make(map[string]RecognizedPaymentSummary)
	appenders := []func(map[string]RecognizedPaymentSummary, *uint, time.Time, time.Time) error{
		appendConfirmedPaymentMonthlySummary,
		appendLegacyConfirmedPaymentMonthlySummary,
		appendLegacyReversedPaymentMonthlySummary,
		appendReversalPaymentMonthlySummary,
		appendLegacyReversalPaymentMonthlySummary,
		appendAlternativePaymentMonthlySummary,
		appendLegacyAlternativePaymentMonthlySummary,
		appendAlternativeRefundReversalMonthlySummary,
		appendLegacyBillFallbackMonthlySummary,
	}

	for _, appendMonthlySummary := range appenders {
		if err := appendMonthlySummary(summaries, businessID, startDate, endDate); err != nil {
			return nil, err
		}
	}

	return summaries, nil
}

func appendConfirmedPaymentMonthlySummary(summaries map[string]RecognizedPaymentSummary, businessID *uint, startDate, endDate time.Time) error {
	methodExpr := paymentSummaryMethodExpression()
	monthExpr := paymentLedgerMonthKeyExpression("payments.confirmed_at")
	query := paymentEventBaseQuery(businessID).
		Select(paymentMonthlySummarySelect(monthExpr, methodExpr, "COALESCE(payments.amount, 0)", "COALESCE(payments.tip_amount, 0)", true)).
		Where("payments.status IN ?", RecognizedPaymentStatuses).
		Where("payments.confirmed_at IS NOT NULL").
		Where("payments.confirmed_at >= ? AND payments.confirmed_at < ?", startDate, endDate).
		Group(monthExpr + ", " + methodExpr)

	return appendPaymentLedgerMonthlySummaryRows(summaries, query)
}

func appendLegacyConfirmedPaymentMonthlySummary(summaries map[string]RecognizedPaymentSummary, businessID *uint, startDate, endDate time.Time) error {
	methodExpr := paymentSummaryMethodExpression()
	monthExpr := paymentLedgerMonthKeyExpression("payments.updated_at")
	query := paymentEventBaseQuery(businessID).
		Select(paymentMonthlySummarySelect(monthExpr, methodExpr, "COALESCE(payments.amount, 0)", "COALESCE(payments.tip_amount, 0)", true)).
		Where("payments.status = ?", PaymentStatusConfirmed).
		Where("payments.confirmed_at IS NULL").
		Where("payments.updated_at >= ? AND payments.updated_at < ?", startDate, endDate).
		Group(monthExpr + ", " + methodExpr)

	return appendPaymentLedgerMonthlySummaryRows(summaries, query)
}

func appendLegacyReversedPaymentMonthlySummary(summaries map[string]RecognizedPaymentSummary, businessID *uint, startDate, endDate time.Time) error {
	methodExpr := paymentSummaryMethodExpression()
	monthExpr := paymentLedgerMonthKeyExpression("payments.created_at")
	query := paymentEventBaseQuery(businessID).
		Select(paymentMonthlySummarySelect(monthExpr, methodExpr, "COALESCE(payments.amount, 0)", "COALESCE(payments.tip_amount, 0)", true)).
		Where("payments.status IN ?", ReversedOrRefundedPaymentStatuses).
		Where("payments.confirmed_at IS NULL").
		Where("payments.created_at >= ? AND payments.created_at < ?", startDate, endDate).
		Group(monthExpr + ", " + methodExpr)

	return appendPaymentLedgerMonthlySummaryRows(summaries, query)
}

func appendReversalPaymentMonthlySummary(summaries map[string]RecognizedPaymentSummary, businessID *uint, startDate, endDate time.Time) error {
	methodExpr := paymentSummaryMethodExpression()
	monthExpr := paymentLedgerMonthKeyExpression("payments.reversed_at")
	query := paymentEventBaseQuery(businessID).
		Select(paymentMonthlySummarySelect(monthExpr, methodExpr, "(-COALESCE(payments.amount, 0))", "(-COALESCE(payments.tip_amount, 0))", false)).
		Where("payments.status IN ?", ReversedOrRefundedPaymentStatuses).
		Where("payments.reversed_at IS NOT NULL").
		Where("payments.reversed_at >= ? AND payments.reversed_at < ?", startDate, endDate).
		Group(monthExpr + ", " + methodExpr)

	return appendPaymentLedgerMonthlySummaryRows(summaries, query)
}

func appendLegacyReversalPaymentMonthlySummary(summaries map[string]RecognizedPaymentSummary, businessID *uint, startDate, endDate time.Time) error {
	methodExpr := paymentSummaryMethodExpression()
	monthExpr := paymentLedgerMonthKeyExpression("payments.updated_at")
	query := paymentEventBaseQuery(businessID).
		Select(paymentMonthlySummarySelect(monthExpr, methodExpr, "(-COALESCE(payments.amount, 0))", "(-COALESCE(payments.tip_amount, 0))", false)).
		Where("payments.status IN ?", ReversedOrRefundedPaymentStatuses).
		Where("payments.reversed_at IS NULL").
		Where("payments.updated_at >= ? AND payments.updated_at < ?", startDate, endDate).
		Group(monthExpr + ", " + methodExpr)

	return appendPaymentLedgerMonthlySummaryRows(summaries, query)
}

func appendAlternativePaymentMonthlySummary(summaries map[string]RecognizedPaymentSummary, businessID *uint, startDate, endDate time.Time) error {
	methodExpr := alternativePaymentSummaryMethodExpression()
	monthExpr := paymentLedgerMonthKeyExpression("alternative_payments.confirmed_at")
	query := alternativePaymentEventBaseQuery(businessID).
		Select(paymentMonthlySummarySelect(monthExpr, methodExpr, "COALESCE(alternative_payments.amount, 0)", "0", true)).
		Where("alternative_payments.status IN ?", RecognizedAltPaymentStatuses).
		Where("alternative_payments.payment_method IN ?", billManagedAlternativePaymentMethodStrings).
		Where("alternative_payments.confirmed_at IS NOT NULL").
		Where("alternative_payments.confirmed_at >= ? AND alternative_payments.confirmed_at < ?", startDate, endDate).
		Group(monthExpr + ", " + methodExpr)

	return appendPaymentLedgerMonthlySummaryRows(summaries, query)
}

func appendLegacyAlternativePaymentMonthlySummary(summaries map[string]RecognizedPaymentSummary, businessID *uint, startDate, endDate time.Time) error {
	methodExpr := alternativePaymentSummaryMethodExpression()
	monthExpr := paymentLedgerMonthKeyExpression("alternative_payments.created_at")
	query := alternativePaymentEventBaseQuery(businessID).
		Select(paymentMonthlySummarySelect(monthExpr, methodExpr, "COALESCE(alternative_payments.amount, 0)", "0", true)).
		Where("alternative_payments.status IN ?", RecognizedAltPaymentStatuses).
		Where("alternative_payments.payment_method IN ?", billManagedAlternativePaymentMethodStrings).
		Where("alternative_payments.confirmed_at IS NULL").
		Where("alternative_payments.created_at >= ? AND alternative_payments.created_at < ?", startDate, endDate).
		Group(monthExpr + ", " + methodExpr)

	return appendPaymentLedgerMonthlySummaryRows(summaries, query)
}

func appendAlternativeRefundReversalMonthlySummary(summaries map[string]RecognizedPaymentSummary, businessID *uint, startDate, endDate time.Time) error {
	methodExpr := alternativePaymentSummaryMethodExpression()
	monthExpr := paymentLedgerMonthKeyExpression("alternative_payments.updated_at")
	query := alternativePaymentEventBaseQuery(businessID).
		Select(paymentMonthlySummarySelect(monthExpr, methodExpr, "(-COALESCE(alternative_payments.amount, 0))", "0", false)).
		Where("alternative_payments.status = ?", AltPaymentStatusRefunded).
		Where("alternative_payments.payment_method IN ?", billManagedAlternativePaymentMethodStrings).
		Where("alternative_payments.updated_at >= ? AND alternative_payments.updated_at < ?", startDate, endDate).
		Group(monthExpr + ", " + methodExpr)

	return appendPaymentLedgerMonthlySummaryRows(summaries, query)
}

func appendLegacyBillFallbackMonthlySummary(summaries map[string]RecognizedPaymentSummary, businessID *uint, startDate, endDate time.Time) error {
	closedAtQuery := legacyBillFallbackBaseQuery(businessID).
		Select(legacyBillFallbackMonthlySummarySelect(paymentLedgerMonthKeyExpression("bills.closed_at"))).
		Where("bills.closed_at IS NOT NULL").
		Where("bills.closed_at >= ? AND bills.closed_at < ?", startDate, endDate).
		Group(paymentLedgerMonthKeyExpression("bills.closed_at"))
	if err := appendPaymentLedgerMonthlySummaryRows(summaries, closedAtQuery); err != nil {
		return err
	}

	createdAtQuery := legacyBillFallbackBaseQuery(businessID).
		Select(legacyBillFallbackMonthlySummarySelect(paymentLedgerMonthKeyExpression("bills.created_at"))).
		Where("bills.closed_at IS NULL").
		Where("bills.created_at >= ? AND bills.created_at < ?", startDate, endDate).
		Group(paymentLedgerMonthKeyExpression("bills.created_at"))
	if err := appendPaymentLedgerMonthlySummaryRows(summaries, createdAtQuery); err != nil {
		return err
	}

	updatedAtQuery := legacyBillFallbackBaseQuery(businessID).
		Select(legacyBillFallbackMonthlySummarySelect(paymentLedgerMonthKeyExpression("bills.updated_at"))).
		Where("bills.closed_at IS NULL").
		Where("(bills.created_at < ? OR bills.created_at >= ?)", startDate, endDate).
		Where("bills.updated_at >= ? AND bills.updated_at < ?", startDate, endDate).
		Group(paymentLedgerMonthKeyExpression("bills.updated_at"))
	return appendPaymentLedgerMonthlySummaryRows(summaries, updatedAtQuery)
}

func appendConfirmedPaymentSummary(summary *RecognizedPaymentSummary, businessID *uint, startDate, endDate time.Time) error {
	methodExpr := paymentSummaryMethodExpression()
	query := paymentEventBaseQuery(businessID).
		Select(paymentSummarySelect(methodExpr, "COALESCE(payments.amount, 0)", "COALESCE(payments.tip_amount, 0)", true)).
		Where("payments.status IN ?", RecognizedPaymentStatuses).
		Where("payments.confirmed_at IS NOT NULL").
		Where("payments.confirmed_at >= ? AND payments.confirmed_at < ?", startDate, endDate).
		Group(methodExpr)

	return appendPaymentLedgerSummaryRows(summary, query)
}

func appendLegacyConfirmedPaymentSummary(summary *RecognizedPaymentSummary, businessID *uint, startDate, endDate time.Time) error {
	methodExpr := paymentSummaryMethodExpression()
	query := paymentEventBaseQuery(businessID).
		Select(paymentSummarySelect(methodExpr, "COALESCE(payments.amount, 0)", "COALESCE(payments.tip_amount, 0)", true)).
		Where("payments.status = ?", PaymentStatusConfirmed).
		Where("payments.confirmed_at IS NULL").
		Where("payments.updated_at >= ? AND payments.updated_at < ?", startDate, endDate).
		Group(methodExpr)

	return appendPaymentLedgerSummaryRows(summary, query)
}

func appendLegacyReversedPaymentSummary(summary *RecognizedPaymentSummary, businessID *uint, startDate, endDate time.Time) error {
	methodExpr := paymentSummaryMethodExpression()
	query := paymentEventBaseQuery(businessID).
		Select(paymentSummarySelect(methodExpr, "COALESCE(payments.amount, 0)", "COALESCE(payments.tip_amount, 0)", true)).
		Where("payments.status IN ?", ReversedOrRefundedPaymentStatuses).
		Where("payments.confirmed_at IS NULL").
		Where("payments.created_at >= ? AND payments.created_at < ?", startDate, endDate).
		Group(methodExpr)

	return appendPaymentLedgerSummaryRows(summary, query)
}

func appendReversalPaymentSummary(summary *RecognizedPaymentSummary, businessID *uint, startDate, endDate time.Time) error {
	methodExpr := paymentSummaryMethodExpression()
	query := paymentEventBaseQuery(businessID).
		Select(paymentSummarySelect(methodExpr, "(-COALESCE(payments.amount, 0))", "(-COALESCE(payments.tip_amount, 0))", false)).
		Where("payments.status IN ?", ReversedOrRefundedPaymentStatuses).
		Where("payments.reversed_at IS NOT NULL").
		Where("payments.reversed_at >= ? AND payments.reversed_at < ?", startDate, endDate).
		Group(methodExpr)

	return appendPaymentLedgerSummaryRows(summary, query)
}

func appendLegacyReversalPaymentSummary(summary *RecognizedPaymentSummary, businessID *uint, startDate, endDate time.Time) error {
	methodExpr := paymentSummaryMethodExpression()
	query := paymentEventBaseQuery(businessID).
		Select(paymentSummarySelect(methodExpr, "(-COALESCE(payments.amount, 0))", "(-COALESCE(payments.tip_amount, 0))", false)).
		Where("payments.status IN ?", ReversedOrRefundedPaymentStatuses).
		Where("payments.reversed_at IS NULL").
		Where("payments.updated_at >= ? AND payments.updated_at < ?", startDate, endDate).
		Group(methodExpr)

	return appendPaymentLedgerSummaryRows(summary, query)
}

func appendAlternativePaymentSummary(summary *RecognizedPaymentSummary, businessID *uint, startDate, endDate time.Time) error {
	methodExpr := alternativePaymentSummaryMethodExpression()
	query := alternativePaymentEventBaseQuery(businessID).
		Select(paymentSummarySelect(methodExpr, "COALESCE(alternative_payments.amount, 0)", "0", true)).
		Where("alternative_payments.status IN ?", RecognizedAltPaymentStatuses).
		Where("alternative_payments.payment_method IN ?", billManagedAlternativePaymentMethodStrings).
		Where("alternative_payments.confirmed_at IS NOT NULL").
		Where("alternative_payments.confirmed_at >= ? AND alternative_payments.confirmed_at < ?", startDate, endDate).
		Group(methodExpr)

	return appendPaymentLedgerSummaryRows(summary, query)
}

func appendLegacyAlternativePaymentSummary(summary *RecognizedPaymentSummary, businessID *uint, startDate, endDate time.Time) error {
	methodExpr := alternativePaymentSummaryMethodExpression()
	query := alternativePaymentEventBaseQuery(businessID).
		Select(paymentSummarySelect(methodExpr, "COALESCE(alternative_payments.amount, 0)", "0", true)).
		Where("alternative_payments.status IN ?", RecognizedAltPaymentStatuses).
		Where("alternative_payments.payment_method IN ?", billManagedAlternativePaymentMethodStrings).
		Where("alternative_payments.confirmed_at IS NULL").
		Where("alternative_payments.created_at >= ? AND alternative_payments.created_at < ?", startDate, endDate).
		Group(methodExpr)

	return appendPaymentLedgerSummaryRows(summary, query)
}

func appendAlternativeRefundReversalSummary(summary *RecognizedPaymentSummary, businessID *uint, startDate, endDate time.Time) error {
	methodExpr := alternativePaymentSummaryMethodExpression()
	query := alternativePaymentEventBaseQuery(businessID).
		Select(paymentSummarySelect(methodExpr, "(-COALESCE(alternative_payments.amount, 0))", "0", false)).
		Where("alternative_payments.status = ?", AltPaymentStatusRefunded).
		Where("alternative_payments.payment_method IN ?", billManagedAlternativePaymentMethodStrings).
		Where("alternative_payments.updated_at >= ? AND alternative_payments.updated_at < ?", startDate, endDate).
		Group(methodExpr)

	return appendPaymentLedgerSummaryRows(summary, query)
}

func appendLegacyBillFallbackSummary(summary *RecognizedPaymentSummary, businessID *uint, startDate, endDate time.Time) error {
	closedAtQuery := legacyBillFallbackBaseQuery(businessID).
		Select(legacyBillFallbackSummarySelect()).
		Where("bills.closed_at IS NOT NULL").
		Where("bills.closed_at >= ? AND bills.closed_at < ?", startDate, endDate)
	if err := appendPaymentLedgerSummaryRows(summary, closedAtQuery); err != nil {
		return err
	}

	createdAtQuery := legacyBillFallbackBaseQuery(businessID).
		Select(legacyBillFallbackSummarySelect()).
		Where("bills.closed_at IS NULL").
		Where("bills.created_at >= ? AND bills.created_at < ?", startDate, endDate)
	if err := appendPaymentLedgerSummaryRows(summary, createdAtQuery); err != nil {
		return err
	}

	updatedAtQuery := legacyBillFallbackBaseQuery(businessID).
		Select(legacyBillFallbackSummarySelect()).
		Where("bills.closed_at IS NULL").
		Where("(bills.created_at < ? OR bills.created_at >= ?)", startDate, endDate).
		Where("bills.updated_at >= ? AND bills.updated_at < ?", startDate, endDate)
	return appendPaymentLedgerSummaryRows(summary, updatedAtQuery)
}

func appendPaymentLedgerSummaryRows(summary *RecognizedPaymentSummary, query *gorm.DB) error {
	var rows []paymentLedgerSummaryRow
	if err := query.Scan(&rows).Error; err != nil {
		return err
	}

	for _, row := range rows {
		appendPaymentLedgerSummaryRow(summary, row)
	}
	return nil
}

func appendPaymentLedgerMonthlySummaryRows(summaries map[string]RecognizedPaymentSummary, query *gorm.DB) error {
	var rows []paymentLedgerMonthlySummaryRow
	if err := query.Scan(&rows).Error; err != nil {
		return err
	}

	for _, row := range rows {
		if strings.TrimSpace(row.MonthKey) == "" {
			continue
		}
		summary, ok := summaries[row.MonthKey]
		if !ok {
			summary = newRecognizedPaymentSummary()
		}
		appendPaymentLedgerSummaryRow(&summary, paymentLedgerSummaryRow{
			PaymentMethod:      row.PaymentMethod,
			RevenueCents:       row.RevenueCents,
			TipCents:           row.TipCents,
			GrossRevenueCents:  row.GrossRevenueCents,
			GrossTipCents:      row.GrossTipCents,
			EventCount:         row.EventCount,
			PositiveEventCount: row.PositiveEventCount,
			ReversalEventCount: row.ReversalEventCount,
		})
		summaries[row.MonthKey] = summary
	}
	return nil
}

func appendPaymentLedgerSummaryRow(summary *RecognizedPaymentSummary, row paymentLedgerSummaryRow) {
	if row.EventCount == 0 {
		return
	}

	method := NormalizeRecognizedPaymentMethod(row.PaymentMethod, "", "")
	summary.TotalRevenueCents += row.RevenueCents
	summary.TotalTipCents += row.TipCents
	summary.GrossRevenueCents += row.GrossRevenueCents
	summary.GrossTipCents += row.GrossTipCents
	summary.EventCount += row.EventCount
	summary.PositiveEventCount += row.PositiveEventCount
	summary.ReversalEventCount += row.ReversalEventCount
	summary.RevenueByMethod[method] += row.RevenueCents
	summary.TipByMethod[method] += row.TipCents
	summary.CountByMethod[method] += row.EventCount
}

func paymentSummaryMethodExpression() string {
	return `
		CASE
			WHEN LOWER(TRIM(COALESCE(payments.payment_method, ''))) = 'plugin'
				OR LOWER(TRIM(COALESCE(payments.payer_addr, ''))) = 'plugin'
				OR SUBSTR(LOWER(TRIM(COALESCE(payments.tx_hash, ''))), 1, 7) = 'plugin_'
				THEN 'plugin'
			WHEN TRIM(COALESCE(payments.payment_method, '')) = '' THEN 'crypto'
			ELSE LOWER(TRIM(COALESCE(payments.payment_method, '')))
		END
	`
}

func alternativePaymentSummaryMethodExpression() string {
	return "LOWER(TRIM(COALESCE(alternative_payments.payment_method, '')))"
}

func paymentLedgerMonthKeyExpression(recognizedAtColumn string) string {
	if db != nil && db.Name() == "postgres" {
		return fmt.Sprintf("TO_CHAR(DATE_TRUNC('month', %s), 'YYYY-MM')", recognizedAtColumn)
	}
	return fmt.Sprintf("strftime('%%Y-%%m', %s)", recognizedAtColumn)
}

func paymentSummarySelect(methodExpr, amountExpr, tipExpr string, positive bool) string {
	grossRevenueExpr := "0"
	grossTipExpr := "0"
	positiveCountExpr := "0"
	reversalCountExpr := "COUNT(*)"
	if positive {
		grossRevenueExpr = fmt.Sprintf("COALESCE(SUM(%s), 0)", amountExpr)
		grossTipExpr = fmt.Sprintf("COALESCE(SUM(%s), 0)", tipExpr)
		positiveCountExpr = "COUNT(*)"
		reversalCountExpr = "0"
	}

	return fmt.Sprintf(`
		%s AS payment_method,
		COALESCE(SUM(%s), 0) AS revenue_cents,
		COALESCE(SUM(%s), 0) AS tip_cents,
		%s AS gross_revenue_cents,
		%s AS gross_tip_cents,
		COUNT(*) AS event_count,
		%s AS positive_event_count,
		%s AS reversal_event_count
	`, methodExpr, amountExpr, tipExpr, grossRevenueExpr, grossTipExpr, positiveCountExpr, reversalCountExpr)
}

func paymentMonthlySummarySelect(monthExpr, methodExpr, amountExpr, tipExpr string, positive bool) string {
	return fmt.Sprintf(`
		%s AS month_key,
		%s
	`, monthExpr, paymentSummarySelect(methodExpr, amountExpr, tipExpr, positive))
}

func legacyBillFallbackSummarySelect() string {
	return `
		'legacy_bill' AS payment_method,
		COALESCE(SUM(CASE
			WHEN COALESCE(bills.paid_amount, 0) > 0 THEN COALESCE(bills.paid_amount, 0)
			ELSE COALESCE(bills.total_amount, 0)
		END), 0) AS revenue_cents,
		COALESCE(SUM(COALESCE(bills.tip_amount, 0)), 0) AS tip_cents,
		COALESCE(SUM(CASE
			WHEN COALESCE(bills.paid_amount, 0) > 0 THEN COALESCE(bills.paid_amount, 0)
			ELSE COALESCE(bills.total_amount, 0)
		END), 0) AS gross_revenue_cents,
		COALESCE(SUM(COALESCE(bills.tip_amount, 0)), 0) AS gross_tip_cents,
		COUNT(*) AS event_count,
		COUNT(*) AS positive_event_count,
		0 AS reversal_event_count
	`
}

func legacyBillFallbackMonthlySummarySelect(monthExpr string) string {
	return fmt.Sprintf(`
		%s AS month_key,
		%s
	`, monthExpr, legacyBillFallbackSummarySelect())
}
