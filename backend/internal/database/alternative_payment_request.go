package database

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const AlternativePaymentRequestTTL = 5 * time.Minute

// AlternativePaymentRequestMaxAge is the outer confirmability window for a
// pending request, derived from created_at so rows with a NULL expires_at
// (legacy guest/operator requests) cannot stay confirmable for days.
const AlternativePaymentRequestMaxAge = 24 * time.Hour

// AlternativePaymentRequestExpiresAt is the earlier of stored expires_at and
// created_at + 24h. Missing expires_at does not mean "never expires".
func AlternativePaymentRequestExpiresAt(payment AlternativePayment) time.Time {
	created := payment.CreatedAt.UTC()
	if created.IsZero() {
		created = payment.UpdatedAt.UTC()
	}
	derived := created.Add(AlternativePaymentRequestMaxAge)
	if payment.ExpiresAt == nil {
		return derived
	}
	stored := payment.ExpiresAt.UTC()
	if stored.Before(derived) {
		return stored
	}
	return derived
}

func AlternativePaymentRequestExpired(payment AlternativePayment, now time.Time) bool {
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	return !AlternativePaymentRequestExpiresAt(payment).After(now)
}

var ErrAlternativePaymentRequestConflict = errors.New("alternative payment idempotency key payload conflict")

const maxAlternativePaymentResolutionReasonLength = 500

type PendingAlternativePaymentCreatedHook func(tx *gorm.DB, payment *AlternativePayment, bill *Bill) error

func expirePendingAlternativePaymentRequestsTx(tx *gorm.DB, billID uint, now time.Time) error {
	// Focused SQLite lifecycle tests intentionally own only bills/orders. The
	// production PostgreSQL schema always has this table; fail closed there.
	if tx.Dialector.Name() != "postgres" && !tx.Migrator().HasTable(&AlternativePayment{}) {
		return nil
	}
	return tx.Model(&AlternativePayment{}).
		Where("bill_id = ? AND status = ?", billID, AltPaymentStatusPending).
		Updates(map[string]any{"status": AltPaymentStatusExpired, "updated_at": now}).Error
}

// CryptoQuoteExceedsRemaining is the quote-issuance check for #529.
// excludeHeldCents is the caller's already-held split share, which
// CreatePendingRequest does not reserve a second time.
func CryptoQuoteExceedsRemaining(bill Bill, amountCents, excludeHeldCents int64, now time.Time) (bool, error) {
	if excludeHeldCents < 0 {
		excludeHeldCents = 0
	}
	available, err := payableRemainingAfterReservations(bill, excludeHeldCents, now)
	if err != nil {
		return false, err
	}
	return amountCents > available, nil
}

func openBillRemainingCents(bill Bill) int64 {
	remaining := bill.TotalAmount - bill.PaidAmount
	if remaining < 0 {
		return 0
	}
	return remaining
}

