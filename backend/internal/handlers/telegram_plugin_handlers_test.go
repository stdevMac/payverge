package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	telegramPlugin "github.com/stdevmac/payverge/backend/internal/plugins/telegram"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func setupTelegramPluginHandlerTest(t *testing.T) (*database.Business, *TelegramPluginHandlers) {
	t.Helper()
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Plugin{},
		&database.BusinessPlugin{},
		&database.TelegramConnectionToken{},
		&database.PluginNotificationDelivery{},
		&database.PluginNotificationDeliveryAttempt{},
	))

	business := createTestBusiness(t)
	require.NoError(t, database.GetDB().Create(&database.Plugin{
		Name:        "telegram",
		DisplayName: "Telegram",
		Category:    database.PluginCategoryIntegration,
		IsActive:    true,
	}).Error)

	pluginService := services.NewPluginService(database.GetDBWrapper())
	plugin := telegramPlugin.NewTelegramPlugin(pluginService, nil)
	return business, NewTelegramPluginHandlers(pluginService, plugin)
}

func TestGenerateConnectionToken_ReturnsURLAndExpiryAndPersistsHash(t *testing.T) {
	business, handlers := setupTelegramPluginHandlerTest(t)
	t.Setenv("TELEGRAM_BOT_USERNAME", "@CustomPayvergeBot")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	handlers.GenerateConnectionToken(c)

	require.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Contains(t, response, "url")
	assert.Contains(t, response, "expires_at")
	assert.NotContains(t, response, "token")
	assert.NotContains(t, response, "connection_url")
	assert.Contains(t, response["url"], "https://t.me/CustomPayvergeBot?start=pv_tg_")

	var stored database.TelegramConnectionToken
	require.NoError(t, database.GetDB().Where("business_id = ?", business.ID).First(&stored).Error)
	assert.Len(t, stored.TokenHash, 64)
}

func TestGenerateConnectionToken_Returns503WhenBotUsernameUnset(t *testing.T) {
	business, handlers := setupTelegramPluginHandlerTest(t)
	t.Setenv("TELEGRAM_BOT_USERNAME", "")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	handlers.GenerateConnectionToken(c)

	require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "telegram_not_configured")
	assert.NotContains(t, w.Body.String(), "t.me/")

	var count int64
	require.NoError(t, database.GetDB().Model(&database.TelegramConnectionToken{}).Where("business_id = ?", business.ID).Count(&count).Error)
	assert.Zero(t, count)
}

func TestTelegramStatus_ReturnsPendingWhenTokenExists(t *testing.T) {
	business, handlers := setupTelegramPluginHandlerTest(t)
	expiresAt := time.Now().UTC().Add(15 * time.Minute)
	require.NoError(t, database.CreateTelegramConnectionToken(&database.TelegramConnectionToken{
		BusinessID: business.ID,
		TokenHash:  "pending-token-hash",
		ExpiresAt:  expiresAt,
		CreatedAt:  expiresAt.Add(-15 * time.Minute),
	}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	handlers.GetConnectionStatus(c)

	require.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, false, response["connected"])
	assert.Equal(t, "pending", response["health"])
	assert.NotEmpty(t, response["pending_token_expires_at"])
}

func TestTelegramStatus_ReturnsConnectedMetadata(t *testing.T) {
	business, handlers := setupTelegramPluginHandlerTest(t)
	plugin, err := database.GetPluginByName("telegram")
	require.NoError(t, err)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"chat_id":           "123456789",
		"chat_type":         "group",
		"chat_title":        "Ops Chat",
		"telegram_username": "owner_user",
		"is_connected":      true,
		"connected_at":      "2026-05-11T12:00:00Z",
		"last_sent_at":      "2026-05-11T12:05:00Z",
		"last_test_sent_at": "2026-05-11T12:04:00Z",
		"failure_count":     0,
	}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	handlers.GetConnectionStatus(c)

	require.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, true, response["connected"])
	assert.Equal(t, "connected", response["health"])
	assert.Equal(t, true, response["delivery_available"])
	assert.Equal(t, "123456789", response["chat_id"])
	assert.Equal(t, "group", response["chat_type"])
	assert.Equal(t, "Ops Chat", response["chat_title"])
	assert.Equal(t, "owner_user", response["telegram_username"])
	assert.Equal(t, float64(0), response["failure_count"])
}

