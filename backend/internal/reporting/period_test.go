package reporting

import (
	"testing"
	"time"
)

// fixedNow is pinned so calendar presets (week/month/…) are deterministic.
var fixedNow = time.Date(2026, 3, 15, 15, 30, 0, 0, time.UTC)

func withFixedNow(t *testing.T) {
	t.Helper()
	prev := nowFunc
	nowFunc = func() time.Time { return fixedNow }
	t.Cleanup(func() { nowFunc = prev })
}

func TestResolveWindow_CustomRangeBeatsPreset(t *testing.T) {
	// Matching accounting.go:354–363: when both a period preset and an explicit
	// range are supplied, the range wins and Label is "custom".
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, loc)
	end := time.Date(2026, 3, 8, 0, 0, 0, 0, loc) // exclusive

	win, err := ResolveWindow("week", &start, &end, loc)
	if err != nil {
		t.Fatalf("ResolveWindow: %v", err)
	}
	if win.Label != "custom" {
		t.Fatalf("Label = %q, want custom", win.Label)
	}
	if !win.Start.Equal(start.UTC()) {
		t.Fatalf("Start = %v, want %v", win.Start, start.UTC())
	}
	if !win.End.Equal(end.UTC()) {
		t.Fatalf("End = %v, want %v", win.End, end.UTC())
	}
	if win.Loc != loc {
		t.Fatalf("Loc mismatch")
	}
	// Half-open window must live in UTC so handlers never re-derive.
	if win.Start.Location() != time.UTC || win.End.Location() != time.UTC {
		t.Fatalf("Start/End must be UTC, got %v / %v", win.Start.Location(), win.End.Location())
	}
}

func TestResolveWindow_BuenosAiresLocalMidnightBoundary(t *testing.T) {
	// Pin business-local midnight for America/Argentina/Buenos_Aires (UTC-3,
	// no current DST). A multi-day custom window must open at local midnight
	// and close at the next local midnight after the inclusive end date —
	// the same half-open contract parseDateRange uses.
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	// Inclusive operator range 2026-03-08 .. 2026-03-10 → half-open
	// [2026-03-08 00:00 ART, 2026-03-11 00:00 ART).
	// ART = UTC-3, so UTC instants are 03:00Z.
	startLocal := time.Date(2026, 3, 8, 0, 0, 0, 0, loc)
	endExclusiveLocal := time.Date(2026, 3, 11, 0, 0, 0, 0, loc)

	win, err := ResolveWindow("", &startLocal, &endExclusiveLocal, loc)
	if err != nil {
		t.Fatalf("ResolveWindow: %v", err)
	}

	wantStart := time.Date(2026, 3, 8, 3, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2026, 3, 11, 3, 0, 0, 0, time.UTC)
	if !win.Start.Equal(wantStart) {
		t.Fatalf("Start = %v, want %v (local midnight ART → 03:00Z)", win.Start, wantStart)
	}
	if !win.End.Equal(wantEnd) {
		t.Fatalf("End = %v, want %v (local next midnight ART → 03:00Z)", win.End, wantEnd)
	}
	if win.Label != "custom" {
		t.Fatalf("Label = %q, want custom", win.Label)
	}
}

func TestResolveWindow_DSTCrossingUSEastern(t *testing.T) {
	// Real DST spring-forward: America/New_York 2026-03-08 02:00 → 03:00.
	// Business-local midnights must still land on wall-clock midnight; the
	// half-open window length in UTC is 23h for the spring-forward day, not 24h.
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	// Inclusive day 2026-03-08 (spring forward) → [local midnight, next midnight).
	startLocal := time.Date(2026, 3, 8, 0, 0, 0, 0, loc)
	endLocal := time.Date(2026, 3, 9, 0, 0, 0, 0, loc)

	win, err := ResolveWindow("custom", &startLocal, &endLocal, loc)
	if err != nil {
		t.Fatalf("ResolveWindow: %v", err)
	}

	// EST is UTC-5; after spring-forward EDT is UTC-4.
	wantStart := time.Date(2026, 3, 8, 5, 0, 0, 0, time.UTC) // midnight EST
	wantEnd := time.Date(2026, 3, 9, 4, 0, 0, 0, time.UTC)   // midnight EDT
	if !win.Start.Equal(wantStart) {
		t.Fatalf("Start = %v, want %v", win.Start, wantStart)
	}
	if !win.End.Equal(wantEnd) {
		t.Fatalf("End = %v, want %v", win.End, wantEnd)
	}
	// Wall-clock day that springs forward is 23 hours in absolute time.
	if got := win.End.Sub(win.Start); got != 23*time.Hour {
		t.Fatalf("DST spring-forward day duration = %v, want 23h", got)
	}
}

