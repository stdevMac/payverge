package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupWebhookRetentionTestDB(t testing.TB) *gorm.DB {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&WebhookEvent{}))
	SetTestDB(gormDB)
	return gormDB
}

func seedWebhookEvent(t testing.TB, g *gorm.DB, webhookID, status string, receivedAt time.Time) {
	t.Helper()
	evt := WebhookEvent{
		Provider:   "stripe",
		WebhookID:  webhookID,
		EventType:  "payment_intent.succeeded",
		Status:     status,
		ReceivedAt: receivedAt,
	}
	require.NoError(t, g.Create(&evt).Error)
}

// TestDeleteProcessedWebhookEventsKeepsRecentAndFailed verifies that only old
// processed rows are deleted; recent processed rows and old failed rows are
// kept for forensics.
func TestDeleteProcessedWebhookEventsKeepsRecentAndFailed(t *testing.T) {
	g := setupWebhookRetentionTestDB(t)
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)

	seedWebhookEvent(t, g, "old-proc", "processed", now.AddDate(0, 0, -100))  // must delete
	seedWebhookEvent(t, g, "recent-proc", "processed", now.AddDate(0, 0, -2)) // keep
	seedWebhookEvent(t, g, "old-failed", "failed", now.AddDate(0, 0, -100))   // keep (forensics)

	cutoff := now.AddDate(0, 0, -90)
	deleted, err := DeleteProcessedWebhookEventsBefore(cutoff, 500)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted, "expected 1 deleted (old processed only)")

	var remaining int64
	require.NoError(t, g.Model(&WebhookEvent{}).Count(&remaining).Error)
	assert.Equal(t, int64(2), remaining, "expected 2 rows kept (recent processed + old failed)")
}

// TestDeleteProcessedWebhookEventsBefore_RespectsBatchLimit verifies that the
// function deletes at most `limit` rows per call so callers can loop safely.
func TestDeleteProcessedWebhookEventsBefore_RespectsBatchLimit(t *testing.T) {
	g := setupWebhookRetentionTestDB(t)
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	cutoff := now.AddDate(0, 0, -90)

	for i := 0; i < 5; i++ {
		seedWebhookEvent(t, g, "old-proc-"+time.Duration(i).String(), "processed", now.AddDate(0, 0, -200-i))
	}

	deleted, err := DeleteProcessedWebhookEventsBefore(cutoff, 2)
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted, "expected only 2 deleted per batch")

	var remaining int64
	require.NoError(t, g.Model(&WebhookEvent{}).Count(&remaining).Error)
	assert.Equal(t, int64(3), remaining, "expected 3 rows still present after one batch")
}

// TestDeleteProcessedWebhookEventsBefore_NilDBReturnsError ensures the guard
// against a nil database connection is exercised.
func TestDeleteProcessedWebhookEventsBefore_NilDBReturnsError(t *testing.T) {
	// Temporarily replace the package-global with nil.
	origDB := db
	db = nil
	t.Cleanup(func() { db = origDB })

	_, err := DeleteProcessedWebhookEventsBefore(time.Now().AddDate(0, 0, -90), 500)
	assert.Error(t, err, "expected error when db is nil")
}
