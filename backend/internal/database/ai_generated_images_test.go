package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newAIImageTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file:ai_gen_images_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&AIGeneratedImage{}))
	db = gormDB
	return gormDB
}

func TestRecordAIGeneratedImage_Writes(t *testing.T) {
	db := newAIImageTestDB(t)
	_ = db

	err := RecordAIGeneratedImage(7, "menu_items/ai_generated/x.png", "generate", "google/gemini-2.5-flash-image")
	require.NoError(t, err)

	var rows []AIGeneratedImage
	require.NoError(t, GetDB().Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, uint(7), rows[0].BusinessID)
	assert.Equal(t, "generate", rows[0].Source)
	assert.Equal(t, "google/gemini-2.5-flash-image", rows[0].Model)
	assert.NotEmpty(t, rows[0].S3Key)
	assert.False(t, rows[0].CreatedAt.IsZero())
}

func TestRecordAIGeneratedImage_RejectsZeroBusiness(t *testing.T) {
	_ = newAIImageTestDB(t)
	assert.Error(t, RecordAIGeneratedImage(0, "k", "generate", "m"))
}
