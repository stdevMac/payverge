package cashregister

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalidAmount       = errors.New("invalid cash register amount")
	ErrInvalidMovementType = errors.New("invalid cash register movement type")
	ErrMissingReason       = errors.New("cash register reason is required")
	ErrSessionAlreadyOpen  = errors.New("cash register session already open")
	ErrSessionNotFound     = errors.New("cash register session not found")
	ErrSessionClosed       = errors.New("cash register session is closed")
)

type Service struct {
	db  *gorm.DB
	now func() time.Time
}

func NewService(db *gorm.DB) *Service {
	return NewServiceWithClock(db, time.Now)
}

func NewServiceWithClock(db *gorm.DB, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{db: db, now: now}
}

type OpenSessionInput struct {
	BusinessID        uint
	OpeningFloatCents int64
	OpeningNote       string
	Actor             database.CashRegisterActor
}

type ManualMovementInput struct {
	BusinessID   uint
	SessionID    uint
	MovementType database.CashRegisterMovementType
	AmountCents  int64
	Reason       string
	Note         string
	Actor        database.CashRegisterActor
}

type CloseSessionInput struct {
	BusinessID       uint
	SessionID        uint
	CountedCashCents int64
	ClosingNote      string
	Actor            database.CashRegisterActor
}

// CurrentSnapshot is the Caja landing payload: open session (if any),
// unassigned cash, and the next-shift suggested float.
type CurrentSnapshot struct {
	Session                    *database.CashRegisterSession
	UnassignedCount            int64
	UnassignedTotalCents       int64
	SuggestedOpeningFloatCents *int64
}

func (s *Service) Current(ctx context.Context, businessID uint) (CurrentSnapshot, error) {
	var snapshot CurrentSnapshot

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		snapshot.Session, err = ensureDemoHouseCashSessionTx(tx, businessID, s.now())
		if err != nil {
			return err
		}
		snapshot.UnassignedCount, snapshot.UnassignedTotalCents, err = database.UnassignedCashAlternativePaymentSummary(tx, businessID)
		return err
	})
	if err != nil {
		return snapshot, err
	}

	// Optional hint: a last-close lookup must not 500 the drawer (#652 review).
	suggested, hintErr := database.FindLastDeclaredOpeningFloatCentsTx(s.db.WithContext(ctx), businessID)
	if hintErr != nil {
		logger.Logger.Warnf("cash register suggested opening float lookup failed: %v", hintErr)
		return snapshot, nil
	}
	snapshot.SuggestedOpeningFloatCents = suggested
	return snapshot, nil
}

func (s *Service) OpenSession(ctx context.Context, input OpenSessionInput) (*database.CashRegisterSession, error) {
	if input.OpeningFloatCents < 0 {
		return nil, ErrInvalidAmount
	}

	var session database.CashRegisterSession
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		open, err := database.FindOpenCashRegisterSessionForBusinessTx(tx, input.BusinessID)
		if err != nil {
			return err
		}
		if open != nil {
			return ErrSessionAlreadyOpen
		}

		session = database.CashRegisterSession{
			BusinessID:        input.BusinessID,
			Status:            database.CashRegisterSessionStatusOpen,
			OpeningFloatCents: input.OpeningFloatCents,
			OpeningNote:       input.OpeningNote,
			OpenedByUserID:    input.Actor.UserID,
			OpenedByStaffID:   input.Actor.StaffID,
			OpenedByLabel:     input.Actor.Label,
			OpenedAt:          s.now(),
		}
		if err := tx.Create(&session).Error; err != nil {
			if isOpenSessionUniqueConstraintError(err) {
				return ErrSessionAlreadyOpen
			}
			return fmt.Errorf("open cash register session: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (s *Service) CreateManualMovement(ctx context.Context, input ManualMovementInput) (*database.CashRegisterMovement, *database.CashRegisterSession, error) {
	if input.MovementType != database.CashRegisterMovementTypeCashIn && input.MovementType != database.CashRegisterMovementTypeCashOut {
		return nil, nil, ErrInvalidMovementType
	}
	if input.AmountCents <= 0 {
		return nil, nil, ErrInvalidAmount
	}

	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return nil, nil, ErrMissingReason
	}

	var movement database.CashRegisterMovement
	var session database.CashRegisterSession
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND business_id = ?", input.SessionID, input.BusinessID).
			First(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSessionNotFound
			}
			return err
		}
		if session.Status != database.CashRegisterSessionStatusOpen {
			return ErrSessionClosed
		}

		movement = database.CashRegisterMovement{
			BusinessID:   input.BusinessID,
			SessionID:    input.SessionID,
			MovementType: input.MovementType,
			AmountCents:  input.AmountCents,
			Reason:       reason,
			Note:         input.Note,
			ActorUserID:  input.Actor.UserID,
			ActorStaffID: input.Actor.StaffID,
			ActorLabel:   input.Actor.Label,
			OccurredAt:   s.now(),
		}
		if err := tx.Create(&movement).Error; err != nil {
			return err
		}

		totals, err := database.RecalculateCashRegisterSessionTotalsTx(tx, session.ID)
		if err != nil {
			return err
		}
		if err := database.UpdateCashRegisterSessionTotalsTx(tx, session.ID, totals); err != nil {
			return err
		}
		return tx.First(&session, session.ID).Error
	})
	if err != nil {
		return nil, nil, err
	}
	return &movement, &session, nil
}

