package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupOpsRetentionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(
		&OpsAssistantThread{},
		&OpsAssistantMessage{},
		&OpsAssistantToolCall{},
		&OpsAssistantRequest{},
	))
	SetTestDB(gormDB)
	return gormDB
}

func TestDeleteInactiveOpsAssistantThreads_PurgesStaleThreadsOnly(t *testing.T) {
	setupOpsRetentionTestDB(t)

	old := time.Now().UTC().Add(-40 * 24 * time.Hour)
	recent := time.Now().UTC().Add(-2 * 24 * time.Hour)

	stale := &OpsAssistantThread{BusinessID: 1, Title: "stale", Locale: "en", LastMessageAt: old, CreatedAt: old}
	require.NoError(t, db.Create(stale).Error)
	active := &OpsAssistantThread{BusinessID: 1, Title: "active", Locale: "en", LastMessageAt: time.Now()}
	require.NoError(t, db.Create(active).Error)
	// Created long ago but written to recently: still live.
	oldButBusy := &OpsAssistantThread{BusinessID: 1, Title: "busy", Locale: "en", LastMessageAt: recent, CreatedAt: old}
	require.NoError(t, db.Create(oldButBusy).Error)
	otherBiz := &OpsAssistantThread{BusinessID: 99, Title: "other", Locale: "en", LastMessageAt: old}
	require.NoError(t, db.Create(otherBiz).Error)

	require.NoError(t, db.Create(&OpsAssistantMessage{
		ThreadID: stale.ID, BusinessID: 1, Role: OpsAssistantRoleUser, Content: "hi",
	}).Error)
	require.NoError(t, db.Create(&OpsAssistantToolCall{
		ThreadID: stale.ID, BusinessID: 1, ToolName: "search_guides", Status: "ok",
	}).Error)
	require.NoError(t, db.Create(&OpsAssistantMessage{
		ThreadID: oldButBusy.ID, BusinessID: 1, Role: OpsAssistantRoleUser, Content: "still here",
	}).Error)

	cutoff := time.Now().UTC().Add(-30 * 24 * time.Hour)
	res, err := DeleteInactiveOpsAssistantThreads(cutoff, 100)
	require.NoError(t, err)
	require.Equal(t, int64(2), res.ThreadsDeleted) // stale + otherBiz
	require.Equal(t, int64(1), res.MessagesDeleted)
	require.Equal(t, int64(1), res.ToolCallsDeleted)

	var ids []uint
	require.NoError(t, db.Model(&OpsAssistantThread{}).Order("id").Pluck("id", &ids).Error)
	require.Equal(t, []uint{active.ID, oldButBusy.ID}, ids)

	var busyMsgs int64
	require.NoError(t, db.Model(&OpsAssistantMessage{}).Where("thread_id = ?", oldButBusy.ID).Count(&busyMsgs).Error)
	require.Equal(t, int64(1), busyMsgs)
}

func TestDeleteInactiveOpsAssistantThreads_RespectsLimit(t *testing.T) {
	setupOpsRetentionTestDB(t)

	old := time.Now().UTC().Add(-40 * 24 * time.Hour)
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&OpsAssistantThread{BusinessID: 1, Title: "t", Locale: "en", LastMessageAt: old}).Error)
	}
	res, err := DeleteInactiveOpsAssistantThreads(time.Now().UTC().Add(-30*24*time.Hour), 2)
	require.NoError(t, err)
	require.Equal(t, int64(2), res.ThreadsDeleted)
	var left int64
	require.NoError(t, db.Model(&OpsAssistantThread{}).Count(&left).Error)
	require.Equal(t, int64(1), left)
}
