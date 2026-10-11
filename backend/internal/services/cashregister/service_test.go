package cashregister_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services/cashregister"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestServiceOpenSessionRejectsSecondOpenSession(t *testing.T) {
	db := openCashRegisterServiceTestDB(t)
	svc := cashregister.NewServiceWithClock(db, fixedCashRegisterNow)
	business := createCashRegisterBusiness(t, db, "biz-open-once")

	_, err := svc.OpenSession(context.Background(), cashregister.OpenSessionInput{
		BusinessID:        business.ID,
		OpeningFloatCents: 10000,
		Actor:             database.CashRegisterActor{Label: "manager"},
	})
	require.NoError(t, err)

	_, err = svc.OpenSession(context.Background(), cashregister.OpenSessionInput{
		BusinessID:        business.ID,
		OpeningFloatCents: 20000,
		Actor:             database.CashRegisterActor{Label: "manager"},
	})
	require.ErrorIs(t, err, cashregister.ErrSessionAlreadyOpen)
}

func TestServiceOpenSessionPersistsActorAndOpeningFloat(t *testing.T) {
	db := openCashRegisterServiceTestDB(t)
	openedAt := time.Date(2026, 6, 27, 14, 15, 16, 0, time.UTC)
	svc := cashregister.NewServiceWithClock(db, func() time.Time { return openedAt })
	business := createCashRegisterBusiness(t, db, "biz-open-actor")
	user := createCashRegisterUser(t, db, "owner@example.test")
	staff := createCashRegisterStaff(t, db, business.ID, "cashier@example.test")

	session, err := svc.OpenSession(context.Background(), cashregister.OpenSessionInput{
		BusinessID:        business.ID,
		OpeningFloatCents: 12345,
		OpeningNote:       "morning float",
		Actor:             database.CashRegisterActor{UserID: &user.ID, StaffID: &staff.ID, Label: "cashier"},
	})
	require.NoError(t, err)

	require.Equal(t, database.CashRegisterSessionStatusOpen, session.Status)
	require.Equal(t, int64(12345), session.OpeningFloatCents)
	require.Equal(t, "morning float", session.OpeningNote)
	require.Equal(t, user.ID, *session.OpenedByUserID)
	require.Equal(t, staff.ID, *session.OpenedByStaffID)
	require.Equal(t, "cashier", session.OpenedByLabel)
	require.Equal(t, openedAt, session.OpenedAt)
}

func TestServiceOpenSessionDoesNotMapNonUniqueCreateFailureToAlreadyOpen(t *testing.T) {
	db := openCashRegisterServiceTestDB(t)
	svc := cashregister.NewServiceWithClock(db, fixedCashRegisterNow)
	createErr := errors.New("forced create failure")
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("cashregister:test_create_error", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "CashRegisterSession" {
			tx.AddError(createErr)
		}
	}))

	_, err := svc.OpenSession(context.Background(), cashregister.OpenSessionInput{
		BusinessID:        createCashRegisterBusiness(t, db, "biz-open-create-error").ID,
		OpeningFloatCents: 10000,
		Actor:             database.CashRegisterActor{Label: "manager"},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, createErr)
	require.False(t, errors.Is(err, cashregister.ErrSessionAlreadyOpen))
}

