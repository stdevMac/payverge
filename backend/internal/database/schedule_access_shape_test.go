package database

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// sqlCapture is a GORM logger that records every executed SQL statement so an
// access-shape test can assert the dangerous query shape is gone (unscoped
// SELECT *, app-side filtering, N+1). The embedded Silent logger satisfies the
// Info/Warn/Error methods; only Trace is overridden.
type sqlCapture struct {
	logger.Interface
	mu   sync.Mutex
	sqls []string
}

func (c *sqlCapture) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	c.mu.Lock()
	c.sqls = append(c.sqls, sql)
	c.mu.Unlock()
}

func (c *sqlCapture) matching(substr string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for _, s := range c.sqls {
		if strings.Contains(s, substr) {
			out = append(out, s)
		}
	}
	return out
}

// TestGetPublishedScheduleForStaffAccessShape proves the staff-scoped read does
// the scoping in SQL — not by loading every shift and filtering in Go. It
// asserts the shifts SELECT is filtered by staff_id (own) OR staff_id IS NULL
// (open), is an explicit column projection (no SELECT *), and never issues an
// unscoped "all shifts for the schedule" query.
func TestGetPublishedScheduleForStaffAccessShape(t *testing.T) {
	cap := &sqlCapture{Interface: logger.Default.LogMode(logger.Silent)}
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: cap})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&Business{}, &Staff{}, &Position{}, &Schedule{}, &Shift{}))
	prev := db
	SetTestDB(gormDB)
	t.Cleanup(func() { SetTestDB(prev) })
	d := GetDBWrapper()

	posID, meID := seedScheduleBusiness(t, 1)
	// A second active staff member in the same business whose shifts must NOT leak.
	other := Staff{BusinessID: 1, Email: "other@biz1.test", Name: "Other", Role: "server", IsActive: true}
	require.NoError(t, db.Create(&other).Error)

	weekStart := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	sched, err := d.GetOrCreateDraftSchedule(1, weekStart, meID)
	require.NoError(t, err)
	mineP := meID
	otherP := other.ID
	require.NoError(t, d.CreateShift(&Shift{BusinessID: 1, ScheduleID: sched.ID, PositionID: posID, StaffID: &mineP, CreatedByStaffID: meID, StartsAt: weekStart.Add(9 * time.Hour), EndsAt: weekStart.Add(17 * time.Hour)}))
	require.NoError(t, d.CreateShift(&Shift{BusinessID: 1, ScheduleID: sched.ID, PositionID: posID, StaffID: &otherP, CreatedByStaffID: meID, StartsAt: weekStart.Add(9 * time.Hour), EndsAt: weekStart.Add(17 * time.Hour)}))
	require.NoError(t, d.CreateShift(&Shift{BusinessID: 1, ScheduleID: sched.ID, PositionID: posID, CreatedByStaffID: meID, StartsAt: weekStart.Add(18 * time.Hour), EndsAt: weekStart.Add(22 * time.Hour)})) // open
	_, _, err = d.PublishSchedule(1, sched.ID, meID)
	require.NoError(t, err)

	cap.mu.Lock()
	cap.sqls = nil
	cap.mu.Unlock()

	gotSched, shifts, err := d.GetPublishedScheduleForStaff(1, weekStart, meID)
	require.NoError(t, err)
	require.NotNil(t, gotSched)

	// Behavior: only own (filled) + open shifts; the other staff's shift is excluded.
	require.Len(t, shifts, 2)
	for _, s := range shifts {
		if s.StaffID != nil {
			require.Equal(t, meID, *s.StaffID, "staff path must never return another staff's assignment")
		}
	}

	shiftQueries := cap.matching("shifts")
	require.NotEmpty(t, shiftQueries, "expected a shifts SELECT to be captured")
	var scoped string
	for _, q := range shiftQueries {
		if strings.Contains(q, "staff_id") {
			scoped = q
		}
	}
	require.NotEmpty(t, scoped, "the shifts read must filter by staff_id in SQL, not in Go")
	require.Contains(t, scoped, "IS NULL", "open shifts (staff_id IS NULL) must be unioned in SQL")
	require.NotContains(t, scoped, "SELECT *", "staff shift read must be an explicit projection, not SELECT *")

	// No N+1: exactly one shifts SELECT for the whole week.
	require.Len(t, shiftQueries, 1, "expected a single bounded shifts query, not N+1")
}

func TestGetPublishedScheduleForStaffExcludesDraft(t *testing.T) {
	d := newScheduleTestDB(t)
	posID, meID := seedScheduleBusiness(t, 1)
	weekStart := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	sched, err := d.GetOrCreateDraftSchedule(1, weekStart, meID)
	require.NoError(t, err)
	mineP := meID
	require.NoError(t, d.CreateShift(&Shift{BusinessID: 1, ScheduleID: sched.ID, PositionID: posID, StaffID: &mineP, CreatedByStaffID: meID, StartsAt: weekStart.Add(9 * time.Hour), EndsAt: weekStart.Add(17 * time.Hour)}))

	// While still a DRAFT, a plain staff member sees nothing.
	gotSched, shifts, err := d.GetPublishedScheduleForStaff(1, weekStart, meID)
	require.NoError(t, err)
	require.Nil(t, gotSched, "staff must not see an unpublished (draft) schedule")
	require.Empty(t, shifts)

	// After publish, the staff sees their own shift.
	_, _, err = d.PublishSchedule(1, sched.ID, meID)
	require.NoError(t, err)
	gotSched, shifts, err = d.GetPublishedScheduleForStaff(1, weekStart, meID)
	require.NoError(t, err)
	require.NotNil(t, gotSched)
	require.Len(t, shifts, 1)
}

// BenchmarkScheduleGet captures the staff-scoped read cost (-benchmem baseline).
func BenchmarkScheduleGet(b *testing.B) {
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
	pos := Position{BusinessID: 1, Name: "Server", IsActive: true}
	if err := db.Create(&pos).Error; err != nil {
		b.Fatal(err)
	}
	me := Staff{BusinessID: 1, Email: "me@biz1.test", Name: "Me", Role: "server", IsActive: true}
	if err := db.Create(&me).Error; err != nil {
		b.Fatal(err)
	}
	d := GetDBWrapper()
	weekStart := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	sched, err := d.GetOrCreateDraftSchedule(1, weekStart, me.ID)
	if err != nil {
		b.Fatal(err)
	}
	mineP := me.ID
	for i := 0; i < 20; i++ {
		start := weekStart.Add(time.Duration(i) * time.Hour)
		_ = d.CreateShift(&Shift{BusinessID: 1, ScheduleID: sched.ID, PositionID: pos.ID, StaffID: &mineP, CreatedByStaffID: me.ID, StartsAt: start, EndsAt: start.Add(2 * time.Hour)})
	}
	if _, _, err := d.PublishSchedule(1, sched.ID, me.ID); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = d.GetPublishedScheduleForStaff(1, weekStart, me.ID)
	}
}
