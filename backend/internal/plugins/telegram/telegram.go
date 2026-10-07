package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/plugins"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// TelegramPlugin implements the Plugin and NotificationPlugin interfaces
type TelegramPlugin struct {
	pluginService *services.PluginService
	bot           *tgbotapi.BotAPI
	botSender     telegramBotSender
}

// Compile-time proof that TelegramPlugin satisfies the integration contract, so
// a drift in either the interface or these method signatures fails the build
// instead of silently dropping Telegram from GetIntegrationPlugins discovery.
var _ plugins.IntegrationPlugin = (*TelegramPlugin)(nil)

type telegramBotSender interface {
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
}

// NotificationSettings represents the notification preferences for Telegram
type NotificationSettings struct {
	OrderNotifications   bool `json:"order_notifications"`
	PaymentNotifications bool `json:"payment_notifications"`
	DailySummary         bool `json:"daily_summary"`
	WeeklySummary        bool `json:"weekly_summary"`
	MonthlySummary       bool `json:"monthly_summary"`
	LowStockAlerts       bool `json:"low_stock_alerts"`
	NewCustomerAlerts    bool `json:"new_customer_alerts"`
	HighValueOrders      bool `json:"high_value_orders"`
}

// TelegramConfig represents the configuration for Telegram plugin
type TelegramConfig struct {
	ChatID               string               `json:"chat_id"`
	BusinessName         string               `json:"business_name"`
	NotificationSettings NotificationSettings `json:"notification_settings"`
	IsConnected          bool                 `json:"is_connected"`
	ConnectedAt          time.Time            `json:"connected_at,omitempty"`
	LastActivity         time.Time            `json:"last_activity,omitempty"`
}

// NewTelegramPlugin creates a new Telegram plugin instance
func NewTelegramPlugin(pluginService *services.PluginService, bot *tgbotapi.BotAPI) *TelegramPlugin {
	plugin := &TelegramPlugin{
		pluginService: pluginService,
		bot:           bot,
	}
	if bot != nil {
		plugin.botSender = bot
	}
	return plugin
}

// Plugin interface implementation

func (tp *TelegramPlugin) GetName() string {
	return "telegram"
}

func (tp *TelegramPlugin) GetDisplayName() string {
	return "Telegram Notifications"
}

func (tp *TelegramPlugin) GetDescription() string {
	return "Send real-time notifications to Telegram for orders, payments, daily summaries, and business alerts. Stay connected with your business 24/7."
}

func (tp *TelegramPlugin) GetCategory() string {
	return "integration"
}

func (tp *TelegramPlugin) GetVersion() string {
	return "1.0.0"
}

func (tp *TelegramPlugin) GetFeatures() string {
	features := []string{
		"Real-time order notifications",
		"Payment confirmations",
		"Daily/Weekly/Monthly summaries",
		"Low stock alerts",
		"New customer notifications",
		"High-value order alerts",
		"Business performance insights",
		"Secure bot connection",
	}
	featuresJSON, _ := json.Marshal(features)
	return string(featuresJSON)
}

func (tp *TelegramPlugin) IsActive() bool {
	return true
}

