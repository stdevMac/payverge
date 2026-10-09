package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/services"
	"github.com/stdevmac/payverge/backend/internal/services/foodcost"
	"github.com/stdevmac/payverge/backend/internal/services/labor"
	"github.com/stdevmac/payverge/backend/internal/services/wastevariance"

	"github.com/gin-gonic/gin"
)

// insightCache holds per-business proactive-insight briefings for a short TTL.
// The underlying queries (inventory, stale bills, paused AI conversations)
// fire several DB round-trips per request; since owners poll the director
// console frequently, caching for 60 s is a material win without making the
// briefings feel stale after a write action (writers call InvalidateInsightCache).
type cachedInsight struct {
	insights  []proactiveInsight
	expiresAt time.Time
}

var (
	insightCache   = map[uint]cachedInsight{}
	insightCacheMu sync.RWMutex
)

const insightCacheTTL = 60 * time.Second
const directorThreadMessagesDefaultLimit = 200
const directorThreadMessagesMaxLimit = 500

// InvalidateInsightCache removes the cached insights for a business, forcing
// the next GetDirectorProactiveInsights call to rebuild. Safe to call often;
// no-op when no entry exists.
func InvalidateInsightCache(businessID uint) {
	insightCacheMu.Lock()
	delete(insightCache, businessID)
	insightCacheMu.Unlock()
}

func directorThreadMessagesLimit(raw string) int {
	requestedLimit, err := strconv.Atoi(raw)
	if err != nil || requestedLimit <= 0 {
		return directorThreadMessagesDefaultLimit
	}
	if requestedLimit > directorThreadMessagesMaxLimit {
		return directorThreadMessagesMaxLimit
	}
	return requestedLimit
}

func ensureOwnerToken(c *gin.Context) bool {
	tokenTypeAny, exists := c.Get("token_type")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return false
	}

	tokenType, ok := tokenTypeAny.(string)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return false
	}

	// Director console is owner-only (OAuth owner token or Web3 token).
	if tokenType == "staff" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Owner access required"})
		return false
	}
	if tokenType != "user" && tokenType != "web3" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Owner access required"})
		return false
	}

	return true
}

func ensureDirectorFeatureAvailable(c *gin.Context, businessID uint) bool {
	business, err := database.GetBusinessByID(businessID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return false
	}

	// Verify the authenticated caller actually owns or has access to this business.
	// Without this check an OAuth user could enumerate any business ID.
	if !CheckBusinessAccess(c, business) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return false
	}

	if RespondIfBusinessLocked(c, business) {
		return false
	}

	return true
}

