package services

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// #902: the Director's only clock was generated_at in UTC, next to a bare IANA
// timezone name. The model was left to do the conversion itself and did not:
// on the New York venue it read "20:00" off the UTC stamp and reported it as
// "8:00 PM NY" — four hours out — and, with no operating hours in the snapshot
// at all, told the owner the venue's hours were unknown while guest Sage on the
// same venue correctly answered "open 11:00–23:00 today".
//
// The fix is to stop asking the model to do timezone math: convert once, in Go,
// in the business timezone (the same rule analytics windows already follow) and
// ship the answer.

// directorLocalClock is the venue's own wall clock plus today's operating
// window, already resolved into the business timezone.
type directorLocalClock struct {
	Timezone string `json:"timezone"`
	// TimezoneKnown is false when the business has no timezone configured and
	// the clock below therefore fell back to UTC.
	TimezoneKnown bool `json:"timezone_known"`
	// Now is the venue-local timestamp with its offset, e.g.
	// "2026-08-23T15:59:00-04:00" — unambiguous even if quoted verbatim.
	Now       string `json:"now"`
	Clock     string `json:"clock"`   // "3:59 PM"
	Weekday   string `json:"weekday"` // "Saturday"
	Date      string `json:"date"`    // "2026-08-23"
	UTCOffset string `json:"utc_offset"`

	// HoursKnown is false when no operating-hours row exists for today.
	// "We do not know" and "we are closed" are different answers.
	HoursKnown  bool   `json:"hours_known"`
	ClosedToday bool   `json:"closed_today"`
	OpenTime    string `json:"open_time,omitempty"`
	CloseTime   string `json:"close_time,omitempty"`
	// Periods carries EVERY open window configured for today. A split-shift
	// venue (lunch 12:00-15:00 plus dinner 19:00-23:00) has two; OpenTime and
	// CloseTime above are only populated when there is exactly one, so the
	// model can never read a single pair as the whole day's schedule.
	Periods []directorHoursPeriod `json:"periods,omitempty"`
	OpenNow bool                  `json:"open_now"`
}

// directorHoursPeriod is one open service window for today.
type directorHoursPeriod struct {
	Open  string `json:"open"`
	Close string `json:"close"`
}

// buildDirectorLocalClock resolves now into loc and folds in today's hours.
//
// hours are today's operating rows; BuildWaiterHoursPeriods — the same helper
// the guest concierge reads through, so the host and the guest cannot disagree
// about the venue's schedule — picks today's open windows out of them. All of
// them: a split shift has two, and reporting only the first told a 16:00 owner
// the venue had shut for the day and a 20:30 owner it was closed mid-service.
func buildDirectorLocalClock(timezone string, loc *time.Location, hours []database.BusinessOperatingHours, now time.Time) directorLocalClock {
	if loc == nil {
		loc = time.UTC
	}
	local := now.In(loc)
	tz := strings.TrimSpace(timezone)

	clock := directorLocalClock{
		Timezone:      tz,
		TimezoneKnown: directorTimezoneIsKnown(tz),
		Now:           local.Format(time.RFC3339),
		Clock:         local.Format("3:04 PM"),
		Weekday:       local.Weekday().String(),
		Date:          local.Format("2006-01-02"),
		UTCOffset:     local.Format("-07:00"),
	}
	if !clock.TimezoneKnown {
		clock.Timezone = "UTC"
	}

	periods, closedToday, known := BuildWaiterHoursPeriods(hours, tz, now)
	clock.HoursKnown = known
	clock.ClosedToday = closedToday
	if !known || closedToday || len(periods) == 0 {
		return clock
	}
	clock.Periods = make([]directorHoursPeriod, 0, len(periods))
	for _, period := range periods {
		clock.Periods = append(clock.Periods, directorHoursPeriod{Open: period.Open, Close: period.Close})
		if directorWithinOperatingWindow(local, period.Open, period.Close) {
			clock.OpenNow = true
		}
	}
	// The flat pair stays the whole answer only for a single-window day; a
	// split shift is described exclusively by Periods so no reader can quote
	// "12:00 to 15:00" as today's closing time.
	if len(clock.Periods) == 1 {
		clock.OpenTime = clock.Periods[0].Open
		clock.CloseTime = clock.Periods[0].Close
	}
	return clock
}

