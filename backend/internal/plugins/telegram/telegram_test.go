package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

type fakeTelegramBotSender struct {
	message tgbotapi.Chattable
	err     error
}

func (s *fakeTelegramBotSender) Send(message tgbotapi.Chattable) (tgbotapi.Message, error) {
	s.message = message
	if s.err != nil {
		return tgbotapi.Message{}, s.err
	}
	return tgbotapi.Message{MessageID: 42}, nil
}

func setupTelegramPluginSenderTest(t *testing.T, connected bool) (*TelegramPlugin, *database.Business, *fakeTelegramBotSender) {
	t.Helper()

	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, gormDB.AutoMigrate(&database.Business{}, &database.Plugin{}, &database.BusinessPlugin{}))
	database.SetTestDB(gormDB)

	business := &database.Business{
		BusinessId:      fmt.Sprintf("telegram-plugin-%d", time.Now().UnixNano()),
		OwnerAddress:    "owner",
		Name:            "Telegram Plugin Test",
		SettlementAddr:  "settlement",
		TippingAddr:     "tipping",
		Timezone:        "UTC",
		DefaultCurrency: "USD",
		DisplayCurrency: "USD",
	}
	require.NoError(t, gormDB.Create(business).Error)

	pluginRow := &database.Plugin{
		Name:        "telegram",
		DisplayName: "Telegram",
		Category:    database.PluginCategoryIntegration,
		IsActive:    true,
	}
	require.NoError(t, gormDB.Create(pluginRow).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, pluginRow.ID, map[string]interface{}{
		"chat_id":      "123456789",
		"is_connected": connected,
	}))

	sender := &fakeTelegramBotSender{}
	plugin := NewTelegramPlugin(services.NewPluginService(database.GetDBWrapper()), nil)
	plugin.botSender = sender
	return plugin, business, sender
}

func TestTelegramBusinessNotificationContext_ThreadsLanguage(t *testing.T) {
	plugin, business, _ := setupTelegramPluginSenderTest(t, true)
	business.Timezone = "America/New_York"
	business.DisplayCurrency = "EUR"
	business.DefaultLanguage = "es-AR"
	require.NoError(t, database.UpdateBusiness(business))

	timezone, currency, language := plugin.businessNotificationContext(business.ID)
	assert.Equal(t, "America/New_York", timezone)
	assert.Equal(t, "EUR", currency)
	assert.Equal(t, "es-AR", language)
}

func TestTelegramBusinessNotificationContext_FallsBackOnLoadFailure(t *testing.T) {
	plugin, _, _ := setupTelegramPluginSenderTest(t, true)

	timezone, currency, language := plugin.businessNotificationContext(99999999)
	assert.Equal(t, "UTC", timezone)
	assert.Equal(t, "USD", currency)
	assert.Equal(t, "", language)
}

func TestTelegramSendPluginNotification_DeliversAndUpdatesState(t *testing.T) {
	plugin, business, sender := setupTelegramPluginSenderTest(t, true)
	delivery := database.PluginNotificationDelivery{
		BusinessID: business.ID,
		EventType:  services.PluginEventOrderCreated,
		EventID:    "order:42",
		Payload: map[string]interface{}{
			"order_number": "O-42",
			"table_name":   "Patio",
			"item_count":   2,
			"total_cents":  2500,
			"currency":     "USD",
		},
	}

	result := plugin.SendPluginNotification(context.Background(), delivery)

	require.NoError(t, result.Err)
	assert.Equal(t, "42", result.ProviderMessageID)
	require.NotNil(t, sender.message)

	config, err := database.GetBusinessPluginConfig(business.ID, "telegram")
	require.NoError(t, err)
	assert.NotEmpty(t, config["last_sent_at"])
	assert.Empty(t, config["last_error"])
	assert.Equal(t, float64(0), config["failure_count"])
}

func TestTelegramSendPluginNotification_RendersInBusinessLanguage(t *testing.T) {
	plugin, business, sender := setupTelegramPluginSenderTest(t, true)
	business.DefaultLanguage = "es"
	require.NoError(t, database.UpdateBusiness(business))

	result := plugin.SendPluginNotification(context.Background(), database.PluginNotificationDelivery{
		BusinessID: business.ID,
		EventType:  services.PluginEventOrderCreated,
		EventID:    "order:77",
		Payload: map[string]interface{}{
			"order_number": "O-77",
			"table_name":   "Patio",
			"item_count":   2,
			"total_cents":  2500,
			"currency":     "USD",
		},
	})

	require.NoError(t, result.Err)
	msg, ok := sender.message.(tgbotapi.MessageConfig)
	require.True(t, ok)
	assert.Contains(t, msg.Text, "Nuevo pedido #O-77")
	assert.Contains(t, msg.Text, "Mesa: Patio")
	assert.NotContains(t, msg.Text, "New order")
}

func TestTelegramConnectBusiness_SendsWelcomeInBusinessLanguage(t *testing.T) {
	plugin, business, sender := setupTelegramPluginSenderTest(t, false)
	business.DefaultLanguage = "es"
	require.NoError(t, database.UpdateBusiness(business))

	require.NoError(t, plugin.ConnectBusiness(business.ID, 555, "Bistró 21"))

	msg, ok := sender.message.(tgbotapi.MessageConfig)
	require.True(t, ok)
	assert.Contains(t, msg.Text, "Bienvenido a las notificaciones de Payverge")
	assert.Contains(t, msg.Text, "Bistró 21")
	assert.NotContains(t, msg.Text, "Welcome to Payverge")
}

