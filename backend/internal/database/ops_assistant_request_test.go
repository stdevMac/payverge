package database

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupOpsRequestDB(t *testing.T) {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gdb.AutoMigrate(&OpsAssistantRequest{}))
	SetTestDB(gdb)
}

func TestClaimOpsAssistantRequest_ReplayCompleted(t *testing.T) {
	setupOpsRequestDB(t)
	row, replay, err := ClaimOpsAssistantRequest(7, "req-1")
	require.NoError(t, err)
	require.False(t, replay)
	require.NotNil(t, row)

	require.NoError(t, CompleteOpsAssistantRequest(row.ID, 1, 2, 3, ""))

	again, replay, err := ClaimOpsAssistantRequest(7, "req-1")
	require.NoError(t, err)
	require.True(t, replay)
	require.Equal(t, row.ID, again.ID)
	require.Equal(t, "completed", again.Status)
}

func TestClaimOpsAssistantRequest_InFlightConflict(t *testing.T) {
	setupOpsRequestDB(t)
	_, _, err := ClaimOpsAssistantRequest(7, "req-2")
	require.NoError(t, err)
	_, _, err = ClaimOpsAssistantRequest(7, "req-2")
	require.ErrorIs(t, err, ErrOpsAssistantRequestInFlight)
}
