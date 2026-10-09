package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services/foodcost"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetBriefingCacheForTest wipes the package-level briefing cache between
// assertions so sibling tests don't leak state (the test binary shares memory).
func resetBriefingCacheForTest(t *testing.T) {
	t.Helper()
	briefingCacheMu.Lock()
	briefingCache = map[uint]cachedBriefing{}
	briefingCacheMu.Unlock()
}

// ---- Gating: owner token + operational business, mirroring proactive-insights

func TestGetDirectorBriefing_DeniesNonOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "briefing-non-owner")

	// Staff token must be rejected before any work.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set("token_type", "staff")
	c.Set("address", "0xOwnerA")

	GetDirectorBriefing(c)
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

	// A different owner (cross-tenant) must be denied by CheckBusinessAccess.
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c2.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c2.Set("token_type", "web3")
	c2.Set("address", "0xOwnerB")

	GetDirectorBriefing(c2)
	assert.Equal(t, http.StatusForbidden, w2.Code, w2.Body.String())
}

func TestGetDirectorBriefing_RejectsSuspendedBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "briefing-core")
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		UpdateColumn("is_active", false).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set("token_type", "web3")
	c.Set("address", "0xOwnerA") // matches OwnerAddress so it passes CheckBusinessAccess

	GetDirectorBriefing(c)

	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "business_suspended")
}

// TestGetDirectorBriefing_GatedByOperationalMiddleware mirrors the
// proactive-insights route-level gate: a suspended business hitting the
// registered route gets the structured 403 business_suspended contract.
func TestGetDirectorBriefing_GatedByOperationalMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "briefing-route-gate")
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		UpdateColumn("is_active", false).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xOwnerA")
		c.Next()
	})
	group := router.Group("/businesses/:id", RequireOperationalBusiness())
	group.GET("/director-console/briefing", RoleBasedAccessMiddleware("director:read"), GetDirectorBriefing)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/businesses/%d/director-console/briefing", business.ID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"code":"business_suspended"`)
}

// ---- 60s cache: second call within TTL does not rebuild ------------------

func TestGetOrBuildBriefing_ServesFromCacheWithinTTL(t *testing.T) {
	resetBriefingCacheForTest(t)

	var builds int
	build := func() briefingResponse {
		builds++
		return briefingResponse{State: "active", Pulse: briefingPulse{Revenue: 4200}}
	}

	first := getOrBuildBriefing(77, build)
	second := getOrBuildBriefing(77, build)

	assert.Equal(t, 1, builds, "second call within the TTL must be served from cache, not rebuilt")
	assert.Equal(t, "active", second.State)
	assert.InDelta(t, 4200.0, second.Pulse.Revenue, 1e-9)
	assert.Equal(t, first, second)
}

func TestGetOrBuildBriefing_RebuildsAfterInvalidate(t *testing.T) {
	resetBriefingCacheForTest(t)

	var builds int
	build := func() briefingResponse {
		builds++
		return briefingResponse{State: "active"}
	}

	getOrBuildBriefing(88, build)
	InvalidateBriefingCache(88)
	getOrBuildBriefing(88, build)

	assert.Equal(t, 2, builds, "invalidation forces a rebuild")
}

// invalidateOwnerHomeCaches must clear BOTH owner-home caches — the briefing
// (which embeds the insights + live open-bill count) and the proactive-insight
// cache — so a write that fires the existing InvalidateInsightCache call sites no
// longer leaves the briefing serving stale "Needs you" items for up to 60s.
func TestInvalidateOwnerHomeCachesClearsBothCaches(t *testing.T) {
	resetBriefingCacheForTest(t)
	const bid = uint(99)

	storeBriefingCache(bid, briefingResponse{State: "active"})
	insightCacheMu.Lock()
	insightCache[bid] = cachedInsight{insights: []proactiveInsight{{ID: "x"}}, expiresAt: time.Now().Add(time.Minute)}
	insightCacheMu.Unlock()
	t.Cleanup(func() { InvalidateInsightCache(bid) })

	if _, ok := briefingFromCache(bid); !ok {
		t.Fatal("briefing cache should be seeded")
	}

	invalidateOwnerHomeCaches(bid)

	_, briefingPresent := briefingFromCache(bid)
	assert.False(t, briefingPresent, "briefing cache must be cleared")
	insightCacheMu.RLock()
	_, insightPresent := insightCache[bid]
	insightCacheMu.RUnlock()
	assert.False(t, insightPresent, "insight cache must be cleared")
}

// ---- foodcost-once: the insight path reuses the precomputed report -------

// TestBuildProactiveInsightsWithFood_ReusesPrecomputedReport proves the
// food-cost-high section is derived from the precomputed foodcost report rather
// than recomputing foodcost.Analyze. With no recipe/analytics data in the DB the
// recomputing (nil) path produces nothing, while the precomputed path surfaces
// the high-food-cost item.
func TestBuildProactiveInsightsWithFood_ReusesPrecomputedReport(t *testing.T) {
	setupStaffHandlerTestDB(t)
	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "briefing-food-reuse")

	report := foodcost.Report{
		BlendedFoodCostPct: 0.5,
		TotalRevenue:       1000,
		Items: []foodcost.ItemMargin{
			{MenuItemName: "Lobster Roll", HasCompleteCost: true, QtySold: 10, AvgPrice: 30, FoodCostPct: 0.5},
		},
	}

	// Precomputed path: the high-food-cost item surfaces from the report.
	withFood := buildProactiveInsightsWithReports(business.ID, &report, nil)
	var foodInsight *proactiveInsight
	for i := range withFood {
		if withFood[i].Type == "food_cost_high" {
			foodInsight = &withFood[i]
			break
		}
	}
	require.NotNil(t, foodInsight, "precomputed report should surface a food_cost_high insight")
	assert.Equal(t, 1, foodInsight.Params["count"])

	// Recomputing path (nil): with no recipe data the foodcost calc yields
	// nothing, so no food_cost_high insight — confirming the surfaced insight
	// above came from the precomputed report, not a recompute.
	nilPath := buildProactiveInsightsWithReports(business.ID, nil, nil)
	for _, ins := range nilPath {
		assert.NotEqual(t, "food_cost_high", ins.Type, "nil path must not surface food_cost_high without recipe data")
	}
}

// TestResolveBusinessLocation covers the server-package timezone helper used by
// the briefing for window/pace math.
func TestResolveBusinessLocation(t *testing.T) {
	assert.Equal(t, time.UTC, resolveBusinessLocation(nil))
	assert.Equal(t, time.UTC, resolveBusinessLocation(&database.Business{Timezone: ""}))

	ny := resolveBusinessLocation(&database.Business{Timezone: "America/New_York"})
	require.NotNil(t, ny)
	assert.Equal(t, "America/New_York", ny.String())

	// Unrecognised zone negative-caches to UTC.
	assert.Equal(t, time.UTC, resolveBusinessLocation(&database.Business{Timezone: "Not/AZone"}))
}
