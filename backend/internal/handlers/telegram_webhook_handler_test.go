package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/server"
)

type recordingTelegramWebhookProcessor struct {
	calls  int
	update server.TelegramWebhookUpdate
}

func (p *recordingTelegramWebhookProcessor) ProcessUpdate(ctx context.Context, update server.TelegramWebhookUpdate) error {
	p.calls++
	p.update = update
	return nil
}

func TestTelegramWebhook_RejectsMissingSecret(t *testing.T) {
	processor := &recordingTelegramWebhookProcessor{}
	handler := NewTelegramWebhookHandler("secret", processor)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"update_id":1}`))

	handler.HandleTelegramWebhook(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Zero(t, processor.calls)
}

func TestTelegramWebhook_RejectsInvalidSecret(t *testing.T) {
	processor := &recordingTelegramWebhookProcessor{}
	handler := NewTelegramWebhookHandler("secret", processor)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"update_id":1}`))
	c.Request.Header.Set("X-Telegram-Bot-Api-Secret-Token", "wrong")

	handler.HandleTelegramWebhook(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Zero(t, processor.calls)
}

func TestTelegramWebhook_RejectsWhenSecretNotConfigured(t *testing.T) {
	processor := &recordingTelegramWebhookProcessor{}
	handler := NewTelegramWebhookHandler("", processor)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"update_id":1}`))

	handler.HandleTelegramWebhook(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Zero(t, processor.calls)
}

func TestTelegramWebhook_UnavailableWhenPlatformNotConfigured(t *testing.T) {
	processor := &recordingTelegramWebhookProcessor{}
	handler := NewUnavailableTelegramWebhookHandler()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"update_id":1}`))
	c.Request.Header.Set("X-Telegram-Bot-Api-Secret-Token", "secret")

	handler.HandleTelegramWebhook(c)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Zero(t, processor.calls)
	assert.Contains(t, w.Body.String(), "telegram_not_configured")
	assert.Contains(t, w.Body.String(), "telegram webhook unavailable")
}

func TestTelegramWebhook_AcceptsValidSecret(t *testing.T) {
	processor := &recordingTelegramWebhookProcessor{}
	handler := NewTelegramWebhookHandler("secret", processor)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"update_id":1,"message":{"text":"/start token","chat":{"id":123,"type":"private"},"from":{"id":456,"username":"owner"}}}`))
	c.Request.Header.Set("X-Telegram-Bot-Api-Secret-Token", "secret")

	handler.HandleTelegramWebhook(c)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, processor.calls)
	assert.Equal(t, int64(1), processor.update.UpdateID)
	require.NotNil(t, processor.update.Message)
	assert.Equal(t, int64(123), processor.update.Message.Chat.ID)
}
