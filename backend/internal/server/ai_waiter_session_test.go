package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateAIWaiterSession_IssuesCryptoTokenAndGreeting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "session-issue", true)
	table := createAIWaiterTable(t, business.ID, "SES-1")

	router := gin.New()
	router.POST("/ai-waiter/:businessId/session", CreateAIWaiterSession)

	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d/session", business.ID), map[string]any{
		"table_code": table.TableCode, "mode": "ordering", "language": "es",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		SessionToken string `json:"session_token"`
		Greeting     string `json:"greeting"`
		ExpiresAt    string `json:"expires_at"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.SessionToken, 64, "expected 64-hex crypto token")
	assert.NotEmpty(t, resp.Greeting)
	assert.NotEmpty(t, resp.ExpiresAt)
}

func TestAIWaiter_ExpiredSession_Rejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "expired-session", true)
	// The conversation helper hardcodes mode="ordering", table_code="A1".
	createAIWaiterTable(t, biz.ID, "A1")
	conv := createAIWaiterConversation(t, biz.ID, "expired-token")
	// Age the conversation past the TTL.
	require.NoError(t, db.Model(&database.AiWaiterConversation{}).
		Where("id = ?", conv.ID).
		Update("created_at", time.Now().Add(-25*time.Hour)).Error)

	router := gin.New()
	router.GET("/ai-waiter/:businessId/messages", GetAiWaiterMessages)

	w := performAIWaiterRequest(t, router, http.MethodGet,
		fmt.Sprintf("/ai-waiter/%d/messages?session_token=%s&mode=ordering&table_code=A1", biz.ID, conv.SessionID), nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "session_expired")
}

func TestCreateAIWaiterSession_SetsHttpOnlyCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "session-cookie", true)

	router := gin.New()
	router.POST("/ai-waiter/:businessId/session", CreateAIWaiterSession)

	w := performAIWaiterRequest(t, router, http.MethodPost,
		fmt.Sprintf("/ai-waiter/%d/session", biz.ID),
		map[string]any{"mode": "concierge", "language": "en"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	sc := w.Header().Get("Set-Cookie")
	assert.Contains(t, sc, "pv_ai_waiter_session=")
	assert.Contains(t, sc, "HttpOnly")
	assert.Contains(t, sc, "SameSite=Lax")
}

func TestGetAiWaiterMessages_AuthFromCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "messages-cookie", true)
	createAIWaiterTable(t, biz.ID, "A1")
	conv := createAIWaiterConversation(t, biz.ID, "messages-cookie-token")
	require.NoError(t, database.SaveAiWaiterMessage(conv.ID, "assistant", "cookie auth works", ""))

	router := gin.New()
	router.GET("/ai-waiter/:businessId/messages", GetAiWaiterMessages)

	// No session_token query param — the HttpOnly pv_ai_waiter_session cookie
	// authenticates the read so the guest UI can keep the token out of the URL
	// (a URL token leaks into proxy/CDN access logs). The stream endpoint is
	// stricter: it binds the tab's in-memory bearer, never this shared cookie.
	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/ai-waiter/%d/messages?mode=ordering&table_code=A1", biz.ID), nil)
	req.AddCookie(&http.Cookie{Name: "pv_ai_waiter_session", Value: conv.SessionID})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "cookie auth works")
}

// Locks the deploy-window precedence: a query session_token must win over a
// (possibly stale) cookie. This is what keeps an old frontend that still sends
// the URL token safe against a new backend that also reads the cookie.
func TestGetAiWaiterMessages_QueryTokenWinsOverCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "messages-precedence", true)
	createAIWaiterTable(t, biz.ID, "A1")
	queryConv := createAIWaiterConversation(t, biz.ID, "query-wins-token")
	cookieConv := createAIWaiterConversation(t, biz.ID, "cookie-decoy-token")
	require.NoError(t, database.SaveAiWaiterMessage(queryConv.ID, "assistant", "from query session", ""))
	require.NoError(t, database.SaveAiWaiterMessage(cookieConv.ID, "assistant", "from cookie session", ""))

	router := gin.New()
	router.GET("/ai-waiter/:businessId/messages", GetAiWaiterMessages)

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/ai-waiter/%d/messages?session_token=%s&mode=ordering&table_code=A1", biz.ID, queryConv.SessionID), nil)
	req.AddCookie(&http.Cookie{Name: "pv_ai_waiter_session", Value: cookieConv.SessionID})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "from query session")
	assert.NotContains(t, w.Body.String(), "from cookie session")
}

func TestHandleAIWaiterStream_RejectsCookieOnlyAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "stream-cookie", true)
	conv := createAIWaiterConversation(t, biz.ID, "stream-cookie-token")

	// Cancel immediately because this test exercises only the authentication
	// gate: the origin-wide cookie must be rejected before any stream opens.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// No session_token query param — only the cookie.
	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/ai-waiter/%d/stream", biz.ID), nil)
	req = req.WithContext(ctx)
	req.AddCookie(&http.Cookie{Name: "pv_ai_waiter_session", Value: conv.SessionID})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "businessId", Value: strconv.Itoa(int(biz.ID))}}
	c.Request = req
	HandleAIWaiterStream(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
}
