package database

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

// timeEntrySvcBenchSeq makes each benchmark-invocation's in-memory DB name
// unique (mirrors availabilitySvcBenchSeq).
var timeEntrySvcBenchSeq atomic.Uint64

// newTimeEntryTestDB opens an isolated in-memory SQLite DB, migrates the tables
// the time-clock service touches (Business + Staff for tenant rows, Position +
// StaffPosition so the labor-actuals aggregate can join the primary pay rate,
// TimeEntry, and RBACAuditLog so the approve audit persists), registers it as
// the package DB, and returns the wrapper. Mirrors newAvailabilityTestDB.
func newTimeEntryTestDB(t *testing.T) *DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Error),
	})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&Business{}, &Staff{}, &Position{}, &StaffPosition{}, &TimeEntry{}, &RBACAuditLog{}))
	// Partial unique: one open punch per (business, staff) — mirrors the genesis baseline index.
	require.NoError(t, gormDB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_time_entries_one_open ON time_entries (business_id, staff_id) WHERE status = 'open'").Error)
	prev := db
	SetTestDB(gormDB)
	t.Cleanup(func() { SetTestDB(prev) })
	return GetDBWrapper()
}

// seedTimeEntryBusiness inserts a business + an active staff member. Returns the
// staff id.
func seedTimeEntryBusiness(t *testing.T, businessID uint) (staffID uint) {
	t.Helper()
	require.NoError(t, db.Create(&Business{ID: businessID, BusinessId: fmt.Sprintf("biz-%d", businessID)}).Error)
	st := Staff{BusinessID: businessID, Email: fmt.Sprintf("s%d@biz%d.test", businessID, businessID), Name: "Sam", Role: "server", IsActive: true}
	require.NoError(t, db.Create(&st).Error)
	return st.ID
}

// TestTimeEntryGenesisShape is the genesis-safety guard: on a fresh DB the SQL
// migration is force-baselined WITHOUT running its DDL, so GORM autoMigrate must
// materialize the schema from the struct tags. This asserts the table, the
// explicit TableName(), every column the 000106 DDL declares, the named indexes
// the DDL creates (or a force-baselined fresh DB silently loses the timesheet
// read + manager review queue), and the DB-side source/status defaults.
func TestTimeEntryGenesisShape(t *testing.T) {
	newTimeEntryTestDB(t)

	require.Equal(t, "time_entries", TimeEntry{}.TableName())

	m := db.Migrator()
	require.True(t, m.HasTable("time_entries"))

	for _, col := range []string{
		"business_id", "staff_id", "shift_id", "clock_in_at", "clock_out_at",
		"break_minutes", "source", "status", "approved_by_staff_id", "note",
		"created_at", "updated_at",
	} {
		require.Truef(t, m.HasColumn(&TimeEntry{}, col), "time_entries missing column %s", col)
	}

	// Genesis index parity: the struct tags must materialize the SAME named
	// indexes the DDL creates, or a force-baselined fresh DB loses the
	// (business_id, staff_id, clock_in_at) timesheet read and the
	// (business_id, status) manager review queue.
	for _, idx := range []string{"idx_time_entries_biz_staff_clockin", "idx_time_entries_biz_status"} {
		require.Truef(t, m.HasIndex(&TimeEntry{}, idx), "time_entries missing genesis index %s", idx)
	}

	staffID := seedTimeEntryBusiness(t, 1)

	e := TimeEntry{BusinessID: 1, StaffID: staffID, ClockInAt: time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)}
	require.NoError(t, db.Create(&e).Error)

	var reloaded TimeEntry
	require.NoError(t, db.First(&reloaded, e.ID).Error)
	require.Equal(t, TimeEntrySourceStaffPunch, reloaded.Source, "source must DB-default to staff_punch")
	require.Equal(t, TimeEntryStatusOpen, reloaded.Status, "status must DB-default to open")
	require.Equal(t, 0, reloaded.BreakMinutes)
	require.Nil(t, reloaded.ClockOutAt)
	require.Nil(t, reloaded.ApprovedByStaffID)
}

