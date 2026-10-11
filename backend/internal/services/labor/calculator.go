// Package labor computes a read-only labor-cost-as-%-of-sales lens by prorating
// paid-payroll actuals over a window ÷ net sales ex-tips. Persists nothing; does
// not change the accounting P&L — labor cost is a display metric, not a deduction.
package labor

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"
)

// ErrUnsupportedPeriod marks an invalid period (client error → 400).
var ErrUnsupportedPeriod = errors.New("labor: unsupported period")

type PayrollProvider interface {
	GetPaidPayrollRunsOverlapping(businessID uint, start, end time.Time) ([]database.PayrollRunCost, error)
}
type SalesProvider interface {
	GetPaymentWindowSummary(businessID uint, startDate, endDate time.Time) (*analytics.PaymentWindowSummary, error)
}
type WindowProvider interface {
	ParsePeriodWindow(period string, loc *time.Location) (time.Time, time.Time, error)
}

// WorkedHoursProvider backs the labor-cost ACTUALS basis (Slice 4): the SUM of
// approved time-clock worked-minutes per staff over a window, each carrying that
// staff's primary StaffPosition pay rate (owner-only). The DB implements it as a
// single SQL aggregate; the return type lives in the database package so this
// package (which already imports database) can name it with no cycle — mirroring
// PayrollProvider returning []database.PayrollRunCost.
type WorkedHoursProvider interface {
	GetApprovedWorkedMinutes(businessID uint, start, end time.Time) ([]database.StaffWorkedMinutes, error)
}

type RunContribution struct {
	PayrollRunID    uint      `json:"payroll_run_id"`
	PeriodStart     time.Time `json:"period_start"`
	PeriodEnd       time.Time `json:"period_end"`
	OverlapFraction float64   `json:"overlap_fraction"`
	LaborCost       float64   `json:"labor_cost"`
}

// StaffActualContribution is one staff member's slice of the ACTUALS basis: the
// approved worked minutes/hours and the labor-$ they imply at their primary pay
// rate. The dollar figure is owner/financial:read-gated in the handler — never
// surfaced to staff.
type StaffActualContribution struct {
	StaffID       uint    `json:"staff_id"`
	WorkedMinutes int     `json:"worked_minutes"`
	WorkedHours   float64 `json:"worked_hours"`
	LaborCost     float64 `json:"labor_cost"`
}

type Report struct {
	Period          string            `json:"period"`
	LaborCost       float64           `json:"labor_cost"`
	NetSales        float64           `json:"net_sales"`
	LaborCostPct    float64           `json:"labor_cost_pct"`
	PayrollRunCount int               `json:"payroll_run_count"`
	HasData         bool              `json:"has_data"`
	Contributions   []RunContribution `json:"contributions"`
	WindowStart     time.Time         `json:"window_start"`
	WindowEnd       time.Time         `json:"window_end"`

	// ACTUALS-basis fields (Slice 4). Populated only by AnalyzeActual; the
	// payroll-basis Analyze leaves them zero/empty (omitempty keeps its JSON
	// byte-compatible).
	Basis              string                    `json:"basis,omitempty"`
	WorkedHours        float64                   `json:"worked_hours,omitempty"`
	StaffContributions []StaffActualContribution `json:"staff_contributions,omitempty"`
}

type Calculator struct {
	payroll PayrollProvider
	sales   SalesProvider
	window  WindowProvider
	worked  WorkedHoursProvider
}

func NewCalculator(payroll PayrollProvider, sales SalesProvider, window WindowProvider) *Calculator {
	return &Calculator{payroll: payroll, sales: sales, window: window}
}

// WithWorkedHours attaches the WorkedHoursProvider the ACTUALS basis needs and
// returns the calculator for chaining. Additive: the existing payroll-basis
// Analyze ignores it, so callers that only need Analyze are unaffected.
func (c *Calculator) WithWorkedHours(w WorkedHoursProvider) *Calculator {
	c.worked = w
	return c
}

func (c *Calculator) Analyze(businessID uint, period string, loc *time.Location) (Report, error) {
	switch period {
	case "":
		period = "week"
	case "week", "month":
	default:
		return Report{}, fmt.Errorf("%w %q (allowed: week, month)", ErrUnsupportedPeriod, period)
	}

	start, end, err := c.window.ParsePeriodWindow(period, loc)
	if err != nil {
		return Report{}, fmt.Errorf("labor: window: %w", err)
	}
	return c.AnalyzeWindow(businessID, start, end, period)
}