func TestServiceCreateManualMovementRequiresManualTypePositiveAmountAndReason(t *testing.T) {
	db := openCashRegisterServiceTestDB(t)
	svc := cashregister.NewServiceWithClock(db, fixedCashRegisterNow)
	business := createCashRegisterBusiness(t, db, "biz-manual-validation")
	session := createOpenCashRegisterSession(t, db, business.ID, 10000, fixedCashRegisterNow())

	_, _, err := svc.CreateManualMovement(context.Background(), cashregister.ManualMovementInput{
		BusinessID:   business.ID,
		SessionID:    session.ID,
		MovementType: database.CashRegisterMovementTypeCashSale,
		AmountCents:  1000,
		Reason:       "cash sale",
		Actor:        database.CashRegisterActor{Label: "manager"},
	})
	require.ErrorIs(t, err, cashregister.ErrInvalidMovementType)

	_, _, err = svc.CreateManualMovement(context.Background(), cashregister.ManualMovementInput{
		BusinessID:   business.ID,
		SessionID:    session.ID,
		MovementType: database.CashRegisterMovementTypeCashIn,
		AmountCents:  0,
		Reason:       "petty cash",
		Actor:        database.CashRegisterActor{Label: "manager"},
	})
	require.ErrorIs(t, err, cashregister.ErrInvalidAmount)

	_, _, err = svc.CreateManualMovement(context.Background(), cashregister.ManualMovementInput{
		BusinessID:   business.ID,
		SessionID:    session.ID,
		MovementType: database.CashRegisterMovementTypeCashOut,
		AmountCents:  500,
		Reason:       "   ",
		Actor:        database.CashRegisterActor{Label: "manager"},
	})
	require.ErrorIs(t, err, cashregister.ErrMissingReason)
}

func TestServiceCreateManualMovementUpdatesOpenSessionTotals(t *testing.T) {
	db := openCashRegisterServiceTestDB(t)
	occurredAt := time.Date(2026, 6, 27, 16, 0, 0, 0, time.UTC)
	svc := cashregister.NewServiceWithClock(db, func() time.Time { return occurredAt })
	business := createCashRegisterBusiness(t, db, "biz-manual-totals")
	session := createOpenCashRegisterSession(t, db, business.ID, 10000, fixedCashRegisterNow())

	movement, updated, err := svc.CreateManualMovement(context.Background(), cashregister.ManualMovementInput{
		BusinessID:   business.ID,
		SessionID:    session.ID,
		MovementType: database.CashRegisterMovementTypeCashIn,
		AmountCents:  2500,
		Reason:       " cash drawer top-up ",
		Note:         "safe transfer",
		Actor:        database.CashRegisterActor{Label: "manager"},
	})
	require.NoError(t, err)
	require.Equal(t, database.CashRegisterMovementTypeCashIn, movement.MovementType)
	require.Equal(t, int64(2500), movement.AmountCents)
	require.Equal(t, "cash drawer top-up", movement.Reason)
	require.Equal(t, "safe transfer", movement.Note)
	require.Equal(t, "manager", movement.ActorLabel)
	require.Equal(t, occurredAt, movement.OccurredAt)
	require.Equal(t, int64(2500), updated.CashInCents)

	_, updated, err = svc.CreateManualMovement(context.Background(), cashregister.ManualMovementInput{
		BusinessID:   business.ID,
		SessionID:    session.ID,
		MovementType: database.CashRegisterMovementTypeCashOut,
		AmountCents:  700,
		Reason:       "bank drop",
		Actor:        database.CashRegisterActor{Label: "manager"},
	})
	require.NoError(t, err)
	require.Equal(t, int64(2500), updated.CashInCents)
	require.Equal(t, int64(700), updated.CashOutCents)
}

func TestServiceCloseSessionComputesBlindCloseSnapshotAndVariance(t *testing.T) {
	db := openCashRegisterServiceTestDB(t)
	closedAt := time.Date(2026, 6, 27, 22, 0, 0, 0, time.UTC)
	svc := cashregister.NewServiceWithClock(db, func() time.Time { return closedAt })
	business := createCashRegisterBusiness(t, db, "biz-close-snapshot")
	staff := createCashRegisterStaff(t, db, business.ID, "closer@example.test")
	session := createOpenCashRegisterSession(t, db, business.ID, 10000, fixedCashRegisterNow())
	createCashRegisterMovement(t, db, business.ID, session.ID, database.CashRegisterMovementTypeCashSale, 20000)
	createCashRegisterMovement(t, db, business.ID, session.ID, database.CashRegisterMovementTypeCashRefund, 1500)
	createCashRegisterMovement(t, db, business.ID, session.ID, database.CashRegisterMovementTypeCashIn, 2500)
	createCashRegisterMovement(t, db, business.ID, session.ID, database.CashRegisterMovementTypeCashOut, 1000)

	closed, err := svc.CloseSession(context.Background(), cashregister.CloseSessionInput{
		BusinessID:       business.ID,
		SessionID:        session.ID,
		CountedCashCents: 30500,
		ClosingNote:      "drawer counted",
		Actor:            database.CashRegisterActor{StaffID: &staff.ID, Label: "closer"},
	})
	require.NoError(t, err)

	require.Equal(t, database.CashRegisterSessionStatusClosed, closed.Status)
	require.Equal(t, int64(20000), closed.CashSalesCents)
	require.Equal(t, int64(1500), closed.CashRefundsCents)
	require.Equal(t, int64(2500), closed.CashInCents)
	require.Equal(t, int64(1000), closed.CashOutCents)
	require.Equal(t, int64(30000), closed.ExpectedCashCents)
	require.Equal(t, int64(30500), closed.CountedCashCents)
	require.Equal(t, int64(500), closed.VarianceCents)
	require.Equal(t, "drawer counted", closed.ClosingNote)
	require.Equal(t, staff.ID, *closed.ClosedByStaffID)
	require.Equal(t, "closer", closed.ClosedByLabel)
	require.NotNil(t, closed.ClosedAt)
	require.Equal(t, closedAt, *closed.ClosedAt)
}

