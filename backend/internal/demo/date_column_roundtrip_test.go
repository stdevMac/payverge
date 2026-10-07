package demo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Live-stack regression (2026-08-23): Postgres stores the demo pointer in a
// `date` column and GORM reads it back as midnight UTC (e.g. 2026-08-22T00:00:00Z),
// not midnight in the instance timezone. appendDueDaysForInstance compared that
// raw instant against BsAs-midnight "today", so a UTC-midnight pointer sat 3h
// EARLIER than the loc-midnight it denotes: the first append of the day saw
// `next` (today, 00:00Z) as strictly before `today` (00:00-03:00) and stamped
// the pointer with TODAY. Every later hourly append was then a no-op and the
// evening's bills never materialized — the showroom froze mid-day.
func TestAppendSurvivesPostgresDateColumnUTCRoundTrip(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "demo-admin-datecol@example.com")

	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	require.NoError(t, err)

	// 01:30 BsAs on Jul 2 — right after the business-day boundary.
	now := time.Date(2026, 7, 2, 1, 30, 0, 0, loc)
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "test-seed-v1", BaselineDays: 3})

	_, err = svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var instance database.DemoInstance
	require.NoError(t, db.Where("admin_user_id = ?", admin.ID).First(&instance).Error)
	require.NotNil(t, instance.LastSimulatedBusinessDate)
	require.Equal(t, "2026-07-01", storedBusinessDate(*instance.LastSimulatedBusinessDate, loc).Format("2006-01-02"),
		"ensure must leave the pointer at yesterday")

	// Simulate the Postgres `date` column read shape: same calendar date, but
	// represented as midnight UTC. SQLite round-trips full timestamps, so the
	// production representation has to be injected explicitly.
	utcPointer := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Model(&database.DemoInstance{}).
		Where("id = ?", instance.ID).
		Update("last_simulated_business_date", utcPointer).Error)

	// First append of the day. It may generate today's early bills, but the
	// pointer must NOT advance past yesterday — today is still in progress.
	require.NoError(t, svc.AppendDueDaysForAdmin(context.Background(), admin.ID))
	require.NoError(t, db.Where("admin_user_id = ?", admin.ID).First(&instance).Error)
	require.Equal(t, "2026-07-01", storedBusinessDate(*instance.LastSimulatedBusinessDate, loc).Format("2006-01-02"),
		"pointer must stay at yesterday while today is in progress, even with a UTC-midnight stored date")

	// 23:30 BsAs — the evening append must now materialize today's bills.
	// Under the bug the pointer was already stamped with today, this append
	// no-ops, and today stays empty.
	now = time.Date(2026, 7, 2, 23, 30, 0, 0, loc)
	require.NoError(t, svc.AppendDueDaysForAdmin(context.Background(), admin.ID))

	var businesses []database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", admin.ID).Find(&businesses).Error)
	require.NotEmpty(t, businesses)
	ids := make([]uint, 0, len(businesses))
	for _, b := range businesses {
		ids = append(ids, b.ID)
	}
	dayStart := time.Date(2026, 7, 2, 0, 0, 0, 0, loc)
	dayEnd := dayStart.AddDate(0, 0, 1)
	var todayBills int64
	require.NoError(t, db.Model(&database.Bill{}).
		Where("business_id IN ? AND status = ? AND closed_at >= ? AND closed_at < ?",
			ids, database.BillStatusPaid, dayStart.UTC(), dayEnd.UTC()).
		Count(&todayBills).Error)
	require.NotZero(t, todayBills, "evening append must materialize today's bills despite the UTC date round-trip")
}

// recencyClaimedEnd must interpret a UTC-midnight stored date as the calendar
// day it denotes, not shift it into the previous BsAs day (which silently
// loosened the staleness check by a day).
func TestRecencyClaimedEndSurvivesUTCDateRoundTrip(t *testing.T) {
	db := newDemoServiceTestDB(t)
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	require.NoError(t, err)

	now := time.Date(2026, 7, 10, 12, 0, 0, 0, loc)
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "test-seed-v1", BaselineDays: 3})

	utcPointer := time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)
	instance := &database.DemoInstance{
		Timezone:                  "America/Argentina/Buenos_Aires",
		LastSimulatedBusinessDate: &utcPointer,
	}
	claimed := svc.recencyClaimedEnd(instance)
	require.Equal(t, "2026-07-09", claimed.Format("2006-01-02"))
	require.Equal(t, loc.String(), claimed.Location().String())
}
