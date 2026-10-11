package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"
)

// TestGetAiInsights_CountsActiveAndCompletedSessions guards against B4 regressions:
// the metrics endpoint must count *all* AiWaiterConversations for the business
// (both "active" and "closed") and report a non-zero average message count when
// messages exist. A previous bug returned zeros for active-only datasets, even
// though the Recent Conversations list rendered 9+ active sessions.
func TestGetAiInsights_CountsActiveAndCompletedSessions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)

	ownedBusiness := createAIWaiterBusiness(t, "insights-owned", true)
	otherBusiness := createAIWaiterBusiness(t, "insights-other", true)
	manager := createAIWaiterStaff(t, ownedBusiness.ID, database.StaffRoleManager, "insights-manager@example.com")

	// 5 active sessions for the owned business, each with 4 messages.
	for i := 0; i < 5; i++ {
		conv := createAIWaiterConversation(t, ownedBusiness.ID, fmt.Sprintf("owned-active-%d", i))
		for m := 0; m < 4; m++ {
			require.NoError(t, database.SaveAiWaiterMessage(conv.ID, "user", fmt.Sprintf("msg %d-%d", i, m), ""))
		}
	}

	// 3 closed sessions for the owned business, each with 6 messages. Two of
	// them count as successful upsells (cart_items_added > 0).
	for i := 0; i < 3; i++ {
		conv := createAIWaiterConversation(t, ownedBusiness.ID, fmt.Sprintf("owned-closed-%d", i))
		require.NoError(t, database.GetDB().Model(conv).Update("status", "closed").Error)
		if i < 2 {
			require.NoError(t, database.GetDB().Model(conv).Update("cart_items_added", 3).Error)
		}
		for m := 0; m < 6; m++ {
			require.NoError(t, database.SaveAiWaiterMessage(conv.ID, "user", fmt.Sprintf("closed msg %d-%d", i, m), ""))
		}
	}

	// Cross-business noise: another business with active sessions must NOT
	// leak into the owned-business insights.
	for i := 0; i < 4; i++ {
		conv := createAIWaiterConversation(t, otherBusiness.ID, fmt.Sprintf("other-active-%d", i))
		require.NoError(t, database.SaveAiWaiterMessage(conv.ID, "user", "ignore me", ""))
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleManager))
		c.Set("staff_id", manager.ID)
		c.Set("staff_business_id", ownedBusiness.ID)
		c.Next()
	})
	router.GET("/businesses/:id/ai/insights", RoleBasedAccessMiddleware("ai_waiter:read"), GetAiInsights)

	w := performAIWaiterRequest(t, router, http.MethodGet, fmt.Sprintf("/businesses/%d/ai/insights", ownedBusiness.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, "GetAiInsights should return 200 for staff with read permission")

	var resp AiInsightsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// 5 active + 3 closed = 8 total
	assert.EqualValues(t, 8, resp.TotalConversations, "insights must count active AND closed sessions for the business")
	// 5*4 + 3*6 = 20 + 18 = 38 messages
	assert.EqualValues(t, 38, resp.TotalMessages, "insights must count messages across all session statuses")
	// 2 of 8 conversations had cart_items_added > 0 → 25% upsell success
	assert.InDelta(t, 25.0, resp.UpsellSuccessRate, 0.001, "upsell rate must reflect cart_items_added > 0 across all sessions")
}