// AskDirector handles owner prompts for Director's Console.
// POST /api/v1/inside/businesses/:id/ai/director/ask
func AskDirector(c *gin.Context) {
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
	// Asking the Director is the LLM-only part of the console (threads,
	// briefing, insights and the apply/undo rail stay data-driven), so with no
	// provider it answers 503 ai_not_configured instead of a canned reply.
	if !aiProviderConfigured() {
		respondAINotConfigured(c)
		return
	}

	service := GetDirectorConsoleService()
	if service == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Director console service unavailable"})
		return
	}

	var req struct {
		Message   string `json:"message" binding:"required"`
		ThreadID  *uint  `json:"thread_id"`
		Locale    string `json:"locale"`
		ActiveTab string `json:"active_tab"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	result, err := service.Ask(c.Request.Context(), services.DirectorAskRequest{
		BusinessID: businessID,
		Message:    req.Message,
		ThreadID:   req.ThreadID,
		Locale:     req.Locale,
		ActiveTab:  req.ActiveTab,
	})
	if err != nil {
		logger.Logger.Errorf("director console: Ask failed (business=%d, thread=%v): %v", businessID, req.ThreadID, err)
		// FIND-060: never surface provider/stack text to owners.
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Director could not complete this request")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"thread": gin.H{
			"id":         result.Thread.ID,
			"title":      result.Thread.Title,
			"updated_at": result.Thread.UpdatedAt,
			"created_at": result.Thread.CreatedAt,
		},
		"assistant_message": gin.H{
			"id":         result.AssistantMessage.ID,
			"created_at": result.AssistantMessage.CreatedAt,
			"response":   result.Response,
		},
		"response":         result.Response,
		"usage":            result.Usage,
		"proposed_actions": result.ProposedActions,
	})
}

// ListDirectorThreads returns owner thread history.
// GET /api/v1/inside/businesses/:id/ai/director/threads
func ListDirectorThreads(c *gin.Context) {
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

	service := GetDirectorConsoleService()
	if service == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Director console service unavailable"})
		return
	}

	// Optional pagination + archived filter. When NO paging/filter params are
	// present we keep the legacy response shape ({threads}) so existing clients
	// and BE-first deploys are unaffected. `?archived=1` returns the archived
	// (soft-deleted) threads for the sidebar's Archived section; `?limit`/`?offset`
	// page the list and add a `total` for a "load older" affordance.
	archived := parseBoolQuery(c, "archived")
	limitStr := strings.TrimSpace(c.Query("limit"))
	offsetStr := strings.TrimSpace(c.Query("offset"))
	usesPaging := archived || limitStr != "" || offsetStr != ""

	if usesPaging {
		limit := 0
		if v, err := strconv.Atoi(limitStr); err == nil {
			limit = v
		}
		offset := 0
		if v, err := strconv.Atoi(offsetStr); err == nil {
			offset = v
		}
		result, err := service.ListThreadsPaged(businessID, services.ListThreadsOptions{
			Archived: archived,
			Limit:    limit,
			Offset:   offset,
		})
		if err != nil {
			RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not list director threads")
			return
		}
		c.JSON(http.StatusOK, gin.H{"threads": result.Threads, "total": result.Total})
		return
	}

	threads, err := service.ListThreads(businessID)
	if err != nil {
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not list director threads")
		return
	}

	c.JSON(http.StatusOK, gin.H{"threads": threads})
}

// parseBoolQuery reports whether the query param reads as truthy ("1"/"true"/"yes").
func parseBoolQuery(c *gin.Context, key string) bool {
	switch strings.ToLower(strings.TrimSpace(c.Query(key))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// GetDirectorThreadMessages returns a thread timeline.
// GET /api/v1/inside/businesses/:id/ai/director/threads/:threadId/messages
func GetDirectorThreadMessages(c *gin.Context) {
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

	service := GetDirectorConsoleService()
	if service == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Director console service unavailable"})
		return
	}

	threadIDStr := c.Param("threadId")
	threadID64, err := strconv.ParseUint(threadIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid thread ID"})
		return
	}

	messages, err := service.ListThreadMessages(businessID, uint(threadID64), directorThreadMessagesLimit(c.Query("limit")))
	if err != nil {
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not load director messages")
		return
	}

	c.JSON(http.StatusOK, gin.H{"messages": messages})
}

// SubmitDirectorFeedback captures owner satisfaction feedback.
// POST /api/v1/inside/businesses/:id/ai/director/messages/:messageId/feedback
func SubmitDirectorFeedback(c *gin.Context) {
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

	service := GetDirectorConsoleService()
	if service == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Director console service unavailable"})
		return
	}

	messageIDStr := c.Param("messageId")
	messageID64, err := strconv.ParseUint(messageIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid message ID"})
		return
	}

	var req struct {
		Vote string `json:"vote" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	var vote database.DirectorFeedbackVote
	switch req.Vote {
	case string(database.DirectorFeedbackPositive):
		vote = database.DirectorFeedbackPositive
	case string(database.DirectorFeedbackNegative):
		vote = database.DirectorFeedbackNegative
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Vote must be 'up' or 'down'"})
		return
	}

	updated, err := service.SubmitFeedback(businessID, uint(messageID64), vote)
	if err != nil {
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not save feedback")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": updated})
}

// GetDirectorProactiveInsights returns deterministic proactive briefings for the owner home.
// GET /api/v1/inside/businesses/:id/director-console/proactive-insights
func GetDirectorProactiveInsights(c *gin.Context) {
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

	insightCacheMu.RLock()
	if cached, ok := insightCache[businessID]; ok && time.Now().Before(cached.expiresAt) {
		insights := cached.insights
		insightCacheMu.RUnlock()
		c.JSON(http.StatusOK, gin.H{"insights": insights})
		return
	}
	insightCacheMu.RUnlock()

	insights := buildProactiveInsights(businessID)

	insightCacheMu.Lock()
	insightCache[businessID] = cachedInsight{insights: insights, expiresAt: time.Now().Add(insightCacheTTL)}
	insightCacheMu.Unlock()

	c.JSON(http.StatusOK, gin.H{"insights": insights})
}

type proactiveInsight struct {
	ID     string                 `json:"id"`
	Type   string                 `json:"type"`
	Params map[string]interface{} `json:"params"`
	CTA    proactiveInsightCTA    `json:"cta"`
}

type proactiveInsightCTA struct {
	Tab string `json:"tab"`
}

func buildProactiveInsights(businessID uint) []proactiveInsight {
	return buildProactiveInsightsWithReports(businessID, nil, nil)
}

