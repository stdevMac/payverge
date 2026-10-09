package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func setupReconcileTestDB(t *testing.T) *database.DB {
	t.Helper()
	previous := database.GetDB()
	t.Cleanup(func() { database.SetTestDB(previous) })

	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{}, &database.Plugin{}, &database.BusinessPlugin{}, &database.ReportSchedule{}, &database.ReportDelivery{},
	))
	database.SetTestDB(gormDB)
	return database.GetDBWrapper()
}

// Fix 6: the schedule row is not created/deleted atomically with the owning
// report plugin's enable/disable, so a disable can orphan an active schedule that
// keeps firing. The worker reconciles at pick-up: a due schedule whose plugin is
// not enabled is deactivated and skipped (no email attempted).
func TestCheckAndSendDueReportsDeactivatesOrphanedSchedule(t *testing.T) {
	dbw := setupReconcileTestDB(t)

	require.NoError(t, database.GetDB().Create(&database.Business{
		Name:  "Acme",
		Email: "owner@acme.test",
	}).Error)

	// The daily report plugin exists in the catalog but is NOT enabled for the
	// business (no BusinessPlugin row) — the orphaned-schedule scenario.
	_, err := database.CreatePlugin(database.Plugin{
		Name:        "daily_email_report",
		DisplayName: "Daily Email Report",
		Category:    "analytics",
		Version:     "1.0.0",
		IsActive:    true,
	})
	require.NoError(t, err)

	sched := &database.ReportSchedule{
		BusinessID: 1,
		Frequency:  database.ReportFrequencyDaily,
		Hour:       8,
		Minute:     0,
		Timezone:   "UTC",
		IsActive:   true,
		NextSendAt: time.Now().Add(-time.Minute), // already due
	}
	require.NoError(t, dbw.CreateReportSchedule(sched))

	// nil email/analytics is safe: the schedule must be skipped before any send.
	rs := NewReportScheduler(dbw, nil, nil)
	rs.checkAndSendDueReports()

	reloaded, err := dbw.GetReportScheduleByBusinessAndFrequency(1, database.ReportFrequencyDaily)
	require.NoError(t, err)
	require.NotNil(t, reloaded)
	require.False(t, reloaded.IsActive, "orphaned schedule (plugin disabled) must be deactivated")
}
