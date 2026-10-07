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

func aiConversationsRouter(role database.StaffRole, staffID, businessID uint) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(role))
		c.Set("staff_id", staffID)
		c.Set("staff_business_id", businessID)
		c.Next()
	})
	router.GET("/businesses/:id/ai/conversations", RoleBasedAccessMiddleware("ai_waiter:read"), GetAiConversations)
	return router
}

// TestGetAiConversations_NoBusinessBlobInPayload guards fix 8: each conversation
// row must NOT serialize a full zero-value Business — the embed is now a pointer
// so omitempty drops it.
func TestGetAiConversations_NoBusinessBlobInPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)

	biz := createAIWaiterBusiness(t, "conv-payload", true)
	manager := createAIWaiterStaff(t, biz.ID, database.StaffRoleManager, "payload-mgr@example.com")
	createAIWaiterConversation(t, biz.ID, "conv-payload-1")

	w := performAIWaiterRequest(t, aiConversationsRouter(database.StaffRoleManager, manager.ID, biz.ID),
		http.MethodGet, fmt.Sprintf("/businesses/%d/ai/conversations", biz.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// The response must not embed a nested business object (only business_id).
	assert.NotContains(t, w.Body.String(), `"business":{`,
		"conversation rows must not serialize a nested Business blob")
	assert.Contains(t, w.Body.String(), `"business_id"`)
}

// TestGetAiConversations_StatusFilterAndActiveCount guards fix 5 (status filter)
// and fix 11 (real active_count independent of the page/filter).
func TestGetAiConversations_StatusFilterAndActiveCount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)

	biz := createAIWaiterBusiness(t, "conv-status", true)
	manager := createAIWaiterStaff(t, biz.ID, database.StaffRoleManager, "status-mgr@example.com")

	// 3 active + 2 closed.
	for i := 0; i < 3; i++ {
		createAIWaiterConversation(t, biz.ID, fmt.Sprintf("active-%d", i))
	}
	for i := 0; i < 2; i++ {
		conv := createAIWaiterConversation(t, biz.ID, fmt.Sprintf("closed-%d", i))
		require.NoError(t, database.GetDB().Model(conv).Update("status", "closed").Error)
	}

	router := aiConversationsRouter(database.StaffRoleManager, manager.ID, biz.ID)

	// No status filter (legacy): all 5, active_count=3.
	w := performAIWaiterRequest(t, router, http.MethodGet,
		fmt.Sprintf("/businesses/%d/ai/conversations", biz.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)
	var all GetAiConversationsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &all))
	assert.EqualValues(t, 5, all.TotalCount, "no filter returns all statuses")
	assert.EqualValues(t, 3, all.ActiveCount, "active_count is the true active total")

	// status=active: only 3 rows, total_count matches the filter, paginator truthful.
	w = performAIWaiterRequest(t, router, http.MethodGet,
		fmt.Sprintf("/businesses/%d/ai/conversations?status=active", biz.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)
	var active GetAiConversationsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &active))
	require.Len(t, active.Conversations, 3)
	assert.EqualValues(t, 3, active.TotalCount, "status filter narrows total_count so pages are honest")
	for _, cv := range active.Conversations {
		assert.Equal(t, "active", cv.Status)
	}
	assert.EqualValues(t, 3, active.ActiveCount)

	// status=closed: only the 2 closed rows.
	w = performAIWaiterRequest(t, router, http.MethodGet,
		fmt.Sprintf("/businesses/%d/ai/conversations?status=closed", biz.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)
	var closed GetAiConversationsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &closed))
	require.Len(t, closed.Conversations, 2)
	assert.EqualValues(t, 2, closed.TotalCount)
}