// buildProactiveInsightsWithReports is buildProactiveInsights with optional
// precomputed foodcost and labor reports. When a report is non-nil the matching
// section (food-cost-high / labor-high) reuses it instead of recomputing
// foodcost.Analyze / labor.Analyze; the briefing front door relies on this so
// each heavy calculator runs exactly once per request and feeds all of its
// consumers (pulse number + threshold insight).
func buildProactiveInsightsWithReports(businessID uint, precomputedFood *foodcost.Report, precomputedLabor *labor.Report) []proactiveInsight {
	var out []proactiveInsight

	// 1. Inventory out-of-stock (qty <= 0). Carry quantity so copy can
	// distinguish true zero from oversold/negative (audit L1-22).
	if outOfStock := getInventoryOutOfStock(businessID); len(outOfStock) > 0 {
		names := make([]string, 0, len(outOfStock))
		// R2-7: keep the two groups apart. has_oversold is a boolean OR over the
		// set, so a mixed set (one negative + one exactly zero) rendered as
		// "2 inventory items are oversold: <both names>" — false for the
		// at-zero half. Honest copy needs each group's own names.
		oversoldNames := make([]string, 0, len(outOfStock))
		zeroNames := make([]string, 0, len(outOfStock))
		for _, item := range outOfStock {
			names = append(names, item.Name)
			if item.Quantity < 0 {
				oversoldNames = append(oversoldNames, item.Name)
			} else {
				zeroNames = append(zeroNames, item.Name)
			}
		}
		oversoldCount := len(oversoldNames)
		out = append(out, proactiveInsight{
			ID:   "inventory-out-of-stock",
			Type: "inventory_out_of_stock",
			Params: map[string]interface{}{
				"count":          len(outOfStock),
				"item_names":     capInsightNames(names),
				"has_oversold":   oversoldCount > 0,
				"oversold_count": oversoldCount,
				"oversold_names": capInsightNames(oversoldNames),
				"zero_names":     capInsightNames(zeroNames),
			},
			CTA: proactiveInsightCTA{Tab: "inventory"},
		})
	}

	// 2. Inventory low-stock (only if slots remain)
	if len(out) < 3 {
		if lowStock := getInventoryLowStock(businessID); len(lowStock) >= 3 {
			names := lowStock
			if len(names) > 3 {
				names = names[:3]
			}
			out = append(out, proactiveInsight{
				ID:   "inventory-low-stock",
				Type: "inventory_low_stock",
				Params: map[string]interface{}{
					"count":      len(lowStock),
					"item_names": names,
				},
				CTA: proactiveInsightCTA{Tab: "inventory"},
			})
		}
	}

	// 3. Open bills stale — surface count + *actual* oldest age (not a hardcoded "2 hours").
	if len(out) < 3 {
		if stale := getStaleOpenBillsInfo(businessID); stale.Count >= 1 {
			out = append(out, proactiveInsight{
				ID:   "stale-open-bills",
				Type: "stale_open_bills",
				Params: map[string]interface{}{
					// count matches the advertised duration bucket (#817);
					// stale_count is everything past the 2h threshold.
					"count":             stale.Count,
					"stale_count":       stale.StaleTotal,
					"threshold_minutes": int(staleBillThreshold / time.Minute),
					"oldest_minutes":    stale.OldestMinutes,
					"duration":          stale.Duration,
				},
				CTA: proactiveInsightCTA{Tab: "bills"},
			})
		}
	}

	// 4. AI Waiter conversations needing reply
	if len(out) < 3 {
		if pauseCount := getPausedAIConversationsCount(businessID); pauseCount >= 1 {
			out = append(out, proactiveInsight{
				ID:   "ai-conversations-pending",
				Type: "ai_conversations_pending",
				Params: map[string]interface{}{
					"count": pauseCount,
				},
				CTA: proactiveInsightCTA{Tab: "ai-waiter"},
			})
		}
	}

	// 5. Food cost over threshold (margin alert)
	if len(out) < 3 {
		// 0.40 is intentionally above the 35% UI/tool threshold — only egregious cases trigger the digest nag.
		var count int
		var names []string
		if precomputedFood != nil {
			count, names = foodCostOverThresholdFromReport(*precomputedFood, 0.40)
		} else {
			count, names = getFoodCostOverThreshold(businessID, 0.40)
		}
		if count >= 1 {
			out = append(out, proactiveInsight{
				ID:   "food-cost-high",
				Type: "food_cost_high",
				Params: map[string]interface{}{
					"count":      count,
					"item_names": names,
				},
				CTA: proactiveInsightCTA{Tab: "accounting"},
			})
		}
	}

	// 6. Tracked inventory loss high (waste alert)
	if len(out) < 3 {
		if amount, worst := getWasteOverThreshold(businessID, 0.08, 50.0); amount > 0 {
			out = append(out, proactiveInsight{
				ID:   "waste-high",
				Type: "waste_high",
				Params: map[string]interface{}{
					"amount":     amount,
					"ingredient": worst,
				},
				CTA: proactiveInsightCTA{Tab: "accounting"},
			})
		}
	}

	// 7. Labor cost high (prime-cost alert)
	if len(out) < 3 {
		var pct, amount float64
		if precomputedLabor != nil {
			pct, amount = laborOverThresholdFromReport(*precomputedLabor, 0.35)
		} else {
			pct, amount = getLaborOverThreshold(businessID, 0.35)
		}
		if pct > 0 {
			out = append(out, proactiveInsight{
				ID:   "labor-high",
				Type: "labor_high",
				Params: map[string]interface{}{
					"pct":    pct,
					"amount": amount,
				},
				CTA: proactiveInsightCTA{Tab: "accounting"},
			})
		}
	}

	// 8. S3-Loop: marketing posts recorded this period (info — only fills a
	// free insight slot so urgent ops cards are never displaced). Count comes
	// from mark_posted activity; optional freeform channel tags surface in
	// params.channels. CTA opens the Marketing Library (tab=marketing).
	if len(out) < 3 {
		if m := buildBriefingMarketing(businessID); m != nil && m.PostsThisPeriod >= 1 {
			out = append(out, proactiveInsight{
				ID:   "marketing-posts-period",
				Type: "marketing_posts",
				Params: map[string]interface{}{
					"count":         m.PostsThisPeriod,
					"period_days":   m.PeriodDays,
					"recent_titles": m.RecentTitles,
					"channels":      m.Channels,
				},
				CTA: proactiveInsightCTA{Tab: "marketing"},
			})
		}
	}

	if len(out) > 3 {
		out = out[:3]
	}
	return out
}

