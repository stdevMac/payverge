package handlers

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/plugins/telegram"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// TelegramPluginHandlers handles Telegram plugin-specific HTTP requests
type TelegramPluginHandlers struct {
	pluginService  *services.PluginService
	telegramPlugin *telegram.TelegramPlugin
}

// NewTelegramPluginHandlers creates a new Telegram plugin handlers instance
func NewTelegramPluginHandlers(pluginService *services.PluginService, telegramPlugin *telegram.TelegramPlugin) *TelegramPluginHandlers {
	return &TelegramPluginHandlers{
		pluginService:  pluginService,
		telegramPlugin: telegramPlugin,
	}
}

// GetConnectionStatusRequest represents the request to get connection status
type GetConnectionStatusRequest struct {
	BusinessID uint `json:"business_id" binding:"required"`
}

// SendTestNotificationRequest represents the request to send a test notification
type SendTestNotificationRequest struct {
	Message string `json:"message" binding:"required"`
}

// GetConnectionStatus returns the Telegram connection status for a business
func (tph *TelegramPluginHandlers) GetConnectionStatus(c *gin.Context) {
	businessID, err := resolveBusinessID(c.Param("id"))
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	status, err := tph.telegramPlugin.GetConnectionStatus(businessID)
	if err != nil {
		log.Printf("Failed to get Telegram connection status for business %d: %v", businessID, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to get Telegram connection status")
		return
	}

	c.JSON(http.StatusOK, status)
}

// SendTestNotification sends a test notification to verify the connection
func (tph *TelegramPluginHandlers) SendTestNotification(c *gin.Context) {
	businessID, err := resolveBusinessID(c.Param("id"))
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	var request SendTestNotificationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		server.RespondBindError(c, err)
		return
	}

	// Send test notification
	if err := tph.telegramPlugin.SendNotification(businessID, "test", request.Message, nil); err != nil {
		log.Printf("Failed to send Telegram test notification for business %d: %v", businessID, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to send Telegram test notification")
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Test notification sent successfully"})
}

// GenerateConnectionToken generates a unique token for Telegram connection
func (tph *TelegramPluginHandlers) GenerateConnectionToken(c *gin.Context) {
	businessID, err := resolveBusinessID(c.Param("id"))
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	result, err := server.DefaultTelegramConnectionService().GenerateConnectionToken(businessID, nil, nil, c.ClientIP())
	if err != nil {
		if errors.Is(err, server.ErrTelegramBotUsernameNotConfigured) {
			server.RespondWithError(c, http.StatusServiceUnavailable, "telegram_not_configured", "Telegram is not configured on this instance (TELEGRAM_BOT_USERNAME is unset).")
			return
		}
		if errors.Is(err, server.ErrTelegramConnectionTokenRateLimited) {
			server.RespondWithError(c, http.StatusTooManyRequests, "telegram_token_rate_limited", "Too many Telegram connection links generated. Please wait before trying again.")
			return
		}
		log.Printf("Failed to generate Telegram connection token for business %d: %v", businessID, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to generate Telegram connection token")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"url":        result.URL,
		"expires_at": result.ExpiresAt,
	})
}

func (tph *TelegramPluginHandlers) RevokeConnectionToken(c *gin.Context) {
	businessID, err := resolveBusinessID(c.Param("id"))
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	if err := database.RevokePendingTelegramConnectionTokens(businessID); err != nil {
		log.Printf("Failed to revoke Telegram connection tokens for business %d: %v", businessID, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to revoke Telegram connection token")
		return
	}

	status, err := tph.telegramPlugin.GetConnectionStatus(businessID)
	if err != nil {
		log.Printf("Failed to load Telegram status after token revoke for business %d: %v", businessID, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to revoke Telegram connection token")
		return
	}
	status["message"] = "Telegram connection token revoked successfully"
	c.JSON(http.StatusOK, status)
}

// DisconnectTelegram disconnects a business from Telegram
func (tph *TelegramPluginHandlers) DisconnectTelegram(c *gin.Context) {
	businessID, err := resolveBusinessID(c.Param("id"))
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	// Get current config
	config, err := tph.pluginService.GetPluginConfig(businessID, "telegram")
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Telegram not configured for this business")
		return
	}

	if err := tph.telegramPlugin.Cleanup(businessID); err != nil {
		log.Printf("Telegram cleanup failed during disconnect for business %d: %v", businessID, err)
	}

	// Update config to disconnect
	config["is_connected"] = false
	config["chat_id"] = ""
	config["chat_type"] = ""
	config["chat_title"] = ""
	config["telegram_username"] = ""
	config["connected_at"] = nil
	config["last_error"] = ""
	config["last_error_at"] = nil
	config["failure_count"] = 0

	if err := database.RevokePendingTelegramConnectionTokens(businessID); err != nil {
		log.Printf("Failed to revoke Telegram connection tokens for business %d: %v", businessID, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to disconnect Telegram")
		return
	}

	if err := tph.pluginService.UpdatePluginConfig(businessID, "telegram", config); err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to disconnect Telegram")
		return
	}
	refreshTelegramConnectedBusinessesMetric()

	c.JSON(http.StatusOK, gin.H{"message": "Telegram disconnected successfully"})
}

func refreshTelegramConnectedBusinessesMetric() {
	count, err := database.CountConnectedTelegramBusinesses()
	if err != nil {
		return
	}
	metrics.TelegramConnectedBusinesses.Set(float64(count))
}