func TestResolveWindow_7dIsRollingNotISOWeek(t *testing.T) {
	// Wednesday is mid-ISO-week so last-7-days start is not Monday.
	prev := nowFunc
	nowFunc = func() time.Time { return time.Date(2026, 3, 11, 18, 0, 0, 0, time.UTC) } // Wed 15:00 ART
	t.Cleanup(func() { nowFunc = prev })

	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	seven, err := ResolveWindow("7d", nil, nil, loc)
	if err != nil {
		t.Fatalf("7d: %v", err)
	}
	week, err := ResolveWindow("week", nil, nil, loc)
	if err != nil {
		t.Fatalf("week: %v", err)
	}
	if seven.Start.Equal(week.Start) {
		t.Fatalf("7d start %v collapsed to ISO week start %v", seven.Start, week.Start)
	}
	wantStart := time.Date(2026, 3, 5, 0, 0, 0, 0, loc).UTC() // today-6
	if !seven.Start.Equal(wantStart) {
		t.Fatalf("7d start = %v, want %v", seven.Start, wantStart)
	}
}

func TestResolveWindow_PeriodPresets(t *testing.T) {
	withFixedNow(t)
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	// fixedNow 2026-03-15 15:30 UTC = 2026-03-15 12:30 ART.
	nowLocal := fixedNow.In(loc)

	t.Run("today", func(t *testing.T) {
		win, err := ResolveWindow("today", nil, nil, loc)
		if err != nil {
			t.Fatalf("ResolveWindow: %v", err)
		}
		if win.Label != "today" {
			t.Fatalf("Label = %q, want today", win.Label)
		}
		wantStart := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc).UTC()
		wantEnd := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc).
			AddDate(0, 0, 1).UTC()
		if !win.Start.Equal(wantStart) || !win.End.Equal(wantEnd) {
			t.Fatalf("today: got [%v, %v) want [%v, %v)", win.Start, win.End, wantStart, wantEnd)
		}
	})

	t.Run("day_alias", func(t *testing.T) {
		// foodcost uses "day"; analytics uses "today". Canonical accepts both.
		win, err := ResolveWindow("day", nil, nil, loc)
		if err != nil {
			t.Fatalf("ResolveWindow: %v", err)
		}
		if win.Label != "day" {
			t.Fatalf("Label = %q, want day", win.Label)
		}
		today, err := ResolveWindow("today", nil, nil, loc)
		if err != nil {
			t.Fatalf("ResolveWindow today: %v", err)
		}
		if !win.Start.Equal(today.Start) || !win.End.Equal(today.End) {
			t.Fatalf("day must match today bounds")
		}
	})

	t.Run("week", func(t *testing.T) {
		win, err := ResolveWindow("week", nil, nil, loc)
		if err != nil {
			t.Fatalf("ResolveWindow: %v", err)
		}
		if win.Label != "week" {
			t.Fatalf("Label = %q, want week", win.Label)
		}
		// L6-1: calendar week Mon 00:00 business local → now (2026-03-15 is Sunday ART).
		wantEnd := fixedNow.In(loc)
		wantStart := time.Date(2026, 3, 9, 0, 0, 0, 0, loc)
		if !win.Start.Equal(wantStart.UTC()) || !win.End.Equal(wantEnd.UTC()) {
			t.Fatalf("week: got [%v, %v) want [%v, %v)", win.Start, win.End, wantStart.UTC(), wantEnd.UTC())
		}
		// Must not still be rolling 7d.
		if win.Start.Equal(wantEnd.AddDate(0, 0, -7).UTC()) {
			t.Fatalf("week start is still rolling 7d")
		}
	})

	t.Run("7d", func(t *testing.T) {
		win, err := ResolveWindow("7d", nil, nil, loc)
		if err != nil {
			t.Fatalf("ResolveWindow: %v", err)
		}
		if win.Label != "7d" {
			t.Fatalf("Label = %q, want 7d", win.Label)
		}
		wantEnd := fixedNow.In(loc)
		todayStart := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, loc)
		wantStart := todayStart.AddDate(0, 0, -6)
		if !win.Start.Equal(wantStart.UTC()) || !win.End.Equal(wantEnd.UTC()) {
			t.Fatalf("7d: got [%v, %v) want [%v, %v)", win.Start, win.End, wantStart.UTC(), wantEnd.UTC())
		}
	})

	t.Run("month", func(t *testing.T) {
		win, err := ResolveWindow("month", nil, nil, loc)
		if err != nil {
			t.Fatalf("ResolveWindow: %v", err)
		}
		wantStart := time.Date(2026, 3, 1, 0, 0, 0, 0, loc)
		wantEnd := fixedNow.In(loc)
		if !win.Start.Equal(wantStart.UTC()) || !win.End.Equal(wantEnd.UTC()) {
			t.Fatalf("month: got [%v, %v) want [%v, %v)", win.Start, win.End, wantStart.UTC(), wantEnd.UTC())
		}
	})

	t.Run("quarter", func(t *testing.T) {
		win, err := ResolveWindow("quarter", nil, nil, loc)
		if err != nil {
			t.Fatalf("ResolveWindow: %v", err)
		}
		// Q1: Jan 1 00:00 ART → now.
		wantStart := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
		wantEnd := fixedNow.In(loc)
		if !win.Start.Equal(wantStart.UTC()) || !win.End.Equal(wantEnd.UTC()) {
			t.Fatalf("quarter: got [%v, %v) want [%v, %v)", win.Start, win.End, wantStart.UTC(), wantEnd.UTC())
		}
	})

	t.Run("year", func(t *testing.T) {
		win, err := ResolveWindow("year", nil, nil, loc)
		if err != nil {
			t.Fatalf("ResolveWindow: %v", err)
		}
		wantStart := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
		wantEnd := fixedNow.In(loc)
		if !win.Start.Equal(wantStart.UTC()) || !win.End.Equal(wantEnd.UTC()) {
			t.Fatalf("year: got [%v, %v) want [%v, %v)", win.Start, win.End, wantStart.UTC(), wantEnd.UTC())
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		if _, err := ResolveWindow("nonsense", nil, nil, loc); err == nil {
			t.Fatal("expected error for unsupported period")
		}
	})
}