func payableRemainingAfterReservations(bill Bill, excludeHeldCents int64, now time.Time) (int64, error) {
	remaining := openBillRemainingCents(bill)
	if bill.ID == 0 {
		return remaining + excludeHeldCents, nil
	}
	db := GetDB()
	if db == nil {
		return 0, errors.New("database is unavailable")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	if db.Dialector.Name() != "postgres" && !db.Migrator().HasTable(&AlternativePayment{}) {
		return remaining + excludeHeldCents, nil
	}
	pending, held, err := billReservationCentsTx(db, bill.ID, now)
	if err != nil {
		return 0, err
	}
	return remaining - pending - held + excludeHeldCents, nil
}

func billReservationCentsTx(tx *gorm.DB, billID uint, now time.Time) (pending, held int64, err error) {
	if tx == nil {
		return 0, 0, errors.New("database is unavailable")
	}
	if tx.Dialector.Name() != "postgres" && !tx.Migrator().HasTable(&AlternativePayment{}) {
		return 0, 0, nil
	}
	pending, err = activeOrdinaryPendingAlternativePaymentCentsTx(tx, billID, now)
	if err != nil {
		return 0, 0, err
	}
	if tx.Dialector.Name() != "postgres" && !tx.Migrator().HasTable(&BillSplitShare{}) {
		return pending, 0, nil
	}
	held, err = activeHeldSplitCentsTx(tx, billID, now)
	if err != nil {
		return 0, 0, err
	}
	return pending, held, nil
}

func activeOrdinaryPendingAlternativePaymentCentsTx(tx *gorm.DB, billID uint, now time.Time) (int64, error) {
	var pending int64
	err := tx.Model(&AlternativePayment{}).
		Select("COALESCE(SUM(amount), 0)").
		Where("bill_id = ? AND status = ? AND created_at > ? AND (expires_at IS NULL OR expires_at > ?) AND participant_addr NOT LIKE ?",
			billID, AltPaymentStatusPending, now.Add(-AlternativePaymentRequestMaxAge), now, billSplitAlternativePaymentParticipantAddrPrefix+"%").
		Scan(&pending).Error
	return pending, err
}

// CreatePendingRequest serializes guest payment-request reservations on the
// bill row. An exact idempotent retry returns the original durable row; a key
// reused for a different canonical payload conflicts.
func (s *AlternativePaymentService) CreatePendingRequest(payment *AlternativePayment, now time.Time, splitGuestSessionID string, splitHoldTTL time.Duration, afterCreate PendingAlternativePaymentCreatedHook) (*AlternativePayment, bool, error) {
	if s == nil || s.repo == nil || s.repo.db == nil || payment == nil {
		return nil, false, errors.New("alternative payment service is unavailable")
	}
	if payment.BillID == 0 || payment.Amount <= 0 || payment.IdempotencyKeyHash == "" || payment.PayloadHash == "" {
		return nil, false, ErrInvalidPaymentAmount
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	var created AlternativePayment
	var replayed bool
	err := s.repo.db.Transaction(func(tx *gorm.DB) error {
		var bill Bill
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&bill, payment.BillID).Error; err != nil {
			return err
		}
		if bill.Status == BillStatusPaid || bill.Status == BillStatusClosed || bill.Status == BillStatusVoided {
			return ErrBillNotPayable
		}

		var existing AlternativePayment
		err := tx.Where("bill_id = ? AND idempotency_key_hash = ?", payment.BillID, payment.IdempotencyKeyHash).
			First(&existing).Error
		switch {
		case err == nil:
			if existing.PayloadHash != payment.PayloadHash {
				return ErrAlternativePaymentRequestConflict
			}
			created = existing
			replayed = true
			return nil
		case !errors.Is(err, gorm.ErrRecordNotFound):
			return err
		}

		if err := tx.Model(&AlternativePayment{}).
			Where(
				"bill_id = ? AND status = ? AND ((expires_at IS NOT NULL AND expires_at <= ?) OR created_at <= ?)",
				payment.BillID, AltPaymentStatusPending, now, now.Add(-AlternativePaymentRequestMaxAge),
			).
			Updates(map[string]any{"status": AltPaymentStatusExpired, "updated_at": now}).Error; err != nil {
			return err
		}
		if _, err := releaseExpiredBillSplitSharesTx(tx, now, &payment.BillID); err != nil {
			return err
		}

		remaining := openBillRemainingCents(bill)
		if remaining <= 0 {
			return ErrPaymentExceedsRemaining
		}

		ordinaryPending, heldSplit, err := billReservationCentsTx(tx, payment.BillID, now)
		if err != nil {
			return err
		}

		if shareID, isSplit := splitShareIDFromAlternativePayment(*payment); isSplit {
			var share BillSplitShare
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&share, shareID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrSplitShareNotFound
				}
				return err
			}
			if share.BillID != payment.BillID {
				return ErrAlternativePaymentBillMismatch
			}
			if strings.TrimSpace(splitGuestSessionID) == "" || share.GuestSessionID != strings.TrimSpace(splitGuestSessionID) {
				return ErrSplitGuestMismatch
			}
			if share.Status != BillSplitShareStatusHeld || (share.HoldExpiresAt != nil && !share.HoldExpiresAt.After(now)) {
				return ErrSplitHoldExpired
			}
			if share.AmountCents != payment.Amount {
				return ErrSplitAmountUnavailable
			}
			// The share already reserves its amount; the request references that
			// reservation and must not reserve it a second time.
			if ordinaryPending+heldSplit > remaining {
				return ErrPaymentExceedsRemaining
			}
			if splitHoldTTL <= 0 {
				splitHoldTTL = defaultBillSplitHoldTTL
			}
			holdExpiresAt := now.Add(splitHoldTTL)
			updates := map[string]any{
				"tip_cents":  payment.TipAmountCents,
				"updated_at": now,
			}
			if share.HoldExpiresAt != nil && share.HoldExpiresAt.Before(holdExpiresAt) {
				updates["hold_expires_at"] = holdExpiresAt
			}
			if err := tx.Model(&BillSplitShare{}).Where("id = ? AND status = ?", share.ID, BillSplitShareStatusHeld).Updates(updates).Error; err != nil {
				return err
			}
		} else if payment.Amount > remaining-ordinaryPending-heldSplit {
			// Pending guest requests already reserve the whole open balance:
			// this is a repeat "pay at the counter", not a stale amount.
			if ordinaryPending > 0 && remaining-ordinaryPending-heldSplit <= 0 {
				return ErrPaymentRequestAlreadyPending
			}
			return ErrPaymentExceedsRemaining
		}

		payment.IdempotencyKey = ""
		payment.Status = AltPaymentStatusPending
		payment.CreatedAt = now
		payment.UpdatedAt = now
		if payment.ExpiresAt == nil {
			expiresAt := now.Add(AlternativePaymentRequestTTL)
			payment.ExpiresAt = &expiresAt
		}
		if err := tx.Create(payment).Error; err != nil {
			return err
		}
		if afterCreate != nil {
			if err := afterCreate(tx, payment, &bill); err != nil {
				return err
			}
		}
		created = *payment
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	created.IdempotencyKey = ""
	created.IdempotencyKeyHash = strings.TrimSpace(created.IdempotencyKeyHash)
	return &created, replayed, nil
}

func CancelPendingAlternativePayment(billID, requestID uint, resolvedBy, reason string, now time.Time) (*AlternativePayment, error) {
	return resolvePendingAlternativePayment(billID, requestID, resolvedBy, reason, AltPaymentStatusCancelled, now)
}

func RejectPendingAlternativePayment(billID, requestID uint, resolvedBy, reason string, now time.Time) (*AlternativePayment, error) {
	return resolvePendingAlternativePayment(billID, requestID, resolvedBy, reason, AltPaymentStatusRejected, now)
}

func resolvePendingAlternativePayment(billID, requestID uint, resolvedBy, reason string, targetStatus AlternativePaymentStatus, now time.Time) (*AlternativePayment, error) {
	if billID == 0 || requestID == 0 {
		return nil, ErrAlternativePaymentBillMismatch
	}
	if targetStatus != AltPaymentStatusCancelled && targetStatus != AltPaymentStatusRejected {
		return nil, ErrAlternativePaymentRequestNotPending
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	resolvedBy = strings.TrimSpace(resolvedBy)
	if resolvedBy == "" {
		resolvedBy = "system"
	}
	reason = strings.TrimSpace(reason)
	reasonRunes := []rune(reason)
	if len(reasonRunes) > maxAlternativePaymentResolutionReasonLength {
		reason = string(reasonRunes[:maxAlternativePaymentResolutionReasonLength])
	}

	var payment AlternativePayment
	var terminalErr error
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&payment, requestID).Error; err != nil {
			return fmt.Errorf("failed to get alternative payment request: %w", err)
		}
		if payment.BillID != billID {
			return ErrAlternativePaymentBillMismatch
		}
		if payment.Status == targetStatus {
			return nil
		}
		if payment.Status == AltPaymentStatusExpired {
			terminalErr = ErrAlternativePaymentRequestExpired
			return nil
		}
		if payment.Status != AltPaymentStatusPending {
			return ErrAlternativePaymentRequestNotPending
		}

		if err := releaseBillSplitShareForAlternativePaymentTx(tx, payment, now); err != nil {
			return err
		}
		if payment.ExpiresAt != nil && !payment.ExpiresAt.After(now) {
			payment.Status = AltPaymentStatusExpired
			payment.UpdatedAt = now
			if err := tx.Model(&AlternativePayment{}).
				Where("id = ? AND status = ?", payment.ID, AltPaymentStatusPending).
				Updates(map[string]any{"status": payment.Status, "updated_at": now}).Error; err != nil {
				return fmt.Errorf("expire alternative payment request: %w", err)
			}
			terminalErr = ErrAlternativePaymentRequestExpired
			return nil
		}

		payment.Status = targetStatus
		payment.ResolvedBy = resolvedBy
		payment.ResolutionReason = reason
		payment.ResolvedAt = &now
		payment.UpdatedAt = now
		result := tx.Model(&AlternativePayment{}).
			Where("id = ? AND status = ?", payment.ID, AltPaymentStatusPending).
			Updates(map[string]any{
				"status":            payment.Status,
				"resolved_by":       payment.ResolvedBy,
				"resolution_reason": payment.ResolutionReason,
				"resolved_at":       payment.ResolvedAt,
				"updated_at":        payment.UpdatedAt,
			})
		if result.Error != nil {
			return fmt.Errorf("resolve alternative payment request: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return ErrAlternativePaymentRequestNotPending
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if terminalErr != nil {
		return nil, terminalErr
	}
	return &payment, nil
}

// GetActiveBillSplitShareForGuest validates a guest-owned held share without
// extending or otherwise mutating it. RequestAlternativePayment uses this for
// request parsing; the authoritative extension and tip write occur only after
// idempotency and capacity checks inside CreatePendingRequest.
func GetActiveBillSplitShareForGuest(shareID, billID uint, guestSessionID string, now time.Time) (*BillSplitShare, error) {
	if shareID == 0 || billID == 0 {
		return nil, ErrSplitShareNotFound
	}
	guestSessionID = strings.TrimSpace(guestSessionID)
	if guestSessionID == "" {
		return nil, ErrSplitGuestMismatch
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var share BillSplitShare
	if err := db.Select("id", "bill_id", "guest_session_id", "display_name", "amount_cents", "tip_cents", "status", "hold_expires_at").First(&share, shareID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSplitShareNotFound
		}
		return nil, err
	}
	if share.BillID != billID {
		return nil, ErrSplitShareNotFound
	}
	if share.GuestSessionID != guestSessionID {
		return nil, ErrSplitGuestMismatch
	}
	if share.Status != BillSplitShareStatusHeld {
		return nil, ErrSplitShareAlreadyFinal
	}
	if share.HoldExpiresAt != nil && !share.HoldExpiresAt.After(now) {
		return nil, ErrSplitHoldExpired
	}
	return &share, nil
}
