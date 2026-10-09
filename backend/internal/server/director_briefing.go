package server

import (
	"math"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/services/foodcost"
	"github.com/stdevmac/payverge/backend/internal/services/labor"
	"github.com/stdevmac/payverge/backend/internal/services/menuengineering"
)

// Sage Briefing front door.
//
// The briefing is the always-present "front door" of the Director Console:
// even when nothing is wrong it speaks first with a GM-voiced read of today's
// numbers and one forward-looking move. This file holds the briefing DTO plus
// the small pure functions that compute the pace projection, the play, the win,
// and the learning/active state. They take already-fetched inputs (analytics
// report, foodcost report, menu-engineering report, the clock) so the math is
// unit-testable without a database; the handler in director_briefing_handler.go
// does the fetching and feeds them.
//
// Money fields are float64 dollars (the repo wire contract); the GM voice and
// all es / es-AR copy live in the frontend i18n templates — the DTO emits
// numbers + identifiers only.

const (
	briefingServiceOpenHour  = 9          // 09:00 business-tz assumed service start
	briefingServiceCloseHour = 23         // 23:00 assumed service end (14h window)
	briefingMinDayFraction   = 0.05       // clamp floor so a morning never projects to infinity
	briefingWeekToMonth      = 30.0 / 7.0 // ≈4.345 weekly→monthly scale for impact estimates
	briefingMinPlayQty       = 3          // weekly qty floor for a meaningful reprice play
	// repriceElasticityHaircut deflates the reprice_up monthly projection so it
	// stops assuming perfectly inelastic demand. The raw impact multiplies the
	// full price delta by current volume, i.e. it pretends every guest keeps
	// buying at the higher price — optimistic and trust-eroding when surfaced as
	// a headline dollar figure. Shaving 25% off yields a conservative, defensible
	// estimate (a mid-point in the 15–30% band; the classifier's 1.5× price cap
	// already bounds the delta, so a moderate haircut is enough).
	repriceElasticityHaircut = 0.25
	briefingWinMinPct        = 5.0        // week growth ≥5% surfaces a win (only when no play)
	briefingStateLearning    = "learning" // brand-new restaurant, no sales/recipe/payroll history
	briefingStateActive      = "active"   // has history → real read + play/win
)

type briefingPulse struct {
	Revenue      float64  `json:"revenue"`
	Projected    *float64 `json:"projected"`
	TypicalDay   *float64 `json:"typical_day"`
	PacePct      *float64 `json:"pace_pct"`
	Orders       int      `json:"orders"`
	AvgTicket    float64  `json:"avg_ticket"`
	FoodCostPct  *float64 `json:"food_cost_pct"`
	LaborCostPct *float64 `json:"labor_cost_pct"`
	OpenBills    int      `json:"open_bills"`
	Remaining    float64  `json:"remaining,omitempty"`
}

type briefingPlay struct {
	Kind           string   `json:"kind"`
	Tab            string   `json:"tab"`
	ItemName       string   `json:"item_name"`
	CurrentPrice   *float64 `json:"current_price"`
	SuggestedPrice *float64 `json:"suggested_price"`
	MonthlyImpact  float64  `json:"monthly_impact"`
}

type briefingWin struct {
	Kind string  `json:"kind"`
	Pct  float64 `json:"pct"`
}

// briefingMarketing is the S3-Loop closed-loop panel: posts the operator
// marked posted this period + deep-link tab back to the Marketing Library.
// Null/omitted when count is zero so the front door stays quiet for businesses
// that have not used Marketing yet. Never includes schedule fields.
type briefingMarketing struct {
	PostsThisPeriod int      `json:"posts_this_period"`
	PeriodDays      int      `json:"period_days"`
	RecentTitles    []string `json:"recent_titles"`
	Channels        []string `json:"channels"`
	// Tab is the dashboard tab CTA (always "marketing"). FE deep-links here.
	Tab string `json:"tab"`
}

type briefingResponse struct {
	State     string             `json:"state"`
	Pulse     briefingPulse      `json:"pulse"`
	Insights  []proactiveInsight `json:"insights"`
	Play      *briefingPlay      `json:"play"`
	Win       *briefingWin       `json:"win"`
	Marketing *briefingMarketing `json:"marketing,omitempty"`
}

// analyticsBucket is one day's revenue, parsed into a business-tz time so the
// pure math can compare weekdays without re-touching the analytics layer.
type analyticsBucket struct {
	Date    time.Time
	Revenue float64
}

