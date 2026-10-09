package services

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/metrics"
)

// pluginDeliveryDisabled tracks plugins whose delivery worker is NOT running, so
// EnqueuePluginNotification can avoid accumulating dead outbox rows that nothing
// will ever consume ("don't enqueue into the void"). Delivery is enabled by
// default — only main.go explicitly disables a plugin when its worker is not
// started (e.g. Telegram with no bot token). Tests and all other plugins are
// therefore unaffected.
var pluginDeliveryDisabled sync.Map // pluginName -> struct{}

// SetPluginDeliveryEnabled records whether a plugin's delivery path is live.
// Called once from main.go after deciding whether the plugin's worker started.
func SetPluginDeliveryEnabled(pluginName string, enabled bool) {
	pluginName = strings.TrimSpace(pluginName)
	if pluginName == "" {
		return
	}
	if enabled {
		pluginDeliveryDisabled.Delete(pluginName)
		if pluginName == "telegram" {
			metrics.TelegramDeliveryAvailable.Set(1)
		}
		return
	}
	pluginDeliveryDisabled.Store(pluginName, struct{}{})
	if pluginName == "telegram" {
		metrics.TelegramDeliveryAvailable.Set(0)
	}
}

// PluginDeliveryEnabled reports whether enqueued notifications for a plugin have
// a consumer. Defaults to true unless explicitly disabled.
func PluginDeliveryEnabled(pluginName string) bool {
	_, disabled := pluginDeliveryDisabled.Load(strings.TrimSpace(pluginName))
	return !disabled
}

// EnqueuePluginNotification writes an outbox row on its own (autocommit) DB
// handle. Use this for call sites that have no ambient domain transaction.
func EnqueuePluginNotification(event PluginNotificationEvent, pluginName string) (*database.PluginNotificationDelivery, bool, error) {
	return EnqueuePluginNotificationTx(nil, event, pluginName)
}

// EnqueuePluginNotificationTx writes the outbox row on the caller's domain-write
// transaction so the notification commits atomically with the domain change —
// eliminating the crash window between the domain commit and a separate outbox
// insert (N-1). Pass tx == nil to enqueue on the package-level DB handle (the
// EnqueuePluginNotification behavior, for call sites without a transaction).
func EnqueuePluginNotificationTx(tx *gorm.DB, event PluginNotificationEvent, pluginName string) (*database.PluginNotificationDelivery, bool, error) {
	pluginName = strings.TrimSpace(pluginName)
	if event.BusinessID == 0 {
		return nil, false, errors.New("business ID is required")
	}
	if pluginName == "" {
		return nil, false, errors.New("plugin name is required")
	}
	if strings.TrimSpace(event.EventType) == "" {
		return nil, false, errors.New("event type is required")
	}
	if strings.TrimSpace(event.EventID) == "" {
		return nil, false, errors.New("event ID is required")
	}
	if !pluginNotificationEventEnabled(pluginName, event.EventType) {
		return nil, false, nil
	}
	// Don't enqueue into the void: if no delivery worker is running for this
	// plugin, dropping the row on the floor here keeps the outbox bounded
	// instead of growing a backlog nothing will ever attempt. The skip is
	// visible: a labelled counter plus a warning so a missing bot token is
	// never a silent no-op.
	if !PluginDeliveryEnabled(pluginName) {
		metrics.PluginNotificationDropped.WithLabelValues(pluginName, event.EventType, "delivery_disabled").Inc()
		logger.Logger.Warnf("plugin notification skipped: delivery disabled (plugin=%s event_type=%s event_id=%s business_id=%d)",
			pluginName, event.EventType, event.EventID, event.BusinessID)
		return nil, false, nil
	}

	now := event.CreatedAt
	if now.IsZero() {
		now = time.Now().UTC()
	}
	payload := event.Payload
	if payload == nil {
		payload = map[string]interface{}{}
	}

	delivery := &database.PluginNotificationDelivery{
		BusinessID:    event.BusinessID,
		PluginName:    pluginName,
		EventType:     event.EventType,
		EventID:       event.EventID,
		Status:        database.PluginNotificationDeliveryStatusPending,
		Payload:       payload,
		NextAttemptAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	var (
		createdDelivery *database.PluginNotificationDelivery
		created         bool
		err             error
	)
	if tx != nil {
		createdDelivery, created, err = database.CreatePluginNotificationDeliveryTx(tx, delivery)
	} else {
		createdDelivery, created, err = database.CreatePluginNotificationDelivery(delivery)
	}
	if err != nil {
		return nil, false, fmt.Errorf("failed to enqueue plugin notification: %w", err)
	}
	if created {
		metrics.PluginNotificationEnqueued.WithLabelValues(pluginName, event.EventType).Inc()
	}
	return createdDelivery, created, nil
}

func pluginNotificationEventEnabled(pluginName string, eventType string) bool {
	if strings.TrimSpace(pluginName) != "telegram" {
		return true
	}
	raw := strings.TrimSpace(os.Getenv("TELEGRAM_NOTIFICATION_EVENTS"))
	if raw == "" || raw == "*" {
		return true
	}
	for _, item := range strings.Split(raw, ",") {
		if strings.TrimSpace(item) == eventType {
			return true
		}
	}
	return false
}
