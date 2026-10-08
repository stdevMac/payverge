package database

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrCashRegisterInvalidAttachMovementType = errors.New("invalid cash register attach movement type")
	ErrCashRegisterPaymentBillNotFound       = errors.New("cash register payment bill not found")
	ErrCashRegisterPaymentBusinessMismatch   = errors.New("cash register payment business mismatch")
)

type CashRegisterActor struct {
	UserID  *uint
	StaffID *uint
	Label   string
}

type CashRegisterTotals struct {
	CashSalesCents   int64
	CashRefundsCents int64
	CashInCents      int64
	CashOutCents     int64
}

func alternativePaymentTotalCashCents(payment AlternativePayment) int64 {
	if payment.BillAmountCents > 0 || payment.TipAmountCents > 0 {
		return payment.BillAmountCents + payment.TipAmountCents
	}
	return payment.Amount
}

func cashRegisterActorForAlternativePayment(payment AlternativePayment, closingStaffID *uint, fallbackLabel string) CashRegisterActor {
	if closingStaffID != nil {
		return CashRegisterActor{
			StaffID: closingStaffID,
			Label:   fmt.Sprintf("staff:%d", *closingStaffID),
		}
	}
	if strings.TrimSpace(payment.ConfirmedBy) != "" {
		return CashRegisterActor{Label: strings.TrimSpace(payment.ConfirmedBy)}
	}
	return CashRegisterActor{Label: fallbackLabel}
}

func FindOpenCashRegisterSessionForBusinessTx(tx *gorm.DB, businessID uint) (*CashRegisterSession, error) {
	var session CashRegisterSession
	err := tx.Where("business_id = ? AND status = ?", businessID, CashRegisterSessionStatusOpen).
		First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// FindLastDeclaredOpeningFloatCentsTx returns the newest closed session's
// declared starting bank. Issue #652: this is opening_float_cents only — never
// counted_cash_cents, which already includes that close's over/short.
func FindLastDeclaredOpeningFloatCentsTx(tx *gorm.DB, businessID uint) (*int64, error) {
	var session CashRegisterSession
	err := tx.Select("opening_float_cents").
		Where("business_id = ? AND status = ?", businessID, CashRegisterSessionStatusClosed).
		Order("closed_at DESC").
		Order("id DESC").
		Take(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	cents := session.OpeningFloatCents
	return &cents, nil
}

func RecalculateCashRegisterSessionTotalsTx(tx *gorm.DB, sessionID uint) (CashRegisterTotals, error) {
	var totals CashRegisterTotals
	err := tx.Model(&CashRegisterMovement{}).
		Select(`
			COALESCE(SUM(CASE WHEN movement_type = ? THEN amount_cents ELSE 0 END), 0) AS cash_sales_cents,
			COALESCE(SUM(CASE WHEN movement_type = ? THEN amount_cents ELSE 0 END), 0) AS cash_refunds_cents,
			COALESCE(SUM(CASE WHEN movement_type = ? THEN amount_cents ELSE 0 END), 0) AS cash_in_cents,
			COALESCE(SUM(CASE WHEN movement_type = ? THEN amount_cents ELSE 0 END), 0) AS cash_out_cents
		`,
			CashRegisterMovementTypeCashSale,
			CashRegisterMovementTypeCashRefund,
			CashRegisterMovementTypeCashIn,
			CashRegisterMovementTypeCashOut,
		).
		Where("session_id = ?", sessionID).
		Scan(&totals).Error
	return totals, err
}

func UpdateCashRegisterSessionTotalsTx(tx *gorm.DB, sessionID uint, totals CashRegisterTotals) error {
	return tx.Model(&CashRegisterSession{}).
		Where("id = ?", sessionID).
		Updates(map[string]interface{}{
			"cash_sales_cents":   totals.CashSalesCents,
			"cash_refunds_cents": totals.CashRefundsCents,
			"cash_in_cents":      totals.CashInCents,
			"cash_out_cents":     totals.CashOutCents,
		}).Error
}

func AttachCashRegisterMovementForAlternativePaymentTx(tx *gorm.DB, businessID uint, payment AlternativePayment, movementType CashRegisterMovementType, amountCents int64, actor CashRegisterActor, occurredAt time.Time) error {
	if movementType != CashRegisterMovementTypeCashSale && movementType != CashRegisterMovementTypeCashRefund {
		return ErrCashRegisterInvalidAttachMovementType
	}
	if payment.PaymentMethod != PaymentMethodCash || amountCents <= 0 {
		return nil
	}

	var session CashRegisterSession
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("business_id = ? AND status = ?", businessID, CashRegisterSessionStatusOpen).
		First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	var bill Bill
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id", "business_id").
		Where("id = ?", payment.BillID).
		First(&bill).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCashRegisterPaymentBillNotFound
		}
		return fmt.Errorf("load cash register payment bill: %w", err)
	}
	if bill.BusinessID != businessID {
		return ErrCashRegisterPaymentBusinessMismatch
	}

	reason := "cash payment"
	if movementType == CashRegisterMovementTypeCashRefund {
		reason = "cash refund"
	}

	movement := CashRegisterMovement{
		BusinessID:           businessID,
		SessionID:            session.ID,
		MovementType:         movementType,
		AmountCents:          amountCents,
		Reason:               reason,
		AlternativePaymentID: &payment.ID,
		BillID:               &payment.BillID,
		ActorUserID:          actor.UserID,
		ActorStaffID:         actor.StaffID,
		ActorLabel:           actor.Label,
		OccurredAt:           occurredAt,
	}
	result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&movement)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return nil
	}
	return incrementOpenCashRegisterSessionTotalTx(tx, session.ID, movementType, amountCents)
}

