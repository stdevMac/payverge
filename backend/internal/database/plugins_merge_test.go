package database

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPluginMergeTestDB(t *testing.T) *DB {
	t.Helper()

	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, gormDB.AutoMigrate(&Business{}, &Plugin{}, &BusinessPlugin{}))
	SetTestDB(gormDB)
	return GetDBWrapper()
}

// N-5: MergeBusinessPluginConfigFields must overwrite ONLY the given keys and
// preserve every other field in the stored config — in particular chat_id, which
// a whole-blob read-modify-write could resurrect from a stale copy.
func TestMergeBusinessPluginConfigFields_PreservesOtherKeys(t *testing.T) {
	db := setupPluginMergeTestDB(t)

	plugin := &Plugin{Name: "telegram", DisplayName: "Telegram", IsActive: true}
	require.NoError(t, db.GetGorm().Create(plugin).Error)

	initial, _ := json.Marshal(map[string]interface{}{
		"chat_id":       "111222333",
		"is_connected":  true,
		"business_name": "Cafe",
		"failure_count": float64(3),
	})
	bp := &BusinessPlugin{BusinessID: 1, PluginID: plugin.ID, IsEnabled: true, Config: string(initial)}
	require.NoError(t, db.GetGorm().Create(bp).Error)

	// A "reconnect" writes a NEW chat_id straight to the row (the operator edit
	// the sender's stale copy must not clobber).
	require.NoError(t, db.GetGorm().Model(&BusinessPlugin{}).Where("id = ?", bp.ID).
		Update("config", `{"chat_id":"999888777","is_connected":true,"business_name":"Cafe","failure_count":3}`).Error)

	// Now the sender records a successful send via the narrow merge.
	require.NoError(t, MergeBusinessPluginConfigFields(1, "telegram",
		MergeBusinessPluginConfigField{Key: "last_error", Value: ""},
		MergeBusinessPluginConfigField{Key: "failure_count", Value: 0},
		MergeBusinessPluginConfigField{Key: "last_error_at", Value: nil}, // nil deletes the key
	))

	merged, err := GetBusinessPluginConfig(1, "telegram")
	require.NoError(t, err)
	assert.Equal(t, "999888777", merged["chat_id"], "reconnect chat_id must survive the merge")
	assert.Equal(t, true, merged["is_connected"])
	assert.Equal(t, "Cafe", merged["business_name"])
	assert.Equal(t, "", merged["last_error"])
	assert.Equal(t, float64(0), merged["failure_count"])
	_, hasErrAt := merged["last_error_at"]
	assert.False(t, hasErrAt, "nil value should delete the key")
}

func TestMergeBusinessPluginConfigFields_NotEnabled(t *testing.T) {
	db := setupPluginMergeTestDB(t)
	_ = db
	err := MergeBusinessPluginConfigFields(42, "telegram",
		MergeBusinessPluginConfigField{Key: "last_error", Value: "x"})
	require.ErrorIs(t, err, ErrBusinessPluginNotEnabled)
}
