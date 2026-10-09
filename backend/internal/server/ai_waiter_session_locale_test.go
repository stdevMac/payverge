package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateAIWaiterSession_PersistsRequestedLocale(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "session-locale", true)
	config.SetAIProviderConfiguredForTesting(t, true)
	table := createAIWaiterTable(t, biz.ID, "LOC-1")

	router := gin.New()
	router.POST("/ai-waiter/:businessId/session", CreateAIWaiterSession)

	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d/session", biz.ID), map[string]any{
		"table_code": table.TableCode, "mode": "ordering", "language": "es-AR",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		SessionToken string `json:"session_token"`
		Greeting     string `json:"greeting"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, services.WaiterGreeting("es-AR", "Alfred", biz.Name, "ordering", false), resp.Greeting)

	var conv database.AiWaiterConversation
	require.NoError(t, database.GetDB().Where("session_id = ?", resp.SessionToken).First(&conv).Error)
	assert.Equal(t, "es-AR", conv.Language)
	assert.Equal(t, "active", conv.Status)
}

func TestCreateAIWaiterSession_ReplaceClosesPredecessorAndStartsLocaleSpecificConversation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "session-replace", true)
	config.SetAIProviderConfiguredForTesting(t, true)
	table := createAIWaiterTable(t, biz.ID, "REP-1")

	router := gin.New()
	router.POST("/ai-waiter/:businessId/session", CreateAIWaiterSession)

	first := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d/session", biz.ID), map[string]any{
		"table_code": table.TableCode, "mode": "ordering", "language": "en",
	})
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	var firstResp struct {
		SessionToken string `json:"session_token"`
	}
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &firstResp))

	second := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d/session", biz.ID), map[string]any{
		"table_code":            table.TableCode,
		"mode":                  "ordering",
		"language":              "es-AR",
		"replace_session_token": firstResp.SessionToken,
	})
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	var secondResp struct {
		SessionToken string `json:"session_token"`
		Greeting     string `json:"greeting"`
	}
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &secondResp))
	require.NotEqual(t, firstResp.SessionToken, secondResp.SessionToken)
	assert.Equal(t, services.WaiterGreeting("es-AR", "Alfred", biz.Name, "ordering", false), secondResp.Greeting)

	var oldConv, newConv database.AiWaiterConversation
	require.NoError(t, database.GetDB().Where("session_id = ?", firstResp.SessionToken).First(&oldConv).Error)
	require.NoError(t, database.GetDB().Where("session_id = ?", secondResp.SessionToken).First(&newConv).Error)
	assert.Equal(t, "closed", oldConv.Status)
	assert.Equal(t, "en", oldConv.Language)
	assert.Equal(t, "active", newConv.Status)
	assert.Equal(t, "es-AR", newConv.Language)

	messages, err := database.GetAiWaiterMessagesForTranscript(oldConv.ID, nil, 20)
	require.NoError(t, err)
	require.NotEmpty(t, messages)
	var sawTransition bool
	for _, msg := range messages {
		if msg.Role == "system" && msg.Content == services.WaiterNewConversationTransition("es-AR") {
			sawTransition = true
		}
	}
	assert.True(t, sawTransition, "closed conversation must expose the locale transition")
}

func TestCreateAIWaiterSession_ReplaceIgnoresUnknownAndForeignTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "session-replace-safe", true)
	other := createAIWaiterBusiness(t, "session-replace-other", true)
	table := createAIWaiterTable(t, biz.ID, "SAFE-1")
	foreign := createAIWaiterConversation(t, other.ID, "foreign-token")

	router := gin.New()
	router.POST("/ai-waiter/:businessId/session", CreateAIWaiterSession)
	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d/session", biz.ID), map[string]any{
		"table_code":            table.TableCode,
		"mode":                  "ordering",
		"language":              "en",
		"replace_session_token": foreign.SessionID,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var reloaded database.AiWaiterConversation
	require.NoError(t, database.GetDB().First(&reloaded, foreign.ID).Error)
	assert.Equal(t, "active", reloaded.Status)
}

func TestGetAiWaiterMessages_HeaderTokenWinsOverCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "messages-header", true)
	createAIWaiterTable(t, biz.ID, "A1")
	headerConv := createAIWaiterConversation(t, biz.ID, "header-wins-token")
	cookieConv := createAIWaiterConversation(t, biz.ID, "cookie-loses-token")
	require.NoError(t, database.SaveAiWaiterMessage(headerConv.ID, "assistant", "from header session", ""))
	require.NoError(t, database.SaveAiWaiterMessage(cookieConv.ID, "assistant", "from cookie session", ""))

	router := gin.New()
	router.GET("/ai-waiter/:businessId/messages", GetAiWaiterMessages)

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/ai-waiter/%d/messages?mode=ordering&table_code=A1", biz.ID), nil)
	req.Header.Set(aiWaiterSessionHeader, headerConv.SessionID)
	req.AddCookie(&http.Cookie{Name: "pv_ai_waiter_session", Value: cookieConv.SessionID})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "from header session")
	assert.NotContains(t, w.Body.String(), "from cookie session")
}

func TestHandleAIWaiter_UpdatesConversationLanguageAndRecordsTransition(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "chat-locale-update", true)
	createAIWaiterTable(t, biz.ID, "A1")
	require.NoError(t, db.AutoMigrate(&database.Menu{}))
	require.NoError(t, db.Create(&database.Menu{BusinessID: biz.ID, Categories: "[]", IsActive: true, Version: 1}).Error)
	conv := createAIWaiterConversation(t, biz.ID, "locale-update-token")
	require.Equal(t, "en", conv.Language)

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", biz.ID), map[string]any{
		"session_token": conv.SessionID,
		"mode":          "ordering",
		"table_code":    "A1",
		"language":      "es-AR",
		"history":       []map[string]any{{"role": "user", "content": "¿Qué me recomendás?"}},
	})
	// Language sync must happen even if the model path is unconfigured.
	assert.NotEqual(t, http.StatusUnauthorized, w.Code)

	var reloaded database.AiWaiterConversation
	require.NoError(t, database.GetDB().First(&reloaded, conv.ID).Error)
	assert.Equal(t, "es-AR", reloaded.Language)

	messages, err := database.GetAiWaiterMessagesForTranscript(conv.ID, nil, 20)
	require.NoError(t, err)
	var sawTransition bool
	for _, msg := range messages {
		if msg.Role == "system" && msg.Content == services.WaiterLanguageTransition("en", "es-AR") {
			sawTransition = true
		}
	}
	assert.True(t, sawTransition)
}

func TestCreateAIWaiterSession_EnToEsARAndBackCreatesLocaleSpecificRows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "session-roundtrip", true)
	table := createAIWaiterTable(t, biz.ID, "RT-1")

	router := gin.New()
	router.POST("/ai-waiter/:businessId/session", CreateAIWaiterSession)

	create := func(language, replace string) string {
		body := map[string]any{
			"table_code": table.TableCode, "mode": "ordering", "language": language,
		}
		if replace != "" {
			body["replace_session_token"] = replace
		}
		w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d/session", biz.ID), body)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var resp struct {
			SessionToken string `json:"session_token"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		return resp.SessionToken
	}

	enToken := create("en", "")
	esToken := create("es-AR", enToken)
	enAgain := create("en", esToken)

	var enConv, esConv, en2Conv database.AiWaiterConversation
	require.NoError(t, database.GetDB().Where("session_id = ?", enToken).First(&enConv).Error)
	require.NoError(t, database.GetDB().Where("session_id = ?", esToken).First(&esConv).Error)
	require.NoError(t, database.GetDB().Where("session_id = ?", enAgain).First(&en2Conv).Error)
	assert.Equal(t, "closed", enConv.Status)
	assert.Equal(t, "en", enConv.Language)
	assert.Equal(t, "closed", esConv.Status)
	assert.Equal(t, "es-AR", esConv.Language)
	assert.Equal(t, "active", en2Conv.Status)
	assert.Equal(t, "en", en2Conv.Language)
}

// With no model configured the waiter answers from the menu snapshot with set
// replies, so its greeting must not present itself as an AI.
func TestCreateAIWaiterSession_NoModelUsesScriptedGreeting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "session-scripted", true)
	table := createAIWaiterTable(t, biz.ID, "SCR-1")
	config.SetAIProviderConfiguredForTesting(t, false)

	router := gin.New()
	router.POST("/ai-waiter/:businessId/session", CreateAIWaiterSession)

	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d/session", biz.ID), map[string]any{
		"table_code": table.TableCode, "mode": "ordering", "language": "en",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Greeting string `json:"greeting"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, services.WaiterScriptedGreeting("en", "Alfred", biz.Name), resp.Greeting)
	assert.NotContains(t, resp.Greeting, "chatting with an AI")
}
