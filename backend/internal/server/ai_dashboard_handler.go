package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/services"
	operational_alerts "github.com/stdevmac/payverge/backend/internal/services/operational_alerts"
	"github.com/stdevmac/payverge/backend/internal/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const maxAIDashboardTranscriptMessages = 200

const aiClaimIdleTTL = 5 * time.Minute

// aiInsightsCacheTTL is the short server-side cache window for the AI-waiter
// insights aggregation. The monitor tab re-hit this endpoint on every
// pagination click, re-running the message JOIN + timeseries aggregation each
// time; a 60s cache (mirrors the briefing cache) collapses that to one build
// per minute per business. Keyed "ai-insights:<businessID>".
const aiInsightsCacheTTL = 60 * time.Second

type cachedAiInsights struct {
	payload   AiInsightsResponse
	expiresAt time.Time
}

var (
	aiInsightsCache   = map[uint]cachedAiInsights{}
	aiInsightsCacheMu sync.RWMutex
)

func aiInsightsFromCache(businessID uint) (AiInsightsResponse, bool) {
	aiInsightsCacheMu.RLock()
	defer aiInsightsCacheMu.RUnlock()
	if cached, ok := aiInsightsCache[businessID]; ok && time.Now().Before(cached.expiresAt) {
		return cached.payload, true
	}
	return AiInsightsResponse{}, false
}

func storeAiInsightsCache(businessID uint, payload AiInsightsResponse) {
	aiInsightsCacheMu.Lock()
	aiInsightsCache[businessID] = cachedAiInsights{payload: payload, expiresAt: time.Now().Add(aiInsightsCacheTTL)}
	aiInsightsCacheMu.Unlock()
}

// aiActorIdentity extracts the acting principal. Staff carry staff_id/name/role;
// owner principals (no staff_role) are labeled "owner".
func aiActorIdentity(c *gin.Context) (staffID *uint, name, role string) {
	if v, ok := c.Get("staff_id"); ok {
		if id, ok := v.(uint); ok {
			sid := id
			staffID = &sid
		}
	}
	if v, ok := c.Get("staff_name"); ok {
		if s, ok := v.(string); ok {
			name = s
		}
	}
	if v, ok := c.Get("staff_role"); ok {
		if s, ok := v.(string); ok {
			role = s
		}
	}
	if role == "" {
		role = "owner"
		if name == "" {
			name = "Owner"
		}
	}
	return
}

// aiActorCanStealClaim reports whether the caller may *explicitly* seize a
// chat already held by someone else (with audit + notify). Owners (no staff
// role) and managers (ai_waiter:write) can; front-line staff cannot. Silent
// concurrent access is no longer allowed — steal must be deliberate.
func aiActorCanStealClaim(c *gin.Context) bool {
	roleVal, ok := c.Get("staff_role")
	if !ok {
		return true // owner principal
	}
	rs, _ := roleVal.(string)
	for _, p := range StaffRolePermissions[database.StaffRole(rs)] {
		if p == PermAIWaiterWrite {
			return true
		}
	}
	return false
}

// aiActorCanOverrideClaim is retained as a thin alias for call sites still
// naming the old "release any" path. Steal-capable actors may force-release.
func aiActorCanOverrideClaim(c *gin.Context) bool {
	return aiActorCanStealClaim(c)
}

// resolveAITakeoverAlertQuietly resolves the ai_takeover alert for a
// conversation after any exit path that puts a human answer in front of the
// guest or hands the chat back to the AI (claim, reply, unpause, release,
// close, stale-claim sweep). Resolving a missing/already-resolved alert is not
// an error. Detached from the request context so a client disconnect after the
// state change committed cannot leave the alert stuck open; failures are
// logged, never fatal to the request.
func resolveAITakeoverAlertQuietly(ctx context.Context, businessID, convID uint, actor operational_alerts.Actor) {
	if err := operational_alerts.NewService(database.GetDB()).ResolveAITakeoverAlert(
		context.WithoutCancel(ctx), businessID, int64(convID), actor,
	); err != nil {
		log.Printf("ai takeover alert resolve failed: conv=%d err=%v", convID, err)
	}
}

