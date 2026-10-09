package server

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/services/foodcost"
	"github.com/stdevmac/payverge/backend/internal/services/labor"
	"github.com/stdevmac/payverge/backend/internal/services/menuengineering"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- computeDayFraction --------------------------------------------------

func TestComputeDayFraction(t *testing.T) {
	loc := time.UTC
	day := func(h, m int) time.Time { return time.Date(2026, 6, 30, h, m, 0, 0, loc) }

	// At open (09:00) elapsed is 0 → clamps to the floor.
	assert.InDelta(t, briefingMinDayFraction, computeDayFraction(day(9, 0), loc), 1e-9)
	// 16:00 is 7h into a 14h window → exactly half.
	assert.InDelta(t, 0.5, computeDayFraction(day(16, 0), loc), 1e-9)
	// After close → full day.
	assert.InDelta(t, 1.0, computeDayFraction(day(23, 30), loc), 1e-9)
	// Before open (02:00) → floor, never the negative raw fraction.
	assert.InDelta(t, briefingMinDayFraction, computeDayFraction(day(2, 0), loc), 1e-9)
}

func TestComputeDayFractionNilLocationIsUTC(t *testing.T) {
	at16UTC := time.Date(2026, 6, 30, 16, 0, 0, 0, time.UTC)
	assert.InDelta(t, 0.5, computeDayFraction(at16UTC, nil), 1e-9)
}

// ---- computePace ---------------------------------------------------------

func TestComputePaceNilBaselineYieldsNilPace(t *testing.T) {
	proj, pace := computePace(1234, nil, 0.5)
	assert.Nil(t, proj)
	assert.Nil(t, pace)

	zero := 0.0
	proj, pace = computePace(1234, &zero, 0.5)
	assert.Nil(t, proj, "zero baseline must not divide")
	assert.Nil(t, pace)
}

func TestComputePaceZeroRevenueYieldsNilPace(t *testing.T) {
	// Before the first sale of the day a projection is pure noise — the old
	// behavior read "running a little behind — on track for ~$0.00, about
	// -100% under a typical Friday" on every pre-open briefing.
	typical := 1800.0
	proj, pace := computePace(0, &typical, 0.1)
	assert.Nil(t, proj)
	assert.Nil(t, pace)
}

func TestComputePaceMorningProjectsUp(t *testing.T) {
	typical := 1800.0
	// Halfway through the day with $1000 booked → projects to $2000, ahead of typical.
	proj, pace := computePace(1000, &typical, 0.5)
	require.NotNil(t, proj)
	require.NotNil(t, pace)
	assert.InDelta(t, 2000.0, *proj, 1e-9)
	assert.InDelta(t, (2000.0-1800.0)/1800.0*100, *pace, 1e-9)
	assert.Greater(t, *proj, 1000.0, "projection runs ahead of the running total")
}

func TestComputePaceEvenDayIsRoughlyZero(t *testing.T) {
	typical := 1800.0
	proj, pace := computePace(900, &typical, 0.5) // 900/0.5 = 1800 == typical
	require.NotNil(t, proj)
	require.NotNil(t, pace)
	assert.InDelta(t, 1800.0, *proj, 1e-9)
	assert.InDelta(t, 0.0, *pace, 1e-9)
}

// ---- typicalDayRevenue ---------------------------------------------------

func bucket(y, m, d int, rev float64, loc *time.Location) analyticsBucket {
	return analyticsBucket{Date: time.Date(y, time.Month(m), d, 0, 0, 0, 0, loc), Revenue: rev}
}