func TestServiceCloseSessionRejectsDoubleClose(t *testing.T) {
	db := openCashRegisterServiceTestDB(t)
	svc := cashregister.NewServiceWithClock(db, fixedCashRegisterNow)
	business := createCashRegisterBusiness(t, db, "biz-double-close")
	session := createOpenCashRegisterSession(t, db, business.ID, 10000, fixedCashRegisterNow())

	_, err := svc.CloseSession(context.Background(), cashregister.CloseSessionInput{
		BusinessID:       business.ID,
		SessionID:        session.ID,
		CountedCashCents: 10000,
		Actor:            database.CashRegisterActor{Label: "manager"},
	})
	require.NoError(t, err)

	_, err = svc.CloseSession(context.Background(), cashregister.CloseSessionInput{
		BusinessID:       business.ID,
		SessionID:        session.ID,
		CountedCashCents: 10000,
		Actor:            database.CashRegisterActor{Label: "manager"},
	})
	require.ErrorIs(t, err, cashregister.ErrSessionClosed)
}

func TestServiceClosedSessionSnapshotDoesNotChangeWhenMovementsAreEdited(t *testing.T) {
	db := openCashRegisterServiceTestDB(t)
	svc := cashregister.NewServiceWithClock(db, fixedCashRegisterNow)
	business := createCashRegisterBusiness(t, db, "biz-closed-stable")
	session := createOpenCashRegisterSession(t, db, business.ID, 10000, fixedCashRegisterNow())
	movement := createCashRegisterMovement(t, db, business.ID, session.ID, database.CashRegisterMovementTypeCashSale, 20000)

	closed, err := svc.CloseSession(context.Background(), cashregister.CloseSessionInput{
		BusinessID:       business.ID,
		SessionID:        session.ID,
		CountedCashCents: 30000,
		Actor:            database.CashRegisterActor{Label: "manager"},
	})
	require.NoError(t, err)
	require.Equal(t, int64(30000), closed.ExpectedCashCents)

	require.NoError(t, db.Model(&database.CashRegisterMovement{}).Where("id = ?", movement.ID).Update("amount_cents", 90000).Error)

	reloaded, err := svc.GetSession(context.Background(), business.ID, session.ID)
	require.NoError(t, err)
	require.Equal(t, int64(30000), reloaded.ExpectedCashCents)
	require.Equal(t, int64(30000), reloaded.CountedCashCents)
	require.Equal(t, int64(0), reloaded.VarianceCents)
}