func TestWorkedMinutesMath(t *testing.T) {
	in := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	out := in.Add(8 * time.Hour)

	// Open entry (no clock-out) => 0 worked minutes.
	require.Equal(t, 0, WorkedMinutes(TimeEntry{ClockInAt: in}))

	// 8h with no break => 480 minutes.
	require.Equal(t, 480, WorkedMinutes(TimeEntry{ClockInAt: in, ClockOutAt: &out}))

	// 8h minus a 30m break => 450 minutes.
	require.Equal(t, 450, WorkedMinutes(TimeEntry{ClockInAt: in, ClockOutAt: &out, BreakMinutes: 30}))

	// A break larger than the elapsed time can never drive worked minutes
	// negative (defensive clamp).
	require.Equal(t, 0, WorkedMinutes(TimeEntry{ClockInAt: in, ClockOutAt: &out, BreakMinutes: 999}))
}

func TestClockInRejectsDoubleClockIn(t *testing.T) {
	d := newTimeEntryTestDB(t)
	staffID := seedTimeEntryBusiness(t, 1)

	e1, err := d.ClockIn(1, staffID, nil)
	require.NoError(t, err)
	require.NotZero(t, e1.ID)
	require.Equal(t, TimeEntryStatusOpen, e1.Status)
	require.Equal(t, TimeEntrySourceStaffPunch, e1.Source)

	// A second clock-in while one is open is rejected.
	_, err = d.ClockIn(1, staffID, nil)
	require.ErrorIs(t, err, ErrAlreadyClockedIn)

	// Exactly one open entry exists.
	var open int64
	require.NoError(t, db.Model(&TimeEntry{}).Where("business_id = ? AND staff_id = ? AND status = ?", 1, staffID, TimeEntryStatusOpen).Count(&open).Error)
	require.Equal(t, int64(1), open)

	// After clock-out a fresh clock-in is allowed again.
	_, err = d.ClockOut(1, staffID)
	require.NoError(t, err)
	e2, err := d.ClockIn(1, staffID, nil)
	require.NoError(t, err)
	require.NotEqual(t, e1.ID, e2.ID)
}

func TestClockOutFlipsStatusAndRejectsWhenNotClockedIn(t *testing.T) {
	d := newTimeEntryTestDB(t)
	staffID := seedTimeEntryBusiness(t, 1)

	// Clock-out with no open entry => ErrNotClockedIn.
	_, err := d.ClockOut(1, staffID)
	require.ErrorIs(t, err, ErrNotClockedIn)

	_, err = d.ClockIn(1, staffID, nil)
	require.NoError(t, err)

	closed, err := d.ClockOut(1, staffID)
	require.NoError(t, err)
	require.Equal(t, TimeEntryStatusPendingReview, closed.Status, "clock-out flips status to pending_review")
	require.NotNil(t, closed.ClockOutAt)

	// A second clock-out is rejected (no open entry remains).
	_, err = d.ClockOut(1, staffID)
	require.ErrorIs(t, err, ErrNotClockedIn)
}

func TestAddBreakBounds(t *testing.T) {
	d := newTimeEntryTestDB(t)
	staffID := seedTimeEntryBusiness(t, 1)

	// No open entry yet.
	_, err := d.AddBreak(1, staffID, 10)
	require.ErrorIs(t, err, ErrNotClockedIn)

	// Open an entry that started 60 minutes ago so a break can be added.
	e := TimeEntry{BusinessID: 1, StaffID: staffID, ClockInAt: time.Now().UTC().Add(-60 * time.Minute), Source: TimeEntrySourceStaffPunch, Status: TimeEntryStatusOpen}
	require.NoError(t, db.Create(&e).Error)

	// Negative minutes rejected.
	_, err = d.AddBreak(1, staffID, -5)
	require.ErrorIs(t, err, ErrInvalidBreak)

	// A 15m break inside the 60m elapsed is accepted.
	updated, err := d.AddBreak(1, staffID, 15)
	require.NoError(t, err)
	require.Equal(t, 15, updated.BreakMinutes)

	// Adding 30 more (45 total < 60 elapsed) is fine.
	updated, err = d.AddBreak(1, staffID, 30)
	require.NoError(t, err)
	require.Equal(t, 45, updated.BreakMinutes)

	// Exceeding elapsed is rejected and does not mutate break_minutes.
	_, err = d.AddBreak(1, staffID, 100)
	require.ErrorIs(t, err, ErrBreakExceedsElapsed)
	var reloaded TimeEntry
	require.NoError(t, db.First(&reloaded, e.ID).Error)
	require.Equal(t, 45, reloaded.BreakMinutes)
}

