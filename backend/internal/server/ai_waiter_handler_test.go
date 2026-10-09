package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/aiwaiterevents"
	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAIWaiterHistoryStrictlyRejectsUnknownTrailingAndWrongVersionV2(t *testing.T) {
	valid := assistantcontract.NewResponse("strict-response", "Do not trust extensions.")
	valid.Answer.Format = assistantcontract.FormatPlainText
	raw, err := json.Marshal(valid)
	require.NoError(t, err)

	tests := []struct {
		name string
		raw  string
	}{
		{name: "null structured value", raw: `null`},
		{name: "unknown field", raw: string(raw[:len(raw)-1]) + `,"model_action":{"href":"https://evil.example"}}`},
		{name: "trailing value", raw: string(raw) + ` {}`},
		{name: "wrong version", raw: string(func() []byte {
			copyResponse := valid
			copyResponse.Version = 3
			encoded, marshalErr := json.Marshal(copyResponse)
			require.NoError(t, marshalErr)
			return encoded
		}())},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := restoreAiWaiterResponseV2(database.AiWaiterMessage{
				ID: 42, Role: "assistant", Content: "Safe readable history", StructuredResponse: test.raw,
			}, "en")
			require.NoError(t, assistantcontract.Validate(got))
			assert.Equal(t, "waiter-message-42", got.ResponseID)
			assert.Equal(t, assistantcontract.StatusDegraded, got.Status)
			assert.Equal(t, assistantcontract.FormatPlainText, got.Answer.Format)
			assert.Equal(t, "Safe readable history", got.Answer.Content)
			assert.Empty(t, got.Actions)
			assert.NotContains(t, got.Answer.Content, "evil.example")
		})
	}
}

func TestAIWaiterHistoryBlankLegacyOrMalformedRowsUseAValidFallback(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
	}{
		{name: "legacy blank", raw: `{}`},
		{name: "malformed blank", raw: `{"version":2`},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := restoreAiWaiterResponseV2(database.AiWaiterMessage{
				ID: 71, Role: "assistant", Content: " \t\n", StructuredResponse: test.raw,
			}, "es")
			require.NoError(t, assistantcontract.Validate(got))
			assert.Equal(t, assistantcontract.StatusDegraded, got.Status)
			assert.Equal(t, assistantcontract.FormatPlainText, got.Answer.Format)
			assert.NotEmpty(t, strings.TrimSpace(got.Answer.Content))
			assert.Empty(t, got.Actions)
		})
	}
}