// briefingProviders is the data seam for the assembler. The handler wires real
// (DB-backed) closures; tests inject spies — crucially to assert FoodCost is
// invoked exactly once per request (the perf contract).
type briefingProviders struct {
	Now          func() time.Time
	FoodCost     func() (foodcost.Report, error)
	Labor        func() (labor.Report, error)
	TodayReport  func() (*analytics.PeriodReport, error)
	WeekReport   func() (*analytics.PeriodReport, error)
	DailyBuckets func() ([]analyticsBucket, error)
	OpenBills    func() int
	// Insights receives the precomputed foodcost AND labor reports so the
	// food-cost-high and labor-high checks reuse them instead of recomputing
	// foodcost.Analyze / labor.Analyze — both are computed exactly once.
	Insights func(food *foodcost.Report, lab *labor.Report) []proactiveInsight
	// MarketingLoop returns the S3-Loop posts-this-period summary. Optional —
	// tests may leave it nil (treated as no marketing activity).
	MarketingLoop func() *briefingMarketing
}

// computeDayFraction returns the elapsed fraction of the assumed service window
// (briefingServiceOpenHour..briefingServiceCloseHour in loc), clamped to
// [briefingMinDayFraction, 1.0]. Before open → floor; after close → 1.0. This is
// the divisor behind the "projected daily sales" every POS shows.
func computeDayFraction(now time.Time, loc *time.Location) float64 {
	if loc == nil {
		loc = time.UTC
	}
	local := now.In(loc)
	elapsedHours := float64(local.Hour()) + float64(local.Minute())/60.0 + float64(local.Second())/3600.0 - briefingServiceOpenHour
	windowHours := float64(briefingServiceCloseHour - briefingServiceOpenHour)
	fraction := elapsedHours / windowHours
	if fraction < briefingMinDayFraction {
		return briefingMinDayFraction
	}
	if fraction > 1.0 {
		return 1.0
	}
	return fraction
}

// computePace projects today's running revenue to a full day and compares it to
// the typical-day baseline. Returns (nil, nil) when there is no usable baseline —
// the frontend then renders the actuals without a pace clause, so a quiet day
// never reads as a disaster.
func computePace(todayRevenue float64, typicalDay *float64, dayFraction float64) (projected, pacePct *float64) {
	if typicalDay == nil || *typicalDay <= 0 || dayFraction <= 0 {
		return nil, nil
	}
	// Before the first sale a projection is pure noise — without this guard
	// every pre-open briefing read "on track for ~$0.00, about -100% under a
	// typical day". The frontend falls back to the paceless actuals line.
	if todayRevenue <= 0 {
		return nil, nil
	}
	proj := todayRevenue / dayFraction
	pct := (proj - *typicalDay) / *typicalDay * 100
	return &proj, &pct
}

// typicalDayRevenue is the trailing same-weekday average (up to the most recent
// 4), falling back to the average of the most recent up-to-7 full days when
// same-weekday history is thin, and nil when there is no history at all. Today's
// bucket is always excluded so a partial day never pollutes its own baseline.
func typicalDayRevenue(buckets []analyticsBucket, today time.Time, loc *time.Location) *float64 {
	if loc == nil {
		loc = time.UTC
	}
	todayLocal := today.In(loc)
	targetWeekday := todayLocal.Weekday()

	var sameWeekday []float64
	var recentDays []float64
	for _, b := range buckets {
		bl := b.Date.In(loc)
		if sameCalendarDay(bl, todayLocal) {
			continue // exclude today's partial bucket
		}
		if bl.After(todayLocal) {
			continue // defensive: ignore any future bucket
		}
		recentDays = append(recentDays, b.Revenue)
		if bl.Weekday() == targetWeekday {
			sameWeekday = append(sameWeekday, b.Revenue)
		}
	}

	// Buckets arrive in ascending date order, so the tail is the most recent.
	if n := len(sameWeekday); n > 0 {
		if n > 4 {
			sameWeekday = sameWeekday[n-4:]
		}
		return averagePtr(sameWeekday)
	}
	if n := len(recentDays); n > 0 {
		if n > 7 {
			recentDays = recentDays[n-7:]
		}
		return averagePtr(recentDays)
	}
	return nil
}

// selectPlay returns the single highest-dollar-impact move from the
// menu-engineering classification: a reprice_up on a plowhorse with pricing
// headroom and enough weekly volume, or a promote on an under-ordered, healthy
// puzzle. Weekly impact is scaled to a monthly estimate. Returns nil when no
// candidate clears a positive impact.
func selectPlay(me menuengineering.Report) *briefingPlay {
	var best *briefingPlay
	var bestImpact float64
	medianQty := math.Round(me.MedianQtySold)

	for _, d := range me.Dishes {
		switch d.Action {
		case "reprice_up":
			if d.SuggestedPrice <= d.AvgPrice || d.QtySold < briefingMinPlayQty {
				continue
			}
			// Apply an elasticity haircut so we don't assume every guest keeps
			// buying at the higher price (see repriceElasticityHaircut doc).
			impact := (d.SuggestedPrice - d.AvgPrice) * float64(d.QtySold) * briefingWeekToMonth * (1 - repriceElasticityHaircut)
			if impact > bestImpact {
				cur := d.AvgPrice
				sug := d.SuggestedPrice
				bestImpact = impact
				best = &briefingPlay{
					Kind:           "reprice_up",
					Tab:            "menu",
					ItemName:       d.MenuItemName,
					CurrentPrice:   &cur,
					SuggestedPrice: &sug,
					MonthlyImpact:  round2(impact),
				}
			}
		case "promote":
			if d.MarginPerUnit <= 0 {
				continue
			}
			gap := medianQty - float64(d.QtySold)
			if gap <= 0 {
				continue
			}
			impact := d.MarginPerUnit * gap * briefingWeekToMonth
			if impact > bestImpact {
				bestImpact = impact
				best = &briefingPlay{
					Kind:          "promote",
					Tab:           "menu",
					ItemName:      d.MenuItemName,
					MonthlyImpact: round2(impact),
				}
			}
		}
	}

	if bestImpact <= 0 {
		return nil
	}
	return best
}

