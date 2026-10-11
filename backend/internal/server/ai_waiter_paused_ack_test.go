package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHandleAIWaiter_PausedReturnsHumanAckNotLastAssistant pins the staff-takeover
// (paused) behavior: while a human holds the conversation, a guest POST must NOT
// receive an echo of the latest assistant DB row (which the client appended as a
// duplicate bubble and replayed pre-pause AI answers). It must return a localized
// "a human is assisting you" acknowledgement flagged human_ack:true so the guest
// client renders it as an ephemeral notice instead of a chat bubble. (R3-AI-1/R3-AI-4)
func TestHandleAIWaiter_PausedReturnsHumanAckNotLastAssistant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupAIWaiterTestDB(t)

	business := createAIWaiterBusiness(t, "paused-ack", true)
	table := createAIWaiterTable(t, business.ID, "PAUSE-TBL-1")

	require.NoError(t, db.AutoMigrate(&database.Menu{}))
	require.NoError(t, db.Create(&database.Menu{
		BusinessID: business.ID,
		Categories: "[]",
		IsActive:   true,
		Version:    1,
	}).Error)

	// A paused conversation with an actively-held claim and a prior assistant row.
	const sessionID = "session-paused-ack"
	conv := createAIWaiterConversation(t, business.ID, sessionID)
	conv.TableCode = table.TableCode
	conv.IsPaused = true
	claimedAt := time.Now()
	conv.ClaimedAt = &claimedAt
	require.NoError(t, db.Save(conv).Error)

	// Seed a prior assistant reply that MUST NOT be echoed back to the guest.
	const priorAssistant = "SECRET_PRE_PAUSE_ANSWER"
	require.NoError(t, database.SaveAiWaiterMessage(conv.ID, "assistant", priorAssistant, ""))

	router := gin.New()
	router.POST("/ai-waiter/:businessId", HandleAIWaiter)

	w := performAIWaiterRequest(t, router, http.MethodPost, fmt.Sprintf("/ai-waiter/%d", business.ID), map[string]any{
		"session_token": sessionID,
		"mode":          "ordering",
		"table_code":    table.TableCode,
		"language":      "es",
		"history": []map[string]any{
			{"role": "user", "content": "hola, sigo esperando"},
		},
	})

	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var resp struct {
		Role     string `json:"role"`
		HumanAck bool   `json:"human_ack"`
		IsPaused bool   `json:"is_paused"`
		Parts    []struct {
			Text string `json:"text"`
		} `json:"parts"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, "model", resp.Role)
	assert.True(t, resp.HumanAck, "paused response must carry human_ack so the client dedupes vs the SSE staff reply")
	assert.True(t, resp.IsPaused)
	require.Len(t, resp.Parts, 1)

	// The ack must NOT be the prior assistant content.
	assert.NotEqual(t, priorAssistant, resp.Parts[0].Text)
	assert.NotContains(t, resp.Parts[0].Text, priorAssistant)
	// It must be the localized ack for the requested guest locale (es).
	assert.NotEmpty(t, resp.Parts[0].Text)
	assert.Contains(t, resp.Parts[0].Text, "equipo", "expected the Spanish human-assisting ack")
}