func TestTypicalDayRevenueSameWeekdayAveragesMostRecentFour(t *testing.T) {
	loc := time.UTC
	today := time.Date(2026, 6, 30, 18, 0, 0, 0, loc) // Tuesday

	buckets := []analyticsBucket{
		bucket(2026, 5, 26, 5000, loc), // Tue (oldest, should be dropped — only 4 kept)
		bucket(2026, 6, 2, 4000, loc),  // Tue
		bucket(2026, 6, 9, 3000, loc),  // Tue
		bucket(2026, 6, 16, 2000, loc), // Tue
		bucket(2026, 6, 23, 1000, loc), // Tue (most recent)
		bucket(2026, 6, 30, 9999, loc), // today — excluded
	}

	got := typicalDayRevenue(buckets, today, loc)
	require.NotNil(t, got)
	// Most recent 4 Tuesdays: 4000,3000,2000,1000 → avg 2500. Oldest 5000 dropped, today excluded.
	assert.InDelta(t, 2500.0, *got, 1e-9)
}

func TestTypicalDayRevenueFallsBackToRecentDaysWhenNoSameWeekday(t *testing.T) {
	loc := time.UTC
	today := time.Date(2026, 6, 30, 18, 0, 0, 0, loc) // Tuesday

	// No Tuesdays present → fall back to the average of recent full days.
	buckets := []analyticsBucket{
		bucket(2026, 6, 27, 100, loc), // Sat
		bucket(2026, 6, 28, 200, loc), // Sun
		bucket(2026, 6, 29, 300, loc), // Mon
	}

	got := typicalDayRevenue(buckets, today, loc)
	require.NotNil(t, got)
	assert.InDelta(t, 200.0, *got, 1e-9) // (100+200+300)/3
}

func TestTypicalDayRevenueEmptyIsNil(t *testing.T) {
	loc := time.UTC
	today := time.Date(2026, 6, 30, 18, 0, 0, 0, loc)
	assert.Nil(t, typicalDayRevenue(nil, today, loc))

	// Only today present → no history → nil.
	only := []analyticsBucket{bucket(2026, 6, 30, 500, loc)}
	assert.Nil(t, typicalDayRevenue(only, today, loc))
}

// ---- selectPlay ----------------------------------------------------------

func TestSelectPlayPicksHigherImpactRepriceOverPromote(t *testing.T) {
	me := menuengineering.Report{
		MedianQtySold: 10,
		Dishes: []menuengineering.DishClass{
			{MenuItemName: "Carbonara", Action: "promote", MarginPerUnit: 5, QtySold: 2},                  // gap 8 → 5*8*30/7 ≈ 171
			{MenuItemName: "Ribeye", Action: "reprice_up", AvgPrice: 10, SuggestedPrice: 12, QtySold: 50}, // 2*50*30/7 ≈ 428
		},
	}
	play := selectPlay(me)
	require.NotNil(t, play)
	assert.Equal(t, "reprice_up", play.Kind)
	assert.Equal(t, "Ribeye", play.ItemName)
	assert.Equal(t, "menu", play.Tab)
	require.NotNil(t, play.CurrentPrice)
	require.NotNil(t, play.SuggestedPrice)
	assert.InDelta(t, 10.0, *play.CurrentPrice, 1e-9)
	assert.InDelta(t, 12.0, *play.SuggestedPrice, 1e-9)
	// Impact carries the elasticity haircut (raw delta*qty*weekToMonth deflated
	// by repriceElasticityHaircut) so it never assumes perfectly inelastic demand.
	assert.InDelta(t, (12.0-10.0)*50*briefingWeekToMonth*(1-repriceElasticityHaircut), play.MonthlyImpact, 0.01)
}