func (tp *TelegramPlugin) GetConfigSchema() string {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"chat_id": map[string]interface{}{
				"type":        "string",
				"title":       "Chat ID",
				"description": "Your Telegram chat ID (will be set automatically when you connect)",
				"readOnly":    true,
			},
			"business_name": map[string]interface{}{
				"type":        "string",
				"title":       "Business Name",
				"description": "Name to display in notifications",
				"maxLength":   100,
			},
			"notification_settings": map[string]interface{}{
				"type":  "object",
				"title": "Notification Preferences",
				"properties": map[string]interface{}{
					"order_notifications": map[string]interface{}{
						"type":        "boolean",
						"title":       "Order Notifications",
						"description": "Receive notifications for new orders",
						"default":     true,
					},
					"payment_notifications": map[string]interface{}{
						"type":        "boolean",
						"title":       "Payment Notifications",
						"description": "Receive notifications for payments",
						"default":     true,
					},
					"daily_summary": map[string]interface{}{
						"type":        "boolean",
						"title":       "Daily Summary",
						"description": "Receive daily business summary",
						"default":     true,
					},
					"weekly_summary": map[string]interface{}{
						"type":        "boolean",
						"title":       "Weekly Summary",
						"description": "Receive weekly business summary",
						"default":     false,
					},
					"monthly_summary": map[string]interface{}{
						"type":        "boolean",
						"title":       "Monthly Summary",
						"description": "Receive monthly business summary",
						"default":     false,
					},
					"low_stock_alerts": map[string]interface{}{
						"type":        "boolean",
						"title":       "Low Stock Alerts",
						"description": "Get notified when items are running low",
						"default":     false,
					},
					"new_customer_alerts": map[string]interface{}{
						"type":        "boolean",
						"title":       "New Customer Alerts",
						"description": "Get notified about new customers",
						"default":     false,
					},
					// Currency-neutral copy on purpose: the venue's currency is
					// whatever `businesses.display_currency`/`default_currency`
					// says (ARS, EUR, …), and there is no USD threshold constant
					// behind this toggle. Naming a "$100" figure mis-states the
					// feature on every non-USD carta (#901).
					"high_value_orders": map[string]interface{}{
						"type":        "boolean",
						"title":       "High Value Orders",
						"description": "Get notified about unusually large orders",
						"default":     false,
					},
				},
			},
			// Workforce alerts route to the operator's Telegram chat via the
			// primary `notifications` preference map read by the notification
			// gate (services.telegramEventPreferenceKeys). They are opt-in
			// (default off): they duplicate the staff app's push/inbox channel,
			// so they only fire when an operator explicitly turns them on.
			"notifications": map[string]interface{}{
				"type":  "object",
				"title": "Workforce Alerts",
				"properties": map[string]interface{}{
					"schedule_published": map[string]interface{}{
						"type":        "boolean",
						"title":       "Schedule published",
						"description": "Get notified when a new staff schedule is published",
						"default":     false,
					},
					"shift_reminder": map[string]interface{}{
						"type":        "boolean",
						"title":       "Shift reminders",
						"description": "Get reminders before staff shifts start",
						"default":     false,
					},
					"coverage_decided": map[string]interface{}{
						"type":        "boolean",
						"title":       "Coverage decisions",
						"description": "Get notified when a shift coverage request is approved or denied",
						"default":     false,
					},
				},
			},
		},
		"required": []string{"business_name"},
	}

	schemaJSON, _ := json.Marshal(schema)
	return string(schemaJSON)
}

func (tp *TelegramPlugin) ValidateConfig(config map[string]interface{}) error {
	// Validate business name
	businessName, ok := config["business_name"].(string)
	if !ok || businessName == "" {
		return errors.New("business_name is required")
	}

	if len(businessName) > 100 {
		return errors.New("business_name must be 100 characters or less")
	}

	// Validate notification settings if provided
	if notificationSettings, exists := config["notification_settings"]; exists {
		if _, ok := notificationSettings.(map[string]interface{}); !ok {
			return errors.New("notification_settings must be an object")
		}
	}

	return nil
}

func (tp *TelegramPlugin) Initialize(businessID uint, config map[string]interface{}) error {
	// Validate configuration first
	if err := tp.ValidateConfig(config); err != nil {
		return fmt.Errorf("telegram plugin initialization failed: %v", err)
	}

	return nil
}

func (tp *TelegramPlugin) Cleanup(businessID uint) error {
	// For Telegram, we might want to:
	// 1. Send a disconnection message to the user
	// 2. Clear the chat ID from configuration
	// 3. Log the disconnection

	config, err := tp.pluginService.GetPluginConfig(businessID, tp.GetName())
	if err != nil {
		return nil // Plugin wasn't configured, nothing to clean up
	}

	// Send disconnection message if chat ID exists
	if chatIDStr, exists := config["chat_id"].(string); exists && chatIDStr != "" {
		if chatID, err := tp.parseChatID(chatIDStr); err == nil {
			tp.sendDisconnectionMessage(chatID, businessID, config)
		}
	}

	return nil
}

// Telegram-specific methods