// ReleaseStaleAiClaims releases takeover claims idle past aiClaimIdleTTL so
// abandoned chats return to the AI and become re-claimable, and resolves the
// orphaned ai_takeover alerts on those conversations (the guest is back with
// the AI; nobody is "taking over" anymore). Keyed on claimed_at, not the staff
// id, so owner claims (NULL claimed_by_staff_id) expire too. businessID 0
// sweeps every business (the background ticker); a non-zero id scopes the
// sweep to one venue (the conversation list poll). The release is one batched
// UPDATE guarded on claimed_at so a claim refreshed after the read survives,
// and alert resolution is one keyed lookup per business. Returns the number
// of conversations released.
func ReleaseStaleAiClaims(ctx context.Context, businessID uint, now time.Time) (int, error) {
	db := database.GetDB().WithContext(ctx)
	idleCutoff := now.Add(-aiClaimIdleTTL)
	q := db.Model(&database.AiWaiterConversation{}).
		Select("id", "business_id").
		Where("claimed_at IS NOT NULL AND claimed_at < ?", idleCutoff)
	if businessID > 0 {
		q = q.Where("business_id = ?", businessID)
	}
	var stale []struct {
		ID         uint
		BusinessID uint
	}
	if err := database.ExcludeAiWaiterOperatorTestConversations(q).Find(&stale).Error; err != nil {
		return 0, err
	}
	if len(stale) == 0 {
		return 0, nil
	}
	ids := make([]uint, 0, len(stale))
	byBusiness := make(map[uint][]uint)
	for _, row := range stale {
		ids = append(ids, row.ID)
		byBusiness[row.BusinessID] = append(byBusiness[row.BusinessID], row.ID)
	}
	if err := db.Model(&database.AiWaiterConversation{}).
		Where("id IN ? AND claimed_at < ?", ids, idleCutoff).
		Updates(map[string]interface{}{
			"claimed_by_staff_id": nil, "claimed_by_name": "", "claimed_by_role": "",
			"claimed_at": nil, "is_paused": false,
		}).Error; err != nil {
		return 0, err
	}
	alerts := operational_alerts.NewService(database.GetDB())
	for bizID, convIDs := range byBusiness {
		if err := alerts.ResolveAlertsForResources(context.WithoutCancel(ctx), bizID,
			database.OperationalAlertResourceTypeAIConversation, convIDs,
			operational_alerts.Actor{Name: "system"}, "claimed"); err != nil {
			log.Printf("ai takeover alert batch resolve failed: business=%d err=%v", bizID, err)
		}
	}
	return len(stale), nil
}

// resolveBusinessFromIDParam reads the :id route parameter, which can be
// either a numeric DB id or the public business_id slug, and returns the
// matching Business. On any error it writes the appropriate JSON response
// and returns ok=false so the caller can early-return.
func resolveBusinessFromIDParam(c *gin.Context) (*database.Business, bool) {
	identifier := utils.BusinessIdentifierFromParam(c, "id")
	if identifier == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return nil, false
	}
	business, err := database.GetBusinessByIdOrBusinessId(identifier)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve business"})
		}
		return nil, false
	}
	return business, true
}

// GetAiConversationsResponse represents the paginated response for conversations
type GetAiConversationsResponse struct {
	Conversations []database.AiWaiterConversation `json:"conversations"`
	Page          int                             `json:"page"`
	TotalPages    int                             `json:"total_pages"`
	TotalCount    int64                           `json:"total_count"`
	// ActiveCount is the real number of active conversations for the business —
	// a cheap COUNT that rides the list response so the "Live chats" stat can
	// reflect the true active total instead of counting the visible page.
	ActiveCount int64 `json:"active_count"`
}

func loadScopedAiConversation(c *gin.Context) (*database.AiWaiterConversation, bool) {
	business, ok := resolveBusinessFromIDParam(c)
	if !ok {
		return nil, false
	}

	convIDStr := c.Param("convId")
	convID, err := strconv.ParseUint(convIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid conversation ID"})
		return nil, false
	}

	var conv database.AiWaiterConversation
	err = database.GetDB().Where("id = ? AND business_id = ?", convID, business.ID).First(&conv).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Conversation not found"})
			return nil, false
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch conversation"})
		return nil, false
	}
	if database.IsAiWaiterOperatorTestTableCode(conv.TableCode) {
		// Sandbox rows are not Live Monitor conversations.
		c.JSON(http.StatusNotFound, gin.H{"error": "Conversation not found"})
		return nil, false
	}

	return &conv, true
}

