package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupImageLimitSettingsTestDB(t *testing.T) *DB {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file:image_limit_settings_test?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	// Drop and recreate the table so each test starts with a clean slate.
	_ = gormDB.Migrator().DropTable(&PlatformSettings{})
	require.NoError(t, gormDB.AutoMigrate(&PlatformSettings{}))

	SetTestDB(gormDB)
	return NewDB()
}

// TestImageLimitSettings_GetUnset pins the safety claim on GetImageLimitSettings:
// a platform that has never been tuned reads the built-in fair-use defaults, not
// zero values.
func TestImageLimitSettings_GetUnset(t *testing.T) {
	db := setupImageLimitSettingsTestDB(t)

	got, err := db.GetImageLimitSettings()
	require.NoError(t, err)

	assert.Equal(t, DefaultImageDailyLimit, got.DailyLimit)
	assert.Equal(t, DefaultImageMonthlyAlert, got.MonthlyAlert)
}

// TestImageLimitSettings_GetMalformed verifies that a value which does not parse
// as an integer falls back to the default rather than to zero.
func TestImageLimitSettings_GetMalformed(t *testing.T) {
	db := setupImageLimitSettingsTestDB(t)

	require.NoError(t, db.SetPlatformSetting(SettingImageDailyLimit, "not-a-number", CategoryGeneral, false))
	require.NoError(t, db.SetPlatformSetting(SettingImageMonthlyAlert, "", CategoryGeneral, false))

	got, err := db.GetImageLimitSettings()
	require.NoError(t, err)

	assert.Equal(t, DefaultImageDailyLimit, got.DailyLimit)
	assert.Equal(t, DefaultImageMonthlyAlert, got.MonthlyAlert)
}

// TestImageLimitSettings_GetNonPositive covers the platform-wide outage vector: a
// stored "0" or negative parses cleanly, so without the clamp the reserve
// predicate (daily_used < limit) could never match and every generation would
// 429 forever.
func TestImageLimitSettings_GetNonPositive(t *testing.T) {
	for _, stored := range []string{"0", "-5"} {
		t.Run(stored, func(t *testing.T) {
			db := setupImageLimitSettingsTestDB(t)

			require.NoError(t, db.SetPlatformSetting(SettingImageDailyLimit, stored, CategoryGeneral, false))
			require.NoError(t, db.SetPlatformSetting(SettingImageMonthlyAlert, stored, CategoryGeneral, false))

			got, err := db.GetImageLimitSettings()
			require.NoError(t, err)

			assert.Equal(t, DefaultImageDailyLimit, got.DailyLimit, "a non-positive stored daily limit must never reach the reserve predicate")
			assert.Equal(t, DefaultImageMonthlyAlert, got.MonthlyAlert)
		})
	}
}

// TestImageLimitSettings_SetRejectsNonPositive verifies the writer refuses the
// same values the reader clamps, so the bad row is never created in the first
// place — the only protection the server-side readers get.
func TestImageLimitSettings_SetRejectsNonPositive(t *testing.T) {
	cases := map[string]ImageLimitSettings{
		"zero struct":    {},
		"zero daily":     {DailyLimit: 0, MonthlyAlert: 2000},
		"negative daily": {DailyLimit: -5, MonthlyAlert: 2000},
		"zero alert":     {DailyLimit: 500, MonthlyAlert: 0},
		"negative alert": {DailyLimit: 500, MonthlyAlert: -1},
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			db := setupImageLimitSettingsTestDB(t)

			err := db.SetImageLimitSettings(input)
			require.ErrorIs(t, err, ErrInvalidImageLimit)

			// Nothing was written, so the reader still serves the defaults.
			v, readErr := db.GetPlatformSettingValue(SettingImageDailyLimit)
			require.NoError(t, readErr)
			assert.Equal(t, "", v, "a rejected write must not open the transaction")
		})
	}
}

// TestImageLimitSettings_RoundTrip verifies SetImageLimitSettings persists both
// fields and GetImageLimitSettings reads them back intact.
func TestImageLimitSettings_RoundTrip(t *testing.T) {
	db := setupImageLimitSettingsTestDB(t)

	want := ImageLimitSettings{DailyLimit: 750, MonthlyAlert: 3000}
	require.NoError(t, db.SetImageLimitSettings(want))

	got, err := db.GetImageLimitSettings()
	require.NoError(t, err)

	assert.Equal(t, want.DailyLimit, got.DailyLimit)
	assert.Equal(t, want.MonthlyAlert, got.MonthlyAlert)
}

// TestImageLimitSettings_Update verifies a second write overwrites the first
// (upsert semantics) rather than appending a duplicate row.
func TestImageLimitSettings_Update(t *testing.T) {
	db := setupImageLimitSettingsTestDB(t)

	require.NoError(t, db.SetImageLimitSettings(ImageLimitSettings{DailyLimit: 500, MonthlyAlert: 2000}))

	second := ImageLimitSettings{DailyLimit: 1200, MonthlyAlert: 5000}
	require.NoError(t, db.SetImageLimitSettings(second))

	got, err := db.GetImageLimitSettings()
	require.NoError(t, err)

	assert.Equal(t, second.DailyLimit, got.DailyLimit)
	assert.Equal(t, second.MonthlyAlert, got.MonthlyAlert)
}

func TestImageLimitSettings_KeyParity(t *testing.T) {
	db := setupImageLimitSettingsTestDB(t)

	require.NoError(t, db.SetImageLimitSettings(ImageLimitSettings{
		DailyLimit:   500,
		MonthlyAlert: 2000,
	}))

	vDaily, err := db.GetPlatformSettingValue(SettingImageDailyLimit)
	require.NoError(t, err)
	assert.Equal(t, "500", vDaily, "SettingImageDailyLimit must match what SetImageLimitSettings writes")

	vAlert, err := db.GetPlatformSettingValue(SettingImageMonthlyAlert)
	require.NoError(t, err)
	assert.Equal(t, "2000", vAlert, "SettingImageMonthlyAlert must match what SetImageLimitSettings writes")
}
