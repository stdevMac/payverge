package services

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"
)

// TestEnqueuePluginNotification_SkipsWhenDeliveryDisabled verifies the
// "don't enqueue into the void" gate: when a plugin's delivery worker is not
// running, enqueue is a clean no-op rather than growing a dead backlog.
func TestEnqueuePluginNotification_SkipsWhenDeliveryDisabled(t *testing.T) {
	_, business := setupPluginNotificationOutboxTestDB(t)

	SetPluginDeliveryEnabled("telegram", false)
	t.Cleanup(func() { SetPluginDeliveryEnabled("telegram", true) })

	event := PluginNotificationEvent{
		BusinessID: business.ID,
		EventType:  "order.created",
		EventID:    "evt-disabled-1",
		Payload:    map[string]interface{}{"k": "v"},
	}
	before := testutil.ToFloat64(metrics.PluginNotificationDropped.WithLabelValues("telegram", "order.created", "delivery_disabled"))

	delivery, created, err := EnqueuePluginNotification(event, "telegram")
	require.NoError(t, err)
	assert.False(t, created)
	assert.Nil(t, delivery)

	pending, err := database.CountPluginNotificationPending("telegram")
	require.NoError(t, err)
	assert.Equal(t, int64(0), pending, "no row should be enqueued when delivery is disabled")
	assert.Equal(t, before+1, testutil.ToFloat64(metrics.PluginNotificationDropped.WithLabelValues("telegram", "order.created", "delivery_disabled")),
		"a skipped enqueue must increment PluginNotificationDropped{reason=delivery_disabled}")

	// Re-enabling restores normal enqueue behavior.
	SetPluginDeliveryEnabled("telegram", true)
	_, created, err = EnqueuePluginNotification(event, "telegram")
	require.NoError(t, err)
	assert.True(t, created)
}

func seedDelivery(t *testing.T, businessID uint, eventID string, status database.PluginNotificationDeliveryStatus, createdAt time.Time) {
	t.Helper()
	row := &database.PluginNotificationDelivery{
		BusinessID:    businessID,
		PluginName:    "telegram",
		EventType:     "order.created",
		EventID:       eventID,
		Status:        status,
		Payload:       map[string]interface{}{},
		NextAttemptAt: createdAt,
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
	}
	require.NoError(t, database.GetDBWrapper().GetGorm().Create(row).Error)
}

// TestExpireStalePluginNotificationDeliveries verifies the TTL reaper drops only
// stale pending/retry rows, leaves fresh and terminal/processing rows alone, and
// is idempotent.
func TestExpireStalePluginNotificationDeliveries(t *testing.T) {
	_, business := setupPluginNotificationOutboxTestDB(t)

	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	old := now.Add(-48 * time.Hour)
	fresh := now.Add(-1 * time.Hour)
	cutoff := now.Add(-24 * time.Hour)

	seedDelivery(t, business.ID, "old-pending", database.PluginNotificationDeliveryStatusPending, old)
	seedDelivery(t, business.ID, "old-retry", database.PluginNotificationDeliveryStatusRetry, old)
	seedDelivery(t, business.ID, "fresh-pending", database.PluginNotificationDeliveryStatusPending, fresh)
	seedDelivery(t, business.ID, "old-delivered", database.PluginNotificationDeliveryStatusDelivered, old)
	seedDelivery(t, business.ID, "old-processing", database.PluginNotificationDeliveryStatusProcessing, old)

	expired, err := database.ExpireStalePluginNotificationDeliveries(cutoff, now)
	require.NoError(t, err)
	assert.Equal(t, int64(2), expired, "only the old pending + old retry rows should expire")

	statusOf := func(eventID string) database.PluginNotificationDeliveryStatus {
		var row database.PluginNotificationDelivery
		require.NoError(t, database.GetDBWrapper().GetGorm().
			Where("event_id = ?", eventID).First(&row).Error)
		return row.Status
	}
	assert.Equal(t, database.PluginNotificationDeliveryStatusDropped, statusOf("old-pending"))
	assert.Equal(t, database.PluginNotificationDeliveryStatusDropped, statusOf("old-retry"))
	assert.Equal(t, database.PluginNotificationDeliveryStatusPending, statusOf("fresh-pending"))
	assert.Equal(t, database.PluginNotificationDeliveryStatusDelivered, statusOf("old-delivered"))
	assert.Equal(t, database.PluginNotificationDeliveryStatusProcessing, statusOf("old-processing"))

	// Idempotent: a second pass finds nothing new to expire.
	expired, err = database.ExpireStalePluginNotificationDeliveries(cutoff, now)
	require.NoError(t, err)
	assert.Equal(t, int64(0), expired)
}

// TestRunPluginOutboxMaintenance_BacklogWarning verifies the janitor surfaces a
// backlog only for a plugin whose delivery is disabled, and does not expire rows
// that are still within the TTL.
func TestRunPluginOutboxMaintenance_BacklogWarning(t *testing.T) {
	_, business := setupPluginNotificationOutboxTestDB(t)

	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	fresh := now.Add(-1 * time.Hour)
	seedDelivery(t, business.ID, "fresh-1", database.PluginNotificationDeliveryStatusPending, fresh)
	seedDelivery(t, business.ID, "fresh-2", database.PluginNotificationDeliveryStatusRetry, fresh)

	// Disabled delivery -> backlog should be reported, nothing expired (fresh).
	SetPluginDeliveryEnabled("telegram", false)
	t.Cleanup(func() { SetPluginDeliveryEnabled("telegram", true) })

	res, err := RunPluginOutboxMaintenance([]string{"telegram"}, 24*time.Hour, now)
	require.NoError(t, err)
	assert.Equal(t, int64(0), res.Expired)
	assert.Equal(t, int64(2), res.Backlog["telegram"])

	// Enabled delivery -> no backlog warning even though rows are still pending.
	SetPluginDeliveryEnabled("telegram", true)
	res, err = RunPluginOutboxMaintenance([]string{"telegram"}, 24*time.Hour, now)
	require.NoError(t, err)
	assert.Empty(t, res.Backlog)
}