// selectWin surfaces a week-over-week revenue win, but only when there is no
// play to lead with (a single forward-looking item beats two).
func selectWin(weekGrowthPct float64, hasPlay bool) *briefingWin {
	if hasPlay || weekGrowthPct < briefingWinMinPct {
		return nil
	}
	return &briefingWin{Kind: "revenue_up_wow", Pct: weekGrowthPct}
}

// briefingState distinguishes a brand-new restaurant (no sales, recipe, or
// payroll history → "learning") from one with enough data for a real read.
func briefingState(todayRevenue, weekRevenue float64, foodHasData, laborHasData bool) string {
	if todayRevenue <= 0 && weekRevenue <= 0 && !foodHasData && !laborHasData {
		return briefingStateLearning
	}
	return briefingStateActive
}

// assembleBriefing builds the briefing from injected providers. foodcost.Report
// is computed exactly once (via providers.FoodCost) and fed to all three
// consumers: the pulse food-cost number, menuengineering.Classify (the play),
// and the proactive-insights food-cost-high check. labor.Report is likewise
// computed once (via providers.Labor) and fed to both the pulse labor number and
// the labor-high check. Provider errors degrade to empty data rather than
// failing the whole briefing — the front door must never collapse to a void.
func assembleBriefing(businessID uint, loc *time.Location, p briefingProviders) briefingResponse {
	now := p.Now()

	foodReport, _ := p.FoodCost() // computed ONCE — see perf contract
	me := menuengineering.Classify(foodReport)
	play := selectPlay(me)

	laborReport, _ := p.Labor()
	today := periodOrEmpty(p.TodayReport)
	week := periodOrEmpty(p.WeekReport)
	buckets, _ := p.DailyBuckets()

	typical := typicalDayRevenue(buckets, now, loc)
	dayFraction := computeDayFraction(now, loc)
	projected, pacePct := computePace(today.TotalRevenue, typical, dayFraction)

	var foodPct *float64
	if foodReport.TotalRevenue > 0 {
		v := foodReport.BlendedFoodCostPct
		foodPct = &v
	}
	var laborPct *float64
	if laborReport.HasData {
		v := laborReport.LaborCostPct
		laborPct = &v
	}

	pulse := briefingPulse{
		Revenue:      today.TotalRevenue,
		Projected:    projected,
		TypicalDay:   typical,
		PacePct:      pacePct,
		Orders:       today.BillCount,
		AvgTicket:    today.AverageTicket,
		FoodCostPct:  foodPct,
		LaborCostPct: laborPct,
		OpenBills:    p.OpenBills(),
		Remaining:    today.FloorRemaining,
	}

	insights := p.Insights(&foodReport, &laborReport)
	// Never emit a nil slice: it serializes to JSON `null`, which the frontend
	// maps over unguarded. A fresh business with no exceptions must send `[]`.
	if insights == nil {
		insights = []proactiveInsight{}
	}

	var win *briefingWin
	if play == nil {
		growth := 0.0
		if week.GrowthRate != nil {
			growth = *week.GrowthRate
		}
		win = selectWin(growth, false)
	}

	state := briefingState(today.TotalRevenue, week.TotalRevenue, len(foodReport.Items) > 0, laborReport.HasData)

	var marketingLoop *briefingMarketing
	if p.MarketingLoop != nil {
		marketingLoop = p.MarketingLoop()
	}

	return briefingResponse{
		State:     state,
		Pulse:     pulse,
		Insights:  insights,
		Play:      play,
		Win:       win,
		Marketing: marketingLoop,
	}
}

// periodOrEmpty calls a period-report provider and degrades a nil/error result
// to a zero report so the assembler never dereferences nil.
func periodOrEmpty(f func() (*analytics.PeriodReport, error)) analytics.PeriodReport {
	r, err := f()
	if err != nil || r == nil {
		return analytics.PeriodReport{}
	}
	return *r
}

func sameCalendarDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func averagePtr(v []float64) *float64 {
	if len(v) == 0 {
		return nil
	}
	var sum float64
	for _, x := range v {
		sum += x
	}
	avg := sum / float64(len(v))
	return &avg
}

func round2(x float64) float64 { return math.Round(x*100) / 100 }