func incrementOpenCashRegisterSessionTotalTx(tx *gorm.DB, sessionID uint, movementType CashRegisterMovementType, amountCents int64) error {
	column := "cash_sales_cents"
	if movementType == CashRegisterMovementTypeCashRefund {
		column = "cash_refunds_cents"
	}
	return tx.Model(&CashRegisterSession{}).
		Where("id = ? AND status = ?", sessionID, CashRegisterSessionStatusOpen).
		Update(column, gorm.Expr(column+" + ?", amountCents)).Error
}

func UnassignedCashAlternativePaymentSummary(db *gorm.DB, businessID uint) (count int64, totalCents int64, err error) {
	row := db.Raw(`
		SELECT COUNT(*), COALESCE(SUM(cash_amount_cents), 0)
		FROM (
			SELECT
				CASE
					WHEN COALESCE(ap.bill_amount_cents, 0) > 0 OR COALESCE(ap.tip_amount_cents, 0) > 0
					THEN COALESCE(ap.bill_amount_cents, 0) + COALESCE(ap.tip_amount_cents, 0)
					ELSE ap.amount
				END AS cash_amount_cents
			FROM alternative_payments ap
			JOIN bills b ON b.id = ap.bill_id
			LEFT JOIN cash_register_movements crm
			  ON crm.alternative_payment_id = ap.id
			 AND crm.movement_type = ?
			 AND crm.business_id = b.business_id
			WHERE b.business_id = ?
			  AND ap.payment_method = ?
			  AND ap.status IN (?, ?)
			  AND crm.id IS NULL

			UNION ALL

			SELECT
				-CASE
					WHEN COALESCE(ap.bill_amount_cents, 0) > 0 OR COALESCE(ap.tip_amount_cents, 0) > 0
					THEN COALESCE(ap.bill_amount_cents, 0) + COALESCE(ap.tip_amount_cents, 0)
					ELSE ap.amount
				END AS cash_amount_cents
			FROM alternative_payments ap
			JOIN bills b ON b.id = ap.bill_id
			LEFT JOIN cash_register_movements crm
			  ON crm.alternative_payment_id = ap.id
			 AND crm.movement_type = ?
			 AND crm.business_id = b.business_id
			WHERE b.business_id = ?
			  AND ap.payment_method = ?
			  AND ap.status = ?
			  AND crm.id IS NULL
		) unassigned_cash_events
	`, CashRegisterMovementTypeCashSale, businessID, PaymentMethodCash, AltPaymentStatusConfirmed, AltPaymentStatusRefunded,
		CashRegisterMovementTypeCashRefund, businessID, PaymentMethodCash, AltPaymentStatusRefunded).Row()
	err = row.Scan(&count, &totalCents)
	return count, totalCents, err
}

