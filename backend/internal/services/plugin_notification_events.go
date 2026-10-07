package services

import "time"

const (
	PluginEventOrderCreated             = "order.created"
	PluginEventOrderStatusChanged       = "order.status_changed"
	PluginEventPaymentReceived          = "payment.received"
	PluginEventReservationCreated       = "reservation.created"
	PluginEventReservationStatusChanged = "reservation.status_changed"
	PluginEventInventoryLowStock        = "inventory.low_stock"
	PluginEventDailySummary             = "summary.daily"
	PluginEventSchedulePublished        = "schedule.published"
	PluginEventShiftReminder            = "shift.reminder"
	PluginEventCoverageDecided          = "coverage.decided"
)

type PluginNotificationEvent struct {
	BusinessID uint
	EventType  string
	EventID    string
	Payload    map[string]interface{}
	CreatedAt  time.Time
}