// AnalyzeWindow computes the payroll-basis labor report for an explicit
// half-open [start, end) window (accounting Overview date range). periodLabel
// is echoed in Report.Period (use "custom" when none).
func (c *Calculator) AnalyzeWindow(businessID uint, start, end time.Time, periodLabel string) (Report, error) {
	if !end.After(start) {
		return Report{}, fmt.Errorf("%w: end must be after start", ErrUnsupportedPeriod)
	}
	if periodLabel == "" {
		periodLabel = "custom"
	}
	runs, err := c.payroll.GetPaidPayrollRunsOverlapping(businessID, start, end)
	if err != nil {
		return Report{}, fmt.Errorf("labor: payroll runs: %w", err)
	}
	summary, err := c.sales.GetPaymentWindowSummary(businessID, start, end)
	if err != nil {
		return Report{}, fmt.Errorf("labor: sales: %w", err)
	}

	report := Report{Period: periodLabel, WindowStart: start, WindowEnd: end}
	report.Contributions = []RunContribution{}
	for _, r := range runs {
		dur := r.PeriodEnd.Sub(r.PeriodStart)
		if dur <= 0 {
			continue // defensive: malformed run period
		}
		ov := minTime(end, r.PeriodEnd).Sub(maxTime(start, r.PeriodStart))
		if ov <= 0 {
			continue
		}
		frac := float64(ov) / float64(dur)
		if frac > 1 {
			// defensive: ov is a subinterval of [PeriodStart,PeriodEnd] so frac<=1
			// in exact arithmetic; this guards only against float rounding.
			frac = 1
		}
		runCost := float64(r.GrossTotal+r.BonusTotal) / 100
		contrib := runCost * frac
		report.LaborCost += contrib
		report.Contributions = append(report.Contributions, RunContribution{
			PayrollRunID: r.ID, PeriodStart: r.PeriodStart, PeriodEnd: r.PeriodEnd,
			OverlapFraction: frac, LaborCost: contrib,
		})
	}
	report.PayrollRunCount = len(report.Contributions)
	report.HasData = report.PayrollRunCount > 0
	if summary != nil {
		report.NetSales = summary.TotalRevenue
	}
	if report.NetSales > 0 {
		report.LaborCostPct = report.LaborCost / report.NetSales
	}
	sort.SliceStable(report.Contributions, func(i, j int) bool {
		a, b := report.Contributions[i], report.Contributions[j]
		if a.LaborCost != b.LaborCost {
			return a.LaborCost > b.LaborCost
		}
		return a.PeriodStart.After(b.PeriodStart)
	})
	return report, nil
}