// ConnectBusiness connects a business to Telegram using a chat ID
func (tp *TelegramPlugin) ConnectBusiness(businessID uint, chatID int64, businessName string) error {
	if !tp.pluginService.IsPluginActive(businessID, tp.GetName()) {
		return errors.New("telegram plugin not enabled for this business")
	}

	// Get current config or create new one
	config, err := tp.pluginService.GetPluginConfig(businessID, tp.GetName())
	if err != nil {
		// Create default config
		config = map[string]interface{}{
			"business_name": businessName,
			"notification_settings": NotificationSettings{
				OrderNotifications:   true,
				PaymentNotifications: true,
				DailySummary:         true,
				WeeklySummary:        false,
				MonthlySummary:       false,
				LowStockAlerts:       false,
				NewCustomerAlerts:    false,
				HighValueOrders:      false,
			},
		}
	}

	// Update config with connection info
	config["chat_id"] = fmt.Sprintf("%d", chatID)
	config["is_connected"] = true
	config["connected_at"] = time.Now()
	config["last_activity"] = time.Now()

	// Update business name if provided
	if businessName != "" {
		config["business_name"] = businessName
	}

	// Save updated config
	if err := tp.pluginService.UpdatePluginConfig(businessID, tp.GetName(), config); err != nil {
		return fmt.Errorf("failed to update telegram config: %w", err)
	}

	// Send welcome message
	tp.sendWelcomeMessage(chatID, businessID, businessName)

	log.Printf("Telegram plugin connected for business %d with chat ID %d", businessID, chatID)
	return nil
}

// SendNotification sends a notification to the connected Telegram chat
func (tp *TelegramPlugin) SendNotification(businessID uint, notificationType string, message string, data map[string]interface{}) error {
	if !tp.pluginService.IsPluginActive(businessID, tp.GetName()) {
		return errors.New("telegram plugin not enabled for this business")
	}

	config, err := tp.pluginService.GetPluginConfig(businessID, tp.GetName())
	if err != nil {
		return fmt.Errorf("failed to get telegram config: %w", err)
	}

	// Check if connected
	isConnected, _ := config["is_connected"].(bool)
	if !isConnected {
		return errors.New("telegram not connected for this business")
	}

	// Get chat ID
	chatIDStr, exists := config["chat_id"].(string)
	if !exists || chatIDStr == "" {
		return errors.New("no chat ID configured")
	}

	chatID, err := tp.parseChatID(chatIDStr)
	if err != nil {
		return fmt.Errorf("invalid chat ID: %w", err)
	}

	// Check notification preferences
	if !tp.shouldSendNotification(config, notificationType) {
		return nil // Notification disabled for this type
	}

	// Format and send message in the business's language.
	businessName, _ := config["business_name"].(string)
	_, _, language := tp.businessNotificationContext(businessID)
	formattedMessage := services.RenderTelegramTestMessage(businessName, message, language, time.Now())
	if err := tp.sendMessage(chatID, formattedMessage); err != nil {
		return err
	}
	if notificationType == "test" {
		// Narrow merge (not a whole-blob RMW) so a concurrent reconnect's chat_id
		// is never clobbered by this stale copy (N-5).
		if err := tp.pluginService.MergePluginConfigFields(businessID, tp.GetName(),
			database.MergeBusinessPluginConfigField{Key: "last_test_sent_at", Value: time.Now().UTC()},
			database.MergeBusinessPluginConfigField{Key: "last_error", Value: ""},
			database.MergeBusinessPluginConfigField{Key: "last_error_at", Value: nil},
		); err != nil {
			log.Printf("telegram: failed to record test-sent state for business %d: %v", businessID, err)
		}
	}
	return nil
}

