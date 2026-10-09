// Package reporting owns the canonical money-window and recognized-revenue
// basis used by every money surface (GetSummary, analytics, P&L, cost health).
//
// Seam 1 of the production-readiness remediation: one window, one revenue
// number. Handlers must not re-derive period bounds or invent alternate
// denominators — they take a Window and divide by RevenueBasis.RecognizedRevenue
// (or declare a different denominator explicitly in the response).
package reporting

import (
	"fmt"
	"strings"
	"time"
)

// nowFunc is the clock used by period presets. Tests override it.
var nowFunc = time.Now

// Window is a resolved half-open [Start, End) in UTC, derived from
// business-local midnights. Every money surface takes a Window; no handler
// re-derives one.
type Window struct {
	Start time.Time
	End   time.Time
	Label string
	Loc   *time.Location
}

// RevenueBasis is the ONE recognized-revenue number for a window. Every
// percentage on a money screen divides by RecognizedRevenue or declares a
// different denominator explicitly in its response.
type RevenueBasis struct {
	RecognizedRevenue float64 // ex-tips, refund-net, both payment tables
	Tips              float64
	BilledTotal       float64
	CollectedTotal    float64
	TransactionCount  int
	RecipeMappedSales float64 // strictly ≤ RecognizedRevenue
	Currency          string
}

// RevenueComponents holds the already-fetched component totals that every
// money surface (GetSummary, analytics window, P&L Revenue line) maps into
// the single RevenueBasis. Full DB aggregation lives with the ledger callers;
// this package only constructs and validates the canonical shape.
type RevenueComponents struct {
	BusinessID        uint
	Window            Window
	RecognizedRevenue float64
	Tips              float64
	BilledTotal       float64
	CollectedTotal    float64
	TransactionCount  int
	RecipeMappedSales float64
	Currency          string
}

// ResolveWindow replaces the three prior period paths (analytics
// ParsePeriodWindow, accounting parseDateRange, costHealthHasCustomRange).
//
// Precedence (matching handlers/accounting.go:354–363):
//   - both start and end non-nil → custom half-open window, Label "custom"
//     (even when period is also set)
//   - otherwise period preset → today/day/yesterday/week/month/quarter/year
//   - only one of start/end set, empty period, or unknown period → error
//
// loc nil falls back to UTC. Start/End on the returned Window are always UTC.
// Optional serviceDayStartMinute (minutes after local midnight) shifts
// today/yesterday onto the venue service day; omitted or 0 keeps midnight.
func ResolveWindow(period string, start, end *time.Time, loc *time.Location, serviceDayStartMinute ...int) (Window, error) {
	return ResolveWindowAt(period, start, end, loc, nowFunc(), serviceDayStartMinute...)
}

// ResolveWindowAt is ResolveWindow with an explicit clock (analytics pins
// s.now in tests; production callers use ResolveWindow).
func ResolveWindowAt(period string, start, end *time.Time, loc *time.Location, now time.Time, serviceDayStartMinute ...int) (Window, error) {
	loc = locOrUTC(loc)
	period = strings.TrimSpace(strings.ToLower(period))
	dayStartMin := 0
	if len(serviceDayStartMinute) > 0 {
		dayStartMin = serviceDayStartMinute[0]
	}

	hasStart := start != nil
	hasEnd := end != nil
	if hasStart != hasEnd {
		return Window{}, fmt.Errorf("reporting: custom range requires both start and end")
	}
	if hasStart && hasEnd {
		if end.Before(*start) {
			return Window{}, fmt.Errorf("reporting: end must be on or after start")
		}
		return Window{
			Start: start.UTC(),
			End:   end.UTC(),
			Label: "custom",
			Loc:   loc,
		}, nil
	}

	if period == "" {
		return Window{}, fmt.Errorf("reporting: period or custom start/end is required")
	}

	now = now.In(loc)
	label := period

	var winStart, winEnd time.Time
	switch period {
	case "today", "day":
		// "day" is the foodcost/director alias for the business service day.
		winStart = serviceDayStart(now, loc, dayStartMin)
		winEnd = winStart.AddDate(0, 0, 1) // next service-day boundary (DST-safe)
	case "yesterday":
		todayStart := serviceDayStart(now, loc, dayStartMin)
		winStart = todayStart.AddDate(0, 0, -1)
		winEnd = todayStart
	case "week":
		// ISO-style calendar week: Monday 00:00 local → now.
		winEnd = now
		winStart = calendarWeekStart(now, loc)
	case "7d", "last_7_days", "last7d":
		// Rolling last 7 local calendar days inclusive of today
		// (today-6 00:00 → now). Distinct from ISO week-to-date.
		label = "7d"
		winEnd = now
		todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		winStart = todayStart.AddDate(0, 0, -6)
	case "month":
		winEnd = now
		winStart = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	case "quarter":
		winEnd = now
		winStart = calendarQuarterStart(now, loc)
	case "year":
		winEnd = now
		winStart = time.Date(now.Year(), time.January, 1, 0, 0, 0, 0, loc)
	default:
		return Window{}, fmt.Errorf("reporting: unsupported period %q", period)
	}

	return Window{
		Start: winStart.UTC(),
		End:   winEnd.UTC(),
		Label: label,
		Loc:   loc,
	}, nil
}