func TestApproveTimeEntryCASAndAudit(t *testing.T) {
	d := newTimeEntryTestDB(t)
	staffID := seedTimeEntryBusiness(t, 1)
	const approver = uint(42)

	_, err := d.ClockIn(1, staffID, nil)
	require.NoError(t, err)
	closed, err := d.ClockOut(1, staffID)
	require.NoError(t, err)

	approved, err := d.ApproveTimeEntry(1, closed.ID, approver)
	require.NoError(t, err)
	require.Equal(t, TimeEntryStatusApproved, approved.Status)
	require.NotNil(t, approved.ApprovedByStaffID)
	require.Equal(t, approver, *approved.ApprovedByStaffID)

	// An RBAC audit row was written for the approval.
	var audits int64
	require.NoError(t, db.Model(&RBACAuditLog{}).
		Where("business_id = ? AND action = ?", 1, RBACActionTimeEntryApproved).Count(&audits).Error)
	require.Equal(t, int64(1), audits, "approving must write an RBAC audit row")

	// Re-approving an already-approved entry is an illegal transition.
	_, err = d.ApproveTimeEntry(1, closed.ID, approver)
	require.ErrorIs(t, err, ErrTimeEntryNotPendingReview)

	// Approving an OPEN entry (not yet clocked out) is also illegal.
	openE, err := d.ClockIn(1, staffID, nil)
	require.NoError(t, err)
	_, err = d.ApproveTimeEntry(1, openE.ID, approver)
	require.ErrorIs(t, err, ErrTimeEntryNotPendingReview)

	// Cross-tenant / unknown entry => not found.
	require.NoError(t, db.Create(&Business{ID: 2, BusinessId: "biz-2"}).Error)
	_, err = d.ApproveTimeEntry(2, closed.ID, approver)
	require.ErrorIs(t, err, ErrTimeEntryNotFound)
	_, err = d.ApproveTimeEntry(1, 999999, approver)
	require.ErrorIs(t, err, ErrTimeEntryNotFound)
}

func TestCreateManualEntry(t *testing.T) {
	d := newTimeEntryTestDB(t)
	staffID := seedTimeEntryBusiness(t, 1)
	in := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	out := in.Add(8 * time.Hour)

	e, err := d.CreateManualEntry(1, staffID, nil, in, &out, 30, "forgot to clock in")
	require.NoError(t, err)
	require.NotZero(t, e.ID)
	require.Equal(t, TimeEntrySourceManagerManual, e.Source)
	require.Equal(t, TimeEntryStatusPendingReview, e.Status, "manual entries land straight in pending_review")
	require.Equal(t, 450, WorkedMinutes(*e))

	// clock_out must be after clock_in.
	_, err = d.CreateManualEntry(1, staffID, nil, out, &in, 0, "bad")
	require.ErrorIs(t, err, ErrTimeEntryInvalidRange)
	_, err = d.CreateManualEntry(1, staffID, nil, in, nil, 0, "no out")
	require.ErrorIs(t, err, ErrTimeEntryInvalidRange)

	// break cannot exceed the elapsed time.
	_, err = d.CreateManualEntry(1, staffID, nil, in, &out, 999, "too much break")
	require.ErrorIs(t, err, ErrBreakExceedsElapsed)

	// staff must belong to the business.
	_, err = d.CreateManualEntry(1, 999999, nil, in, &out, 0, "ghost")
	require.ErrorIs(t, err, ErrStaffNotFound)
}

func TestListTimesheetRowScoping(t *testing.T) {
	d := newTimeEntryTestDB(t)
	staffA := seedTimeEntryBusiness(t, 1)
	other := Staff{BusinessID: 1, Email: "b@b1.test", Name: "B", Role: "server", IsActive: true}
	require.NoError(t, db.Create(&other).Error)
	staffB := other.ID
	in := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	out := in.Add(time.Hour)

	_, err := d.CreateManualEntry(1, staffA, nil, in, &out, 0, "a")
	require.NoError(t, err)
	_, err = d.CreateManualEntry(1, staffB, nil, in, &out, 0, "b")
	require.NoError(t, err)

	mine, err := d.ListTimesheet(1, staffA)
	require.NoError(t, err)
	require.Len(t, mine, 1)
	require.Equal(t, staffA, mine[0].StaffID)
}