func TestTelegramSendNotification_RendersTestMessageInBusinessLanguage(t *testing.T) {
	plugin, business, sender := setupTelegramPluginSenderTest(t, true)
	business.DefaultLanguage = "es"
	require.NoError(t, database.UpdateBusiness(business))

	require.NoError(t, plugin.SendNotification(business.ID, "test", "hola equipo", nil))

	msg, ok := sender.message.(tgbotapi.MessageConfig)
	require.True(t, ok)
	assert.Contains(t, msg.Text, "Notificación")
	assert.Contains(t, msg.Text, "hola equipo")
	assert.Contains(t, msg.Text, "Enviado a las")
	assert.NotContains(t, msg.Text, "Sent at")
}

func TestTelegramSendPluginNotification_DropsWhenDisconnected(t *testing.T) {
	plugin, business, sender := setupTelegramPluginSenderTest(t, false)

	result := plugin.SendPluginNotification(context.Background(), database.PluginNotificationDelivery{
		BusinessID: business.ID,
		EventType:  services.PluginEventOrderCreated,
		EventID:    "order:42",
		Payload:    map[string]interface{}{"order_number": "O-42"},
	})

	assert.True(t, result.Drop)
	assert.Equal(t, "telegram_not_connected", result.ErrorCode)
	assert.Nil(t, sender.message)
}

func TestTelegramSendNotification_UpdatesLastTestSentAtOnSuccess(t *testing.T) {
	plugin, business, _ := setupTelegramPluginSenderTest(t, true)

	err := plugin.SendNotification(business.ID, "test", "Test message", nil)

	require.NoError(t, err)
	config, err := database.GetBusinessPluginConfig(business.ID, "telegram")
	require.NoError(t, err)
	assert.NotEmpty(t, config["last_test_sent_at"])
	assert.Empty(t, config["last_error"])
}

func TestTelegramSendPluginNotification_DropsWhenEventDisabled(t *testing.T) {
	plugin, business, sender := setupTelegramPluginSenderTest(t, true)
	config, err := database.GetBusinessPluginConfig(business.ID, "telegram")
	require.NoError(t, err)
	config["notifications"] = map[string]interface{}{"low_inventory": false}
	require.NoError(t, database.UpdateBusinessPluginConfig(business.ID, "telegram", config))

	result := plugin.SendPluginNotification(context.Background(), database.PluginNotificationDelivery{
		BusinessID: business.ID,
		EventType:  services.PluginEventInventoryLowStock,
		EventID:    "inventory:7:low_stock:2026-05-11",
		Payload:    map[string]interface{}{"item_name": "Tomatoes"},
	})

	assert.True(t, result.Drop)
	assert.Equal(t, "telegram_notification_disabled", result.ErrorCode)
	assert.Nil(t, sender.message)
}

func TestTelegramSendPluginNotification_ClassifiesRetryableRateLimit(t *testing.T) {
	plugin, business, sender := setupTelegramPluginSenderTest(t, true)
	sender.err = errors.New("Too Many Requests: retry after 3")

	result := plugin.SendPluginNotification(context.Background(), database.PluginNotificationDelivery{
		BusinessID: business.ID,
		EventType:  services.PluginEventOrderCreated,
		EventID:    "order:42",
		Payload:    map[string]interface{}{"order_number": "O-42"},
	})

	require.Error(t, result.Err)
	assert.False(t, result.PermanentFailure)
	assert.Equal(t, "telegram_rate_limited", result.ErrorCode)
	assert.Equal(t, 3*time.Second, result.RetryAfter)
}

func TestTelegramSendPluginNotification_ClassifiesForbiddenAsPermanent(t *testing.T) {
	plugin, business, sender := setupTelegramPluginSenderTest(t, true)
	sender.err = errors.New("Forbidden: bot was blocked by the user")

	result := plugin.SendPluginNotification(context.Background(), database.PluginNotificationDelivery{
		BusinessID: business.ID,
		EventType:  services.PluginEventPaymentReceived,
		EventID:    "payment:42",
		Payload:    map[string]interface{}{"amount_cents": 4200},
	})

	assert.True(t, result.PermanentFailure)
	assert.Equal(t, "telegram_forbidden", result.ErrorCode)
	assert.Contains(t, result.ErrorMessage, "bot was blocked")
}

func TestTelegramSendPluginNotification_TruncatesLongMessage(t *testing.T) {
	plugin, business, sender := setupTelegramPluginSenderTest(t, true)

	result := plugin.SendPluginNotification(context.Background(), database.PluginNotificationDelivery{
		BusinessID: business.ID,
		EventType:  services.PluginEventOrderCreated,
		EventID:    "order:42",
		Payload: map[string]interface{}{
			"order_number": "O-42",
			"notes":        strings.Repeat("long ", 1200),
		},
	})

	require.NoError(t, result.Err)
	msg, ok := sender.message.(tgbotapi.MessageConfig)
	require.True(t, ok)
	assert.LessOrEqual(t, len(msg.Text), 4096)
	assert.Contains(t, msg.Text, "truncated")
	assert.Equal(t, "HTML", msg.ParseMode)
}