// GetConnectionStatus returns the connection status for a business
func (tp *TelegramPlugin) GetConnectionStatus(businessID uint) (map[string]interface{}, error) {
	now := time.Now().UTC()
	pendingToken, pendingErr := database.GetPendingTelegramConnectionToken(businessID, now)
	queueDepth := telegramDeliveryQueueDepth(now)

	config, businessPluginEnabled, platformPluginActive, stateErr := database.GetBusinessPluginConfigState(businessID, tp.GetName())
	if stateErr != nil || !businessPluginEnabled || !platformPluginActive {
		health := "not_connected"
		var pendingExpiresAt interface{}
		if stateErr == nil && (!businessPluginEnabled || !platformPluginActive) {
			health = "disabled"
		} else if pendingErr == nil && pendingToken != nil {
			health = "pending"
			pendingExpiresAt = pendingToken.ExpiresAt
		}
		return applyTelegramPlatformAvailability(map[string]interface{}{
			"connected":                false,
			"plugin_enabled":           businessPluginEnabled && platformPluginActive,
			"health":                   health,
			"pending_token_expires_at": pendingExpiresAt,
			"delivery_queue_depth":     queueDepth,
		}), nil
	}

	isConnected, _ := config["is_connected"].(bool)
	chatID, _ := config["chat_id"].(string)
	businessName, _ := config["business_name"].(string)
	chatType, _ := config["chat_type"].(string)
	chatTitle, _ := config["chat_title"].(string)
	telegramUsername, _ := config["telegram_username"].(string)
	lastError, _ := config["last_error"].(string)
	failureCount := normalizeTelegramFailureCount(config["failure_count"])
	connectedAt := parseTelegramConfigTime(config["connected_at"])
	lastActivity := parseTelegramConfigTime(config["last_activity"])
	lastSentAt := parseTelegramConfigTime(config["last_sent_at"])
	lastTestSentAt := parseTelegramConfigTime(config["last_test_sent_at"])
	lastErrorAt := parseTelegramConfigTime(config["last_error_at"])

	health := "not_connected"
	if isConnected {
		health = "connected"
		if strings.TrimSpace(lastError) != "" {
			health = "degraded"
		}
	} else if pendingErr == nil && pendingToken != nil {
		health = "pending"
	}

	var pendingExpiresAt interface{}
	if pendingErr == nil && pendingToken != nil {
		pendingExpiresAt = pendingToken.ExpiresAt
	}

	return applyTelegramPlatformAvailability(map[string]interface{}{
		"connected":                isConnected,
		"plugin_enabled":           true,
		"health":                   health,
		"chat_id":                  chatID,
		"chat_type":                chatType,
		"chat_title":               chatTitle,
		"telegram_username":        telegramUsername,
		"business_name":            businessName,
		"connected_at":             connectedAt,
		"last_activity":            lastActivity,
		"last_sent_at":             lastSentAt,
		"last_test_sent_at":        lastTestSentAt,
		"last_error":               lastError,
		"last_error_at":            lastErrorAt,
		"failure_count":            failureCount,
		"pending_token_expires_at": pendingExpiresAt,
		"delivery_queue_depth":     queueDepth,
	}), nil
}

// applyTelegramPlatformAvailability overlays process-level delivery
// availability so a business cannot present as connected when the bot token
// is absent and the worker will drop every notification.
func applyTelegramPlatformAvailability(status map[string]interface{}) map[string]interface{} {
	available := services.PluginDeliveryEnabled("telegram")
	status["delivery_available"] = available
	if !available {
		status["connected"] = false
		status["health"] = "disabled"
	}
	return status
}

func telegramDeliveryQueueDepth(now time.Time) int64 {
	queueDepth, err := database.CountPluginNotificationBacklog("telegram", now)
	if err != nil {
		return 0
	}
	return queueDepth
}

func normalizeTelegramFailureCount(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func parseTelegramConfigTime(value interface{}) interface{} {
	switch typed := value.(type) {
	case time.Time:
		if typed.IsZero() {
			return nil
		}
		return typed
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil
		}
		parsed, err := time.Parse(time.RFC3339Nano, trimmed)
		if err != nil {
			return nil
		}
		return parsed
	default:
		return nil
	}
}

// Helper methods

func (tp *TelegramPlugin) parseChatID(chatIDStr string) (int64, error) {
	var chatID int64
	if _, err := fmt.Sscanf(chatIDStr, "%d", &chatID); err != nil {
		return 0, err
	}
	return chatID, nil
}

func (tp *TelegramPlugin) shouldSendNotification(config map[string]interface{}, notificationType string) bool {
	notificationSettings, exists := config["notification_settings"]
	if !exists {
		return true // Default to sending if no settings
	}

	settings, ok := notificationSettings.(map[string]interface{})
	if !ok {
		return true
	}

	switch notificationType {
	case "order":
		if enabled, ok := settings["order_notifications"].(bool); ok {
			return enabled
		}
	case "payment":
		if enabled, ok := settings["payment_notifications"].(bool); ok {
			return enabled
		}
	case "daily_summary":
		if enabled, ok := settings["daily_summary"].(bool); ok {
			return enabled
		}
	case "weekly_summary":
		if enabled, ok := settings["weekly_summary"].(bool); ok {
			return enabled
		}
	case "monthly_summary":
		if enabled, ok := settings["monthly_summary"].(bool); ok {
			return enabled
		}
	case "low_stock":
		if enabled, ok := settings["low_stock_alerts"].(bool); ok {
			return enabled
		}
	case "new_customer":
		if enabled, ok := settings["new_customer_alerts"].(bool); ok {
			return enabled
		}
	case "high_value_order":
		if enabled, ok := settings["high_value_orders"].(bool); ok {
			return enabled
		}
	}

	return true // Default to sending
}

