package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAdminActionsTestDB(t *testing.T) {
	t.Helper()
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&AdminAction{}))
	SetTestDB(gormDB)
}

func TestGetAdminActionsForBusinessContextIncludesBusinessIDDetails(t *testing.T) {
	setupAdminActionsTestDB(t)

	require.NoError(t, CreateAdminAction(1, 0, "suspend_business", map[string]interface{}{
		"business_id": uint(42),
		"reason":      "test suspension",
	}))

	actions, err := GetAdminActionsForBusinessContext(42, 0, 10)
	require.NoError(t, err)
	require.Len(t, actions, 1)
	assert.Equal(t, "suspend_business", actions[0].ActionType)
}