func TestGetAiConversations_EmptyListIsJSONArrayNotNull(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "conv-empty-json", true)
	manager := createAIWaiterStaff(t, biz.ID, database.StaffRoleManager, "empty-json-mgr@example.com")

	w := performAIWaiterRequest(t, aiConversationsRouter(database.StaffRoleManager, manager.ID, biz.ID),
		http.MethodGet, fmt.Sprintf("/businesses/%d/ai/conversations", biz.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"conversations":[]`)
	assert.NotContains(t, w.Body.String(), `"conversations":null`)
	var resp GetAiConversationsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Conversations)
	assert.Empty(t, resp.Conversations)
}

func aiMessagesRouter(businessID uint) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleManager))
		c.Set("staff_business_id", businessID)
		c.Next()
	})
	router.GET("/businesses/:id/ai/conversations/:convId/messages",
		RoleBasedAccessMiddleware("ai_waiter:read"), GetAiConversationMessages)
	return router
}

// TestGetAiConversationMessages_SinceCursor guards fix 6: an optional `since`
// returns only messages created after the cursor (incremental), while the
// default still returns the full bounded transcript.
func TestGetAiConversationMessages_SinceCursor(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)

	biz := createAIWaiterBusiness(t, "since", true)
	manager := createAIWaiterStaff(t, biz.ID, database.StaffRoleManager, "since-mgr@example.com")
	conv := createAIWaiterConversation(t, biz.ID, "since-conv")
	_ = manager

	base := time.Now().Add(-time.Hour).UTC()
	for i := 0; i < 5; i++ {
		require.NoError(t, database.SaveAiWaiterMessage(conv.ID, "user", fmt.Sprintf("m%d", i), ""))
	}
	// Force deterministic timestamps: msg 0..4 at base+i minutes.
	var msgs []database.AiWaiterMessage
	require.NoError(t, database.GetDB().Where("conversation_id = ?", conv.ID).Order("id ASC").Find(&msgs).Error)
	require.Len(t, msgs, 5)
	for i := range msgs {
		require.NoError(t, database.GetDB().Model(&database.AiWaiterMessage{}).
			Where("id = ?", msgs[i].ID).Update("created_at", base.Add(time.Duration(i)*time.Minute)).Error)
	}

	router := aiMessagesRouter(biz.ID)

	// Full transcript (no since).
	w := performAIWaiterRequest(t, router, http.MethodGet,
		fmt.Sprintf("/businesses/%d/ai/conversations/%d/messages", biz.ID, conv.ID), nil)
	require.Equal(t, http.StatusOK, w.Code)
	var full []database.AiWaiterMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &full))
	require.Len(t, full, 5)

	// since = base+2min30s → only msgs 3 and 4 (created strictly after).
	since := base.Add(2*time.Minute + 30*time.Second).Format(time.RFC3339Nano)
	w = performAIWaiterRequest(t, router, http.MethodGet,
		fmt.Sprintf("/businesses/%d/ai/conversations/%d/messages?since=%s", biz.ID, conv.ID, since), nil)
	require.Equal(t, http.StatusOK, w.Code)
	var incremental []database.AiWaiterMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &incremental))
	require.Len(t, incremental, 2, "since cursor returns only newer messages")
	assert.Equal(t, "m3", incremental[0].Content)
	assert.Equal(t, "m4", incremental[1].Content)
}

// TestGetAiInsights_NoTimestampPluck guards fix 4's access shape: the insights
// endpoint must NOT pluck the raw created_at rows into Go — the timeseries must
// come from GROUP BY aggregates. We assert no statement selects created_at from
// ai_waiter_conversations without an aggregate (COUNT/GROUP).
func TestGetAiInsights_NoTimestampPluck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupAIWaiterTestDBWithLogger(t, recorder)

	biz := createAIWaiterBusiness(t, "no-pluck", true)
	manager := createAIWaiterStaff(t, biz.ID, database.StaffRoleManager, "nopluck-mgr@example.com")
	now := time.Now().UTC()
	for i := 0; i < 20; i++ {
		conv := createAIWaiterConversation(t, biz.ID, fmt.Sprintf("np-%d", i))
		require.NoError(t, database.GetDB().Model(conv).Update("created_at", now.AddDate(0, 0, -(i%10))).Error)
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
	w := performAIWaiterRequest(t, router, http.MethodGet,
		fmt.Sprintf("/businesses/%d/ai/insights", biz.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// No bare "SELECT created_at FROM ai_waiter_conversations" plucking every
	// timestamp. The attribution query legitimately references the conv table in
	// an EXISTS subquery but its top-level FROM is bill_items — skip it — and
	// then any statement reading created_at off the conv table must aggregate.
	for _, stmt := range recorder.statements {
		lower := strings.ToLower(strings.Join(strings.Fields(stmt), " "))
		if strings.Contains(lower, "bill_items") {
			continue // attribution aggregate, not a conv-table pluck
		}
		if !strings.Contains(lower, "ai_waiter_conversations") {
			continue
		}
		if strings.Contains(lower, "created_at") {
			// A read of the conv table that touches created_at must aggregate
			// (GROUP BY buckets) — never pluck raw rows.
			assert.Contains(t, lower, "group by",
				"timeseries read must aggregate created_at, not pluck rows; got: %s", stmt)
		}
	}

	var resp AiInsightsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.ConversationTrends7d, 7)
	require.Len(t, resp.BusiestHours, 24)
	assert.EqualValues(t, 20, resp.Conversations30d)
}

// TestGetAiInsights_ServesFromCacheWithinTTL guards fix 4's 60s cache: a second
// call within the TTL must not re-run the aggregation queries.
func TestGetAiInsights_ServesFromCacheWithinTTL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupAIWaiterTestDBWithLogger(t, recorder)

	biz := createAIWaiterBusiness(t, "cache-ttl", true)
	manager := createAIWaiterStaff(t, biz.ID, database.StaffRoleManager, "cache-mgr@example.com")
	createAIWaiterConversation(t, biz.ID, "cache-conv")

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

	// First call: builds and caches.
	w := performAIWaiterRequest(t, router, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, w.Code)

	// Second call within TTL: the aggregation queries must NOT re-run. Auth
	// (staff-permission + business lookup) still reads the DB — that's unrelated
	// to the insights build — so we assert specifically that the folded
	// conversation-count aggregate (the marker of a rebuild) did not fire.
	recorder.statements = nil
	w = performAIWaiterRequest(t, router, http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, w.Code)
	for _, stmt := range recorder.statements {
		lower := strings.ToLower(stmt)
		assert.NotContains(t, lower, "total_convs",
			"insights aggregation must not re-run within the cache TTL; got: %s", stmt)
		assert.NotContains(t, lower, "group by",
			"timeseries aggregation must not re-run within the cache TTL; got: %s", stmt)
	}
}