func (tp *TelegramPlugin) sendMessage(chatID int64, message string) error {
	sender := tp.messageSender()
	if sender == nil {
		return errors.New("telegram bot not initialized")
	}

	msg := tgbotapi.NewMessage(chatID, message)
	msg.ParseMode = "Markdown"

	_, err := sender.Send(msg)
	if err != nil {
		log.Printf("Failed to send Telegram message to chat %d: %v", chatID, err)
		return fmt.Errorf("failed to send telegram message: %w", err)
	}

	return nil
}

func (tp *TelegramPlugin) SendTelegramMessage(chatID int64, message string) error {
	return tp.sendMessage(chatID, message)
}

func (tp *TelegramPlugin) SendPluginNotification(ctx context.Context, delivery database.PluginNotificationDelivery) services.PluginNotificationSendResult {
	startedAt := time.Now()
	config, err := tp.connectedConfig(delivery.BusinessID)
	if err != nil {
		result := services.PluginNotificationSendResult{
			Drop:         true,
			ErrorCode:    "telegram_not_connected",
			ErrorMessage: err.Error(),
		}
		tp.recordTelegramSendMetrics(startedAt, "dropped", result.ErrorCode)
		return result
	}

	chatID, err := tp.parseChatID(fmt.Sprintf("%v", config["chat_id"]))
	if err != nil {
		result := services.PluginNotificationSendResult{
			Drop:         true,
			ErrorCode:    "telegram_invalid_chat_id",
			ErrorMessage: err.Error(),
		}
		tp.recordTelegramSendMetrics(startedAt, "dropped", result.ErrorCode)
		return result
	}

	if !services.TelegramEventNotificationEnabled(config, delivery.EventType) {
		result := services.PluginNotificationSendResult{
			Drop:         true,
			ErrorCode:    "telegram_notification_disabled",
			ErrorMessage: "telegram notification disabled for event type",
		}
		tp.recordTelegramSendMetrics(startedAt, "dropped", result.ErrorCode)
		return result
	}

	timezone, currency, language := tp.businessNotificationContext(delivery.BusinessID)
	message, err := services.RenderTelegramNotification(services.PluginNotificationEvent{
		BusinessID: delivery.BusinessID,
		EventType:  delivery.EventType,
		EventID:    delivery.EventID,
		Payload:    delivery.Payload,
		CreatedAt:  delivery.CreatedAt,
	}, timezone, currency, language)
	if err != nil {
		result := services.PluginNotificationSendResult{
			PermanentFailure: true,
			ErrorCode:        "telegram_render_failed",
			ErrorMessage:     err.Error(),
			Err:              err,
		}
		tp.recordTelegramSendMetrics(startedAt, "failed", result.ErrorCode)
		tp.recordTelegramSendFailure(delivery.BusinessID, result)
		return result
	}

	message = truncateTelegramMessage(message)
	metrics.TelegramMessageLength.Observe(float64(len(message)))
	providerMessageID, err := tp.sendHTMLMessage(ctx, chatID, message)
	if err != nil {
		result := classifyTelegramSendError(err)
		tp.recordTelegramSendMetrics(startedAt, "failed", result.ErrorCode)
		tp.recordTelegramSendFailure(delivery.BusinessID, result)
		return result
	}

	result := services.PluginNotificationSendResult{ProviderMessageID: providerMessageID}
	tp.recordTelegramSendMetrics(startedAt, "sent", "ok")
	tp.recordTelegramSendSuccess(delivery.BusinessID)
	return result
}

func (tp *TelegramPlugin) recordTelegramSendMetrics(startedAt time.Time, status string, reason string) {
	status = strings.TrimSpace(status)
	if status == "" {
		status = "unknown"
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "unknown"
	}
	metrics.TelegramSendAttempts.WithLabelValues(status, reason).Inc()
	metrics.TelegramSendDuration.WithLabelValues(status, reason).Observe(time.Since(startedAt).Seconds())
}