func TestServiceListSessionsOrdersNewestFirstAndScopesBusiness(t *testing.T) {
	db := openCashRegisterServiceTestDB(t)
	svc := cashregister.NewService(db)
	business := createCashRegisterBusiness(t, db, "biz-list")
	otherBusiness := createCashRegisterBusiness(t, db, "biz-list-other")
	first := createClosedCashRegisterSession(t, db, business.ID, time.Date(2026, 6, 27, 8, 0, 0, 0, time.UTC))
	second := createClosedCashRegisterSession(t, db, business.ID, time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC))
	createClosedCashRegisterSession(t, db, otherBusiness.ID, time.Date(2026, 6, 27, 18, 0, 0, 0, time.UTC))

	sessions, total, err := svc.ListSessions(context.Background(), business.ID, 0, 0)
	require.NoError(t, err)

	require.Equal(t, int64(2), total)
	require.Len(t, sessions, 2)
	require.Equal(t, second.ID, sessions[0].ID)
	require.Equal(t, first.ID, sessions[1].ID)
	require.Equal(t, business.ID, sessions[0].BusinessID)
	require.Equal(t, business.ID, sessions[1].BusinessID)
}

// A shift opened earlier but closed later must still sort above an earlier close
// (operators read History as a close ledger, not an open ledger).
func TestServiceListSessionsOrdersByClosedAtNotOpenedAt(t *testing.T) {
	db := openCashRegisterServiceTestDB(t)
	svc := cashregister.NewService(db)
	business := createCashRegisterBusiness(t, db, "biz-list-closed-at")

	openedEarly := time.Date(2026, 6, 27, 8, 0, 0, 0, time.UTC)
	closedLate := openedEarly.Add(14 * time.Hour) // 22:00
	longShift := database.CashRegisterSession{
		BusinessID:        business.ID,
		Status:            database.CashRegisterSessionStatusClosed,
		OpeningFloatCents: 10000,
		ExpectedCashCents: 10000,
		CountedCashCents:  10000,
		OpenedByLabel:     "manager",
		OpenedAt:          openedEarly,
		ClosedByLabel:     "manager",
		ClosedAt:          &closedLate,
	}
	require.NoError(t, db.Create(&longShift).Error)

	openedLater := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	closedEarlier := openedLater.Add(2 * time.Hour) // 14:00
	shortShift := database.CashRegisterSession{
		BusinessID:        business.ID,
		Status:            database.CashRegisterSessionStatusClosed,
		OpeningFloatCents: 10000,
		ExpectedCashCents: 10000,
		CountedCashCents:  10000,
		OpenedByLabel:     "manager",
		OpenedAt:          openedLater,
		ClosedByLabel:     "manager",
		ClosedAt:          &closedEarlier,
	}
	require.NoError(t, db.Create(&shortShift).Error)

	sessions, total, err := svc.ListSessions(context.Background(), business.ID, 0, 0)
	require.NoError(t, err)
	require.Equal(t, int64(2), total)
	require.Len(t, sessions, 2)
	require.Equal(t, longShift.ID, sessions[0].ID, "newest closed_at must lead history")
	require.Equal(t, shortShift.ID, sessions[1].ID)
}

// L2-29: "Turnos cerrados" must not include the currently-open session.
func TestServiceListSessionsExcludesOpenSession(t *testing.T) {
	db := openCashRegisterServiceTestDB(t)
	svc := cashregister.NewService(db)
	business := createCashRegisterBusiness(t, db, "biz-list-closed-only")
	closed := createClosedCashRegisterSession(t, db, business.ID, time.Date(2026, 6, 27, 8, 0, 0, 0, time.UTC))
	_ = createOpenCashRegisterSession(t, db, business.ID, 10000, time.Date(2026, 6, 28, 9, 0, 0, 0, time.UTC))

	sessions, total, err := svc.ListSessions(context.Background(), business.ID, 0, 0)
	require.NoError(t, err)
	require.Equal(t, int64(1), total, "open session must not count toward Turnos cerrados")
	require.Len(t, sessions, 1)
	require.Equal(t, closed.ID, sessions[0].ID)
	require.Equal(t, database.CashRegisterSessionStatusClosed, sessions[0].Status)
}