// BuildDigestInsights adapts buildProactiveInsights for use by the
// DirectorDigestScheduler (which lives in the services package).
func BuildDigestInsights(businessID uint) []services.DigestInsight {
	raw := buildProactiveInsights(businessID)
	out := make([]services.DigestInsight, len(raw))
	for i, r := range raw {
		out[i] = services.DigestInsight{Type: r.Type, Params: r.Params}
	}
	return out
}

// capInsightNames trims an insight name list to the 3 items the copy renders.
// The full magnitude always travels as a separate count param.
func capInsightNames(names []string) []string {
	if len(names) > 3 {
		return names[:3]
	}
	return names
}

// inventoryOutOfStockItem is a name+qty row for out-of-stock insights (L1-22).
type inventoryOutOfStockItem struct {
	Name     string
	Quantity float64
}

// getInventoryOutOfStock returns active inventory items with quantity <= 0,
// including current_quantity so callers can branch zero vs oversold copy.
// The InventoryItem model uses CurrentQuantity (IsActive gates active rows).
// On DB error returns nil and logs — surfacing an error insight is worse than
// silence, but a silent success ("all good!") during an outage would mislead.
func getInventoryOutOfStock(businessID uint) []inventoryOutOfStockItem {
	var items []inventoryOutOfStockItem
	if err := database.GetDB().Raw(
		`SELECT name, current_quantity AS quantity FROM inventory_items
		 WHERE business_id = ? AND is_active = true AND current_quantity <= 0
		 ORDER BY current_quantity ASC, name
		 LIMIT 10`,
		businessID,
	).Scan(&items).Error; err != nil {
		logger.Logger.Errorf("proactive-insights: inventory out-of-stock query failed for business %d: %v", businessID, err)
		return nil
	}
	return items
}

// getInventoryLowStock returns names of active items above 0 but at or below reorder_threshold.
// reorder_threshold is the par-level analog in this model.
func getInventoryLowStock(businessID uint) []string {
	var items []struct {
		Name string
	}
	if err := database.GetDB().Raw(
		`SELECT name FROM inventory_items
		 WHERE business_id = ? AND is_active = true
		   AND current_quantity > 0
		   AND reorder_threshold > 0 AND current_quantity <= reorder_threshold
		 ORDER BY current_quantity / NULLIF(reorder_threshold, 0), name
		 LIMIT 10`,
		businessID,
	).Scan(&items).Error; err != nil {
		logger.Logger.Errorf("proactive-insights: inventory low-stock query failed for business %d: %v", businessID, err)
		return nil
	}
	names := make([]string, 0, len(items))
	for _, i := range items {
		names = append(names, i.Name)
	}
	return names
}

