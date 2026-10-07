package demo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// houseRailDrawer mirrors what cashregister.ensureDemoHouseCashSessionTx
// writes when an operator lands on Caja: a live drawer signed by the house
// rail, not by the demo seeder.
func houseRailDrawer(t *testing.T, db *gorm.DB, businessID uint, openedAt time.Time) database.CashRegisterSession {
	t.Helper()
	session := database.CashRegisterSession{
		BusinessID:        businessID,
		Status:            database.CashRegisterSessionStatusOpen,
		OpeningFloatCents: 6000000,
		OpeningNote:       "Demo dinner drawer",
		OpenedByLabel:     "demo-dinner-drawer",
		OpenedAt:          openedAt,
		CreatedAt:         openedAt,
		UpdatedAt:         openedAt,
	}
	require.NoError(t, db.Create(&session).Error)
	return session
}

// #857: live venue 142 held session 360 (generated, 07:19 → 16:00, AR$
// 139.300) overlapping the still-open session 362 (12:57) by three hours.
// Mid-shift "¿cuánto hay en caja?" answered zero on the drawer the floor was
// actually using while the closed twin carried the whole night's cash, so the
// 23:00 count could only fail.
//
// The generated shift must hand over AT the live drawer's opening, and the
// day's cash must follow the drawer that was open when each check was paid.
// Nothing may close the live drawer.
func TestGeneratedShiftHandsOverToLiveDrawer(t *testing.T) {
	db := newDemoServiceTestDB(t)
	day := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	now := day.Add(16 * time.Hour)
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "test-seed", BaselineDays: 30})

	business := database.Business{BusinessId: "caja-handover", Name: "Caja Handover", IsActive: true}
	require.NoError(t, db.Create(&business).Error)

	// Lunch cash lands on the generated shift; dinner cash lands after the
	// house rail opened the live drawer.
	seedConfirmedCashBill(t, db, svc, business.ID, "B-lunch-1", 4500000, database.PaymentMethodCash, day.Add(11*time.Hour))
	seedConfirmedCashBill(t, db, svc, business.ID, "B-dinner-1", 9430000, database.PaymentMethodCash, day.Add(14*time.Hour))

	// Morning tick: the generator owns the whole day.
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return svc.generateCashSessionForDay(context.Background(), tx, business.ID, day)
	}))

	// 12:57 — an operator opens Caja and the house rail arms a live drawer.
	handover := day.Add(12*time.Hour + 57*time.Minute)
	drawer := houseRailDrawer(t, db, business.ID, handover)

	// Two more hourly ticks; neither may push the generated shift past 12:57.
	for tick := 0; tick < 2; tick++ {
		now = now.Add(time.Hour)
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			return svc.generateCashSessionForDay(context.Background(), tx, business.ID, day)
		}))
	}

	var sessions []database.CashRegisterSession
	require.NoError(t, db.Where("business_id = ?", business.ID).Order("opened_at ASC").Find(&sessions).Error)
	require.Len(t, sessions, 2, "the generator must not mint a shift per tick beside the live drawer")

	generated, live := sessions[0], sessions[1]
	require.Equal(t, demoCashActorLabel, generated.OpenedByLabel)
	require.Equal(t, drawer.ID, live.ID)

	require.NotNil(t, generated.ClosedAt)
	require.False(t, generated.ClosedAt.After(handover),
		"the generated shift must hand over at the live drawer's opening, not run past it")
	require.Equal(t, int64(4500000), generated.CashSalesCents,
		"the closed shift keeps only the cash taken before the handover")

	require.Equal(t, database.CashRegisterSessionStatusOpen, live.Status, "the live drawer must never be closed")
	require.Nil(t, live.ClosedAt)
	require.Equal(t, int64(9430000), live.CashSalesCents,
		"the drawer the floor is using must hold the cash taken since it opened")
	require.Zero(t, live.CountedCashCents, "an open drawer has not been counted yet")
	require.Zero(t, live.VarianceCents)

	// The books still balance: nothing was double-counted or dropped.
	require.Equal(t, int64(13930000), generated.CashSalesCents+live.CashSalesCents)
	expected := generated.OpeningFloatCents + generated.CashSalesCents - generated.CashRefundsCents + generated.CashInCents - generated.CashOutCents
	require.Equal(t, expected, generated.ExpectedCashCents)
	require.Equal(t, generated.CountedCashCents-generated.ExpectedCashCents, generated.VarianceCents)

	// Every seeded cash tender is now backed by a movement on its drawer, so
	// Caja stops reporting "efectivo sin asignar" for demo money.
	count, totalCents, err := database.UnassignedCashAlternativePaymentSummary(db, business.ID)
	require.NoError(t, err)
	require.Zero(t, count, "demo cash must be assigned to a drawer")
	require.Zero(t, totalCents)

	var cashSaleMovements int64
	require.NoError(t, db.Model(&database.CashRegisterMovement{}).
		Where("business_id = ? AND movement_type = ?", business.ID, database.CashRegisterMovementTypeCashSale).
		Count(&cashSaleMovements).Error)
	require.Equal(t, int64(2), cashSaleMovements, "re-running the tick must not duplicate movements")
}

