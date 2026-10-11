package handlers

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
)

// WebhookReviewHandler serves the admin review of failed webhook_events rows
// for every payment provider plugin.
type WebhookReviewHandler struct {
	db *database.DB
}

// NewWebhookReviewHandler builds the admin failed-webhook review handler.
func NewWebhookReviewHandler(db *database.DB) *WebhookReviewHandler {
	return &WebhookReviewHandler{db: db}
}

// AdminGetAllFailedWebhooks returns failed webhook events across all providers.
// GET /api/v1/admin/webhooks/failed
func (h *WebhookReviewHandler) AdminGetAllFailedWebhooks(c *gin.Context) {
	limit := 50
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 200 {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "limit must be between 1 and 200")
			return
		}
		limit = parsed
	}

	events, err := h.db.GetAllFailedWebhookEvents(limit)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to get failed webhooks")
		return
	}

	result := make([]map[string]interface{}, 0, len(events))
	for _, e := range events {
		result = append(result, webhookEventAdminPayload(e))
	}

	c.JSON(http.StatusOK, gin.H{
		"events": result,
		"count":  len(result),
	})
}

type acknowledgeFailedWebhookRequest struct {
	Reason string `json:"reason"`
}

// AdminAcknowledgeFailedWebhook marks a failed webhook as ignored after review.
// POST /api/v1/admin/webhooks/:id/acknowledge
func (h *WebhookReviewHandler) AdminAcknowledgeFailedWebhook(c *gin.Context) {
	idStr := c.Param("id")
	var id uint
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid webhook event ID")
		return
	}

	event, err := h.db.GetWebhookEventByID(id)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to get webhook event")
		return
	}
	if event == nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Webhook event not found")
		return
	}
	if event.Status != "failed" {
		server.RespondWithError(c, http.StatusConflict, server.ErrCodeInvalidInput, "Only failed webhook events can be acknowledged")
		return
	}

	var req acknowledgeFailedWebhookRequest
	if c.Request.Body != nil {
		if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
			server.RespondBindError(c, err)
			return
		}
	}

	updated, err := h.db.MarkWebhookEventIgnored(id, req.Reason)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to acknowledge webhook event")
		return
	}

	c.JSON(http.StatusOK, gin.H{"event": webhookEventAdminPayload(*updated)})
}

func webhookEventAdminPayload(e database.WebhookEvent) map[string]interface{} {
	return map[string]interface{}{
		"id":           e.ID,
		"provider":     e.Provider,
		"webhook_id":   e.WebhookID,
		"event_type":   e.EventType,
		"status":       e.Status,
		"error":        e.Error,
		"received_at":  e.ReceivedAt,
		"processed_at": e.ProcessedAt,
		"created_at":   e.CreatedAt,
	}
}