// serviceDayStart returns the start of the venue service day containing now.
// startMinute 0 is local midnight; 240 is a 04:00 cutoff.
func serviceDayStart(now time.Time, loc *time.Location, startMinute int) time.Time {
	if startMinute < 0 || startMinute >= 24*60 {
		startMinute = 0
	}
	local := now.In(loc)
	hour := startMinute / 60
	min := startMinute % 60
	start := time.Date(local.Year(), local.Month(), local.Day(), hour, min, 0, 0, loc)
	if local.Before(start) {
		start = start.AddDate(0, 0, -1)
	}
	return start
}

// NewRevenueBasis constructs the canonical recognized-revenue number from
// component totals already fetched by a caller. GetSummary-derived revenue,
// analytics window revenue, and the P&L Revenue line must all feed the same
// components for a given (businessID, window) so every surface shares one
// RecognizedRevenue. RecipeMappedSales must be ≤ RecognizedRevenue.
func NewRevenueBasis(c RevenueComponents) (RevenueBasis, error) {
	if c.RecipeMappedSales > c.RecognizedRevenue {
		return RevenueBasis{}, fmt.Errorf(
			"reporting: RecipeMappedSales (%.4f) must be ≤ RecognizedRevenue (%.4f)",
			c.RecipeMappedSales, c.RecognizedRevenue,
		)
	}
	return RevenueBasis{
		RecognizedRevenue: c.RecognizedRevenue,
		Tips:              c.Tips,
		BilledTotal:       c.BilledTotal,
		CollectedTotal:    c.CollectedTotal,
		TransactionCount:  c.TransactionCount,
		RecipeMappedSales: c.RecipeMappedSales,
		Currency:          c.Currency,
	}, nil
}

// PriorWindow returns the immediately preceding calendar period of the same
// shape as w. Preset labels (year/month/quarter/week/today/day/yesterday) shift
// by the matching calendar unit via AddDate so MTD/YTD baselines land on the
// prior unit's same wall-clock span ("vs año previo"), not start.Add(-duration)
// which skews mid-period for incomplete windows and across DST. Custom ranges
// fall back to an equal local-calendar-day window ending at w.Start.
func PriorWindow(w Window) Window {
	loc := locOrUTC(w.Loc)
	startLocal := w.Start.In(loc)
	endLocal := w.End.In(loc)

	var prevStart, prevEnd time.Time
	switch strings.ToLower(strings.TrimSpace(w.Label)) {
	case "year":
		prevStart = startLocal.AddDate(-1, 0, 0)
		prevEnd = endLocal.AddDate(-1, 0, 0)
	case "quarter":
		prevStart = startLocal.AddDate(0, -3, 0)
		prevEnd = endLocal.AddDate(0, -3, 0)
	case "month":
		prevStart = startLocal.AddDate(0, -1, 0)
		prevEnd = endLocal.AddDate(0, -1, 0)
	case "week":
		prevStart = startLocal.AddDate(0, 0, -7)
		prevEnd = endLocal.AddDate(0, 0, -7)
	case "7d", "last_7_days", "last7d":
		prevStart = startLocal.AddDate(0, 0, -7)
		prevEnd = endLocal.AddDate(0, 0, -7)
	case "today", "day", "yesterday":
		prevStart = startLocal.AddDate(0, 0, -1)
		prevEnd = endLocal.AddDate(0, 0, -1)
	default:
		// custom / unknown: equal local calendar-day length ending at start
		days := calendarDaysBetweenLocal(startLocal, endLocal)
		if days < 1 {
			days = 1
		}
		dayStart := time.Date(startLocal.Year(), startLocal.Month(), startLocal.Day(), 0, 0, 0, 0, loc)
		prevStart = dayStart.AddDate(0, 0, -days)
		prevEnd = startLocal
	}

	// Month-end normalization guard: AddDate on an End of day 29–31 can
	// normalize forward (Mar 31 → "Feb 31" → Mar 3), which would make the prior
	// window overlap the current window's first days and contaminate the growth
	// baseline. Clamp the shifted prior End to the current window's Start.
	if prevEnd.After(startLocal) {
		prevEnd = startLocal
	}

	return Window{
		Start: prevStart.UTC(),
		End:   prevEnd.UTC(),
		Label: "prior:" + w.Label,
		Loc:   loc,
	}
}

// calendarDaysBetweenLocal counts local calendar days from a's date to b's date.
func calendarDaysBetweenLocal(a, b time.Time) int {
	au := time.Date(a.Year(), a.Month(), a.Day(), 0, 0, 0, 0, time.UTC)
	bu := time.Date(b.Year(), b.Month(), b.Day(), 0, 0, 0, 0, time.UTC)
	return int(bu.Sub(au).Hours() / 24)
}

// calendarWeekStart returns Monday 00:00 of the week containing t, in loc.
func calendarWeekStart(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	offset := (int(local.Weekday()) + 6) % 7 // Monday=0 … Sunday=6
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	return day.AddDate(0, 0, -offset)
}

// calendarQuarterStart returns the first day 00:00 of the calendar quarter
// containing t, in loc (Jan/Apr/Jul/Oct).
func calendarQuarterStart(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	qMonth := time.Month((int(local.Month())-1)/3*3 + 1)
	return time.Date(local.Year(), qMonth, 1, 0, 0, 0, 0, loc)
}

func locOrUTC(loc *time.Location) *time.Location {
	if loc == nil {
		return time.UTC
	}
	return loc
}