func TestAIWaiterHistoryBoundsOuterContentToTheSafeProjectedAnswer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "task5-history-bounds", true)
	createAIWaiterTable(t, business.ID, "A1")
	conv := createAIWaiterConversation(t, business.ID, "task5-history-bounds-token")
	rows := []database.AiWaiterMessage{
		{ConversationID: conv.ID, Role: "assistant", Content: " \n\t", StructuredResponse: `{}`, CreatedAt: time.Now()},
		{ConversationID: conv.ID, Role: "assistant", Content: strings.Repeat("x", 13000), StructuredResponse: `{}`, CreatedAt: time.Now().Add(time.Second)},
		{ConversationID: conv.ID, Role: "assistant", Content: "", StructuredResponse: `{"version":2`, CreatedAt: time.Now().Add(2 * time.Second)},
	}
	for index := range rows {
		require.NoError(t, db.Create(&rows[index]).Error)
	}

	router := gin.New()
	router.GET("/ai-waiter/:businessId/messages", GetAiWaiterMessages)
	w := performAIWaiterRequest(t, router, http.MethodGet,
		fmt.Sprintf("/ai-waiter/%d/messages?session_token=%s&mode=ordering&table_code=A1&language=es", business.ID, conv.SessionID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var messages []struct {
		Content    string                      `json:"content"`
		ResponseV2 *assistantcontract.Response `json:"response_v2"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &messages))
	require.Len(t, messages, len(rows))
	for _, message := range messages {
		require.NotNil(t, message.ResponseV2)
		require.NoError(t, assistantcontract.Validate(*message.ResponseV2))
		assert.Equal(t, message.ResponseV2.Answer.Content, message.Content)
		assert.NotEmpty(t, strings.TrimSpace(message.Content))
		assert.LessOrEqual(t, len([]rune(message.Content)), 12000)
	}
}

func TestAIWaiterDirectV2SaveFailureHasNoDurableIDOrSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	business, tableCode := seedAIWaiterChatBusiness(t, db, "task5-save-failure", "task5-save-failure-token")
	provider := &stubWaiterProvider{resp: &llm.Response{Text: "Payment succeeded for guest@example.com"}}
	service, err := services.NewAIService(provider, llm.ModelConfig{Chat: "m", Image: "i", Director: "d", Menu: "mn"})
	require.NoError(t, err)
	SetAIService(service)
	t.Cleanup(func() { SetAIService(nil) })

	const callbackName = "task5:reject_assistant_save"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(callbackName, func(tx *gorm.DB) {
		if message, ok := tx.Statement.Dest.(*database.AiWaiterMessage); ok && message.Role == "assistant" {
			tx.AddError(errors.New("forced assistant persistence failure"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Create().Remove(callbackName) })
	conv, ok := database.FindAiWaiterConversation("task5-save-failure-token", business.ID, "ordering", tableCode)
	require.True(t, ok)
	events, cancel, err := aiwaiterevents.GetHub().Subscribe(conv.ID)
	require.NoError(t, err)
	defer cancel()

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": conv.SessionID, "mode": "ordering", "table_code": tableCode, "language": "en",
		"history": []map[string]any{{"role": "user", "content": "Tell me a joke."}},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var direct struct {
		ID         uint                       `json:"id"`
		ResponseV2 assistantcontract.Response `json:"response_v2"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &direct))
	assert.Zero(t, direct.ID)
	require.NoError(t, assistantcontract.Validate(direct.ResponseV2))
	assert.NotContains(t, w.Body.String(), "Payment succeeded")
	assert.NotContains(t, w.Body.String(), "guest@example.com")
	select {
	case <-events:
		t.Fatal("failed persistence must not publish a phantom SSE event")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestAIWaiterDirectV2EntropyFailureStopsBeforeProviderOrPersistence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	business, tableCode := seedAIWaiterChatBusiness(t, db, "task5-entropy", "task5-entropy-token")
	provider := &countingProvider{}
	service, err := services.NewAIService(provider, llm.ModelConfig{Chat: "m", Image: "i", Director: "d", Menu: "mn"})
	require.NoError(t, err)
	SetAIService(service)
	t.Cleanup(func() { SetAIService(nil) })

	previous := newAIWaiterResponseID
	newAIWaiterResponseID = func(uint) (string, error) { return "", errors.New("entropy unavailable") }
	t.Cleanup(func() { newAIWaiterResponseID = previous })

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": "task5-entropy-token", "mode": "ordering", "table_code": tableCode, "language": "en",
		"history": []map[string]any{{"role": "user", "content": "Please add a burger."}},
	})
	assert.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
	assert.Zero(t, provider.calls)
	var count int64
	require.NoError(t, db.Model(&database.AiWaiterMessage{}).Count(&count).Error)
	assert.Zero(t, count, "entropy failure must happen before user or assistant persistence")
}