func TestServiceCurrentDoesNotExposeExpectedCashForOpenSession(t *testing.T) {
	db := openCashRegisterServiceTestDB(t)
	svc := cashregister.NewServiceWithClock(db, fixedCashRegisterNow)
	business := createCashRegisterBusiness(t, db, "biz-current")
	session, err := svc.OpenSession(context.Background(), cashregister.OpenSessionInput{
		BusinessID:        business.ID,
		OpeningFloatCents: 10000,
		Actor:             database.CashRegisterActor{Label: "manager"},
	})
	require.NoError(t, err)
	createCashRegisterMovement(t, db, business.ID, session.ID, database.CashRegisterMovementTypeCashSale, 2500)

	snapshot, err := svc.Current(context.Background(), business.ID)
	require.NoError(t, err)

	require.Equal(t, int64(0), snapshot.UnassignedCount)
	require.Equal(t, int64(0), snapshot.UnassignedTotalCents)
	require.Nil(t, snapshot.SuggestedOpeningFloatCents)
	require.NotNil(t, snapshot.Session)
	require.Equal(t, database.CashRegisterSessionStatusOpen, snapshot.Session.Status)
	require.Equal(t, int64(0), snapshot.Session.ExpectedCashCents)
	require.Equal(t, int64(0), snapshot.Session.CountedCashCents)
	require.Equal(t, int64(0), snapshot.Session.VarianceCents)
}

// #652: next shift inherits the last declared starting bank, never the counted
// close that already includes a shortage (session #254: counted 1473.07, short 1.83).
func TestServiceCurrentSuggestsLastDeclaredOpeningFloatNotCountedShortage(t *testing.T) {
	db := openCashRegisterServiceTestDB(t)
	svc := cashregister.NewService(db)
	business := createCashRegisterBusiness(t, db, "biz-suggested-float")
	other := createCashRegisterBusiness(t, db, "biz-suggested-float-other")

	olderClosedAt := time.Date(2026, 6, 26, 20, 0, 0, 0, time.UTC)
	older := database.CashRegisterSession{
		BusinessID:        business.ID,
		Status:            database.CashRegisterSessionStatusClosed,
		OpeningFloatCents: 90000,
		ExpectedCashCents: 95000,
		CountedCashCents:  95000,
		OpenedByLabel:     "manager",
		OpenedAt:          olderClosedAt.Add(-8 * time.Hour),
		ClosedByLabel:     "manager",
		ClosedAt:          &olderClosedAt,
	}
	require.NoError(t, db.Create(&older).Error)

	shortageClosedAt := time.Date(2026, 6, 27, 20, 0, 0, 0, time.UTC)
	shortage := database.CashRegisterSession{
		BusinessID:        business.ID,
		Status:            database.CashRegisterSessionStatusClosed,
		OpeningFloatCents: 20000,
		ExpectedCashCents: 147490,
		CountedCashCents:  147307,
		VarianceCents:     -183,
		OpenedByLabel:     "manager",
		OpenedAt:          shortageClosedAt.Add(-8 * time.Hour),
		ClosedByLabel:     "manager",
		ClosedAt:          &shortageClosedAt,
	}
	require.NoError(t, db.Create(&shortage).Error)

	otherClosedAt := time.Date(2026, 6, 28, 20, 0, 0, 0, time.UTC)
	otherSession := database.CashRegisterSession{
		BusinessID:        other.ID,
		Status:            database.CashRegisterSessionStatusClosed,
		OpeningFloatCents: 50000,
		ExpectedCashCents: 50000,
		CountedCashCents:  50000,
		OpenedByLabel:     "manager",
		OpenedAt:          otherClosedAt.Add(-8 * time.Hour),
		ClosedByLabel:     "manager",
		ClosedAt:          &otherClosedAt,
	}
	require.NoError(t, db.Create(&otherSession).Error)

	snapshot, err := svc.Current(context.Background(), business.ID)
	require.NoError(t, err)
	require.Nil(t, snapshot.Session)
	require.NotNil(t, snapshot.SuggestedOpeningFloatCents)
	require.Equal(t, int64(20000), *snapshot.SuggestedOpeningFloatCents)
}

