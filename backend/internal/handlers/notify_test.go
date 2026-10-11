package handlers

import (
	"fmt"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newNotifyTestDB(t *testing.T) (*database.DB, func()) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	g, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, err := g.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, g.AutoMigrate(&database.Business{}, &database.Staff{}, &database.User{}, &database.StaffNotification{}))
	database.SetTestDB(g)
	return database.GetDBWrapper(), func() { _ = sqlDB.Close() }
}

func TestNotifyStaffWritesLocalizedInboxRows(t *testing.T) {
	db, cleanup := newNotifyTestDB(t)
	defer cleanup()
	require.NoError(t, db.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1", DefaultLanguage: "en"}).Error)
	s := database.Staff{BusinessID: 1, Email: "s@b1.test", Name: "S", Role: "server", IsActive: true}
	require.NoError(t, db.GetGorm().Create(&s).Error)
	require.NoError(t, db.GetGorm().Create(&database.User{Email: "s@b1.test", LanguageSelected: "es-AR"}).Error)

	// targets include a 0 and a duplicate — both must be dropped.
	notifyStaff(db, 1, []uint{s.ID, 0, s.ID}, "shift.assigned", services.PushKeyShiftAssigned,
		services.PushArgs{ShiftDate: "2026-07-03"}, "/staff/home?tab=schedule")

	rows, err := db.ListStaffNotifications(1, s.ID, 50, 0)
	require.NoError(t, err)
	require.Len(t, rows, 1, "exactly one row, de-duped, 0 dropped")
	require.Equal(t, "shift.assigned", rows[0].Kind)
	require.Equal(t, "/staff/home?tab=schedule", rows[0].URL)
	require.Contains(t, rows[0].Body, "Tenés", "localized to the staffer's es-AR language (voseo)")
	require.NotContains(t, rows[0].Title+rows[0].Body, "$")
}

func TestNotifyStaffNoTargetsIsNoop(t *testing.T) {
	db, cleanup := newNotifyTestDB(t)
	defer cleanup()
	require.NoError(t, db.GetGorm().Create(&database.Business{ID: 1, BusinessId: "biz-1"}).Error)
	notifyStaff(db, 1, []uint{0, 0}, "shift.assigned", services.PushKeyShiftAssigned, services.PushArgs{}, "/x")
	// No staff rows resolved — nothing inserted, no panic.
	var n int64
	require.NoError(t, db.GetGorm().Model(&database.StaffNotification{}).Count(&n).Error)
	require.EqualValues(t, 0, n)
}