func (tp *TelegramPlugin) connectedConfig(businessID uint) (map[string]interface{}, error) {
	if tp.pluginService == nil || !tp.pluginService.IsPluginActive(businessID, tp.GetName()) {
		return nil, errors.New("telegram plugin not enabled for this business")
	}
	config, err := tp.pluginService.GetPluginConfig(businessID, tp.GetName())
	if err != nil {
		return nil, err
	}
	isConnected, _ := config["is_connected"].(bool)
	if !isConnected {
		return nil, errors.New("telegram not connected for this business")
	}
	if strings.TrimSpace(fmt.Sprintf("%v", config["chat_id"])) == "" {
		return nil, errors.New("telegram chat ID is missing")
	}
	return config, nil
}

// businessNotificationContext loads the rendering context for a business's
// Telegram notifications in a single DB read: timezone, display currency, and
// the operator's language (business.DefaultLanguage, threaded into
// RenderTelegramNotification for localization). On load failure it returns
// neutral defaults and an empty language, which the renderer resolves to English.
func (tp *TelegramPlugin) businessNotificationContext(businessID uint) (string, string, string) {
	business, err := database.GetBusinessByID(businessID)
	if err != nil {
		return "UTC", "USD", ""
	}
	timezone := strings.TrimSpace(business.Timezone)
	if timezone == "" {
		timezone = "UTC"
	}
	currency := strings.TrimSpace(business.DisplayCurrency)
	if currency == "" {
		currency = strings.TrimSpace(business.DefaultCurrency)
	}
	if currency == "" {
		currency = "USD"
	}
	return timezone, currency, business.DefaultLanguage
}

// telegramSendTimeout is the hard upper bound on a single Telegram API send.
// It must stay well below the worker's 2-minute stale-processing reclaim: a
// hung HTTP call that outlives the reclaim window gets its row reclaimed and
// redelivered — a duplicate message.
const telegramSendTimeout = 30 * time.Second