// A drawer that was already open before the generated shift would have
// started owns the whole day: no second shift is minted beside it, and the
// day's cash lands on the drawer the floor is holding.
func TestLiveDrawerOpenedBeforeShiftOwnsTheDay(t *testing.T) {
	db := newDemoServiceTestDB(t)
	day := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	now := day.Add(20 * time.Hour)
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "test-seed", BaselineDays: 30})

	business := database.Business{BusinessId: "caja-early-drawer", Name: "Caja Early Drawer", IsActive: true}
	require.NoError(t, db.Create(&business).Error)

	drawer := houseRailDrawer(t, db, business.ID, day.Add(5*time.Hour))
	seedConfirmedCashBill(t, db, svc, business.ID, "B-all-day", 2750000, database.PaymentMethodCash, day.Add(13*time.Hour))

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return svc.generateCashSessionForDay(context.Background(), tx, business.ID, day)
	}))

	var sessions []database.CashRegisterSession
	require.NoError(t, db.Where("business_id = ?", business.ID).Find(&sessions).Error)
	require.Len(t, sessions, 1, "no generated shift may overlap a drawer that opened first")
	require.Equal(t, drawer.ID, sessions[0].ID)
	require.Equal(t, database.CashRegisterSessionStatusOpen, sessions[0].Status)
	require.Equal(t, int64(2750000), sessions[0].CashSalesCents)
}

// A day with a single generated shift keeps the whole day's cash — the
// partition must not change the pre-#857 answer.
func TestReconcileCashSessionsKeepsSingleDrawerWhole(t *testing.T) {
	db := newDemoServiceTestDB(t)
	day := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "test-seed", BaselineDays: 30})

	business := database.Business{BusinessId: "caja-single", Name: "Caja Single", IsActive: true}
	require.NoError(t, db.Create(&business).Error)

	// One tender before the shift opened and one after it closed still belong
	// to the only drawer of the day.
	seedConfirmedCashBill(t, db, svc, business.ID, "B-pre-open", 100000, database.PaymentMethodCash, day.Add(2*time.Hour))
	seedConfirmedCashBill(t, db, svc, business.ID, "B-service", 300000, database.PaymentMethodCash, day.Add(13*time.Hour))
	seedConfirmedCashBill(t, db, svc, business.ID, "B-post-close", 50000, database.PaymentMethodCash, day.Add(23*time.Hour+50*time.Minute))

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return svc.generateCashSessionForDay(context.Background(), tx, business.ID, day)
	}))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return svc.ReconcileCashSessions(context.Background(), tx, business.ID, day, day.AddDate(0, 0, 2))
	}))

	var session database.CashRegisterSession
	require.NoError(t, db.Where("business_id = ?", business.ID).First(&session).Error)
	require.Equal(t, int64(450000), session.CashSalesCents)
	expected := session.OpeningFloatCents + session.CashSalesCents - session.CashRefundsCents + session.CashInCents - session.CashOutCents
	require.Equal(t, expected, session.ExpectedCashCents)
	require.Equal(t, session.CountedCashCents-session.ExpectedCashCents, session.VarianceCents)
}