// TestSelectPlayExtremeMarginBoundedAndHaircut runs the real classifier on a
// plowhorse whose food-cost % is far above the menu median (the $42 → $99.54
// case). The suggestion fed to selectPlay must be clamped (≤ 1.5× current), and
// the surfaced MonthlyImpact must carry the elasticity haircut.
func TestSelectPlayExtremeMarginBoundedAndHaircut(t *testing.T) {
	fc := foodcost.Report{Period: "week", Items: []foodcost.ItemMargin{
		{MenuItemID: "a", MenuItemName: "A", FoodCostPct: 0.20, QtySold: 40, AvgPrice: 20, UnitCost: 4, MarginPerUnit: 16, HasCompleteCost: true},
		{MenuItemID: "b", MenuItemName: "B", FoodCostPct: 0.22, QtySold: 30, AvgPrice: 20, UnitCost: 4.4, MarginPerUnit: 15.6, HasCompleteCost: true},
		{MenuItemID: "c", MenuItemName: "C", FoodCostPct: 0.24, QtySold: 20, AvgPrice: 20, UnitCost: 4.8, MarginPerUnit: 15.2, HasCompleteCost: true},
		// extreme plowhorse: $42 price, high food-cost% → naive target would be ~$99.
		{MenuItemID: "steak", MenuItemName: "Steak Plate", FoodCostPct: 0.85, QtySold: 50, AvgPrice: 42, UnitCost: 35.7, MarginPerUnit: 6.3, HasCompleteCost: true},
	}}
	me := menuengineering.Classify(fc)
	play := selectPlay(me)
	require.NotNil(t, play)
	assert.Equal(t, "reprice_up", play.Kind)
	assert.Equal(t, "Steak Plate", play.ItemName)
	require.NotNil(t, play.CurrentPrice)
	require.NotNil(t, play.SuggestedPrice)
	// Bounded: never above 1.5× the current $42 (would have been ~$99.54 unclamped).
	assert.LessOrEqual(t, *play.SuggestedPrice, *play.CurrentPrice*1.5+0.01)
	assert.Greater(t, *play.SuggestedPrice, *play.CurrentPrice, "still a genuine uplift")
	// Impact matches the haircut-adjusted formula.
	want := (*play.SuggestedPrice - *play.CurrentPrice) * 50 * briefingWeekToMonth * (1 - repriceElasticityHaircut)
	assert.InDelta(t, want, play.MonthlyImpact, 0.01)
}

func TestSelectPlayExcludesRepriceBelowQtyFloor(t *testing.T) {
	me := menuengineering.Report{
		Dishes: []menuengineering.DishClass{
			{MenuItemName: "Rare Special", Action: "reprice_up", AvgPrice: 10, SuggestedPrice: 14, QtySold: 2}, // qty < floor(3)
		},
	}
	assert.Nil(t, selectPlay(me), "a reprice candidate below the weekly qty floor must not surface")
}

func TestSelectPlayEmptyDishesIsNil(t *testing.T) {
	assert.Nil(t, selectPlay(menuengineering.Report{}))
}

func TestSelectPlayPromoteUsesMedianGap(t *testing.T) {
	me := menuengineering.Report{
		MedianQtySold: 10,
		Dishes: []menuengineering.DishClass{
			{MenuItemName: "Puzzle Dish", Action: "promote", MarginPerUnit: 5, QtySold: 2},
		},
	}
	play := selectPlay(me)
	require.NotNil(t, play)
	assert.Equal(t, "promote", play.Kind)
	assert.Equal(t, "Puzzle Dish", play.ItemName)
	assert.Nil(t, play.CurrentPrice, "promote plays carry no price pair")
	assert.Nil(t, play.SuggestedPrice)
	// gap = round(10) - 2 = 8 → 5 * 8 * (30/7)
	assert.InDelta(t, 5.0*8.0*briefingWeekToMonth, play.MonthlyImpact, 0.01)
}

func TestSelectPlayPromoteAtOrAboveMedianHasNoImpact(t *testing.T) {
	me := menuengineering.Report{
		MedianQtySold: 5,
		Dishes: []menuengineering.DishClass{
			{MenuItemName: "Already Popular", Action: "promote", MarginPerUnit: 5, QtySold: 9}, // gap clamped to 0
		},
	}
	assert.Nil(t, selectPlay(me))
}

// ---- selectWin -----------------------------------------------------------