func TestListForReviewFiltersByStatus(t *testing.T) {
	d := newTimeEntryTestDB(t)
	staffID := seedTimeEntryBusiness(t, 1)
	// Non-overlapping windows — CreateManualEntry rejects same-interval doubles.
	pendingIn := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	pendingOut := pendingIn.Add(time.Hour)
	approvedIn := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	approvedOut := approvedIn.Add(time.Hour)

	pending, err := d.CreateManualEntry(1, staffID, nil, pendingIn, &pendingOut, 0, "p")
	require.NoError(t, err)
	approvedSrc, err := d.CreateManualEntry(1, staffID, nil, approvedIn, &approvedOut, 0, "a")
	require.NoError(t, err)
	_, err = d.ApproveTimeEntry(1, approvedSrc.ID, 42)
	require.NoError(t, err)

	// Default (status "") is the pending-review queue.
	queue, err := d.ListForReview(1, "")
	require.NoError(t, err)
	require.Len(t, queue, 1)
	require.Equal(t, pending.ID, queue[0].ID)

	approved, err := d.ListForReview(1, TimeEntryStatusApproved)
	require.NoError(t, err)
	require.Len(t, approved, 1)
	require.Equal(t, approvedSrc.ID, approved[0].ID)
}

// timeEntrySQLRecorder captures the SQL the timesheet/review reads emit so the
// access-shape test can assert the query stays narrow + bounded (perf gate).
type timeEntrySQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *timeEntrySQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func (r *timeEntrySQLRecorder) selectsFromTimeEntries() []string {
	var out []string
	for _, s := range r.statements {
		n := strings.ToLower(strings.TrimSpace(s))
		if strings.HasPrefix(n, "select") && (strings.Contains(n, "from `time_entries`") || strings.Contains(n, `from "time_entries"`) || strings.Contains(n, "from time_entries")) {
			out = append(out, n)
		}
	}
	return out
}

// TestListForReviewAccessShape is the perf access-shape gate: the manager review
// queue and the staff timesheet read must each be a SINGLE bounded query over an
// explicit projection (no SELECT *, no N+1), so the (business_id, status) /
// (business_id, staff_id, clock_in_at) indexes carry the load.
func TestListForReviewAccessShape(t *testing.T) {
	recorder := &timeEntrySQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	d := newTimeEntryTestDB(t)
	staffID := seedTimeEntryBusiness(t, 1)
	in := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		clockIn := in.AddDate(0, 0, i)
		clockOut := clockIn.Add(time.Hour)
		_, err := d.CreateManualEntry(1, staffID, nil, clockIn, &clockOut, 0, "x")
		require.NoError(t, err)
	}

	// Manager review queue.
	recorder.statements = nil
	db.Logger = recorder
	_, err := d.ListForReview(1, TimeEntryStatusPendingReview)
	require.NoError(t, err)
	db.Logger = logger.Default.LogMode(logger.Error)

	stmts := recorder.selectsFromTimeEntries()
	require.Len(t, stmts, 1, "review queue must be exactly one query, got: %v", recorder.statements)
	require.NotContains(t, stmts[0], "select *", "must not SELECT * the time_entries row")
	require.NotContains(t, stmts[0], "`time_entries`.*", "must use an explicit projection")
	require.Contains(t, stmts[0], "limit", "list must be bounded by a LIMIT")

	// Staff timesheet read (scoped to own staff_id).
	recorder.statements = nil
	db.Logger = recorder
	_, err = d.ListTimesheet(1, staffID)
	require.NoError(t, err)
	db.Logger = logger.Default.LogMode(logger.Error)

	stmts = recorder.selectsFromTimeEntries()
	require.Len(t, stmts, 1, "timesheet read must be exactly one query")
	require.NotContains(t, stmts[0], "select *")
	require.Contains(t, stmts[0], "staff_id", "timesheet read must be row-scoped by staff_id in SQL")
	require.Contains(t, stmts[0], "limit")
}

