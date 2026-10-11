package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type rdrRow struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
}

func setupTestDBForRepo(t *testing.T) *gorm.DB {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gdb.AutoMigrate(&rdrRow{}))
	return gdb
}

func TestGetByDateRange_RejectsNonIdentifierDateField(t *testing.T) {
	gdb := setupTestDBForRepo(t)
	repo := NewRepository[rdrRow](gdb)

	_, err := repo.GetByDateRange("created_at = 1 OR 1=1 --", time.Now().Add(-time.Hour), time.Now(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid date field")

	// a clean column name still works
	_, err = repo.GetByDateRange("created_at", time.Now().Add(-time.Hour), time.Now().Add(time.Hour), "")
	assert.NoError(t, err)
}