// TestGetAiInsights_NoLeakAcrossBusinesses guarantees the insights query is
// scoped by business_id (no global counts, no cross-tenant join leaks).
func TestGetAiInsights_NoLeakAcrossBusinesses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)

	scopedBusiness := createAIWaiterBusiness(t, "scope-mine", true)
	noiseBusiness := createAIWaiterBusiness(t, "scope-noise", true)
	manager := createAIWaiterStaff(t, scopedBusiness.ID, database.StaffRoleManager, "scope-manager@example.com")

	// Heavy noise in another business.
	for i := 0; i < 7; i++ {
		conv := createAIWaiterConversation(t, noiseBusiness.ID, fmt.Sprintf("noise-%d", i))
		for m := 0; m < 5; m++ {
			require.NoError(t, database.SaveAiWaiterMessage(conv.ID, "user", "noise", ""))
		}
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleManager))
		c.Set("staff_id", manager.ID)
		c.Set("staff_business_id", scopedBusiness.ID)
		c.Next()
	})
	router.GET("/businesses/:id/ai/insights", RoleBasedAccessMiddleware("ai_waiter:read"), GetAiInsights)

	w := performAIWaiterRequest(t, router, http.MethodGet, fmt.Sprintf("/businesses/%d/ai/insights", scopedBusiness.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)

	var resp AiInsightsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.EqualValues(t, 0, resp.TotalConversations, "noise from other businesses must not appear in scoped insights")
	assert.EqualValues(t, 0, resp.TotalMessages, "messages from other businesses must not appear in scoped insights")
	assert.InDelta(t, 0.0, resp.UpsellSuccessRate, 0.001)
}

// TestGetAiInsights_TimeSeriesShape guards the audit-A2 time-series cards: the
// insights endpoint must return a 7-day zero-filled daily conversation trend and
// a 24-bucket hour-of-day distribution, windowed (7d / 30d) and business-scoped.
func TestGetAiInsights_TimeSeriesShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)

	biz := createAIWaiterBusiness(t, "insights-timeseries", true)
	manager := createAIWaiterStaff(t, biz.ID, database.StaffRoleManager, "ts-manager@example.com")

	now := time.Now().UTC()
	mk := func(suffix string, ts time.Time) {
		c := createAIWaiterConversation(t, biz.ID, "ts-"+suffix)
		require.NoError(t, database.GetDB().Model(c).Update("created_at", ts).Error)
	}
	// Within 7 days: 3 today + 2 three days ago = 5.
	for i := 0; i < 3; i++ {
		mk(fmt.Sprintf("today-%d", i), now)
	}
	for i := 0; i < 2; i++ {
		mk(fmt.Sprintf("d3-%d", i), now.AddDate(0, 0, -3))
	}
	// Within 30 days but outside 7: 1 ten days ago.
	mk("d10", now.AddDate(0, 0, -10))
	// Outside 30 days: 1 forty days ago — must be excluded from the windowed aggregates.
	mk("d40", now.AddDate(0, 0, -40))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleManager))
		c.Set("staff_id", manager.ID)
		c.Set("staff_business_id", biz.ID)
		c.Next()
	})
	router.GET("/businesses/:id/ai/insights", RoleBasedAccessMiddleware("ai_waiter:read"), GetAiInsights)

	w := performAIWaiterRequest(t, router, http.MethodGet, fmt.Sprintf("/businesses/%d/ai/insights", biz.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)

	var resp AiInsightsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.EqualValues(t, 7, resp.TotalConversations, "unbounded total counts every conversation")
	require.Len(t, resp.ConversationTrends7d, 7, "trend must be 7 zero-filled daily buckets")
	require.Len(t, resp.BusiestHours, 24, "busiest hours must be 24 zero-filled buckets")

	trendSum := 0
	for _, p := range resp.ConversationTrends7d {
		trendSum += p.Value
	}
	assert.Equal(t, 5, trendSum, "7-day trend counts only the 5 conversations from the last 7 days")

	hourSum := 0
	for i, h := range resp.BusiestHours {
		assert.Equal(t, i, h.Hour, "hour buckets must be contiguous 0..23")
		hourSum += h.Value
	}
	assert.Equal(t, 6, hourSum, "hour-of-day counts the 6 conversations within the 30-day window (40-days-ago excluded)")
}

