package server

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRecordProvenanceOnRegenerate(t *testing.T) {
	gormDB, err := gorm.Open(sqlite.Open("file:ai_prov_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&database.AIGeneratedImage{}))
	database.SetTestDB(gormDB)

	require.NoError(t, database.RecordAIGeneratedImage(42, "menu_items/ai_generated/y.png", "regenerate", "google/gemini-2.5-flash-image"))

	var rows []database.AIGeneratedImage
	require.NoError(t, database.GetDB().Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, "regenerate", rows[0].Source)
	assert.Equal(t, "google/gemini-2.5-flash-image", rows[0].Model)
}