func TestResolveWindow_PartialCustomRangeErrors(t *testing.T) {
	loc := time.UTC
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	if _, err := ResolveWindow("week", &start, nil, loc); err == nil {
		t.Fatal("expected error when only start is set")
	}
	if _, err := ResolveWindow("week", nil, &start, loc); err == nil {
		t.Fatal("expected error when only end is set")
	}
	if _, err := ResolveWindow("", nil, nil, loc); err == nil {
		t.Fatal("expected error when neither period nor range is set")
	}
}

func TestResolveWindow_NilLocationFallsBackToUTC(t *testing.T) {
	withFixedNow(t)
	win, err := ResolveWindow("today", nil, nil, nil)
	if err != nil {
		t.Fatalf("ResolveWindow: %v", err)
	}
	if win.Loc != time.UTC {
		t.Fatalf("Loc = %v, want UTC", win.Loc)
	}
	wantStart := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	if !win.Start.Equal(wantStart) {
		t.Fatalf("Start = %v, want %v", win.Start, wantStart)
	}
}

// TestRevenueBasis_OneNumberAcrossSurfaces asserts that for one
// (businessID, start, end) the package yields exactly one RevenueBasis, and
// that GetSummary-derived revenue, analytics window revenue, and the P&L
// Revenue line all resolve to it. Full DB aggregation is out of package scope
// for Task 1 — surfaces feed component totals into NewRevenueBasis.
func TestRevenueBasis_OneNumberAcrossSurfaces(t *testing.T) {
	const businessID uint = 42
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, loc)
	end := time.Date(2026, 3, 8, 0, 0, 0, 0, loc)

	win, err := ResolveWindow("", &start, &end, loc)
	if err != nil {
		t.Fatalf("ResolveWindow: %v", err)
	}

	// Shared component totals that GetSummary, analytics, and P&L would each
	// derive for this business+window from the recognized-payment ledger.
	components := RevenueComponents{
		BusinessID:        businessID,
		Window:            win,
		RecognizedRevenue: 5600.00, // ex-tips, refund-net, both payment tables
		Tips:              420.00,
		BilledTotal:       6100.00,
		CollectedTotal:    5600.00,
		TransactionCount:  87,
		RecipeMappedSales: 4100.00, // strictly ≤ RecognizedRevenue
		Currency:          "ARS",
	}

	// Surface 1 — GetSummary: AutoIncomeTotal / BilledTotal / CollectedTotal.
	fromSummary, err := NewRevenueBasis(components)
	if err != nil {
		t.Fatalf("NewRevenueBasis(summary): %v", err)
	}

	// Surface 2 — analytics window: TotalRevenue / TotalTips for the same window.
	fromAnalytics, err := NewRevenueBasis(components)
	if err != nil {
		t.Fatalf("NewRevenueBasis(analytics): %v", err)
	}

	// Surface 3 — P&L Revenue line (ComposeProfitLoss maps AutoIncomeTotal → Revenue).
	fromPnL, err := NewRevenueBasis(components)
	if err != nil {
		t.Fatalf("NewRevenueBasis(pnl): %v", err)
	}

	// Exactly one recognized-revenue number for the window.
	if fromSummary.RecognizedRevenue != fromAnalytics.RecognizedRevenue ||
		fromSummary.RecognizedRevenue != fromPnL.RecognizedRevenue {
		t.Fatalf("surfaces disagree: summary=%v analytics=%v pnl=%v",
			fromSummary.RecognizedRevenue, fromAnalytics.RecognizedRevenue, fromPnL.RecognizedRevenue)
	}
	if fromSummary != fromAnalytics || fromSummary != fromPnL {
		t.Fatalf("RevenueBasis must be identical across GetSummary, analytics, and P&L; got\n  summary=%+v\n  analytics=%+v\n  pnl=%+v",
			fromSummary, fromAnalytics, fromPnL)
	}
	if fromSummary.RecognizedRevenue != 5600.00 {
		t.Fatalf("RecognizedRevenue = %v, want 5600", fromSummary.RecognizedRevenue)
	}
	if fromSummary.RecipeMappedSales > fromSummary.RecognizedRevenue {
		t.Fatalf("RecipeMappedSales %v must be ≤ RecognizedRevenue %v",
			fromSummary.RecipeMappedSales, fromSummary.RecognizedRevenue)
	}
	if fromSummary.Currency != "ARS" {
		t.Fatalf("Currency = %q, want ARS", fromSummary.Currency)
	}

	// A second call with the same inputs must return the same value (no hidden state).
	again, err := NewRevenueBasis(components)
	if err != nil {
		t.Fatalf("NewRevenueBasis again: %v", err)
	}
	if again != fromSummary {
		t.Fatalf("NewRevenueBasis is not pure: first=%+v second=%+v", fromSummary, again)
	}
}

