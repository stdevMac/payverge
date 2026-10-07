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

func setupRetentionJanitorTestDB(t testing.TB) *gorm.DB {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&database.AiWaiterConversation{}, &database.AiWaiterMessage{}))
	database.SetTestDB(gormDB)
	return gormDB
}

func seedConv(t testing.TB, g *gorm.DB, sessionID string, createdAt time.Time, msgs int) {
	t.Helper()
	conv := database.AiWaiterConversation{
		SessionID: sessionID, BusinessID: 1, TableCode: "T1",
		Mode: "ordering", Status: "active", CreatedAt: createdAt, UpdatedAt: createdAt,
	}
	require.NoError(t, g.Create(&conv).Error)
	for i := 0; i < msgs; i++ {
		require.NoError(t, g.Create(&database.AiWaiterMessage{
			ConversationID: conv.ID, Role: "user", Content: "hi", CreatedAt: createdAt,
		}).Error)
	}
}

func TestRunAiTranscriptRetention_DeletesBeyondWindow(t *testing.T) {
	g := setupRetentionJanitorTestDB(t)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)

	seedConv(t, g, "old", now.AddDate(0, 0, -120), 4) // expired
	seedConv(t, g, "edge", now.AddDate(0, 0, -91), 2) // expired (just over 90d)
	seedConv(t, g, "fresh", now.AddDate(0, 0, -5), 3) // kept

	res, err := RunAiTranscriptRetention(90, 500, now)
	require.NoError(t, err)
	assert.Equal(t, int64(2), res.ConversationsDeleted)
	assert.Equal(t, int64(6), res.MessagesDeleted)

	var convCount int64
	require.NoError(t, g.Model(&database.AiWaiterConversation{}).Count(&convCount).Error)
	assert.Equal(t, int64(1), convCount)
}

func TestRunAiTranscriptRetention_ZeroDaysDisables(t *testing.T) {
	g := setupRetentionJanitorTestDB(t)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	seedConv(t, g, "old", now.AddDate(0, 0, -400), 2)

	res, err := RunAiTranscriptRetention(0, 500, now)
	require.NoError(t, err)
	assert.Equal(t, int64(0), res.ConversationsDeleted)

	var convCount int64
	require.NoError(t, g.Model(&database.AiWaiterConversation{}).Count(&convCount).Error)
	assert.Equal(t, int64(1), convCount)
}

func TestRunAiTranscriptRetention_DrainsAcrossBatches(t *testing.T) {
	g := setupRetentionJanitorTestDB(t)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 7; i++ {
		seedConv(t, g, "old-"+time.Duration(i).String(), now.AddDate(0, 0, -200-i), 0)
	}

	res, err := RunAiTranscriptRetention(90, 2, now)
	require.NoError(t, err)
	assert.Equal(t, int64(7), res.ConversationsDeleted)

	var convCount int64
	require.NoError(t, g.Model(&database.AiWaiterConversation{}).Count(&convCount).Error)
	assert.Equal(t, int64(0), convCount)
}