func TestAIWaiterDirectV2IsPersistedPublishedAndLegacyProjectedOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "task5-direct-v2", true)
	table := createAIWaiterTable(t, business.ID, "V2-DIRECT")
	require.NoError(t, db.AutoMigrate(&database.Menu{}))
	menuJSON, err := json.Marshal([]database.MenuCategory{{
		ID: "mains", Name: "Mains", Items: []database.MenuItem{{
			ID: "burger-stable-id", Name: "Harvest Burger owner@example.com", Price: 14, Currency: "USD", IsAvailable: true,
		}},
	}})
	require.NoError(t, err)
	require.NoError(t, db.Create(&database.Menu{
		BusinessID: business.ID, Categories: string(menuJSON), IsActive: true, Version: 1,
	}).Error)

	provider := &stubWaiterProvider{resp: &llm.Response{
		Text: "Done! I added it.",
		ToolCalls: []llm.ToolCall{{
			ID: "provider-call", Name: "add_to_cart", Args: map[string]any{
				"item_type": "menu_item", "menu_item_id": "burger-stable-id",
				"item_name": "Harvest Burger owner@example.com", "quantity": float64(2),
				"notes": "email guest@example.com or call +54 9 11 5555 1234",
			},
		}},
	}}
	service, err := services.NewAIService(provider, llm.ModelConfig{Chat: "m", Image: "i", Director: "d", Menu: "mn"})
	require.NoError(t, err)
	SetAIService(service)
	t.Cleanup(func() { SetAIService(nil) })

	conv := createAIWaiterConversation(t, business.ID, "task5-direct-v2-token")
	conv.TableCode = table.TableCode
	require.NoError(t, db.Save(conv).Error)
	events, cancel, err := aiwaiterevents.GetHub().Subscribe(conv.ID)
	require.NoError(t, err)
	defer cancel()

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": conv.SessionID, "mode": "ordering", "table_code": table.TableCode, "language": "en",
		"history": []map[string]any{{"role": "user", "content": "Please add two Harvest Burgers owner@example.com; email guest@example.com or call +54 9 11 5555 1234."}},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var direct struct {
		ID         uint                         `json:"id"`
		Role       string                       `json:"role"`
		Parts      []map[string]json.RawMessage `json:"parts"`
		ResponseV2 assistantcontract.Response   `json:"response_v2"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &direct))
	require.NoError(t, assistantcontract.Validate(direct.ResponseV2))
	assert.NotContains(t, w.Body.String(), "provider-call")
	assert.NotContains(t, w.Body.String(), "Done! I added it.")
	assert.Equal(t, "model", direct.Role)
	assert.NotZero(t, direct.ID)
	assert.Contains(t, direct.ResponseV2.Answer.Content, "Harvest Burger [redacted-email]")
	require.Len(t, direct.ResponseV2.Actions, 1)
	action := direct.ResponseV2.Actions[0]
	assert.Equal(t, "add_cart_item", action.Type)
	assert.Equal(t, "burger-stable-id", action.Target.ID)
	require.NotNil(t, action.Target.Quantity)
	assert.Equal(t, 2, *action.Target.Quantity)
	require.NotNil(t, action.Target.Notes)
	assert.Equal(t, "email [redacted-email] or call [redacted-phone]", *action.Target.Notes)
	assert.NotContains(t, w.Body.String(), "guest@example.com")
	assert.NotContains(t, w.Body.String(), "owner@example.com")
	assert.NotContains(t, w.Body.String(), "+54 9 11 5555 1234")
	require.Len(t, direct.Parts, 2)
	var legacyText string
	require.NoError(t, json.Unmarshal(direct.Parts[0]["text"], &legacyText))
	assert.Equal(t, direct.ResponseV2.Answer.Content, legacyText)
	var legacyCall struct {
		Name string         `json:"name"`
		Args map[string]any `json:"args"`
	}
	require.NoError(t, json.Unmarshal(direct.Parts[1]["functionCall"], &legacyCall))
	assert.Equal(t, "add_to_cart", legacyCall.Name)
	assert.Equal(t, "Harvest Burger [redacted-email]", legacyCall.Args["item_name"])
	assert.Equal(t, action.Target.ID, legacyCall.Args["menu_item_id"])
	assert.Equal(t, float64(*action.Target.Quantity), legacyCall.Args["quantity"])
	assert.Equal(t, *action.Target.Notes, legacyCall.Args["notes"])

	var stored database.AiWaiterMessage
	require.NoError(t, db.Where("id = ?", direct.ID).First(&stored).Error)
	assert.Equal(t, direct.ResponseV2.Answer.Content, stored.Content)
	assert.NotContains(t, stored.ToolCalls, "provider-call")
	assert.NotContains(t, stored.Content, "Done! I added it.")
	require.NotEqual(t, "{}", stored.StructuredResponse)
	var persisted assistantcontract.Response
	require.NoError(t, json.Unmarshal([]byte(stored.StructuredResponse), &persisted))
	assert.Equal(t, direct.ResponseV2, persisted)

	select {
	case event := <-events:
		assert.Equal(t, direct.ID, event.ID)
		require.NotNil(t, event.ResponseV2)
		assert.Equal(t, direct.ResponseV2, *event.ResponseV2)
		assert.NotContains(t, event.Content, "guest@example.com")
	case <-time.After(time.Second):
		t.Fatal("assistant V2 must be published to the session SSE hub")
	}

	historyRouter := gin.New()
	historyRouter.GET("/ai-waiter/:businessId/messages", GetAiWaiterMessages)
	historyResponse := performAIWaiterRequest(t, historyRouter, http.MethodGet,
		fmt.Sprintf("/ai-waiter/%d/messages?session_token=%s&mode=ordering&table_code=%s", business.ID, conv.SessionID, table.TableCode), nil)
	require.Equal(t, http.StatusOK, historyResponse.Code, historyResponse.Body.String())
	var restored []struct {
		ID         uint                        `json:"id"`
		ResponseV2 *assistantcontract.Response `json:"response_v2"`
	}
	require.NoError(t, json.Unmarshal(historyResponse.Body.Bytes(), &restored))
	var restoredAssistant *assistantcontract.Response
	for _, message := range restored {
		if message.ID == direct.ID {
			restoredAssistant = message.ResponseV2
		}
	}
	require.NotNil(t, restoredAssistant)
	assert.Equal(t, direct.ResponseV2, *restoredAssistant)
}

func TestAIWaiterDirectV2PreservesAllergenNoticeAndStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "task5-allergen-v2", true)
	table := createAIWaiterTable(t, business.ID, "V2-ALLERGEN")
	require.NoError(t, db.AutoMigrate(&database.Menu{}))
	menuJSON, err := json.Marshal([]database.MenuCategory{{
		ID: "mains", Name: "Mains", Items: []database.MenuItem{{
			ID: "burger-allergen-id", Name: "Harvest Burger", Price: 14, Currency: "USD",
			IsAvailable: true, Allergens: []string{"peanuts"},
		}},
	}})
	require.NoError(t, err)
	require.NoError(t, db.Create(&database.Menu{
		BusinessID: business.ID, Categories: string(menuJSON), IsActive: true, Version: 1,
	}).Error)
	conv := createAIWaiterConversation(t, business.ID, "task5-allergen-v2-token")
	conv.TableCode = table.TableCode
	require.NoError(t, db.Save(conv).Error)

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": conv.SessionID, "mode": "ordering", "table_code": table.TableCode, "language": "en",
		"history": []map[string]any{{"role": "user", "content": "Does the Harvest Burger contain peanuts?"}},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var direct struct {
		ID         uint                       `json:"id"`
		ResponseV2 assistantcontract.Response `json:"response_v2"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &direct))
	require.NoError(t, assistantcontract.Validate(direct.ResponseV2))
	assert.NotEmpty(t, direct.ResponseV2.Notices)
	assert.NotEmpty(t, direct.ResponseV2.Status)

	var stored database.AiWaiterMessage
	require.NoError(t, db.First(&stored, direct.ID).Error)
	var persisted assistantcontract.Response
	require.NoError(t, json.Unmarshal([]byte(stored.StructuredResponse), &persisted))
	assert.Equal(t, direct.ResponseV2.Notices, persisted.Notices)
	assert.Equal(t, direct.ResponseV2.Status, persisted.Status)
}

