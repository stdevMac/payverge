package labor

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeWindow struct {
	start, end time.Time
	gotPeriod  string
}

func (f *fakeWindow) ParsePeriodWindow(period string, _ *time.Location) (time.Time, time.Time, error) {
	f.gotPeriod = period
	return f.start, f.end, nil
}

type fakePayroll struct {
	runs             []database.PayrollRunCost
	gotStart, gotEnd time.Time
}

func (f *fakePayroll) GetPaidPayrollRunsOverlapping(_ uint, s, e time.Time) ([]database.PayrollRunCost, error) {
	f.gotStart, f.gotEnd = s, e
	return f.runs, nil
}

type fakeSales struct {
	summary          *analytics.PaymentWindowSummary
	gotStart, gotEnd time.Time
}

func (f *fakeSales) GetPaymentWindowSummary(_ uint, s, e time.Time) (*analytics.PaymentWindowSummary, error) {
	f.gotStart, f.gotEnd = s, e
	return f.summary, nil
}

func day(n int) time.Time { return time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, n) }

func TestAnalyze_ProratesAndExcludesTips(t *testing.T) {
	win := &fakeWindow{start: day(0), end: day(7)}
	pay := &fakePayroll{runs: []database.PayrollRunCost{
		{ID: 1, PeriodStart: day(0), PeriodEnd: day(14), GrossTotal: 100000, BonusTotal: 0},  // 14d period, frac .5 → $500
		{ID: 2, PeriodStart: day(3), PeriodEnd: day(5), GrossTotal: 20000, BonusTotal: 5000}, // inside, frac 1 → $250
	}}
	sales := &fakeSales{summary: &analytics.PaymentWindowSummary{TotalRevenue: 3000, TotalTips: 900}}
	c := NewCalculator(pay, sales, win)
	rep, err := c.Analyze(1, "week", nil)
	require.NoError(t, err)
	assert.InDelta(t, 750.0, rep.LaborCost, 0.01)
	assert.InDelta(t, 3000.0, rep.NetSales, 0.01)
	assert.InDelta(t, 0.25, rep.LaborCostPct, 0.0001)
	assert.Equal(t, 2, rep.PayrollRunCount)
	assert.True(t, rep.HasData)
	assert.Equal(t, pay.gotStart, sales.gotStart) // same window for numerator + denominator
	assert.Equal(t, pay.gotEnd, sales.gotEnd)
	// run #1 has the larger LaborCost ($500 vs $250) so it sorts first.
	assert.Equal(t, uint(1), rep.Contributions[0].PayrollRunID)
	assert.InDelta(t, 0.5, rep.Contributions[0].OverlapFraction, 0.0001)
}

// TestAnalyzeWindow_UsesExplicitBounds skips ParsePeriodWindow and feeds the
// payroll + sales providers the caller-supplied [start, end).
func TestAnalyzeWindow_UsesExplicitBounds(t *testing.T) {
	start, end := day(0), day(14)
	pay := &fakePayroll{runs: []database.PayrollRunCost{
		{ID: 1, PeriodStart: day(0), PeriodEnd: day(14), GrossTotal: 100000, BonusTotal: 0},
	}}
	sales := &fakeSales{summary: &analytics.PaymentWindowSummary{TotalRevenue: 2000}}
	// Window provider must not be consulted for AnalyzeWindow.
	win := &fakeWindow{start: day(99), end: day(100)}
	c := NewCalculator(pay, sales, win)
	rep, err := c.AnalyzeWindow(1, start, end, "custom")
	require.NoError(t, err)
	assert.Equal(t, "custom", rep.Period)
	assert.Equal(t, start, pay.gotStart)
	assert.Equal(t, end, pay.gotEnd)
	assert.Equal(t, start, sales.gotStart)
	assert.Equal(t, end, sales.gotEnd)
	assert.Equal(t, "", win.gotPeriod, "ParsePeriodWindow must not run for AnalyzeWindow")
	assert.InDelta(t, 1000.0, rep.LaborCost, 0.01)
	assert.InDelta(t, 0.5, rep.LaborCostPct, 0.0001)
}