// directorTimezoneIsKnown reports whether tz names a zone tzdata can load.
// Business.Timezone is operator-writable free text: ResolveLocation (the
// repo's cached time.LoadLocation) falls back to UTC for a name like
// "Mars/Olympus", and presenting that UTC clock as "Mars/Olympus local time"
// is exactly the relabelled-UTC answer #902 set out to remove. A zone that
// resolves keeps its own name; anything else reads as unset.
func directorTimezoneIsKnown(tz string) bool {
	if tz == "" {
		return false
	}
	return database.ResolveLocation(tz).String() == tz
}

// directorWithinOperatingWindow reports whether the venue-local time falls in
// [open, close). A close time at or before the open time is an overnight
// window (a bar open 18:00–02:00), not an empty one.
func directorWithinOperatingWindow(local time.Time, open, closeAt string) bool {
	openMin, okOpen := directorClockMinutes(open)
	closeMin, okClose := directorClockMinutes(closeAt)
	if !okOpen || !okClose {
		return false
	}
	nowMin := local.Hour()*60 + local.Minute()
	if closeMin > openMin {
		return nowMin >= openMin && nowMin < closeMin
	}
	// Overnight: open until closeMin the next calendar day.
	return nowMin >= openMin || nowMin < closeMin
}

// directorClockMinutes parses a stored "HH:MM" operating time into minutes
// past midnight.
func directorClockMinutes(value string) (int, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) < 2 {
		return 0, false
	}
	hour, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || hour < 0 || hour > 24 {
		return 0, false
	}
	minute, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil || minute < 0 || minute > 59 {
		return 0, false
	}
	return hour*60 + minute, true
}

// directorLocalClockProse renders the clock as the instruction the model needs:
// this is the venue's time, do not re-derive it, and here is today's schedule.
func directorLocalClockProse(clock directorLocalClock) string {
	parts := make([]string, 0, 3)
	if clock.TimezoneKnown {
		parts = append(parts, fmt.Sprintf(
			"Local time at the venue right now: %s, %s %s (%s local time, UTC%s). Every clock time you state must be this venue's local time — never restate a UTC timestamp as a local one.",
			clock.Weekday, clock.Date, clock.Clock, clock.Timezone, clock.UTCOffset))
	} else {
		parts = append(parts, fmt.Sprintf(
			"This venue has no timezone set in Business settings, so the clock below is UTC: %s, %s %s. Say the timezone is unset rather than naming a city time.",
			clock.Weekday, clock.Date, clock.Clock))
	}

	switch {
	case !clock.HoursKnown:
		parts = append(parts, "Operating hours for today are not configured in Business settings, so say the hours are not set — never invent a schedule and never claim the venue is open or closed right now.")
	case clock.ClosedToday:
		parts = append(parts, "Hours today: the venue is scheduled CLOSED all day.")
	case clock.OpenNow:
		parts = append(parts, fmt.Sprintf("Hours today: %s local, so the venue is OPEN right now.", directorHoursPeriodList(clock)))
	default:
		parts = append(parts, fmt.Sprintf("Hours today: %s local, so the venue is CLOSED right now.", directorHoursPeriodList(clock)))
	}
	return strings.Join(parts, " ")
}

// directorHoursPeriodList renders today's windows as "12:00 to 15:00 and
// 19:00 to 23:00" so a split shift is stated in full rather than truncated to
// its first service.
func directorHoursPeriodList(clock directorLocalClock) string {
	windows := make([]string, 0, len(clock.Periods))
	for _, period := range clock.Periods {
		windows = append(windows, fmt.Sprintf("%s to %s", period.Open, period.Close))
	}
	if len(windows) == 0 {
		return fmt.Sprintf("%s to %s", clock.OpenTime, clock.CloseTime)
	}
	if len(windows) == 1 {
		return windows[0]
	}
	return strings.Join(windows[:len(windows)-1], ", ") + " and " + windows[len(windows)-1]
}