func TestSelectWin(t *testing.T) {
	// Above threshold and no play → win.
	win := selectWin(8.0, false)
	require.NotNil(t, win)
	assert.Equal(t, "revenue_up_wow", win.Kind)
	assert.InDelta(t, 8.0, win.Pct, 1e-9)

	// A play present suppresses the win.
	assert.Nil(t, selectWin(8.0, true))

	// Below threshold → nil.
	assert.Nil(t, selectWin(4.0, false))
}

// ---- briefingState -------------------------------------------------------

func TestBriefingState(t *testing.T) {
	assert.Equal(t, "learning", briefingState(0, 0, false, false))
	assert.Equal(t, "active", briefingState(100, 0, false, false))
	assert.Equal(t, "active", briefingState(0, 500, false, false))
	// Recipe or payroll history alone is enough to leave the learning state.
	assert.Equal(t, "active", briefingState(0, 0, true, false))
	assert.Equal(t, "active", briefingState(0, 0, false, true))
}

// S3-Loop: marketing panel rides on assembleBriefing when MarketingLoop is set.
func TestAssembleBriefing_IncludesMarketingLoopWhenPresent(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 7, 26, 16, 0, 0, 0, loc)
	providers := briefingProviders{
		Now:      func() time.Time { return now },
		FoodCost: func() (foodcost.Report, error) { return foodcost.Report{}, nil },
		Labor:    func() (labor.Report, error) { return labor.Report{}, nil },
		TodayReport: func() (*analytics.PeriodReport, error) {
			return &analytics.PeriodReport{TotalRevenue: 100, BillCount: 3, AverageTicket: 33}, nil
		},
		WeekReport: func() (*analytics.PeriodReport, error) {
			return &analytics.PeriodReport{TotalRevenue: 700, GrowthRate: func() *float64 { v := float64(2); return &v }()}, nil
		},
		DailyBuckets: func() ([]analyticsBucket, error) { return nil, nil },
		OpenBills:    func() int { return 0 },
		Insights:     func(fr *foodcost.Report, lr *labor.Report) []proactiveInsight { return nil },
		MarketingLoop: func() *briefingMarketing {
			return &briefingMarketing{
				PostsThisPeriod: 2,
				PeriodDays:      7,
				RecentTitles:    []string{"Feature ribeye", "Happy hour"},
				Channels:        []string{"Instagram Stories"},
				Tab:             "marketing",
			}
		},
	}
	out := assembleBriefing(7, loc, providers)
	require.NotNil(t, out.Marketing)
	assert.Equal(t, 2, out.Marketing.PostsThisPeriod)
	assert.Equal(t, 7, out.Marketing.PeriodDays)
	assert.Equal(t, "marketing", out.Marketing.Tab)
	assert.Equal(t, []string{"Feature ribeye", "Happy hour"}, out.Marketing.RecentTitles)
	assert.Equal(t, []string{"Instagram Stories"}, out.Marketing.Channels)
}

func TestAssembleBriefing_OmitsMarketingWhenLoopNil(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 7, 26, 16, 0, 0, 0, loc)
	providers := briefingProviders{
		Now:      func() time.Time { return now },
		FoodCost: func() (foodcost.Report, error) { return foodcost.Report{}, nil },
		Labor:    func() (labor.Report, error) { return labor.Report{}, nil },
		TodayReport: func() (*analytics.PeriodReport, error) {
			return &analytics.PeriodReport{TotalRevenue: 100, BillCount: 3, AverageTicket: 33}, nil
		},
		WeekReport: func() (*analytics.PeriodReport, error) {
			return &analytics.PeriodReport{TotalRevenue: 700, GrowthRate: func() *float64 { v := float64(2); return &v }()}, nil
		},
		DailyBuckets: func() ([]analyticsBucket, error) { return nil, nil },
		OpenBills:    func() int { return 0 },
		Insights:     func(fr *foodcost.Report, lr *labor.Report) []proactiveInsight { return nil },
	}
	out := assembleBriefing(7, loc, providers)
	assert.Nil(t, out.Marketing)
}