func TestAIWaiterHistoryRestoresV2AndSafelyProjectsLegacyOrMalformedRows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "task5-history-v2", true)
	createAIWaiterTable(t, business.ID, "A1")
	conv := createAIWaiterConversation(t, business.ID, "task5-history-v2-token")

	legacyID, _, err := database.SaveAiWaiterMessageReturningID(conv.ID, "assistant", "Legacy [unsafe](javascript:alert(1))", `[{"name":"raw_model_call"}]`)
	require.NoError(t, err)
	valid := assistantcontract.NewResponse("waiter-persisted-response", "Please confirm allergens with staff.")
	valid.Answer.Format = assistantcontract.FormatPlainText
	valid.Notices = []assistantcontract.Notice{{ID: "allergen_notice", Kind: "safety", Message: "Please confirm allergens with staff."}}
	require.NoError(t, assistantcontract.Validate(valid))
	validJSON, err := json.Marshal(valid)
	require.NoError(t, err)
	validID, _, err := database.SaveAiWaiterMessageReturningIDV2(conv.ID, "assistant", valid.Answer.Content, "", string(validJSON))
	require.NoError(t, err)
	malformed := database.AiWaiterMessage{
		ConversationID: conv.ID, Role: "assistant", Content: "Safe readable fallback",
		StructuredResponse: `{"version":2,"actions":[{"href":"javascript:alert(1)"}]`, CreatedAt: time.Now(),
	}
	require.NoError(t, db.Create(&malformed).Error)

	router := gin.New()
	router.GET("/ai-waiter/:businessId/messages", GetAiWaiterMessages)
	w := performAIWaiterRequest(t, router, http.MethodGet,
		fmt.Sprintf("/ai-waiter/%d/messages?session_token=%s&mode=ordering&table_code=A1", business.ID, conv.SessionID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "raw_model_call")
	assert.NotContains(t, w.Body.String(), "javascript:alert(1)\"}]")

	var history []struct {
		ID         uint                        `json:"id"`
		Role       string                      `json:"role"`
		Content    string                      `json:"content"`
		ResponseV2 *assistantcontract.Response `json:"response_v2"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &history))
	require.Len(t, history, 3)

	assert.Equal(t, legacyID, history[0].ID)
	require.NotNil(t, history[0].ResponseV2)
	require.NoError(t, assistantcontract.Validate(*history[0].ResponseV2))
	assert.Equal(t, assistantcontract.FormatPlainText, history[0].ResponseV2.Answer.Format)
	assert.Equal(t, history[0].Content, history[0].ResponseV2.Answer.Content)
	assert.Empty(t, history[0].ResponseV2.Actions)

	assert.Equal(t, validID, history[1].ID)
	require.NotNil(t, history[1].ResponseV2)
	assert.Equal(t, valid, *history[1].ResponseV2)
	assert.Equal(t, assistantcontract.StatusComplete, history[1].ResponseV2.Status)
	require.Len(t, history[1].ResponseV2.Notices, 1)

	assert.Equal(t, malformed.ID, history[2].ID)
	require.NotNil(t, history[2].ResponseV2)
	require.NoError(t, assistantcontract.Validate(*history[2].ResponseV2))
	assert.Equal(t, assistantcontract.StatusDegraded, history[2].ResponseV2.Status)
	assert.Equal(t, assistantcontract.FormatPlainText, history[2].ResponseV2.Answer.Format)
	assert.Equal(t, "Safe readable fallback", history[2].ResponseV2.Answer.Content)
	assert.Empty(t, history[2].ResponseV2.Actions)
}

// seedAIWaiterChatBusiness builds the minimal fixture an ordering-mode
// HandleAIWaiter call needs: an AI-Pro business with AI enabled, an active
// (empty) menu, a table whose code matches the conversation, and a persisted
// conversation. Returns the business and the table code.
func seedAIWaiterChatBusiness(t testing.TB, db *gorm.DB, suffix, sessionToken string) (*database.Business, string) {
	t.Helper()
	business := createAIWaiterBusiness(t, suffix, true)
	require.NoError(t, db.AutoMigrate(&database.Menu{}, &database.Offer{}, &database.Bundle{}))
	require.NoError(t, db.Create(&database.Menu{BusinessID: business.ID, Categories: "[]", IsActive: true, Version: 1}).Error)
	// createAIWaiterConversation hardcodes TableCode="A1", Mode="ordering".
	createAIWaiterTable(t, business.ID, "A1")
	createAIWaiterConversation(t, business.ID, sessionToken)
	return business, "A1"
}

func TestHandleAIWaiter_FetchesTableOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	business, tableCode := seedAIWaiterChatBusiness(t, db, "fetch-once", "fetch-once-token")

	svc, _ := services.NewAIService(okProvider{}, llm.ModelConfig{Chat: "m", Image: "i", Director: "d", Menu: "mn"})
	SetAIService(svc)
	t.Cleanup(func() { SetAIService(nil) })

	var tableQueries int
	const cbName = "count_tables"
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(cbName, func(g *gorm.DB) {
		if g.Statement != nil && g.Statement.Table == "tables" {
			tableQueries++
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove(cbName) })

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": "fetch-once-token", "mode": "ordering", "table_code": tableCode, "language": "en",
		"history": []map[string]any{{"role": "user", "content": "Can I add another coffee?"}},
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.LessOrEqual(t, tableQueries, 1, "ordering chat path must resolve the table at most once (got %d)", tableQueries)
}

func BenchmarkHandleAIWaiterTableFetch(b *testing.B) {
	gin.SetMode(gin.TestMode)
	// Silence GORM logging (the no-open-bill path logs a "no such table: bills"
	// warning per call) so the benchmark output stays clean and timing is not
	// polluted by stderr formatting.
	db := setupAIWaiterTestDBWithLogger(b, logger.Default.LogMode(logger.Silent))
	business, tableCode := seedAIWaiterChatBusiness(b, db, "bench-table", "bench-table-token")

	svc, _ := services.NewAIService(okProvider{}, llm.ModelConfig{Chat: "m", Image: "i", Director: "d", Menu: "mn"})
	SetAIService(svc)
	b.Cleanup(func() { SetAIService(nil) })

	// Bump the daily budget out of the way so every iteration reaches the AI path.
	b.Setenv("AI_WAITER_DAILY_MESSAGE_BUDGET", "100000000")
	// Same for the per-network daily cap (every httptest request shares one
	// RemoteAddr); the quota check itself still runs on every iteration.
	b.Setenv(envAIWaiterDailyPerIP, "100000000")

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	path := fmt.Sprintf("/ai-waiter/%d", business.ID)
	body := map[string]any{
		"session_token": "bench-table-token", "mode": "ordering", "table_code": tableCode, "language": "en",
		"history": []map[string]any{{"role": "user", "content": "Can I add another coffee?"}},
	}
	// Each call persists a user + assistant turn; the per-session cap
	// (maxAIWaiterMessagesPerSession) would start returning 429 after a few dozen
	// iterations and skew the measurement. Truncate the transcript before the cap,
	// with the timer paused so the cleanup is not measured.
	clearMessages := func() {
		require.NoError(b, db.Exec("DELETE FROM ai_waiter_messages").Error)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%(maxAIWaiterMessagesPerSession/2) == 0 {
			b.StopTimer()
			clearMessages()
			b.StartTimer()
		}
		w := performAIWaiterRequest(b, router, http.MethodPost, path, body)
		if w.Code != http.StatusOK {
			b.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
	}
}
