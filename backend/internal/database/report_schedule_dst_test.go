package database

import (
	"testing"
	"time"
)

// Across a DST transition the report must still fire at the configured
// wall-clock hour. Computing next-run by adding fixed 24h durations drifts the
// send by an hour; computing it on the calendar date in the business timezone
// (letting the tz database absorb the offset change) keeps the hour stable.
func TestCalculateNextSendTimeStableAcrossDST(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	// 2026 US DST springs forward on Sunday, March 8 (02:00 -> 03:00).
	// "Now" is Saturday March 7 at 10:00 local, report scheduled daily at 09:00.
	now := time.Date(2026, time.March, 7, 10, 0, 0, 0, loc)

	sched := &ReportSchedule{
		Frequency: ReportFrequencyDaily,
		Hour:      9,
		Minute:    0,
		Timezone:  "America/New_York",
	}

	next := nextSendTimeFrom(sched, now)

	if got := next.Hour(); got != 9 {
		t.Errorf("expected send hour 9 after spring-forward, got %d (%s)", got, next)
	}
	if next.Day() != 8 || next.Month() != time.March {
		t.Errorf("expected next send on March 8, got %s", next)
	}
	// Sanity: it must be strictly after now.
	if !next.After(now) {
		t.Errorf("next send %s must be after now %s", next, now)
	}
}

// Weekly schedules must also hold their wall-clock hour across a fall-back
// boundary (Nov 1, 2026: 02:00 -> 01:00).
func TestCalculateNextSendTimeWeeklyStableAcrossFallBack(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	// Thursday Oct 29, 2026 at 12:00; weekly report Sundays (0) at 08:00. The
	// next Sunday is Nov 1, the fall-back day.
	now := time.Date(2026, time.October, 29, 12, 0, 0, 0, loc)

	sched := &ReportSchedule{
		Frequency: ReportFrequencyWeekly,
		DayOfWeek: 0,
		Hour:      8,
		Minute:    0,
		Timezone:  "America/New_York",
	}

	next := nextSendTimeFrom(sched, now)

	if got := next.Hour(); got != 8 {
		t.Errorf("expected send hour 8 across fall-back, got %d (%s)", got, next)
	}
	if next.Weekday() != time.Sunday {
		t.Errorf("expected next send on Sunday, got %s (%s)", next.Weekday(), next)
	}
	if next.Day() != 1 || next.Month() != time.November {
		t.Errorf("expected next send on Nov 1, got %s", next)
	}
}

func TestCalculateNextSendTimeSpringForwardGapNeverSendsEarly(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	// 02:30 does not exist on this date. The deterministic policy is to move it
	// through the one-hour gap to 03:30, never backwards to 01:30.
	now := time.Date(2026, time.March, 7, 12, 0, 0, 0, loc)
	next := nextSendTimeFrom(&ReportSchedule{
		Frequency: ReportFrequencyDaily,
		Hour:      2,
		Minute:    30,
		Timezone:  loc.String(),
	}, now)
	if next.Day() != 8 || next.Hour() != 3 || next.Minute() != 30 {
		t.Fatalf("nonexistent 02:30 must shift forward to 03:30, got %s", next)
	}
}

func TestCalculateNextSendTimeFallBackOverlapOccursOnce(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	// Go resolves the ambiguous 01:30 to the first occurrence. Once that instant
	// has passed, calculate the next calendar day rather than firing again in the
	// repeated hour.
	now := time.Date(2026, time.November, 1, 1, 45, 0, 0, loc).Add(time.Hour)
	next := nextSendTimeFrom(&ReportSchedule{
		Frequency: ReportFrequencyDaily,
		Hour:      1,
		Minute:    30,
		Timezone:  loc.String(),
	}, now)
	if next.Day() != 2 || next.Hour() != 1 || next.Minute() != 30 {
		t.Fatalf("ambiguous 01:30 must occur once, got %s", next)
	}
}