// ---- assembleBriefing access-shape: foodcost computed exactly once -------

func TestAssembleBriefingComputesFoodCostExactlyOnce(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 6, 30, 16, 0, 0, 0, loc)

	report := foodcost.Report{
		BlendedFoodCostPct: 0.28,
		TotalRevenue:       5000,
		Items: []foodcost.ItemMargin{
			{MenuItemName: "Carbonara", HasCompleteCost: true, QtySold: 20, AvgPrice: 18, FoodCostPct: 0.30, MarginPerUnit: 12},
			{MenuItemName: "Ribeye", HasCompleteCost: true, QtySold: 12, AvgPrice: 40, FoodCostPct: 0.45, MarginPerUnit: 22},
		},
	}

	var foodCalls int
	var insightFood *foodcost.Report
	providers := briefingProviders{
		Now: func() time.Time { return now },
		FoodCost: func() (foodcost.Report, error) {
			foodCalls++
			return report, nil
		},
		Labor: func() (labor.Report, error) { return labor.Report{HasData: true, LaborCostPct: 0.24}, nil },
		TodayReport: func() (*analytics.PeriodReport, error) {
			return &analytics.PeriodReport{TotalRevenue: 1000, BillCount: 30, AverageTicket: 33.3}, nil
		},
		WeekReport: func() (*analytics.PeriodReport, error) {
			return &analytics.PeriodReport{TotalRevenue: 7000, GrowthRate: func() *float64 { v := float64(9); return &v }()}, nil
		},
		DailyBuckets: func() ([]analyticsBucket, error) {
			return []analyticsBucket{bucket(2026, 6, 23, 900, loc)}, nil
		},
		OpenBills: func() int { return 4 },
		// The insights consumer must receive the precomputed report and must NOT
		// recompute foodcost — it derives the food-cost-high check from the report.
		Insights: func(fr *foodcost.Report, lr *labor.Report) []proactiveInsight {
			insightFood = fr
			require.NotNil(t, fr, "insights consumer must receive the precomputed foodcost report")
			return nil
		},
	}

	out := assembleBriefing(7, loc, providers)

	assert.Equal(t, 1, foodCalls, "foodcost.Analyze must be computed exactly once per briefing request")
	assert.NotNil(t, insightFood, "insights consumer ran and received the precomputed report")
	require.NotNil(t, out.Pulse.FoodCostPct)
	assert.InDelta(t, 0.28, *out.Pulse.FoodCostPct, 1e-9)
	require.NotNil(t, out.Pulse.LaborCostPct)
	assert.InDelta(t, 0.24, *out.Pulse.LaborCostPct, 1e-9)
	assert.Equal(t, 30, out.Pulse.Orders)
	assert.Equal(t, 4, out.Pulse.OpenBills)
	assert.Equal(t, "active", out.State)
}

// ---- assembleBriefing access-shape: labor computed exactly once ----------

