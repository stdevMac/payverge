package server

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/services/foodcost"
	"github.com/stdevmac/payverge/backend/internal/services/labor"

	"github.com/gin-gonic/gin"
)

// briefingCache holds the assembled Sage briefing per business for a short TTL.
// The briefing fans out across analytics + the foodcost/labor/menuengineering
// calculators (several DB round-trips), and the owner home polls it, so a 60s
// cache is a material win. Writers that change the underlying data should call
// InvalidateBriefingCache. Mirrors the insightCache pattern; conceptually keyed
// "briefing:<businessID>".
type cachedBriefing struct {
	briefing  briefingResponse
	expiresAt time.Time
}

var (
	briefingCache   = map[uint]cachedBriefing{}
	briefingCacheMu sync.RWMutex
)

const briefingCacheTTL = 60 * time.Second

// serverTZCache negative-caches IANA timezone lookups so repeated briefing
// assembly doesn't re-read tzdata. Mirrors the analytics handler's tzCache.
var serverTZCache sync.Map

// InvalidateBriefingCache removes the cached briefing for a business, forcing a
// rebuild on the next request. Safe to call often; no-op when absent.
func InvalidateBriefingCache(businessID uint) {
	briefingCacheMu.Lock()
	delete(briefingCache, businessID)
	briefingCacheMu.Unlock()
}

// invalidateOwnerHomeCaches drops both owner-home caches — the proactive-insight
// cache and the Sage briefing cache that embeds those same insights plus the
// live open-bill count — after a write that changes the underlying signals. The
// briefing is a superset of the insights, so the two always invalidate together;
// writers call this instead of InvalidateInsightCache alone so a future write
// site can't leave the briefing showing stale "Needs you" items for up to 60s.
func invalidateOwnerHomeCaches(businessID uint) {
	InvalidateInsightCache(businessID)
	InvalidateBriefingCache(businessID)
}

func briefingFromCache(businessID uint) (briefingResponse, bool) {
	briefingCacheMu.RLock()
	defer briefingCacheMu.RUnlock()
	if cached, ok := briefingCache[businessID]; ok && time.Now().Before(cached.expiresAt) {
		return cached.briefing, true
	}
	return briefingResponse{}, false
}

func storeBriefingCache(businessID uint, b briefingResponse) {
	briefingCacheMu.Lock()
	briefingCache[businessID] = cachedBriefing{briefing: b, expiresAt: time.Now().Add(briefingCacheTTL)}
	briefingCacheMu.Unlock()
}

// getOrBuildBriefing returns the cached briefing when fresh, otherwise builds it
// via build(), stores it, and returns it. Extracted so the 60s-cache contract is
// testable without a full request.
func getOrBuildBriefing(businessID uint, build func() briefingResponse) briefingResponse {
	if cached, ok := briefingFromCache(businessID); ok {
		return cached
	}
	b := build()
	storeBriefingCache(businessID, b)
	return b
}

// resolveBusinessLocation returns the *time.Location for a business's IANA
// timezone, falling back to UTC when empty or unrecognised. Mirrors the
// analytics handler helper; results are cached (unknown zones negative-cached to
// UTC) so repeated briefing assembly avoids tzdata reads.
func resolveBusinessLocation(business *database.Business) *time.Location {
	if business == nil || business.Timezone == "" {
		return time.UTC
	}
	if cached, ok := serverTZCache.Load(business.Timezone); ok {
		return cached.(*time.Location)
	}
	loc, err := time.LoadLocation(business.Timezone)
	if err != nil {
		serverTZCache.Store(business.Timezone, time.UTC) // negative cache
		return time.UTC
	}
	serverTZCache.Store(business.Timezone, loc)
	return loc
}

