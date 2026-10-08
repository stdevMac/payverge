package server

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/aiwaiterevents"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAIWaiterStream_RejectsFifthLiveConnection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "stream-cap", true)
	conv := createAIWaiterConversation(t, business.ID, "tok-stream-cap")

	hub := aiwaiterevents.NewHubWithLimits(4, 10, 100)
	restore := aiwaiterevents.SetHubForTest(hub)
	t.Cleanup(restore)
	for i := 0; i < 4; i++ {
		_, cancel, ok := hub.SubscribeLimited(conv.ID, "")
		require.True(t, ok)
		t.Cleanup(cancel)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := streamRequest(t, strconv.Itoa(int(business.ID)), conv.SessionID, ctx)
	assert.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"code":"sse_connection_limit"`)
	assert.Contains(t, w.Body.String(), "Too many live connections")
}

func TestAIWaiterStream_KeepaliveEndsWhenSessionExpires(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "stream-expire-tick", true)
	conv := createAIWaiterConversation(t, business.ID, "tok-stream-expire-tick")

	prev := aiWaiterStreamKeepalive
	aiWaiterStreamKeepalive = 20 * time.Millisecond
	t.Cleanup(func() { aiWaiterStreamKeepalive = prev })

	require.NoError(t, database.GetDB().Model(&database.AiWaiterConversation{}).
		Where("id = ?", conv.ID).
		Update("created_at", time.Now().Add(-aiWaiterSessionTTL+50*time.Millisecond)).Error)
	require.NoError(t, database.GetDB().First(conv, conv.ID).Error)
	require.False(t, aiWaiterSessionExpired(conv), "session must still be valid at connect")

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	w := streamRequest(t, strconv.Itoa(int(business.ID)), conv.SessionID, ctx)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := w.Body.String()
	assert.Contains(t, body, "event: error")
	assert.Contains(t, body, "session_expired")
}

func TestAIWaiterStreamStillAllowed_AIDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "stream-ai-off", true)
	conv := createAIWaiterConversation(t, business.ID, "tok-stream-ai-off")

	code, ok := aiWaiterStreamStillAllowed(conv, business.ID, true)
	require.True(t, ok, code)
	assert.Empty(t, code)

	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		Updates(map[string]any{"ai_ai_enabled": false}).Error)

	code, ok = aiWaiterStreamStillAllowed(conv, business.ID, true)
	assert.False(t, ok)
	assert.Equal(t, "ai_disabled", code)
}

func TestAIWaiterStream_KeepaliveEndsWhenAIDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAIWaiterTestDB(t)
	business := createAIWaiterBusiness(t, "stream-ai-off-tick", true)
	conv := createAIWaiterConversation(t, business.ID, "tok-stream-ai-off-tick")

	prevKeepalive, prevRecheck := aiWaiterStreamKeepalive, aiWaiterStreamBusinessRecheck
	aiWaiterStreamKeepalive = 20 * time.Millisecond
	aiWaiterStreamBusinessRecheck = 0
	t.Cleanup(func() {
		aiWaiterStreamKeepalive = prevKeepalive
		aiWaiterStreamBusinessRecheck = prevRecheck
	})

	go func() {
		time.Sleep(60 * time.Millisecond)
		_ = database.GetDB().Model(&database.Business{}).
			Where("id = ?", business.ID).
			Updates(map[string]any{"ai_ai_enabled": false}).Error
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	w := streamRequest(t, strconv.Itoa(int(business.ID)), conv.SessionID, ctx)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := w.Body.String()
	assert.Contains(t, body, "event: error")
	assert.Contains(t, body, "ai_disabled")
}
