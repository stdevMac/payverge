package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/middleware"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type okProvider struct{}

func (okProvider) Generate(_ context.Context, _ llm.GenerateRequest) (*llm.Response, error) {
	return &llm.Response{Text: "ok"}, nil
}

func TestHandleAIWaiter_PerSessionCap(t *testing.T) {
	t.Setenv("AI_WAITER_DAILY_MESSAGE_BUDGET", "100000")
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "cap-session", true)
	table := createAIWaiterTable(t, business.ID, "CAP-1")
	require.NoError(t, db.AutoMigrate(&database.Menu{}))
	require.NoError(t, db.Create(&database.Menu{BusinessID: business.ID, Categories: "[]", IsActive: true, Version: 1}).Error)
	conv := createAIWaiterConversation(t, business.ID, "cap-token")
	conv.TableCode = table.TableCode
	require.NoError(t, db.Save(conv).Error)
	for i := 0; i < 60; i++ {
		require.NoError(t, db.Create(&database.AiWaiterMessage{ConversationID: conv.ID, Role: "user", Content: "x", CreatedAt: time.Now()}).Error)
	}
	svc, _ := services.NewAIService(okProvider{}, llm.ModelConfig{Chat: "m", Image: "i", Director: "d", Menu: "mn"})
	SetAIService(svc)
	t.Cleanup(func() { SetAIService(nil) })

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": "cap-token", "mode": "ordering", "table_code": table.TableCode, "language": "en",
		"history": []map[string]any{{"role": "user", "content": "one more"}},
	})
	require.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "over_budget", resp["code"])
}

func TestHandleAIWaiter_PerBusinessDailyBudget(t *testing.T) {
	t.Setenv("AI_WAITER_DAILY_MESSAGE_BUDGET", "2")
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "cap-biz", true)
	table := createAIWaiterTable(t, business.ID, "CAP-2")
	require.NoError(t, db.AutoMigrate(&database.Menu{}))
	require.NoError(t, db.Create(&database.Menu{BusinessID: business.ID, Categories: "[]", IsActive: true, Version: 1}).Error)
	conv := createAIWaiterConversation(t, business.ID, "biz-token")
	conv.TableCode = table.TableCode
	require.NoError(t, db.Save(conv).Error)
	for i := 0; i < 2; i++ {
		// .UTC() matters: the budget window starts at UTC midnight, and the
		// SQLite test driver compares datetimes as strings — a local-offset
		// timestamp sorts before "today" for a few hours after UTC midnight.
		require.NoError(t, db.Create(&database.AiWaiterMessage{ConversationID: conv.ID, Role: "assistant", Content: "x", CreatedAt: time.Now().UTC()}).Error)
	}
	svc, _ := services.NewAIService(okProvider{}, llm.ModelConfig{Chat: "m", Image: "i", Director: "d", Menu: "mn"})
	SetAIService(svc)
	t.Cleanup(func() { SetAIService(nil) })

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": "biz-token", "mode": "ordering", "table_code": table.TableCode, "language": "en",
		"history": []map[string]any{{"role": "user", "content": "hi"}},
	})
	require.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
}

func TestAllowSessionCreate_CapsPerIP(t *testing.T) {
	// Reset the shared limiter so this test is deterministic regardless of order.
	sessionCreateLimiter.mu.Lock()
	sessionCreateLimiter.window = time.Time{}
	sessionCreateLimiter.counts = map[string]int{}
	sessionCreateLimiter.mu.Unlock()

	ip := "203.0.113.7"
	allowed := 0
	for i := 0; i < maxSessionsPerIPPerHour+5; i++ {
		if allowSessionCreate(ip) {
			allowed++
		}
	}
	assert.Equal(t, maxSessionsPerIPPerHour, allowed)
	assert.True(t, allowSessionCreate("198.51.100.9")) // different IP unaffected
}

// The guest session cap buckets IPv6 clients per /64, so rotating source
// addresses inside one delegation does not reset it.
func TestCreateAIWaiterSession_CapsPerIPv6Slash64(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "session-v6-cap", true)

	sessionCreateLimiter.mu.Lock()
	sessionCreateLimiter.window = time.Now().UTC().Truncate(time.Hour)
	sessionCreateLimiter.counts = map[string]int{middleware.RateLimitKeyForIP("2001:db8:aa:bb::1"): maxSessionsPerIPPerHour}
	sessionCreateLimiter.mu.Unlock()
	t.Cleanup(func() {
		sessionCreateLimiter.mu.Lock()
		sessionCreateLimiter.counts = map[string]int{}
		sessionCreateLimiter.mu.Unlock()
	})

	router := gin.New()
	router.POST("/ai-waiter/:businessId/session", CreateAIWaiterSession)
	create := func(remoteAddr string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/ai-waiter/%d/session", biz.ID), strings.NewReader(`{"mode":"concierge","language":"en"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = remoteAddr
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	w := create("[2001:db8:aa:bb:dead:beef:0:2]:443")
	assert.Equal(t, http.StatusTooManyRequests, w.Code, "same /64 as the exhausted bucket: %s", w.Body.String())
	w = create("[2001:db8:aa:cc::1]:443")
	assert.Equal(t, http.StatusOK, w.Code, "different /64: %s", w.Body.String())
}
