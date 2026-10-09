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

// captureDB opens an isolated in-memory SQLite DB wired to a fresh sqlCapture so
// a live-floor access-shape test can assert the emitted SQL. Mirrors the inline
// setup in TestGetPublishedScheduleForStaffAccessShape but shared by the two
// live-floor reads. Returns the wrapper and the capture; the caller resets
// cap.sqls right before the method under test.
func captureDB(t *testing.T, models ...interface{}) (*DB, *sqlCapture) {
	t.Helper()
	cap := &sqlCapture{Interface: logger.Default.LogMode(logger.Silent)}
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: cap})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(models...))
	prev := db
	SetTestDB(gormDB)
	t.Cleanup(func() { SetTestDB(prev) })
	return GetDBWrapper(), cap
}

// TestGetActiveShiftsForDayAccessShape proves the live-floor shift scan does the
// filtering in SQL: only published, assigned (staff_id NOT NULL), filled shifts
// that overlap the day window — the open shift and the out-of-window shift are
// excluded by the query, not in Go. Explicit projection, single bounded query.
func TestGetActiveShiftsForDayAccessShape(t *testing.T) {
	d, cap := captureDB(t, &Business{}, &Staff{}, &Position{}, &Schedule{}, &Shift{})
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)

	day := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	dayStart := day
	dayEnd := day.Add(24 * time.Hour)
	a, b := uint(11), uint(22)
	mk := func(staffID *uint, published bool, status string, start, end time.Time) {
		require.NoError(t, db.Create(&Shift{
			BusinessID: 1, ScheduleID: 1, PositionID: 1, StaffID: staffID,
			StartsAt: start, EndsAt: end, Status: status, Published: published, CreatedByStaffID: 1,
		}).Error)
	}
	// In-window assigned+published+filled (kept).
	mk(&a, true, ShiftStatusFilled, day.Add(9*time.Hour), day.Add(17*time.Hour))
	mk(&b, true, ShiftStatusFilled, day.Add(11*time.Hour), day.Add(19*time.Hour))
	// Overnight straddling the day start (kept: ends after dayStart, starts before dayEnd).
	mk(&a, true, ShiftStatusFilled, day.Add(-3*time.Hour), day.Add(4*time.Hour))
	// Open (unassigned) — excluded.
	mk(nil, true, ShiftStatusOpen, day.Add(18*time.Hour), day.Add(22*time.Hour))
	// Assigned but NOT published — excluded.
	mk(&b, false, ShiftStatusFilled, day.Add(9*time.Hour), day.Add(17*time.Hour))
	// Next day, outside the window — excluded.
	mk(&a, true, ShiftStatusFilled, dayEnd.Add(9*time.Hour), dayEnd.Add(17*time.Hour))

	cap.mu.Lock()
	cap.sqls = nil
	cap.mu.Unlock()

	shifts, err := d.GetActiveShiftsForDay(1, dayStart, dayEnd)
	require.NoError(t, err)
	require.Len(t, shifts, 3, "only the three published, assigned, in-window shifts")
	for _, s := range shifts {
		require.NotNil(t, s.StaffID, "the scan must never return an open (unassigned) shift")
		require.Equal(t, ShiftStatusFilled, s.Status)
		require.True(t, s.Published)
	}

	q := cap.matching("shifts")
	require.NotEmpty(t, q, "expected a shifts SELECT")
	require.Len(t, q, 1, "single bounded query, not N+1")
	require.Contains(t, q[0], "staff_id IS NOT NULL", "assigned-only filter must be in SQL")
	require.Contains(t, q[0], "starts_at", "day-window filter must be in SQL")
	require.NotContains(t, q[0], "SELECT *", "explicit projection, not SELECT *")
	require.NotContains(t, q[0], "reminded_at", "operator-only claim column must not be projected")
}

