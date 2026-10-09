package cashregister

import (
	"context"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"gorm.io/gorm"
)

const (
	// DemoHouseRailOpenedByLabel marks the auto-opened dinner drawer.
	DemoHouseRailOpenedByLabel = "demo-dinner-drawer"
	DemoHouseRailOpeningNote   = "Demo dinner drawer"
)

// EnsureDemoHouseCashSession returns the open drawer if one exists.
// For kind=demo showrooms that have never been closed by an operator, it
// opens a house cash session so dinner is takeable without inventing
// processor credentials (#769). Real venues and a human close (durable
// actor-id marker) are left alone — cash stays 409 until an operator
// opens Caja. Guest GET must not call this; heal only on operator Current
// and cash-record.
func (s *Service) EnsureDemoHouseCashSession(ctx context.Context, businessID uint) (*database.CashRegisterSession, error) {
	var session *database.CashRegisterSession
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		session, err = ensureDemoHouseCashSessionTx(tx, businessID, s.now())
		return err
	})
	return session, err
}

func ensureDemoHouseCashSessionTx(tx *gorm.DB, businessID uint, now time.Time) (*database.CashRegisterSession, error) {
	open, err := database.FindOpenCashRegisterSessionForBusinessTx(tx, businessID)
	if err != nil {
		return nil, err
	}
	if open != nil {
		return open, nil
	}

	isDemo, err := database.BusinessUsesDemoHouseRailTx(tx, businessID)
	if err != nil || !isDemo {
		return nil, err
	}

	humanClosed, err := database.HasHumanClosedCashRegisterSessionTx(tx, businessID)
	if err != nil {
		return nil, err
	}
	if humanClosed {
		return nil, nil
	}

	lastClosed, err := database.FindLastClosedCashRegisterSessionTx(tx, businessID)
	if err != nil {
		return nil, err
	}

	openingFloatCents := int64(0)
	if lastClosed != nil {
		openingFloatCents = lastClosed.OpeningFloatCents
	} else {
		suggested, hintErr := database.FindLastDeclaredOpeningFloatCentsTx(tx, businessID)
		if hintErr != nil {
			return nil, hintErr
		}
		if suggested != nil {
			openingFloatCents = *suggested
		}
	}

	session := database.CashRegisterSession{
		BusinessID:        businessID,
		Status:            database.CashRegisterSessionStatusOpen,
		OpeningFloatCents: openingFloatCents,
		OpeningNote:       DemoHouseRailOpeningNote,
		OpenedByLabel:     DemoHouseRailOpenedByLabel,
		OpenedAt:          now,
	}
	if err := tx.Create(&session).Error; err != nil {
		if isOpenSessionUniqueConstraintError(err) {
			return database.FindOpenCashRegisterSessionForBusinessTx(tx, businessID)
		}
		return nil, err
	}
	return &session, nil
}