// AnalyzeActual computes the labor-cost ACTUALS basis: laborCost = Σ over staff
// (approved workedMinutes/60 × that staff's primary StaffPosition pay rate);
// LaborCostPct = laborCost / NetSales. It reuses the same window + sales
// providers as Analyze, so the numerator and denominator share the window. It is
// purely a display lens — worked hours NEVER auto-write a PayrollLineItem. The
// existing Analyze/Report/Calculator are untouched; this is additive.
func (c *Calculator) AnalyzeActual(businessID uint, period string, loc *time.Location) (Report, error) {
	switch period {
	case "":
		period = "week"
	case "week", "month":
	default:
		return Report{}, fmt.Errorf("%w %q (allowed: week, month)", ErrUnsupportedPeriod, period)
	}
	if c.worked == nil {
		return Report{}, errors.New("labor: worked-hours provider not configured (call WithWorkedHours)")
	}

	start, end, err := c.window.ParsePeriodWindow(period, loc)
	if err != nil {
		return Report{}, fmt.Errorf("labor: window: %w", err)
	}
	rows, err := c.worked.GetApprovedWorkedMinutes(businessID, start, end)
	if err != nil {
		return Report{}, fmt.Errorf("labor: worked minutes: %w", err)
	}
	summary, err := c.sales.GetPaymentWindowSummary(businessID, start, end)
	if err != nil {
		return Report{}, fmt.Errorf("labor: sales: %w", err)
	}

	report := Report{Period: period, Basis: "actual", WindowStart: start, WindowEnd: end}
	report.Contributions = []RunContribution{}
	report.StaffContributions = []StaffActualContribution{}
	for _, r := range rows {
		mins := r.Minutes
		if mins < 0 {
			mins = 0 // defensive: validated writes never produce negative worked minutes
		}
		hours := float64(mins) / 60
		cost := hours * float64(r.RateCents) / 100
		report.LaborCost += cost
		report.WorkedHours += hours
		report.StaffContributions = append(report.StaffContributions, StaffActualContribution{
			StaffID: r.StaffID, WorkedMinutes: mins, WorkedHours: hours, LaborCost: cost,
		})
	}
	report.HasData = len(report.StaffContributions) > 0
	if summary != nil {
		report.NetSales = summary.TotalRevenue
	}
	if report.NetSales > 0 {
		report.LaborCostPct = report.LaborCost / report.NetSales
	}
	sort.SliceStable(report.StaffContributions, func(i, j int) bool {
		a, b := report.StaffContributions[i], report.StaffContributions[j]
		if a.LaborCost != b.LaborCost {
			return a.LaborCost > b.LaborCost
		}
		return a.StaffID < b.StaffID
	})
	return report, nil
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// ---- Slice 6: build-time scheduled-labor preview (display-only; no payroll write) ----
// Deliberately a SEPARATE type + provider set so the paid-payroll Analyze path,
// its Calculator, Report, and call site (accounting.go) stay byte-for-byte intact.

const (
	WarnOvertime   = "overtime"
	WarnPostedLate = "posted_late"
	WarnMinorLate  = "minor_late"
)

type ScheduledLaborProvider interface {
	GetScheduleShiftLines(businessID, scheduleID uint) ([]database.ScheduleShiftLine, error)
	GetStaffPositionRates(businessID uint) (map[database.StaffPositionKey]int64, error)
}
type ScheduleSettingsProvider interface {
	GetOrCreateBusinessScheduleSettings(businessID uint) (*database.BusinessScheduleSettings, error)
}
type MinorChecker interface {
	IsMinor(businessID, staffID uint) bool
}

type PositionLaborLine struct {
	PositionID     uint  `json:"position_id"`
	ShiftCount     int   `json:"shift_count"`
	Minutes        int   `json:"minutes"`
	LaborCostCents int64 `json:"-"` // owner/financial-gated; never auto-serialized
}
type Warning struct {
	Code    string `json:"code"`
	StaffID *uint  `json:"staff_id,omitempty"`
	Detail  int    `json:"detail"`
}
type ScheduledPreview struct {
	ScheduleID       uint                `json:"schedule_id"`
	TotalMinutes     int                 `json:"total_minutes"`
	LaborCostCents   int64               `json:"-"` // gated
	SalesTargetCents int64               `json:"-"` // gated (caller input)
	LaborCostPct     float64             `json:"labor_cost_pct"`
	Lines            []PositionLaborLine `json:"lines"`
	Warnings         []Warning           `json:"warnings"`
}
type ScheduledPreviewInput struct {
	BusinessID       uint
	ScheduleID       uint
	WeekStart        time.Time
	PublishedAt      *time.Time // nil = still draft
	Now              time.Time  // business-TZ now
	Loc              *time.Location
	SalesTargetCents int64
}

type ScheduledCalculator struct {
	shifts   ScheduledLaborProvider
	settings ScheduleSettingsProvider
	minors   MinorChecker
}

func NewScheduledCalculator(shifts ScheduledLaborProvider, settings ScheduleSettingsProvider, minors MinorChecker) *ScheduledCalculator {
	return &ScheduledCalculator{shifts: shifts, settings: settings, minors: minors}
}

// shiftWorkedMinutes = gross span − breaks, floored at 0 (matches the time-clock
// worked-minutes formula, contracts §2).
func shiftWorkedMinutes(s database.ScheduleShiftLine) int {
	m := int(s.EndsAt.Sub(s.StartsAt)/time.Minute) - s.BreakMinutes
	if m < 0 {
		return 0
	}
	return m
}

func (sc *ScheduledCalculator) Preview(in ScheduledPreviewInput) (ScheduledPreview, error) {
	shifts, err := sc.shifts.GetScheduleShiftLines(in.BusinessID, in.ScheduleID)
	if err != nil {
		return ScheduledPreview{}, fmt.Errorf("labor: schedule shifts: %w", err)
	}
	rates, err := sc.shifts.GetStaffPositionRates(in.BusinessID)
	if err != nil {
		return ScheduledPreview{}, fmt.Errorf("labor: staff rates: %w", err)
	}
	settings, err := sc.settings.GetOrCreateBusinessScheduleSettings(in.BusinessID)
	if err != nil {
		return ScheduledPreview{}, fmt.Errorf("labor: settings: %w", err)
	}

	out := ScheduledPreview{ScheduleID: in.ScheduleID, SalesTargetCents: in.SalesTargetCents}
	out.Lines = []PositionLaborLine{}
	out.Warnings = []Warning{}

	byPos := map[uint]*PositionLaborLine{}
	weeklyMinByStaff := map[uint]int{}

	for _, s := range shifts {
		mins := shiftWorkedMinutes(s)
		out.TotalMinutes += mins

		line := byPos[s.PositionID]
		if line == nil {
			line = &PositionLaborLine{PositionID: s.PositionID}
			byPos[s.PositionID] = line
		}
		line.ShiftCount++
		line.Minutes += mins

		// Cost only for assigned shifts — an open shift has no assignee, so it
		// has no rate and stays unpriced (contributes hours, $0).
		if s.StaffID != nil {
			weeklyMinByStaff[*s.StaffID] += mins
			if rate, ok := rates[database.StaffPositionKey{StaffID: *s.StaffID, PositionID: s.PositionID}]; ok {
				cost := int64(mins) * rate / 60 // (cents·min) ÷ (min/hr) = cents
				line.LaborCostCents += cost
				out.LaborCostCents += cost
			}
		}
	}

	for _, line := range byPos {
		out.Lines = append(out.Lines, *line)
	}
	sort.Slice(out.Lines, func(i, j int) bool { return out.Lines[i].PositionID < out.Lines[j].PositionID })

	if in.SalesTargetCents > 0 {
		out.LaborCostPct = float64(out.LaborCostCents) / float64(in.SalesTargetCents)
	}

	out.Warnings = append(out.Warnings, overtimeWarnings(weeklyMinByStaff, settings.OvertimeWeeklyMinutes)...)
	if w, ok := postedLateWarning(in.WeekStart, in.PublishedAt, in.Now, settings.PostedLeadDays); ok {
		out.Warnings = append(out.Warnings, w)
	}
	out.Warnings = append(out.Warnings, minorLateWarnings(shifts, settings.MinorCutoffMin, in.Loc, func(staffID uint) bool {
		return sc.minors.IsMinor(in.BusinessID, staffID)
	})...)

	return out, nil
}

// overtimeWarnings flags each staff whose summed weekly shift minutes exceed the
// configured threshold (default 2400 = 40h). threshold <= 0 disables the check.
// Deterministic order (staff id asc) for stable responses + tests.
func overtimeWarnings(weeklyMinByStaff map[uint]int, thresholdMin int) []Warning {
	if thresholdMin <= 0 {
		return nil
	}
	ids := make([]uint, 0, len(weeklyMinByStaff))
	for id := range weeklyMinByStaff {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]Warning, 0)
	for _, id := range ids {
		if weeklyMinByStaff[id] > thresholdMin {
			sid := id
			out = append(out, Warning{Code: WarnOvertime, StaffID: &sid, Detail: weeklyMinByStaff[id]})
		}
	}
	return out
}

