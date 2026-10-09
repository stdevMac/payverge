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

	"github.com/stdevmac/payverge/backend/internal/aiwaiterevents"
	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubWaiterProvider is a canned llm.Provider used to drive the AI waiter
// handler's response serialization without a live model.
type stubWaiterProvider struct {
	resp *llm.Response
}

func (s *stubWaiterProvider) Generate(_ context.Context, _ llm.GenerateRequest) (*llm.Response, error) {
	return s.resp, nil
}

// TestHandleAIWaiter_ResponseWireShape pins the public POST /ai-waiter/:businessId
// response to the legacy {role:"model", parts:[{text}|{functionCall:{name,args}}]}
// shape the guest AiWaiter.tsx parses. This is the contract that must survive
// the Gemini->OpenRouter migration with no frontend changes.
func TestHandleAIWaiter_ResponseWireShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	var telemetry []llm.AITelemetryEvent
	llm.SetTelemetrySink(func(event llm.AITelemetryEvent) { telemetry = append(telemetry, event) })
	t.Cleanup(func() { llm.SetTelemetrySink(nil) })

	business := createAIWaiterBusiness(t, "wire-shape", true)
	table := createAIWaiterTable(t, business.ID, "WIRE-TBL-1")

	// The handler requires an active menu before it reaches the AI call.
	// setupAIWaiterTestDB does not migrate the menus table, so do it here and
	// seed a minimal active menu that includes the item the model will cart.
	// Cart tool calls are validated against this menu before wire/persistence.
	require.NoError(t, db.AutoMigrate(&database.Menu{}))
	wireCats, err := json.Marshal([]database.MenuCategory{{
		ID:   "mains",
		Name: "Mains",
		Items: []database.MenuItem{{
			ID:          "burger-1",
			Name:        "Burger",
			Price:       10.0,
			Currency:    "USD",
			IsAvailable: true,
		}},
	}})
	require.NoError(t, err)
	require.NoError(t, db.Create(&database.Menu{
		BusinessID: business.ID,
		Categories: string(wireCats),
		IsActive:   true,
		Version:    1,
	}).Error)

	stub := &stubWaiterProvider{resp: &llm.Response{
		Text: "Sure, adding that now!",
		ToolCalls: []llm.ToolCall{{
			ID:   "call_1",
			Name: "add_to_cart",
			Args: map[string]any{"item_name": "Burger", "quantity": float64(2)},
		}},
	}}
	aiSvc, err := services.NewAIService(stub, llm.ModelConfig{Chat: "test-chat", Image: "test-image", Director: "test-director", Menu: "test-menu"})
	require.NoError(t, err)
	SetAIService(aiSvc)
	t.Cleanup(func() { SetAIService(nil) })

	// Create a known conversation so the session gate passes.
	conv := createAIWaiterConversation(t, business.ID, "session-wire-shape")
	conv.TableCode = table.TableCode
	require.NoError(t, db.Save(conv).Error)
	events, cancel, err := aiwaiterevents.GetHub().Subscribe(conv.ID)
	require.NoError(t, err)
	defer cancel()

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	router.GET("/ai-waiter/:businessId/messages", GetAiWaiterMessages)

	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": "session-wire-shape",
		"mode":          "ordering",
		"table_code":    table.TableCode,
		"history": []map[string]any{
			{"role": "user", "content": "add two burgers"},
		},
	})

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	require.Len(t, telemetry, 1)
	require.Equal(t, "waiter", telemetry[0].Surface)
	require.Equal(t, "v2", telemetry[0].ContractVersion, "waiter always serves the V2 contract")
	require.Equal(t, "ok", telemetry[0].Outcome)
	require.Equal(t, "offered", telemetry[0].ActionOutcome)
	require.Equal(t, "en", telemetry[0].Language)
	require.Equal(t, "verified", telemetry[0].SchemaOutcome)
	require.True(t, telemetry[0].V2ShadowValid, "disabled wire still retains a validated V2 shadow")
	require.Equal(t, "verified", telemetry[0].SourceOutcome)
	require.Equal(t, "verified", telemetry[0].EntityOutcome)
	require.Equal(t, "none", telemetry[0].LanguageOutcome, "short response text is insufficient language evidence")
	require.Positive(t, telemetry[0].TimeToFirstMs, "validated response availability must populate TTFC")
	require.LessOrEqual(t, telemetry[0].TimeToFirstMs, telemetry[0].LatencyMs)
	var resp struct {
		Role  string `json:"role"`
		Parts []struct {
			Text         string `json:"text"`
			FunctionCall *struct {
				Name string         `json:"name"`
				Args map[string]any `json:"args"`
			} `json:"functionCall"`
		} `json:"parts"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	var direct map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &direct))
	require.Contains(t, direct, "response_v2", "waiter always exposes the V2 wire shape")

	assert.Equal(t, "model", resp.Role)
	require.Len(t, resp.Parts, 2)

	// First part: assistant text.
	assert.Equal(t, "I can help add Burger.", resp.Parts[0].Text)
	assert.NotContains(t, strings.ToLower(resp.Parts[0].Text), "adding that now")
	assert.Nil(t, resp.Parts[0].FunctionCall)

	// Second part: the tool call in {name,args} shape the guest AiWaiter parses.
	require.NotNil(t, resp.Parts[1].FunctionCall)
	assert.Equal(t, "add_to_cart", resp.Parts[1].FunctionCall.Name)
	assert.Equal(t, "Burger", resp.Parts[1].FunctionCall.Args["item_name"])
	assert.Equal(t, "menu_item", resp.Parts[1].FunctionCall.Args["item_type"])
	assert.Equal(t, "burger-1", resp.Parts[1].FunctionCall.Args["menu_item_id"])
	assert.Equal(t, float64(2), resp.Parts[1].FunctionCall.Args["quantity"])

	var stored database.AiWaiterMessage
	require.NoError(t, db.Where("conversation_id = ? AND role = ?", conv.ID, "assistant").First(&stored).Error)
	require.NotEmpty(t, stored.StructuredResponse, "the validated V2 envelope must be durably persisted")
	var persisted assistantcontract.Response
	require.NoError(t, json.Unmarshal([]byte(stored.StructuredResponse), &persisted))
	require.NoError(t, assistantcontract.Validate(persisted))
	assert.Equal(t, resp.Parts[0].Text, persisted.Answer.Content)

	deadline := time.After(time.Second)
	for waiting := true; waiting; {
		select {
		case event := <-events:
			if event.Role != "assistant" {
				continue
			}
			require.NotNil(t, event.ResponseV2, "waiter always publishes V2 over SSE")
			assert.Equal(t, persisted.Answer.Content, event.Content)
			waiting = false
		case <-deadline:
			t.Fatal("expected the persisted assistant message to publish over SSE")
		}
	}
	require.NoError(t, db.Create(&database.AiWaiterMessage{
		ConversationID:     conv.ID,
		Role:               "assistant",
		Content:            "Email guest@example.com",
		StructuredResponse: `{"version":2`,
		CreatedAt:          time.Now().Add(time.Second),
	}).Error)

	history := performAIWaiterRequest(t, router, http.MethodGet,
		fmt.Sprintf("/ai-waiter/%d/messages?session_token=%s&mode=ordering&table_code=%s&language=en", business.ID, conv.SessionID, table.TableCode), nil)
	require.Equal(t, http.StatusOK, history.Code, history.Body.String())
	var historyMessages []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(history.Body.Bytes(), &historyMessages))
	require.NotEmpty(t, historyMessages)
	assert.NotContains(t, history.Body.String(), "guest@example.com")
	assert.Contains(t, history.Body.String(), "[redacted-email]")
	var assistantHistoryCount int
	for _, message := range historyMessages {
		var role string
		require.NoError(t, json.Unmarshal(message["role"], &role))
		if role != "model" && role != "assistant" {
			require.NotContains(t, message, "response_v2", "guest messages never carry assistant payloads")
			continue
		}
		assistantHistoryCount++
		require.Contains(t, message, "response_v2", "assistant history always carries V2")
	}
	require.Positive(t, assistantHistoryCount, "history must include assistant messages")
}

func TestHandleAIWaiter_RequestBoundaryTelemetryCoversMalformedAndInvalidBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "telemetry-boundary", true)
	var telemetry []llm.AITelemetryEvent
	llm.SetTelemetrySink(func(event llm.AITelemetryEvent) { telemetry = append(telemetry, event) })
	t.Cleanup(func() { llm.SetTelemetrySink(nil) })

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	malformed := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), strings.NewReader(`{"history":`))
	malformed.Header.Set("Content-Type", "application/json")
	malformedResponse := httptest.NewRecorder()
	router.ServeHTTP(malformedResponse, malformed)

	require.Equal(t, http.StatusBadRequest, malformedResponse.Code)
	require.Len(t, telemetry, 1)
	assert.Equal(t, "invalid", telemetry[0].Outcome)
	assert.Equal(t, business.ID, telemetry[0].BusinessID)
	assert.False(t, telemetry[0].V2ShadowValid)

	unknown := httptest.NewRequest(http.MethodPost, "/ai-waiter/not-a-business", strings.NewReader(`{"history":[]}`))
	unknown.Header.Set("Content-Type", "application/json")
	unknownResponse := httptest.NewRecorder()
	router.ServeHTTP(unknownResponse, unknown)

	require.Equal(t, http.StatusNotFound, unknownResponse.Code)
	require.Len(t, telemetry, 2)
	assert.Equal(t, "invalid", telemetry[1].Outcome)
	assert.Zero(t, telemetry[1].BusinessID)
	assert.False(t, telemetry[1].V2ShadowValid)
}

func TestApplyAIWaiterResponseTelemetryVerifiesValidatedRetainedObjects(t *testing.T) {
	event := llm.AITelemetryEvent{
		ActionOutcome: "none", SourceOutcome: "none", EntityOutcome: "none",
		SchemaOutcome: "none", LanguageOutcome: "none",
	}
	response := assistantcontract.NewResponse("waiter-decision-provenance", "Here is an option.")
	response.Actions = []assistantcontract.Action{{
		ID: "open-menu", Type: "navigate", Label: "Open menu",
		Target: assistantcontract.ActionTarget{Kind: "payverge_page", ID: "menu", Href: "/menu"},
		State:  "ready", Confirmation: "none",
	}}
	response.Sources = []assistantcontract.Source{{
		ID: "menu-source", Type: "menu_item", Title: "Soup",
		Origin: "business_menu", RetrievedAt: "2026-08-08T12:00:00Z",
	}}
	response.Entities = []assistantcontract.Entity{{
		ID: "menu_item:soup", Type: "menu_item", DisplayName: "Soup",
		Availability: "available", SourceID: "menu-source",
	}}

	applyAIWaiterResponseTelemetry(&event, response, "none")

	assert.Equal(t, "offered", event.ActionOutcome)
	assert.Equal(t, "verified", event.SourceOutcome)
	assert.Equal(t, "verified", event.EntityOutcome)
}

// TestHandleAIWaiter_InvalidCartCallNeverSerializedOrPersisted ensures hallucinated
// add_to_cart names are stripped from the guest wire payload AND the saved assistant
// ToolCalls JSON before any representation leaves the handler.
func TestHandleAIWaiter_InvalidCartCallNeverSerializedOrPersisted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	var telemetry []llm.AITelemetryEvent
	llm.SetTelemetrySink(func(event llm.AITelemetryEvent) { telemetry = append(telemetry, event) })
	t.Cleanup(func() { llm.SetTelemetrySink(nil) })

	business := createAIWaiterBusiness(t, "invalid-cart", true)
	table := createAIWaiterTable(t, business.ID, "INV-CART-1")

	require.NoError(t, db.AutoMigrate(&database.Menu{}))
	cats := []database.MenuCategory{{
		ID:   "mains",
		Name: "Mains",
		Items: []database.MenuItem{{
			ID:          "burger-1",
			Name:        "House Burger",
			Price:       12.0,
			Currency:    "USD",
			IsAvailable: true,
		}},
	}}
	raw, err := json.Marshal(cats)
	require.NoError(t, err)
	require.NoError(t, db.Create(&database.Menu{
		BusinessID: business.ID,
		Categories: string(raw),
		IsActive:   true,
		Version:    1,
	}).Error)

	stub := &stubWaiterProvider{resp: &llm.Response{
		Text: "Adding those for you!",
		ToolCalls: []llm.ToolCall{
			{
				ID:   "call_bad",
				Name: "add_to_cart",
				Args: map[string]any{"item_name": "Unicorn Steak", "quantity": float64(1)},
			},
			{
				ID:   "call_good",
				Name: "add_to_cart",
				Args: map[string]any{"item_name": "House Burger", "quantity": float64(1)},
			},
		},
	}}
	aiSvc, err := services.NewAIService(stub, llm.ModelConfig{Chat: "test-chat", Image: "test-image", Director: "test-director", Menu: "test-menu"})
	require.NoError(t, err)
	SetAIService(aiSvc)
	t.Cleanup(func() { SetAIService(nil) })

	conv := createAIWaiterConversation(t, business.ID, "session-invalid-cart")
	conv.TableCode = table.TableCode
	require.NoError(t, db.Save(conv).Error)

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)

	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": "session-invalid-cart",
		"mode":          "ordering",
		"table_code":    table.TableCode,
		"history": []map[string]any{
			{"role": "user", "content": "add unicorn steak and house burger"},
		},
	})

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	require.NotContains(t, w.Body.String(), "Unicorn Steak")
	require.Contains(t, w.Body.String(), "House Burger")

	var saved database.AiWaiterMessage
	require.NoError(t, db.Where("conversation_id = ? AND role = ?", conv.ID, "assistant").First(&saved).Error)
	require.NotContains(t, saved.ToolCalls, "Unicorn Steak")
	require.Contains(t, saved.ToolCalls, "House Burger")
	require.Len(t, telemetry, 1)
	assert.Equal(t, "rejected", telemetry[0].ActionOutcome, "explicit rejected tool provenance wins over retained actions")
}

// TestHandleAIWaiter_TextOnlyResponse covers the dominant runtime case: the model
// returns plain text with no tool calls. The response must still carry the legacy
// {role:"model", parts:[{text}]} shape with exactly one part and no functionCall,
// and no add_to_cart must be counted on the conversation row.
func TestHandleAIWaiter_TextOnlyResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)
	var telemetry []llm.AITelemetryEvent
	llm.SetTelemetrySink(func(event llm.AITelemetryEvent) { telemetry = append(telemetry, event) })
	t.Cleanup(func() { llm.SetTelemetrySink(nil) })

	business := createAIWaiterBusiness(t, "text-only", true)
	table := createAIWaiterTable(t, business.ID, "WIRE-TBL-2")

	// The handler requires an active menu before it reaches the AI call.
	require.NoError(t, db.AutoMigrate(&database.Menu{}))
	require.NoError(t, db.Create(&database.Menu{
		BusinessID: business.ID,
		Categories: "[]",
		IsActive:   true,
		Version:    1,
	}).Error)

	stub := &stubWaiterProvider{resp: &llm.Response{
		Text: "Here are our specials today.",
	}}
	aiSvc, err := services.NewAIService(stub, llm.ModelConfig{Chat: "test-chat", Image: "test-image", Director: "test-director", Menu: "test-menu"})
	require.NoError(t, err)
	SetAIService(aiSvc)
	t.Cleanup(func() { SetAIService(nil) })

	// Create a known conversation so the session gate passes.
	sessionConv := createAIWaiterConversation(t, business.ID, "session-text-only")
	sessionConv.TableCode = table.TableCode
	require.NoError(t, db.Save(sessionConv).Error)

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)

	const sessionID = "session-text-only"
	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": sessionID,
		"mode":          "ordering",
		"table_code":    table.TableCode,
		"history": []map[string]any{
			{"role": "user", "content": "what are the specials?"},
		},
	})

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	var enabledWire map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &enabledWire))
	require.Contains(t, enabledWire, "response_v2", "enabled rollout must preserve the dual V1+V2 wire")

	var resp struct {
		Role  string `json:"role"`
		Parts []struct {
			Text         string `json:"text"`
			FunctionCall *struct {
				Name string         `json:"name"`
				Args map[string]any `json:"args"`
			} `json:"functionCall"`
		} `json:"parts"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, "model", resp.Role)
	require.Len(t, resp.Parts, 1)
	assert.Equal(t, "I can help with questions about the menu and your visit.", resp.Parts[0].Text)
	assert.NotEqual(t, "Here are our specials today.", resp.Parts[0].Text)
	assert.Nil(t, resp.Parts[0].FunctionCall)

	// No tool call was returned, so no add_to_cart must have been counted.
	// CartItemsAdded is a plain int column on AiWaiterConversation, queryable by
	// session_id, so read it back directly — this proves no tool call was persisted.
	var conv database.AiWaiterConversation
	require.NoError(t, db.Where("session_id = ?", sessionID).First(&conv).Error)
	assert.Equal(t, 0, conv.CartItemsAdded)
	require.Len(t, telemetry, 1)
	assert.Equal(t, "v2", telemetry[0].ContractVersion, "enabled rollout serves the V2 contract")
	assert.True(t, telemetry[0].V2ShadowValid)
	assert.Positive(t, telemetry[0].TimeToFirstMs)
}

// TestHandleAIWaiter_StripsCartToolsWhenOrderingDisabled ensures Closed Mode /
// kitchen-off never wires add_to_cart to the guest (server defense-in-depth).
func TestHandleAIWaiter_StripsCartToolsWhenOrderingDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)

	business := createAIWaiterBusiness(t, "kitchen-off", true)
	business.KitchenEnabled = false
	business.OrdersEnabled = false
	require.NoError(t, db.Save(business).Error)
	table := createAIWaiterTable(t, business.ID, "KIT-OFF-1")

	require.NoError(t, db.AutoMigrate(&database.Menu{}))
	wireCats, err := json.Marshal([]database.MenuCategory{{
		ID: "mains", Name: "Mains",
		Items: []database.MenuItem{{ID: "burger-1", Name: "Burger", Price: 10.0, Currency: "USD", IsAvailable: true}},
	}})
	require.NoError(t, err)
	require.NoError(t, db.Create(&database.Menu{BusinessID: business.ID, Categories: string(wireCats), IsActive: true, Version: 1}).Error)

	stub := &stubWaiterProvider{resp: &llm.Response{
		Text: "",
		ToolCalls: []llm.ToolCall{{
			ID: "call_1", Name: "add_to_cart",
			Args: map[string]any{"item_name": "Burger", "quantity": float64(1)},
		}},
	}}
	aiSvc, err := services.NewAIService(stub, llm.ModelConfig{Chat: "test-chat", Image: "test-image", Director: "test-director", Menu: "test-menu"})
	require.NoError(t, err)
	SetAIService(aiSvc)
	t.Cleanup(func() { SetAIService(nil) })

	conv := createAIWaiterConversation(t, business.ID, "session-kitchen-off")
	conv.TableCode = table.TableCode
	require.NoError(t, db.Save(conv).Error)

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)
	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": "session-kitchen-off",
		"mode":          "ordering",
		"table_code":    table.TableCode,
		"history":       []map[string]any{{"role": "user", "content": "add a burger"}},
	})
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
	body := w.Body.String()
	assert.NotContains(t, body, "add_to_cart")
	assert.NotContains(t, body, "functionCall")
	assert.Contains(t, strings.ToLower(body), "paused")
}
