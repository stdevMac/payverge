package cashregister_test

import (
	"context"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services/cashregister"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEnsureDemoHouseCashSessionOpensSeededDemoDrawer(t *testing.T) {
	db := openCashRegisterServiceTestDB(t)
	now := time.Date(2026, 8, 21, 20, 15, 0, 0, time.UTC)
	svc := cashregister.NewServiceWithClock(db, func() time.Time { return now })
	business := createDemoHouseRailBusiness(t, db, "demo-lounge-86")

	closedAt := time.Date(2026, 8, 21, 4, 0, 0, 0, time.UTC) // 00:00 EDT
	require.NoError(t, db.Create(&database.CashRegisterSession{
		BusinessID:        business.ID,
		Status:            database.CashRegisterSessionStatusClosed,
		OpeningFloatCents: 20000,
		CashSalesCents:    118230,
		OpenedByLabel:     "demo-manager",
		OpenedAt:          closedAt.Add(-2 * time.Hour),
		ClosedByLabel:     "demo-manager",
		ClosedAt:          &closedAt,
	}).Error)

	session, err := svc.EnsureDemoHouseCashSession(context.Background(), business.ID)
	require.NoError(t, err)
	require.NotNil(t, session)
	require.Equal(t, database.CashRegisterSessionStatusOpen, session.Status)
	require.Equal(t, cashregister.DemoHouseRailOpenedByLabel, session.OpenedByLabel)
	require.Equal(t, cashregister.DemoHouseRailOpeningNote, session.OpeningNote)
	require.Equal(t, int64(20000), session.OpeningFloatCents)
	require.Equal(t, now, session.OpenedAt)

	again, err := svc.EnsureDemoHouseCashSession(context.Background(), business.ID)
	require.NoError(t, err)
	require.Equal(t, session.ID, again.ID)

	snapshot, err := svc.Current(context.Background(), business.ID)
	require.NoError(t, err)
	require.NotNil(t, snapshot.Session)
	require.Equal(t, session.ID, snapshot.Session.ID)
}

func TestEnsureDemoHouseCashSessionLeavesRealVenueClosed(t *testing.T) {
	db := openCashRegisterServiceTestDB(t)
	svc := cashregister.NewServiceWithClock(db, fixedCashRegisterNow)
	business := createCashRegisterBusiness(t, db, "real-venue")
	_ = createClosedCashRegisterSession(t, db, business.ID, time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC))

	session, err := svc.EnsureDemoHouseCashSession(context.Background(), business.ID)
	require.NoError(t, err)
	require.Nil(t, session)

	snapshot, err := svc.Current(context.Background(), business.ID)
	require.NoError(t, err)
	require.Nil(t, snapshot.Session)
}

func TestEnsureDemoHouseCashSessionLeavesStickyIsDemoRealVenueClosed(t *testing.T) {
	db := openCashRegisterServiceTestDB(t)
	svc := cashregister.NewServiceWithClock(db, fixedCashRegisterNow)
	business := database.Business{
		BusinessId:     "sticky-is-demo-real",
		OwnerAddress:   "owner-sticky-is-demo-real",
		Name:           "sticky-is-demo-real",
		SettlementAddr: "settlement-sticky-is-demo-real",
		TippingAddr:    "tipping-sticky-is-demo-real",
		IsDemo:         true,
		Kind:           database.BusinessKindReal,
	}
	require.NoError(t, db.Create(&business).Error)

	session, err := svc.EnsureDemoHouseCashSession(context.Background(), business.ID)
	require.NoError(t, err)
	require.Nil(t, session)

	snapshot, err := svc.Current(context.Background(), business.ID)
	require.NoError(t, err)
	require.Nil(t, snapshot.Session)

	var count int64
	require.NoError(t, db.Model(&database.CashRegisterSession{}).Where("business_id = ?", business.ID).Count(&count).Error)
	require.Zero(t, count)
}

