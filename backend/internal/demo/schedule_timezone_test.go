package demo

import (
	"context"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

// #247 — published demo shifts seeded as UTC wall times rendered as
// 4:00 AM–12:00 PM in America/New_York, so dinner looked unstaffed while
// the venue took dinner orders. Seed must use venue-local walls and cover
// evening service.
func TestDemoPublishedShiftsCoverDinnerInVenueTimezone(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "demo-admin-schedule-tz@example.com")

	loc, err := time.LoadLocation(defaultTimezone)
	require.NoError(t, err)
	// Wednesday mid-service so the published week is unambiguous.
	now := time.Date(2026, 8, 12, 15, 0, 0, 0, loc)
	svc := NewService(db, Options{
		Now:          func() time.Time { return now },
		SeedVersion:  "test-seed-schedule-tz",
		BaselineDays: 7,
	})
	_, err = svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var businesses []database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", admin.ID).Find(&businesses).Error)
	require.NotEmpty(t, businesses)

	for _, business := range businesses {
		require.Equal(t, defaultTimezone, business.Timezone,
			"demo business must advertise the venue zone the schedule is seeded in")

		var shifts []database.Shift
		require.NoError(t, db.Where("business_id = ? AND published = ?", business.ID, true).Find(&shifts).Error)
		require.GreaterOrEqual(t, len(shifts), 5, "business %d needs a published roster", business.ID)

		dinnerCount := 0
		collapsedBugCount := 0
		for _, shift := range shifts {
			startLocal := shift.StartsAt.In(loc)
			endLocal := shift.EndsAt.In(loc)
			startHour, startMin, _ := startLocal.Clock()
			endHour, endMin, _ := endLocal.Clock()

			// Classic UTC-wall seed bug: 08:00–16:00 UTC → 04:00–12:00 NY.
			if startHour == 4 && startMin == 0 && endHour == 12 && endMin == 0 {
				collapsedBugCount++
			}
			// Dinner coverage: starts at/after 16:00 venue-local (or notes say dinner).
			if startHour >= 16 {
				dinnerCount++
			}
		}

		require.Zero(t, collapsedBugCount,
			"business %d: %d published shifts collapse to 4:00–12:00 venue-local (UTC wall seed / missing TZ)",
			business.ID, collapsedBugCount)
		require.GreaterOrEqual(t, dinnerCount, 3,
			"business %d: need ≥3 dinner-start shifts in %s, got %d (Fri/Sat evenings must be staffed)",
			business.ID, defaultTimezone, dinnerCount)

		// Open coverage shift must also be dinner-shaped, not noon UTC.
		var openShifts []database.Shift
		require.NoError(t, db.Where(
			"business_id = ? AND staff_id IS NULL AND published = ?", business.ID, true,
		).Find(&openShifts).Error)
		require.NotEmpty(t, openShifts, "business %d missing open coverage shift", business.ID)
		for _, open := range openShifts {
			require.GreaterOrEqual(t, open.StartsAt.In(loc).Hour(), 16,
				"open shift %d starts at %s venue-local; must be dinner coverage",
				open.ID, open.StartsAt.In(loc).Format(time.Kitchen))
		}
	}
}
