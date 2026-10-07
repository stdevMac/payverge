package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

type recordingPluginNotificationSender struct {
	results []PluginNotificationSendResult
	sent    []database.PluginNotificationDelivery
}

func (s *recordingPluginNotificationSender) SendPluginNotification(_ context.Context, delivery database.PluginNotificationDelivery) PluginNotificationSendResult {
	s.sent = append(s.sent, delivery)
	if len(s.results) == 0 {
		return PluginNotificationSendResult{ProviderMessageID: fmt.Sprintf("provider-%d", delivery.ID)}
	}
	result := s.results[0]
	s.results = s.results[1:]
	return result
}

func setupPluginNotificationOutboxTestDB(t testing.TB) (*database.DB, *database.Business) {
	t.Helper()

	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.PluginNotificationDelivery{},
		&database.PluginNotificationDeliveryAttempt{},
	))
	database.SetTestDB(gormDB)

	db := database.GetDBWrapper()
	business := &database.Business{
		BusinessId:     "plugin-notification-test",
		OwnerAddress:   "owner",
		Name:           "Plugin Notification Test",
		SettlementAddr: "settlement",
		TippingAddr:    "tipping",
	}
	require.NoError(t, db.GetGorm().Create(business).Error)
	return db, business
}

func TestEnqueuePluginNotification_DeduplicatesEvent(t *testing.T) {
	_, business := setupPluginNotificationOutboxTestDB(t)
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	event := PluginNotificationEvent{
		BusinessID: business.ID,
		EventType:  PluginEventOrderCreated,
		EventID:    "order:42",
		Payload:    map[string]interface{}{"order_number": "O-42"},
		CreatedAt:  now,
	}

	first, created, err := EnqueuePluginNotification(event, "telegram")
	require.NoError(t, err)
	assert.True(t, created)
	require.NotZero(t, first.ID)

	second, created, err := EnqueuePluginNotification(event, "telegram")
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, first.ID, second.ID)
}

func TestEnqueuePluginNotification_RespectsTelegramEventAllowList(t *testing.T) {
	_, business := setupPluginNotificationOutboxTestDB(t)
	t.Setenv("TELEGRAM_NOTIFICATION_EVENTS", PluginEventPaymentReceived)

	delivery, created, err := EnqueuePluginNotification(PluginNotificationEvent{
		BusinessID: business.ID,
		EventType:  PluginEventOrderCreated,
		EventID:    "order:disabled-by-flag",
		Payload:    map[string]interface{}{"order_number": "O-disabled"},
		CreatedAt:  time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC),
	}, "telegram")

	require.NoError(t, err)
	assert.False(t, created)
	assert.Nil(t, delivery)

	_, created, err = EnqueuePluginNotification(PluginNotificationEvent{
		BusinessID: business.ID,
		EventType:  PluginEventPaymentReceived,
		EventID:    "payment:enabled-by-flag",
		Payload:    map[string]interface{}{"amount_cents": 1200},
		CreatedAt:  time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC),
	}, "telegram")
	require.NoError(t, err)
	assert.True(t, created)
}

func TestEnqueueTelegramInventoryLowStockAlerts_DeduplicatesPerItemPerDay(t *testing.T) {
	db, business := setupPluginNotificationOutboxTestDB(t)
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	items := []database.InventoryItemHealth{{
		ID:               7,
		Name:             "Tomatoes",
		Unit:             "kg",
		CurrentQuantity:  2,
		ReorderThreshold: 5,
		Status:           "low_stock",
	}}

	created, err := EnqueueTelegramInventoryLowStockAlerts(business.ID, items, now)
	require.NoError(t, err)
	assert.Equal(t, 1, created)

	created, err = EnqueueTelegramInventoryLowStockAlerts(business.ID, items, now)
	require.NoError(t, err)
	assert.Equal(t, 0, created)

	var deliveries []database.PluginNotificationDelivery
	require.NoError(t, db.GetGorm().Find(&deliveries).Error)
	require.Len(t, deliveries, 1)
	assert.Equal(t, PluginEventInventoryLowStock, deliveries[0].EventType)
	assert.Equal(t, "inventory:7:low_stock:2026-05-11", deliveries[0].EventID)
	assert.Equal(t, "Tomatoes", deliveries[0].Payload["item_name"])
}

