package analytics

import (
	"testing"
	"time"
)

func TestParsePeriodWindow_MatchesParsePeriod(t *testing.T) {
	// Pin "now" so the wrapper and the underlying call read the same instant.
	// Without this the two clock reads race and the calendar week/month windows
	// differ by a sub-microsecond, flaking the equality assertion.
	fixed := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	svc := NewAnalyticsService(nil).WithClock(func() time.Time { return fixed })
	loc := time.UTC
	for _, p := range []string{"today", "week", "month"} {
		gotStart, gotEnd, err := svc.ParsePeriodWindow(p, loc)
		if err != nil {
			t.Fatalf("ParsePeriodWindow(%q) error: %v", p, err)
		}
		wantStart, wantEnd, _ := svc.parsePeriod(p, loc)
		if !gotStart.Equal(wantStart) || !gotEnd.Equal(wantEnd) {
			t.Fatalf("%q: got [%v,%v) want [%v,%v)", p, gotStart, gotEnd, wantStart, wantEnd)
		}
		if !gotEnd.After(gotStart) {
			t.Fatalf("%q: end must be after start", p)
		}
	}
	if _, _, err := svc.ParsePeriodWindow("nonsense", loc); err == nil {
		t.Fatal("expected error for unsupported period")
	}
}

// TestParsePeriod_CalendarWindowsBusinessTZ locks L6-1: week/month/quarter/year
// are business-local calendar periods to now, not rolling N-day windows.
// America/Argentina/Buenos_Aires is fixed UTC-3 (no DST) so wall-clock math is
// deterministic.
func TestParsePeriod_CalendarWindowsBusinessTZ(t *testing.T) {
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	// 2026-03-15 15:30 UTC = 2026-03-15 12:30 ART (Sunday).
	// Week starts Monday 2026-03-09 00:00 ART.
	// Month starts 2026-03-01 00:00 ART.
	// Quarter (Q1) starts 2026-01-01 00:00 ART.
	// Year starts 2026-01-01 00:00 ART.
	fixedUTC := time.Date(2026, 3, 15, 15, 30, 0, 0, time.UTC)
	svc := NewAnalyticsService(nil).WithClock(func() time.Time { return fixedUTC })
	nowLocal := fixedUTC.In(loc)

	cases := []struct {
		period    string
		wantStart time.Time
		wantEnd   time.Time
	}{
		{
			period:    "today",
			wantStart: time.Date(2026, 3, 15, 0, 0, 0, 0, loc),
			wantEnd:   time.Date(2026, 3, 16, 0, 0, 0, 0, loc),
		},
		{
			period:    "yesterday",
			wantStart: time.Date(2026, 3, 14, 0, 0, 0, 0, loc),
			wantEnd:   time.Date(2026, 3, 15, 0, 0, 0, 0, loc),
		},
		{
			// Sunday → calendar week from prior Monday 00:00 ART → now.
			period:    "week",
			wantStart: time.Date(2026, 3, 9, 0, 0, 0, 0, loc),
			wantEnd:   nowLocal,
		},
		{
			period:    "month",
			wantStart: time.Date(2026, 3, 1, 0, 0, 0, 0, loc),
			wantEnd:   nowLocal,
		},
		{
			period:    "quarter",
			wantStart: time.Date(2026, 1, 1, 0, 0, 0, 0, loc),
			wantEnd:   nowLocal,
		},
		{
			period:    "year",
			wantStart: time.Date(2026, 1, 1, 0, 0, 0, 0, loc),
			wantEnd:   nowLocal,
		},
	}

	for _, tc := range cases {
		t.Run(tc.period, func(t *testing.T) {
			start, end, err := svc.parsePeriod(tc.period, loc)
			if err != nil {
				t.Fatalf("parsePeriod(%q): %v", tc.period, err)
			}
			if !start.Equal(tc.wantStart) {
				t.Errorf("start = %v, want %v", start, tc.wantStart)
			}
			if !end.Equal(tc.wantEnd) {
				t.Errorf("end = %v, want %v", end, tc.wantEnd)
			}
			// Half-open: start must be before end.
			if !end.After(start) {
				t.Errorf("end must be after start: [%v, %v)", start, end)
			}
			// Must NOT be rolling: rolling week would start 7 days before now.
			if tc.period == "week" {
				rolling := nowLocal.AddDate(0, 0, -7)
				if start.Equal(rolling) {
					t.Errorf("week start is still rolling 7d (%v); want calendar Monday", rolling)
				}
			}
			if tc.period == "month" {
				rolling := nowLocal.AddDate(0, -1, 0)
				if start.Equal(rolling) {
					t.Errorf("month start is still rolling 1m (%v); want calendar day-1", rolling)
				}
			}
		})
	}
}