func (tp *TelegramPlugin) sendHTMLMessage(ctx context.Context, chatID int64, message string) (string, error) {
	sender := tp.messageSender()
	if sender == nil {
		return "", errors.New("telegram bot not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, telegramSendTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// The tgbotapi client offers no per-request context, so run the send in a
	// goroutine and race it against ctx. On timeout/cancel the goroutine is
	// abandoned (buffered channel — no leak of the result write); the worker
	// regains control instead of stalling past the reclaim window.
	type sendOutcome struct {
		sent tgbotapi.Message
		err  error
	}
	outcome := make(chan sendOutcome, 1)
	go func() {
		msg := tgbotapi.NewMessage(chatID, message)
		msg.ParseMode = "HTML"
		sent, err := sender.Send(msg)
		outcome <- sendOutcome{sent: sent, err: err}
	}()

	select {
	case <-ctx.Done():
		return "", fmt.Errorf("telegram send aborted: %w", ctx.Err())
	case out := <-outcome:
		if out.err != nil {
			return "", out.err
		}
		if out.sent.MessageID == 0 {
			return "", nil
		}
		return strconv.Itoa(out.sent.MessageID), nil
	}
}

func (tp *TelegramPlugin) messageSender() telegramBotSender {
	if tp.botSender != nil {
		return tp.botSender
	}
	if tp.bot != nil {
		return tp.bot
	}
	return nil
}

func truncateTelegramMessage(message string) string {
	const maxTelegramMessageLength = 4096
	const truncateAt = 3900
	if len(message) <= maxTelegramMessageLength {
		return message
	}
	// Rune-safe cut: a byte slice at 3900 can split a multi-byte UTF-8
	// character, which Telegram rejects with a 400 → 5 futile retries →
	// dropped message. Walk back to a rune start first.
	cut := truncateAt
	for cut > 0 && !utf8.RuneStart(message[cut]) {
		cut--
	}
	head := message[:cut]
	// Don't cut inside an HTML entity ("&amp;" → dangling "&a") or tag
	// ("<b>" → dangling "<b"); either is a Telegram HTML parse error. If the
	// tail after the last '&'/'<' is unterminated, cut before it. Entities and
	// tags are short, so this only ever trims a few extra bytes.
	if i := strings.LastIndexByte(head, '&'); i >= 0 && !strings.Contains(head[i:], ";") {
		head = head[:i]
	}
	if i := strings.LastIndexByte(head, '<'); i >= 0 && !strings.Contains(head[i:], ">") {
		head = head[:i]
	}
	return strings.TrimSpace(head) + "\n\n[truncated]"
}

func classifyTelegramSendError(err error) services.PluginNotificationSendResult {
	message := err.Error()
	normalized := strings.ToLower(message)
	result := services.PluginNotificationSendResult{
		Err:          err,
		ErrorCode:    "telegram_send_failed",
		ErrorMessage: message,
	}
	if strings.Contains(normalized, "too many requests") || strings.Contains(normalized, "retry after") || strings.Contains(normalized, "429") {
		result.ErrorCode = "telegram_rate_limited"
		result.RetryAfter = parseTelegramRetryAfter(normalized)
		return result
	}
	if strings.Contains(normalized, "forbidden") || strings.Contains(normalized, "bot was blocked") {
		result.PermanentFailure = true
		result.ErrorCode = "telegram_forbidden"
		return result
	}
	if strings.Contains(normalized, "chat not found") || strings.Contains(normalized, "invalid chat") {
		result.PermanentFailure = true
		result.ErrorCode = "telegram_invalid_chat"
		return result
	}
	return result
}

func parseTelegramRetryAfter(message string) time.Duration {
	re := regexp.MustCompile(`retry after\s+(\d+)`)
	matches := re.FindStringSubmatch(message)
	if len(matches) != 2 {
		return 0
	}
	seconds, err := strconv.Atoi(matches[1])
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

// recordTelegramSendSuccess narrows the write to only the sender-owned status
// fields via an atomic merge, so it never resurrects a stale chat_id an operator
// changed concurrently (N-5).
func (tp *TelegramPlugin) recordTelegramSendSuccess(businessID uint) {
	if err := tp.pluginService.MergePluginConfigFields(businessID, tp.GetName(),
		database.MergeBusinessPluginConfigField{Key: "last_sent_at", Value: time.Now().UTC()},
		database.MergeBusinessPluginConfigField{Key: "last_error", Value: ""},
		database.MergeBusinessPluginConfigField{Key: "last_error_at", Value: nil},
		database.MergeBusinessPluginConfigField{Key: "failure_count", Value: 0},
	); err != nil {
		log.Printf("telegram: failed to record send success for business %d: %v", businessID, err)
	}
}

func (tp *TelegramPlugin) recordTelegramSendFailure(businessID uint, result services.PluginNotificationSendResult) {
	// The failure counter is best-effort telemetry; read the current value for
	// the increment base but write only the sender-owned fields via an atomic
	// merge so a concurrent reconnect's chat_id is never clobbered (N-5).
	failureCount := 1
	if config, err := tp.pluginService.GetPluginConfig(businessID, tp.GetName()); err == nil {
		failureCount = normalizeTelegramFailureCount(config["failure_count"]) + 1
	}
	if err := tp.pluginService.MergePluginConfigFields(businessID, tp.GetName(),
		database.MergeBusinessPluginConfigField{Key: "last_error", Value: result.ErrorMessage},
		database.MergeBusinessPluginConfigField{Key: "last_error_at", Value: time.Now().UTC()},
		database.MergeBusinessPluginConfigField{Key: "failure_count", Value: failureCount},
	); err != nil {
		log.Printf("telegram: failed to record send failure for business %d: %v", businessID, err)
	}
}

func (tp *TelegramPlugin) sendWelcomeMessage(chatID int64, businessID uint, businessName string) {
	_, _, language := tp.businessNotificationContext(businessID)
	message := services.RenderTelegramWelcomeMessage(businessName, language)

	if err := tp.sendMessage(chatID, message); err != nil {
		log.Printf("WARNING: failed to send Telegram connection message: %v", err)
	}
}

func (tp *TelegramPlugin) sendDisconnectionMessage(chatID int64, businessID uint, config map[string]interface{}) {
	businessName, _ := config["business_name"].(string)
	_, _, language := tp.businessNotificationContext(businessID)
	message := services.RenderTelegramDisconnectMessage(businessName, language)

	if err := tp.sendMessage(chatID, message); err != nil {
		log.Printf("WARNING: failed to send Telegram disconnection message: %v", err)
	}
}

// Register the plugin
func init() {
	plugins.RegisterPluginInitializer("telegram", func(pluginService *services.PluginService) {
		telegramPlugin := NewTelegramPlugin(pluginService, nil)
		plugins.GlobalRegistry.RegisterPlugin(telegramPlugin)
	})
}
