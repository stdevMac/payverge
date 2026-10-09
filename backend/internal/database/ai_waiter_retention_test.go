package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupRetentionTestDB(t testing.TB) *gorm.DB {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&AiWaiterConversation{}, &AiWaiterMessage{}))
	SetTestDB(gormDB)
	return gormDB
}

func seedConversation(t testing.TB, g *gorm.DB, createdAt time.Time, msgCount int) uint {
	t.Helper()
	conv := AiWaiterConversation{
		SessionID:  "sess-" + createdAt.Format("20060102150405.000000"),
		BusinessID: 1,
		TableCode:  "T1",
		Mode:       "ordering",
		Status:     "active",
		CreatedAt:  createdAt,
		UpdatedAt:  createdAt,
	}
	require.NoError(t, g.Create(&conv).Error)
	for i := 0; i < msgCount; i++ {
		require.NoError(t, g.Create(&AiWaiterMessage{
			ConversationID: conv.ID,
			Role:           "user",
			Content:        "hello",
			CreatedAt:      createdAt,
		}).Error)
	}
	return conv.ID
}

func TestDeleteExpiredAiWaiterTranscripts_RemovesOldKeepsRecent(t *testing.T) {
	g := setupRetentionTestDB(t)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	cutoff := now.AddDate(0, 0, -90)

	oldConv := seedConversation(t, g, now.AddDate(0, 0, -120), 3)  // expired
	freshConv := seedConversation(t, g, now.AddDate(0, 0, -10), 2) // kept

	res, err := DeleteExpiredAiWaiterTranscripts(cutoff, 1000)
	require.NoError(t, err)
	assert.Equal(t, int64(1), res.ConversationsDeleted)
	assert.Equal(t, int64(3), res.MessagesDeleted)

	var convCount, msgCount int64
	require.NoError(t, g.Model(&AiWaiterConversation{}).Count(&convCount).Error)
	require.NoError(t, g.Model(&AiWaiterMessage{}).Count(&msgCount).Error)
	assert.Equal(t, int64(1), convCount)
	assert.Equal(t, int64(2), msgCount)

	_ = oldConv
	_ = freshConv
}

func TestDeleteExpiredAiWaiterTranscripts_RespectsBatchLimit(t *testing.T) {
	g := setupRetentionTestDB(t)
	now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	cutoff := now.AddDate(0, 0, -90)

	for i := 0; i < 5; i++ {
		seedConversation(t, g, now.AddDate(0, 0, -200-i), 0)
	}

	res, err := DeleteExpiredAiWaiterTranscripts(cutoff, 2)
	require.NoError(t, err)
	assert.Equal(t, int64(2), res.ConversationsDeleted)

	var convCount int64
	require.NoError(t, g.Model(&AiWaiterConversation{}).Count(&convCount).Error)
	assert.Equal(t, int64(3), convCount)
}
