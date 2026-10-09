package database

// Access-shape + perf guard for the Slice-6 labor-preview reads. The
// ScheduledCalculator.Preview path performs exactly three reads —
// GetScheduleShiftLines, GetStaffPositionRates, GetOrCreateBusinessScheduleSettings
// — and an in-memory Σ; these tests assert each read is a narrow projection (no
// SELECT *) and that the read count stays bounded regardless of shift volume (no
// per-shift rate lookup / N+1). The bounded-count case drives the DB readers
// directly rather than through services/labor so this internal test never forms a
// database→labor import cycle (labor imports database).

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// laborReadDBSeq makes each in-memory DSN unique per harness call so a benchmark
// run with -count=N (which re-invokes the body) gets a fresh shared-cache DB
// instead of colliding on the prior run's seeded rows.
var laborReadDBSeq atomic.Int64

type laborSQLRecorder struct {
	logger.Interface
	stmts []string
}

func (r *laborSQLRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	r.stmts = append(r.stmts, sql)
}

func newLaborReadTestDB(t testing.TB) (*DB, *gorm.DB, *laborSQLRecorder) {
	t.Helper()
	rec := &laborSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), laborReadDBSeq.Add(1))
	g, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: rec})
	require.NoError(t, err)
	sqlDB, err := g.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, g.AutoMigrate(&Business{}, &Staff{}, &Position{}, &StaffPosition{}, &Schedule{}, &Shift{}, &BusinessScheduleSettings{}))
	prev := db
	SetTestDB(g)
	t.Cleanup(func() { SetTestDB(prev) })
	return GetDBWrapper(), g, rec
}

func selectStmts(rec *laborSQLRecorder) []string {
	var out []string
	for _, s := range rec.stmts {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(s)), "select") {
			out = append(out, s)
		}
	}
	return out
}

func uptrDB(v uint) *uint { return &v }

func TestGetScheduleShiftLines_NarrowProjectionNoSelectStar(t *testing.T) {
	d, g, rec := newLaborReadTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&Schedule{ID: 9, BusinessID: 1, WeekStart: time.Now()}).Error)
	for i := 0; i < 20; i++ {
		require.NoError(t, g.Create(&Shift{BusinessID: 1, ScheduleID: 9, PositionID: 3,
			StartsAt: time.Now(), EndsAt: time.Now().Add(8 * time.Hour), CreatedByStaffID: 1}).Error)
	}
	rec.stmts = nil
	lines, err := d.GetScheduleShiftLines(1, 9)
	require.NoError(t, err)
	require.Len(t, lines, 20)
	sels := selectStmts(rec)
	require.Len(t, sels, 1, "exactly one SELECT, no N+1")
	low := strings.ToLower(sels[0])
	require.NotContains(t, low, "select *", "must be a narrow projection")
	require.NotContains(t, low, "select `shifts`.*")
	for _, col := range []string{"staff_id", "position_id", "starts_at", "ends_at", "break_minutes"} {
		require.Contains(t, low, col)
	}
}

func TestGetScheduleShiftLinesInWindow_NarrowProjectionNoSelectStar(t *testing.T) {
	d, g, rec := newLaborReadTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&Schedule{ID: 9, BusinessID: 1, WeekStart: time.Now()}).Error)
	now := time.Now().UTC() // reader bounds in UTC (matches GetApprovedWorkedMinutes); seed UTC
	for i := 0; i < 10; i++ {
		require.NoError(t, g.Create(&Shift{BusinessID: 1, ScheduleID: 9, PositionID: 3,
			StartsAt: now.Add(time.Duration(i) * time.Hour), EndsAt: now.Add(time.Duration(i+8) * time.Hour), CreatedByStaffID: 1}).Error)
	}
	// One out-of-window shift a month earlier must be excluded.
	require.NoError(t, g.Create(&Shift{BusinessID: 1, ScheduleID: 9, PositionID: 3,
		StartsAt: now.Add(-30 * 24 * time.Hour), EndsAt: now.Add(-29 * 24 * time.Hour), CreatedByStaffID: 1}).Error)
	rec.stmts = nil
	lines, err := d.GetScheduleShiftLinesInWindow(1, now.Add(-time.Hour), now.Add(24*time.Hour))
	require.NoError(t, err)
	require.Len(t, lines, 10, "out-of-window shift excluded by the [start,end) bound")
	sels := selectStmts(rec)
	require.Len(t, sels, 1, "exactly one SELECT, no N+1")
	require.NotContains(t, strings.ToLower(sels[0]), "select *")
}

