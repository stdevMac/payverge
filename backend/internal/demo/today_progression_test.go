package demo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Live-review regression (2026-07-03): the day generator wrote TODAY's full
// day of bills up front — at 10am the briefing saw the whole day's revenue,
// divided it by a ~0.1 elapsed-day fraction, and proclaimed "964% ahead of a
// typical Friday". Bills must only exist once their closed_at has passed, so
// the showroom accrues revenue through the day like a real restaurant, and
// the daily pointer must not advance past yesterday until today is complete
// (the hourly append re-runs today idempotently, materializing newly-closed
// bills).
func TestTodayBillsMaterializeAsTheDayProgresses(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "demo-admin-progression@example.com")

	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	require.NoError(t, err)

	// 08:00 BsAs — before the first demo bill of the day (bills open 11:00+).
	now := time.Date(2026, 7, 2, 8, 0, 0, 0, loc)
	svc := NewService(db, Options{Now: func() time.Time { return now }, SeedVersion: "test-seed-v1", BaselineDays: 3})

	_, err = svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var businesses []database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", admin.ID).Find(&businesses).Error)
	require.NotEmpty(t, businesses)
	ids := make([]uint, 0, len(businesses))
	for _, b := range businesses {
		ids = append(ids, b.ID)
	}

	today := time.Date(2026, 7, 2, 0, 0, 0, 0, loc)
	// Only the generated service-day bills — the seeder also plants a couple
	// of static OPEN/partial bills dated near "now" (the live open-table story),
	// which legitimately exist before close. Service-day bills are identified
	// by closed_at falling on the calendar day (real-path B{id}-* numbers).
	countDay := func(day time.Time) int64 {
		dayStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
		dayEnd := dayStart.AddDate(0, 0, 1)
		var n int64
		require.NoError(t, db.Model(&database.Bill{}).
			Where("business_id IN ? AND status = ? AND closed_at >= ? AND closed_at < ?",
				ids, database.BillStatusPaid, dayStart.UTC(), dayEnd.UTC()).
			Count(&n).Error)
		return n
	}
	countToday := func() int64 { return countDay(today) }
	countYesterday := func() int64 { return countDay(today.AddDate(0, 0, -1)) }

	require.NotZero(t, countYesterday(), "completed days must be fully seeded")
	require.Zero(t, countToday(), "no bill may exist before its closed_at has passed")

	// The pointer must stay at yesterday so later appends revisit today.
	var instance database.DemoInstance
	require.NoError(t, db.Where("admin_user_id = ?", admin.ID).First(&instance).Error)
	require.NotNil(t, instance.LastSimulatedBusinessDate)
	require.Equal(t, today.AddDate(0, 0, -1).Format("2006-01-02"),
		instance.LastSimulatedBusinessDate.In(loc).Format("2006-01-02"))

	// 23:30 BsAs — service is over; the hourly append must materialize today.
	now = time.Date(2026, 7, 2, 23, 30, 0, 0, loc)
	require.NoError(t, svc.AppendDueDaysForAdmin(context.Background(), admin.ID))

	require.NotZero(t, countToday(), "today's bills must appear once their close time passes")
	var future int64
	require.NoError(t, db.Model(&database.Bill{}).
		Where("business_id IN ? AND closed_at > ?", ids, now.UTC()).
		Count(&future).Error)
	require.Zero(t, future, "no bill may close in the future")

	// Appending again must be a no-op (idempotent hourly cadence).
	before := countToday()
	require.NoError(t, svc.AppendDueDaysForAdmin(context.Background(), admin.ID))
	require.Equal(t, before, countToday())

	// Next morning: yesterday (=our "today") is complete, pointer advances.
	now = time.Date(2026, 7, 3, 4, 0, 0, 0, loc)
	require.NoError(t, svc.AppendDueDaysForAdmin(context.Background(), admin.ID))
	require.NoError(t, db.Where("admin_user_id = ?", admin.ID).First(&instance).Error)
	require.Equal(t, today.Format("2006-01-02"),
		instance.LastSimulatedBusinessDate.In(loc).Format("2006-01-02"))
}
