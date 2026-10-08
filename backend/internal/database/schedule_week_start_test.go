package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// #854 — the operator Schedule tab (and the live-floor "quién trabaja hoy"
// strip) went blank on a fully staffed Sunday service: GET /schedule?week=
// 2026-08-17 returned shifts=[] while five staff and a 21:00 party were on the
// books.
//
// schedules.week_start carries the week-start CALENDAR DAY, but nothing pins the
// instant it is stored at: the readers (parseWeekValue/weekFloorUTC in
// handlers/schedule.go, weekStartUTCMidnight in handlers/livefloor.go) build UTC
// midnight, while the writer that seeded the showroom week stored venue-local
// midnight — 2026-08-17T00:00:00-03:00 == 2026-08-17T03:00:00Z for
// America/Argentina/Buenos_Aires. An exact-equality lookup misses by three
// hours and reports "no schedule this week".
//
// These tests drive the real reads (GetScheduleForWeek,
// GetPublishedScheduleForStaff, GetOrCreateDraftSchedule) against a week row
// written exactly the way the venue-local writer writes it.

const (
	baTZ    = "America/Argentina/Buenos_Aires" // UTC-3, west of Greenwich
	tokyoTZ = "Asia/Tokyo"                     // UTC+9, east of Greenwich
)

// showroomWeekMonday is the week-start day of the live report: Monday
// 2026-08-17, whose Sunday is the 2026-08-23 dinner service that came back
// empty.
var showroomWeekMonday = struct{ y, m, d int }{2026, 8, 17}

// canonicalWeekStart is the value every reader computes for that week.
func canonicalWeekStart() time.Time {
	return time.Date(showroomWeekMonday.y, time.Month(showroomWeekMonday.m), showroomWeekMonday.d, 0, 0, 0, 0, time.UTC)
}

// venueLocalWeekStart is the value a venue-local writer stores for the same
// calendar week.
func venueLocalWeekStart(tz string) time.Time {
	loc := ResolveLocation(tz)
	return time.Date(showroomWeekMonday.y, time.Month(showroomWeekMonday.m), showroomWeekMonday.d, 0, 0, 0, 0, loc)
}

// seedShowroomWeek writes a published week whose week_start sits at venue-local
// midnight, plus the Sunday 16:00-00:00 dinner shift that must surface.
func seedShowroomWeek(t *testing.T, businessID uint, tz string, positionID, staffID uint) (uint, time.Time) {
	t.Helper()
	loc := ResolveLocation(tz)
	sched := Schedule{
		BusinessID: businessID,
		WeekStart:  venueLocalWeekStart(tz),
		Status:     ScheduleStatusPublished,
		Notes:      "Semana publicada",
	}
	require.NoError(t, db.Create(&sched).Error)

	sundayStart := time.Date(2026, 8, 23, 16, 0, 0, 0, loc)
	sh := Shift{
		BusinessID:       businessID,
		ScheduleID:       sched.ID,
		StaffID:          &staffID,
		PositionID:       positionID,
		StartsAt:         sundayStart.UTC(),
		EndsAt:           sundayStart.Add(8 * time.Hour).UTC(),
		BreakMinutes:     30,
		Status:           ShiftStatusFilled,
		Published:        true,
		Notes:            "Domingo noche",
		CreatedByStaffID: staffID,
	}
	require.NoError(t, db.Create(&sh).Error)
	return sched.ID, sundayStart
}

func TestGetScheduleForWeekResolvesVenueLocalWeekStart(t *testing.T) {
	d := newScheduleTestDB(t)
	posID, staffID := seedScheduleBusiness(t, 1)
	schedID, _ := seedShowroomWeek(t, 1, baTZ, posID, staffID)

	sched, shifts, err := d.GetScheduleForWeek(1, canonicalWeekStart())
	require.NoError(t, err)
	require.NotNil(t, sched, "the week's schedule must resolve from the UTC-midnight week key even though it was stored at venue-local midnight")
	require.Equal(t, schedID, sched.ID)
	require.Len(t, shifts, 1, "the Sunday dinner shift must come back with the week")
	require.Equal(t, "Domingo noche", shifts[0].Notes)
}