// TestParsePeriod_CalendarWeekMondayBoundary covers a mid-week instant and a
// Monday itself so the Monday-start math is not only tested from Sunday.
func TestParsePeriod_CalendarWeekMondayBoundary(t *testing.T) {
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	// Wednesday 2026-03-11 18:00 ART = 2026-03-11 21:00 UTC.
	// Calendar week [Mon 2026-03-09 00:00 ART, now).
	wedUTC := time.Date(2026, 3, 11, 21, 0, 0, 0, time.UTC)
	svc := NewAnalyticsService(nil).WithClock(func() time.Time { return wedUTC })
	start, end, err := svc.parsePeriod("week", loc)
	if err != nil {
		t.Fatalf("parsePeriod(week): %v", err)
	}
	wantStart := time.Date(2026, 3, 9, 0, 0, 0, 0, loc)
	wantEnd := wedUTC.In(loc)
	if !start.Equal(wantStart) || !end.Equal(wantEnd) {
		t.Fatalf("week mid-week: got [%v, %v) want [%v, %v)", start, end, wantStart, wantEnd)
	}

	// Monday morning: week start is that same local midnight.
	monUTC := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC) // 09:00 ART Monday
	svcMon := NewAnalyticsService(nil).WithClock(func() time.Time { return monUTC })
	startMon, endMon, err := svcMon.parsePeriod("week", loc)
	if err != nil {
		t.Fatalf("parsePeriod(week) Monday: %v", err)
	}
	wantMonStart := time.Date(2026, 3, 9, 0, 0, 0, 0, loc)
	if !startMon.Equal(wantMonStart) {
		t.Fatalf("Monday week start = %v, want %v", startMon, wantMonStart)
	}
	if !endMon.Equal(monUTC.In(loc)) {
		t.Fatalf("Monday week end = %v, want %v", endMon, monUTC.In(loc))
	}
}

// TestParsePeriod_CalendarQuarterBoundaries covers Q2/Q3/Q4 start months.
func TestParsePeriod_CalendarQuarterBoundaries(t *testing.T) {
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	cases := []struct {
		name      string
		fixedUTC  time.Time
		wantStart time.Time
	}{
		{
			name:      "Q2_May",
			fixedUTC:  time.Date(2026, 5, 20, 15, 0, 0, 0, time.UTC),
			wantStart: time.Date(2026, 4, 1, 0, 0, 0, 0, loc),
		},
		{
			name:      "Q3_July",
			fixedUTC:  time.Date(2026, 7, 1, 6, 0, 0, 0, time.UTC), // 03:00 ART July 1
			wantStart: time.Date(2026, 7, 1, 0, 0, 0, 0, loc),
		},
		{
			name:      "Q4_November",
			fixedUTC:  time.Date(2026, 11, 15, 18, 0, 0, 0, time.UTC),
			wantStart: time.Date(2026, 10, 1, 0, 0, 0, 0, loc),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewAnalyticsService(nil).WithClock(func() time.Time { return tc.fixedUTC })
			start, end, err := svc.parsePeriod("quarter", loc)
			if err != nil {
				t.Fatalf("parsePeriod(quarter): %v", err)
			}
			if !start.Equal(tc.wantStart) {
				t.Errorf("start = %v, want %v", start, tc.wantStart)
			}
			if !end.Equal(tc.fixedUTC.In(loc)) {
				t.Errorf("end = %v, want %v", end, tc.fixedUTC.In(loc))
			}
		})
	}
}