// labor.Analyze has the same dual-consumer shape foodcost does — the pulse
// labor number AND the labor-high insight. On a calm briefing (fewer than 3
// exception insights, so the labor slot is reached) that would recompute labor
// twice on every cache miss unless the single report is threaded to both. This
// locks the seam contract: Labor() runs once and its report reaches the insights
// consumer.
func TestAssembleBriefingComputesLaborExactlyOnce(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 6, 30, 16, 0, 0, 0, loc)

	var laborCalls int
	var insightLabor *labor.Report
	providers := briefingProviders{
		Now:      func() time.Time { return now },
		FoodCost: func() (foodcost.Report, error) { return foodcost.Report{}, nil },
		Labor: func() (labor.Report, error) {
			laborCalls++
			return labor.Report{HasData: true, NetSales: 10000, LaborCost: 4000, LaborCostPct: 0.40}, nil
		},
		TodayReport: func() (*analytics.PeriodReport, error) {
			return &analytics.PeriodReport{TotalRevenue: 1000, BillCount: 20, AverageTicket: 50}, nil
		},
		WeekReport:   func() (*analytics.PeriodReport, error) { return &analytics.PeriodReport{TotalRevenue: 7000}, nil },
		DailyBuckets: func() ([]analyticsBucket, error) { return nil, nil },
		OpenBills:    func() int { return 0 },
		// The insights consumer must receive the precomputed labor report and must
		// NOT recompute labor — it derives the labor-high check from the report.
		Insights: func(fr *foodcost.Report, lr *labor.Report) []proactiveInsight {
			insightLabor = lr
			require.NotNil(t, lr, "insights consumer must receive the precomputed labor report")
			return nil
		},
	}

	out := assembleBriefing(7, loc, providers)

	assert.Equal(t, 1, laborCalls, "labor.Analyze must be computed exactly once per briefing request")
	require.NotNil(t, insightLabor, "insights consumer ran and received the precomputed labor report")
	require.NotNil(t, out.Pulse.LaborCostPct)
	assert.InDelta(t, 0.40, *out.Pulse.LaborCostPct, 1e-9)
}

// The precomputed labor report must drive the labor-high insight (no recompute):
// a report over threshold surfaces labor_high; a healthy one does not.
func TestLaborOverThresholdFromReport(t *testing.T) {
	pct, amount := laborOverThresholdFromReport(
		labor.Report{HasData: true, NetSales: 10000, LaborCost: 4000, LaborCostPct: 0.40}, 0.35)
	assert.InDelta(t, 0.40, pct, 1e-9)
	assert.InDelta(t, 4000.0, amount, 1e-9)

	// At/under threshold, no payroll data, or zero net sales → nothing to flag.
	zp, za := laborOverThresholdFromReport(labor.Report{HasData: true, NetSales: 10000, LaborCostPct: 0.30}, 0.35)
	assert.Zero(t, zp)
	assert.Zero(t, za)
	np, na := laborOverThresholdFromReport(labor.Report{HasData: false, LaborCostPct: 0.99}, 0.35)
	assert.Zero(t, np)
	assert.Zero(t, na)
}

func TestAssembleBriefingWinSurfacesOnlyWithoutPlay(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 6, 30, 16, 0, 0, 0, loc)

	// A report with no actionable plays (no reprice_up / promote candidates).
	flatReport := foodcost.Report{TotalRevenue: 0}

	providers := briefingProviders{
		Now:         func() time.Time { return now },
		FoodCost:    func() (foodcost.Report, error) { return flatReport, nil },
		Labor:       func() (labor.Report, error) { return labor.Report{}, nil },
		TodayReport: func() (*analytics.PeriodReport, error) { return &analytics.PeriodReport{TotalRevenue: 500}, nil },
		WeekReport: func() (*analytics.PeriodReport, error) {
			return &analytics.PeriodReport{TotalRevenue: 7000, GrowthRate: func() *float64 { v := float64(12); return &v }()}, nil
		},
		DailyBuckets: func() ([]analyticsBucket, error) { return nil, nil },
		OpenBills:    func() int { return 0 },
		Insights:     func(fr *foodcost.Report, lr *labor.Report) []proactiveInsight { return nil },
	}

	out := assembleBriefing(7, loc, providers)
	assert.Nil(t, out.Play, "no actionable play for a flat report")
	require.NotNil(t, out.Win, "a 12% week-over-week gain surfaces as a win when no play exists")
	assert.Equal(t, "revenue_up_wow", out.Win.Kind)
	assert.InDelta(t, 12.0, out.Win.Pct, 1e-9)
}

