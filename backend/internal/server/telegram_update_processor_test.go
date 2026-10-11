package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

type recordingTelegramReplySender struct {
	chatIDs  []int64
	messages []string
}

func (s *recordingTelegramReplySender) SendTelegramMessage(chatID int64, text string) error {
	s.chatIDs = append(s.chatIDs, chatID)
	s.messages = append(s.messages, text)
	return nil
}

func setupTelegramUpdateProcessorTestDB(t *testing.T) (*database.Business, string) {
	t.Helper()

	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Plugin{},
		&database.BusinessPlugin{},
		&database.TelegramConnectionToken{},
		&database.TelegramUpdateReceipt{},
	))
	database.SetTestDB(gormDB)

	business := &database.Business{
		BusinessId:     "telegram-update-test",
		OwnerAddress:   "owner",
		Name:           "Telegram Update Test",
		SettlementAddr: "settlement",
		TippingAddr:    "tipping",
	}
	require.NoError(t, gormDB.Create(business).Error)

	require.NoError(t, gormDB.Create(&database.Plugin{
		Name:        "telegram",
		DisplayName: "Telegram",
		Category:    database.PluginCategoryIntegration,
		IsActive:    true,
	}).Error)

	rawToken := "pv_tg_valid_token"
	require.NoError(t, database.CreateTelegramConnectionToken(&database.TelegramConnectionToken{
		BusinessID: business.ID,
		TokenHash:  HashTelegramConnectionToken(rawToken),
		ExpiresAt:  time.Date(2026, 5, 11, 12, 15, 0, 0, time.UTC),
		CreatedAt:  time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC),
	}))

	return business, rawToken
}

func TestTelegramUpdateProcessor_BindsValidStartToken(t *testing.T) {
	business, rawToken := setupTelegramUpdateProcessorTestDB(t)
	now := time.Date(2026, 5, 11, 12, 5, 0, 0, time.UTC)
	replies := &recordingTelegramReplySender{}
	processor := &TelegramUpdateProcessor{
		ReplySender: replies,
		Now:         func() time.Time { return now },
	}

	err := processor.ProcessUpdate(context.Background(), TelegramWebhookUpdate{
		UpdateID: 1001,
		Message: &TelegramWebhookMessage{
			Text: "/start " + rawToken,
			Chat: TelegramWebhookChat{
				ID:    123456789,
				Type:  "private",
				Title: "Owner Chat",
			},
			From: &TelegramWebhookUser{Username: "owner_user"},
		},
	})

	require.NoError(t, err)
	require.Len(t, replies.messages, 1)
	assert.Contains(t, replies.messages[0], "Connected to Payverge")

	config, err := database.GetBusinessPluginConfig(business.ID, "telegram")
	require.NoError(t, err)
	assert.Equal(t, "123456789", config["chat_id"])
	assert.Equal(t, "private", config["chat_type"])
	assert.Equal(t, "Owner Chat", config["chat_title"])
	assert.Equal(t, "owner_user", config["telegram_username"])
	assert.Equal(t, true, config["is_connected"])
	assert.Empty(t, config["last_error"])

	var token database.TelegramConnectionToken
	require.NoError(t, database.GetDB().Where("token_hash = ?", HashTelegramConnectionToken(rawToken)).First(&token).Error)
	require.NotNil(t, token.UsedAt)
	assert.Equal(t, int64(123456789), *token.UsedByChatID)
}

func TestTelegramUpdateProcessor_DeduplicatesUpdateID(t *testing.T) {
	_, rawToken := setupTelegramUpdateProcessorTestDB(t)
	now := time.Date(2026, 5, 11, 12, 5, 0, 0, time.UTC)
	replies := &recordingTelegramReplySender{}
	processor := &TelegramUpdateProcessor{
		ReplySender: replies,
		Now:         func() time.Time { return now },
	}
	update := TelegramWebhookUpdate{
		UpdateID: 1001,
		Message: &TelegramWebhookMessage{
			Text: "/start " + rawToken,
			Chat: TelegramWebhookChat{ID: 123456789, Type: "private"},
			From: &TelegramWebhookUser{Username: "owner_user"},
		},
	}

	require.NoError(t, processor.ProcessUpdate(context.Background(), update))
	require.NoError(t, processor.ProcessUpdate(context.Background(), update))

	assert.Len(t, replies.messages, 1)
}