// BenchmarkListForReview is the perf baseline for the manager review queue.
func BenchmarkListForReview(b *testing.B) {
	dsn := fmt.Sprintf("file:bench-timeentry-%d?mode=memory&cache=shared", timeEntrySvcBenchSeq.Add(1))
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(b, err)
	sqlDB, err := gormDB.DB()
	require.NoError(b, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(b, gormDB.AutoMigrate(&Business{}, &Staff{}, &TimeEntry{}))
	prev := db
	SetTestDB(gormDB)
	defer SetTestDB(prev)
	d := GetDBWrapper()

	require.NoError(b, db.Create(&Business{ID: 1, BusinessId: "biz-bench"}).Error)
	st := Staff{BusinessID: 1, Email: "bench@b1.test", Name: "Bench", Role: "server", IsActive: true}
	require.NoError(b, db.Create(&st).Error)
	in := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	out := in.Add(time.Hour)
	for i := 0; i < 200; i++ {
		require.NoError(b, db.Create(&TimeEntry{BusinessID: 1, StaffID: st.ID, ClockInAt: in.AddDate(0, 0, i), ClockOutAt: &out, Source: TimeEntrySourceManagerManual, Status: TimeEntryStatusPendingReview}).Error)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := d.ListForReview(1, TimeEntryStatusPendingReview)
		require.NoError(b, err)
	}
}

func TestGetApprovedWorkedMinutesAggregates(t *testing.T) {
	d := newTimeEntryTestDB(t)
	staffA := seedTimeEntryBusiness(t, 1)
	other := Staff{BusinessID: 1, Email: "b@b1.test", Name: "B", Role: "server", IsActive: true}
	require.NoError(t, db.Create(&other).Error)
	staffB := other.ID

	// staffA has a primary position @ $20/h; staffB has none (rate 0).
	pos := Position{BusinessID: 1, Name: "Server", IsActive: true}
	require.NoError(t, db.Create(&pos).Error)
	require.NoError(t, db.Create(&StaffPosition{BusinessID: 1, StaffID: staffA, PositionID: pos.ID, PayRateCents: 2000, IsPrimary: true}).Error)

	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 7)

	mk := func(staffID uint, clockIn time.Time, dur time.Duration, breakMin int, status string) {
		out := clockIn.Add(dur)
		require.NoError(t, db.Create(&TimeEntry{
			BusinessID: 1, StaffID: staffID, ClockInAt: clockIn, ClockOutAt: &out,
			BreakMinutes: breakMin, Status: status, Source: TimeEntrySourceManagerManual,
		}).Error)
	}

	// staffA: 8h - 30m break = 450, plus 2h = 120 -> 570 approved minutes in window.
	mk(staffA, start.Add(9*time.Hour), 8*time.Hour, 30, TimeEntryStatusApproved)
	mk(staffA, start.AddDate(0, 0, 1).Add(9*time.Hour), 2*time.Hour, 0, TimeEntryStatusApproved)
	// staffA: pending_review entry must NOT count.
	mk(staffA, start.AddDate(0, 0, 2).Add(9*time.Hour), 5*time.Hour, 0, TimeEntryStatusPendingReview)
	// staffA: approved but OUTSIDE the window must NOT count.
	mk(staffA, start.AddDate(0, 0, 30).Add(9*time.Hour), 5*time.Hour, 0, TimeEntryStatusApproved)
	// staffA: still-open entry (no clock-out) must NOT count.
	require.NoError(t, db.Create(&TimeEntry{BusinessID: 1, StaffID: staffA, ClockInAt: start.Add(time.Hour), Status: TimeEntryStatusOpen, Source: TimeEntrySourceStaffPunch}).Error)
	// staffB: 3h = 180 approved minutes, no primary position -> rate 0.
	mk(staffB, start.AddDate(0, 0, 1).Add(9*time.Hour), 3*time.Hour, 0, TimeEntryStatusApproved)

	rows, err := d.GetApprovedWorkedMinutes(1, start, end)
	require.NoError(t, err)

	byStaff := map[uint]StaffWorkedMinutes{}
	for _, r := range rows {
		byStaff[r.StaffID] = r
	}
	require.Len(t, byStaff, 2, "only staff with approved in-window entries appear")

	require.Equal(t, 570, byStaff[staffA].Minutes)
	require.Equal(t, int64(2000), byStaff[staffA].RateCents)

	require.Equal(t, 180, byStaff[staffB].Minutes)
	require.Equal(t, int64(0), byStaff[staffB].RateCents, "no primary position -> rate 0")
}

// TestGetApprovedWorkedMinutesAccessShape asserts the actuals aggregate is a
// SINGLE GROUP BY query (not per-row hydration) over an explicit projection.
func TestListTimesheetRange(t *testing.T) {
	d := newTimeEntryTestDB(t)
	staffID := seedTimeEntryBusiness(t, 1)
	mk := func(day int) {
		in := time.Date(2026, 7, day, 9, 0, 0, 0, time.UTC)
		out := in.Add(8 * time.Hour)
		_, err := d.CreateManualEntry(1, staffID, nil, in, &out, 30, "x")
		require.NoError(t, err)
	}
	mk(1)
	mk(8)
	mk(20)

	from := time.Date(2026, 7, 5, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC) // half-open [from, to)
	rows, err := d.ListTimesheetRange(1, staffID, from, to, "")
	require.NoError(t, err)
	require.Len(t, rows, 1, "only the Jul 8 entry falls in [Jul 5, Jul 15)")
}