// GetAiConversations retrieves paginated conversation history for a business
// GET /api/v1/business/:id/ai/conversations
func GetAiConversations(c *gin.Context) {
	business, ok := resolveBusinessFromIDParam(c)
	if !ok {
		return
	}
	businessID := business.ID

	// Pagination
	pageStr := c.DefaultQuery("page", "1")
	page, _ := strconv.Atoi(pageStr)
	if page < 1 {
		page = 1
	}
	limit := 20
	offset := (page - 1) * limit

	// Optional status filter (BE-first: absent => all statuses, the legacy
	// behavior). Whitelisted so the client can't inject arbitrary predicates.
	// Front-line staff sessions request status=active so the paginator is
	// truthful (previously they client-filtered a server page of ALL statuses,
	// producing "500 pages, all empty"); owners/managers get the control for free.
	statusFilter := strings.ToLower(strings.TrimSpace(c.Query("status")))
	if statusFilter != "active" && statusFilter != "closed" {
		statusFilter = ""
	}

	conversations := make([]database.AiWaiterConversation, 0)
	var totalcount int64

	// Fetch data
	db := database.GetDB()

	// Lazily release claims idle past the TTL for this business. The
	// background sweep (ReleaseStaleAiClaims, started from main) normally gets
	// there first, so this is one narrow indexed read with nothing to write.
	if _, err := ReleaseStaleAiClaims(c.Request.Context(), businessID, time.Now()); err != nil {
		log.Printf("ai stale claim sweep failed: business=%d err=%v", businessID, err)
	}

	// Build the scoped base query once (business + optional status) so the COUNT
	// and the page read use the identical predicate — otherwise total_pages and
	// the returned rows disagree when a status filter is active.
	scoped := func() *gorm.DB {
		q := database.ExcludeAiWaiterOperatorTestConversations(
			db.Model(&database.AiWaiterConversation{}).Where("business_id = ?", businessID),
		)
		if statusFilter != "" {
			q = q.Where("status = ?", statusFilter)
		}
		return q
	}

	if err := scoped().Count(&totalcount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch conversations"})
		return
	}
	if err := scoped().Order("updated_at desc").Limit(limit).Offset(offset).Find(&conversations).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch conversations"})
		return
	}

	// Real active count for the "Live chats" stat — a cheap COUNT independent of
	// the current page/filter, so it stays truthful even on page 2 or a closed
	// filter. Operator sandbox rows are excluded so test chat never inflates
	// Live Monitor.
	var activeCount int64
	database.ExcludeAiWaiterOperatorTestConversations(
		db.Model(&database.AiWaiterConversation{}).
			Where("business_id = ? AND status = ?", businessID, "active"),
	).Count(&activeCount)

	totalPages := int((totalcount + int64(limit) - 1) / int64(limit))

	c.JSON(http.StatusOK, GetAiConversationsResponse{
		Conversations: conversations,
		Page:          page,
		TotalPages:    totalPages,
		TotalCount:    totalcount,
		ActiveCount:   activeCount,
	})
}

// GetAiConversationMessages retrieves the full transcript for a specific conversation
// GET /api/v1/business/:id/ai/conversations/:convId/messages
func GetAiConversationMessages(c *gin.Context) {
	conv, ok := loadScopedAiConversation(c)
	if !ok {
		return
	}

	limit := maxAIDashboardTranscriptMessages
	if requestedLimit, err := strconv.Atoi(c.Query("limit")); err == nil && requestedLimit > 0 && requestedLimit < limit {
		limit = requestedLimit
	}

	// Optional `since` cursor (RFC3339) for incremental polling: the operator
	// transcript modal polls every few seconds, and re-downloading the full
	// 200-message payload each time is wasteful. When present, return only the
	// messages created strictly after `since` (chronological), so the client can
	// append. Absent => the legacy full bounded transcript (BE-first).
	var since *time.Time
	if raw := strings.TrimSpace(c.Query("since")); raw != "" {
		if ts, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			since = &ts
		} else if ts, err := time.Parse(time.RFC3339, raw); err == nil {
			since = &ts
		}
	}

	// The masking/projection contract lives in GetAiWaiterMessagesForTranscript
	// (the guest path already uses its `since` support); the access-shape and
	// masking regression tests guard it.
	messages, err := database.GetAiWaiterMessagesForTranscript(conv.ID, since, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch messages"})
		return
	}

	c.JSON(http.StatusOK, messages)
}