// staleBillThreshold is the age at which an open bill is considered stale and surfaces in the
// proactive insights feed. Chosen to be longer than a typical full-service meal (2 h) so that
// routine dining doesn't trigger false-positive alerts that train owners to ignore the briefings
// feed. This is the *surfacing* threshold only — the lifecycle sweeper's *abandon* threshold
// (default 24h, see services.DefaultBillAbandonThreshold) is a separate, larger decision.
const staleBillThreshold = 120 * time.Minute

// staleOpenBillsInfo is the honest payload for the stale-bills insight. The
// briefing copy renders "{count} bills open longer than {duration}", so Count
// must be the number of bills at least as old as the rendered Duration bucket
// — NOT everything past the 2h surfacing threshold. Pairing the total stale
// count with the oldest bill's age called tonight's 7h service bill a "6-day"
// walkout (#817). StaleTotal keeps the full past-threshold count for
// consumers that speak about the threshold, not the oldest age.
type staleOpenBillsInfo struct {
	Count         int64
	StaleTotal    int64
	OldestAge     time.Duration
	OldestMinutes int
	Duration      string
}

// FormatInsightDuration renders a duration for operator copy ("3 days", "2 hours").
// Exported for unit tests and shared briefing surfaces.
func FormatInsightDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	minutes := int(d / time.Minute)
	if minutes < 60 {
		if minutes == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", minutes)
	}
	hours := minutes / 60
	if hours < 24 {
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	}
	days := hours / 24
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}

// insightDurationFloor returns the lower bound of the bucket that
// FormatInsightDuration renders for d ("6 days" → 6*24h). A bill "open longer
// than {duration}" is exactly one whose age is >= this floor.
func insightDurationFloor(d time.Duration) time.Duration {
	if d < 0 {
		return 0
	}
	minutes := int(d / time.Minute)
	if minutes < 60 {
		return time.Duration(minutes) * time.Minute
	}
	hours := minutes / 60
	if hours < 24 {
		return time.Duration(hours) * time.Hour
	}
	days := hours / 24
	return time.Duration(days) * 24 * time.Hour
}

// getStaleOpenBillsInfo returns how many open bills are past staleBillThreshold
// and the actual age of the oldest such bill (for honest copy).
func getStaleOpenBillsInfo(businessID uint) staleOpenBillsInfo {
	cutoff := time.Now().Add(-staleBillThreshold)
	db := database.GetDB()
	statuses := []string{string(database.BillStatusOpen), string(database.BillStatusPartial)}

	var count int64
	if err := db.Model(&database.Bill{}).
		Where("business_id = ? AND status IN ? AND created_at < ?", businessID, statuses, cutoff).
		Count(&count).Error; err != nil {
		logger.Logger.Errorf("proactive-insights: stale-open-bills count failed for business %d: %v", businessID, err)
		return staleOpenBillsInfo{}
	}
	if count == 0 {
		return staleOpenBillsInfo{}
	}

	// Load the oldest matching row (created_at ASC) rather than MIN() + Scan,
	// which is flaky across SQLite (string timestamps) vs Postgres (timestamptz).
	var oldestBill database.Bill
	if err := db.Select("id", "created_at").
		Where("business_id = ? AND status IN ? AND created_at < ?", businessID, statuses, cutoff).
		Order("created_at ASC").
		Limit(1).
		First(&oldestBill).Error; err != nil || oldestBill.CreatedAt.IsZero() {
		if err != nil {
			logger.Logger.Errorf("proactive-insights: stale-open-bills oldest failed for business %d: %v", businessID, err)
		}
		// With the threshold as the advertised duration, every stale bill
		// matches it, so the full count stays honest.
		return staleOpenBillsInfo{
			Count:         count,
			StaleTotal:    count,
			OldestAge:     staleBillThreshold,
			OldestMinutes: int(staleBillThreshold / time.Minute),
			Duration:      FormatInsightDuration(staleBillThreshold),
		}
	}

	age := time.Since(oldestBill.CreatedAt)
	if age < 0 {
		age = 0
	}

	// #817: the copy claims every counted bill is "open longer than
	// {duration}". Count only bills at least as old as the rendered bucket —
	// tonight's 7h service bill must not be sold as a 6-day walkout.
	bucketCount := int64(1)
	if count > 1 {
		bucketCutoff := time.Now().Add(-insightDurationFloor(age))
		if err := db.Model(&database.Bill{}).
			Where("business_id = ? AND status IN ? AND created_at <= ?", businessID, statuses, bucketCutoff).
			Count(&bucketCount).Error; err != nil {
			logger.Logger.Errorf("proactive-insights: stale-open-bills bucket count failed for business %d: %v", businessID, err)
			bucketCount = 1 // the oldest bill itself is always in the bucket
		}
		if bucketCount < 1 {
			bucketCount = 1
		}
	}
	return staleOpenBillsInfo{
		Count:         bucketCount,
		StaleTotal:    count,
		OldestAge:     age,
		OldestMinutes: int(age / time.Minute),
		Duration:      FormatInsightDuration(age),
	}
}