// TestAnalyze_LeftStraddleAndContainment exercises the two geometries the other
// cases miss: a run that STARTS BEFORE the window (left clamp → maxTime returns
// the window start) and the window sitting STRICTLY INSIDE a long run (clamped on
// both sides → frac = window/runDuration). The proration denominator is the run's
// own duration in both.
func TestAnalyze_LeftStraddleAndContainment(t *testing.T) {
	win := &fakeWindow{start: day(0), end: day(7)}
	pay := &fakePayroll{runs: []database.PayrollRunCost{
		// Left-straddle: [day-3, day3) — overlap [day0,day3)=3d over a 6d run → frac 0.5 → $400.
		{ID: 1, PeriodStart: day(-3), PeriodEnd: day(3), GrossTotal: 80000, BonusTotal: 0},
		// Window inside run: [day-7, day14) — overlap == full 7d window over a 21d run → frac 1/3 → $300.
		{ID: 2, PeriodStart: day(-7), PeriodEnd: day(14), GrossTotal: 90000, BonusTotal: 0},
	}}
	c := NewCalculator(pay, &fakeSales{summary: &analytics.PaymentWindowSummary{TotalRevenue: 1000}}, win)
	rep, err := c.Analyze(1, "week", nil)
	require.NoError(t, err)
	assert.Equal(t, 2, rep.PayrollRunCount)
	byID := map[uint]float64{}
	fracByID := map[uint]float64{}
	for _, ct := range rep.Contributions {
		byID[ct.PayrollRunID] = ct.LaborCost
		fracByID[ct.PayrollRunID] = ct.OverlapFraction
	}
	assert.InDelta(t, 0.5, fracByID[1], 0.0001)
	assert.InDelta(t, 400.0, byID[1], 0.01)
	assert.InDelta(t, 1.0/3.0, fracByID[2], 0.0001)
	assert.InDelta(t, 300.0, byID[2], 0.01)
	assert.InDelta(t, 700.0, rep.LaborCost, 0.01)
}

func TestAnalyze_ExcludesNonOverlappingRuns(t *testing.T) {
	win := &fakeWindow{start: day(10), end: day(17)}
	pay := &fakePayroll{runs: []database.PayrollRunCost{
		{ID: 1, PeriodStart: day(0), PeriodEnd: day(5), GrossTotal: 99999}, // entirely before window
	}}
	c := NewCalculator(pay, &fakeSales{summary: &analytics.PaymentWindowSummary{TotalRevenue: 100}}, win)
	rep, err := c.Analyze(1, "week", nil)
	require.NoError(t, err)
	assert.Equal(t, 0.0, rep.LaborCost)
	assert.False(t, rep.HasData)
}

func TestAnalyze_EmptySerializesContributionsAsArray(t *testing.T) {
	win := &fakeWindow{start: day(0), end: day(7)}
	c := NewCalculator(&fakePayroll{}, &fakeSales{summary: &analytics.PaymentWindowSummary{TotalRevenue: 100}}, win)
	rep, err := c.Analyze(1, "", nil) // "" → week
	require.NoError(t, err)
	assert.False(t, rep.HasData)
	assert.Equal(t, "week", rep.Period)
	assert.Equal(t, "week", win.gotPeriod)
	b, _ := json.Marshal(rep)
	assert.Contains(t, string(b), `"contributions":[]`)
}

func TestAnalyze_RejectsDayAndToday(t *testing.T) {
	c := NewCalculator(&fakePayroll{}, &fakeSales{}, &fakeWindow{start: day(0), end: day(1)})
	_, err := c.Analyze(1, "day", nil)
	assert.ErrorIs(t, err, ErrUnsupportedPeriod)
	_, err = c.Analyze(1, "today", nil)
	assert.ErrorIs(t, err, ErrUnsupportedPeriod)
}

func TestAnalyze_NetSalesZeroGuards(t *testing.T) {
	win := &fakeWindow{start: day(0), end: day(7)}
	pay := &fakePayroll{runs: []database.PayrollRunCost{{ID: 1, PeriodStart: day(0), PeriodEnd: day(7), GrossTotal: 50000}}}
	c := NewCalculator(pay, &fakeSales{summary: &analytics.PaymentWindowSummary{TotalRevenue: 0}}, win)
	rep, err := c.Analyze(1, "week", nil)
	require.NoError(t, err)
	assert.InDelta(t, 500.0, rep.LaborCost, 0.01)
	assert.Equal(t, 0.0, rep.LaborCostPct) // no divide-by-zero, pct stays 0
	assert.True(t, rep.HasData)            // runs exist even with no sales
}

func TestAnalyze_DeductionsAreNotInTheProjection(t *testing.T) {
	// The reader projects only gross+bonus; the calculator sums gross+bonus. There is
	// no Deduction field on PayrollRunCost, so labor cost = (gross+bonus)/100 only.
	win := &fakeWindow{start: day(0), end: day(7)}
	pay := &fakePayroll{runs: []database.PayrollRunCost{{ID: 1, PeriodStart: day(0), PeriodEnd: day(7), GrossTotal: 80000, BonusTotal: 20000}}}
	c := NewCalculator(pay, &fakeSales{summary: &analytics.PaymentWindowSummary{TotalRevenue: 4000}}, win)
	rep, err := c.Analyze(1, "week", nil)
	require.NoError(t, err)
	assert.InDelta(t, 1000.0, rep.LaborCost, 0.01) // (80000+20000)/100
	assert.InDelta(t, 0.25, rep.LaborCostPct, 0.0001)
}

type fakeWorked struct {
	rows             []database.StaffWorkedMinutes
	gotStart, gotEnd time.Time
}