func TestNewRevenueBasis_RejectsRecipeMappedAboveRecognized(t *testing.T) {
	win := Window{
		Start: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC),
		Label: "custom",
		Loc:   time.UTC,
	}
	_, err := NewRevenueBasis(RevenueComponents{
		BusinessID:        1,
		Window:            win,
		RecognizedRevenue: 100,
		RecipeMappedSales: 150,
		Currency:          "USD",
	})
	if err == nil {
		t.Fatal("expected error when RecipeMappedSales > RecognizedRevenue")
	}
}

func TestWindowAndRevenueBasisTypesExist(t *testing.T) {
	// Compile-time / shape guard so later tasks can depend on these fields.
	_ = Window{Start: time.Time{}, End: time.Time{}, Label: "", Loc: time.UTC}
	_ = RevenueBasis{
		RecognizedRevenue: 0,
		Tips:              0,
		BilledTotal:       0,
		CollectedTotal:    0,
		TransactionCount:  0,
		RecipeMappedSales: 0,
		Currency:          "",
	}
}

// TestPriorWindow_CalendarYearNotDurationShift locks L6-10: year growth baseline
// must be the prior calendar year of the same shape (shift by AddDate(-1)), not
// start.Add(-duration) which lands mid-previous-year for MTD windows.
func TestPriorWindow_CalendarYearNotDurationShift(t *testing.T) {
	loc := time.UTC
	// MTD-year window: [2026-01-01, 2026-05-20).
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	end := time.Date(2026, 5, 20, 12, 0, 0, 0, loc)
	w := Window{Start: start.UTC(), End: end.UTC(), Label: "year", Loc: loc}

	prior := PriorWindow(w)

	wantStart := time.Date(2025, 1, 1, 0, 0, 0, 0, loc).UTC()
	wantEnd := time.Date(2025, 5, 20, 12, 0, 0, 0, loc).UTC()
	if !prior.Start.Equal(wantStart) {
		t.Fatalf("PriorWindow year Start = %v, want %v (calendar prior year)", prior.Start, wantStart)
	}
	if !prior.End.Equal(wantEnd) {
		t.Fatalf("PriorWindow year End = %v, want %v", prior.End, wantEnd)
	}
	// Naive duration shift would put start at ~2025-08-14 — must not do that.
	naive := start.Add(-end.Sub(start))
	if prior.Start.Equal(naive.UTC()) {
		t.Fatalf("PriorWindow year used duration shift (%v); want calendar prior year", naive)
	}
}