func TestEnsureDemoHouseCashSessionRespectsHumanCloseOnDemo(t *testing.T) {
	db := openCashRegisterServiceTestDB(t)
	svc := cashregister.NewServiceWithClock(db, fixedCashRegisterNow)
	business := createDemoHouseRailBusiness(t, db, "demo-core-85")
	closer := createCashRegisterUser(t, db, "manager@demo.test")

	closedAt := time.Date(2026, 8, 21, 16, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&database.CashRegisterSession{
		BusinessID:        business.ID,
		Status:            database.CashRegisterSessionStatusClosed,
		OpeningFloatCents: 15000,
		OpenedByLabel:     "manager",
		OpenedAt:          closedAt.Add(-8 * time.Hour),
		ClosedByUserID:    &closer.ID,
		ClosedByLabel:     "manager",
		ClosedAt:          &closedAt,
	}).Error)

	session, err := svc.EnsureDemoHouseCashSession(context.Background(), business.ID)
	require.NoError(t, err)
	require.Nil(t, session)
}

func TestEnsureDemoHouseCashSessionDoesNotReopenAfterNewerDemoSeedClose(t *testing.T) {
	db := openCashRegisterServiceTestDB(t)
	svc := cashregister.NewServiceWithClock(db, fixedCashRegisterNow)
	business := createDemoHouseRailBusiness(t, db, "demo-human-then-seed")
	closer := createCashRegisterUser(t, db, "closer@demo.test")

	humanClosedAt := time.Date(2026, 8, 21, 16, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&database.CashRegisterSession{
		BusinessID:        business.ID,
		Status:            database.CashRegisterSessionStatusClosed,
		OpeningFloatCents: 15000,
		OpenedByLabel:     "manager",
		OpenedAt:          humanClosedAt.Add(-8 * time.Hour),
		ClosedByUserID:    &closer.ID,
		ClosedByLabel:     "manager",
		ClosedAt:          &humanClosedAt,
	}).Error)

	seedClosedAt := time.Date(2026, 8, 22, 4, 0, 0, 0, time.UTC)
	require.NoError(t, db.Create(&database.CashRegisterSession{
		BusinessID:        business.ID,
		Status:            database.CashRegisterSessionStatusClosed,
		OpeningFloatCents: 15000,
		OpenedByLabel:     "demo-manager",
		OpenedAt:          seedClosedAt.Add(-2 * time.Hour),
		ClosedByLabel:     "demo-manager",
		ClosedAt:          &seedClosedAt,
	}).Error)

	session, err := svc.EnsureDemoHouseCashSession(context.Background(), business.ID)
	require.NoError(t, err)
	require.Nil(t, session)

	snapshot, err := svc.Current(context.Background(), business.ID)
	require.NoError(t, err)
	require.Nil(t, snapshot.Session)

	var openCount int64
	require.NoError(t, db.Model(&database.CashRegisterSession{}).
		Where("business_id = ? AND status = ?", business.ID, database.CashRegisterSessionStatusOpen).
		Count(&openCount).Error)
	require.Zero(t, openCount)
}

func TestEnsureDemoHouseCashSessionOpensWhenDemoHasNoHistory(t *testing.T) {
	db := openCashRegisterServiceTestDB(t)
	svc := cashregister.NewServiceWithClock(db, fixedCashRegisterNow)
	business := createDemoHouseRailBusiness(t, db, "fresh-demo")

	session, err := svc.EnsureDemoHouseCashSession(context.Background(), business.ID)
	require.NoError(t, err)
	require.NotNil(t, session)
	require.Equal(t, database.CashRegisterSessionStatusOpen, session.Status)
	require.Equal(t, int64(0), session.OpeningFloatCents)
}

func createDemoHouseRailBusiness(t testing.TB, db *gorm.DB, businessID string) database.Business {
	t.Helper()

	business := database.Business{
		BusinessId:     businessID,
		OwnerAddress:   "owner-" + businessID,
		Name:           businessID,
		SettlementAddr: "settlement-" + businessID,
		TippingAddr:    "tipping-" + businessID,
		IsDemo:         true,
		Kind:           database.BusinessKindDemo,
	}
	require.NoError(t, db.Create(&business).Error)
	return business
}
