package services

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

const telegramNotificationEligibilityTTL = 5 * time.Second

type telegramNotificationEligibility struct {
	eligible bool
	config   map[string]interface{}
	cachedAt time.Time
}

var (
	telegramNotificationEligibilityMu    sync.RWMutex
	telegramNotificationEligibilityCache = make(map[uint]*telegramNotificationEligibility)
)

func init() {
	database.RegisterOnDBChange(ResetTelegramNotificationEligibilityCache)
}

// ShouldEnqueueTelegramNotification reports whether the business has a
// connected Telegram config and the event is enabled for that config.
// Businesses without Telegram configured can skip the outbox write entirely.
func ShouldEnqueueTelegramNotification(businessID uint, eventType string) bool {
	if businessID == 0 || strings.TrimSpace(eventType) == "" {
		return false
	}

	if entry := cachedTelegramNotificationEligibility(businessID); entry != nil {
		return entry.eligible && TelegramEventNotificationEnabled(entry.config, eventType)
	}

	eligible, config, err := loadTelegramNotificationEligibility(businessID)
	if err != nil {
		// Fail closed (X-8): an unexpected lookup error must not enqueue a
		// notification we cannot verify is wanted — otherwise the outbox
		// janitor/TTL has to reap undeliverable rows. Skip and log.
		log.Printf("telegram gate: eligibility lookup failed, skipping enqueue: business_id=%d event=%s error=%v",
			businessID, eventType, err)
		return false
	}

	storeTelegramNotificationEligibility(businessID, eligible, config)
	return eligible && TelegramEventNotificationEnabled(config, eventType)
}

func loadTelegramNotificationEligibility(businessID uint) (bool, map[string]interface{}, error) {
	config, isEnabled, platformIsActive, err := database.GetBusinessPluginConfigState(businessID, "telegram")
	if err != nil {
		if errors.Is(err, database.ErrBusinessPluginNotEnabled) {
			return false, nil, nil
		}
		return false, nil, fmt.Errorf("load telegram config: %w", err)
	}
	if !isEnabled || !platformIsActive {
		return false, config, nil
	}
	if !telegramConnectionConfigured(config) {
		return false, config, nil
	}
	return true, config, nil
}

func telegramConnectionConfigured(config map[string]interface{}) bool {
	if config == nil {
		return false
	}
	isConnected, _ := config["is_connected"].(bool)
	if !isConnected {
		return false
	}
	return telegramConfigString(config["chat_id"]) != ""
}

func telegramConfigString(value interface{}) string {
	if value == nil {
		return ""
	}
	if str, ok := value.(string); ok {
		return strings.TrimSpace(str)
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}

// TelegramEventNotificationEnabled reports whether a business's Telegram config
// has the given event type enabled. It is the single source of truth shared by
// the enqueue gate (skip outbox writes for disabled events) and the plugin send
// path (drop deliveries for disabled events), so the two can never disagree.
func TelegramEventNotificationEnabled(config map[string]interface{}, eventType string) bool {
	if config == nil {
		return false
	}

	key, legacyKey, fallback := telegramEventPreferenceKeys(eventType)
	if key != "" {
		if notifications, ok := config["notifications"].(map[string]interface{}); ok {
			if enabled, exists := notifications[key].(bool); exists {
				return enabled
			}
		}
	}
	if legacyKey != "" {
		if settings, ok := config["notification_settings"].(map[string]interface{}); ok {
			if enabled, exists := settings[legacyKey].(bool); exists {
				return enabled
			}
		}
	}
	return fallback
}

// telegramEventPreferenceKeys is the single source of truth (shared by the
// enqueue gate and the plugin send path) mapping an event type to its
// preference key, legacy preference key, and default-when-unset.
//
// Only events Telegram can actually render appear here. Everything else —
// including order.status_changed, which has no renderer — returns a false
// default so we never enqueue something we cannot deliver. Low-volume,
// noisy events (low inventory, daily summary) are opt-in (default off).
// The workforce events (schedule published, shift reminder, coverage decided)
// are also opt-in: they duplicate the staff-facing push/inbox channel on the
// operator's business chat, so they stay off unless an operator asks for them.
func telegramEventPreferenceKeys(eventType string) (string, string, bool) {
	switch eventType {
	case PluginEventOrderCreated:
		return "order_created", "order_notifications", true
	case PluginEventPaymentReceived:
		return "payment_received", "payment_notifications", true
	case PluginEventReservationCreated:
		return "reservation_created", "", true
	case PluginEventReservationStatusChanged:
		return "reservation_status_changed", "", true
	case PluginEventInventoryLowStock:
		return "low_inventory", "low_stock_alerts", false
	case PluginEventDailySummary:
		return "daily_summary", "daily_summary", false
	case PluginEventSchedulePublished:
		return "schedule_published", "", false
	case PluginEventShiftReminder:
		return "shift_reminder", "", false
	case PluginEventCoverageDecided:
		return "coverage_decided", "", false
	default:
		return "", "", false
	}
}

func cachedTelegramNotificationEligibility(businessID uint) *telegramNotificationEligibility {
	telegramNotificationEligibilityMu.RLock()
	entry := telegramNotificationEligibilityCache[businessID]
	telegramNotificationEligibilityMu.RUnlock()
	if entry == nil || time.Since(entry.cachedAt) >= telegramNotificationEligibilityTTL {
		return nil
	}
	return entry
}

func storeTelegramNotificationEligibility(businessID uint, eligible bool, config map[string]interface{}) {
	telegramNotificationEligibilityMu.Lock()
	telegramNotificationEligibilityCache[businessID] = &telegramNotificationEligibility{
		eligible: eligible,
		config:   config,
		cachedAt: time.Now(),
	}
	telegramNotificationEligibilityMu.Unlock()
}

// ResetTelegramNotificationEligibilityCache clears cached Telegram eligibility
// state. Tests call SetTestDB frequently, so this keeps cached decisions in
// sync with the current database.
func ResetTelegramNotificationEligibilityCache() {
	telegramNotificationEligibilityMu.Lock()
	telegramNotificationEligibilityCache = make(map[uint]*telegramNotificationEligibility)
	telegramNotificationEligibilityMu.Unlock()
}