func TestTelegramUpdateProcessor_RejectsExpiredToken(t *testing.T) {
	business, rawToken := setupTelegramUpdateProcessorTestDB(t)
	now := time.Date(2026, 5, 11, 12, 30, 0, 0, time.UTC)
	replies := &recordingTelegramReplySender{}
	processor := &TelegramUpdateProcessor{
		ReplySender: replies,
		Now:         func() time.Time { return now },
	}

	err := processor.ProcessUpdate(context.Background(), TelegramWebhookUpdate{
		UpdateID: 1002,
		Message: &TelegramWebhookMessage{
			Text: "/start " + rawToken,
			Chat: TelegramWebhookChat{ID: 123456789, Type: "private"},
			From: &TelegramWebhookUser{Username: "owner_user"},
		},
	})

	require.NoError(t, err)
	require.Len(t, replies.messages, 1)
	assert.Contains(t, replies.messages[0], "invalid or expired")

	_, err = database.GetBusinessPluginConfig(business.ID, "telegram")
	require.Error(t, err)
	assert.ErrorIs(t, err, database.ErrBusinessPluginNotEnabled)
}

func TestTelegramUpdateProcessor_DoesNotConsumeTokenWhenBindingFails(t *testing.T) {
	_, rawToken := setupTelegramUpdateProcessorTestDB(t)
	now := time.Date(2026, 5, 11, 12, 5, 0, 0, time.UTC)
	require.NoError(t, database.GetDB().Where("name = ?", "telegram").Delete(&database.Plugin{}).Error)

	processor := &TelegramUpdateProcessor{
		ReplySender: &recordingTelegramReplySender{},
		Now:         func() time.Time { return now },
	}

	err := processor.ProcessUpdate(context.Background(), TelegramWebhookUpdate{
		UpdateID: 1006,
		Message: &TelegramWebhookMessage{
			Text: "/start " + rawToken,
			Chat: TelegramWebhookChat{ID: 123456789, Type: "private"},
			From: &TelegramWebhookUser{Username: "owner_user"},
		},
	})

	require.Error(t, err)

	var token database.TelegramConnectionToken
	require.NoError(t, database.GetDB().Where("token_hash = ?", HashTelegramConnectionToken(rawToken)).First(&token).Error)
	assert.Nil(t, token.UsedAt)
	assert.Nil(t, token.UsedByChatID)
}

func TestTelegramUpdateProcessor_RejectsChannelChatWithoutConsumingToken(t *testing.T) {
	business, rawToken := setupTelegramUpdateProcessorTestDB(t)
	now := time.Date(2026, 5, 11, 12, 5, 0, 0, time.UTC)
	replies := &recordingTelegramReplySender{}
	processor := &TelegramUpdateProcessor{
		ReplySender: replies,
		Now:         func() time.Time { return now },
	}

	err := processor.ProcessUpdate(context.Background(), TelegramWebhookUpdate{
		UpdateID: 1004,
		Message: &TelegramWebhookMessage{
			Text: "/start " + rawToken,
			Chat: TelegramWebhookChat{ID: -100123456789, Type: "channel", Title: "Broadcast Channel"},
			From: &TelegramWebhookUser{Username: "owner_user"},
		},
	})

	require.NoError(t, err)
	require.Len(t, replies.messages, 1)
	assert.Contains(t, replies.messages[0], "private chat or group")

	var token database.TelegramConnectionToken
	require.NoError(t, database.GetDB().Where("token_hash = ?", HashTelegramConnectionToken(rawToken)).First(&token).Error)
	assert.Nil(t, token.UsedAt)

	_, err = database.GetBusinessPluginConfig(business.ID, "telegram")
	require.Error(t, err)
	assert.ErrorIs(t, err, database.ErrBusinessPluginNotEnabled)
}

func TestTelegramUpdateProcessor_IgnoresNonStartMessage(t *testing.T) {
	setupTelegramUpdateProcessorTestDB(t)
	now := time.Date(2026, 5, 11, 12, 5, 0, 0, time.UTC)
	replies := &recordingTelegramReplySender{}
	processor := &TelegramUpdateProcessor{
		ReplySender: replies,
		Now:         func() time.Time { return now },
	}

	err := processor.ProcessUpdate(context.Background(), TelegramWebhookUpdate{
		UpdateID: 1003,
		Message: &TelegramWebhookMessage{
			Text: "hello",
			Chat: TelegramWebhookChat{ID: 123456789, Type: "private"},
		},
	})

	require.NoError(t, err)
	require.Len(t, replies.messages, 1)
	assert.Contains(t, replies.messages[0], "no Payverge connection command")
}

