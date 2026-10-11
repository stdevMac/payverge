package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleAIWaiter_UnknownSessionToken_Returns404SessionUnknown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "gate-chat", true)
	table := createAIWaiterTable(t, business.ID, "GATE-1")
	require.NoError(t, db.AutoMigrate(&database.Menu{}))
	require.NoError(t, db.Create(&database.Menu{BusinessID: business.ID, Categories: "[]", IsActive: true, Version: 1}).Error)

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)

	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": "never-issued-token", "mode": "ordering", "table_code": table.TableCode, "language": "en",
		"history": []map[string]any{{"role": "user", "content": "hi"}},
	})
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "AUTH_SESSION_UNKNOWN", resp["code"])

	var count int64
	require.NoError(t, db.Model(&database.AiWaiterConversation{}).Where("business_id = ?", business.ID).Count(&count).Error)
	assert.Zero(t, count, "unknown token must not create a conversation")
}

func TestGetAiWaiterMessages_UnknownSessionToken_Returns404SessionUnknown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "gate-msgs", true)
	createAIWaiterTable(t, business.ID, "GATE-2")

	router := gin.New()
	router.GET("/ai-waiter/:businessId/messages", GetAiWaiterMessages)
	w := performAIWaiterRequest(t, router, http.MethodGet, fmt.Sprintf("/ai-waiter/%d/messages?session_token=never&mode=ordering&table_code=GATE-2", business.ID), nil)
	require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "AUTH_SESSION_UNKNOWN", resp["code"])
}

// TestHandleAIWaiter_AcceptsSessionTokenBodyField guards the wire contract
// used by the guest UI (AiWaiter.tsx): the chat POST sends the session token
// in the `session_token` body field. A valid token must resolve the
// conversation and reach the model (200), not be rejected as session_unknown.
func TestHandleAIWaiter_AcceptsSessionTokenBodyField(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "chat-token", true)
	table := createAIWaiterTable(t, business.ID, "CHAT-TBL-1")

	require.NoError(t, db.AutoMigrate(&database.Menu{}))
	require.NoError(t, db.Create(&database.Menu{BusinessID: business.ID, Categories: "[]", IsActive: true, Version: 1}).Error)

	stub := &stubWaiterProvider{resp: &llm.Response{Text: "Hola, con gusto."}}
	aiSvc, err := services.NewAIService(stub, llm.ModelConfig{Chat: "test-chat", Image: "test-image", Director: "test-director", Menu: "test-menu"})
	require.NoError(t, err)
	SetAIService(aiSvc)
	t.Cleanup(func() { SetAIService(nil) })

	conv := createAIWaiterConversation(t, business.ID, "valid-chat-token")
	conv.TableCode = table.TableCode
	require.NoError(t, db.Save(conv).Error)

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": conv.SessionID,
		"mode":          "ordering",
		"table_code":    table.TableCode,
		"history":       []map[string]any{{"role": "user", "content": "hola"}},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

// TestGetAiWaiterMessages_AcceptsSessionTokenQueryParam guards the wire
// contract used by the guest UI (AiWaiter.tsx) and the /stream endpoint:
// the session token is sent as the `session_token` query param. A valid
// token must resolve the conversation and return 200, not a 400
// "Session ID is required".
func TestGetAiWaiterMessages_AcceptsSessionTokenQueryParam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "msgs-token", true)
	createAIWaiterTable(t, business.ID, "A1")
	conv := createAIWaiterConversation(t, business.ID, "valid-token-abc")

	router := gin.New()
	router.GET("/ai-waiter/:businessId/messages", GetAiWaiterMessages)
	w := performAIWaiterRequest(t, router, http.MethodGet, fmt.Sprintf("/ai-waiter/%d/messages?session_token=%s&mode=ordering&table_code=%s", business.ID, conv.SessionID, conv.TableCode), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}
