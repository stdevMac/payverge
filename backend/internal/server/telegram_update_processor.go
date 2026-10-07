package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"gorm.io/gorm"
)

const (
	TelegramConnectionSuccessMessage   = "Connected to Payverge. Telegram notifications are now enabled for this business."
	TelegramConnectionInvalidMessage   = "This Payverge connection link is invalid or expired. Generate a new link from your Payverge dashboard."
	TelegramConnectionNoCommandMessage = "This Telegram update was received, but no Payverge connection command was found."
	TelegramConnectionInvalidChatType  = "Telegram notifications can only be connected from a private chat or group, not a channel."
)

type TelegramReplySender interface {
	SendTelegramMessage(chatID int64, text string) error
}

type TelegramUpdateProcessor struct {
	ReplySender TelegramReplySender
	Now         func() time.Time
}

type TelegramWebhookUpdate struct {
	UpdateID          int64                         `json:"update_id"`
	Message           *TelegramWebhookMessage       `json:"message"`
	EditedMessage     *TelegramWebhookMessage       `json:"edited_message"`
	ChannelPost       *TelegramWebhookMessage       `json:"channel_post"`
	EditedChannelPost *TelegramWebhookMessage       `json:"edited_channel_post"`
	CallbackQuery     *TelegramWebhookCallbackQuery `json:"callback_query"`
}

type TelegramWebhookCallbackQuery struct {
	ID      string                  `json:"id"`
	Data    string                  `json:"data"`
	From    *TelegramWebhookUser    `json:"from"`
	Message *TelegramWebhookMessage `json:"message"`
}

type TelegramWebhookMessage struct {
	MessageID int64                `json:"message_id"`
	Text      string               `json:"text"`
	Chat      TelegramWebhookChat  `json:"chat"`
	From      *TelegramWebhookUser `json:"from"`
}

type TelegramWebhookChat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Username string `json:"username"`
}

type TelegramWebhookUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

func (p *TelegramUpdateProcessor) ProcessUpdate(_ context.Context, update TelegramWebhookUpdate) error {
	chatID, chatIDPtr := telegramUpdateChatID(update)
	isNew, err := database.RecordTelegramUpdateReceipt(&database.TelegramUpdateReceipt{
		UpdateID:    update.UpdateID,
		ChatID:      chatIDPtr,
		Status:      "received",
		ProcessedAt: p.now(),
	})
	if err != nil {
		return err
	}
	if !isNew {
		return nil
	}

	token, ok := telegramStartToken(update)
	if !ok {
		metrics.TelegramConnectionFailures.WithLabelValues("missing_start_command").Inc()
		return p.reply(chatID, TelegramConnectionNoCommandMessage)
	}

	if !telegramChatTypeCanConnect(update) {
		metrics.TelegramConnectionFailures.WithLabelValues("unsupported_chat_type").Inc()
		return p.reply(chatID, TelegramConnectionInvalidChatType)
	}

	_, err = consumeAndBindTelegramConnection(HashTelegramConnectionToken(token), chatID, telegramUpdateUsername(update), update, p.now())
	if err != nil {
		if errors.Is(err, database.ErrTelegramConnectionTokenUnavailable) {
			metrics.TelegramConnectionFailures.WithLabelValues("invalid_or_expired_token").Inc()
			return p.reply(chatID, TelegramConnectionInvalidMessage)
		}
		// Transient failure (DB/network): drop the receipt so Telegram's retry
		// re-processes this update instead of having it deduped away. (TG-2)
		if delErr := database.DeleteTelegramUpdateReceipt(update.UpdateID); delErr != nil {
			log.Printf("telegram: failed to roll back update receipt %d after transient bind error: %v", update.UpdateID, delErr)
		}
		return err
	}

	metrics.TelegramConnectionTokensConsumed.Inc()

	return p.reply(chatID, TelegramConnectionSuccessMessage)
}

// effectiveMessage normalizes the several Telegram update shapes (message,
// edited_message, channel_post, edited_channel_post, callback_query) into a
// single message the connection logic can consume. Returns nil when the
// update carries no usable message.
func (update TelegramWebhookUpdate) effectiveMessage() *TelegramWebhookMessage {
	switch {
	case update.Message != nil:
		return update.Message
	case update.EditedMessage != nil:
		return update.EditedMessage
	case update.ChannelPost != nil:
		return update.ChannelPost
	case update.EditedChannelPost != nil:
		return update.EditedChannelPost
	case update.CallbackQuery != nil && update.CallbackQuery.Message != nil:
		msg := *update.CallbackQuery.Message
		if msg.From == nil {
			msg.From = update.CallbackQuery.From
		}
		return &msg
	default:
		return nil
	}
}

func consumeAndBindTelegramConnection(tokenHash string, chatID int64, username string, update TelegramWebhookUpdate, now time.Time) (*database.TelegramConnectionToken, error) {
	var consumed *database.TelegramConnectionToken
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		token, err := database.ConsumeTelegramConnectionTokenTx(tx, tokenHash, chatID, username, now)
		if err != nil {
			return err
		}
		if err := bindTelegramBusinessConnectionTx(tx, token.BusinessID, update, now); err != nil {
			return err
		}
		consumed = token
		return nil
	})
	if err == nil {
		refreshTelegramConnectedBusinessesMetric()
	}
	return consumed, err
}

func telegramChatTypeCanConnect(update TelegramWebhookUpdate) bool {
	msg := update.effectiveMessage()
	if msg == nil {
		return false
	}
	chatType := strings.ToLower(strings.TrimSpace(msg.Chat.Type))
	return chatType == "" || chatType == "private" || chatType == "group" || chatType == "supergroup"
}