func TestGetStaffPositionRates_SingleSelectNoStar(t *testing.T) {
	d, g, rec := newLaborReadTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	for i := 1; i <= 10; i++ {
		require.NoError(t, g.Create(&StaffPosition{BusinessID: 1, StaffID: uint(i), PositionID: 3, PayRateCents: 2000}).Error)
	}
	rec.stmts = nil
	m, err := d.GetStaffPositionRates(1)
	require.NoError(t, err)
	require.Len(t, m, 10)
	sels := selectStmts(rec)
	require.Len(t, sels, 1)
	require.NotContains(t, strings.ToLower(sels[0]), "select *")
}

// TestScheduledPreviewReads_BoundedQueryCountNoNPlus1 exercises the exact three
// reads ScheduledCalculator.Preview performs and asserts the read count does NOT
// scale with shift volume (no per-shift rate lookup). Driven directly against the
// DB readers to keep this an internal database test (no labor import cycle).
func TestScheduledPreviewReads_BoundedQueryCountNoNPlus1(t *testing.T) {
	d, g, rec := newLaborReadTestDB(t)
	require.NoError(t, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, g.Create(&Schedule{ID: 9, BusinessID: 1, WeekStart: time.Now()}).Error)
	require.NoError(t, g.Create(&BusinessScheduleSettings{BusinessID: 1, OvertimeWeeklyMinutes: 2400, PostedLeadDays: 7}).Error)
	require.NoError(t, g.Create(&StaffPosition{BusinessID: 1, StaffID: 7, PositionID: 3, PayRateCents: 2000}).Error)
	for i := 0; i < 50; i++ { // many shifts: must NOT scale query count
		require.NoError(t, g.Create(&Shift{BusinessID: 1, ScheduleID: 9, StaffID: uptrDB(7), PositionID: 3,
			StartsAt: time.Now(), EndsAt: time.Now().Add(8 * time.Hour), CreatedByStaffID: 1}).Error)
	}
	rec.stmts = nil
	_, err := d.GetScheduleShiftLines(1, 9)
	require.NoError(t, err)
	_, err = d.GetStaffPositionRates(1)
	require.NoError(t, err)
	_, err = d.GetOrCreateBusinessScheduleSettings(1)
	require.NoError(t, err)
	require.LessOrEqual(t, len(selectStmts(rec)), 3, "shifts + rates + settings only; no per-shift rate lookup")
}

func BenchmarkScheduledPreviewReads(b *testing.B) {
	d, g, _ := newLaborReadTestDB(b)
	require.NoError(b, g.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(b, g.Create(&Schedule{ID: 9, BusinessID: 1, WeekStart: time.Now()}).Error)
	require.NoError(b, g.Create(&BusinessScheduleSettings{BusinessID: 1, OvertimeWeeklyMinutes: 2400, PostedLeadDays: 7}).Error)
	for s := uint(1); s <= 2; s++ {
		require.NoError(b, g.Create(&StaffPosition{BusinessID: 1, StaffID: s, PositionID: 3, PayRateCents: 2000}).Error)
	}
	for i := 0; i < 40; i++ {
		require.NoError(b, g.Create(&Shift{BusinessID: 1, ScheduleID: 9, StaffID: uptrDB(uint(i%2 + 1)), PositionID: 3,
			StartsAt: time.Now(), EndsAt: time.Now().Add(8 * time.Hour), CreatedByStaffID: 1}).Error)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = d.GetScheduleShiftLines(1, 9)
		_, _ = d.GetStaffPositionRates(1)
		_, _ = d.GetOrCreateBusinessScheduleSettings(1)
	}
}
