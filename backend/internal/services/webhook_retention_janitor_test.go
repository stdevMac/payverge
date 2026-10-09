package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func setupWebhookJanitorTestDB(t testing.TB) *gorm.DB {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&database.WebhookEvent{}))
	database.SetTestDB(gormDB)
	return gormDB
}

func seedWebhookEvt(t testing.TB, g *gorm.DB, webhookID, status string, receivedAt time.Time) {
	t.Helper()
	evt := database.WebhookEvent{
		Provider:   "stripe",
		WebhookID:  webhookID,
		EventType:  "payment_intent.succeeded",
		Status:     status,
		ReceivedAt: receivedAt,
	}
	require.NoError(t, g.Create(&evt).Error)
}

func TestRunWebhookEventRetention_DeletesBeyondWindow(t *testing.T) {
	g := setupWebhookJanitorTestDB(t)
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)

	seedWebhookEvt(t, g, "old-proc", "processed", now.AddDate(0, 0, -120)) // expired
	seedWebhookEvt(t, g, "edge-proc", "processed", now.AddDate(0, 0, -61)) // expired (just over 60d)
	seedWebhookEvt(t, g, "fresh-proc", "processed", now.AddDate(0, 0, -5)) // kept
	seedWebhookEvt(t, g, "old-fail", "failed", now.AddDate(0, 0, -200))    // kept (forensics)

	n, err := RunWebhookEventRetention(60, 500, now)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n, "expected 2 old processed rows deleted")

	var remaining int64
	require.NoError(t, g.Model(&database.WebhookEvent{}).Count(&remaining).Error)
	assert.Equal(t, int64(2), remaining, "expected 2 rows kept (fresh processed + old failed)")
}

func TestRunWebhookEventRetention_ZeroDaysDisables(t *testing.T) {
	g := setupWebhookJanitorTestDB(t)
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	seedWebhookEvt(t, g, "old-proc", "processed", now.AddDate(0, 0, -400))

	n, err := RunWebhookEventRetention(0, 500, now)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n, "zero retentionDays should disable deletion")

	var remaining int64
	require.NoError(t, g.Model(&database.WebhookEvent{}).Count(&remaining).Error)
	assert.Equal(t, int64(1), remaining, "row must be kept when janitor is disabled")
}

func TestRunWebhookEventRetention_DrainsAcrossBatches(t *testing.T) {
	g := setupWebhookJanitorTestDB(t)
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 7; i++ {
		seedWebhookEvt(t, g, "old-proc-"+time.Duration(i).String(), "processed", now.AddDate(0, 0, -200-i))
	}

	n, err := RunWebhookEventRetention(60, 2, now)
	require.NoError(t, err)
	assert.Equal(t, int64(7), n, "expected all 7 expired rows swept across multiple batches")

	var remaining int64
	require.NoError(t, g.Model(&database.WebhookEvent{}).Count(&remaining).Error)
	assert.Equal(t, int64(0), remaining)
}
