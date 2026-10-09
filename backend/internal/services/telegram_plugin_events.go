package services

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

type PluginDailySummary struct {
	BusinessID       uint
	Date             time.Time
	RevenueCents     int64
	OrderCount       int
	PaymentCount     int
	ReservationCount int
	Currency         string
	GeneratedAt      time.Time
}

// MaybeEnqueueTelegramInventoryLowStockAlerts gates the (relatively expensive)
// low-stock summary + enqueue on the business actually having Telegram connected
// and the low-inventory event enabled. Every caller — order placement and
// manual stock adjustments alike — must funnel through here so the gate is
// applied uniformly (INV-L2); the manual-adjustment path previously skipped it
// and ran a full GetInventorySummary even with no Telegram connection.
func MaybeEnqueueTelegramInventoryLowStockAlerts(businessID uint, observedAt time.Time) (int, error) {
	if !ShouldEnqueueTelegramNotification(businessID, PluginEventInventoryLowStock) {
		return 0, nil
	}
	return EnqueueTelegramInventoryLowStockAlertsForBusiness(businessID, observedAt)
}

func EnqueueTelegramInventoryLowStockAlertsForBusiness(businessID uint, observedAt time.Time) (int, error) {
	summary, err := database.GetInventorySummary(businessID)
	if err != nil {
		return 0, err
	}
	if !summary.Settings.LowStockWarningsEnabled {
		return 0, nil
	}

	items := make([]database.InventoryItemHealth, 0, len(summary.LowStockDetails)+len(summary.OutOfStockDetails))
	items = append(items, summary.LowStockDetails...)
	items = append(items, summary.OutOfStockDetails...)
	return EnqueueTelegramInventoryLowStockAlerts(businessID, items, observedAt)
}

// inventoryLowStockEventID is the canonical outbox dedup key for inventory
// low-stock alerts. The outbox unique key is (business, plugin, event_type,
// event_id), so BOTH the on-demand path (order/adjustment) and the 15-min
// scheduler must produce this exact string for the same (item, status, day) or
// the operator gets duplicate Telegram alerts (INV-L1).
func inventoryLowStockEventID(itemID uint, status string, observedAt time.Time) string {
	return fmt.Sprintf("inventory:%d:%s:%s", itemID, status, observedAt.UTC().Format("2006-01-02"))
}

func EnqueueTelegramInventoryLowStockAlerts(businessID uint, items []database.InventoryItemHealth, observedAt time.Time) (int, error) {
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	createdCount := 0

	for _, item := range items {
		status := strings.TrimSpace(item.Status)
		if status == "" {
			status = "low_stock"
		}
		if item.ID == 0 {
			continue
		}
		_, created, err := EnqueuePluginNotification(PluginNotificationEvent{
			BusinessID: businessID,
			EventType:  PluginEventInventoryLowStock,
			EventID:    inventoryLowStockEventID(item.ID, status, observedAt),
			Payload: map[string]interface{}{
				"inventory_item_id":  item.ID,
				"item_name":          item.Name,
				"sku":                item.SKU,
				"category":           item.Category,
				"unit":               item.Unit,
				"status":             status,
				"remaining_quantity": formatTelegramQuantity(item.CurrentQuantity),
				"threshold_quantity": formatTelegramQuantity(item.ReorderThreshold),
			},
			CreatedAt: observedAt.UTC(),
		}, "telegram")
		if err != nil {
			return createdCount, err
		}
		if created {
			createdCount++
		}
	}

	return createdCount, nil
}

func EnqueueTelegramDailySummary(summary PluginDailySummary) (int, error) {
	generatedAt := summary.GeneratedAt
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}
	date := summary.Date
	if date.IsZero() {
		date = generatedAt.AddDate(0, 0, -1)
	}
	day := date.UTC().Format("2006-01-02")
	currency := strings.TrimSpace(summary.Currency)
	if currency == "" {
		currency = "USD"
	}

	_, created, err := EnqueuePluginNotification(PluginNotificationEvent{
		BusinessID: summary.BusinessID,
		EventType:  PluginEventDailySummary,
		EventID:    "summary:daily:" + day,
		Payload: map[string]interface{}{
			"date":              day,
			"revenue_cents":     summary.RevenueCents,
			"order_count":       summary.OrderCount,
			"payment_count":     summary.PaymentCount,
			"reservation_count": summary.ReservationCount,
			"currency":          currency,
		},
		CreatedAt: generatedAt.UTC(),
	}, "telegram")
	if err != nil {
		return 0, err
	}
	if !created {
		return 0, nil
	}
	return 1, nil
}

func formatTelegramQuantity(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
