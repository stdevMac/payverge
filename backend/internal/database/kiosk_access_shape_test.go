package database

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// kioskBenchDB spins up an isolated in-memory SQLite DB migrated with the kiosk
// models and installs it as the test DB. Mirrors captureDB but takes a
// testing.TB so benchmarks can reuse it (captureDB is *testing.T only).
func kioskBenchDB(tb testing.TB, models ...interface{}) *DB {
	tb.Helper()
	// A private per-connection in-memory DB (not cache=shared): the benchmark
	// harness re-invokes the function to calibrate b.N, and a shared-cache DB
	// would survive between invocations and collide on re-seed. MaxOpenConns(1)
	// keeps the single connection (and thus the one in-memory DB) stable.
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(tb, err)
	sqlDB, err := gormDB.DB()
	require.NoError(tb, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(tb, gormDB.AutoMigrate(models...))
	prev := db
	SetTestDB(gormDB)
	tb.Cleanup(func() { SetTestDB(prev) })
	return GetDBWrapper()
}

// seedKioskStaff creates an active staffer; when pin is non-empty a PIN is set.
func seedKioskStaff(tb testing.TB, id, businessID uint, name, pin string) {
	tb.Helper()
	require.NoError(tb, GetDB().Create(&Staff{
		ID: id, BusinessID: businessID, Email: fmt.Sprintf("s%d@biz.test", id),
		Name: name, Role: StaffRoleServer, InvitedBy: "owner@biz.test", IsActive: true,
	}).Error)
	if pin != "" {
		require.NoError(tb, SetStaffPin(id, pin))
	}
}

// seedInactiveStaff creates a deactivated staffer. is_active is written via an
// explicit column update because GORM substitutes the `default:true` tag for a
// zero-value bool on insert (creating with IsActive:false would store true).
func seedInactiveStaff(tb testing.TB, id, businessID uint, name string) {
	tb.Helper()
	require.NoError(tb, GetDB().Create(&Staff{
		ID: id, BusinessID: businessID, Email: fmt.Sprintf("s%d@biz.test", id),
		Name: name, Role: StaffRoleServer, InvitedBy: "owner@biz.test", IsActive: true,
	}).Error)
	require.NoError(tb, GetDB().Table("staff").Where("id = ?", id).Update("is_active", false).Error)
}

// TestKioskRosterAccessShape locks the kiosk roster query shape (perf gate):
// exactly one staff read and one open-entries read (no N+1 across the roster),
// explicit projections (no SELECT *), pin_hash never selected, tenant-scoped in
// SQL, active-only, and bounded by LIMIT.
func TestKioskRosterAccessShape(t *testing.T) {
	d, cap := captureDB(t, &Business{}, &Staff{}, &TimeEntry{})
	require.NoError(t, GetDB().Create(&Business{ID: 1, BusinessId: "b1", Timezone: "UTC"}).Error)
	seedKioskStaff(t, 10, 1, "Dana", "123456")
	seedKioskStaff(t, 11, 1, "Eli", "654321")
	seedKioskStaff(t, 12, 1, "Fio", "") // no PIN
	// Dana is on the clock.
	require.NoError(t, GetDB().Create(&TimeEntry{BusinessID: 1, StaffID: 10, ClockInAt: time.Now().UTC(), Status: TimeEntryStatusOpen}).Error)

	cap.mu.Lock()
	cap.sqls = nil
	cap.mu.Unlock()

	rows, err := d.KioskRoster(1)
	require.NoError(t, err)
	require.Len(t, rows, 3)

	// The staff read is uniquely identified by pin_set_at; the open-entries read
	// by clock_in_at. Exactly one of each — a roster of N staff must not fan out
	// into N reads.
	staffSQLs := cap.matching("pin_set_at")
	require.Len(t, staffSQLs, 1, "roster must issue exactly one staff read (no N+1)")
	entrySQLs := cap.matching("clock_in_at")
	require.Len(t, entrySQLs, 1, "roster must issue exactly one open-entries read")

	staffSQL := staffSQLs[0]
	require.NotContains(t, staffSQL, "SELECT *", "staff read must use an explicit projection")
	require.NotContains(t, staffSQL, "pin_hash", "kiosk reads must never select the bcrypt hash")
	require.Contains(t, staffSQL, "business_id", "staff read must be tenant-scoped in SQL")
	require.Contains(t, staffSQL, "is_active", "roster must filter to active staff in SQL")
	require.Contains(t, strings.ToUpper(staffSQL), "LIMIT", "roster staff read must be bounded")

	entrySQL := entrySQLs[0]
	require.NotContains(t, entrySQL, "SELECT *", "open-entries read must use an explicit projection")
	require.Contains(t, entrySQL, "business_id", "open-entries read must be tenant-scoped in SQL")
	require.Contains(t, entrySQL, "status", "open-entries read must filter on status in SQL")
}

// TestKioskRosterAnnotations proves the in-memory join: has_pin reflects PIN
// enrollment and on_clock reflects an open punch.
func TestKioskRosterAnnotations(t *testing.T) {
	d, _ := captureDB(t, &Business{}, &Staff{}, &TimeEntry{})
	require.NoError(t, GetDB().Create(&Business{ID: 1, BusinessId: "b1", Timezone: "UTC"}).Error)
	seedKioskStaff(t, 10, 1, "Dana", "123456") // has pin, on clock
	seedKioskStaff(t, 11, 1, "Eli", "654321")  // has pin, off clock
	seedKioskStaff(t, 12, 1, "Fio", "")        // no pin, off clock
	clockIn := time.Now().UTC().Add(-90 * time.Minute)
	require.NoError(t, GetDB().Create(&TimeEntry{BusinessID: 1, StaffID: 10, ClockInAt: clockIn, Status: TimeEntryStatusOpen}).Error)

	rows, err := d.KioskRoster(1)
	require.NoError(t, err)
	require.Len(t, rows, 3)

	byID := map[uint]KioskStaffMember{}
	for _, r := range rows {
		byID[r.StaffID] = r
	}
	require.True(t, byID[10].HasPin)
	require.True(t, byID[10].OnClock)
	require.NotNil(t, byID[10].ClockInAt)
	require.WithinDuration(t, clockIn, *byID[10].ClockInAt, time.Second)
	require.True(t, byID[11].HasPin)
	require.False(t, byID[11].OnClock)
	require.False(t, byID[12].HasPin)
	require.Nil(t, byID[12].ClockInAt)
}

// TestKioskRosterTenantIsolation proves the roster never crosses businesses and
// excludes inactive staff.
func TestKioskRosterTenantIsolation(t *testing.T) {
	d, _ := captureDB(t, &Business{}, &Staff{}, &TimeEntry{})
	require.NoError(t, GetDB().Create(&Business{ID: 1, BusinessId: "b1", Timezone: "UTC"}).Error)
	require.NoError(t, GetDB().Create(&Business{ID: 2, BusinessId: "b2", Timezone: "UTC"}).Error)
	seedKioskStaff(t, 10, 1, "Dana", "123456")
	seedKioskStaff(t, 20, 2, "Zed", "111111") // other business
	seedInactiveStaff(t, 13, 1, "Gone")       // inactive in business 1

	rows, err := d.KioskRoster(1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, uint(10), rows[0].StaffID)
}

// TestGetKioskStaffTenantScoped proves the single-staff lookup enforces
// business + active in SQL — the tenant safety net for the punch path.
func TestGetKioskStaffTenantScoped(t *testing.T) {
	d, _ := captureDB(t, &Business{}, &Staff{}, &TimeEntry{})
	require.NoError(t, GetDB().Create(&Business{ID: 1, BusinessId: "b1", Timezone: "UTC"}).Error)
	require.NoError(t, GetDB().Create(&Business{ID: 2, BusinessId: "b2", Timezone: "UTC"}).Error)
	seedKioskStaff(t, 10, 1, "Dana", "123456")
	seedKioskStaff(t, 20, 2, "Zed", "111111")
	seedInactiveStaff(t, 13, 1, "Gone")

	got, err := d.GetKioskStaff(1, 10)
	require.NoError(t, err)
	require.Equal(t, "Dana", got.Name)
	require.True(t, got.HasPin)

	// Cross-tenant → not found.
	_, err = d.GetKioskStaff(1, 20)
	require.ErrorIs(t, err, ErrStaffNotFound)
	// Inactive → not found.
	_, err = d.GetKioskStaff(1, 13)
	require.ErrorIs(t, err, ErrStaffNotFound)
}

// TestGetOpenEntryToggleState proves the toggle predicate the handler relies on.
func TestGetOpenEntryToggleState(t *testing.T) {
	d, _ := captureDB(t, &Business{}, &Staff{}, &TimeEntry{})
	require.NoError(t, GetDB().Create(&Business{ID: 1, BusinessId: "b1", Timezone: "UTC"}).Error)
	seedKioskStaff(t, 10, 1, "Dana", "123456")

	_, err := d.GetOpenEntry(1, 10)
	require.ErrorIs(t, err, ErrNotClockedIn, "no punch yet → not clocked in")

	require.NoError(t, GetDB().Create(&TimeEntry{BusinessID: 1, StaffID: 10, ClockInAt: time.Now().UTC(), Status: TimeEntryStatusOpen}).Error)
	got, err := d.GetOpenEntry(1, 10)
	require.NoError(t, err)
	require.Equal(t, uint(10), got.StaffID)
}

// BenchmarkKioskRoster measures the two-query roster read against a 50-person
// roster with a third clocked in.
func BenchmarkKioskRoster(b *testing.B) {
	d := kioskBenchDB(b, &Business{}, &Staff{}, &TimeEntry{})
	require.NoError(b, GetDB().Create(&Business{ID: 1, BusinessId: "b1", Timezone: "UTC"}).Error)
	for i := uint(1); i <= 50; i++ {
		pin := ""
		if i%2 == 0 {
			pin = "123456"
		}
		seedKioskStaff(b, i, 1, fmt.Sprintf("Staff %d", i), pin)
		if i%3 == 0 {
			require.NoError(b, GetDB().Create(&TimeEntry{BusinessID: 1, StaffID: i, ClockInAt: time.Now().UTC(), Status: TimeEntryStatusOpen}).Error)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := d.KioskRoster(1); err != nil {
			b.Fatal(err)
		}
	}
}
