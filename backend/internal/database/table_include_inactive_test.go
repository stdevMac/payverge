package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"
)

// TestGetTablesWithStatusExcludesInactiveByDefault locks the legacy behavior:
// with no includeInactive arg, soft-deleted (is_active=false) tables must not
// appear — deactivation still hides tables from the default board.
func TestGetTablesWithStatusExcludesInactiveByDefault(t *testing.T) {
	businessID := setupTableStatusPerfDB(t, logger.Default.LogMode(logger.Silent))

	// Soft-delete one table (mirrors DeleteTable).
	var victim Table
	require.NoError(t, db.Where("business_id = ?", businessID).Order("id ASC").First(&victim).Error)
	require.NoError(t, db.Model(&Table{}).Where("id = ?", victim.ID).Update("is_active", false).Error)

	rows, err := GetTablesWithStatus(businessID, false)
	require.NoError(t, err)
	assert.Len(t, rows, 499, "default board must exclude the soft-deleted table")
	for _, row := range rows {
		tbl, ok := row["table"].(Table)
		require.True(t, ok)
		assert.NotEqual(t, victim.ID, tbl.ID, "inactive table must not appear by default")
	}
}

// TestGetTablesWithStatusIncludeInactive proves the trapdoor is escapable:
// include_inactive=true surfaces the soft-deleted table so the operator can
// reach it and reactivate.
func TestGetTablesWithStatusIncludeInactive(t *testing.T) {
	businessID := setupTableStatusPerfDB(t, logger.Default.LogMode(logger.Silent))

	var victim Table
	require.NoError(t, db.Where("business_id = ?", businessID).Order("id ASC").First(&victim).Error)
	require.NoError(t, db.Model(&Table{}).Where("id = ?", victim.ID).Update("is_active", false).Error)

	rows, err := GetTablesWithStatus(businessID, true)
	require.NoError(t, err)
	assert.Len(t, rows, 500, "include_inactive must return every table, active or not")

	var found bool
	for _, row := range rows {
		tbl, ok := row["table"].(Table)
		require.True(t, ok)
		if tbl.ID == victim.ID {
			found = true
			assert.False(t, tbl.IsActive, "the recovered table must carry is_active=false")
		}
	}
	assert.True(t, found, "the soft-deleted table must be reachable with include_inactive")
}