// TestListEntriesForDayAccessShape proves the live-floor entry scan filters in
// SQL: every open punch (even one opened before the window) plus every punch
// that clocked in during the day; a closed punch from a prior day is excluded.
func TestListEntriesForDayAccessShape(t *testing.T) {
	d, cap := captureDB(t, &Business{}, &Staff{}, &TimeEntry{})
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)

	day := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	dayStart := day
	dayEnd := day.Add(24 * time.Hour)
	mkOpen := func(staffID uint, in time.Time) {
		require.NoError(t, db.Create(&TimeEntry{
			BusinessID: 1, StaffID: staffID, ClockInAt: in, Status: TimeEntryStatusOpen,
		}).Error)
	}
	mkClosed := func(staffID uint, in, out time.Time) {
		o := out
		require.NoError(t, db.Create(&TimeEntry{
			BusinessID: 1, StaffID: staffID, ClockInAt: in, ClockOutAt: &o,
			Status: TimeEntryStatusPendingReview,
		}).Error)
	}
	// Open punch opened BEFORE the window (overnight) — kept (still on the floor).
	mkOpen(11, day.Add(-2*time.Hour))
	// Closed punch that clocked in today — kept.
	mkClosed(22, day.Add(9*time.Hour), day.Add(13*time.Hour))
	// Open punch that clocked in today — kept.
	mkOpen(33, day.Add(14*time.Hour))
	// Closed punch from YESTERDAY — excluded (not open, clocked in before window).
	mkClosed(44, day.Add(-20*time.Hour), day.Add(-14*time.Hour))

	cap.mu.Lock()
	cap.sqls = nil
	cap.mu.Unlock()

	entries, err := d.ListEntriesForDay(1, dayStart, dayEnd)
	require.NoError(t, err)
	require.Len(t, entries, 3, "two open (incl. overnight) + one clocked-in-today closed")
	for _, e := range entries {
		require.NotEqual(t, uint(44), e.StaffID, "a prior-day closed punch must be excluded")
	}

	q := cap.matching("time_entries")
	require.NotEmpty(t, q, "expected a time_entries SELECT")
	require.Len(t, q, 1, "single bounded query, not N+1")
	require.True(t, strings.Contains(q[0], "status") && strings.Contains(q[0], "clock_in_at"),
		"open-OR-clocked-in-today filter must be in SQL")
	require.NotContains(t, q[0], "SELECT *", "explicit projection, not SELECT *")
}

// BenchmarkGetActiveShiftsForDay captures the live-floor shift scan cost.
func BenchmarkGetActiveShiftsForDay(b *testing.B) {
	prev := db
	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", b.Name(), scheduleSvcBenchSeq.Add(1))
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		b.Fatal(err)
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	if err := gdb.AutoMigrate(&Business{}, &Staff{}, &Position{}, &Schedule{}, &Shift{}); err != nil {
		b.Fatal(err)
	}
	SetTestDB(gdb)
	defer func() { SetTestDB(prev) }()

	if err := db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error; err != nil {
		b.Fatal(err)
	}
	day := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 40; i++ {
		sid := uint(i%12 + 1)
		start := day.Add(time.Duration(i%14) * time.Hour)
		if err := db.Create(&Shift{
			BusinessID: 1, ScheduleID: 1, PositionID: 1, StaffID: &sid,
			StartsAt: start, EndsAt: start.Add(8 * time.Hour),
			Status: ShiftStatusFilled, Published: true, CreatedByStaffID: 1,
		}).Error; err != nil {
			b.Fatal(err)
		}
	}
	d := GetDBWrapper()
	dayEnd := day.Add(24 * time.Hour)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = d.GetActiveShiftsForDay(1, day, dayEnd)
	}
}

// BenchmarkListEntriesForDay captures the live-floor entry scan cost.
func BenchmarkListEntriesForDay(b *testing.B) {
	prev := db
	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", b.Name(), timeEntrySvcBenchSeq.Add(1))
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		b.Fatal(err)
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	if err := gdb.AutoMigrate(&Business{}, &Staff{}, &TimeEntry{}); err != nil {
		b.Fatal(err)
	}
	SetTestDB(gdb)
	defer func() { SetTestDB(prev) }()

	if err := db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error; err != nil {
		b.Fatal(err)
	}
	day := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 40; i++ {
		in := day.Add(time.Duration(i%14) * time.Hour)
		e := TimeEntry{BusinessID: 1, StaffID: uint(i%12 + 1), ClockInAt: in, Status: TimeEntryStatusOpen}
		if i%2 == 0 {
			out := in.Add(4 * time.Hour)
			e.ClockOutAt = &out
			e.Status = TimeEntryStatusPendingReview
		}
		if err := db.Create(&e).Error; err != nil {
			b.Fatal(err)
		}
	}
	d := GetDBWrapper()
	dayEnd := day.Add(24 * time.Hour)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = d.ListEntriesForDay(1, day, dayEnd)
	}
}
