package services

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #902: on the US AI-Pro demo venue (America/New_York, hours 11:00–23:00)
// the Director reported "20:00 UTC" as "8:00 PM NY" — four hours out — and said
// the hours were unknown, while guest Sage on the same venue answered
// "open 11:00–23:00 today". The snapshot's only clock was generated_at in UTC
// next to a bare timezone name, and it carried no operating hours at all.

// newYorkAt is the reported moment: 2026-08-23 is a Sunday, 20:00 UTC is
// 16:00 EDT (UTC-04:00). Any code that prints "8" here is reading UTC.
func newYorkAt(t *testing.T, clock string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, clock)
	require.NoError(t, err)
	return ts
}

func nyLocation(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	return loc
}

func sundayHours(open, closeAt string) []database.BusinessOperatingHours {
	return []database.BusinessOperatingHours{{DayOfWeek: 0, OpenTime: open, CloseTime: closeAt}}
}

// TestBuildDirectorLocalClockConvertsUTCIntoTheVenueClock is the RED proof for
// the reported label: 20:00 UTC must read 4:00 PM in New York, never 8:00 PM.
func TestBuildDirectorLocalClockConvertsUTCIntoTheVenueClock(t *testing.T) {
	now := newYorkAt(t, "2026-08-23T20:00:00Z")

	clock := buildDirectorLocalClock("America/New_York", nyLocation(t), sundayHours("11:00", "23:00"), now)

	assert.True(t, clock.TimezoneKnown)
	assert.Equal(t, "America/New_York", clock.Timezone)
	assert.Equal(t, "4:00 PM", clock.Clock, "20:00 UTC is 4 PM in New York; 8 PM is the raw UTC hour relabelled")
	assert.Equal(t, "-04:00", clock.UTCOffset)
	assert.Equal(t, "2026-08-23", clock.Date)
	assert.Equal(t, "Sunday", clock.Weekday)
	assert.Equal(t, "2026-08-23T16:00:00-04:00", clock.Now)
}

// TestBuildDirectorLocalClockReportsTodaysHoursAndOpenState covers the other
// half of the report: the Director said hours were unknown while guest Sage
// read them fine.
func TestBuildDirectorLocalClockReportsTodaysHoursAndOpenState(t *testing.T) {
	now := newYorkAt(t, "2026-08-23T20:00:00Z") // 16:00 local, inside 11:00–23:00

	clock := buildDirectorLocalClock("America/New_York", nyLocation(t), sundayHours("11:00", "23:00"), now)

	require.True(t, clock.HoursKnown, "the venue has hours configured for today")
	assert.False(t, clock.ClosedToday)
	assert.Equal(t, "11:00", clock.OpenTime)
	assert.Equal(t, "23:00", clock.CloseTime)
	assert.True(t, clock.OpenNow, "16:00 local sits inside 11:00–23:00")
}

func TestBuildDirectorLocalClockKnowsWhenTheVenueIsClosedRightNow(t *testing.T) {
	// 13:00 UTC = 09:00 EDT, two hours before the doors open.
	clock := buildDirectorLocalClock("America/New_York", nyLocation(t), sundayHours("11:00", "23:00"),
		newYorkAt(t, "2026-08-23T13:00:00Z"))

	require.True(t, clock.HoursKnown)
	assert.False(t, clock.OpenNow)
	assert.Equal(t, "9:00 AM", clock.Clock)
}

func TestBuildDirectorLocalClockSeparatesUnknownHoursFromClosed(t *testing.T) {
	now := newYorkAt(t, "2026-08-23T20:00:00Z")

	unknown := buildDirectorLocalClock("America/New_York", nyLocation(t), nil, now)
	assert.False(t, unknown.HoursKnown, "no row for today is 'not configured', not 'closed'")
	assert.False(t, unknown.ClosedToday)
	assert.False(t, unknown.OpenNow)

	closed := buildDirectorLocalClock("America/New_York", nyLocation(t),
		[]database.BusinessOperatingHours{{DayOfWeek: 0, IsClosed: true}}, now)
	assert.True(t, closed.HoursKnown)
	assert.True(t, closed.ClosedToday)
	assert.False(t, closed.OpenNow)
}