func TestTelegramUpdateProcessor_DoesNotLogRawToken(t *testing.T) {
	_, rawToken := setupTelegramUpdateProcessorTestDB(t)
	var logs bytes.Buffer
	previousOutput := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previousOutput)

	processor := &TelegramUpdateProcessor{
		ReplySender: &recordingTelegramReplySender{},
		Now:         func() time.Time { return time.Date(2026, 5, 11, 12, 30, 0, 0, time.UTC) },
	}

	err := processor.ProcessUpdate(context.Background(), TelegramWebhookUpdate{
		UpdateID: 1005,
		Message: &TelegramWebhookMessage{
			Text: "/start " + rawToken,
			Chat: TelegramWebhookChat{ID: 123456789, Type: "private"},
		},
	})

	require.NoError(t, err)
	assert.NotContains(t, logs.String(), rawToken)
}

func TestTelegramUpdateProcessor_BindsTokenFromEditedMessage(t *testing.T) {
	business, rawToken := setupTelegramUpdateProcessorTestDB(t)
	now := time.Date(2026, 5, 11, 12, 5, 0, 0, time.UTC)
	processor := &TelegramUpdateProcessor{ReplySender: &recordingTelegramReplySender{}, Now: func() time.Time { return now }}

	err := processor.ProcessUpdate(context.Background(), TelegramWebhookUpdate{
		UpdateID: 2001,
		EditedMessage: &TelegramWebhookMessage{
			Text: "/start " + rawToken,
			Chat: TelegramWebhookChat{ID: 555, Type: "private"},
			From: &TelegramWebhookUser{Username: "edituser"},
		},
	})
	require.NoError(t, err)
	require.True(t, telegramBusinessIsConnected(t, business.ID))
}

func TestTelegramUpdateProcessor_BindsTokenFromCallbackQuery(t *testing.T) {
	business, rawToken := setupTelegramUpdateProcessorTestDB(t)
	now := time.Date(2026, 5, 11, 12, 5, 0, 0, time.UTC)
	processor := &TelegramUpdateProcessor{ReplySender: &recordingTelegramReplySender{}, Now: func() time.Time { return now }}

	err := processor.ProcessUpdate(context.Background(), TelegramWebhookUpdate{
		UpdateID: 2002,
		CallbackQuery: &TelegramWebhookCallbackQuery{
			From: &TelegramWebhookUser{Username: "cbuser"},
			Message: &TelegramWebhookMessage{
				Text: "/start " + rawToken,
				Chat: TelegramWebhookChat{ID: 777, Type: "group"},
			},
		},
	})
	require.NoError(t, err)
	require.True(t, telegramBusinessIsConnected(t, business.ID))
}

func TestTelegramUpdateProcessor_RejectsChannelPost(t *testing.T) {
	_, rawToken := setupTelegramUpdateProcessorTestDB(t)
	now := time.Date(2026, 5, 11, 12, 5, 0, 0, time.UTC)
	replies := &recordingTelegramReplySender{}
	processor := &TelegramUpdateProcessor{ReplySender: replies, Now: func() time.Time { return now }}

	err := processor.ProcessUpdate(context.Background(), TelegramWebhookUpdate{
		UpdateID: 2003,
		ChannelPost: &TelegramWebhookMessage{
			Text: "/start " + rawToken,
			Chat: TelegramWebhookChat{ID: 999, Type: "channel"},
		},
	})
	require.NoError(t, err)
	require.Contains(t, replies.messages, TelegramConnectionInvalidChatType)
}

// telegramBusinessIsConnected reads the persisted telegram BusinessPlugin
// config and reports whether is_connected is true.
func telegramBusinessIsConnected(t *testing.T, businessID uint) bool {
	t.Helper()
	var plugin database.Plugin
	require.NoError(t, database.GetDB().Where("name = ?", "telegram").First(&plugin).Error)
	var bp database.BusinessPlugin
	require.NoError(t, database.GetDB().Where("business_id = ? AND plugin_id = ?", businessID, plugin.ID).First(&bp).Error)
	var cfg map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(bp.Config), &cfg))
	connected, _ := cfg["is_connected"].(bool)
	return connected
}

