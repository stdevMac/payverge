package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTelegramPluginTestDB(t *testing.T) *DB {
	t.Helper()

	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, gormDB.AutoMigrate(
		&Business{},
		&TelegramConnectionToken{},
		&TelegramUpdateReceipt{},
		&PluginNotificationDelivery{},
		&PluginNotificationDeliveryAttempt{},
	))

	SetTestDB(gormDB)
	return GetDBWrapper()
}

func createTelegramPluginTestBusiness(t *testing.T, db *DB) *Business {
	t.Helper()

	suffix := fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())
	business := &Business{
		BusinessId:     "telegram-" + suffix,
		OwnerAddress:   "owner-" + suffix,
		Name:           "Telegram Test",
		SettlementAddr: "settlement-" + suffix,
		TippingAddr:    "tipping-" + suffix,
	}
	require.NoError(t, db.GetGorm().Create(business).Error)
	return business
}

// consumeTelegramConnectionTokenInTx drives the live ConsumeTelegramConnectionTokenTx
// predicate inside a transaction, the way the Telegram update processor does.
func consumeTelegramConnectionTokenInTx(t *testing.T, db *DB, tokenHash string, chatID int64, username string, now time.Time) (*TelegramConnectionToken, error) {
	t.Helper()
	var consumed *TelegramConnectionToken
	err := db.GetGorm().Transaction(func(tx *gorm.DB) error {
		token, err := ConsumeTelegramConnectionTokenTx(tx, tokenHash, chatID, username, now)
		if err != nil {
			return err
		}
		consumed = token
		return nil
	})
	if err != nil {
		return nil, err
	}
	return consumed, nil
}

func TestConsumeTelegramConnectionToken_ConsumesValidPendingToken(t *testing.T) {
	db := setupTelegramPluginTestDB(t)
	business := createTelegramPluginTestBusiness(t, db)
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)

	require.NoError(t, CreateTelegramConnectionToken(&TelegramConnectionToken{
		BusinessID: business.ID,
		TokenHash:  "hash-valid",
		ExpiresAt:  now.Add(15 * time.Minute),
		CreatedAt:  now,
	}))

	consumed, err := consumeTelegramConnectionTokenInTx(t, db, "hash-valid", 123456789, "owner_user", now)

	require.NoError(t, err)
	require.NotNil(t, consumed)
	assert.Equal(t, business.ID, consumed.BusinessID)
	assert.Equal(t, int64(123456789), *consumed.UsedByChatID)
	assert.Equal(t, "owner_user", consumed.UsedByUsername)
	require.NotNil(t, consumed.UsedAt)
	assert.Equal(t, now, *consumed.UsedAt)
}

func TestConsumeTelegramConnectionToken_RejectsExpiredToken(t *testing.T) {
	db := setupTelegramPluginTestDB(t)
	business := createTelegramPluginTestBusiness(t, db)
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)

	require.NoError(t, CreateTelegramConnectionToken(&TelegramConnectionToken{
		BusinessID: business.ID,
		TokenHash:  "hash-expired",
		ExpiresAt:  now.Add(-time.Minute),
		CreatedAt:  now.Add(-30 * time.Minute),
	}))

	consumed, err := consumeTelegramConnectionTokenInTx(t, db, "hash-expired", 123456789, "owner_user", now)

	require.Error(t, err)
	assert.Nil(t, consumed)
	assert.ErrorIs(t, err, ErrTelegramConnectionTokenUnavailable)
}

func TestConsumeTelegramConnectionToken_RejectsUsedToken(t *testing.T) {
	db := setupTelegramPluginTestDB(t)
	business := createTelegramPluginTestBusiness(t, db)
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	usedAt := now.Add(-time.Minute)
	usedChatID := int64(222)

	require.NoError(t, CreateTelegramConnectionToken(&TelegramConnectionToken{
		BusinessID:   business.ID,
		TokenHash:    "hash-used",
		ExpiresAt:    now.Add(15 * time.Minute),
		UsedAt:       &usedAt,
		UsedByChatID: &usedChatID,
		CreatedAt:    now.Add(-5 * time.Minute),
	}))

	consumed, err := consumeTelegramConnectionTokenInTx(t, db, "hash-used", 123456789, "owner_user", now)

	require.Error(t, err)
	assert.Nil(t, consumed)
	assert.ErrorIs(t, err, ErrTelegramConnectionTokenUnavailable)
}

func TestConsumeTelegramConnectionToken_RejectsRevokedToken(t *testing.T) {
	db := setupTelegramPluginTestDB(t)
	business := createTelegramPluginTestBusiness(t, db)
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	revokedAt := now.Add(-time.Minute)

	require.NoError(t, CreateTelegramConnectionToken(&TelegramConnectionToken{
		BusinessID: business.ID,
		TokenHash:  "hash-revoked",
		ExpiresAt:  now.Add(15 * time.Minute),
		RevokedAt:  &revokedAt,
		CreatedAt:  now.Add(-5 * time.Minute),
	}))

	consumed, err := consumeTelegramConnectionTokenInTx(t, db, "hash-revoked", 123456789, "owner_user", now)

	require.Error(t, err)
	assert.Nil(t, consumed)
	assert.ErrorIs(t, err, ErrTelegramConnectionTokenUnavailable)
}