func TestDirectorWithinOperatingWindowHandlesOvernightBars(t *testing.T) {
	loc := nyLocation(t)
	at := func(hh, mm int) time.Time { return time.Date(2026, 8, 23, hh, mm, 0, 0, loc) }

	// A bar open 18:00 to 02:00 is open at 23:30 and at 00:30, shut at 15:00.
	assert.True(t, directorWithinOperatingWindow(at(23, 30), "18:00", "02:00"))
	assert.True(t, directorWithinOperatingWindow(at(0, 30), "18:00", "02:00"))
	assert.False(t, directorWithinOperatingWindow(at(15, 0), "18:00", "02:00"))

	// A normal daytime window stays exclusive at close.
	assert.True(t, directorWithinOperatingWindow(at(11, 0), "11:00", "23:00"))
	assert.False(t, directorWithinOperatingWindow(at(23, 0), "11:00", "23:00"))
	assert.False(t, directorWithinOperatingWindow(at(10, 59), "11:00", "23:00"))

	// Garbage clock strings must not fabricate an open venue.
	assert.False(t, directorWithinOperatingWindow(at(12, 0), "abc", "23:00"))
	assert.False(t, directorWithinOperatingWindow(at(12, 0), "11:00", ""))
}

func TestDirectorClockMinutesRejectsMalformedTimes(t *testing.T) {
	for _, bad := range []string{"", "11", "aa:00", "11:xx", "-1:00", "25:00", "11:60"} {
		_, ok := directorClockMinutes(bad)
		assert.False(t, ok, "%q must not parse", bad)
	}
	mins, ok := directorClockMinutes("11:30")
	require.True(t, ok)
	assert.Equal(t, 690, mins)
}

// TestDirectorLocalClockProseForbidsRestatingUTC is the prose-level guard: the
// grounding must both hand the model the local clock AND tell it not to
// re-derive one.
func TestDirectorLocalClockProseForbidsRestatingUTC(t *testing.T) {
	prose := directorLocalClockProse(buildDirectorLocalClock(
		"America/New_York", nyLocation(t), sundayHours("11:00", "23:00"),
		newYorkAt(t, "2026-08-23T20:00:00Z")))

	assert.Contains(t, prose, "4:00 PM")
	assert.NotContains(t, prose, "8:00 PM", "the UTC hour must never appear as the venue's clock")
	assert.Contains(t, prose, "America/New_York")
	assert.Contains(t, prose, "UTC-04:00")
	assert.Contains(t, prose, "11:00 to 23:00")
	assert.Contains(t, prose, "OPEN right now")
}

func TestDirectorLocalClockProseSaysHoursAreUnsetRatherThanGuessing(t *testing.T) {
	prose := directorLocalClockProse(buildDirectorLocalClock(
		"America/New_York", nyLocation(t), nil, newYorkAt(t, "2026-08-23T20:00:00Z")))

	assert.Contains(t, prose, "not configured")
	assert.NotContains(t, prose, "OPEN right now")
	assert.NotContains(t, prose, "CLOSED right now")
}

func TestDirectorLocalClockProseFlagsAnUnsetTimezone(t *testing.T) {
	prose := directorLocalClockProse(buildDirectorLocalClock("", time.UTC, nil, newYorkAt(t, "2026-08-23T20:00:00Z")))

	assert.Contains(t, prose, "no timezone set")
	assert.Contains(t, prose, "8:00 PM", "with no timezone the clock is honestly UTC")
}

// TestBuildContextGroundsTheVenueLocalClock drives the real buildContext: the
// venue clock and today's hours must reach the model, and generated_at must no
// longer be a bare UTC stamp.
func TestBuildContextGroundsTheVenueLocalClock(t *testing.T) {
	db := setupDirectorGroundingTestDB(t)
	require.NoError(t, db.GetGorm().AutoMigrate(&database.BusinessOperatingHours{}))
	service := NewDirectorConsoleService(db, analytics.NewAnalyticsService(db), nil, nil)

	business := database.Business{
		Name:            "Still Water Lounge",
		SettlementAddr:  "s-902-ctx",
		TippingAddr:     "t-902-ctx",
		DefaultCurrency: "USD",
		DisplayCurrency: "USD",
		Timezone:        "America/New_York",
	}
	require.NoError(t, db.GetGorm().Create(&business).Error)

	// Hours for every day so the assertion does not depend on when it runs.
	for day := 0; day < 7; day++ {
		require.NoError(t, db.GetGorm().Create(&database.BusinessOperatingHours{
			BusinessID: business.ID, DayOfWeek: day, OpenTime: "11:00", CloseTime: "23:00",
		}).Error)
	}

	ctx, err := service.buildContext(DirectorAskRequest{
		BusinessID: business.ID,
		Message:    "What time is it and are we open?",
		Locale:     "en",
	}, &business)
	require.NoError(t, err)

	nyNow := time.Now().In(nyLocation(t))
	assert.Equal(t, "America/New_York", ctx.LocalClock.Timezone)
	assert.True(t, ctx.LocalClock.TimezoneKnown)
	assert.Equal(t, nyNow.Format("2006-01-02"), ctx.LocalClock.Date, "the date must be the venue's local date")
	assert.Equal(t, nyNow.Format("3:04 PM"), ctx.LocalClock.Clock)
	require.True(t, ctx.LocalClock.HoursKnown, "hours are configured, so they must never read as unknown")
	assert.Equal(t, "11:00", ctx.LocalClock.OpenTime)
	assert.Equal(t, "23:00", ctx.LocalClock.CloseTime)
	assert.Contains(t, ctx.LocalClockSummary, "America/New_York")
	assert.Contains(t, ctx.LocalClockSummary, "11:00 to 23:00")

	// generated_at must be the venue's clock with its offset, not a Z stamp.
	assert.NotContains(t, ctx.GeneratedAt, "Z", "a bare UTC stamp is the thing the model misread as venue time")
	assert.Equal(t, nyNow.Format(time.RFC3339)[:13], ctx.GeneratedAt[:13])

	blob, err := json.Marshal(ctx)
	require.NoError(t, err)
	assert.Contains(t, string(blob), `"local_clock"`)
	assert.Contains(t, string(blob), `"open_time":"11:00"`)
}