func TestTelegramUpdateProcessor_ReplaysAfterTransientBindFailure(t *testing.T) {
	_, rawToken := setupTelegramUpdateProcessorTestDB(t)
	now := time.Date(2026, 5, 11, 12, 5, 0, 0, time.UTC)

	// Delete the telegram plugin so bind fails transiently on the first attempt.
	require.NoError(t, database.GetDB().Where("name = ?", "telegram").Delete(&database.Plugin{}).Error)

	processor := &TelegramUpdateProcessor{
		ReplySender: &recordingTelegramReplySender{},
		Now:         func() time.Time { return now },
	}

	update := TelegramWebhookUpdate{
		UpdateID: 2004,
		Message: &TelegramWebhookMessage{
			Text: "/start " + rawToken,
			Chat: TelegramWebhookChat{ID: 123456789, Type: "private"},
			From: &TelegramWebhookUser{Username: "owner_user"},
		},
	}

	// First attempt: binding fails transiently.
	err := processor.ProcessUpdate(context.Background(), update)
	require.Error(t, err, "first attempt should fail transiently")

	// Receipt must be deleted so the retry is not deduped away.
	var receipt database.TelegramUpdateReceipt
	receiptErr := database.GetDB().Where("update_id = ?", update.UpdateID).First(&receipt).Error
	require.Error(t, receiptErr, "receipt must be deleted after transient failure")

	// Token must NOT be consumed yet.
	var token database.TelegramConnectionToken
	require.NoError(t, database.GetDB().Where("token_hash = ?", HashTelegramConnectionToken(rawToken)).First(&token).Error)
	assert.Nil(t, token.UsedAt, "token must not be consumed after transient failure")

	// Re-create the telegram plugin so the retry succeeds.
	require.NoError(t, database.GetDB().Create(&database.Plugin{
		Name:        "telegram",
		DisplayName: "Telegram",
		Category:    database.PluginCategoryIntegration,
		IsActive:    true,
	}).Error)

	// Second attempt: must succeed and consume the token.
	err = processor.ProcessUpdate(context.Background(), update)
	require.NoError(t, err, "second attempt should succeed after transient failure was rolled back")

	require.NoError(t, database.GetDB().Where("token_hash = ?", HashTelegramConnectionToken(rawToken)).First(&token).Error)
	assert.NotNil(t, token.UsedAt, "token must be consumed on retry")
}

// TestTelegramDefaultConnectionConfig_OnlyAdvertisesRenderableEvents is the TG-2
// regression guard. The seeded default config must not advertise an event the
// Telegram tier cannot render. order.status_changed has no renderer (it was
// stopped from enqueuing in a prior fix), so a default of true for
// "order_status_changed" misleads operators into thinking it's a live toggle.
// Assert it is gone, and that every event the default DOES advertise actually
// resolves through the gate to a renderable event.
func TestTelegramDefaultConnectionConfig_OnlyAdvertisesRenderableEvents(t *testing.T) {
	config := telegramDefaultConnectionConfigForBusinessName("Bistro 21")

	notifications, ok := config["notifications"].(map[string]interface{})
	require.True(t, ok, "default config must carry a notifications map")

	// The stale, unrenderable key must not be advertised at all.
	_, present := notifications["order_status_changed"]
	assert.False(t, present, "order_status_changed has no Telegram renderer; the default must not advertise it")

	// Every advertised notification key must map to an event the gate recognizes
	// as renderable. We prove this by isolating each key in an otherwise-empty
	// notifications map set to true and confirming the gate enables the event it
	// belongs to. A stale/unrenderable key (no preference mapping) would fall
	// through to the false default and fail here.
	renderableEvents := []string{
		services.PluginEventOrderCreated,
		services.PluginEventPaymentReceived,
		services.PluginEventReservationCreated,
		services.PluginEventReservationStatusChanged,
		services.PluginEventInventoryLowStock,
		services.PluginEventDailySummary,
	}
	for key := range notifications {
		matched := false
		for _, event := range renderableEvents {
			isolated := map[string]interface{}{
				"notifications": map[string]interface{}{key: true},
			}
			if services.TelegramEventNotificationEnabled(isolated, event) {
				matched = true
				break
			}
		}
		assert.Truef(t, matched, "default config advertises notification key %q which maps to no renderable Telegram event", key)
	}
}