func TestGetScheduleForWeekResolvesEasternHemisphereWeekStart(t *testing.T) {
	d := newScheduleTestDB(t)
	posID, staffID := seedScheduleBusiness(t, 1)
	// Tokyo local midnight lands on the PREVIOUS UTC calendar day
	// (2026-08-16T15:00:00Z), so a "floor to the UTC day" read is not enough.
	require.Equal(t, 16, venueLocalWeekStart(tokyoTZ).UTC().Day())
	schedID, _ := seedShowroomWeek(t, 1, tokyoTZ, posID, staffID)

	sched, shifts, err := d.GetScheduleForWeek(1, canonicalWeekStart())
	require.NoError(t, err)
	require.NotNil(t, sched)
	require.Equal(t, schedID, sched.ID)
	require.Len(t, shifts, 1)
}

func TestGetScheduleForWeekPrefersCanonicalWeekStartRow(t *testing.T) {
	d := newScheduleTestDB(t)
	posID, staffID := seedScheduleBusiness(t, 1)
	seedShowroomWeek(t, 1, baTZ, posID, staffID) // legacy off-grid row

	canonical := Schedule{BusinessID: 1, WeekStart: canonicalWeekStart(), Status: ScheduleStatusDraft, Notes: "canonical"}
	require.NoError(t, db.Create(&canonical).Error)

	sched, shifts, err := d.GetScheduleForWeek(1, canonicalWeekStart())
	require.NoError(t, err)
	require.NotNil(t, sched)
	require.Equal(t, canonical.ID, sched.ID, "an exact UTC-midnight row always wins over an off-grid one")
	require.Empty(t, shifts)
}

func TestGetScheduleForWeekIgnoresAdjacentWeeks(t *testing.T) {
	d := newScheduleTestDB(t)
	seedScheduleBusiness(t, 1)
	loc := ResolveLocation(baTZ)
	for _, day := range []int{10, 24} { // the weeks either side of 2026-08-17
		prev := Schedule{BusinessID: 1, WeekStart: time.Date(2026, 8, day, 0, 0, 0, 0, loc), Status: ScheduleStatusPublished}
		require.NoError(t, db.Create(&prev).Error)
	}

	sched, shifts, err := d.GetScheduleForWeek(1, canonicalWeekStart())
	require.NoError(t, err)
	require.Nil(t, sched, "neighbouring weeks must never answer for the requested week")
	require.Empty(t, shifts)
}

func TestGetPublishedScheduleForStaffResolvesVenueLocalWeekStart(t *testing.T) {
	d := newScheduleTestDB(t)
	posID, staffID := seedScheduleBusiness(t, 1)
	schedID, _ := seedShowroomWeek(t, 1, baTZ, posID, staffID)

	sched, shifts, err := d.GetPublishedScheduleForStaff(1, canonicalWeekStart(), staffID)
	require.NoError(t, err)
	require.NotNil(t, sched, "a staff member must see their own published week")
	require.Equal(t, schedID, sched.ID)
	require.Len(t, shifts, 1)
	require.Equal(t, staffID, *shifts[0].StaffID)
}

func TestGetOrCreateDraftScheduleReusesVenueLocalWeekStart(t *testing.T) {
	d := newScheduleTestDB(t)
	posID, staffID := seedScheduleBusiness(t, 1)
	schedID, _ := seedShowroomWeek(t, 1, baTZ, posID, staffID)

	sched, err := d.GetOrCreateDraftSchedule(1, canonicalWeekStart(), staffID)
	require.NoError(t, err)
	require.NotNil(t, sched)
	require.Equal(t, schedID, sched.ID, "the existing week must be reused, not shadowed by a second row")

	var count int64
	require.NoError(t, db.Model(&Schedule{}).Where("business_id = ?", 1).Count(&count).Error)
	require.EqualValues(t, 1, count, "get-or-create must not duplicate the week")
}

func TestNormalizeWeekStart(t *testing.T) {
	canonical := canonicalWeekStart()
	require.True(t, NormalizeWeekStart(canonical).Equal(canonical), "an already-canonical value is unchanged")
	require.True(t, NormalizeWeekStart(venueLocalWeekStart(baTZ)).Equal(canonical), "venue-local midnight west of Greenwich keeps its calendar day")
	require.True(t, NormalizeWeekStart(venueLocalWeekStart(tokyoTZ)).Equal(canonical), "venue-local midnight east of Greenwich keeps its calendar day")
}
