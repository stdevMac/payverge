package services

import (
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRunOpsAssistantRetention_DisabledWhenZero(t *testing.T) {
	res, err := RunOpsAssistantRetention(0, 100, time.Now())
	require.NoError(t, err)
	require.Equal(t, int64(0), res.ThreadsDeleted)
}

func TestRunOpsAssistantRetention_DeletesInactiveThreads(t *testing.T) {
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(
		&database.OpsAssistantThread{},
		&database.OpsAssistantMessage{},
		&database.OpsAssistantToolCall{},
		&database.OpsAssistantRequest{},
	))
	database.SetTestDB(gormDB)

	old := time.Now().UTC().Add(-40 * 24 * time.Hour)
	for i := 0; i < 3; i++ {
		require.NoError(t, gormDB.Create(&database.OpsAssistantThread{
			BusinessID: 1, Title: "old", Locale: "en", LastMessageAt: old,
		}).Error)
	}
	fresh := &database.OpsAssistantThread{BusinessID: 1, Title: "fresh", Locale: "en", LastMessageAt: time.Now().UTC()}
	require.NoError(t, gormDB.Create(fresh).Error)

	// Batch size 2 forces a second batch; the pass must drain all stale threads.
	res, err := RunOpsAssistantRetention(30, 2, time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, int64(3), res.ThreadsDeleted)

	var left []uint
	require.NoError(t, gormDB.Model(&database.OpsAssistantThread{}).Pluck("id", &left).Error)
	require.Equal(t, []uint{fresh.ID}, left)
}