func TestListTimesheetRangeAccessShape(t *testing.T) {
	recorder := &timeEntrySQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	d := newTimeEntryTestDB(t)
	staffID := seedTimeEntryBusiness(t, 1)
	in := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	out := in.Add(time.Hour)
	_, err := d.CreateManualEntry(1, staffID, nil, in, &out, 0, "x")
	require.NoError(t, err)

	recorder.statements = nil
	db.Logger = recorder
	_, err = d.ListTimesheetRange(1, staffID, in.AddDate(0, 0, -1), in.AddDate(0, 0, 1), "")
	require.NoError(t, err)
	db.Logger = logger.Default.LogMode(logger.Error)

	stmts := recorder.selectsFromTimeEntries()
	require.Len(t, stmts, 1, "range read must be exactly one query")
	require.NotContains(t, stmts[0], "select *")
	require.Contains(t, stmts[0], "staff_id", "range read must be row-scoped by staff_id in SQL")
	require.Contains(t, stmts[0], "clock_in_at", "range read must filter on clock_in_at (uses the composite index)")
	require.Contains(t, stmts[0], "limit")
}

func BenchmarkListTimesheetRange(b *testing.B) {
	dsn := fmt.Sprintf("file:bench-tsrange-%d?mode=memory&cache=shared", timeEntrySvcBenchSeq.Add(1))
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(b, err)
	sqlDB, err := gormDB.DB()
	require.NoError(b, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(b, gormDB.AutoMigrate(&Business{}, &Staff{}, &TimeEntry{}))
	prev := db
	SetTestDB(gormDB)
	defer SetTestDB(prev)
	d := GetDBWrapper()
	require.NoError(b, db.Create(&Business{ID: 1, BusinessId: "biz-bench"}).Error)
	st := Staff{BusinessID: 1, Email: "bench@b1.test", Name: "Bench", Role: "server", IsActive: true}
	require.NoError(b, db.Create(&st).Error)
	base := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	out := base.Add(time.Hour)
	for i := 0; i < 200; i++ {
		require.NoError(b, db.Create(&TimeEntry{BusinessID: 1, StaffID: st.ID, ClockInAt: base.AddDate(0, 0, i), ClockOutAt: &out, Source: TimeEntrySourceManagerManual, Status: TimeEntryStatusApproved}).Error)
	}
	from := base.AddDate(0, 0, 30)
	to := base.AddDate(0, 0, 90)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := d.ListTimesheetRange(1, st.ID, from, to, ""); err != nil {
			b.Fatal(err)
		}
	}
}

func TestGetApprovedWorkedMinutesAccessShape(t *testing.T) {
	recorder := &timeEntrySQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	d := newTimeEntryTestDB(t)
	staffID := seedTimeEntryBusiness(t, 1)
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 7)
	for i := 0; i < 5; i++ {
		in := start.AddDate(0, 0, i).Add(9 * time.Hour)
		out := in.Add(time.Hour)
		require.NoError(t, db.Create(&TimeEntry{BusinessID: 1, StaffID: staffID, ClockInAt: in, ClockOutAt: &out, Status: TimeEntryStatusApproved, Source: TimeEntrySourceManagerManual}).Error)
	}

	recorder.statements = nil
	db.Logger = recorder
	_, err := d.GetApprovedWorkedMinutes(1, start, end)
	require.NoError(t, err)
	db.Logger = logger.Default.LogMode(logger.Error)

	stmts := recorder.selectsFromTimeEntries()
	require.Len(t, stmts, 1, "actuals aggregate must be exactly one query, got: %v", recorder.statements)
	require.NotContains(t, stmts[0], "select *", "must not SELECT * the time_entries row")
	require.Contains(t, stmts[0], "group by", "must aggregate in SQL (GROUP BY), not per-row")
	require.Contains(t, stmts[0], "sum(", "must SUM worked minutes in SQL")
}

