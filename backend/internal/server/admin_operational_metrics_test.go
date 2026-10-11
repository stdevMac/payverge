package server

import (
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Scrubbed clones historically wrote error_logs.additional_info as the plain
// string "[redacted]". That is not valid JSON for GORM serializer:json maps and
// used to 500 GET /admin/stats via getOperationalMetrics. After the soft-fail
// fix, bad rows must not fail the whole stats payload.
func TestGetOperationalMetrics_ScrubbedAdditionalInfoDoesNotFail(t *testing.T) {
	dsn := "file:admin_ops_metrics_scrub?mode=memory&cache=shared"
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	sqlDB.SetMaxOpenConns(1)

	previous := database.GetDB()
	database.SetTestDB(gormDB)
	t.Cleanup(func() { database.SetTestDB(previous) })

	require.NoError(t, gormDB.AutoMigrate(&database.ErrorLog{}, &database.AdminAction{}))
	// Insert a row the way a broken scrub would leave it: non-JSON additional_info.
	// Use raw SQL so we bypass GORM's serializer (which would reject the write).
	require.NoError(t, gormDB.Exec(`
		INSERT INTO error_logs (created_at, timestamp, source, component, function, error, message, stack, user_id, request_id, metadata, additional_info)
		VALUES (?, ?, 'web', 'test', 'Test', '[redacted]', '[redacted]', '[redacted]', '[redacted]', 'req-1', '{"_redacted":true}', '[redacted]')
	`, time.Now().UTC(), time.Now().UTC()).Error)

	var stats AdminStats
	err = getOperationalMetrics(&stats)
	// Soft-fail path: must not return an error that 500s the admin stats handler.
	require.NoError(t, err)
	require.NotNil(t, stats.RecentErrors)
}
