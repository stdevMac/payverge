package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// L4-7: pause/claim/reply on a CLOSED conversation must be rejected with a
// 409 instead of silently re-arming the row. CloseAiConversation clears the
// claim and pause exactly because "a closed row must not keep a live claim or
// stay paused" — accepting these transitions afterwards produced the audit's
// nonsense "CERRADA + PAUSADO" state.

func closeAIWaiterConversation(t *testing.T, convID uint) {
	t.Helper()
	require.NoError(t, database.GetDB().Model(&database.AiWaiterConversation{}).
		Where("id = ?", convID).
		Update("status", "closed").Error)
}

func decodeErrorCode(t *testing.T, body string) string {
	t.Helper()
	var payload struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &payload), body)
	return payload.Code
}

func TestToggleAiPause_RejectsClosedConversation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "pause-closed", true)
	conv := createAIWaiterConversation(t, biz.ID, "pause-closed-sess")
	mgr := createAIWaiterStaff(t, biz.ID, database.StaffRoleManager, "mgr-pause@ex.com")
	closeAIWaiterConversation(t, conv.ID)

	w := performAIWaiterRequest(t,
		aiStaffRouter(mgr.ID, database.StaffRoleManager, biz.ID),
		http.MethodPost,
		fmt.Sprintf("/businesses/%d/ai/conversations/%d/pause", biz.ID, conv.ID),
		map[string]any{"is_paused": true},
	)

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Equal(t, "conversation_closed", decodeErrorCode(t, w.Body.String()))

	var reloaded database.AiWaiterConversation
	require.NoError(t, database.GetDB().First(&reloaded, conv.ID).Error)
	assert.Equal(t, "closed", reloaded.Status)
	assert.False(t, reloaded.IsPaused, "a closed conversation must never become CERRADA + PAUSADO")
}

func TestClaimAiConversation_RejectsClosedConversation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "claim-closed", true)
	conv := createAIWaiterConversation(t, biz.ID, "claim-closed-sess")
	srv := createAIWaiterStaff(t, biz.ID, database.StaffRoleServer, "srv-claim@ex.com")
	closeAIWaiterConversation(t, conv.ID)

	w := performAIWaiterRequest(t,
		aiStaffRouter(srv.ID, database.StaffRoleServer, biz.ID),
		http.MethodPost, claimPath(biz.ID, conv.ID), nil)

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Equal(t, "conversation_closed", decodeErrorCode(t, w.Body.String()))

	var reloaded database.AiWaiterConversation
	require.NoError(t, database.GetDB().First(&reloaded, conv.ID).Error)
	assert.Nil(t, reloaded.ClaimedByStaffID)
	assert.Nil(t, reloaded.ClaimedAt)
	assert.False(t, reloaded.IsPaused)
}

// Managers may override the claim gate — the closed gate must still win.
func TestClaimAiConversation_RejectsClosedConversationForManager(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "claim-closed-mgr", true)
	conv := createAIWaiterConversation(t, biz.ID, "claim-closed-mgr-sess")
	mgr := createAIWaiterStaff(t, biz.ID, database.StaffRoleManager, "mgr-claim@ex.com")
	closeAIWaiterConversation(t, conv.ID)

	w := performAIWaiterRequest(t,
		aiStaffRouter(mgr.ID, database.StaffRoleManager, biz.ID),
		http.MethodPost, claimPath(biz.ID, conv.ID), nil)

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Equal(t, "conversation_closed", decodeErrorCode(t, w.Body.String()))

	var reloaded database.AiWaiterConversation
	require.NoError(t, database.GetDB().First(&reloaded, conv.ID).Error)
	assert.False(t, reloaded.IsPaused)
}

func TestPostAiReply_RejectsClosedConversation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	biz := createAIWaiterBusiness(t, "reply-closed", true)
	conv := createAIWaiterConversation(t, biz.ID, "reply-closed-sess")
	mgr := createAIWaiterStaff(t, biz.ID, database.StaffRoleManager, "mgr-reply@ex.com")
	closeAIWaiterConversation(t, conv.ID)

	w := performAIWaiterRequest(t,
		aiStaffRouter(mgr.ID, database.StaffRoleManager, biz.ID),
		http.MethodPost,
		fmt.Sprintf("/businesses/%d/ai/conversations/%d/reply", biz.ID, conv.ID),
		map[string]any{"content": "hello?"},
	)

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Equal(t, "conversation_closed", decodeErrorCode(t, w.Body.String()))

	var count int64
	require.NoError(t, database.GetDB().Model(&database.AiWaiterMessage{}).
		Where("conversation_id = ?", conv.ID).Count(&count).Error)
	assert.Zero(t, count, "no message may be persisted into a closed conversation")
}
