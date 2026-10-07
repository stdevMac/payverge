package handlers

import (
	"context"
	"crypto/subtle"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/server"
)

type TelegramWebhookProcessor interface {
	ProcessUpdate(ctx context.Context, update server.TelegramWebhookUpdate) error
}

type TelegramWebhookHandler struct {
	secret      string
	processor   TelegramWebhookProcessor
	unavailable bool
}

func NewTelegramWebhookHandler(secret string, processor TelegramWebhookProcessor) *TelegramWebhookHandler {
	return &TelegramWebhookHandler{
		secret:    strings.TrimSpace(secret),
		processor: processor,
	}
}

// NewUnavailableTelegramWebhookHandler registers a refuse-only webhook so
// POST /api/v1/webhooks/telegram is never a bare 404 when the bot token or
// webhook secret is missing. Callers get 503 + a stable error code.
func NewUnavailableTelegramWebhookHandler() *TelegramWebhookHandler {
	return &TelegramWebhookHandler{unavailable: true}
}

func (h *TelegramWebhookHandler) HandleTelegramWebhook(c *gin.Context) {
	if h.unavailable {
		metrics.TelegramWebhookUpdates.WithLabelValues("unavailable").Inc()
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "telegram webhook unavailable",
			"code":  "telegram_not_configured",
		})
		return
	}
	if !h.validSecret(c.GetHeader("X-Telegram-Bot-Api-Secret-Token")) {
		metrics.TelegramWebhookUpdates.WithLabelValues("unauthorized").Inc()
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var update server.TelegramWebhookUpdate
	if err := c.ShouldBindJSON(&update); err != nil {
		metrics.TelegramWebhookUpdates.WithLabelValues("invalid_json").Inc()
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid telegram update"})
		return
	}

	if h.processor == nil {
		metrics.TelegramWebhookUpdates.WithLabelValues("processor_unavailable").Inc()
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "telegram webhook processor unavailable"})
		return
	}

	if err := h.processor.ProcessUpdate(c.Request.Context(), update); err != nil {
		log.Printf("Telegram webhook processing failed for update_id=%d: %v", update.UpdateID, err)
		metrics.TelegramWebhookUpdates.WithLabelValues("processing_failed").Inc()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "telegram webhook processing failed"})
		return
	}

	metrics.TelegramWebhookUpdates.WithLabelValues("accepted").Inc()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *TelegramWebhookHandler) validSecret(header string) bool {
	if h.secret == "" {
		return false
	}
	header = strings.TrimSpace(header)
	if header == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(header), []byte(h.secret)) == 1
}