func (s *Service) CloseSession(ctx context.Context, input CloseSessionInput) (*database.CashRegisterSession, error) {
	if input.CountedCashCents < 0 {
		return nil, ErrInvalidAmount
	}

	var session database.CashRegisterSession
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND business_id = ?", input.SessionID, input.BusinessID).
			First(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSessionNotFound
			}
			return err
		}
		if session.Status != database.CashRegisterSessionStatusOpen {
			return ErrSessionClosed
		}

		totals, err := database.RecalculateCashRegisterSessionTotalsTx(tx, session.ID)
		if err != nil {
			return err
		}

		expected := session.OpeningFloatCents + totals.CashSalesCents - totals.CashRefundsCents + totals.CashInCents - totals.CashOutCents
		closedAt := s.now()
		updates := map[string]interface{}{
			"status":              database.CashRegisterSessionStatusClosed,
			"cash_sales_cents":    totals.CashSalesCents,
			"cash_refunds_cents":  totals.CashRefundsCents,
			"cash_in_cents":       totals.CashInCents,
			"cash_out_cents":      totals.CashOutCents,
			"expected_cash_cents": expected,
			"counted_cash_cents":  input.CountedCashCents,
			"variance_cents":      input.CountedCashCents - expected,
			"closing_note":        input.ClosingNote,
			"closed_by_user_id":   input.Actor.UserID,
			"closed_by_staff_id":  input.Actor.StaffID,
			"closed_by_label":     input.Actor.Label,
			"closed_at":           &closedAt,
		}
		if err := tx.Model(&database.CashRegisterSession{}).Where("id = ?", session.ID).Updates(updates).Error; err != nil {
			return err
		}
		return tx.First(&session, session.ID).Error
	})
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (s *Service) ListSessions(ctx context.Context, businessID uint, limit int, offset int) ([]database.CashRegisterSession, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	// Closed-only: the FE "Turnos cerrados" header and history list both use
	// this endpoint. Including the open session inflated the counter (L2-29).
	query := s.db.WithContext(ctx).Model(&database.CashRegisterSession{}).
		Where("business_id = ? AND status = ?", businessID, database.CashRegisterSessionStatusClosed)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var sessions []database.CashRegisterSession
	// Closed-session history is a close ledger: newest close first. Ordering by
	// opened_at put a long early shift above a later shift that closed more
	// recently (dinner QA #108 / #157). Closed-only filter means closed_at is set.
	err := query.Order("closed_at DESC").Order("id DESC").Limit(limit).Offset(offset).Find(&sessions).Error
	if err != nil {
		return nil, 0, err
	}
	return sessions, total, nil
}

func (s *Service) GetSession(ctx context.Context, businessID uint, sessionID uint) (*database.CashRegisterSession, error) {
	var session database.CashRegisterSession
	err := s.db.WithContext(ctx).
		Preload("Movements", func(db *gorm.DB) *gorm.DB {
			return db.Order("occurred_at DESC").Order("id DESC")
		}).
		Where("id = ? AND business_id = ?", sessionID, businessID).
		First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (s *Service) UnassignedCash(ctx context.Context, businessID uint) (count int64, totalCents int64, err error) {
	return database.UnassignedCashAlternativePaymentSummary(s.db.WithContext(ctx), businessID)
}

// ListUnassignedCash returns the individual unassigned cash tenders (bounded,
// newest first) plus the total matching count, so the dashboard can list what
// makes up the unassigned-cash total instead of showing only a headline number.
func (s *Service) ListUnassignedCash(ctx context.Context, businessID uint, limit, offset int) ([]database.UnassignedCashAlternativePaymentItem, int64, error) {
	return database.ListUnassignedCashAlternativePayments(s.db.WithContext(ctx), businessID, limit, offset)
}

func isOpenSessionUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}

	message := err.Error()
	return strings.Contains(message, "idx_cash_register_sessions_one_open_per_business") ||
		strings.Contains(message, "UNIQUE constraint failed: cash_register_sessions.business_id") ||
		(strings.Contains(message, "duplicate key") && strings.Contains(message, "cash_register_sessions"))
}