// getPausedAIConversationsCount returns how many AI Waiter conversations are paused and not closed.
func getPausedAIConversationsCount(businessID uint) int64 {
	var count int64
	if err := database.ExcludeAiWaiterOperatorTestConversations(
		database.GetDB().Model(&database.AiWaiterConversation{}).
			Where("business_id = ? AND is_paused = true AND status != 'closed'", businessID),
	).Count(&count).Error; err != nil {
		logger.Logger.Errorf("proactive-insights: paused-AI-conversations query failed for business %d: %v", businessID, err)
		return 0
	}
	return count
}

// getFoodCostOverThreshold returns the count and up-to-3 names of recipe-mapped
// menu items whose weekly food cost exceeds thresholdPct (0..1), for the
// proactive digest. Best-effort: any error yields zero results (no digest noise).
func getFoodCostOverThreshold(businessID uint, thresholdPct float64) (int, []string) {
	db := database.GetDBWrapper()
	if db == nil {
		return 0, nil
	}
	loc := time.UTC
	if biz, err := database.GetBusinessByID(businessID); err == nil && biz != nil {
		if l, err := time.LoadLocation(biz.Timezone); err == nil && l != nil {
			loc = l
		}
	}
	calc := foodcost.NewCalculator(db, analytics.NewAnalyticsService(db))
	report, err := calc.Analyze(businessID, "week", loc)
	if err != nil {
		logger.Logger.Errorf("proactive-insights: food-cost-over-threshold failed for business %d: %v", businessID, err)
		return 0, nil
	}
	return foodCostOverThresholdFromReport(report, thresholdPct)
}

// foodCostOverThresholdFromReport returns the count and up-to-3 names of
// recipe-mapped, sold items whose food cost exceeds thresholdPct (0..1) from an
// already-computed report. Extracted from getFoodCostOverThreshold so the
// briefing can reuse a single foodcost.Analyze result across all consumers
// instead of recomputing it per consumer.
func foodCostOverThresholdFromReport(report foodcost.Report, thresholdPct float64) (int, []string) {
	var names []string
	for _, it := range report.Items {
		if it.HasCompleteCost && it.QtySold > 0 && it.FoodCostPct > thresholdPct {
			names = append(names, it.MenuItemName)
		}
	}
	count := len(names)
	if len(names) > 3 {
		names = names[:3]
	}
	return count, names
}

// getWasteOverThreshold returns (trackedLoss$, worstLossIngredientName) when the
// week's tracked inventory loss exceeds BOTH the absolute floor AND, when a
// theoretical baseline exists, pctOfTheoretical of theoretical food cost.
// Returns (0,"") otherwise. Always "week".
func getWasteOverThreshold(businessID uint, pctOfTheoretical, floor float64) (float64, string) {
	db := database.GetDBWrapper()
	if db == nil {
		return 0, ""
	}
	loc := time.UTC
	if biz, err := database.GetBusinessByID(businessID); err == nil && biz != nil {
		if l, err := time.LoadLocation(biz.Timezone); err == nil && l != nil {
			loc = l
		}
	}
	an := analytics.NewAnalyticsService(db)
	calc := wastevariance.NewCalculator(db, an, db, db, an)
	report, err := calc.Analyze(businessID, "week", loc)
	if err != nil {
		logger.Logger.Errorf("proactive-insights: waste-over-threshold failed for business %d: %v", businessID, err)
		return 0, ""
	}
	if report.TrackedLossCost < floor {
		return 0, ""
	}
	if report.TheoreticalUsageCost > 0 && report.TrackedLossCost < pctOfTheoretical*report.TheoreticalUsageCost {
		return 0, ""
	}
	worst := ""
	for _, ig := range report.Ingredients { // sorted tracked-loss desc
		if ig.TrackedLossCost > 0 {
			worst = ig.Name
			break
		}
	}
	return report.TrackedLossCost, worst
}

