package database

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAIHistoryDeleteDB(t *testing.T) *gorm.DB {
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
		&DirectorConsoleThread{},
		&DirectorConsoleMessage{},
		&DirectorToolCall{},
		&DirectorProposedAction{},
		&DirectorActionAudit{},
	))
	SetTestDB(gormDB)
	return gormDB
}

func TestPermanentlyDeleteDirectorConsoleThread(t *testing.T) {
	setupAIHistoryDeleteDB(t)
	thread, err := CreateDirectorConsoleThread(5, "strategy", "en")
	require.NoError(t, err)
	require.NoError(t, SaveDirectorConsoleMessage(&DirectorConsoleMessage{
		ThreadID: thread.ID, BusinessID: 5, Role: DirectorMessageRoleUser, Content: "how are sales?",
	}))

	require.ErrorIs(t, PermanentlyDeleteDirectorConsoleThread(9, thread.ID), ErrAIHistoryNotFound)
	require.NoError(t, PermanentlyDeleteDirectorConsoleThread(5, thread.ID))

	var count int64
	require.NoError(t, db.Model(&DirectorConsoleThread{}).Count(&count).Error)
	require.Equal(t, int64(0), count)
}