// BenchmarkGetDirectorBriefing measures the per-request assembly hot path
// (menu-engineering classification + pace/play/win math) with realistic fixture
// sizes. The provider closures are stubbed so the benchmark isolates the CPU and
// allocation cost of assembling the briefing — the DB round-trips behind the real
// providers are cached for 60s and are not the per-request bottleneck.
func BenchmarkGetDirectorBriefing(b *testing.B) {
	loc := time.UTC
	now := time.Date(2026, 6, 30, 16, 0, 0, 0, loc)

	items := make([]foodcost.ItemMargin, 0, 24)
	for i := 0; i < 24; i++ {
		items = append(items, foodcost.ItemMargin{
			MenuItemID:      string(rune('a' + i)),
			MenuItemName:    "Dish " + string(rune('A'+i)),
			HasCompleteCost: true,
			QtySold:         (i*7)%40 + 1,
			AvgPrice:        float64(8 + i),
			UnitCost:        float64(3 + i/2),
			FoodCostPct:     0.20 + float64(i%10)/100,
			MarginPerUnit:   float64(5 + i),
		})
	}
	report := foodcost.Report{BlendedFoodCostPct: 0.31, TotalRevenue: 12000, Items: items}

	buckets := make([]analyticsBucket, 0, 28)
	day := time.Date(2026, 6, 30, 0, 0, 0, 0, loc).AddDate(0, 0, -27)
	for i := 0; i < 28; i++ {
		buckets = append(buckets, analyticsBucket{Date: day.AddDate(0, 0, i), Revenue: float64(800 + (i*53)%600)})
	}

	providers := briefingProviders{
		Now:      func() time.Time { return now },
		FoodCost: func() (foodcost.Report, error) { return report, nil },
		Labor:    func() (labor.Report, error) { return labor.Report{HasData: true, LaborCostPct: 0.24}, nil },
		TodayReport: func() (*analytics.PeriodReport, error) {
			return &analytics.PeriodReport{TotalRevenue: 4200, BillCount: 84, AverageTicket: 37}, nil
		},
		WeekReport: func() (*analytics.PeriodReport, error) {
			return &analytics.PeriodReport{TotalRevenue: 28000, GrowthRate: func() *float64 { v := float64(6); return &v }()}, nil
		},
		DailyBuckets: func() ([]analyticsBucket, error) { return buckets, nil },
		OpenBills:    func() int { return 5 },
		Insights:     func(fr *foodcost.Report, lr *labor.Report) []proactiveInsight { return nil },
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out := assembleBriefing(7, loc, providers)
		if math.IsNaN(out.Pulse.Revenue) {
			b.Fatal("unexpected NaN revenue")
		}
	}
}

// ---- wire contract: insights is always a JSON array ----------------------

// A fresh business has no exception insights, so the Insights consumer returns
// a nil slice. A nil Go slice marshals to JSON `null`, which the frontend maps
// over unguarded — crashing the whole Director Console tab into its error
// boundary. The response must always serialize `insights` as `[]`.
func TestAssembleBriefingInsightsMarshalsAsEmptyArrayNotNull(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 6, 30, 16, 0, 0, 0, loc)

	providers := briefingProviders{
		Now:          func() time.Time { return now },
		FoodCost:     func() (foodcost.Report, error) { return foodcost.Report{}, nil },
		Labor:        func() (labor.Report, error) { return labor.Report{}, nil },
		TodayReport:  func() (*analytics.PeriodReport, error) { return &analytics.PeriodReport{}, nil },
		WeekReport:   func() (*analytics.PeriodReport, error) { return &analytics.PeriodReport{}, nil },
		DailyBuckets: func() ([]analyticsBucket, error) { return nil, nil },
		OpenBills:    func() int { return 0 },
		Insights:     func(fr *foodcost.Report, lr *labor.Report) []proactiveInsight { return nil },
	}

	out := assembleBriefing(7, loc, providers)
	require.NotNil(t, out.Insights, "insights must be a non-nil slice so it marshals as []")

	raw, err := json.Marshal(out)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"insights":[]`)
	assert.NotContains(t, string(raw), `"insights":null`)
}