// getLaborOverThreshold returns (laborCostPct, laborCost$) when the week's labor
// cost exceeds thresholdPct of net sales (e.g. 0.35 == 35%). Returns (0,0)
// otherwise — including when there is no payroll data or no sales. Always "week".
func getLaborOverThreshold(businessID uint, thresholdPct float64) (float64, float64) {
	db := database.GetDBWrapper()
	if db == nil {
		return 0, 0
	}
	loc := time.UTC
	if biz, err := database.GetBusinessByID(businessID); err == nil && biz != nil {
		if l, err := time.LoadLocation(biz.Timezone); err == nil && l != nil {
			loc = l
		}
	}
	an := analytics.NewAnalyticsService(db)
	calc := labor.NewCalculator(db, an, an)
	report, err := calc.Analyze(businessID, "week", loc)
	if err != nil {
		logger.Logger.Errorf("proactive-insights: labor-over-threshold failed for business %d: %v", businessID, err)
		return 0, 0
	}
	return laborOverThresholdFromReport(report, thresholdPct)
}

// laborOverThresholdFromReport is the pure threshold check over an
// already-computed labor report. It lets the briefing reuse the single
// labor.Analyze it ran for the pulse instead of recomputing it for the
// labor-high insight (the labor-once perf contract, mirroring foodcost).
func laborOverThresholdFromReport(report labor.Report, thresholdPct float64) (float64, float64) {
	if !report.HasData || report.NetSales <= 0 || report.LaborCostPct <= thresholdPct {
		return 0, 0
	}
	return report.LaborCostPct, report.LaborCost
}

// parseBusinessAndThread reads :id and :threadId from the route params, runs
// owner-token + feature-availability guards, and returns both IDs. On any
// failure the response is already written and ok=false; callers should return
// immediately.
func parseBusinessAndThread(c *gin.Context) (businessID uint, threadID uint, ok bool) {
	if !ensureOwnerToken(c) {
		return 0, 0, false
	}

	businessIDStr := c.Param("id")
	businessID64, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return 0, 0, false
	}
	businessID = uint(businessID64)
	if !ensureDirectorFeatureAvailable(c, businessID) {
		return 0, 0, false
	}

	threadIDStr := c.Param("threadId")
	threadID64, err := strconv.ParseUint(threadIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid thread ID"})
		return 0, 0, false
	}
	threadID = uint(threadID64)

	// Verify the thread belongs to this business so an owner can't address a
	// sibling thread by guessing the ID.
	if _, err := database.GetDirectorConsoleThreadByID(businessID, threadID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Director thread not found"})
		return 0, 0, false
	}

	return businessID, threadID, true
}