// aiTrendPoint is one day in the 7-day conversation trend.
type aiTrendPoint struct {
	Date  string `json:"date"` // YYYY-MM-DD (business-local calendar day)
	Value int    `json:"value"`
}

// aiHourPoint is one hour-of-day bucket (0-23) in the busiest-hours distribution.
type aiHourPoint struct {
	Hour  int `json:"hour"`
	Value int `json:"value"`
}

// Helper to provide simple insights
type AiInsightsResponse struct {
	TotalConversations   int64          `json:"total_conversations"`
	TotalMessages        int64          `json:"total_messages"`
	UpsellSuccessRate    float64        `json:"upsell_success_rate"`
	ConversationTrends7d []aiTrendPoint `json:"conversation_trends_7d"`
	BusiestHours         []aiHourPoint  `json:"busiest_hours"`
	// 30-day window (same window as the timeseries below).
	Conversations30d int64 `json:"conversations_30d"`
	// Bill-level attribution: value of guest orders placed during/just after
	// AI conversations that added cart items. Correlation, not per-item
	// causation — UI labels must reflect that.
	AttributedOrders30d  int64   `json:"attributed_orders_30d"`
	AttributedRevenue30d float64 `json:"attributed_revenue_30d"`
}

// GetAiInsights retrieves aggregated stats
// GET /api/v1/business/:id/ai/insights
func GetAiInsights(c *gin.Context) {
	business, ok := resolveBusinessFromIDParam(c)
	if !ok {
		return
	}
	businessID := business.ID

	// Short server-side cache: the monitor tab re-hits this endpoint on every
	// pagination click. Serve a fresh (≤60s) build without re-running the
	// aggregation each time.
	if cached, ok := aiInsightsFromCache(businessID); ok {
		c.JSON(http.StatusOK, cached)
		return
	}

	db := database.GetDB()

	// Each read below feeds one insights payload. A failed query must surface a
	// 500 instead of silently emitting zero counts / empty buckets, which read as
	// "no activity" and hide a real DB fault from operators and monitoring.
	fail := func(err error, op string) bool {
		if err != nil {
			log.Printf("[ai-insights] %s failed (business=%d): %v", op, businessID, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load insights"})
			return true
		}
		return false
	}

	// DUP-04: fold totalConvs + successfulUpsells into one conditional-aggregate
	// query instead of two separate COUNTs over the same table. COALESCE+SUM(CASE)
	// is portable across SQLite (tests) and Postgres (prod).
	var convCounts struct {
		TotalConvs        int64
		SuccessfulUpsells int64
	}
	if fail(database.ExcludeAiWaiterOperatorTestConversations(
		db.Model(&database.AiWaiterConversation{}).
			Select("COUNT(*) AS total_convs, COALESCE(SUM(CASE WHEN cart_items_added > 0 THEN 1 ELSE 0 END), 0) AS successful_upsells").
			Where("business_id = ?", businessID),
	).Scan(&convCounts).Error, "count conversations") {
		return
	}
	totalConvs := convCounts.TotalConvs
	successfulUpsells := convCounts.SuccessfulUpsells

	// Message count: a scoped COUNT over the join — no rows materialized, just
	// the aggregate. (The unbounded row read never existed here; the audit's
	// "no row materialization" concern was the timeseries Pluck below, now gone.)
	var totalMsgs int64
	if fail(db.Model(&database.AiWaiterMessage{}).
		Joins("JOIN ai_waiter_conversations ON ai_waiter_messages.conversation_id = ai_waiter_conversations.id").
		Where("ai_waiter_conversations.business_id = ? AND ai_waiter_conversations.table_code <> ?",
			businessID, database.AiWaiterOperatorTestTableCode).
		Count(&totalMsgs).Error, "count messages") {
		return
	}

	rate := 0.0
	if totalConvs > 0 {
		rate = float64(successfulUpsells) / float64(totalConvs) * 100
	}

	// Time-series cards (audit A2): computed with SQL GROUP BY aggregates
	// instead of plucking every created_at into Go (the old path materialized up
	// to 10k timestamps per request). Day-bucketed 7-day trend + hour-of-day
	// distribution over the 30-day window, both zero-filled here in the
	// business's stored timezone (L4-9 / S-13). Instant bounds stay absolute.
	loc := database.ResolveBusinessLocation(business)
	now := time.Now().In(loc)
	windowStart := now.AddDate(0, 0, -30)
	localMidnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	sevenDayStart := localMidnight.AddDate(0, 0, -6)

	series, err := database.GetAiConversationTimeSeries(businessID, windowStart, sevenDayStart, loc)
	if fail(err, "load conversation timeseries") {
		return
	}

	attributed, err := database.GetAiAttributedOrderValue(businessID, windowStart, now)
	if fail(err, "attributed order value") {
		return
	}

	trends := make([]aiTrendPoint, 0, 7)
	for i := 0; i < 7; i++ {
		key := sevenDayStart.AddDate(0, 0, i).Format("2006-01-02")
		trends = append(trends, aiTrendPoint{Date: key, Value: series.DayCounts[key]})
	}
	hours := make([]aiHourPoint, 0, 24)
	for h := 0; h < 24; h++ {
		hours = append(hours, aiHourPoint{Hour: h, Value: series.HourCounts[h]})
	}

	payload := AiInsightsResponse{
		TotalConversations:   totalConvs,
		TotalMessages:        totalMsgs,
		UpsellSuccessRate:    rate,
		ConversationTrends7d: trends,
		BusiestHours:         hours,
		Conversations30d:     series.Total30d,
		AttributedOrders30d:  attributed.OrderCount,
		AttributedRevenue30d: attributed.RevenueDollars,
	}
	storeAiInsightsCache(businessID, payload)
	c.JSON(http.StatusOK, payload)
}

// rejectClosedAiConversation writes the 409 that mutating actions (pause,
// claim, reply) return on a closed conversation. Close deliberately clears
// the claim and pause; re-arming a finished guest session afterwards produced
// the audit's contradictory "CERRADA + PAUSADO" state (L4-7).
func rejectClosedAiConversation(c *gin.Context, conv *database.AiWaiterConversation) bool {
	if conv.Status != "closed" {
		return false
	}
	RespondWithError(c, http.StatusConflict, "conversation_closed",
		"This conversation is closed and can no longer be handled")
	return true
}

// ToggleAiPause updates the pause state of a conversation (Takeover)
// POST /api/v1/business/:id/ai/conversations/:convId/pause
func ToggleAiPause(c *gin.Context) {
	conv, ok := loadScopedAiConversation(c)
	if !ok {
		return
	}
	if rejectClosedAiConversation(c, conv) {
		return
	}

	var req struct {
		IsPaused bool `json:"is_paused"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// status predicate repeated on the write: the row may close between the
	// scoped load above and this UPDATE, and a closed row must never re-pause.
	db := database.GetDB()
	res := db.Model(&database.AiWaiterConversation{}).
		Where("id = ? AND status <> ?", conv.ID, "closed").
		Update("is_paused", req.IsPaused)
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update pause state"})
		return
	}
	if res.RowsAffected == 0 {
		RespondWithError(c, http.StatusConflict, "conversation_closed",
			"This conversation is closed and can no longer be handled")
		return
	}

	// Unpausing hands the guest back to the AI — the open ai_takeover alert
	// (guest waiting in a paused chat) is answered.
	if !req.IsPaused {
		staffID, name, _ := aiActorIdentity(c)
		resolveAITakeoverAlertQuietly(c.Request.Context(), conv.BusinessID, conv.ID,
			operational_alerts.Actor{StaffID: staffID, Name: name})
	}

	invalidateOwnerHomeCaches(conv.BusinessID)

	c.Status(http.StatusOK)
}

// PostAiReply sends a manual reply from staff (Takeover).
// POST /api/v1/business/:id/ai/conversations/:convId/reply
func PostAiReply(c *gin.Context) {
	conv, ok := loadScopedAiConversation(c)
	if !ok {
		return
	}
	if rejectClosedAiConversation(c, conv) {
		return
	}

	var req struct {
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	// Mirror the guest path's byte cap: this reply is persisted and pushed over
	// SSE to the guest, so an unbounded operator-authored blob shouldn't be
	// stored or broadcast even though the caller is authenticated staff.
	if len(req.Content) > maxAIWaiterMessageBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Reply is too long"})
		return
	}

	staffID, name, role := aiActorIdentity(c)

	// Everyone — including owners/managers — must hold a fresh claim to reply.
	// Silent concurrent access is forbidden; managers steal via Claim with
	// {"steal":true} first (audited + notifies the previous holder).
	if !aiActorHoldsFreshClaim(conv, staffID, role) {
		c.JSON(http.StatusConflict, gin.H{
			"error":           "Pick up the conversation before replying",
			"code":            "claim_held",
			"claimed_by_name": conv.ClaimedByName,
			"claimed_by_role": conv.ClaimedByRole,
		})
		return
	}

	// 1. Save message to database with authorship.
	msgID, createdAt, err := database.SaveAiWaiterStaffReply(conv.ID, req.Content, staffID, name, role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save message"})
		return
	}
	// Refresh the claim hold so an active conversation does not idle-expire.
	database.GetDB().Model(&database.AiWaiterConversation{}).Where("id = ?", conv.ID).Update("claimed_at", time.Now())
	// Publish the exact row we just saved (staff replies persist as role "assistant").
	publishAiWaiterMessage(conv, msgID, "assistant", req.Content, createdAt)

	// A human answered the guest — the ai_takeover alert is handled regardless
	// of claim state (owners/managers may reply without claiming).
	resolveAITakeoverAlertQuietly(c.Request.Context(), conv.BusinessID, conv.ID,
		operational_alerts.Actor{StaffID: staffID, Name: name})

	// 2. If it's a WhatsApp conversation, send via WhatsApp.
	if strings.Contains(conv.SessionID, "whatsapp") || strings.Contains(conv.SessionID, "@s.whatsapp.net") {
		wm := GetWhatsAppManager()
		if wm != nil {
			parts := strings.Split(conv.SessionID, ":")
			jidStr := conv.SessionID
			if len(parts) > 1 {
				jidStr = parts[1]
			}
			businessID, content := conv.BusinessID, req.Content
			logger.SafeGo(func() { _ = wm.SendManualToJID(businessID, jidStr, content) })
		}
	}

	c.Status(http.StatusOK)
}

// CloseAiConversation marks a conversation as "closed"
// POST /api/v1/business/:id/ai/conversations/:convId/close
func CloseAiConversation(c *gin.Context) {
	conv, ok := loadScopedAiConversation(c)
	if !ok {
		return
	}

	// A closed row must not keep a live claim or stay paused — otherwise it
	// lingers as "held" in dashboards and can never idle-expire cleanly.
	db := database.GetDB()
	if err := db.Model(&database.AiWaiterConversation{}).Where("id = ?", conv.ID).Updates(map[string]interface{}{
		"status":              "closed",
		"claimed_by_staff_id": nil,
		"claimed_by_name":     "",
		"claimed_by_role":     "",
		"claimed_at":          nil,
		"is_paused":           false,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to close conversation"})
		return
	}

	// Closing the conversation ends the takeover — resolve the alert.
	staffID, name, _ := aiActorIdentity(c)
	resolveAITakeoverAlertQuietly(c.Request.Context(), conv.BusinessID, conv.ID,
		operational_alerts.Actor{StaffID: staffID, Name: name})

	c.Status(http.StatusOK)
}

// ClaimAiConversation atomically assigns the conversation to the acting staff
// and pauses the AI. Succeeds only if unclaimed, already held by the caller,
// the existing claim is idle past the TTL, or the caller passes steal=true and
// is steal-capable (manager/owner). Steal is audited and notifies the previous
// holder — silent concurrent access is no longer allowed.
// POST /api/v1/business/:id/ai/conversations/:convId/claim
func ClaimAiConversation(c *gin.Context) {
	conv, ok := loadScopedAiConversation(c)
	if !ok {
		return
	}
	if rejectClosedAiConversation(c, conv) {
		return
	}

	var req struct {
		Steal bool `json:"steal"`
	}
	_ = c.ShouldBindJSON(&req)

	staffID, name, role := aiActorIdentity(c)
	now := time.Now()
	idleCutoff := now.Add(-aiClaimIdleTTL)
	canSteal := aiActorCanStealClaim(c)

	// Snapshot previous holder for steal audit/notify.
	prevStaffID := conv.ClaimedByStaffID
	prevName := conv.ClaimedByName
	prevRole := conv.ClaimedByRole
	prevFresh := conv.ClaimedAt != nil && conv.ClaimedAt.After(idleCutoff)
	selfHold := aiActorHoldsFreshClaim(conv, staffID, role)

	if prevFresh && !selfHold {
		if !req.Steal {
			if canSteal {
				c.JSON(http.StatusConflict, gin.H{
					"error":           "Conversation already being handled — confirm steal to take over",
					"code":            "claim_steal_required",
					"claimed_by_name": prevName,
					"claimed_by_role": prevRole,
				})
				return
			}
			c.JSON(http.StatusConflict, gin.H{
				"error":           "Conversation already being handled",
				"code":            "claim_held",
				"claimed_by_name": prevName,
				"claimed_by_role": prevRole,
			})
			return
		}
		if !canSteal {
			c.JSON(http.StatusConflict, gin.H{
				"error":           "Conversation already being handled",
				"code":            "claim_held",
				"claimed_by_name": prevName,
				"claimed_by_role": prevRole,
			})
			return
		}
	}

	// The closed predicate rides on the write too: if the row closes between
	// the scoped load and this UPDATE, the claim must not land.
	db := database.GetDB()
	q := db.Model(&database.AiWaiterConversation{}).
		Where("id = ? AND status <> ?", conv.ID, "closed")
	// Even steal-capable actors only expand the WHERE when steal=true.
	if !(req.Steal && canSteal && prevFresh && !selfHold) {
		if staffID != nil {
			q = q.Where("claimed_at IS NULL OR claimed_at < ? OR claimed_by_staff_id = ?", idleCutoff, *staffID)
		} else {
			// Owner self re-claim: free/idle or already held by owner principal.
			q = q.Where("claimed_at IS NULL OR claimed_at < ? OR (claimed_by_staff_id IS NULL AND claimed_by_role = ?)", idleCutoff, "owner")
		}
	}

	res := q.Updates(map[string]interface{}{
		"claimed_by_staff_id": staffID,
		"claimed_by_name":     name,
		"claimed_by_role":     role,
		"claimed_at":          now,
		"is_paused":           true,
	})
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to claim conversation"})
		return
	}
	if res.RowsAffected == 0 {
		var current database.AiWaiterConversation
		database.GetDB().Select("claimed_by_name", "claimed_by_role").Where("id = ?", conv.ID).First(&current)
		c.JSON(http.StatusConflict, gin.H{
			"error":           "Conversation already being handled",
			"code":            "claim_held",
			"claimed_by_name": current.ClaimedByName,
			"claimed_by_role": current.ClaimedByRole,
		})
		return
	}

	// Audited steal + notify previous holder (best-effort notify).
	if req.Steal && canSteal && prevFresh && !selfHold && (prevStaffID != nil || prevName != "") {
		// Never staff_id=0 — rbac_audit_logs FK to staff; owner principal has nil staff_id.
		staffForAudit := uint(0)
		if staffID != nil && *staffID != 0 {
			staffForAudit = *staffID
		} else if prevStaffID != nil && *prevStaffID != 0 {
			staffForAudit = *prevStaffID
		}
		if staffForAudit != 0 {
			_ = db.Create(&database.RBACAuditLog{
				StaffID:    staffForAudit,
				BusinessID: conv.BusinessID,
				Action:     database.RBACActionClaimStolen,
				ChangedBy:  name,
				Reason: fmt.Sprintf("ai_conversation:%d prev=%s (%s) stealer=%s (%s)",
					conv.ID, prevName, prevRole, name, role),
				CreatedAt: now.UTC(),
			}).Error
		}
		if prevStaffID != nil && *prevStaffID != 0 {
			services.NotifyStaff(database.GetDBWrapper(), GetWebPushService(), conv.BusinessID,
				[]uint{*prevStaffID}, "claim.stolen", services.PushKeyClaimStolen,
				services.PushArgs{StealerName: name, Resource: "conversation"},
				fmt.Sprintf("/business/%d/ai-waiter", conv.BusinessID),
			)
		}
	}

	// A human now holds the conversation — the open ai_takeover alert (guest
	// waiting in a paused, unclaimed chat) is answered.
	resolveAITakeoverAlertQuietly(c.Request.Context(), conv.BusinessID, conv.ID,
		operational_alerts.Actor{StaffID: staffID, Name: name})

	invalidateOwnerHomeCaches(conv.BusinessID)
	c.JSON(http.StatusOK, gin.H{
		"claimed_by_staff_id": staffID,
		"claimed_by_name":     name,
		"claimed_by_role":     role,
		"claimed_at":          now,
	})
}

// aiActorHoldsFreshClaim reports whether the actor holds a non-idle claim.
func aiActorHoldsFreshClaim(conv *database.AiWaiterConversation, staffID *uint, role string) bool {
	if conv.ClaimedAt == nil || !conv.ClaimedAt.After(time.Now().Add(-aiClaimIdleTTL)) {
		return false
	}
	if staffID != nil && conv.ClaimedByStaffID != nil && *staffID == *conv.ClaimedByStaffID {
		return true
	}
	if staffID == nil && conv.ClaimedByStaffID == nil && conv.ClaimedByRole == "owner" && role == "owner" {
		return true
	}
	return false
}

// ReleaseAiConversation clears the claim and resumes the AI. Front-line staff
// may release only their own claim; owners/managers may release any.
// Force-release of another actor's live claim is audited (L4-8).
// POST /api/v1/business/:id/ai/conversations/:convId/release
func ReleaseAiConversation(c *gin.Context) {
	conv, ok := loadScopedAiConversation(c)
	if !ok {
		return
	}

	staffID, name, role := aiActorIdentity(c)
	selfHold := conv.ClaimedByStaffID != nil && staffID != nil && *conv.ClaimedByStaffID == *staffID
	// Owner principal holding their own claim (staff_id nil, role owner).
	if !selfHold && staffID == nil && conv.ClaimedByStaffID == nil && conv.ClaimedByRole == "owner" && role == "owner" {
		selfHold = true
	}
	forceRelease := false
	if !aiActorCanOverrideClaim(c) {
		if !selfHold {
			c.JSON(http.StatusConflict, gin.H{"error": "Conversation is handled by someone else", "code": "claim_held"})
			return
		}
	} else if !selfHold && (conv.ClaimedByStaffID != nil || conv.ClaimedByName != "") {
		// Manager/owner releasing someone else's claim — record the audit.
		forceRelease = true
	}

	prevName := conv.ClaimedByName
	prevRole := conv.ClaimedByRole
	prevStaffID := conv.ClaimedByStaffID

	db := database.GetDB()
	if err := db.Model(&database.AiWaiterConversation{}).Where("id = ?", conv.ID).Updates(map[string]interface{}{
		"claimed_by_staff_id": nil,
		"claimed_by_name":     "",
		"claimed_by_role":     "",
		"claimed_at":          nil,
		"is_paused":           false,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to release conversation"})
		return
	}

	if forceRelease {
		staffForAudit := uint(0)
		if staffID != nil && *staffID != 0 {
			staffForAudit = *staffID
		} else if prevStaffID != nil && *prevStaffID != 0 {
			staffForAudit = *prevStaffID
		}
		if staffForAudit != 0 {
			_ = db.Create(&database.RBACAuditLog{
				StaffID:    staffForAudit,
				BusinessID: conv.BusinessID,
				Action:     database.RBACActionClaimForceReleased,
				ChangedBy:  name,
				Reason: fmt.Sprintf("ai_conversation:%d prev=%s (%s) releaser=%s (%s)",
					conv.ID, prevName, prevRole, name, role),
				CreatedAt: time.Now().UTC(),
			}).Error
		}
		if prevStaffID != nil && *prevStaffID != 0 {
			services.NotifyStaff(database.GetDBWrapper(), GetWebPushService(), conv.BusinessID,
				[]uint{*prevStaffID}, "claim.force_released", services.PushKeyClaimStolen,
				services.PushArgs{StealerName: name, Resource: "conversation"},
				fmt.Sprintf("/business/%d/ai-waiter", conv.BusinessID),
			)
		}
	}

	// Releasing hands the guest back to the AI — resolve the takeover alert.
	resolveAITakeoverAlertQuietly(c.Request.Context(), conv.BusinessID, conv.ID,
		operational_alerts.Actor{StaffID: staffID, Name: name})

	invalidateOwnerHomeCaches(conv.BusinessID)
	c.Status(http.StatusOK)
}
