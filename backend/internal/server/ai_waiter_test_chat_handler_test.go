package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateAIWaiterTestSession_NoGuestCookieAndIsolatedRow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "op-test-session", true)

	router := gin.New()
	router.POST("/businesses/:id/ai/test-chat/session", CreateAIWaiterTestSession)

	w := performAIWaiterRequest(t, router, http.MethodPost,
		fmt.Sprintf("/businesses/%d/ai/test-chat/session", biz.ID),
		map[string]any{"language": "en"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Critical security property: must never Set-Cookie the guest session.
	assert.Empty(t, w.Header().Get("Set-Cookie"), "operator test session must not set cookies")
	for _, v := range w.Header().Values("Set-Cookie") {
		assert.NotContains(t, v, "pv_ai_waiter_session=")
	}

	var resp struct {
		SessionToken string `json:"session_token"`
		Greeting     string `json:"greeting"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.True(t, strings.HasPrefix(resp.SessionToken, aiWaiterOperatorTestSessionPrefix))
	assert.NotEmpty(t, resp.Greeting)

	var conv database.AiWaiterConversation
	require.NoError(t, database.GetDB().
		Where("session_id = ? AND business_id = ?", resp.SessionToken, biz.ID).
		First(&conv).Error)
	assert.Equal(t, database.AiWaiterOperatorTestTableCode, conv.TableCode)
	assert.Equal(t, aiWaiterOperatorTestMode, conv.Mode)
}

func TestCreateAIWaiterTestSession_UsesRequestedGuestLocale(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "op-test-locale", true)
	biz.DefaultLanguage = "en"
	require.NoError(t, database.GetDB().Save(biz).Error)

	router := gin.New()
	router.POST("/businesses/:id/ai/test-chat/session", CreateAIWaiterTestSession)

	for _, locale := range []string{"en", "es", "es-AR"} {
		w := performAIWaiterRequest(t, router, http.MethodPost,
			fmt.Sprintf("/businesses/%d/ai/test-chat/session", biz.ID),
			map[string]any{"language": locale})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp struct {
			SessionToken string `json:"session_token"`
			Greeting     string `json:"greeting"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, services.WaiterGreeting(locale, "Alfred", biz.Name, aiWaiterOperatorTestMode, true), resp.Greeting)

		var conv database.AiWaiterConversation
		require.NoError(t, database.GetDB().
			Where("session_id = ? AND business_id = ?", resp.SessionToken, biz.ID).
			First(&conv).Error)
		assert.Equal(t, locale, conv.Language)
	}
}

func TestCreateAIWaiterSession_RejectsOperatorTestTableCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "reject-op-table", true)

	router := gin.New()
	router.POST("/ai-waiter/:businessId/session", CreateAIWaiterSession)

	w := performAIWaiterRequest(t, router, http.MethodPost,
		fmt.Sprintf("/ai-waiter/%d/session", biz.ID),
		map[string]any{
			"table_code": database.AiWaiterOperatorTestTableCode,
			"mode":       "concierge",
			"language":   "en",
		})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Empty(t, w.Header().Get("Set-Cookie"))
}

func TestGetAiConversations_ExcludesOperatorTestRows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "op-test-monitor", true)
	createAIWaiterTable(t, biz.ID, "A1")
	guest := createAIWaiterConversation(t, biz.ID, "guest-monitor-token")

	_, err := database.GetOrCreateAiWaiterConversation(
		"optest-hidden-token",
		biz.ID,
		database.AiWaiterOperatorTestTableCode,
		"en",
		aiWaiterOperatorTestMode,
	)
	require.NoError(t, err)

	router := gin.New()
	router.GET("/businesses/:id/ai/conversations", GetAiConversations)

	w := performAIWaiterRequest(t, router, http.MethodGet,
		fmt.Sprintf("/businesses/%d/ai/conversations", biz.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp GetAiConversationsResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, int64(1), resp.TotalCount)
	require.Equal(t, int64(1), resp.ActiveCount)
	require.Len(t, resp.Conversations, 1)
	assert.Equal(t, guest.ID, resp.Conversations[0].ID)
	assert.NotEqual(t, database.AiWaiterOperatorTestTableCode, resp.Conversations[0].TableCode)
}