// PatchDirectorThread renames a thread.
// PATCH /api/v1/inside/businesses/:id/ai/director/threads/:threadId
func PatchDirectorThread(c *gin.Context) {
	businessID, threadID, ok := parseBusinessAndThread(c)
	if !ok {
		return
	}

	var req struct {
		Title string `json:"title" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	cleanTitle := strings.TrimSpace(req.Title)
	if cleanTitle == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Title must not be empty"})
		return
	}
	// Cap at 200 characters on a rune boundary — a byte slice can cut a
	// multibyte rune in half, and PostgreSQL rejects the invalid UTF-8.
	if utf8.RuneCountInString(cleanTitle) > 200 {
		cleanTitle = string([]rune(cleanTitle)[:200])
	}

	now := time.Now()
	if err := database.GetDB().Model(&database.DirectorConsoleThread{}).
		Where("id = ? AND business_id = ?", threadID, businessID).
		Updates(map[string]interface{}{
			"title":      cleanTitle,
			"updated_at": now,
		}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to rename director thread"})
		return
	}

	thread, err := database.GetDirectorConsoleThreadByID(businessID, threadID)
	if err != nil {
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not load director thread")
		return
	}

	c.JSON(http.StatusOK, gin.H{"thread": thread})
}

// ArchiveDirectorThread soft-archives a thread (restorable until permanent delete).
// PATCH /api/v1/inside/businesses/:id/ai/director/threads/:threadId/archive
func ArchiveDirectorThread(c *gin.Context) {
	businessID, threadID, ok := parseBusinessAndThread(c)
	if !ok {
		return
	}

	now := time.Now()
	res := database.GetDB().Model(&database.DirectorConsoleThread{}).
		Where("id = ? AND business_id = ? AND archived_at IS NULL", threadID, businessID).
		Updates(map[string]interface{}{
			"archived_at": now,
			"updated_at":  now,
		})
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to archive director thread", "code": "archive_failed"})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Director thread not found", "code": "not_found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"archived": true, "archived_at": now})
}

// RestoreDirectorThread clears archive for a tenant-owned thread.
// POST /api/v1/inside/businesses/:id/ai/director/threads/:threadId/restore
func RestoreDirectorThread(c *gin.Context) {
	businessID, threadID, ok := parseBusinessAndThread(c)
	if !ok {
		return
	}
	res := database.GetDB().Exec(
		`UPDATE director_console_threads SET archived_at = NULL, updated_at = ? WHERE id = ? AND business_id = ? AND archived_at IS NOT NULL`,
		time.Now().UTC(), threadID, businessID,
	)
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to restore director thread", "code": "restore_failed"})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Director thread not found", "code": "not_found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"restored": true})
}

// DeleteDirectorThread permanently erases a thread and child rows (irreversible).
// DELETE /api/v1/inside/businesses/:id/ai/director/threads/:threadId
// Soft-archive moved to PATCH .../archive (Wave 5 lifecycle split).
func DeleteDirectorThread(c *gin.Context) {
	businessID, threadID, ok := parseBusinessAndThread(c)
	if !ok {
		return
	}

	if err := database.PermanentlyDeleteDirectorConsoleThread(businessID, threadID); err != nil {
		if errors.Is(err, database.ErrAIHistoryNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Director thread not found", "code": "not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to permanently delete director thread", "code": "delete_failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"deleted": true, "permanent": true})
}

// setPinned toggles the pinned flag on a thread; shared by Pin/Unpin handlers
// so the small bit of duplication stays in one place.
func setPinned(c *gin.Context, pinned bool) {
	businessID, threadID, ok := parseBusinessAndThread(c)
	if !ok {
		return
	}

	now := time.Now()
	if err := database.GetDB().Model(&database.DirectorConsoleThread{}).
		Where("id = ? AND business_id = ?", threadID, businessID).
		Updates(map[string]interface{}{
			"pinned":     pinned,
			"updated_at": now,
		}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update pin state"})
		return
	}

	thread, err := database.GetDirectorConsoleThreadByID(businessID, threadID)
	if err != nil {
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not load director thread")
		return
	}

	c.JSON(http.StatusOK, gin.H{"thread": thread})
}

// PinDirectorThread marks a thread as pinned so it sorts to the top of the
// owner's thread list.
// POST /api/v1/inside/businesses/:id/ai/director/threads/:threadId/pin
func PinDirectorThread(c *gin.Context) {
	setPinned(c, true)
}

// UnpinDirectorThread clears the pinned flag.
// POST /api/v1/inside/businesses/:id/ai/director/threads/:threadId/unpin
func UnpinDirectorThread(c *gin.Context) {
	setPinned(c, false)
}

// ExportDirectorThread returns a thread transcript as Markdown for download.
// GET /api/v1/inside/businesses/:id/ai/director/threads/:threadId/export?format=md
func ExportDirectorThread(c *gin.Context) {
	businessID, threadID, ok := parseBusinessAndThread(c)
	if !ok {
		return
	}

	format := strings.ToLower(strings.TrimSpace(c.Query("format")))
	if format == "" {
		format = "md"
	}
	if format != "md" && format != "markdown" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported export format"})
		return
	}

	thread, err := database.GetDirectorConsoleThreadByID(businessID, threadID)
	if err != nil {
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not load director thread")
		return
	}

	messages, err := database.ListDirectorConsoleMessages(businessID, threadID, 500)
	if err != nil {
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Could not export director thread")
		return
	}

	md := renderThreadAsMarkdown(thread, messages)

	filename := fmt.Sprintf("director-thread-%d.md", thread.ID)
	c.Header("Content-Type", "text/markdown; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.String(http.StatusOK, md)
}

// renderThreadAsMarkdown formats a thread + messages as a portable transcript.
// Keeps the output deterministic (no timestamps in fractional seconds) so the
// file diffs cleanly when an owner re-exports.
func renderThreadAsMarkdown(thread *database.DirectorConsoleThread, messages []database.DirectorConsoleMessage) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(thread.Title)
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("- Thread ID: %d\n", thread.ID))
	b.WriteString(fmt.Sprintf("- Locale: %s\n", thread.Locale))
	b.WriteString(fmt.Sprintf("- Created: %s\n", thread.CreatedAt.UTC().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- Updated: %s\n", thread.UpdatedAt.UTC().Format(time.RFC3339)))
	if thread.Pinned {
		b.WriteString("- Pinned: true\n")
	}
	if thread.ArchivedAt != nil {
		b.WriteString(fmt.Sprintf("- Archived: %s\n", thread.ArchivedAt.UTC().Format(time.RFC3339)))
	}
	b.WriteString("\n---\n\n")

	for _, msg := range messages {
		role := strings.Title(string(msg.Role)) //nolint:staticcheck // Title is fine for ASCII role names.
		b.WriteString(fmt.Sprintf("## %s — %s\n\n", role, msg.CreatedAt.UTC().Format(time.RFC3339)))
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			content = "_(no content)_"
		}
		b.WriteString(content)
		b.WriteString("\n\n")
	}

	return b.String()
}