// TestRejectTimeEntry proves reject is a CAS from pending_review → rejected that
// records the reason (in Note) + an RBAC audit row and returns the entry to the
// staffer's attention. Rejecting a non-pending entry is an illegal transition;
// cross-tenant/unknown is not-found.
func TestRejectTimeEntry(t *testing.T) {
	d := newTimeEntryTestDB(t)
	staffID := seedTimeEntryBusiness(t, 1)
	const manager = uint(42)

	_, err := d.ClockIn(1, staffID, nil)
	require.NoError(t, err)
	closed, err := d.ClockOut(1, staffID)
	require.NoError(t, err)

	rejected, err := d.RejectTimeEntry(1, closed.ID, manager, "clock-out looks wrong")
	require.NoError(t, err)
	require.Equal(t, TimeEntryStatusRejected, rejected.Status)
	require.Contains(t, rejected.Note, "clock-out looks wrong")

	// An RBAC audit row was written for the rejection.
	var audits int64
	require.NoError(t, db.Model(&RBACAuditLog{}).
		Where("business_id = ? AND action = ?", 1, RBACActionTimeEntryRejected).Count(&audits).Error)
	require.Equal(t, int64(1), audits)

	// Re-rejecting a rejected entry is an illegal transition.
	_, err = d.RejectTimeEntry(1, closed.ID, manager, "again")
	require.ErrorIs(t, err, ErrTimeEntryNotPendingReview)

	// Approving a REJECTED entry is also illegal (it must be edited back to
	// pending or re-submitted, not silently approved).
	_, err = d.ApproveTimeEntry(1, closed.ID, manager)
	require.ErrorIs(t, err, ErrTimeEntryNotPendingReview)

	// Cross-tenant / unknown => not found.
	require.NoError(t, db.Create(&Business{ID: 2, BusinessId: "biz-2"}).Error)
	_, err = d.RejectTimeEntry(2, closed.ID, manager, "x")
	require.ErrorIs(t, err, ErrTimeEntryNotFound)
	_, err = d.RejectTimeEntry(1, 999999, manager, "x")
	require.ErrorIs(t, err, ErrTimeEntryNotFound)
}

// TestEditTimeEntry proves a manager edit adjusts clock-in/out (+break/note),
// validates the range, keeps the entry reviewable, and records an RBAC audit row
// preserving the ORIGINAL clock-in/out so the change is accountable (the model
// has no dedicated audit columns).
func TestEditTimeEntry(t *testing.T) {
	d := newTimeEntryTestDB(t)
	staffID := seedTimeEntryBusiness(t, 1)
	const manager = uint(42)

	_, err := d.ClockIn(1, staffID, nil)
	require.NoError(t, err)
	closed, err := d.ClockOut(1, staffID)
	require.NoError(t, err)
	origIn := closed.ClockInAt

	newIn := origIn.Add(-30 * time.Minute)
	newOut := origIn.Add(8 * time.Hour)
	edited, err := d.EditTimeEntry(1, closed.ID, manager, newIn, &newOut, 15, "adjusted for early start")
	require.NoError(t, err)
	require.Equal(t, newIn.UTC(), edited.ClockInAt.UTC())
	require.NotNil(t, edited.ClockOutAt)
	require.Equal(t, newOut.UTC(), edited.ClockOutAt.UTC())
	require.Equal(t, 15, edited.BreakMinutes)
	require.Contains(t, edited.Note, "adjusted for early start")
	// Still reviewable after an edit (a manager fixes then approves).
	require.Equal(t, TimeEntryStatusPendingReview, edited.Status)

	// The audit row preserves the original clock-in (accountability, no columns).
	var audit RBACAuditLog
	require.NoError(t, db.Model(&RBACAuditLog{}).
		Where("business_id = ? AND action = ?", 1, RBACActionTimeEntryEdited).
		First(&audit).Error)
	require.Contains(t, audit.Reason, origIn.UTC().Format(time.RFC3339))

	// An invalid range (out <= in) is rejected before persist.
	bad := origIn.Add(-time.Hour)
	_, err = d.EditTimeEntry(1, closed.ID, manager, newIn, &bad, 0, "")
	require.ErrorIs(t, err, ErrTimeEntryInvalidRange)

	// An APPROVED entry cannot be edited (locked once approved).
	approved, err := d.ApproveTimeEntry(1, closed.ID, manager)
	require.NoError(t, err)
	require.Equal(t, TimeEntryStatusApproved, approved.Status)
	_, err = d.EditTimeEntry(1, closed.ID, manager, newIn, &newOut, 0, "")
	require.ErrorIs(t, err, ErrTimeEntryNotEditable)

	// Cross-tenant / unknown => not found.
	require.NoError(t, db.Create(&Business{ID: 2, BusinessId: "biz-2"}).Error)
	_, err = d.EditTimeEntry(2, closed.ID, manager, newIn, &newOut, 0, "")
	require.ErrorIs(t, err, ErrTimeEntryNotFound)
}

