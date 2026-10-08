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

// availabilitySvcBenchSeq makes each benchmark-invocation's in-memory DB name
// unique (mirrors scheduleSvcBenchSeq in schedule_service_test.go).
var availabilitySvcBenchSeq atomic.Uint64

// newAvailabilityTestDB opens an isolated in-memory SQLite DB, migrates the
// tables the availability/time-off service touches (Business, Staff for tenant
// validation, StaffAvailability + TimeOffRequest, and RBACAuditLog so the
// decision audit write persists), registers it as the package DB, and returns
// the wrapper. Mirrors newScheduleTestDB.
func newAvailabilityTestDB(t *testing.T) *DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Error),
	})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&Business{}, &Staff{}, &StaffAvailability{}, &TimeOffRequest{}, &RBACAuditLog{}))
	prev := db
	SetTestDB(gormDB)
	t.Cleanup(func() { SetTestDB(prev) })
	return GetDBWrapper()
}

// seedAvailabilityBusiness inserts a business + an active staff member so the
// service has tenant rows to resolve. Returns the staff id.
func seedAvailabilityBusiness(t *testing.T, businessID uint) (staffID uint) {
	t.Helper()
	require.NoError(t, db.Create(&Business{ID: businessID, BusinessId: fmt.Sprintf("biz-%d", businessID)}).Error)
	st := Staff{BusinessID: businessID, Email: fmt.Sprintf("s%d@biz%d.test", businessID, businessID), Name: "Sam", Role: "server", IsActive: true}
	require.NoError(t, db.Create(&st).Error)
	return st.ID
}

// TestAvailabilityTimeOffGenesisShape is the genesis-safety guard: on a fresh DB
// the SQL migration is force-baselined WITHOUT running its DDL, so GORM
// autoMigrate must materialize the schema from the struct tags. This asserts the
// tables, the explicit TableName()s, every column the 000105 DDL declares, the
// named indexes the DDL creates (or a force-baselined fresh DB silently loses
// them), the DB-side status default, and that the availability index is
// (correctly) NON-unique — a staff may hold several windows per weekday.
func TestAvailabilityTimeOffGenesisShape(t *testing.T) {
	newAvailabilityTestDB(t)

	require.Equal(t, "staff_availabilities", StaffAvailability{}.TableName())
	require.Equal(t, "time_off_requests", TimeOffRequest{}.TableName())

	m := db.Migrator()
	require.True(t, m.HasTable("staff_availabilities"))
	require.True(t, m.HasTable("time_off_requests"))

	for _, col := range []string{"business_id", "staff_id", "weekday", "start_min", "end_min", "kind", "created_at", "updated_at"} {
		require.Truef(t, m.HasColumn(&StaffAvailability{}, col), "staff_availabilities missing column %s", col)
	}
	for _, col := range []string{"business_id", "staff_id", "starts_at", "ends_at", "reason", "status", "decided_by_staff_id", "decided_at", "created_at", "updated_at"} {
		require.Truef(t, m.HasColumn(&TimeOffRequest{}, col), "time_off_requests missing column %s", col)
	}

	// Genesis index parity: the struct tags must materialize the SAME named
	// indexes the DDL creates, or a force-baselined fresh DB loses the
	// (business_id, staff_id, weekday) overlay lookup and the time-off queues.
	require.True(t, m.HasIndex(&StaffAvailability{}, "idx_staff_availabilities_biz_staff_weekday"), "availability composite index must exist on genesis")
	for _, idx := range []string{"idx_time_off_requests_biz_status", "idx_time_off_requests_biz_staff"} {
		require.Truef(t, m.HasIndex(&TimeOffRequest{}, idx), "time_off_requests missing genesis index %s", idx)
	}

	staffID := seedAvailabilityBusiness(t, 1)

	// The availability index is NON-unique: a staff legitimately holds multiple
	// windows on the same weekday (e.g. preferred 09:00-12:00 + unavailable
	// 14:00-16:00). Two rows with the same (business_id, staff_id, weekday) must
	// BOTH insert.
	require.NoError(t, db.Create(&StaffAvailability{BusinessID: 1, StaffID: staffID, Weekday: 1, StartMin: 540, EndMin: 720, Kind: AvailabilityKindPreferred}).Error)
	require.NoError(t, db.Create(&StaffAvailability{BusinessID: 1, StaffID: staffID, Weekday: 1, StartMin: 840, EndMin: 960, Kind: AvailabilityKindUnavailable}).Error)

	req := TimeOffRequest{
		BusinessID: 1, StaffID: staffID,
		StartsAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		EndsAt:   time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC),
		Reason:   "vacation",
	}
	require.NoError(t, db.Create(&req).Error)

	var reloaded TimeOffRequest
	require.NoError(t, db.First(&reloaded, req.ID).Error)
	require.Equal(t, TimeOffStatusPending, reloaded.Status, "status must DB-default to pending")
	require.Nil(t, reloaded.DecidedByStaffID)
	require.Nil(t, reloaded.DecidedAt)
}