// TestDirectorLocalClockGroundingSurvivesTheOutputGuard drives the real guard.
// Asserting only that the clock's key names are non-sensitive proves nothing on
// its own: the answer is scrubbed by guardDirectorOutput, so the venue clock has
// to be round-tripped through it. The clock is grounding, not customer PII, so
// none of it may come back redacted and the answer must not be discarded.
func TestDirectorLocalClockGroundingSurvivesTheOutputGuard(t *testing.T) {
	for _, key := range []string{"local_clock", "local_clock_summary", "open_time", "close_time", "utc_offset", "local_time", "periods"} {
		assert.False(t, directorKeyIsSensitive(key), "%q must not be scrubbed out of the Director payload", key)
	}

	clock := buildDirectorLocalClock("America/New_York", nyLocation(t), sundaySplitShiftHours(),
		newYorkAt(t, "2026-08-24T00:30:00Z"))
	payload := &directorContext{}
	payload.Business.ID = 7
	payload.LocalClock = clock
	payload.LocalClockSummary = directorLocalClockProse(clock)

	resp := DirectorStructuredResponse{
		Summary:   "It is 8:30 PM on Sunday at the venue (America/New_York, UTC-04:00).",
		Diagnosis: "Today runs 12:00 to 15:00 and 19:00 to 23:00 local, so you are open right now.",
		Evidence: []string{
			"Venue local time 2026-08-23T20:30:00-04:00",
			"Dinner service runs 19:00 to 23:00 local",
		},
		ActionPlan: []DirectorAction{{
			Title:       "Staff the 19:00 to 23:00 service",
			Description: "The venue stays open until 23:00 local.",
			DeepLink:    "/business/7/dashboard?tab=staff",
			Priority:    "medium",
		}},
		ExpectedImpact: "Covers the 19:00 to 23:00 window.",
		FollowUps:      []string{"Compare this with last Sunday's 12:00 to 15:00 service?"},
	}

	out := (&DirectorConsoleService{}).guardDirectorOutput(payload, 7, "en", resp)
	require.Len(t, out.ActionPlan, 1, "a clean, clock-grounded answer must not be discarded")

	blob := strings.Join([]string{
		out.Summary, out.Diagnosis, strings.Join(out.Evidence, " "),
		out.ActionPlan[0].Title, out.ActionPlan[0].Description,
		out.ExpectedImpact, strings.Join(out.FollowUps, " "),
	}, " ")
	assert.NotContains(t, blob, directorRedactionToken, "the venue clock is grounding, never customer PII")
	for _, fact := range []string{
		"8:30 PM", "America/New_York", "UTC-04:00",
		"12:00 to 15:00", "19:00 to 23:00", "2026-08-23T20:30:00-04:00",
	} {
		assert.Contains(t, blob, fact, "%q must survive the output guard", fact)
	}
}

// --- #902 repair: split shifts and unverified timezone names ---------------
//
// ValidateBusinessOperatingHours permits TWO rows per day, so a venue running
// lunch and dinner has two windows. Reading only the first row told an owner at
// 16:00 that the venue closed at 15:00 for the day, and an owner at 20:30 that
// the venue was CLOSED while dinner service was actually running.

