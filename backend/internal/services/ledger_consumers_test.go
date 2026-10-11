package services

import (
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupServicesLedgerTestDB(t *testing.T) *database.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Bill{},
		&database.Payment{},
		&database.AlternativePayment{},
		&database.BusinessMilestoneEvent{},
		&database.Order{},
		&database.ReportSchedule{},
	))
	return database.GetDBWrapper()
}

func TestDirectorContextActiveBillsIncludePartialBills(t *testing.T) {
	db := setupServicesLedgerTestDB(t)
	service := NewDirectorConsoleService(db, analytics.NewAnalyticsService(db), nil, nil)
	// UTC: SQLite compares stored timestamps as text, so a local-offset
	// timestamp after 21:00 at -03:00 sorts before a UTC service-day start.
	now := time.Now().UTC()
	business := createServicesLedgerBusiness(t, db, "Director Active Ledger Restaurant")

	// Place fixtures inside the current service day: fixed offsets like
	// now-30m straddle the day boundary just after midnight and drop the
	// earlier bills out of "today".
	dayStart := database.ServiceDayStart(now, database.ResolveBusinessLocation(&business), business.ServiceDayStartMinute).UTC()
	elapsed := now.Sub(dayStart)
	at := func(quarter int) time.Time { return dayStart.Add(elapsed * time.Duration(quarter) / 4) }
	createServicesLedgerBill(t, db, business.ID, "DIRECTOR-OPEN", 1000, 0, database.BillStatusOpen, at(1))
	createServicesLedgerBill(t, db, business.ID, "DIRECTOR-PARTIAL", 1000, 500, database.BillStatusPartial, at(2))
	createServicesLedgerBill(t, db, business.ID, "DIRECTOR-PAID", 1000, 1000, database.BillStatusPaid, at(3))

	ctx, err := service.buildContext(DirectorAskRequest{
		BusinessID: business.ID,
		Message:    "How is the floor doing?",
		Locale:     "en",
		ActiveTab:  "overview",
	}, &business)
	require.NoError(t, err)

	assert.Equal(t, 2, ctx.Metrics.ActiveBills)
	assert.True(t, ctx.DataReadiness.PaymentsEnabled, "open/partial bills mean payments work even without a plugin")
	assert.Equal(t, "established", ctx.DataReadiness.State)
	assert.InDelta(t, 15.0, ctx.Metrics.TodayFloorRemaining, 0.01, "open $10 + remaining $5 on partial")
	assert.InDelta(t, 15.0, ctx.Metrics.TodayRevenue, 0.01, "recognized paid amounts today, not leftover remaining")
	assert.Contains(t, ctx.MetricsSummary, "Collected today")
	assert.Contains(t, ctx.MetricsSummary, "Remaining on open checks")
}

func TestMilestoneTrackerProcessesPendingOutboxEventsIdempotently(t *testing.T) {
	db := setupServicesLedgerTestDB(t)
	tracker := NewMilestoneTracker(db)
	business := createServicesLedgerBusiness(t, db, "Milestone Outbox Restaurant")
	business.Email = "owner@example.com"
	business.OwnerName = "Owner"
	require.NoError(t, db.GetGorm().Save(&business).Error)

	event := database.BusinessMilestoneEvent{
		BusinessID:     business.ID,
		MilestoneType:  database.BusinessMilestoneTypeRevenue,
		ThresholdCents: 100000,
		Status:         database.BusinessMilestoneStatusPending,
		Payload: map[string]interface{}{
			"display_text": "$1,000",
		},
	}
	require.NoError(t, db.GetGorm().Create(&event).Error)

	require.NoError(t, tracker.ProcessPendingMilestones(business.ID))
	require.NoError(t, tracker.ProcessPendingMilestones(business.ID))

	var reloaded database.BusinessMilestoneEvent
	require.NoError(t, db.GetGorm().First(&reloaded, event.ID).Error)
	assert.Equal(t, database.BusinessMilestoneStatusSent, reloaded.Status)
	assert.NotNil(t, reloaded.SentAt)
}

func createServicesLedgerBusiness(t *testing.T, db *database.DB, name string) database.Business {
	t.Helper()

	business := database.Business{
		Name:           name,
		SettlementAddr: "settlement-" + name,
		TippingAddr:    "tipping-" + name,
	}
	require.NoError(t, db.GetGorm().Create(&business).Error)
	return business
}

func createServicesLedgerBill(t *testing.T, db *database.DB, businessID uint, number string, total, paid int64, status database.BillStatus, at time.Time) database.Bill {
	t.Helper()

	bill := database.Bill{
		BusinessID:     businessID,
		BillNumber:     fmt.Sprintf("%s-%d", number, time.Now().UnixNano()),
		Subtotal:       total,
		TotalAmount:    total,
		PaidAmount:     paid,
		Status:         status,
		SettlementAddr: "settlement-" + number,
		TippingAddr:    "tipping-" + number,
		CreatedAt:      at,
		UpdatedAt:      at,
	}
	require.NoError(t, db.GetGorm().Create(&bill).Error)
	return bill
}