// TestListForReviewPaged proves the paginated manager review read: offset/limit
// window a bounded page, from/to bound a date window, and total reflects the FULL
// matching set (not the page). Legacy ListForReview stays unchanged.
func TestListForReviewPaged(t *testing.T) {
	d := newTimeEntryTestDB(t)
	staffID := seedTimeEntryBusiness(t, 1)
	in := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 25; i++ {
		clockIn := in.AddDate(0, 0, i)
		clockOut := clockIn.Add(time.Hour)
		_, err := d.CreateManualEntry(1, staffID, nil, clockIn, &clockOut, 0, "x")
		require.NoError(t, err)
	}

	// First page of 10 over the full pending set.
	page, total, err := d.ListForReviewPaged(1, TimeEntryStatusPendingReview, time.Time{}, time.Time{}, 0, 10)
	require.NoError(t, err)
	require.Equal(t, int64(25), total, "total reflects the full matching set")
	require.Len(t, page, 10)

	// Second page.
	page2, total2, err := d.ListForReviewPaged(1, TimeEntryStatusPendingReview, time.Time{}, time.Time{}, 10, 10)
	require.NoError(t, err)
	require.Equal(t, int64(25), total2)
	require.Len(t, page2, 10)
	require.NotEqual(t, page[0].ID, page2[0].ID, "pages don't overlap")

	// Date window: only entries clocking in during the first 5 days.
	from := in
	to := in.AddDate(0, 0, 5)
	windowed, wTotal, err := d.ListForReviewPaged(1, TimeEntryStatusPendingReview, from, to, 0, 100)
	require.NoError(t, err)
	require.Equal(t, int64(5), wTotal, "window bounds the total")
	require.Len(t, windowed, 5)

	// Limit is clamped (a huge/zero limit doesn't unbound the read).
	_, _, err = d.ListForReviewPaged(1, TimeEntryStatusPendingReview, time.Time{}, time.Time{}, 0, 100000)
	require.NoError(t, err)
}

// TestListForReviewPagedAccessShape locks the paged read to a bounded, N+1-free
// shape: a COUNT + one bounded projected SELECT, no SELECT *.
func TestListForReviewPagedAccessShape(t *testing.T) {
	recorder := &timeEntrySQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	d := newTimeEntryTestDB(t)
	staffID := seedTimeEntryBusiness(t, 1)
	in := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 12; i++ {
		clockIn := in.AddDate(0, 0, i)
		clockOut := clockIn.Add(time.Hour)
		_, err := d.CreateManualEntry(1, staffID, nil, clockIn, &clockOut, 0, "x")
		require.NoError(t, err)
	}

	recorder.statements = nil
	db.Logger = recorder
	_, _, err := d.ListForReviewPaged(1, TimeEntryStatusPendingReview, time.Time{}, time.Time{}, 0, 10)
	require.NoError(t, err)
	db.Logger = logger.Default.LogMode(logger.Error)

	stmts := recorder.selectsFromTimeEntries()
	require.LessOrEqual(t, len(stmts), 2, "paged read is a COUNT + one page SELECT, never N+1; got %v", recorder.statements)
	for _, s := range stmts {
		require.NotContains(t, s, "select *", "must not SELECT * the time_entries row")
	}
}

// BenchmarkListForReviewPaged is the perf baseline for the paginated review read.
func BenchmarkListForReviewPaged(b *testing.B) {
	dsn := fmt.Sprintf("file:bench-timeentry-paged-%d?mode=memory&cache=shared", timeEntrySvcBenchSeq.Add(1))
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(b, err)
	sqlDB, err := gormDB.DB()
	require.NoError(b, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(b, gormDB.AutoMigrate(&Business{}, &Staff{}, &TimeEntry{}))
	prev := db
	SetTestDB(gormDB)
	defer SetTestDB(prev)
	d := GetDBWrapper()

	require.NoError(b, db.Create(&Business{ID: 1, BusinessId: "biz-bench"}).Error)
	st := Staff{BusinessID: 1, Email: "bench@b1.test", Name: "Bench", Role: "server", IsActive: true}
	require.NoError(b, db.Create(&st).Error)
	in := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	out := in.Add(time.Hour)
	for i := 0; i < 500; i++ {
		require.NoError(b, db.Create(&TimeEntry{BusinessID: 1, StaffID: st.ID, ClockInAt: in.AddDate(0, 0, i), ClockOutAt: &out, Source: TimeEntrySourceManagerManual, Status: TimeEntryStatusPendingReview}).Error)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := d.ListForReviewPaged(1, TimeEntryStatusPendingReview, time.Time{}, time.Time{}, 0, 50)
		require.NoError(b, err)
	}
}
