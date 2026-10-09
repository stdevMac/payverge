package database

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupReservationSettingsReadTestDB wires the package-level db var to a fresh
// in-memory SQLite instance with just the reservation_settings table migrated.
func setupReservationSettingsReadTestDB(t testing.TB) *gorm.DB {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err, "open in-memory database")

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db = gormDB
	require.NoError(t, db.AutoMigrate(&ReservationSettings{}))
	return gormDB
}

// failOnWriteCallbacks registers gorm:create and gorm:update fail-fast callbacks
// against reservation_settings so any hidden write-on-read is caught loudly.
func failOnWriteCallbacks(t testing.TB, gormDB *gorm.DB, label string) {
	t.Helper()

	createName := "payverge:test_fail_on_create_" + label
	updateName := "payverge:test_fail_on_update_" + label

	require.NoError(t, gormDB.Callback().Create().Before("gorm:create").Register(createName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "reservation_settings" {
			t.Fatalf("unexpected INSERT against reservation_settings during pure read (%s)", label)
		}
	}))
	require.NoError(t, gormDB.Callback().Update().Before("gorm:update").Register(updateName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Table == "reservation_settings" {
			t.Fatalf("unexpected UPDATE against reservation_settings during pure read (%s)", label)
		}
	}))
	t.Cleanup(func() {
		_ = gormDB.Callback().Create().Remove(createName)
		_ = gormDB.Callback().Update().Remove(updateName)
	})
}

// GetReservationSettingsForRead must NEVER persist on a read. A settings row that
// would trigger needsUpdate in normalizeReservationSettings (MaxAdvanceDays = 0)
// is returned normalized IN MEMORY without issuing any INSERT or UPDATE.
func TestGetReservationSettingsForReadDoesNotPersistOnNeedsUpdate(t *testing.T) {
	gormDB := setupReservationSettingsReadTestDB(t)

	// Seed a row that normalizeReservationSettings will want to fix. Force
	// max_advance_days = 0 with a raw UPDATE: a struct literal of 0 is the Go
	// zero value, so GORM omits it from the INSERT and the column's default:30
	// kicks in — we must write the 0 explicitly to exercise the needsUpdate path.
	seeded := ReservationSettings{
		BusinessID:           42,
		Enabled:              true,
		SendReminderEmail:    true,
		ReminderHoursBefore:  24,
		ExternalPartnerLinks: JSONRawMessage("[]"),
	}
	require.NoError(t, gormDB.Create(&seeded).Error)
	require.NoError(t, gormDB.Model(&ReservationSettings{}).Where("business_id = ?", 42).Update("max_advance_days", 0).Error)

	// Sanity: the persisted row genuinely starts at 0 (would trip needsUpdate).
	var preRead ReservationSettings
	require.NoError(t, gormDB.Where("business_id = ?", 42).First(&preRead).Error)
	require.Equal(t, 0, preRead.MaxAdvanceDays, "test seed must persist max_advance_days = 0")

	// From here on, any write to reservation_settings is a bug.
	failOnWriteCallbacks(t, gormDB, "needs_update")

	settings, err := GetReservationSettingsForRead(42)
	require.NoError(t, err)
	require.NotNil(t, settings)

	// Returned settings are normalized in memory...
	assert.Equal(t, 30, settings.MaxAdvanceDays, "read variant should normalize MaxAdvanceDays in memory")

	// ...but the persisted row is untouched (still 0).
	var persisted ReservationSettings
	require.NoError(t, gormDB.Where("business_id = ?", 42).First(&persisted).Error)
	assert.Equal(t, 0, persisted.MaxAdvanceDays, "read variant must not persist normalized values")
}

// GetReservationSettingsForRead on a business with no settings row returns
// in-memory defaults WITHOUT inserting a new row.
func TestGetReservationSettingsForReadReturnsDefaultsWithoutCreate(t *testing.T) {
	gormDB := setupReservationSettingsReadTestDB(t)

	// No row seeded. Any INSERT/UPDATE here is a bug.
	failOnWriteCallbacks(t, gormDB, "not_found")

	settings, err := GetReservationSettingsForRead(7)
	require.NoError(t, err)
	require.NotNil(t, settings)
	assert.Equal(t, uint(7), settings.BusinessID)
	assert.Equal(t, 30, settings.MaxAdvanceDays, "defaults should be applied in memory")
	assert.True(t, settings.SendReminderEmail, "defaults should enable reminder email")

	// No row was created.
	var count int64
	require.NoError(t, gormDB.Model(&ReservationSettings{}).Where("business_id = ?", 7).Count(&count).Error)
	assert.Equal(t, int64(0), count, "read variant must not create a settings row on not-found")
}

// Contrast: the persist variant GetReservationSettings (editor path) DOES still
// Save when normalizeReservationSettings reports needsUpdate. This proves the two
// variants differ and the editor path is intact.
func TestGetReservationSettingsPersistsOnNeedsUpdate(t *testing.T) {
	gormDB := setupReservationSettingsReadTestDB(t)

	seeded := ReservationSettings{
		BusinessID:           99,
		Enabled:              true,
		SendReminderEmail:    true,
		ReminderHoursBefore:  24,
		ExternalPartnerLinks: JSONRawMessage("[]"),
	}
	require.NoError(t, gormDB.Create(&seeded).Error)
	// Force max_advance_days = 0 so normalizeReservationSettings reports needsUpdate
	// (a struct literal 0 is omitted by GORM in favor of the column default:30).
	require.NoError(t, gormDB.Model(&ReservationSettings{}).Where("business_id = ?", 99).Update("max_advance_days", 0).Error)

	settings, err := GetReservationSettings(99)
	require.NoError(t, err)
	require.NotNil(t, settings)
	assert.Equal(t, 30, settings.MaxAdvanceDays)

	// The persist variant DID write the normalized value back.
	var persisted ReservationSettings
	require.NoError(t, gormDB.Where("business_id = ?", 99).First(&persisted).Error)
	assert.Equal(t, 30, persisted.MaxAdvanceDays, "persist variant should write normalized values back")
}