// newBriefingProviders wires the real (DB-backed) data closures for the
// assembler. foodcost.Analyze is invoked once (via the FoodCost closure) and its
// result is fed by assembleBriefing to the pulse, the play, and the insights
// food-cost-high check — see the perf contract.
func newBriefingProviders(businessID uint, loc *time.Location) briefingProviders {
	db := database.GetDBWrapper()
	an := analytics.NewAnalyticsService(db)

	return briefingProviders{
		Now: time.Now,
		FoodCost: func() (foodcost.Report, error) {
			report, err := foodcost.NewCalculator(db, an).Analyze(businessID, "week", loc)
			if err != nil {
				logger.Logger.Errorf("briefing: foodcost analyze failed for business %d: %v", businessID, err)
			}
			return report, err
		},
		Labor: func() (labor.Report, error) {
			report, err := labor.NewCalculator(db, an, an).Analyze(businessID, "week", loc)
			if err != nil {
				logger.Logger.Errorf("briefing: labor analyze failed for business %d: %v", businessID, err)
			}
			return report, err
		},
		TodayReport: func() (*analytics.PeriodReport, error) {
			report, err := an.GetPeriodReport(businessID, "today", loc)
			if err != nil {
				logger.Logger.Errorf("briefing: today period report failed for business %d: %v", businessID, err)
			}
			return report, err
		},
		WeekReport: func() (*analytics.PeriodReport, error) {
			report, err := an.GetPeriodReport(businessID, "week", loc)
			if err != nil {
				logger.Logger.Errorf("briefing: week period report failed for business %d: %v", businessID, err)
			}
			return report, err
		},
		DailyBuckets: func() ([]analyticsBucket, error) {
			now := time.Now().In(loc)
			from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -27)
			series, err := an.GetDailySeries(businessID, from, now)
			if err != nil || series == nil {
				if err != nil {
					logger.Logger.Errorf("briefing: daily series failed for business %d: %v", businessID, err)
				}
				return nil, err
			}
			buckets := make([]analyticsBucket, 0, len(series.Buckets))
			for _, b := range series.Buckets {
				day, perr := time.ParseInLocation("2006-01-02", b.Date, loc)
				if perr != nil {
					continue
				}
				buckets = append(buckets, analyticsBucket{Date: day, Revenue: b.Revenue})
			}
			return buckets, nil
		},
		OpenBills: func() int {
			if db == nil {
				return 0
			}
			// Use the dedicated COUNT rather than len(summaries): the summary
			// fetch is now bounded (DefaultActiveBillSummaryLimit) for the live
			// board, so counting its rows would undercount a very busy business.
			n, err := db.GetActiveBillCountByBusinessID(businessID)
			if err != nil {
				logger.Logger.Errorf("briefing: active bill count failed for business %d: %v", businessID, err)
				return 0
			}
			return int(n)
		},
		Insights: func(food *foodcost.Report, lab *labor.Report) []proactiveInsight {
			return buildProactiveInsightsWithReports(businessID, food, lab)
		},
		MarketingLoop: func() *briefingMarketing {
			return buildBriefingMarketing(businessID)
		},
	}
}

// marketingPostsPeriodDays is the lookback window for Director marketing
// closed-loop surfaces (briefing panel + proactive insight). Not a schedule —
// operators post when ready; this is only "posts recorded in the last N days".
const marketingPostsPeriodDays = 7

// buildBriefingMarketing loads posted marketing activity for the period and
// returns a non-nil panel only when at least one post was recorded. Failures
// degrade to nil so the front door never collapses.
func buildBriefingMarketing(businessID uint) *briefingMarketing {
	db := database.GetDBWrapper()
	if db == nil {
		return nil
	}
	since := time.Now().UTC().AddDate(0, 0, -marketingPostsPeriodDays)
	period, err := db.RecentMarketingPosts(businessID, since, 5)
	if err != nil {
		logger.Logger.Errorf("briefing: marketing posts query failed for business %d: %v", businessID, err)
		return nil
	}
	if period.Count < 1 {
		return nil
	}
	titles := period.RecentTitles
	if titles == nil {
		titles = []string{}
	}
	channels := period.Channels
	if channels == nil {
		channels = []string{}
	}
	return &briefingMarketing{
		PostsThisPeriod: int(period.Count),
		PeriodDays:      marketingPostsPeriodDays,
		RecentTitles:    titles,
		Channels:        channels,
		Tab:             "marketing",
	}
}

// GetDirectorBriefing returns the always-present, GM-voiced briefing for the
// owner home: pulse + insights + one play (or win).
// GET /api/v1/inside/businesses/:id/director-console/briefing
//
// Gating is identical to proactive-insights: owner token + operational
// (not suspended or closed) business. The assembled briefing is cached 60s (briefing:<businessID>).
func GetDirectorBriefing(c *gin.Context) {
	if !ensureOwnerToken(c) {
		return
	}

	businessIDStr := c.Param("id")
	businessID64, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}
	businessID := uint(businessID64)
	if !ensureDirectorFeatureAvailable(c, businessID) {
		return
	}

	if cached, ok := briefingFromCache(businessID); ok {
		c.JSON(http.StatusOK, cached)
		return
	}

	business, err := database.GetBusinessByID(businessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}
	loc := resolveBusinessLocation(business)

	briefing := getOrBuildBriefing(businessID, func() briefingResponse {
		return assembleBriefing(businessID, loc, newBriefingProviders(businessID, loc))
	})

	c.JSON(http.StatusOK, briefing)
}