func TestGetAvailabilityEmpty(t *testing.T) {
	d := newAvailabilityTestDB(t)
	staffID := seedAvailabilityBusiness(t, 1)
	got, err := d.GetAvailability(1, staffID)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestReplaceAvailabilityRoundTrips(t *testing.T) {
	d := newAvailabilityTestDB(t)
	staffID := seedAvailabilityBusiness(t, 1)

	windows := []StaffAvailability{
		{Weekday: 1, StartMin: 540, EndMin: 720, Kind: AvailabilityKindPreferred},
		{Weekday: 1, StartMin: 840, EndMin: 960, Kind: AvailabilityKindUnavailable},
		{Weekday: 3, StartMin: 600, EndMin: 1020, Kind: AvailabilityKindPreferred},
	}
	saved, err := d.ReplaceAvailability(1, staffID, windows)
	require.NoError(t, err)
	require.Len(t, saved, 3)
	for _, w := range saved {
		require.Equal(t, uint(1), w.BusinessID)
		require.Equal(t, staffID, w.StaffID, "service must stamp the caller's own staff_id")
		require.NotZero(t, w.ID)
	}

	// Replacing again wipes the prior set (delete-then-insert), not appends.
	saved, err = d.ReplaceAvailability(1, staffID, []StaffAvailability{
		{Weekday: 5, StartMin: 0, EndMin: 1440, Kind: AvailabilityKindUnavailable},
	})
	require.NoError(t, err)
	require.Len(t, saved, 1)
	require.Equal(t, 5, saved[0].Weekday)

	var count int64
	require.NoError(t, db.Model(&StaffAvailability{}).Where("business_id = ? AND staff_id = ?", 1, staffID).Count(&count).Error)
	require.Equal(t, int64(1), count, "old windows replaced, not accumulated")

	// Replacing with an empty set clears all windows.
	saved, err = d.ReplaceAvailability(1, staffID, nil)
	require.NoError(t, err)
	require.Empty(t, saved)
	require.NoError(t, db.Model(&StaffAvailability{}).Where("business_id = ? AND staff_id = ?", 1, staffID).Count(&count).Error)
	require.Equal(t, int64(0), count)
}

func TestReplaceAvailabilityScopedToOwnStaff(t *testing.T) {
	d := newAvailabilityTestDB(t)
	staffA := seedAvailabilityBusiness(t, 1)
	other := Staff{BusinessID: 1, Email: "b@b1.test", Name: "B", Role: "server", IsActive: true}
	require.NoError(t, db.Create(&other).Error)
	staffB := other.ID

	// Seed B's availability.
	_, err := d.ReplaceAvailability(1, staffB, []StaffAvailability{
		{Weekday: 2, StartMin: 540, EndMin: 720, Kind: AvailabilityKindPreferred},
	})
	require.NoError(t, err)

	// A replaces "own" availability, but the body windows try to claim staffB.
	// The service must IGNORE the body staff_id and write only A's rows.
	saved, err := d.ReplaceAvailability(1, staffA, []StaffAvailability{
		{StaffID: staffB, Weekday: 4, StartMin: 600, EndMin: 900, Kind: AvailabilityKindPreferred},
	})
	require.NoError(t, err)
	require.Len(t, saved, 1)
	require.Equal(t, staffA, saved[0].StaffID, "body staff_id must be overridden with the caller's own id")

	// B's availability is untouched (the delete was scoped to A).
	bWindows, err := d.GetAvailability(1, staffB)
	require.NoError(t, err)
	require.Len(t, bWindows, 1)
	require.Equal(t, 2, bWindows[0].Weekday)
}

func TestReplaceAvailabilityValidation(t *testing.T) {
	d := newAvailabilityTestDB(t)
	staffID := seedAvailabilityBusiness(t, 1)

	cases := [][]StaffAvailability{
		{{Weekday: 7, StartMin: 540, EndMin: 720, Kind: AvailabilityKindPreferred}},  // weekday > 6
		{{Weekday: -1, StartMin: 540, EndMin: 720, Kind: AvailabilityKindPreferred}}, // weekday < 0
		{{Weekday: 1, StartMin: 720, EndMin: 720, Kind: AvailabilityKindPreferred}},  // start == end
		{{Weekday: 1, StartMin: 800, EndMin: 700, Kind: AvailabilityKindPreferred}},  // start > end
		{{Weekday: 1, StartMin: -1, EndMin: 720, Kind: AvailabilityKindPreferred}},   // start < 0
		{{Weekday: 1, StartMin: 540, EndMin: 1441, Kind: AvailabilityKindPreferred}}, // end > 1440
		{{Weekday: 1, StartMin: 540, EndMin: 720, Kind: "maybe"}},                    // bad kind
	}
	for i, windows := range cases {
		_, err := d.ReplaceAvailability(1, staffID, windows)
		require.ErrorIsf(t, err, ErrAvailabilityInvalid, "case %d must be rejected", i)
	}

	// Reject-before-persist: a batch with one bad window writes NOTHING.
	_, err := d.ReplaceAvailability(1, staffID, []StaffAvailability{
		{Weekday: 1, StartMin: 540, EndMin: 720, Kind: AvailabilityKindPreferred},
		{Weekday: 9, StartMin: 540, EndMin: 720, Kind: AvailabilityKindPreferred},
	})
	require.ErrorIs(t, err, ErrAvailabilityInvalid)
	var count int64
	require.NoError(t, db.Model(&StaffAvailability{}).Where("business_id = ?", 1).Count(&count).Error)
	require.Equal(t, int64(0), count, "an invalid batch must not partially persist")
}

// TestGetTeamAvailabilityGroupsByStaff proves the manager overlay read groups
// every staff member's windows by staff_id, keeps the weekday/start ordering, and
// never leaks another tenant's availability.
func TestGetTeamAvailabilityGroupsByStaff(t *testing.T) {
	d := newAvailabilityTestDB(t)
	staffA := seedAvailabilityBusiness(t, 1)
	other := Staff{BusinessID: 1, Email: "b@b1.test", Name: "B", Role: "server", IsActive: true}
	require.NoError(t, db.Create(&other).Error)
	staffB := other.ID

	_, err := d.ReplaceAvailability(1, staffA, []StaffAvailability{
		{Weekday: 1, StartMin: 540, EndMin: 720, Kind: AvailabilityKindPreferred},
		{Weekday: 1, StartMin: 840, EndMin: 960, Kind: AvailabilityKindUnavailable},
	})
	require.NoError(t, err)
	_, err = d.ReplaceAvailability(1, staffB, []StaffAvailability{
		{Weekday: 3, StartMin: 600, EndMin: 1020, Kind: AvailabilityKindPreferred},
	})
	require.NoError(t, err)

	// A second tenant's windows must never surface in business 1's overlay.
	require.NoError(t, db.Create(&Business{ID: 2, BusinessId: "biz-2"}).Error)
	otherBiz := Staff{BusinessID: 2, Email: "c@b2.test", Name: "C", Role: "server", IsActive: true}
	require.NoError(t, db.Create(&otherBiz).Error)
	_, err = d.ReplaceAvailability(2, otherBiz.ID, []StaffAvailability{
		{Weekday: 2, StartMin: 540, EndMin: 720, Kind: AvailabilityKindPreferred},
	})
	require.NoError(t, err)

	byStaff, err := d.GetTeamAvailability(1)
	require.NoError(t, err)
	require.Len(t, byStaff, 2, "one entry per staff with windows in business 1")
	require.Len(t, byStaff[staffA], 2)
	require.Len(t, byStaff[staffB], 1)
	require.NotContains(t, byStaff, otherBiz.ID, "another tenant's availability must not leak")
	// Grouped windows keep the (weekday, start_min) ordering.
	require.Equal(t, 540, byStaff[staffA][0].StartMin)
	require.Equal(t, 840, byStaff[staffA][1].StartMin)

	// A business with no windows returns a non-nil empty map (not nil).
	require.NoError(t, db.Create(&Business{ID: 3, BusinessId: "biz-3"}).Error)
	empty, err := d.GetTeamAvailability(3)
	require.NoError(t, err)
	require.NotNil(t, empty)
	require.Empty(t, empty)
}

// TestGetTeamAvailabilityAccessShape is the perf access-shape gate for the
// manager overlay read: it must be a SINGLE bounded query over an explicit
// projection (no SELECT *, no per-staff N+1), tenant-scoped in SQL.
func TestGetTeamAvailabilityAccessShape(t *testing.T) {
	cap := &sqlCapture{Interface: logger.Default.LogMode(logger.Silent)}
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: cap})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&Business{}, &Staff{}, &StaffAvailability{}))
	prev := db
	SetTestDB(gormDB)
	t.Cleanup(func() { SetTestDB(prev) })
	d := GetDBWrapper()

	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	// Several staff, several windows each — the overlay must stay one query.
	for i := 0; i < 4; i++ {
		st := Staff{BusinessID: 1, Email: fmt.Sprintf("s%d@b1.test", i), Name: "S", Role: "server", IsActive: true}
		require.NoError(t, db.Create(&st).Error)
		_, err := d.ReplaceAvailability(1, st.ID, []StaffAvailability{
			{Weekday: 1, StartMin: 540, EndMin: 720, Kind: AvailabilityKindPreferred},
			{Weekday: 2, StartMin: 540, EndMin: 720, Kind: AvailabilityKindUnavailable},
		})
		require.NoError(t, err)
	}

	cap.mu.Lock()
	cap.sqls = nil
	cap.mu.Unlock()

	byStaff, err := d.GetTeamAvailability(1)
	require.NoError(t, err)
	require.Len(t, byStaff, 4)

	var selects []string
	for _, q := range cap.matching("staff_availabilities") {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(q)), "select") {
			selects = append(selects, strings.ToLower(q))
		}
	}
	require.Len(t, selects, 1, "team availability must be exactly one query, not N+1 per staff")
	require.NotContains(t, selects[0], "select *", "must not SELECT * the availability row")
	require.NotContains(t, selects[0], "`staff_availabilities`.*", "must use an explicit projection")
	require.Contains(t, selects[0], "business_id", "read must be tenant-scoped in SQL")
	require.Contains(t, selects[0], "limit", "read must be bounded by a LIMIT")
}

