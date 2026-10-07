package database

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPlatformSettingsTestDB(t *testing.T) *DB {
	t.Helper()

	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(&PlatformSettings{}))

	return NewDB()
}