// postedLateWarning fires when the schedule is (or would be) published inside the
// posted-lead window: effective time > WeekStart − leadDays. For a still-draft
// schedule the effective time is `now` (publishing now would already be late);
// for a published one it is PublishedAt. leadDays <= 0 disables the check.
func postedLateWarning(weekStart time.Time, publishedAt *time.Time, now time.Time, leadDays int) (Warning, bool) {
	if leadDays <= 0 {
		return Warning{}, false
	}
	deadline := weekStart.AddDate(0, 0, -leadDays)
	effective := now
	if publishedAt != nil {
		effective = *publishedAt
	}
	if effective.After(deadline) {
		return Warning{Code: WarnPostedLate, Detail: leadDays}, true
	}
	return Warning{}, false
}

// minorLateWarnings flags a minor-flagged staff scheduled to end work past the
// cutoff minute-of-day (business-TZ). A shift crossing midnight is always late.
// nil cutoff = feature off. One warning per minor (first late shift wins).
func minorLateWarnings(shifts []database.ScheduleShiftLine, cutoffMin *int, loc *time.Location, isMinor func(staffID uint) bool) []Warning {
	if cutoffMin == nil {
		return nil
	}
	if loc == nil {
		loc = time.UTC
	}
	seen := map[uint]bool{}
	out := make([]Warning, 0)
	for _, s := range shifts {
		if s.StaffID == nil || seen[*s.StaffID] || !isMinor(*s.StaffID) {
			continue
		}
		start := s.StartsAt.In(loc)
		end := s.EndsAt.In(loc)
		crossesDay := end.YearDay() != start.YearDay() || end.Year() != start.Year()
		endMinOfDay := end.Hour()*60 + end.Minute()
		if crossesDay || endMinOfDay > *cutoffMin {
			sid := *s.StaffID
			seen[sid] = true
			out = append(out, Warning{Code: WarnMinorLate, StaffID: &sid, Detail: *cutoffMin})
		}
	}
	return out
}
