package database

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupCustomURLTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&Business{}))
	SetTestDB(gormDB)
	return gormDB
}

// The public slug lookup must be case-insensitive: with the 000147 unique
// index on lower(custom_url) there is exactly one owner per slug, and a guest
// typing any casing must reach it.
func TestGetBusinessByCustomURL_CaseInsensitive(t *testing.T) {
	db := setupCustomURLTestDB(t)
	require.NoError(t, db.Create(&Business{
		BusinessId: "biz-slug", Name: "Slug Biz", CustomURL: "MiCafe",
		IsActive: true, BusinessPageEnabled: true,
	}).Error)

	found, err := GetBusinessByCustomURL("micafe")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "biz-slug", found.BusinessId)

	found, err = GetBusinessByCustomURL("MICAFE")
	require.NoError(t, err)
	require.NotNil(t, found)

	// Empty string must never resolve (many rows legitimately hold '').
	_, err = GetBusinessByCustomURL("")
	assert.Error(t, err)
}
