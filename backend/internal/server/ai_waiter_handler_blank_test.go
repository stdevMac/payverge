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

// TestHandleAIWaiter_WhitespaceOnlyResponseIsTreatedAsEmpty pins the
// defense-in-depth guard for a whitespace-only model completion. The guest gets
// bounded server-owned copy instead of a blank retry loop, and raw whitespace
// can never be persisted or replayed.
func TestHandleAIWaiter_WhitespaceOnlyResponseIsTreatedAsEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)

	business := createAIWaiterBusiness(t, "blank-resp", true)
	table := createAIWaiterTable(t, business.ID, "BLANK-TBL-1")

	require.NoError(t, db.AutoMigrate(&database.Menu{}))
	require.NoError(t, db.Create(&database.Menu{
		BusinessID: business.ID,
		Categories: "[]",
		IsActive:   true,
		Version:    1,
	}).Error)

	stub := &stubWaiterProvider{resp: &llm.Response{Text: "\n  \n"}}
	aiSvc, err := services.NewAIService(stub, llm.ModelConfig{Chat: "test-chat", Image: "test-image", Director: "test-director", Menu: "test-menu"})
	require.NoError(t, err)
	SetAIService(aiSvc)
	t.Cleanup(func() { SetAIService(nil) })

	const sessionID = "session-blank-resp"
	conv := createAIWaiterConversation(t, business.ID, sessionID)
	conv.TableCode = table.TableCode
	require.NoError(t, db.Save(conv).Error)

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)

	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": sessionID,
		"mode":          "ordering",
		"table_code":    table.TableCode,
		"history": []map[string]any{
			{"role": "user", "content": "what are the specials?"},
		},
	})

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var resp struct {
		Role  string `json:"role"`
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	require.Len(t, resp.Parts, 1)
	assert.Equal(t, "I can help with questions about the menu and your visit.", resp.Parts[0].Text)
	assert.NotEqual(t, "\n  \n", resp.Parts[0].Text)

	var saved database.AiWaiterMessage
	require.NoError(t, db.Where("conversation_id = ? AND role = ?", conv.ID, "assistant").First(&saved).Error)
	assert.Equal(t, resp.Parts[0].Text, saved.Content)
	assert.Empty(t, saved.ToolCalls)
}
