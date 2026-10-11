package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupReportDeliveryTestDB(t *testing.T) *DB {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gdb.AutoMigrate(&Business{}, &ReportSchedule{}, &ReportDelivery{}))
	return &DB{conn: gdb}
}

func createDueReportSchedule(t *testing.T, db *DB, due time.Time) ReportSchedule {
	t.Helper()
	business := Business{Name: "Report delivery", Email: "owner@example.test"}
	require.NoError(t, db.conn.Create(&business).Error)
	schedule := ReportSchedule{
		BusinessID: business.ID,
		Frequency:  ReportFrequencyDaily,
		Hour:       due.In(time.UTC).Hour(),
		Minute:     due.In(time.UTC).Minute(),
		Timezone:   "UTC",
		IsActive:   true,
		NextSendAt: due,
	}
	require.NoError(t, db.CreateReportSchedule(&schedule))
	return schedule
}

func TestReportDeliveryClaimDoesNotAdvanceSchedule(t *testing.T) {
	db := setupReportDeliveryTestDB(t)
	due := time.Date(2026, 3, 9, 8, 0, 0, 0, time.UTC)
	schedule := createDueReportSchedule(t, db, due)

	delivery, created, err := db.EnsureReportDelivery(&schedule, due)
	require.NoError(t, err)
	require.True(t, created)
	claimed, err := db.ClaimReportDelivery(delivery.ID, "worker-a", due, 5*time.Minute, 3)
	require.NoError(t, err)
	require.NotNil(t, claimed)
	require.Equal(t, ReportDeliveryStateLeased, claimed.State)
	require.Equal(t, 1, claimed.AttemptCount)

	reloaded, err := db.GetReportScheduleByID(schedule.ID)
	require.NoError(t, err)
	require.Nil(t, reloaded.LastSentAt, "claiming is not delivery")
	require.Equal(t, due, reloaded.NextSendAt)
}

func TestReportDeliveryLeaseExpiryAndAcknowledgement(t *testing.T) {
	db := setupReportDeliveryTestDB(t)
	due := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	schedule := createDueReportSchedule(t, db, due)
	delivery, _, err := db.EnsureReportDelivery(&schedule, due)
	require.NoError(t, err)

	first, err := db.ClaimReportDelivery(delivery.ID, "worker-a", due, time.Minute, 3)
	require.NoError(t, err)
	require.NotNil(t, first)

	tooEarly, err := db.ClaimReportDelivery(delivery.ID, "worker-b", due.Add(59*time.Second), time.Minute, 3)
	require.NoError(t, err)
	require.Nil(t, tooEarly)

	reclaimed, err := db.ClaimReportDelivery(delivery.ID, "worker-b", due.Add(time.Minute), time.Minute, 3)
	require.NoError(t, err)
	require.NotNil(t, reclaimed)
	require.Equal(t, 2, reclaimed.AttemptCount)

	next := nextSendTimeFrom(&schedule, due.Add(time.Nanosecond))
	won, err := db.AcknowledgeReportDelivery(delivery.ID, "worker-b", due.Add(time.Minute), next)
	require.NoError(t, err)
	require.True(t, won)

	reloaded, err := db.GetReportScheduleByID(schedule.ID)
	require.NoError(t, err)
	require.NotNil(t, reloaded.LastSentAt)
	require.Equal(t, next, reloaded.NextSendAt)

	row, err := db.GetReportDeliveryByID(delivery.ID)
	require.NoError(t, err)
	require.Equal(t, ReportDeliveryStateSent, row.State)
	require.NotNil(t, row.SentAt)
}

func TestReportDeliveryWindowUsesBusinessCalendarAcrossDST(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	schedule := ReportSchedule{Frequency: ReportFrequencyDaily, Timezone: loc.String()}
	// The report sent after spring-forward covers a 23-hour local day.
	due := time.Date(2026, time.March, 9, 8, 0, 0, 0, loc)
	start, end := ReportWindowForSchedule(&schedule, due)
	require.Equal(t, time.Date(2026, time.March, 8, 0, 0, 0, 0, loc), start)
	require.Equal(t, time.Date(2026, time.March, 9, 0, 0, 0, 0, loc), end)
	require.Equal(t, 23*time.Hour, end.Sub(start))

	schedule.Frequency = ReportFrequencyWeekly
	start, end = ReportWindowForSchedule(&schedule, due)
	require.Equal(t, time.Date(2026, time.March, 2, 0, 0, 0, 0, loc), start)
	require.Equal(t, time.Date(2026, time.March, 9, 0, 0, 0, 0, loc), end)
}

func TestReportDeliveryAcknowledgementPreservesNewerScheduleConfiguration(t *testing.T) {
	db := setupReportDeliveryTestDB(t)
	due := time.Date(2026, 8, 8, 8, 0, 0, 0, time.UTC)
	schedule := createDueReportSchedule(t, db, due)
	delivery, _, err := db.EnsureReportDelivery(&schedule, due)
	require.NoError(t, err)
	claimed, err := db.ClaimReportDelivery(delivery.ID, "worker", due, time.Minute, 3)
	require.NoError(t, err)
	require.NotNil(t, claimed)

	operatorNext := due.Add(10 * 24 * time.Hour)
	require.NoError(t, db.conn.Model(&ReportSchedule{}).Where("id = ?", schedule.ID).Update("next_send_at", operatorNext).Error)
	won, err := db.AcknowledgeReportDelivery(delivery.ID, "worker", due.Add(time.Second), due.Add(24*time.Hour))
	require.NoError(t, err)
	require.True(t, won)

	reloaded, err := db.GetReportScheduleByID(schedule.ID)
	require.NoError(t, err)
	require.Equal(t, operatorNext, reloaded.NextSendAt)
	require.NotNil(t, reloaded.LastSentAt)
}