func TestServiceCurrentOmitsSuggestedFloatWhenLastCloseLookupFails(t *testing.T) {
	db := openCashRegisterServiceTestDB(t)
	svc := cashregister.NewService(db)
	business := createCashRegisterBusiness(t, db, "biz-hint-fail-open")
	_ = createClosedCashRegisterSession(t, db, business.ID, time.Date(2026, 6, 27, 8, 0, 0, 0, time.UTC))
	open := createOpenCashRegisterSession(t, db, business.ID, 12500, time.Date(2026, 6, 28, 9, 0, 0, 0, time.UTC))

	hintErr := errors.New("forced last-declared-float lookup failure")
	failLastDeclaredOpeningFloatLookup(t, db, hintErr)

	snapshot, err := svc.Current(context.Background(), business.ID)
	require.NoError(t, err)
	require.NotNil(t, snapshot.Session)
	require.Equal(t, open.ID, snapshot.Session.ID)
	require.Nil(t, snapshot.SuggestedOpeningFloatCents)
}

func TestServiceUnassignedCashSummarizesConfirmedCashWithoutSessionMovement(t *testing.T) {
	db := openCashRegisterServiceTestDB(t)
	svc := cashregister.NewService(db)
	business := createCashRegisterBusiness(t, db, "biz-unassigned")
	otherBusiness := createCashRegisterBusiness(t, db, "biz-unassigned-other")
	bill := createCashRegisterBill(t, db, business.ID, "B-1")
	otherBill := createCashRegisterBill(t, db, otherBusiness.ID, "B-2")
	session := createOpenCashRegisterSession(t, db, business.ID, 0, fixedCashRegisterNow())

	unassigned := createAlternativePayment(t, db, bill.ID, database.PaymentMethodCash, database.AltPaymentStatusConfirmed, 1200)
	secondUnassigned := createAlternativePayment(t, db, bill.ID, database.PaymentMethodCash, database.AltPaymentStatusConfirmed, 800)
	assigned := createAlternativePayment(t, db, bill.ID, database.PaymentMethodCash, database.AltPaymentStatusConfirmed, 3000)
	card := createAlternativePayment(t, db, bill.ID, database.PaymentMethodCard, database.AltPaymentStatusConfirmed, 4000)
	pending := createAlternativePayment(t, db, bill.ID, database.PaymentMethodCash, database.AltPaymentStatusPending, 5000)
	other := createAlternativePayment(t, db, otherBill.ID, database.PaymentMethodCash, database.AltPaymentStatusConfirmed, 6000)
	_ = unassigned
	_ = secondUnassigned
	_ = card
	_ = pending
	_ = other
	createCashRegisterMovementForPayment(t, db, business.ID, session.ID, assigned.ID, database.CashRegisterMovementTypeCashSale, assigned.Amount)

	count, total, err := svc.UnassignedCash(context.Background(), business.ID)
	require.NoError(t, err)

	require.Equal(t, int64(2), count)
	require.Equal(t, int64(2000), total)
}

func openCashRegisterServiceTestDB(t testing.TB) *gorm.DB {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	cfg := &gorm.Config{}
	if _, isBench := t.(*testing.B); isBench {
		cfg.Logger = gormlogger.Default.LogMode(gormlogger.Silent)
	}
	db, err := gorm.Open(sqlite.Open("file:"+dsnName+"?mode=memory&cache=shared"), cfg)
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	require.NoError(t, db.AutoMigrate(
		&database.User{},
		&database.Business{},
		&database.Staff{},
		&database.Bill{},
		&database.BillSplitShare{},
		&database.AlternativePayment{},
		&database.CashRegisterSession{},
		&database.CashRegisterMovement{},
	))
	return db
}

func fixedCashRegisterNow() time.Time {
	return time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
}