func TestEnqueueTelegramDailySummary_DeduplicatesPerBusinessDate(t *testing.T) {
	db, business := setupPluginNotificationOutboxTestDB(t)
	date := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)

	created, err := EnqueueTelegramDailySummary(PluginDailySummary{
		BusinessID:       business.ID,
		Date:             date,
		RevenueCents:     12345,
		OrderCount:       12,
		PaymentCount:     8,
		ReservationCount: 3,
		Currency:         "USD",
		GeneratedAt:      date.Add(24 * time.Hour),
	})
	require.NoError(t, err)
	assert.Equal(t, 1, created)

	created, err = EnqueueTelegramDailySummary(PluginDailySummary{
		BusinessID:   business.ID,
		Date:         date,
		RevenueCents: 99999,
		GeneratedAt:  date.Add(24 * time.Hour),
	})
	require.NoError(t, err)
	assert.Equal(t, 0, created)

	var delivery database.PluginNotificationDelivery
	require.NoError(t, db.GetGorm().First(&delivery).Error)
	assert.Equal(t, PluginEventDailySummary, delivery.EventType)
	assert.Equal(t, "summary:daily:2026-05-10", delivery.EventID)
	assert.Equal(t, float64(12345), delivery.Payload["revenue_cents"])
}

func TestTelegramNotificationWorker_DeliversPendingMessage(t *testing.T) {
	db, business := setupPluginNotificationOutboxTestDB(t)
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	_, created, err := EnqueuePluginNotification(PluginNotificationEvent{
		BusinessID: business.ID,
		EventType:  PluginEventOrderCreated,
		EventID:    "order:42",
		Payload:    map[string]interface{}{"order_number": "O-42"},
		CreatedAt:  now,
	}, "telegram")
	require.NoError(t, err)
	require.True(t, created)

	sender := &recordingPluginNotificationSender{}
	worker := NewPluginNotificationWorker("telegram", sender)
	worker.Now = func() time.Time { return now }

	processed, err := worker.ProcessDue(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Len(t, sender.sent, 1)

	var delivery database.PluginNotificationDelivery
	require.NoError(t, db.GetGorm().Where("business_id = ?", business.ID).First(&delivery).Error)
	assert.Equal(t, database.PluginNotificationDeliveryStatusDelivered, delivery.Status)
	require.NotNil(t, delivery.DeliveredAt)
}

func TestTelegramNotificationWorker_RetriesTransientFailure(t *testing.T) {
	db, business := setupPluginNotificationOutboxTestDB(t)
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	_, created, err := EnqueuePluginNotification(PluginNotificationEvent{
		BusinessID: business.ID,
		EventType:  PluginEventPaymentReceived,
		EventID:    "payment:42",
		Payload:    map[string]interface{}{"amount_cents": 4200},
		CreatedAt:  now,
	}, "telegram")
	require.NoError(t, err)
	require.True(t, created)

	sender := &recordingPluginNotificationSender{results: []PluginNotificationSendResult{{
		Err:          errors.New("temporary telegram outage"),
		ErrorCode:    "telegram_unavailable",
		ErrorMessage: "temporary telegram outage",
	}}}
	worker := NewPluginNotificationWorker("telegram", sender)
	worker.Now = func() time.Time { return now }

	processed, err := worker.ProcessDue(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	var delivery database.PluginNotificationDelivery
	require.NoError(t, db.GetGorm().Where("business_id = ?", business.ID).First(&delivery).Error)
	assert.Equal(t, database.PluginNotificationDeliveryStatusRetry, delivery.Status)
	assert.Equal(t, "telegram_unavailable", delivery.LastErrorCode)
	assert.True(t, delivery.NextAttemptAt.After(now))
}

func TestTelegramNotificationWorker_DropsDisconnectedBusiness(t *testing.T) {
	db, business := setupPluginNotificationOutboxTestDB(t)
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	_, created, err := EnqueuePluginNotification(PluginNotificationEvent{
		BusinessID: business.ID,
		EventType:  PluginEventOrderCreated,
		EventID:    "order:42",
		Payload:    map[string]interface{}{"order_number": "O-42"},
		CreatedAt:  now,
	}, "telegram")
	require.NoError(t, err)
	require.True(t, created)

	sender := &recordingPluginNotificationSender{results: []PluginNotificationSendResult{{
		Drop:         true,
		ErrorCode:    "telegram_not_connected",
		ErrorMessage: "telegram not connected",
	}}}
	worker := NewPluginNotificationWorker("telegram", sender)
	worker.Now = func() time.Time { return now }

	processed, err := worker.ProcessDue(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	var delivery database.PluginNotificationDelivery
	require.NoError(t, db.GetGorm().Where("business_id = ?", business.ID).First(&delivery).Error)
	assert.Equal(t, database.PluginNotificationDeliveryStatusDropped, delivery.Status)
	assert.Equal(t, "telegram_not_connected", delivery.LastErrorCode)
}

func TestTelegramNotificationWorker_ReclaimsStrandedProcessingRow(t *testing.T) {
	db, business := setupPluginNotificationOutboxTestDB(t)
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	staleAt := now.Add(-30 * time.Minute)

	// Simulate a row a previous worker claimed (status -> processing,
	// attempt_count incremented) but crashed before writing terminal state.
	stranded := &database.PluginNotificationDelivery{
		BusinessID:    business.ID,
		PluginName:    "telegram",
		EventType:     PluginEventOrderCreated,
		EventID:       "order:stranded",
		Status:        database.PluginNotificationDeliveryStatusProcessing,
		Payload:       map[string]interface{}{"order_number": "O-stranded"},
		AttemptCount:  1,
		NextAttemptAt: staleAt,
		LastAttemptAt: &staleAt,
		CreatedAt:     staleAt,
		UpdatedAt:     staleAt,
	}
	require.NoError(t, db.GetGorm().Create(stranded).Error)

	sender := &recordingPluginNotificationSender{}
	worker := NewPluginNotificationWorker("telegram", sender)
	worker.Now = func() time.Time { return now }
	worker.ReclaimStaleAfter = 5 * time.Minute

	// ProcessDue must reclaim the stranded processing row before claiming, so it
	// becomes claimable again and is delivered in the same sweep.
	processed, err := worker.ProcessDue(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	require.Len(t, sender.sent, 1)
	assert.Equal(t, "order:stranded", sender.sent[0].EventID)

	var delivery database.PluginNotificationDelivery
	require.NoError(t, db.GetGorm().First(&delivery, stranded.ID).Error)
	assert.Equal(t, database.PluginNotificationDeliveryStatusDelivered, delivery.Status)
	require.NotNil(t, delivery.DeliveredAt)
}

func TestTelegramNotificationWorker_LeavesFreshProcessingRow(t *testing.T) {
	db, business := setupPluginNotificationOutboxTestDB(t)
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	freshAt := now.Add(-30 * time.Second)

	fresh := &database.PluginNotificationDelivery{
		BusinessID:    business.ID,
		PluginName:    "telegram",
		EventType:     PluginEventOrderCreated,
		EventID:       "order:in-flight",
		Status:        database.PluginNotificationDeliveryStatusProcessing,
		Payload:       map[string]interface{}{"order_number": "O-in-flight"},
		AttemptCount:  1,
		NextAttemptAt: freshAt,
		LastAttemptAt: &freshAt,
		CreatedAt:     freshAt,
		UpdatedAt:     freshAt,
	}
	require.NoError(t, db.GetGorm().Create(fresh).Error)

	sender := &recordingPluginNotificationSender{}
	worker := NewPluginNotificationWorker("telegram", sender)
	worker.Now = func() time.Time { return now }
	worker.ReclaimStaleAfter = 5 * time.Minute

	// A row still within the stale window is actively being sent; ProcessDue must
	// not reclaim it (no double-send).
	processed, err := worker.ProcessDue(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 0, processed)
	assert.Empty(t, sender.sent)

	var delivery database.PluginNotificationDelivery
	require.NoError(t, db.GetGorm().First(&delivery, fresh.ID).Error)
	assert.Equal(t, database.PluginNotificationDeliveryStatusProcessing, delivery.Status)
}

func TestTelegramNotificationWorker_FailsPermanentTelegramError(t *testing.T) {
	db, business := setupPluginNotificationOutboxTestDB(t)
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	_, created, err := EnqueuePluginNotification(PluginNotificationEvent{
		BusinessID: business.ID,
		EventType:  PluginEventPaymentReceived,
		EventID:    "payment:42",
		Payload:    map[string]interface{}{"amount_cents": 4200},
		CreatedAt:  now,
	}, "telegram")
	require.NoError(t, err)
	require.True(t, created)

	sender := &recordingPluginNotificationSender{results: []PluginNotificationSendResult{{
		PermanentFailure: true,
		ErrorCode:        "telegram_forbidden",
		ErrorMessage:     "bot was blocked",
	}}}
	worker := NewPluginNotificationWorker("telegram", sender)
	worker.Now = func() time.Time { return now }

	processed, err := worker.ProcessDue(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	var delivery database.PluginNotificationDelivery
	require.NoError(t, db.GetGorm().Where("business_id = ?", business.ID).First(&delivery).Error)
	assert.Equal(t, database.PluginNotificationDeliveryStatusFailed, delivery.Status)
	assert.Equal(t, "telegram_forbidden", delivery.LastErrorCode)
	require.NotNil(t, delivery.FailedAt)
}

// N-1: EnqueuePluginNotificationTx writes the outbox row on the caller's
// transaction so it commits atomically with the domain write. A rolled-back
// transaction leaves no orphaned outbox row.
func TestEnqueuePluginNotificationTx_RollsBackWithTransaction(t *testing.T) {
	db, business := setupPluginNotificationOutboxTestDB(t)
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	event := PluginNotificationEvent{
		BusinessID: business.ID,
		EventType:  PluginEventOrderCreated,
		EventID:    "order:rollback",
		Payload:    map[string]interface{}{"order_number": "O-rb"},
		CreatedAt:  now,
	}

	sentinel := errors.New("domain write failed")
	err := db.GetGorm().Transaction(func(tx *gorm.DB) error {
		_, created, err := EnqueuePluginNotificationTx(tx, event, "telegram")
		require.NoError(t, err)
		require.True(t, created)
		return sentinel // force rollback
	})
	require.ErrorIs(t, err, sentinel)

	var count int64
	require.NoError(t, db.GetGorm().Model(&database.PluginNotificationDelivery{}).
		Where("event_id = ?", "order:rollback").Count(&count).Error)
	assert.Equal(t, int64(0), count, "rolled-back tx must not leave an orphaned outbox row")
}

func TestEnqueuePluginNotificationTx_CommitsWithTransaction(t *testing.T) {
	db, business := setupPluginNotificationOutboxTestDB(t)
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	event := PluginNotificationEvent{
		BusinessID: business.ID,
		EventType:  PluginEventOrderCreated,
		EventID:    "order:commit",
		Payload:    map[string]interface{}{"order_number": "O-c"},
		CreatedAt:  now,
	}

	require.NoError(t, db.GetGorm().Transaction(func(tx *gorm.DB) error {
		_, created, err := EnqueuePluginNotificationTx(tx, event, "telegram")
		require.NoError(t, err)
		require.True(t, created)
		return nil
	}))

	var count int64
	require.NoError(t, db.GetGorm().Model(&database.PluginNotificationDelivery{}).
		Where("event_id = ?", "order:commit").Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

// N-4: a failed send in the middle of a batch must not affect the delivered
// state of the messages that DID send. Per-message marking makes each delivered
// row independent of its neighbors.
func TestTelegramNotificationWorker_PerMessageDeliveredMark(t *testing.T) {
	db, business := setupPluginNotificationOutboxTestDB(t)
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	for _, id := range []string{"order:a", "order:b", "order:c"} {
		_, created, err := EnqueuePluginNotification(PluginNotificationEvent{
			BusinessID: business.ID,
			EventType:  PluginEventOrderCreated,
			EventID:    id,
			Payload:    map[string]interface{}{"order_number": id},
			CreatedAt:  now,
		}, "telegram")
		require.NoError(t, err)
		require.True(t, created)
	}

	// Claim order is next_attempt_at ASC, id ASC → insertion order a, b, c.
	sender := &recordingPluginNotificationSender{results: []PluginNotificationSendResult{
		{ProviderMessageID: "msg-a"},
		{Err: errors.New("transient"), ErrorCode: "telegram_unavailable", ErrorMessage: "transient"},
		{ProviderMessageID: "msg-c"},
	}}
	worker := NewPluginNotificationWorker("telegram", sender)
	worker.Now = func() time.Time { return now }

	processed, err := worker.ProcessDue(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, processed)

	statusOf := func(eventID string) database.PluginNotificationDeliveryStatus {
		var d database.PluginNotificationDelivery
		require.NoError(t, db.GetGorm().Where("event_id = ?", eventID).First(&d).Error)
		return d.Status
	}
	assert.Equal(t, database.PluginNotificationDeliveryStatusDelivered, statusOf("order:a"))
	assert.Equal(t, database.PluginNotificationDeliveryStatusRetry, statusOf("order:b"))
	assert.Equal(t, database.PluginNotificationDeliveryStatusDelivered, statusOf("order:c"))
}