// TestGetAiInsights_ConvCountsUseSingleQuery asserts DUP-04: the two separate
// COUNT queries over ai_waiter_conversations (totalConvs and successfulUpsells)
// must be folded into a single conditional-aggregate statement. Before the fix
// there were 2 COUNT statements touching ai_waiter_conversations; after the fix
// there must be exactly 1 statement that contains the conditional aggregate
// (identified by "total_convs" or "sum(case when cart_items_added").
// The test also asserts numeric equality: seed 5 conversations (2 with
// cart_items_added>0) and verify total_conversations=5, upsell_success_rate=40.
func TestGetAiInsights_ConvCountsUseSingleQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupAIWaiterTestDBWithLogger(t, recorder)

	biz := createAIWaiterBusiness(t, "dup04-counts", true)
	manager := createAIWaiterStaff(t, biz.ID, database.StaffRoleManager, "dup04@example.com")

	// 5 conversations: 2 with cart_items_added>0 (upsells), 3 without.
	for i := 0; i < 5; i++ {
		conv := createAIWaiterConversation(t, biz.ID, fmt.Sprintf("dup04-%d", i))
		if i < 2 {
			require.NoError(t, database.GetDB().Model(conv).Update("cart_items_added", 1).Error)
		}
		// Add one message so totalMessages > 0 (exercises the JOIN count path too).
		require.NoError(t, database.SaveAiWaiterMessage(conv.ID, "user", fmt.Sprintf("msg %d", i), ""))
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleManager))
		c.Set("staff_id", manager.ID)
		c.Set("staff_business_id", biz.ID)
		c.Next()
	})
	router.GET("/businesses/:id/ai/insights", RoleBasedAccessMiddleware("ai_waiter:read"), GetAiInsights)

	recorder.statements = nil
	w := performAIWaiterRequest(t, router, http.MethodGet, fmt.Sprintf("/businesses/%d/ai/insights", biz.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Count how many SQL statements touch ai_waiter_conversations for the
	// purpose of aggregating conv counts (the conditional aggregate).  Exclude
	// the createdAts Pluck (which selects "created_at") by requiring the
	// statement to contain the folded aggregate marker.
	convAggregateStmts := 0
	for _, stmt := range recorder.statements {
		lower := strings.ToLower(stmt)
		isConvTable := strings.Contains(lower, "ai_waiter_conversations")
		hasFoldedAggregate := strings.Contains(lower, "total_convs") ||
			strings.Contains(lower, "sum(case when cart_items_added") ||
			strings.Contains(lower, "sum(case when `cart_items_added")
		if isConvTable && hasFoldedAggregate {
			convAggregateStmts++
		}
	}
	// DUP-04: must be exactly ONE conditional-aggregate statement (not two separate COUNTs).
	assert.Equal(t, 1, convAggregateStmts,
		"GetAiInsights must fold totalConvs+successfulUpsells into exactly 1 conditional-aggregate query over ai_waiter_conversations (DUP-04); got statements: %v",
		recorder.statements)

	// Numeric equality: total=5, upsells=2, rate=2/5*100=40.
	var resp AiInsightsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.EqualValues(t, 5, resp.TotalConversations, "total_conversations must equal seed count")
	assert.InDelta(t, 40.0, resp.UpsellSuccessRate, 0.001, "upsell_success_rate must be 2/5*100=40")
}

// BenchmarkGetAiInsights baselines the insights handler with the added
// time-series aggregation over a business with 500 conversations spread across
// the 30-day window (audit A2 performance gate).
func BenchmarkGetAiInsights(b *testing.B) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(b)

	biz := createAIWaiterBusiness(b, "insights-bench", true)
	manager := createAIWaiterStaff(b, biz.ID, database.StaffRoleManager, "bench-manager@example.com")
	now := time.Now().UTC()
	for i := 0; i < 500; i++ {
		c := createAIWaiterConversation(b, biz.ID, fmt.Sprintf("bench-%d", i))
		require.NoError(b, database.GetDB().Model(c).Update("created_at", now.AddDate(0, 0, -(i%30))).Error)
		if i%5 == 0 {
			require.NoError(b, database.SaveAiWaiterMessage(c.ID, "user", "m", ""))
		}
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleManager))
		c.Set("staff_id", manager.ID)
		c.Set("staff_business_id", biz.ID)
		c.Next()
	})
	router.GET("/businesses/:id/ai/insights", RoleBasedAccessMiddleware("ai_waiter:read"), GetAiInsights)
	path := fmt.Sprintf("/businesses/%d/ai/insights", biz.ID)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := performAIWaiterRequest(b, router, http.MethodGet, path, nil)
		if w.Code != http.StatusOK {
			b.Fatalf("unexpected status %d", w.Code)
		}
	}
}