func createCashRegisterUser(t *testing.T, db *gorm.DB, email string) database.User {
	t.Helper()

	user := database.User{Email: email, Name: email}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func createCashRegisterBusiness(t testing.TB, db *gorm.DB, businessID string) database.Business {
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

func createCashRegisterStaff(t *testing.T, db *gorm.DB, businessID uint, email string) database.Staff {
	t.Helper()

	staff := database.Staff{
		BusinessID: businessID,
		Email:      email,
		Name:       email,
		Role:       database.StaffRole("manager"),
		InvitedBy:  "owner",
	}
	require.NoError(t, db.Create(&staff).Error)
	return staff
}

func createOpenCashRegisterSession(t *testing.T, db *gorm.DB, businessID uint, openingFloatCents int64, openedAt time.Time) database.CashRegisterSession {
	t.Helper()

	session := database.CashRegisterSession{
		BusinessID:        businessID,
		Status:            database.CashRegisterSessionStatusOpen,
		OpeningFloatCents: openingFloatCents,
		OpenedByLabel:     "manager",
		OpenedAt:          openedAt,
	}
	require.NoError(t, db.Create(&session).Error)
	return session
}

func createClosedCashRegisterSession(t testing.TB, db *gorm.DB, businessID uint, openedAt time.Time) database.CashRegisterSession {
	t.Helper()

	closedAt := openedAt.Add(8 * time.Hour)
	session := database.CashRegisterSession{
		BusinessID:        businessID,
		Status:            database.CashRegisterSessionStatusClosed,
		OpeningFloatCents: 10000,
		ExpectedCashCents: 10000,
		CountedCashCents:  10000,
		OpenedByLabel:     "manager",
		OpenedAt:          openedAt,
		ClosedByLabel:     "manager",
		ClosedAt:          &closedAt,
	}
	require.NoError(t, db.Create(&session).Error)
	return session
}

func createCashRegisterMovement(t *testing.T, db *gorm.DB, businessID uint, sessionID uint, movementType database.CashRegisterMovementType, amountCents int64) database.CashRegisterMovement {
	t.Helper()

	movement := database.CashRegisterMovement{
		BusinessID:   businessID,
		SessionID:    sessionID,
		MovementType: movementType,
		AmountCents:  amountCents,
		Reason:       "test",
		ActorLabel:   "manager",
		OccurredAt:   fixedCashRegisterNow(),
	}
	require.NoError(t, db.Create(&movement).Error)
	return movement
}

func createCashRegisterMovementForPayment(t *testing.T, db *gorm.DB, businessID uint, sessionID uint, paymentID uint, movementType database.CashRegisterMovementType, amountCents int64) database.CashRegisterMovement {
	t.Helper()

	movement := createCashRegisterMovement(t, db, businessID, sessionID, movementType, amountCents)
	require.NoError(t, db.Model(&database.CashRegisterMovement{}).Where("id = ?", movement.ID).Update("alternative_payment_id", paymentID).Error)
	movement.AlternativePaymentID = &paymentID
	return movement
}

func createCashRegisterBill(t *testing.T, db *gorm.DB, businessID uint, billNumber string) database.Bill {
	t.Helper()

	bill := database.Bill{
		BusinessID:          businessID,
		BillNumber:          billNumber,
		SettlementAddr:      "settlement",
		TippingAddr:         "tipping",
		Status:              database.BillStatusPaid,
		TotalAmount:         10000,
		PaidAmount:          10000,
		AlternativePayments: nil,
	}
	require.NoError(t, db.Create(&bill).Error)
	return bill
}

func createAlternativePayment(t *testing.T, db *gorm.DB, billID uint, method database.AlternativePaymentMethod, status database.AlternativePaymentStatus, amountCents int64) database.AlternativePayment {
	t.Helper()

	now := fixedCashRegisterNow()
	payment := database.AlternativePayment{
		BillID:          billID,
		ParticipantAddr: "cashier",
		ParticipantName: "Cashier",
		Amount:          amountCents,
		BillAmountCents: amountCents,
		PaymentMethod:   method,
		Status:          status,
		ConfirmedBy:     "manager",
		ConfirmedAt:     &now,
		IdempotencyKey:  "",
		TipAmountCents:  0,
	}
	require.NoError(t, db.Create(&payment).Error)
	return payment
}

func failLastDeclaredOpeningFloatLookup(t *testing.T, db *gorm.DB, fail error) {
	t.Helper()

	const callback = "caja652:fail_hint"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement == nil {
			return
		}
		for _, sel := range tx.Statement.Selects {
			if strings.Contains(sel, "opening_float_cents") {
				_ = tx.AddError(fail)
				return
			}
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callback) })
}