func TestRecordTelegramUpdateReceipt_DeduplicatesUpdateID(t *testing.T) {
	db := setupTelegramPluginTestDB(t)
	business := createTelegramPluginTestBusiness(t, db)
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)

	first, err := RecordTelegramUpdateReceipt(&TelegramUpdateReceipt{
		UpdateID:    987654,
		BusinessID:  &business.ID,
		ChatID:      int64Ptr(111),
		Status:      "processed",
		ProcessedAt: now,
	})
	require.NoError(t, err)
	assert.True(t, first)

	second, err := RecordTelegramUpdateReceipt(&TelegramUpdateReceipt{
		UpdateID:    987654,
		BusinessID:  &business.ID,
		ChatID:      int64Ptr(111),
		Status:      "processed",
		ProcessedAt: now.Add(time.Minute),
	})
	require.NoError(t, err)
	assert.False(t, second)
}

func TestCreatePluginNotificationDelivery_DeduplicatesBusinessPluginEvent(t *testing.T) {
	db := setupTelegramPluginTestDB(t)
	business := createTelegramPluginTestBusiness(t, db)
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)

	delivery, created, err := CreatePluginNotificationDelivery(&PluginNotificationDelivery{
		BusinessID:    business.ID,
		PluginName:    "telegram",
		EventType:     "order.created",
		EventID:       "order:42",
		Status:        PluginNotificationDeliveryStatusPending,
		Payload:       map[string]interface{}{"order_number": "O-42"},
		NextAttemptAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	require.NoError(t, err)
	assert.True(t, created)
	require.NotZero(t, delivery.ID)

	duplicate, created, err := CreatePluginNotificationDelivery(&PluginNotificationDelivery{
		BusinessID:    business.ID,
		PluginName:    "telegram",
		EventType:     "order.created",
		EventID:       "order:42",
		Status:        PluginNotificationDeliveryStatusPending,
		Payload:       map[string]interface{}{"order_number": "O-42-duplicate"},
		NextAttemptAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, delivery.ID, duplicate.ID)
	assert.Equal(t, "O-42", duplicate.Payload["order_number"])
}

func TestClaimPluginNotificationDeliveries_ClaimsPendingAndRetryOnly(t *testing.T) {
	db := setupTelegramPluginTestDB(t)
	business := createTelegramPluginTestBusiness(t, db)
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)

	seedDeliveries := []PluginNotificationDelivery{
		{
			BusinessID:    business.ID,
			PluginName:    "telegram",
			EventType:     "order.created",
			EventID:       "pending",
			Status:        PluginNotificationDeliveryStatusPending,
			Payload:       map[string]interface{}{"event": "pending"},
			NextAttemptAt: now.Add(-time.Minute),
		},
		{
			BusinessID:    business.ID,
			PluginName:    "telegram",
			EventType:     "payment.received",
			EventID:       "retry",
			Status:        PluginNotificationDeliveryStatusRetry,
			Payload:       map[string]interface{}{"event": "retry"},
			NextAttemptAt: now,
		},
		{
			BusinessID:    business.ID,
			PluginName:    "telegram",
			EventType:     "reservation.created",
			EventID:       "future",
			Status:        PluginNotificationDeliveryStatusPending,
			Payload:       map[string]interface{}{"event": "future"},
			NextAttemptAt: now.Add(time.Hour),
		},
		{
			BusinessID:    business.ID,
			PluginName:    "telegram",
			EventType:     "order.created",
			EventID:       "delivered",
			Status:        PluginNotificationDeliveryStatusDelivered,
			Payload:       map[string]interface{}{"event": "delivered"},
			NextAttemptAt: now.Add(-time.Minute),
		},
		{
			BusinessID:    business.ID,
			PluginName:    "stripe",
			EventType:     "order.created",
			EventID:       "other-plugin",
			Status:        PluginNotificationDeliveryStatusPending,
			Payload:       map[string]interface{}{"event": "other-plugin"},
			NextAttemptAt: now.Add(-time.Minute),
		},
	}
	require.NoError(t, db.GetGorm().Create(&seedDeliveries).Error)

	claimed, err := ClaimPluginNotificationDeliveries("telegram", 10, now)

	require.NoError(t, err)
	require.Len(t, claimed, 2)
	assert.Equal(t, "pending", claimed[0].EventID)
	assert.Equal(t, "retry", claimed[1].EventID)

	for _, delivery := range claimed {
		assert.Equal(t, PluginNotificationDeliveryStatusProcessing, delivery.Status)
		assert.NotNil(t, delivery.LastAttemptAt)
	}
}

func TestReclaimStaleProcessingPluginNotificationDeliveries_ReclaimsStaleRowToRetry(t *testing.T) {
	db := setupTelegramPluginTestDB(t)
	business := createTelegramPluginTestBusiness(t, db)
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	staleAt := now.Add(-30 * time.Minute)

	stale := &PluginNotificationDelivery{
		BusinessID:    business.ID,
		PluginName:    "telegram",
		EventType:     "order.created",
		EventID:       "stale-processing",
		Status:        PluginNotificationDeliveryStatusProcessing,
		Payload:       map[string]interface{}{"event": "stale"},
		AttemptCount:  1,
		NextAttemptAt: staleAt,
		LastAttemptAt: &staleAt,
		CreatedAt:     staleAt,
		UpdatedAt:     staleAt,
	}
	require.NoError(t, db.GetGorm().Create(stale).Error)

	reclaimed, err := ReclaimStaleProcessingPluginNotificationDeliveries("telegram", 5*time.Minute, 5, now)
	require.NoError(t, err)
	assert.Equal(t, int64(1), reclaimed)

	var reloaded PluginNotificationDelivery
	require.NoError(t, db.GetGorm().First(&reloaded, stale.ID).Error)
	assert.Equal(t, PluginNotificationDeliveryStatusRetry, reloaded.Status)

	// A reclaimed row must become claimable again.
	claimed, err := ClaimPluginNotificationDeliveries("telegram", 10, now)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	assert.Equal(t, "stale-processing", claimed[0].EventID)
}

func TestReclaimStaleProcessingPluginNotificationDeliveries_LeavesFreshProcessingRow(t *testing.T) {
	db := setupTelegramPluginTestDB(t)
	business := createTelegramPluginTestBusiness(t, db)
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	freshAt := now.Add(-30 * time.Second)

	fresh := &PluginNotificationDelivery{
		BusinessID:    business.ID,
		PluginName:    "telegram",
		EventType:     "order.created",
		EventID:       "fresh-processing",
		Status:        PluginNotificationDeliveryStatusProcessing,
		Payload:       map[string]interface{}{"event": "fresh"},
		AttemptCount:  1,
		NextAttemptAt: freshAt,
		LastAttemptAt: &freshAt,
		CreatedAt:     freshAt,
		UpdatedAt:     freshAt,
	}
	require.NoError(t, db.GetGorm().Create(fresh).Error)

	reclaimed, err := ReclaimStaleProcessingPluginNotificationDeliveries("telegram", 5*time.Minute, 5, now)
	require.NoError(t, err)
	assert.Equal(t, int64(0), reclaimed)

	var reloaded PluginNotificationDelivery
	require.NoError(t, db.GetGorm().First(&reloaded, fresh.ID).Error)
	assert.Equal(t, PluginNotificationDeliveryStatusProcessing, reloaded.Status)
}

func TestReclaimStaleProcessingPluginNotificationDeliveries_FailsExhaustedRow(t *testing.T) {
	db := setupTelegramPluginTestDB(t)
	business := createTelegramPluginTestBusiness(t, db)
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	staleAt := now.Add(-30 * time.Minute)

	exhausted := &PluginNotificationDelivery{
		BusinessID:    business.ID,
		PluginName:    "telegram",
		EventType:     "order.created",
		EventID:       "exhausted-processing",
		Status:        PluginNotificationDeliveryStatusProcessing,
		Payload:       map[string]interface{}{"event": "exhausted"},
		AttemptCount:  5, // already at the max attempt cap
		NextAttemptAt: staleAt,
		LastAttemptAt: &staleAt,
		CreatedAt:     staleAt,
		UpdatedAt:     staleAt,
	}
	require.NoError(t, db.GetGorm().Create(exhausted).Error)

	reclaimed, err := ReclaimStaleProcessingPluginNotificationDeliveries("telegram", 5*time.Minute, 5, now)
	require.NoError(t, err)
	assert.Equal(t, int64(1), reclaimed)

	var reloaded PluginNotificationDelivery
	require.NoError(t, db.GetGorm().First(&reloaded, exhausted.ID).Error)
	assert.Equal(t, PluginNotificationDeliveryStatusFailed, reloaded.Status)
	require.NotNil(t, reloaded.FailedAt)

	// An exhausted row must NOT be reclaimable for delivery.
	claimed, err := ClaimPluginNotificationDeliveries("telegram", 10, now)
	require.NoError(t, err)
	assert.Empty(t, claimed)
}

func TestTelegramPluginProductionModels_IncludedInAutoMigrateSet(t *testing.T) {
	models := telegramPluginProductionModels()

	assert.Contains(t, models, &TelegramConnectionToken{})
	assert.Contains(t, models, &TelegramUpdateReceipt{})
	assert.Contains(t, models, &PluginNotificationDelivery{})
	assert.Contains(t, models, &PluginNotificationDeliveryAttempt{})
}

func TestPluginNotificationClaimUsesPostgresSkipLocked(t *testing.T) {
	assert.True(t, pluginNotificationClaimUsesSkipLocked("postgres"))
	assert.False(t, pluginNotificationClaimUsesSkipLocked("sqlite"))
}

func int64Ptr(value int64) *int64 {
	return &value
}