func sundaySplitShiftHours() []database.BusinessOperatingHours {
	return []database.BusinessOperatingHours{
		{DayOfWeek: 0, OpenTime: "12:00", CloseTime: "15:00"},
		{DayOfWeek: 0, OpenTime: "19:00", CloseTime: "23:00"},
	}
}

func TestBuildDirectorLocalClockListsEveryOperatingPeriodForToday(t *testing.T) {
	// 20:00 UTC is 16:00 in New York — after lunch service, before dinner.
	clock := buildDirectorLocalClock("America/New_York", nyLocation(t), sundaySplitShiftHours(),
		newYorkAt(t, "2026-08-23T20:00:00Z"))

	require.True(t, clock.HoursKnown)
	assert.False(t, clock.ClosedToday)
	assert.False(t, clock.OpenNow, "16:00 falls between the two services")

	prose := directorLocalClockProse(clock)
	assert.Contains(t, prose, "12:00 to 15:00")
	assert.Contains(t, prose, "19:00 to 23:00", "dinner service must not be dropped from today's hours")
	assert.Contains(t, prose, "CLOSED right now")

	blob, err := json.Marshal(clock)
	require.NoError(t, err)
	assert.Contains(t, string(blob), "19:00", "the payload must carry every window, not just the first")
	assert.Contains(t, string(blob), "23:00")
}

func TestBuildDirectorLocalClockIsOpenDuringTheSecondServiceOfASplitShift(t *testing.T) {
	// 00:30 UTC on the 24th is 20:30 in New York on Sunday the 23rd — dinner.
	clock := buildDirectorLocalClock("America/New_York", nyLocation(t), sundaySplitShiftHours(),
		newYorkAt(t, "2026-08-24T00:30:00Z"))

	assert.Equal(t, "8:30 PM", clock.Clock)
	require.True(t, clock.HoursKnown)
	assert.False(t, clock.ClosedToday)
	assert.True(t, clock.OpenNow, "dinner service is running, so the venue is OPEN")

	prose := directorLocalClockProse(clock)
	assert.Contains(t, prose, "19:00 to 23:00")
	assert.Contains(t, prose, "OPEN right now")
	assert.NotContains(t, prose, "CLOSED right now")
}

func TestBuildDirectorLocalClockKeepsSingleWindowVenuesUnchanged(t *testing.T) {
	clock := buildDirectorLocalClock("America/New_York", nyLocation(t), sundayHours("11:00", "23:00"),
		newYorkAt(t, "2026-08-23T20:00:00Z"))

	assert.Equal(t, "11:00", clock.OpenTime)
	assert.Equal(t, "23:00", clock.CloseTime)
	assert.True(t, clock.OpenNow)
	assert.Contains(t, directorLocalClockProse(clock),
		"Hours today: 11:00 to 23:00 local, so the venue is OPEN right now.",
		"a single-window venue must read exactly as it did before split shifts were handled")
}

// Business.Timezone is operator-writable free text. ResolveBusinessLocation
// hands back UTC for a name tzdata does not know, so calling that clock
// "Mars/Olympus local time" is a UTC clock wearing a venue's name — exactly the
// four-hours-out failure #902 set out to remove.
func TestBuildDirectorLocalClockTreatsAnUnloadableTimezoneAsUnset(t *testing.T) {
	for _, tz := range []string{"Mars/Olympus", "GMT+3", "EST5EDT/nope", "America/Nueva_York"} {
		clock := buildDirectorLocalClock(tz, time.UTC, sundayHours("11:00", "23:00"),
			newYorkAt(t, "2026-08-23T20:00:00Z"))

		assert.Falsef(t, clock.TimezoneKnown, "%q is not a loadable IANA zone", tz)
		assert.NotEqualf(t, tz, clock.Timezone, "%q must never be presented as the venue's zone", tz)

		prose := directorLocalClockProse(clock)
		assert.Contains(t, prose, "no timezone set")
		assert.NotContains(t, prose, tz)
		assert.Contains(t, prose, "8:00 PM", "with no usable zone the clock is honestly UTC")
	}
}

func TestBuildDirectorLocalClockKeepsRealTimezonesKnown(t *testing.T) {
	for _, tz := range []string{"America/New_York", "America/Argentina/Buenos_Aires", "Europe/Madrid", "UTC"} {
		loc, err := time.LoadLocation(tz)
		require.NoError(t, err)
		clock := buildDirectorLocalClock(tz, loc, nil, newYorkAt(t, "2026-08-23T20:00:00Z"))
		assert.Truef(t, clock.TimezoneKnown, "%q is a real IANA zone", tz)
		assert.Equal(t, tz, clock.Timezone)
	}
}
