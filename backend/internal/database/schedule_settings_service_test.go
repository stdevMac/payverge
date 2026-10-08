package database

import (
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// scheduleBenchSeq makes each benchmark-invocation's in-memory DB name unique.
// The benchmark runner re-invokes the function while sizing b.N, and a
// cache=shared in-memory SQLite keyed on a fixed name would survive between
// invocations (the pooled connection is never closed), colliding on the seeded
// Business row. A per-invocation suffix gives each a fresh database.
var scheduleBenchSeq atomic.Uint64

// newScheduleSettingsTestDB opens an isolated in-memory SQLite DB, migrates the
// tables used by the schedule-settings service, registers it as the package DB,
// and returns the wrapper. Mirrors newPositionTestDB's idiom.
func newScheduleSettingsTestDB(t *testing.T) *DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Error),
	})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&Business{}, &BusinessScheduleSettings{}))
	prev := db
	SetTestDB(gormDB)
	t.Cleanup(func() { SetTestDB(prev) })
	return GetDBWrapper()
}

func TestScheduleSettingsLazyCreateDefaults(t *testing.T) {
	d := newScheduleSettingsTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)

	s, err := d.GetOrCreateBusinessScheduleSettings(1)
	require.NoError(t, err)
	require.Equal(t, uint(1), s.BusinessID)
	require.Equal(t, 1, s.WeekStartDay)
	require.Equal(t, 480, s.DefaultShiftMinutes)
	require.Equal(t, 3, s.ReminderLeadHours)
	require.Equal(t, 2400, s.OvertimeWeeklyMinutes)
	require.Equal(t, 7, s.PostedLeadDays)
	require.Nil(t, s.MinorCutoffMin)
	require.Nil(t, s.QuietHoursStartMin)
	require.Nil(t, s.QuietHoursEndMin)
}

func TestScheduleSettingsIdempotentNoDuplicateRow(t *testing.T) {
	d := newScheduleSettingsTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)

	first, err := d.GetOrCreateBusinessScheduleSettings(1)
	require.NoError(t, err)
	second, err := d.GetOrCreateBusinessScheduleSettings(1)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID, "second call must reuse the row, not insert a duplicate")

	var count int64
	require.NoError(t, db.Model(&BusinessScheduleSettings{}).Where("business_id = ?", 1).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestScheduleSettingsUpdatePersistsAndTenantScoped(t *testing.T) {
	d := newScheduleSettingsTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, db.Create(&Business{ID: 2, BusinessId: "biz-2"}).Error)
	_, err := d.GetOrCreateBusinessScheduleSettings(1)
	require.NoError(t, err)

	cutoff := 1320
	updated, err := d.UpdateBusinessScheduleSettings(1, map[string]interface{}{
		"week_start_day":   0,
		"posted_lead_days": 14,
		"minor_cutoff_min": cutoff,
	})
	require.NoError(t, err)
	require.Equal(t, 0, updated.WeekStartDay)
	require.Equal(t, 14, updated.PostedLeadDays)
	require.NotNil(t, updated.MinorCutoffMin)
	require.Equal(t, cutoff, *updated.MinorCutoffMin)

	other, err := d.GetOrCreateBusinessScheduleSettings(2)
	require.NoError(t, err)
	require.Equal(t, 1, other.WeekStartDay)
	require.Equal(t, 7, other.PostedLeadDays)
}

func TestScheduleSettingsAccessShape(t *testing.T) {
	d := newScheduleSettingsTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	_, err := d.GetOrCreateBusinessScheduleSettings(1)
	require.NoError(t, err)
	_, err = d.GetOrCreateBusinessScheduleSettings(1)
	require.NoError(t, err)
	var total int64
	require.NoError(t, db.Model(&BusinessScheduleSettings{}).Count(&total).Error)
	require.Equal(t, int64(1), total, "repeated reads must not create extra rows")
}

func BenchmarkGetOrCreateBusinessScheduleSettings(b *testing.B) {
	prev := db
	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", b.Name(), scheduleBenchSeq.Add(1))
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Error),
	})
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		b.Fatal(err)
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	if err := gdb.AutoMigrate(&Business{}, &BusinessScheduleSettings{}); err != nil {
		b.Fatal(err)
	}
	SetTestDB(gdb)
	defer func() { SetTestDB(prev) }()
	if err := db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error; err != nil {
		b.Fatal(err)
	}
	d := GetDBWrapper()
	if _, err := d.GetOrCreateBusinessScheduleSettings(1); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = d.GetOrCreateBusinessScheduleSettings(1)
	}
}