func TestHandleAIWaiter_PublicPathRejectsOperatorTestSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "op-test-public-reject", true)
	conv, err := database.GetOrCreateAiWaiterConversation(
		"optest-public-reject",
		biz.ID,
		database.AiWaiterOperatorTestTableCode,
		"en",
		aiWaiterOperatorTestMode,
	)
	require.NoError(t, err)

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)

	w := performAIWaiterRequest(t, router, http.MethodPost,
		fmt.Sprintf("/ai-waiter/%d", biz.ID),
		map[string]any{
			"history":       []map[string]string{{"role": "user", "content": "hi"}},
			"language":      "en",
			"session_token": conv.SessionID,
			"mode":          "concierge",
			"table_code":    database.AiWaiterOperatorTestTableCode,
		})
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), ErrCodeSessionUnknown)
}

func TestHandleAIWaiterTestChat_IgnoresGuestCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "op-test-no-cookie", true)
	createAIWaiterTable(t, biz.ID, "A1")
	guestConv := createAIWaiterConversation(t, biz.ID, "guest-cookie-decoy")

	opConv, err := database.GetOrCreateAiWaiterConversation(
		"optest-no-cookie-token",
		biz.ID,
		database.AiWaiterOperatorTestTableCode,
		"en",
		aiWaiterOperatorTestMode,
	)
	require.NoError(t, err)

	router := gin.New()
	router.POST("/businesses/:id/ai/test-chat", HandleAIWaiterTestChat)

	// Send with guest cookie present but no explicit sandbox token — must 404,
	// never hitch onto the guest cookie session.
	body := `{"history":[{"role":"user","content":"Any peanut dishes?"}],"language":"en"}`
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/businesses/%d/ai/test-chat", biz.ID),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "pv_ai_waiter_session", Value: guestConv.SessionID})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())

	// Explicit sandbox token works (may 503 if AI unset — still not a cookie path).
	bodyOK := fmt.Sprintf(
		`{"history":[{"role":"user","content":"Any peanut dishes?"}],"language":"en","session_token":%q}`,
		opConv.SessionID,
	)
	req2 := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/businesses/%d/ai/test-chat", biz.ID),
		strings.NewReader(bodyOK))
	req2.Header.Set("Content-Type", "application/json")
	req2.AddCookie(&http.Cookie{Name: "pv_ai_waiter_session", Value: guestConv.SessionID})
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Empty(t, w2.Header().Get("Set-Cookie"))
	// Session resolved past AUTH_SESSION_UNKNOWN (menu tables are absent in this
	// harness, so the next failure is "Menu not found" — still proves we did not
	// hitch onto the guest cookie conversation).
	assert.NotContains(t, w2.Body.String(), ErrCodeSessionUnknown)
	assert.Contains(t, w2.Body.String(), "Menu not found")
}

func TestCountAiWaiterMessagesSince_ExcludesOperatorTest(t *testing.T) {
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "op-test-quota", true)
	createAIWaiterTable(t, biz.ID, "A1")
	guest := createAIWaiterConversation(t, biz.ID, "guest-quota-token")
	require.NoError(t, database.SaveAiWaiterMessage(guest.ID, "user", "guest msg", ""))

	op, err := database.GetOrCreateAiWaiterConversation(
		"optest-quota-token",
		biz.ID,
		database.AiWaiterOperatorTestTableCode,
		"en",
		aiWaiterOperatorTestMode,
	)
	require.NoError(t, err)
	require.NoError(t, database.SaveAiWaiterMessage(op.ID, "user", "sandbox msg", ""))

	n := database.CountAiWaiterMessagesSince(biz.ID, guest.CreatedAt.Add(-time.Minute))
	assert.Equal(t, int64(1), n, "operator sandbox messages must not count toward guest quota")
}