func (f *fakeWorked) GetApprovedWorkedMinutes(_ uint, s, e time.Time) ([]database.StaffWorkedMinutes, error) {
	f.gotStart, f.gotEnd = s, e
	return f.rows, nil
}

func TestAnalyzeActual_SumsApprovedMinutesTimesRate(t *testing.T) {
	win := &fakeWindow{start: day(0), end: day(7)}
	// staff 7: 450 min @ $20/h (2000c) -> 7.5h * $20 = $150
	// staff 9: 120 min @ $30/h (3000c) -> 2.0h * $30 = $60
	// staff 5: 60 min @ $0 (no primary position) -> $0, but 1h counted
	worked := &fakeWorked{rows: []database.StaffWorkedMinutes{
		{StaffID: 7, Minutes: 450, RateCents: 2000},
		{StaffID: 9, Minutes: 120, RateCents: 3000},
		{StaffID: 5, Minutes: 60, RateCents: 0},
	}}
	sales := &fakeSales{summary: &analytics.PaymentWindowSummary{TotalRevenue: 2100, TotalTips: 300}}
	c := NewCalculator(&fakePayroll{}, sales, win).WithWorkedHours(worked)

	rep, err := c.AnalyzeActual(1, "week", nil)
	require.NoError(t, err)
	assert.Equal(t, "actual", rep.Basis)
	assert.InDelta(t, 210.0, rep.LaborCost, 0.01)   // 150 + 60 + 0
	assert.InDelta(t, 10.5, rep.WorkedHours, 0.001) // 7.5 + 2.0 + 1.0
	assert.InDelta(t, 2100.0, rep.NetSales, 0.01)
	assert.InDelta(t, 0.1, rep.LaborCostPct, 0.0001) // 210 / 2100
	assert.True(t, rep.HasData)
	require.Len(t, rep.StaffContributions, 3)
	// Sorted by labor cost desc: staff 7 ($150) first, then staff 9 ($60), then staff 5 ($0).
	assert.Equal(t, uint(7), rep.StaffContributions[0].StaffID)
	assert.InDelta(t, 150.0, rep.StaffContributions[0].LaborCost, 0.01)
	assert.Equal(t, 450, rep.StaffContributions[0].WorkedMinutes)
	assert.InDelta(t, 7.5, rep.StaffContributions[0].WorkedHours, 0.001)
	assert.Equal(t, uint(5), rep.StaffContributions[2].StaffID)
	assert.InDelta(t, 0.0, rep.StaffContributions[2].LaborCost, 0.01)
	// Same window drives numerator + denominator.
	assert.Equal(t, worked.gotStart, sales.gotStart)
	assert.Equal(t, worked.gotEnd, sales.gotEnd)
}

func TestAnalyzeActual_EmptyAndZeroSalesGuards(t *testing.T) {
	win := &fakeWindow{start: day(0), end: day(7)}
	// No approved entries -> empty, has_data false, contributions an array.
	c := NewCalculator(&fakePayroll{}, &fakeSales{summary: &analytics.PaymentWindowSummary{TotalRevenue: 100}}, win).
		WithWorkedHours(&fakeWorked{})
	rep, err := c.AnalyzeActual(1, "", nil) // "" -> week
	require.NoError(t, err)
	assert.False(t, rep.HasData)
	assert.Equal(t, "week", rep.Period)
	// StaffContributions is a non-nil empty slice (the handler builds the wire
	// array from it; the Report's own json omits it when empty).
	require.NotNil(t, rep.StaffContributions)
	assert.Len(t, rep.StaffContributions, 0)

	// Worked minutes but zero sales -> no divide-by-zero.
	c2 := NewCalculator(&fakePayroll{}, &fakeSales{summary: &analytics.PaymentWindowSummary{TotalRevenue: 0}}, win).
		WithWorkedHours(&fakeWorked{rows: []database.StaffWorkedMinutes{{StaffID: 1, Minutes: 60, RateCents: 6000}}})
	rep2, err := c2.AnalyzeActual(1, "week", nil)
	require.NoError(t, err)
	assert.InDelta(t, 60.0, rep2.LaborCost, 0.01) // 1h * $60
	assert.Equal(t, 0.0, rep2.LaborCostPct)
	assert.True(t, rep2.HasData)
}

func TestAnalyzeActual_RejectsBadPeriodAndMissingProvider(t *testing.T) {
	win := &fakeWindow{start: day(0), end: day(1)}
	c := NewCalculator(&fakePayroll{}, &fakeSales{}, win).WithWorkedHours(&fakeWorked{})
	_, err := c.AnalyzeActual(1, "day", nil)
	assert.ErrorIs(t, err, ErrUnsupportedPeriod)

	// No worked-hours provider configured -> a clear error, not a nil panic.
	c2 := NewCalculator(&fakePayroll{}, &fakeSales{}, win)
	_, err = c2.AnalyzeActual(1, "week", nil)
	require.Error(t, err)
}