// UnassignedCashAlternativePaymentItem is one inspectable line of the unassigned
// cash panel: a confirmed cash tender whose cash-sale hasn't been recorded in a
// register session. AmountCents is the cash value of that tender.
type UnassignedCashAlternativePaymentItem struct {
	ID              uint      `json:"id"`
	BillID          uint      `json:"bill_id"`
	BillNumber      string    `json:"bill_number"`
	TableName       string    `json:"table_name"`
	ParticipantName string    `json:"participant_name"`
	AmountCents     int64     `json:"amount_cents"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

// ListUnassignedCashAlternativePayments returns the individual confirmed cash
// tenders that have not yet been recorded as a cash-sale in any register session
// (the "cash-in" leg of the unassigned-cash summary), newest first, bounded by
// limit/offset. It mirrors the confirmed-cash-sale branch of the summary query
// so the list and the count stay consistent. Returns the page plus the total
// matching row count.
func ListUnassignedCashAlternativePayments(db *gorm.DB, businessID uint, limit, offset int) ([]UnassignedCashAlternativePaymentItem, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	// Base builder: confirmed/refunded cash alt-payments on this business's bills
	// with NO cash-sale movement recorded (the unassigned cash-in leg). Mirrors
	// the summary query's predicates so the list and count stay consistent.
	base := db.
		Table("alternative_payments AS ap").
		Joins("JOIN bills b ON b.id = ap.bill_id").
		Joins(`LEFT JOIN cash_register_movements crm
			ON crm.alternative_payment_id = ap.id
			AND crm.movement_type = ?
			AND crm.business_id = b.business_id`, CashRegisterMovementTypeCashSale).
		Where("b.business_id = ?", businessID).
		Where("ap.payment_method = ?", PaymentMethodCash).
		Where("ap.status IN (?, ?)", AltPaymentStatusConfirmed, AltPaymentStatusRefunded).
		Where("crm.id IS NULL")

	var total int64
	if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []UnassignedCashAlternativePaymentItem
	err := base.Session(&gorm.Session{}).
		Select(`
			ap.id AS id,
			ap.bill_id AS bill_id,
			COALESCE(b.bill_number, '') AS bill_number,
			'' AS table_name,
			COALESCE(ap.participant_name, '') AS participant_name,
			CASE
				WHEN COALESCE(ap.bill_amount_cents, 0) > 0 OR COALESCE(ap.tip_amount_cents, 0) > 0
				THEN COALESCE(ap.bill_amount_cents, 0) + COALESCE(ap.tip_amount_cents, 0)
				ELSE ap.amount
			END AS amount_cents,
			ap.status AS status,
			ap.created_at AS created_at
		`).
		Order("ap.created_at DESC, ap.id DESC").
		Limit(limit).
		Offset(offset).
		Scan(&items).Error
	if err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

// BusinessUsesDemoHouseRailTx reports whether a business is a Demo Center
// showroom (kind=demo). Leftover real venues with a sticky is_demo=true flag
// must not auto-open Caja. Demo showrooms keep payment plugins off on purpose
// (#711 / #526) and need a house cash drawer to take dinner (#769).
func BusinessUsesDemoHouseRailTx(tx *gorm.DB, businessID uint) (bool, error) {
	var row struct {
		Kind string `gorm:"column:kind"`
	}
	err := tx.Table("businesses").
		Select("kind").
		Where("id = ?", businessID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(row.Kind), string(BusinessKindDemo)), nil
}

// FindLastClosedCashRegisterSessionTx returns the newest closed session, or
// nil when the business has no close ledger.
func FindLastClosedCashRegisterSessionTx(tx *gorm.DB, businessID uint) (*CashRegisterSession, error) {
	var session CashRegisterSession
	err := tx.Where("business_id = ? AND status = ?", businessID, CashRegisterSessionStatusClosed).
		Order("closed_at DESC").
		Order("id DESC").
		Take(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// HasHumanClosedCashRegisterSessionTx reports whether an operator has ever
// closed a drawer on this venue. Actor ids are the durable marker — display
// labels are overwritten by the next demo-manager midnight seed close.
func HasHumanClosedCashRegisterSessionTx(tx *gorm.DB, businessID uint) (bool, error) {
	var id uint
	err := tx.Model(&CashRegisterSession{}).
		Select("id").
		Where("business_id = ? AND status = ?", businessID, CashRegisterSessionStatusClosed).
		Where("closed_by_user_id IS NOT NULL OR closed_by_staff_id IS NOT NULL").
		Limit(1).
		Scan(&id).Error
	if err != nil {
		return false, err
	}
	return id != 0, nil
}