// BenchmarkGetTeamAvailability is the perf baseline for the manager overlay read
// over a busy roster (run with -benchmem).
func BenchmarkGetTeamAvailability(b *testing.B) {
	dsn := fmt.Sprintf("file:bench-teamavail-%d?mode=memory&cache=shared", availabilitySvcBenchSeq.Add(1))
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(b, err)
	sqlDB, err := gormDB.DB()
	require.NoError(b, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(b, gormDB.AutoMigrate(&Business{}, &Staff{}, &StaffAvailability{}))
	prev := db
	SetTestDB(gormDB)
	defer SetTestDB(prev)
	d := GetDBWrapper()

	require.NoError(b, db.Create(&Business{ID: 1, BusinessId: "biz-bench"}).Error)
	// 25 staff × 10 windows each = a busy roster.
	for i := 0; i < 25; i++ {
		st := Staff{BusinessID: 1, Email: fmt.Sprintf("bench%d@b1.test", i), Name: "Bench", Role: "server", IsActive: true}
		require.NoError(b, db.Create(&st).Error)
		windows := make([]StaffAvailability, 0, 10)
		for w := 0; w < 5; w++ {
			windows = append(windows,
				StaffAvailability{Weekday: w, StartMin: 540, EndMin: 720, Kind: AvailabilityKindPreferred},
				StaffAvailability{Weekday: w, StartMin: 840, EndMin: 1020, Kind: AvailabilityKindUnavailable},
			)
		}
		_, err := d.ReplaceAvailability(1, st.ID, windows)
		require.NoError(b, err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := d.GetTeamAvailability(1)
		require.NoError(b, err)
	}
}

// timeOffSQLRecorder captures the SQL statements the time-off list emits so the
// access-shape test can assert the query stays narrow + bounded (perf gate).
type timeOffSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *timeOffSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func (r *timeOffSQLRecorder) selectsFromTimeOff() []string {
	var out []string
	for _, s := range r.statements {
		n := strings.ToLower(strings.TrimSpace(s))
		if strings.HasPrefix(n, "select") && (strings.Contains(n, "from `time_off_requests`") || strings.Contains(n, `from "time_off_requests"`)) {
			out = append(out, n)
		}
	}
	return out
}

// TestListTimeOffAccessShape is the perf access-shape gate: the manager queue
// and the staff "my requests" list must each be a SINGLE bounded query over an
// explicit projection (no SELECT *, no N+1), so the (business_id, status) /
// (business_id, staff_id) indexes carry the load and the result set is capped.
func TestListTimeOffAccessShape(t *testing.T) {
	recorder := &timeOffSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	d := newAvailabilityTestDB(t)
	staffID := seedAvailabilityBusiness(t, 1)
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		_, err := d.CreateTimeOff(1, staffID, base.AddDate(0, 0, i), base.AddDate(0, 0, i+1), "x")
		require.NoError(t, err)
	}

	// Manager queue (all by status, no staff scope).
	recorder.statements = nil
	db.Logger = recorder
	_, err := d.ListTimeOff(1, TimeOffStatusPending, nil)
	require.NoError(t, err)
	db.Logger = logger.Default.LogMode(logger.Error)

	stmts := recorder.selectsFromTimeOff()
	require.Len(t, stmts, 1, "manager list must be exactly one query, got: %v", recorder.statements)
	require.NotContains(t, stmts[0], "select *", "must not SELECT * the time_off_requests row")
	require.NotContains(t, stmts[0], "`time_off_requests`.*", "must use an explicit projection")
	require.Contains(t, stmts[0], "limit", "list must be bounded by a LIMIT")

	// Staff "my requests" list (scoped to own staff_id).
	recorder.statements = nil
	db.Logger = recorder
	scope := staffID
	_, err = d.ListTimeOff(1, "", &scope)
	require.NoError(t, err)
	db.Logger = logger.Default.LogMode(logger.Error)

	stmts = recorder.selectsFromTimeOff()
	require.Len(t, stmts, 1, "staff list must be exactly one query")
	require.NotContains(t, stmts[0], "select *")
	require.Contains(t, stmts[0], "staff_id", "staff list must be row-scoped by staff_id in SQL")
	require.Contains(t, stmts[0], "limit")
}

// BenchmarkListTimeOff is the perf baseline for the manager time-off queue
// (run with -benchmem).
func BenchmarkListTimeOff(b *testing.B) {
	dsn := fmt.Sprintf("file:bench-timeoff-%d?mode=memory&cache=shared", availabilitySvcBenchSeq.Add(1))
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(b, err)
	sqlDB, err := gormDB.DB()
	require.NoError(b, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(b, gormDB.AutoMigrate(&Business{}, &Staff{}, &TimeOffRequest{}))
	prev := db
	SetTestDB(gormDB)
	defer SetTestDB(prev)
	d := GetDBWrapper()

	require.NoError(b, db.Create(&Business{ID: 1, BusinessId: "biz-bench"}).Error)
	st := Staff{BusinessID: 1, Email: "bench@b1.test", Name: "Bench", Role: "server", IsActive: true}
	require.NoError(b, db.Create(&st).Error)
	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 200; i++ {
		_, err := d.CreateTimeOff(1, st.ID, base.AddDate(0, 0, i), base.AddDate(0, 0, i+1), "bench")
		require.NoError(b, err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := d.ListTimeOff(1, TimeOffStatusPending, nil)
		require.NoError(b, err)
	}
}

func TestCreateTimeOff(t *testing.T) {
	d := newAvailabilityTestDB(t)
	staffID := seedAvailabilityBusiness(t, 1)
	starts := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	ends := starts.AddDate(0, 0, 2)

	req, err := d.CreateTimeOff(1, staffID, starts, ends, "family")
	require.NoError(t, err)
	require.NotZero(t, req.ID)
	require.Equal(t, TimeOffStatusPending, req.Status, "new requests start pending")
	require.Equal(t, staffID, req.StaffID)
	require.Equal(t, "family", req.Reason)
	require.Nil(t, req.DecidedByStaffID)

	// end must be after start.
	_, err = d.CreateTimeOff(1, staffID, ends, starts, "bad")
	require.ErrorIs(t, err, ErrTimeOffInvalidRange)
	_, err = d.CreateTimeOff(1, staffID, starts, starts, "equal")
	require.ErrorIs(t, err, ErrTimeOffInvalidRange)
}

func TestListTimeOffRowScoping(t *testing.T) {
	d := newAvailabilityTestDB(t)
	staffA := seedAvailabilityBusiness(t, 1)
	other := Staff{BusinessID: 1, Email: "b@b1.test", Name: "B", Role: "server", IsActive: true}
	require.NoError(t, db.Create(&other).Error)
	staffB := other.ID
	base := time.Now().UTC().Add(24 * time.Hour)

	aReq, err := d.CreateTimeOff(1, staffA, base, base.AddDate(0, 0, 1), "a")
	require.NoError(t, err)
	_, err = d.CreateTimeOff(1, staffB, base, base.AddDate(0, 0, 1), "b")
	require.NoError(t, err)
	// A second tenant's request must never leak.
	require.NoError(t, db.Create(&Business{ID: 2, BusinessId: "biz-2"}).Error)
	otherBizStaff := Staff{BusinessID: 2, Email: "c@b2.test", Name: "C", Role: "server", IsActive: true}
	require.NoError(t, db.Create(&otherBizStaff).Error)
	_, err = d.CreateTimeOff(2, otherBizStaff.ID, base, base.AddDate(0, 0, 1), "c")
	require.NoError(t, err)

	// Manager view (scope nil): all pending in business 1, never business 2.
	all, err := d.ListTimeOff(1, TimeOffStatusPending, nil)
	require.NoError(t, err)
	require.Len(t, all, 2)
	for _, r := range all {
		require.Equal(t, uint(1), r.BusinessID)
	}

	// Staff view (scope = A): only A's own requests.
	scope := staffA
	mine, err := d.ListTimeOff(1, "", &scope)
	require.NoError(t, err)
	require.Len(t, mine, 1)
	require.Equal(t, aReq.ID, mine[0].ID)
	require.Equal(t, staffA, mine[0].StaffID)

	// Status filter narrows the manager queue.
	_, err = d.DecideTimeOff(1, aReq.ID, 999, true, "approved")
	require.NoError(t, err)
	stillPending, err := d.ListTimeOff(1, TimeOffStatusPending, nil)
	require.NoError(t, err)
	require.Len(t, stillPending, 1, "the approved request drops out of the pending queue")
	approved, err := d.ListTimeOff(1, TimeOffStatusApproved, nil)
	require.NoError(t, err)
	require.Len(t, approved, 1)
}

func TestDecideTimeOffStateMachine(t *testing.T) {
	d := newAvailabilityTestDB(t)
	staffID := seedAvailabilityBusiness(t, 1)
	base := time.Now().UTC().Add(24 * time.Hour)
	const approver = uint(42)

	// Approve happy path.
	req, err := d.CreateTimeOff(1, staffID, base, base.AddDate(0, 0, 1), "a")
	require.NoError(t, err)
	decided, err := d.DecideTimeOff(1, req.ID, approver, true, "ok")
	require.NoError(t, err)
	require.Equal(t, TimeOffStatusApproved, decided.Status)
	require.NotNil(t, decided.DecidedByStaffID)
	require.Equal(t, approver, *decided.DecidedByStaffID)
	require.NotNil(t, decided.DecidedAt)

	// An RBAC audit row was written for the decision (best-effort, but present
	// on the happy path).
	var audits int64
	require.NoError(t, db.Model(&RBACAuditLog{}).
		Where("business_id = ? AND action = ?", 1, RBACActionTimeOffDecided).Count(&audits).Error)
	require.Equal(t, int64(1), audits, "approving must write an RBAC audit row")

	// Deciding a non-pending request is an illegal transition.
	_, err = d.DecideTimeOff(1, req.ID, approver, false, "again")
	require.ErrorIs(t, err, ErrTimeOffNotPending)

	// Deny happy path on a fresh request.
	req2, err := d.CreateTimeOff(1, staffID, base, base.AddDate(0, 0, 1), "b")
	require.NoError(t, err)
	decided2, err := d.DecideTimeOff(1, req2.ID, approver, false, "no")
	require.NoError(t, err)
	require.Equal(t, TimeOffStatusDenied, decided2.Status)

	// Cross-tenant decision -> not found (never leaks another business's row).
	require.NoError(t, db.Create(&Business{ID: 2, BusinessId: "biz-2"}).Error)
	_, err = d.DecideTimeOff(2, req2.ID, approver, true, "x")
	require.ErrorIs(t, err, ErrTimeOffNotFound)

	// Deciding a wholly unknown request -> not found.
	_, err = d.DecideTimeOff(1, 999999, approver, true, "x")
	require.ErrorIs(t, err, ErrTimeOffNotFound)
}

func TestInstantElapsedBoundary(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	require.False(t, InstantElapsed(time.Time{}, now), "zero instant is not elapsed")
	require.False(t, InstantElapsed(now, now), "boundary instant is still live")
	require.False(t, InstantElapsed(now.Add(time.Nanosecond), now))
	require.True(t, InstantElapsed(now.Add(-time.Nanosecond), now))
}

func TestDecideTimeOffRejectsElapsedRequest(t *testing.T) {
	d := newAvailabilityTestDB(t)
	staffID := seedAvailabilityBusiness(t, 1)
	now := time.Now().UTC()
	const approver = uint(42)

	past, err := d.CreateTimeOff(1, staffID, now.Add(-48*time.Hour), now.Add(-time.Hour), "past")
	require.NoError(t, err)
	_, err = d.DecideTimeOff(1, past.ID, approver, true, "late")
	require.ErrorIs(t, err, ErrTimeOffExpired)

	// Instant still in the future remains decidable (boundary: ends_at == now is
	// not elapsed because InstantElapsed is strictly Before).
	live, err := d.CreateTimeOff(1, staffID, now.Add(time.Hour), now.Add(2*time.Hour), "live")
	require.NoError(t, err)
	decided, err := d.DecideTimeOff(1, live.ID, approver, true, "ok")
	require.NoError(t, err)
	require.Equal(t, TimeOffStatusApproved, decided.Status)
}
