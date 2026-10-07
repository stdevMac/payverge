package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/aiwaiterevents"
	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAIWaiterSSEIncludesTheSameResponseV2(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "stream-v2", true)
	conv := createAIWaiterConversation(t, business.ID, "tok-stream-v2")
	response := assistantcontract.NewResponse("waiter-stream-response", "Grounded stream answer")
	response.Answer.Format = assistantcontract.FormatPlainText

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(50 * time.Millisecond)
		aiwaiterevents.GetHub().PublishMessage(conv.ID, aiwaiterevents.MessageEvent{
			ID: 99, Role: "assistant", Content: response.Answer.Content, ResponseV2: &response,
		})
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	w := streamRequest(t, strconv.Itoa(int(business.ID)), conv.SessionID, ctx)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"response_v2":{"version":2,"response_id":"waiter-stream-response"`)
	assert.Contains(t, w.Body.String(), `"content":"Grounded stream answer"`)
}

func TestAIWaiterSSEUserEventsNeverCarryAssistantResponseV2(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "stream-user-no-v2", true)
	conv := createAIWaiterConversation(t, business.ID, "tok-stream-user-no-v2")
	hub := aiwaiterevents.GetHub()
	ch, cancel, err := hub.Subscribe(conv.ID)
	require.NoError(t, err)
	defer cancel()

	publishAiWaiterMessage(conv, 7, "user", "guest message", time.Now())
	select {
	case event := <-ch:
		assert.Nil(t, event.ResponseV2)
	case <-time.After(time.Second):
		t.Fatal("user event should still be delivered")
	}
}

func TestAIWaiterSSERedactsStaffContentAndCarriesResponseV2(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "stream-staff-v1", true)
	conv := createAIWaiterConversation(t, business.ID, "tok-stream-staff-v1")
	hub := aiwaiterevents.GetHub()
	ch, cancel, err := hub.Subscribe(conv.ID)
	require.NoError(t, err)
	defer cancel()

	publishAiWaiterMessage(conv, 8, "assistant", "Email guest@example.com", time.Now())
	select {
	case event := <-ch:
		assert.Equal(t, "assistant", event.Role)
		assert.Equal(t, "Email [redacted-email]", event.Content)
		require.NotNil(t, event.ResponseV2)
		assert.Equal(t, "Email [redacted-email]", event.ResponseV2.Answer.Content)
	case <-time.After(time.Second):
		t.Fatal("assistant events must deliver the redacted staff message with its V2 payload")
	}
}

func TestAIWaiterSSEScopesMatchingTokensByConversation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	businessA := createAIWaiterBusiness(t, "stream-same-token-a", true)
	businessB := createAIWaiterBusiness(t, "stream-same-token-b", true)
	convA := createAIWaiterConversation(t, businessA.ID, "shared-cross-business-token")
	convB := createAIWaiterConversation(t, businessB.ID, "shared-cross-business-token")
	hub := aiwaiterevents.GetHub()
	streamA, cancelA, err := hub.Subscribe(convA.ID)
	require.NoError(t, err)
	defer cancelA()
	streamB, cancelB, err := hub.Subscribe(convB.ID)
	require.NoError(t, err)
	defer cancelB()

	publishAiWaiterMessage(convA, 22, "assistant", "business A reply", time.Now())
	select {
	case got := <-streamA:
		assert.Equal(t, uint(22), got.ID)
	case <-time.After(time.Second):
		t.Fatal("the matching conversation must receive its event")
	}
	select {
	case <-streamB:
		t.Fatal("same session token in another business must not receive the event")
	case <-time.After(100 * time.Millisecond):
	}
}

func streamRequest(t testing.TB, businessParam, sessionToken string, ctx context.Context) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet,
		"/ai-waiter/"+businessParam+"/stream", nil)
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "businessId", Value: businessParam}}
	c.Request = req
	HandleAIWaiterStream(c)
	return w
}

// The legacy session_token query-param fallback leaked tokens into proxy logs.
// A valid token supplied solely as a query param is ignored.
func TestAIWaiterStream_QueryParamTokenIsIgnored(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "stream-noquery", true)
	conv := createAIWaiterConversation(t, business.ID, "tok-noquery")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := httptest.NewRequest(http.MethodGet,
		"/ai-waiter/"+strconv.Itoa(int(business.ID))+"/stream?session_token="+conv.SessionID, nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "businessId", Value: strconv.Itoa(int(business.ID))}}
	c.Request = req
	HandleAIWaiterStream(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code,
		"query-param session_token must be ignored — header bearer only")
}

func TestAIWaiterStream_HeaderBearerWinsOverDecoyCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "stream-header-wins", true)
	headerConv := createAIWaiterConversation(t, business.ID, "header-token")
	decoyBusiness := createAIWaiterBusiness(t, "stream-cookie-decoy", true)
	createAIWaiterConversation(t, decoyBusiness.ID, "cookie-decoy-token")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := httptest.NewRequest(http.MethodGet,
		"/ai-waiter/"+strconv.Itoa(int(business.ID))+"/stream", nil)
	req.Header.Set("Authorization", "Bearer "+headerConv.SessionID)
	req.AddCookie(&http.Cookie{Name: "pv_ai_waiter_session", Value: "cookie-decoy-token"})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "businessId", Value: strconv.Itoa(int(business.ID))}}
	c.Request = req
	HandleAIWaiterStream(c)

	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestAIWaiterStream_MalformedOrWrongHeaderNeverFallsBackToCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "stream-header-strict", true)
	conv := createAIWaiterConversation(t, business.ID, "valid-cookie-token")

	for _, tc := range []struct {
		name   string
		header string
		status int
	}{
		{name: "wrong scheme", header: "Token " + conv.SessionID, status: http.StatusUnauthorized},
		{name: "lowercase scheme", header: "bearer " + conv.SessionID, status: http.StatusUnauthorized},
		{name: "blank bearer", header: "Bearer ", status: http.StatusUnauthorized},
		{name: "wrong bearer", header: "Bearer wrong-token", status: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			req := httptest.NewRequest(http.MethodGet,
				"/ai-waiter/"+strconv.Itoa(int(business.ID))+"/stream", nil)
			req.Header.Set("Authorization", tc.header)
			req.AddCookie(&http.Cookie{Name: "pv_ai_waiter_session", Value: conv.SessionID})
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = gin.Params{{Key: "businessId", Value: strconv.Itoa(int(business.ID))}}
			c.Request = req
			HandleAIWaiterStream(c)
			assert.Equal(t, tc.status, w.Code, w.Body.String())
		})
	}
}

func TestAIWaiterStream_UnknownTokenIs404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "stream-unknown", true)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := streamRequest(t, strconv.Itoa(int(business.ID)), "deadbeefdeadbeef", ctx)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), `"code":"AUTH_SESSION_UNKNOWN"`)
}

func TestAIWaiterStream_RequiresAiEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "stream-disabled", false)
	conv := createAIWaiterConversation(t, business.ID, "tok-disabled")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := streamRequest(t, strconv.Itoa(int(business.ID)), conv.SessionID, ctx)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestAIWaiterStream_EmitsConnectedThenDeliversPublishedMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "stream-deliver", true)
	conv := createAIWaiterConversation(t, business.ID, "tok-deliver")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		time.Sleep(50 * time.Millisecond)
		aiwaiterevents.GetHub().PublishMessage(conv.ID, aiwaiterevents.MessageEvent{
			ID: 99, Role: "assistant", Content: "table 4 ready",
		})
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	w := streamRequest(t, strconv.Itoa(int(business.ID)), conv.SessionID, ctx)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	body := w.Body.String()
	assert.Contains(t, body, "event: connected")
	assert.Contains(t, body, "event: message.created")
	assert.Contains(t, body, `"content":"table 4 ready"`)
	assert.True(t, strings.Index(body, "event: connected") < strings.Index(body, "event: message.created"),
		"connected frame must precede the message frame")
}

func TestAIWaiterStream_KeepaliveCommentOnIdle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "stream-keepalive", true)
	conv := createAIWaiterConversation(t, business.ID, "tok-keepalive")

	prev := aiWaiterStreamKeepalive
	aiWaiterStreamKeepalive = 20 * time.Millisecond
	t.Cleanup(func() { aiWaiterStreamKeepalive = prev })

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	w := streamRequest(t, strconv.Itoa(int(business.ID)), conv.SessionID, ctx)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), ": ping")
}

func TestAIWaiterStream_RejectsBusinessNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := streamRequest(t, "999999", "anytoken", ctx)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAIWaiterStream_TokenScopedToBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	bizA := createAIWaiterBusiness(t, "stream-scope-a", true)
	bizB := createAIWaiterBusiness(t, "stream-scope-b", true)
	convB := createAIWaiterConversation(t, bizB.ID, "tok-belongs-to-b")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := streamRequest(t, strconv.Itoa(int(bizA.ID)), convB.SessionID, ctx)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), `"code":"AUTH_SESSION_UNKNOWN"`)
	_ = require.NotNil
}

func TestPostAiReply_PublishesStreamEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "reply-pub", true)
	createAIWaiterTable(t, business.ID, "A1")
	manager := createAIWaiterStaff(t, business.ID, database.StaffRoleManager, "mgr-pub@example.com")
	conv := createAIWaiterConversation(t, business.ID, "tok-reply-pub")
	// Decision #4: replying requires holding a fresh claim, managers included.
	// This test is about the stream event, so give the manager the claim rather
	// than asserting the claim rule here.
	require.NoError(t, database.GetDB().Model(&database.AiWaiterConversation{}).
		Where("id = ?", conv.ID).
		Updates(map[string]interface{}{
			"claimed_by_staff_id": manager.ID,
			"claimed_by_name":     "Publisher",
			"claimed_by_role":     "manager",
			"claimed_at":          time.Now(),
		}).Error)

	hub := aiwaiterevents.GetHub()
	ch, cancel, err := hub.Subscribe(conv.ID)
	require.NoError(t, err)
	defer cancel()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleManager))
		c.Set("staff_id", manager.ID)
		c.Set("staff_business_id", business.ID)
		c.Next()
	})
	router.POST("/businesses/:id/ai/conversations/:convId/reply",
		RoleBasedAccessMiddleware("ai_waiter:write"), PostAiReply)

	w := performAIWaiterRequest(t, router, http.MethodPost,
		fmt.Sprintf("/businesses/%d/ai/conversations/%d/reply", business.ID, conv.ID),
		map[string]any{"content": "Email guest@example.com or call +54 9 11 5555 1234."})
	require.Equal(t, http.StatusOK, w.Code)

	var staffEvent aiwaiterevents.MessageEvent
	select {
	case evt := <-ch:
		staffEvent = evt
		assert.Equal(t, "assistant", evt.Role)
		assert.Equal(t, "Email [redacted-email] or call [redacted-phone].", evt.Content)
		assert.NotZero(t, evt.ID)
		require.NotNil(t, evt.ResponseV2)
		assert.Equal(t, evt.Content, evt.ResponseV2.Answer.Content)
		var stored database.AiWaiterMessage
		require.NoError(t, database.GetDB().First(&stored, evt.ID).Error)
		assert.Equal(t, evt.Content, stored.Content)
	case <-time.After(time.Second):
		t.Fatal("staff reply must publish a message.created event to the session hub")
	}

	historyRouter := gin.New()
	historyRouter.GET("/ai-waiter/:businessId/messages", GetAiWaiterMessages)
	history := performAIWaiterRequest(t, historyRouter, http.MethodGet,
		fmt.Sprintf("/ai-waiter/%d/messages?session_token=%s&mode=ordering&table_code=A1&language=en", business.ID, conv.SessionID), nil)
	require.Equal(t, http.StatusOK, history.Code, history.Body.String())
	var messages []struct {
		ID         uint                        `json:"id"`
		Content    string                      `json:"content"`
		ResponseV2 *assistantcontract.Response `json:"response_v2"`
	}
	require.NoError(t, json.Unmarshal(history.Body.Bytes(), &messages))
	require.Len(t, messages, 1)
	assert.Equal(t, staffEvent.ID, messages[0].ID)
	assert.Equal(t, staffEvent.Content, messages[0].Content)
	require.NotNil(t, messages[0].ResponseV2)
	assert.Equal(t, *staffEvent.ResponseV2, *messages[0].ResponseV2)
}

func TestHandleAIWaiter_PausedTakeoverPublishesNothingExtra(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.Menu{}))
	business := createAIWaiterBusiness(t, "paused-nodup", true)
	createAIWaiterTable(t, business.ID, "A1")
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: business.ID, Categories: "[]", IsActive: true, Version: 1,
	}).Error)
	conv := createAIWaiterConversation(t, business.ID, "tok-paused")
	require.NoError(t, database.GetDB().Model(&database.AiWaiterConversation{}).
		Where("id = ?", conv.ID).Update("is_paused", true).Error)
	require.NoError(t, database.SaveAiWaiterMessage(conv.ID, "assistant", "human here", ""))

	hub := aiwaiterevents.GetHub()
	ch, cancel, err := hub.Subscribe(conv.ID)
	require.NoError(t, err)
	defer cancel()

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	w := performAIWaiterRequest(t, router, http.MethodPost,
		fmt.Sprintf("/ai-waiter/%d", business.ID),
		map[string]any{
			"history":       []map[string]any{{"role": "user", "content": "hi"}},
			"session_token": conv.SessionID,
			"table_code":    "A1",
			"mode":          "ordering",
			"language":      "en",
		})
	require.Equal(t, http.StatusOK, w.Code)

	select {
	case <-ch:
		t.Fatal("paused takeover read must not publish a phantom message.created")
	case <-time.After(150 * time.Millisecond):
	}
}

// TestPublishAiWaiterMessage_IssuesNoQuery proves the publish path takes its
// fields from the caller and never reads the DB (finding SSE-02). Under the old
// re-query implementation this would count >=1 query AND publish whatever the
// DB returned instead of the caller's exact id/role/content.
func TestPublishAiWaiterMessage_IssuesNoQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "pub-noquery", true)
	conv := createAIWaiterConversation(t, business.ID, "tok-pub-noquery")

	// A persisted row exists so the OLD re-query path has something to fetch
	// and "succeed" with — the new path must still ignore it.
	require.NoError(t, database.SaveAiWaiterMessage(conv.ID, "assistant", "stale row", ""))

	var queries int64
	const cbName = "perf:publish_no_query"
	require.NoError(t, database.GetDB().Callback().Query().Before("gorm:query").
		Register(cbName, func(*gorm.DB) { atomic.AddInt64(&queries, 1) }))
	t.Cleanup(func() {
		_ = database.GetDB().Callback().Query().Remove(cbName)
	})

	hub := aiwaiterevents.GetHub()
	ch, cancel, err := hub.Subscribe(conv.ID)
	require.NoError(t, err)
	defer cancel()

	atomic.StoreInt64(&queries, 0)
	publishAiWaiterMessage(conv, 42, "assistant", "hi", time.Now())

	select {
	case evt := <-ch:
		assert.Equal(t, uint(42), evt.ID, "must publish the caller-held id, not a re-queried row")
		assert.Equal(t, "assistant", evt.Role)
		assert.Equal(t, "hi", evt.Content)
	case <-time.After(time.Second):
		t.Fatal("publish must deliver the caller's message to the session hub")
	}

	assert.Equal(t, int64(0), atomic.LoadInt64(&queries),
		"publish must issue zero DB queries (got %d)", atomic.LoadInt64(&queries))
}

// TestPublishAiWaiterMessage_NoRaceReQuery proves the publish path is immune to
// the concurrent-newer-message race the re-query had: it must publish exactly
// the message the caller saved (A), never a newer row (B) written between the
// save and the publish. Under the old re-query, B would be published.
func TestPublishAiWaiterMessage_NoRaceReQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "pub-norace", true)
	conv := createAIWaiterConversation(t, business.ID, "tok-pub-norace")

	// Caller saves message A and holds its fields.
	idA, createdA, err := database.SaveAiWaiterMessageReturningID(conv.ID, "assistant", "message A", "")
	require.NoError(t, err)

	// A concurrent newer message B lands for the same conversation before publish.
	_, _, err = database.SaveAiWaiterMessageReturningID(conv.ID, "assistant", "message B (newer)", "")
	require.NoError(t, err)

	hub := aiwaiterevents.GetHub()
	ch, cancel, err := hub.Subscribe(conv.ID)
	require.NoError(t, err)
	defer cancel()

	publishAiWaiterMessage(conv, idA, "assistant", "message A", createdA)

	select {
	case evt := <-ch:
		assert.Equal(t, idA, evt.ID, "must publish A's id, not the newer B")
		assert.Equal(t, "message A", evt.Content, "must publish A's content, not the newer B")
	case <-time.After(time.Second):
		t.Fatal("publish must deliver message A to the session hub")
	}
}

func TestAIWaiterStreamRejectsWhenConversationSlotsAreFull(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "stream-cap", true)
	conv := createAIWaiterConversation(t, business.ID, "tok-stream-cap")

	hub := aiwaiterevents.GetHub()
	for i := 0; i < aiwaiterevents.DefaultMaxPerConversation; i++ {
		_, cancel, err := hub.Subscribe(conv.ID)
		require.NoError(t, err)
		t.Cleanup(cancel)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := streamRequest(t, strconv.Itoa(int(business.ID)), conv.SessionID, ctx)
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.NotEqual(t, "text/event-stream", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), `"code":"sse_connection_limit"`)
}