// reloadSession re-reads a drawer straight from the DB.
func reloadSession(t *testing.T, db *gorm.DB, id uint) database.CashRegisterSession {
	t.Helper()
	var out database.CashRegisterSession
	require.NoError(t, db.First(&out, id).Error)
	return out
}

// #857, day two: the house rail (cashregister.ensureDemoHouseCashSessionTx)
// only ever OPENS the demo drawer — nothing but a human close ever ends it. A
// drawer opened at 12:57 is therefore still open tomorrow, and both of the
// handover queries were scoped to [day, day+24h), so on D+1 the carried-over
// drawer was invisible: the generator saw an empty day, minted a fresh
// 07:xx→now CLOSED twin straddling the live drawer, and stamped D+1's cash
// onto the twin. That is a line-for-line replay of the reported live state
// (venue 142: closed 07:19→16:00 holding the cash beside the still-open 362).
//
// A drawer that is still open owns every day from midnight until someone
// closes it.
func TestNoClosedTwinIsMintedBesideADrawerCarriedPastMidnight(t *testing.T) {
	db := newDemoServiceTestDB(t)
	day := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	next := day.AddDate(0, 0, 1)
	now := day.Add(16 * time.Hour)
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "test-seed", BaselineDays: 30})

	business := database.Business{BusinessId: "caja-overnight", Name: "Caja Overnight", IsActive: true}
	require.NoError(t, db.Create(&business).Error)

	generate := func(target time.Time) {
		require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
			return svc.generateCashSessionForDay(context.Background(), tx, business.ID, target)
		}))
	}

	// Day one: lunch on the generated shift, dinner after the operator opened
	// the live drawer at 12:57.
	seedConfirmedCashBill(t, db, svc, business.ID, "B-d1-lunch", 4500000, database.PaymentMethodCash, day.Add(11*time.Hour))
	seedConfirmedCashBill(t, db, svc, business.ID, "B-d1-dinner", 9430000, database.PaymentMethodCash, day.Add(14*time.Hour))
	generate(day)
	drawer := houseRailDrawer(t, db, business.ID, day.Add(12*time.Hour+57*time.Minute))
	generate(day)

	// Day two. Nobody closed the drawer overnight; the floor keeps taking cash
	// into the same till and the hourly append walks onto the new day.
	now = next.Add(16 * time.Hour)
	seedConfirmedCashBill(t, db, svc, business.ID, "B-d2-lunch", 7000000, database.PaymentMethodCash, next.Add(13*time.Hour))
	generate(next)
	generate(next) // a second hourly tick must not mint one either

	var sessions []database.CashRegisterSession
	require.NoError(t, db.Where("business_id = ?", business.ID).Order("opened_at ASC").Find(&sessions).Error)
	require.Len(t, sessions, 2,
		"a drawer that is still open owns the next day: no closed twin may be minted beside it")

	generated := sessions[0]
	require.Equal(t, demoCashActorLabel, generated.OpenedByLabel)
	require.NotNil(t, generated.ClosedAt)
	require.False(t, generated.ClosedAt.After(day.Add(12*time.Hour+57*time.Minute)))
	require.Equal(t, int64(4500000), generated.CashSalesCents,
		"the closed shift keeps only the cash taken before it handed over")

	live := reloadSession(t, db, drawer.ID)
	require.Equal(t, database.CashRegisterSessionStatusOpen, live.Status, "the live drawer must never be closed")
	require.Nil(t, live.ClosedAt)
	require.Equal(t, int64(9430000+7000000), live.CashSalesCents,
		"the till the floor is holding carries every check taken since it opened, across midnight")
	require.Zero(t, live.CountedCashCents, "an open drawer has not been counted yet")
	require.Zero(t, live.VarianceCents)

	// Nothing was double-counted or dropped on the way across midnight.
	require.Equal(t, int64(4500000+9430000+7000000), generated.CashSalesCents+live.CashSalesCents)
	count, totalCents, err := database.UnassignedCashAlternativePaymentSummary(db, business.ID)
	require.NoError(t, err)
	require.Zero(t, count)
	require.Zero(t, totalCents)

	var cashSaleMovements int64
	require.NoError(t, db.Model(&database.CashRegisterMovement{}).
		Where("business_id = ? AND movement_type = ?", business.ID, database.CashRegisterMovementTypeCashSale).
		Count(&cashSaleMovements).Error)
	require.Equal(t, int64(3), cashSaleMovements, "re-running the tick must not duplicate movements")
}