func TestPriorWindow_CalendarMonthMTD(t *testing.T) {
	loc := time.UTC
	start := time.Date(2026, 5, 1, 0, 0, 0, 0, loc)
	end := time.Date(2026, 5, 20, 12, 0, 0, 0, loc)
	w := Window{Start: start.UTC(), End: end.UTC(), Label: "month", Loc: loc}

	prior := PriorWindow(w)
	wantStart := time.Date(2026, 4, 1, 0, 0, 0, 0, loc).UTC()
	wantEnd := time.Date(2026, 4, 20, 12, 0, 0, 0, loc).UTC()
	if !prior.Start.Equal(wantStart) || !prior.End.Equal(wantEnd) {
		t.Fatalf("PriorWindow month = [%v, %v), want [%v, %v)", prior.Start, prior.End, wantStart, wantEnd)
	}
}

// TestPriorWindow_MonthEndDoesNotOverlapCurrentWindow: for month/quarter
// windows whose End falls on day 29–31, AddDate normalizes forward
// (Mar 31 → "Feb 31" → Mar 3), so the naive prior window would OVERLAP the
// current window's first days and contaminate the growth baseline. The shifted
// prior End must clamp to the current window's Start.
func TestPriorWindow_MonthEndDoesNotOverlapCurrentWindow(t *testing.T) {
	loc := time.UTC
	// Full March MTD window ending on the 31st: [2026-03-01, 2026-03-31 15:00).
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, loc)
	end := time.Date(2026, 3, 31, 15, 0, 0, 0, loc)
	w := Window{Start: start.UTC(), End: end.UTC(), Label: "month", Loc: loc}

	prior := PriorWindow(w)

	wantStart := time.Date(2026, 2, 1, 0, 0, 0, 0, loc).UTC()
	if !prior.Start.Equal(wantStart) {
		t.Fatalf("PriorWindow month Start = %v, want %v", prior.Start, wantStart)
	}
	if prior.End.After(w.Start) {
		t.Fatalf("PriorWindow month End = %v overlaps current window start %v; must clamp to start", prior.End, w.Start)
	}
	// Clamped exactly to the current window's start (Feb has no day 31).
	if !prior.End.Equal(w.Start) {
		t.Fatalf("PriorWindow month End = %v, want clamp to current start %v", prior.End, w.Start)
	}
}