func TestTelegramStatus_ReturnsDisabledAndQueueDepth(t *testing.T) {
	business, handlers := setupTelegramPluginHandlerTest(t)
	now := time.Now().UTC()
	plugin, err := database.GetPluginByName("telegram")
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&database.BusinessPlugin{
		BusinessID: business.ID,
		PluginID:   plugin.ID,
		IsEnabled:  false,
		Config:     `{"is_connected":false}`,
	}).Error)
	_, created, err := database.CreatePluginNotificationDelivery(&database.PluginNotificationDelivery{
		BusinessID:    business.ID,
		PluginName:    "telegram",
		EventType:     services.PluginEventOrderCreated,
		EventID:       "order:queue-depth",
		Status:        database.PluginNotificationDeliveryStatusPending,
		Payload:       map[string]interface{}{"order_number": "O-queue"},
		NextAttemptAt: now.Add(-time.Minute),
		CreatedAt:     now.Add(-time.Minute),
	})
	require.NoError(t, err)
	require.True(t, created)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	handlers.GetConnectionStatus(c)

	require.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, false, response["connected"])
	assert.Equal(t, false, response["plugin_enabled"])
	assert.Equal(t, "disabled", response["health"])
	assert.Equal(t, true, response["delivery_available"])
	assert.Equal(t, float64(1), response["delivery_queue_depth"])
}

func TestTelegramStatus_DisconnectedWhenPlatformDeliveryDisabled(t *testing.T) {
	business, handlers := setupTelegramPluginHandlerTest(t)
	plugin, err := database.GetPluginByName("telegram")
	require.NoError(t, err)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"chat_id":      "123456789",
		"is_connected": true,
	}))

	services.SetPluginDeliveryEnabled("telegram", false)
	t.Cleanup(func() { services.SetPluginDeliveryEnabled("telegram", true) })

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	handlers.GetConnectionStatus(c)

	require.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, false, response["connected"], "must not present as connected when the bot token / worker is absent")
	assert.Equal(t, false, response["delivery_available"])
	assert.Equal(t, "disabled", response["health"])
}

func TestTelegramDisconnect_RevokesPendingTokensAndClearsConfig(t *testing.T) {
	business, handlers := setupTelegramPluginHandlerTest(t)
	plugin, err := database.GetPluginByName("telegram")
	require.NoError(t, err)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, plugin.ID, map[string]interface{}{
		"chat_id":           "123456789",
		"chat_type":         "group",
		"chat_title":        "Ops Chat",
		"telegram_username": "owner_user",
		"is_connected":      true,
		"connected_at":      "2026-05-11T12:00:00Z",
		"last_error":        "previous failure",
		"failure_count":     3,
	}))
	require.NoError(t, database.CreateTelegramConnectionToken(&database.TelegramConnectionToken{
		BusinessID: business.ID,
		TokenHash:  "pending-token-hash",
		ExpiresAt:  time.Now().Add(15 * time.Minute),
		CreatedAt:  time.Now(),
	}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	handlers.DisconnectTelegram(c)

	require.Equal(t, http.StatusOK, w.Code)
	config, err := database.GetBusinessPluginConfig(business.ID, "telegram")
	require.NoError(t, err)
	assert.Equal(t, false, config["is_connected"])
	assert.Empty(t, config["chat_id"])
	assert.Empty(t, config["chat_type"])
	assert.Empty(t, config["chat_title"])
	assert.Empty(t, config["telegram_username"])
	assert.Empty(t, config["last_error"])
	assert.Equal(t, float64(0), config["failure_count"])

	var token database.TelegramConnectionToken
	require.NoError(t, database.GetDB().Where("business_id = ?", business.ID).First(&token).Error)
	require.NotNil(t, token.RevokedAt)
}

func TestTelegramRevokeConnectionToken_RevokesPendingTokens(t *testing.T) {
	business, handlers := setupTelegramPluginHandlerTest(t)
	require.NoError(t, database.CreateTelegramConnectionToken(&database.TelegramConnectionToken{
		BusinessID: business.ID,
		TokenHash:  "pending-token-hash",
		ExpiresAt:  time.Now().Add(15 * time.Minute),
		CreatedAt:  time.Now(),
	}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	handlers.RevokeConnectionToken(c)

	require.Equal(t, http.StatusOK, w.Code)
	var response map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "not_connected", response["health"])

	var token database.TelegramConnectionToken
	require.NoError(t, database.GetDB().Where("business_id = ?", business.ID).First(&token).Error)
	require.NotNil(t, token.RevokedAt)
}