// The second symptom of the same root cause: ReconcileCashSessions re-runs the
// per-day pass over the whole baseline every hour, and the day's figures were
// written ABSOLUTELY. Real cash the floor took on a later day — added to the
// still-open drawer by database.incrementOpenCashRegisterSessionTotalTx — was
// therefore reset back down to the sum of the day being reconciled.
//
// The demo may re-derive its own seeded tenders. It may not delete a peso the
// floor actually rang up, and it may not move a real operator's movement onto
// one of its own shifts.
func TestReconcileKeepsCashTheLiveDrawerAccruedOnALaterDay(t *testing.T) {
	db := newDemoServiceTestDB(t)
	day := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	next := day.AddDate(0, 0, 1)
	now := day.Add(16 * time.Hour)
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "test-seed", BaselineDays: 30})

	business := database.Business{BusinessId: "caja-reconcile-live", Name: "Caja Reconcile Live", IsActive: true}
	require.NoError(t, db.Create(&business).Error)

	seedConfirmedCashBill(t, db, svc, business.ID, "B-live-lunch", 4500000, database.PaymentMethodCash, day.Add(11*time.Hour))
	seedConfirmedCashBill(t, db, svc, business.ID, "B-live-dinner", 9430000, database.PaymentMethodCash, day.Add(14*time.Hour))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return svc.generateCashSessionForDay(context.Background(), tx, business.ID, day)
	}))
	drawer := houseRailDrawer(t, db, business.ID, day.Add(12*time.Hour+57*time.Minute))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return svc.generateCashSessionForDay(context.Background(), tx, business.ID, day)
	}))

	// The floor rings up a real cash check the next day, through the real door
	// (a cash_sale movement plus the open-drawer increment).
	realAt := next.Add(15 * time.Hour)
	realBill := database.Bill{BusinessID: business.ID, BillNumber: "B-real-cash", Status: database.BillStatusPaid,
		TotalAmount: 2500000, PaidAmount: 2500000, CreatedAt: realAt.Add(-time.Hour), ClosedAt: &realAt}
	require.NoError(t, db.Create(&realBill).Error)
	realPayment := database.AlternativePayment{
		BillID: realBill.ID, Amount: 2500000, BillAmountCents: 2500000,
		PaymentMethod: database.PaymentMethodCash, Status: database.AltPaymentStatusConfirmed, ConfirmedAt: &realAt,
	}
	require.NoError(t, db.Create(&realPayment).Error)
	cashier := uint(4242)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return database.AttachCashRegisterMovementForAlternativePaymentTx(tx, business.ID, realPayment,
			database.CashRegisterMovementTypeCashSale, 2500000,
			database.CashRegisterActor{StaffID: &cashier, Label: "cajera"}, realAt)
	}))
	require.Equal(t, int64(9430000+2500000), reloadSession(t, db, drawer.ID).CashSalesCents,
		"precondition: the live drawer holds its own cash plus the real check")

	// The hourly reconcile walks the whole baseline, day one included.
	now = next.Add(20 * time.Hour)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return svc.ReconcileCashSessions(context.Background(), tx, business.ID, day, next.AddDate(0, 0, 1))
	}))

	live := reloadSession(t, db, drawer.ID)
	require.Equal(t, database.CashRegisterSessionStatusOpen, live.Status)
	require.Nil(t, live.ClosedAt)
	require.Equal(t, int64(9430000+2500000), live.CashSalesCents,
		"re-deriving day one must not delete the cash the floor took on day two")

	// The operator's own movement stays on the operator's drawer.
	var realMovement database.CashRegisterMovement
	require.NoError(t, db.Where("alternative_payment_id = ?", realPayment.ID).First(&realMovement).Error)
	require.Equal(t, drawer.ID, realMovement.SessionID,
		"the demo must never re-point a movement it did not write")
	require.Equal(t, "cajera", realMovement.ActorLabel)
}