func (p *TelegramUpdateProcessor) now() time.Time {
	if p.Now != nil {
		return p.Now().UTC()
	}
	return time.Now().UTC()
}

func (p *TelegramUpdateProcessor) reply(chatID int64, message string) error {
	if chatID == 0 || p.ReplySender == nil {
		return nil
	}
	if err := p.ReplySender.SendTelegramMessage(chatID, message); err != nil {
		metrics.TelegramConnectionFailures.WithLabelValues("reply_failed").Inc()
	}
	return nil
}

func telegramUpdateChatID(update TelegramWebhookUpdate) (int64, *int64) {
	msg := update.effectiveMessage()
	if msg == nil || msg.Chat.ID == 0 {
		return 0, nil
	}
	chatID := msg.Chat.ID
	return chatID, &chatID
}

func telegramStartToken(update TelegramWebhookUpdate) (string, bool) {
	msg := update.effectiveMessage()
	if msg == nil {
		return "", false
	}
	fields := strings.Fields(strings.TrimSpace(msg.Text))
	if len(fields) < 2 {
		return "", false
	}
	command := strings.ToLower(fields[0])
	if command != "/start" && !strings.HasPrefix(command, "/start@") {
		return "", false
	}
	token := strings.TrimSpace(fields[1])
	if !strings.HasPrefix(token, telegramConnectionTokenPrefix) {
		return "", false
	}
	return token, true
}

func telegramUpdateUsername(update TelegramWebhookUpdate) string {
	msg := update.effectiveMessage()
	if msg == nil {
		return ""
	}
	if msg.From != nil && strings.TrimSpace(msg.From.Username) != "" {
		return strings.TrimSpace(msg.From.Username)
	}
	return strings.TrimSpace(msg.Chat.Username)
}

func bindTelegramBusinessConnectionTx(tx *gorm.DB, businessID uint, update TelegramWebhookUpdate, now time.Time) error {
	var plugin database.Plugin
	if err := tx.Where("name = ?", "telegram").First(&plugin).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("telegram plugin is not registered: %w", err)
		}
		return fmt.Errorf("failed to load telegram plugin: %w", err)
	}

	config := telegramDefaultConnectionConfigFromTx(tx, businessID)
	var existing database.BusinessPlugin
	err := tx.Where("business_id = ? AND plugin_id = ?", businessID, plugin.ID).First(&existing).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("failed to load telegram plugin config: %w", err)
		}
	} else if strings.TrimSpace(existing.Config) != "" {
		if err := json.Unmarshal([]byte(existing.Config), &config); err != nil {
			return fmt.Errorf("failed to unmarshal telegram plugin config: %w", err)
		}
	}

	if msg := update.effectiveMessage(); msg != nil {
		config["chat_id"] = fmt.Sprintf("%d", msg.Chat.ID)
		config["chat_type"] = strings.TrimSpace(msg.Chat.Type)
		config["chat_title"] = strings.TrimSpace(msg.Chat.Title)
		config["telegram_username"] = telegramUpdateUsername(update)
	}
	config["is_connected"] = true
	config["connected_at"] = now
	config["last_activity"] = now
	config["last_error"] = ""
	config["last_error_at"] = nil
	config["failure_count"] = 0

	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal telegram plugin config: %w", err)
	}

	businessPlugin := database.BusinessPlugin{
		BusinessID: businessID,
		PluginID:   plugin.ID,
		IsEnabled:  true,
		Config:     string(configJSON),
	}
	if err := tx.Where("business_id = ? AND plugin_id = ?", businessID, plugin.ID).
		Assign(database.BusinessPlugin{IsEnabled: true, Config: string(configJSON)}).
		FirstOrCreate(&businessPlugin).Error; err != nil {
		return fmt.Errorf("failed to enable telegram plugin: %w", err)
	}
	return nil
}

func refreshTelegramConnectedBusinessesMetric() {
	count, err := database.CountConnectedTelegramBusinesses()
	if err != nil {
		return
	}
	metrics.TelegramConnectedBusinesses.Set(float64(count))
}

func telegramDefaultConnectionConfigFromTx(tx *gorm.DB, businessID uint) map[string]interface{} {
	businessName := "Your Business"
	var business database.Business
	if err := tx.First(&business, businessID).Error; err == nil && strings.TrimSpace(business.Name) != "" {
		businessName = business.Name
	}
	return telegramDefaultConnectionConfigForBusinessName(businessName)
}

func telegramDefaultConnectionConfigForBusinessName(businessName string) map[string]interface{} {
	if strings.TrimSpace(businessName) == "" {
		businessName = "Your Business"
	}
	return map[string]interface{}{
		"business_name": businessName,
		"notification_settings": map[string]interface{}{
			"order_notifications":   true,
			"payment_notifications": true,
			"daily_summary":         true,
			"weekly_summary":        false,
			"monthly_summary":       false,
			"low_stock_alerts":      false,
			"new_customer_alerts":   false,
			"high_value_orders":     false,
		},
		// Only events the Telegram tier can actually render are advertised here.
		// order.status_changed is intentionally omitted: it has no renderer (it
		// was stopped from enqueuing in a prior fix), so seeding it true would
		// advertise a toggle that can never deliver. See telegramEventPreferenceKeys.
		"notifications": map[string]interface{}{
			"order_created":              true,
			"payment_received":           true,
			"reservation_created":        true,
			"reservation_status_changed": true,
			"low_inventory":              false,
			"daily_summary":              false,
		},
	}
}