// Quarter shape: End on day 31 with a short landing month must also never
// cross into the current window.
func TestPriorWindow_QuarterEndDoesNotOverlapCurrentWindow(t *testing.T) {
	loc := time.UTC
	// Q2 QTD window ending May 31: [2026-04-01, 2026-05-31 12:00).
	start := time.Date(2026, 4, 1, 0, 0, 0, 0, loc)
	end := time.Date(2026, 5, 31, 12, 0, 0, 0, loc)
	w := Window{Start: start.UTC(), End: end.UTC(), Label: "quarter", Loc: loc}

	prior := PriorWindow(w)

	wantStart := time.Date(2026, 1, 1, 0, 0, 0, 0, loc).UTC()
	if !prior.Start.Equal(wantStart) {
		t.Fatalf("PriorWindow quarter Start = %v, want %v", prior.Start, wantStart)
	}
	if prior.End.After(w.Start) {
		t.Fatalf("PriorWindow quarter End = %v overlaps current window start %v", prior.End, w.Start)
	}
}

func TestPriorWindow_TodayIsPreviousCalendarDay(t *testing.T) {
	loc := time.UTC
	start := time.Date(2026, 5, 20, 0, 0, 0, 0, loc)
	end := time.Date(2026, 5, 21, 0, 0, 0, 0, loc)
	w := Window{Start: start.UTC(), End: end.UTC(), Label: "today", Loc: loc}
	prior := PriorWindow(w)
	wantStart := time.Date(2026, 5, 19, 0, 0, 0, 0, loc).UTC()
	wantEnd := time.Date(2026, 5, 20, 0, 0, 0, 0, loc).UTC()
	if !prior.Start.Equal(wantStart) || !prior.End.Equal(wantEnd) {
		t.Fatalf("PriorWindow today = [%v, %v), want [%v, %v)", prior.Start, prior.End, wantStart, wantEnd)
	}
}

func TestResolveWindowAt_ServiceDayCutoff(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	// 01:30 local with a 04:00 service-day start → still "yesterday".
	now := time.Date(2026, 8, 12, 1, 30, 0, 0, loc)
	win, err := ResolveWindowAt("today", nil, nil, loc, now, 240)
	if err != nil {
		t.Fatalf("ResolveWindowAt: %v", err)
	}
	wantStart := time.Date(2026, 8, 11, 4, 0, 0, 0, loc).UTC()
	wantEnd := time.Date(2026, 8, 12, 4, 0, 0, 0, loc).UTC()
	if !win.Start.Equal(wantStart) || !win.End.Equal(wantEnd) {
		t.Fatalf("service-day today = [%v, %v), want [%v, %v)", win.Start, win.End, wantStart, wantEnd)
	}

	yest, err := ResolveWindowAt("yesterday", nil, nil, loc, now, 240)
	if err != nil {
		t.Fatalf("ResolveWindowAt yesterday: %v", err)
	}
	wantYStart := time.Date(2026, 8, 10, 4, 0, 0, 0, loc).UTC()
	if !yest.Start.Equal(wantYStart) || !yest.End.Equal(wantStart) {
		t.Fatalf("service-day yesterday = [%v, %v), want [%v, %v)", yest.Start, yest.End, wantYStart, wantStart)
	}
}
