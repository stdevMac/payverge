package database

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// scheduleSvcBenchSeq makes each benchmark-invocation's in-memory DB name unique
// (see scheduleBenchSeq in schedule_settings_service_test.go for the rationale).
var scheduleSvcBenchSeq atomic.Uint64

// newScheduleTestDB opens an isolated in-memory SQLite DB, migrates the tables
// the schedule/shift service touches (Business, Staff, Position for validation,
// plus Schedule + Shift), registers it as the package DB, and returns the
// wrapper. Mirrors newScheduleSettingsTestDB.
func newScheduleTestDB(t *testing.T) *DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Error),
	})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&Business{}, &Staff{}, &Position{}, &Schedule{}, &Shift{}))
	prev := db
	SetTestDB(gormDB)
	t.Cleanup(func() { SetTestDB(prev) })
	return GetDBWrapper()
}

// seedScheduleBusiness inserts a business, a position, and an active staff member
// so service-layer validation (position-in-business, active-staff-in-business)
// has rows to resolve. Returns the position and staff IDs.
func seedScheduleBusiness(t *testing.T, businessID uint) (positionID, staffID uint) {
	t.Helper()
	require.NoError(t, db.Create(&Business{ID: businessID, BusinessId: fmt.Sprintf("biz-%d", businessID)}).Error)
	pos := Position{BusinessID: businessID, Name: "Server", IsActive: true}
	require.NoError(t, db.Create(&pos).Error)
	st := Staff{BusinessID: businessID, Email: fmt.Sprintf("s%d@biz%d.test", businessID, businessID), Name: "Sam", Role: "server", IsActive: true}
	require.NoError(t, db.Create(&st).Error)
	return pos.ID, st.ID
}

// TestScheduleShiftGenesisShape is the genesis-safety guard: on a fresh DB the
// SQL migration is force-baselined WITHOUT running its DDL, so GORM autoMigrate
// must materialize the schema from the struct tags. This asserts the tables,
// the explicit TableName()s, and every column the 000104 DDL declares (notably
// the nullable reminded_at the shift-reminder scheduler claims on) all exist,
// and that a row round-trips with the DB-side status/published/break defaults.
func TestScheduleShiftGenesisShape(t *testing.T) {
	newScheduleTestDB(t)

	require.Equal(t, "schedules", Schedule{}.TableName())
	require.Equal(t, "shifts", Shift{}.TableName())

	m := db.Migrator()
	require.True(t, m.HasTable("schedules"))
	require.True(t, m.HasTable("shifts"))

	for _, col := range []string{"business_id", "week_start", "status", "published_at", "published_by_staff_id", "notes", "created_at", "updated_at"} {
		require.Truef(t, m.HasColumn(&Schedule{}, col), "schedules missing column %s", col)
	}
	for _, col := range []string{"business_id", "schedule_id", "staff_id", "position_id", "starts_at", "ends_at", "break_minutes", "status", "published", "notes", "created_by_staff_id", "reminded_at", "created_at", "updated_at"} {
		require.Truef(t, m.HasColumn(&Shift{}, col), "shifts missing column %s", col)
	}

	// Genesis index parity: the struct tags must materialize the SAME named
	// indexes/constraints the DDL creates, or a force-baselined fresh DB silently
	// loses the (business_id, week_start) uniqueness and the shift composites.
	require.True(t, m.HasIndex(&Schedule{}, "idx_schedules_biz_week"), "composite unique (business_id, week_start) must exist on genesis")
	for _, idx := range []string{"idx_shifts_biz_schedule", "idx_shifts_biz_staff_starts", "idx_shifts_biz_status"} {
		require.Truef(t, m.HasIndex(&Shift{}, idx), "shifts missing genesis index %s", idx)
	}

	posID, staffID := seedScheduleBusiness(t, 1)
	weekStart := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	sched := Schedule{BusinessID: 1, WeekStart: weekStart}
	require.NoError(t, db.Create(&sched).Error)

	// The unique index must actually reject a duplicate (business_id, week_start);
	// GetOrCreateDraftSchedule's create-race recovery depends on this failing.
	dup := Schedule{BusinessID: 1, WeekStart: weekStart}
	require.Error(t, db.Create(&dup).Error, "duplicate (business_id, week_start) must violate the unique index on genesis")

	var reloadedSched Schedule
	require.NoError(t, db.First(&reloadedSched, sched.ID).Error)
	require.Equal(t, ScheduleStatusDraft, reloadedSched.Status, "status must DB-default to draft")
	require.Nil(t, reloadedSched.PublishedAt)
	require.Nil(t, reloadedSched.PublishedByStaffID)

	sh := Shift{
		BusinessID: 1, ScheduleID: sched.ID, PositionID: posID,
		StartsAt:         weekStart.Add(9 * time.Hour),
		EndsAt:           weekStart.Add(17 * time.Hour),
		CreatedByStaffID: staffID,
	}
	require.NoError(t, db.Create(&sh).Error)

	var reloadedShift Shift
	require.NoError(t, db.First(&reloadedShift, sh.ID).Error)
	require.Equal(t, ShiftStatusPrivateDraft, reloadedShift.Status, "status must DB-default to private_draft")
	require.False(t, reloadedShift.Published)
	require.Equal(t, 0, reloadedShift.BreakMinutes)
	require.Nil(t, reloadedShift.StaffID, "unset staff_id is an open shift (NULL)")
	require.Nil(t, reloadedShift.RemindedAt, "reminded_at starts NULL")
}

func TestGetOrCreateDraftScheduleLazyAndIdempotent(t *testing.T) {
	d := newScheduleTestDB(t)
	_, staffID := seedScheduleBusiness(t, 1)
	weekStart := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)

	first, err := d.GetOrCreateDraftSchedule(1, weekStart, staffID)
	require.NoError(t, err)
	require.Equal(t, ScheduleStatusDraft, first.Status)
	require.Equal(t, uint(1), first.BusinessID)

	second, err := d.GetOrCreateDraftSchedule(1, weekStart, staffID)
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID, "second call reuses the row, no duplicate")

	var count int64
	require.NoError(t, db.Model(&Schedule{}).Where("business_id = ?", 1).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestCreateShiftValidation(t *testing.T) {
	d := newScheduleTestDB(t)
	posID, staffID := seedScheduleBusiness(t, 1)
	// A second tenant whose position must NOT be assignable from business 1.
	otherPosID, otherStaffID := seedScheduleBusiness(t, 2)
	weekStart := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	sched, err := d.GetOrCreateDraftSchedule(1, weekStart, staffID)
	require.NoError(t, err)

	// end <= start rejected.
	bad := &Shift{BusinessID: 1, ScheduleID: sched.ID, PositionID: posID, CreatedByStaffID: staffID,
		StartsAt: weekStart.Add(17 * time.Hour), EndsAt: weekStart.Add(9 * time.Hour)}
	require.ErrorIs(t, d.CreateShift(bad), ErrShiftInvalidRange)

	// cross-tenant position rejected.
	xpos := &Shift{BusinessID: 1, ScheduleID: sched.ID, PositionID: otherPosID, CreatedByStaffID: staffID,
		StartsAt: weekStart.Add(9 * time.Hour), EndsAt: weekStart.Add(17 * time.Hour)}
	require.ErrorIs(t, d.CreateShift(xpos), ErrPositionNotFound)

	// cross-tenant staff rejected.
	staffPtr := otherStaffID
	xstaff := &Shift{BusinessID: 1, ScheduleID: sched.ID, PositionID: posID, StaffID: &staffPtr, CreatedByStaffID: staffID,
		StartsAt: weekStart.Add(9 * time.Hour), EndsAt: weekStart.Add(17 * time.Hour)}
	require.ErrorIs(t, d.CreateShift(xstaff), ErrStaffNotFound)

	// happy path (assigned).
	ok := &Shift{BusinessID: 1, ScheduleID: sched.ID, PositionID: posID, StaffID: &staffID, CreatedByStaffID: staffID,
		StartsAt: weekStart.Add(9 * time.Hour), EndsAt: weekStart.Add(17 * time.Hour)}
	require.NoError(t, d.CreateShift(ok))
	require.NotZero(t, ok.ID)
	require.Equal(t, ShiftStatusPrivateDraft, ok.Status)
}

func TestUpdateShiftFieldsAllowListAndTenant(t *testing.T) {
	d := newScheduleTestDB(t)
	posID, staffID := seedScheduleBusiness(t, 1)
	weekStart := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	sched, err := d.GetOrCreateDraftSchedule(1, weekStart, staffID)
	require.NoError(t, err)
	sh := &Shift{BusinessID: 1, ScheduleID: sched.ID, PositionID: posID, CreatedByStaffID: staffID,
		StartsAt: weekStart.Add(9 * time.Hour), EndsAt: weekStart.Add(17 * time.Hour)}
	require.NoError(t, d.CreateShift(sh))

	// Whitelisted: assign staff + bump break. Non-whitelisted business_id/published ignored.
	require.NoError(t, d.UpdateShiftFields(1, sh.ID, map[string]interface{}{
		"staff_id":      staffID,
		"break_minutes": 30,
		"business_id":   999,  // must be ignored
		"published":     true, // must be ignored
	}))
	var reloaded Shift
	require.NoError(t, db.First(&reloaded, sh.ID).Error)
	require.NotNil(t, reloaded.StaffID)
	require.Equal(t, staffID, *reloaded.StaffID)
	require.Equal(t, 30, reloaded.BreakMinutes)
	require.Equal(t, uint(1), reloaded.BusinessID, "business_id must not be patchable")
	require.False(t, reloaded.Published, "published must not be patchable here")

	// Unassign via explicit nil staff_id (NULL).
	require.NoError(t, d.UpdateShiftFields(1, sh.ID, map[string]interface{}{"staff_id": nil}))
	require.NoError(t, db.First(&reloaded, sh.ID).Error)
	require.Nil(t, reloaded.StaffID)

	// Invalid range on partial update rejected (new end before existing start).
	require.ErrorIs(t, d.UpdateShiftFields(1, sh.ID, map[string]interface{}{
		"ends_at": weekStart.Add(1 * time.Hour),
	}), ErrShiftInvalidRange)

	// Cross-tenant update is a no-op miss.
	require.ErrorIs(t, d.UpdateShiftFields(2, sh.ID, map[string]interface{}{"break_minutes": 5}), ErrShiftNotFound)
}

func TestDeleteShiftTenantScoped(t *testing.T) {
	d := newScheduleTestDB(t)
	posID, staffID := seedScheduleBusiness(t, 1)
	weekStart := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	sched, err := d.GetOrCreateDraftSchedule(1, weekStart, staffID)
	require.NoError(t, err)
	sh := &Shift{BusinessID: 1, ScheduleID: sched.ID, PositionID: posID, CreatedByStaffID: staffID,
		StartsAt: weekStart.Add(9 * time.Hour), EndsAt: weekStart.Add(17 * time.Hour)}
	require.NoError(t, d.CreateShift(sh))

	require.ErrorIs(t, d.DeleteShift(2, sh.ID), ErrShiftNotFound)
	require.NoError(t, d.DeleteShift(1, sh.ID))
	var count int64
	require.NoError(t, db.Model(&Shift{}).Where("id = ?", sh.ID).Count(&count).Error)
	require.Equal(t, int64(0), count)
}

func TestPublishScheduleStatusSplitAndIdempotent(t *testing.T) {
	d := newScheduleTestDB(t)
	posID, staffID := seedScheduleBusiness(t, 1)
	weekStart := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	sched, err := d.GetOrCreateDraftSchedule(1, weekStart, staffID)
	require.NoError(t, err)

	assigned := &Shift{BusinessID: 1, ScheduleID: sched.ID, PositionID: posID, StaffID: &staffID, CreatedByStaffID: staffID,
		StartsAt: weekStart.Add(9 * time.Hour), EndsAt: weekStart.Add(17 * time.Hour)}
	require.NoError(t, d.CreateShift(assigned))
	open := &Shift{BusinessID: 1, ScheduleID: sched.ID, PositionID: posID, CreatedByStaffID: staffID,
		StartsAt: weekStart.Add(18 * time.Hour), EndsAt: weekStart.Add(22 * time.Hour)}
	require.NoError(t, d.CreateShift(open))

	published, shifts, err := d.PublishSchedule(1, sched.ID, staffID)
	require.NoError(t, err)
	require.Equal(t, ScheduleStatusPublished, published.Status)
	require.NotNil(t, published.PublishedAt)
	require.NotNil(t, published.PublishedByStaffID)
	require.Equal(t, staffID, *published.PublishedByStaffID)
	require.Len(t, shifts, 2)

	byID := map[uint]Shift{}
	for _, s := range shifts {
		byID[s.ID] = s
		require.True(t, s.Published)
	}
	require.Equal(t, ShiftStatusFilled, byID[assigned.ID].Status, "assigned -> filled")
	require.Equal(t, ShiftStatusOpen, byID[open.ID].Status, "unassigned -> open")

	// Idempotent: a second publish affects zero draft rows.
	_, _, err = d.PublishSchedule(1, sched.ID, staffID)
	require.ErrorIs(t, err, ErrScheduleNotDraft)

	// Cross-tenant publish also yields ErrScheduleNotDraft (zero rows).
	_, _, err = d.PublishSchedule(2, sched.ID, staffID)
	require.ErrorIs(t, err, ErrScheduleNotDraft)
}

func TestGetScheduleForWeek(t *testing.T) {
	d := newScheduleTestDB(t)
	posID, staffID := seedScheduleBusiness(t, 1)
	weekStart := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)

	// No schedule yet -> nil, nil, nil (not an error).
	sched, shifts, err := d.GetScheduleForWeek(1, weekStart)
	require.NoError(t, err)
	require.Nil(t, sched)
	require.Empty(t, shifts)

	created, err := d.GetOrCreateDraftSchedule(1, weekStart, staffID)
	require.NoError(t, err)
	sh := &Shift{BusinessID: 1, ScheduleID: created.ID, PositionID: posID, CreatedByStaffID: staffID,
		StartsAt: weekStart.Add(9 * time.Hour), EndsAt: weekStart.Add(17 * time.Hour)}
	require.NoError(t, d.CreateShift(sh))

	got, gotShifts, err := d.GetScheduleForWeek(1, weekStart)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, created.ID, got.ID)
	require.Len(t, gotShifts, 1)
}

// TestUpdateShiftFieldsResetsReminderClaimOnReassignOrMove is the stale-reminder
// regression: a shift that has ALREADY been reminded (reminded_at stamped) and is
// then reassigned to another staffer or moved to a different start time must have
// its reminder claim cleared in the same update so the scheduler re-reminds the
// right person at the right time. A no-op or unrelated edit (notes, same staff)
// must NOT re-arm the claim (no duplicate reminder).
func TestUpdateShiftFieldsResetsReminderClaimOnReassignOrMove(t *testing.T) {
	d := newScheduleTestDB(t)
	posID, staffID := seedScheduleBusiness(t, 1)
	// Second active staffer to reassign to.
	st2 := Staff{BusinessID: 1, Email: "s2@biz1.test", Name: "Sky", Role: "server", IsActive: true}
	require.NoError(t, db.Create(&st2).Error)

	weekStart := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	sched, err := d.GetOrCreateDraftSchedule(1, weekStart, staffID)
	require.NoError(t, err)

	mkRemindedShift := func() *Shift {
		sh := &Shift{BusinessID: 1, ScheduleID: sched.ID, PositionID: posID, StaffID: &staffID,
			CreatedByStaffID: staffID,
			StartsAt:         weekStart.Add(9 * time.Hour), EndsAt: weekStart.Add(17 * time.Hour)}
		require.NoError(t, d.CreateShift(sh))
		remindedAt := weekStart.Add(6 * time.Hour)
		require.NoError(t, db.Model(&Shift{}).Where("id = ?", sh.ID).
			Update("reminded_at", remindedAt).Error)
		return sh
	}

	// Reassign to another staffer → claim cleared.
	sh := mkRemindedShift()
	require.NoError(t, d.UpdateShiftFields(1, sh.ID, map[string]interface{}{"staff_id": st2.ID}))
	load := func(id uint) Shift {
		var s Shift
		require.NoError(t, db.First(&s, id).Error)
		return s
	}
	require.Nil(t, load(sh.ID).RemindedAt, "reassigning a reminded shift must clear reminded_at")

	// Move start time → claim cleared.
	sh = mkRemindedShift()
	require.NoError(t, d.UpdateShiftFields(1, sh.ID, map[string]interface{}{
		"starts_at": weekStart.Add(13 * time.Hour),
	}))
	require.Nil(t, load(sh.ID).RemindedAt, "moving a reminded shift must clear reminded_at")

	// Notes-only edit → claim untouched (no duplicate reminder).
	sh = mkRemindedShift()
	require.NoError(t, d.UpdateShiftFields(1, sh.ID, map[string]interface{}{"notes": "bring keys"}))
	require.NotNil(t, load(sh.ID).RemindedAt, "a notes-only edit must not re-arm the reminder")

	// Same staff_id + same starts_at (no-op values) → claim untouched.
	sh = mkRemindedShift()
	require.NoError(t, d.UpdateShiftFields(1, sh.ID, map[string]interface{}{
		"staff_id":  staffID,
		"starts_at": sh.StartsAt,
	}))
	require.NotNil(t, load(sh.ID).RemindedAt, "a no-op staff/start edit must not re-arm the reminder")
}

// TestCopyScheduleWeek proves the transactional copy: every source shift is
// re-created in the destination week with its start/end shifted by the exact week
// delta, staff/position/break/notes preserved, into a get-or-created destination
// draft. The count returned matches the rows inserted.
func TestCopyScheduleWeek(t *testing.T) {
	d := newScheduleTestDB(t)
	posID, staffID := seedScheduleBusiness(t, 1)

	fromWeek := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC) // Mon
	toWeek := fromWeek.AddDate(0, 0, 7)

	src, err := d.GetOrCreateDraftSchedule(1, fromWeek, staffID)
	require.NoError(t, err)
	// Two source shifts: one assigned, one open.
	require.NoError(t, d.CreateShift(&Shift{
		BusinessID: 1, ScheduleID: src.ID, PositionID: posID, StaffID: &staffID,
		StartsAt: fromWeek.Add(9 * time.Hour), EndsAt: fromWeek.Add(17 * time.Hour),
		BreakMinutes: 30, Notes: "AM", CreatedByStaffID: staffID,
	}))
	require.NoError(t, d.CreateShift(&Shift{
		BusinessID: 1, ScheduleID: src.ID, PositionID: posID, StaffID: nil,
		StartsAt: fromWeek.AddDate(0, 0, 1).Add(17 * time.Hour), EndsAt: fromWeek.AddDate(0, 0, 1).Add(23 * time.Hour),
		BreakMinutes: 0, Notes: "PM open", CreatedByStaffID: staffID,
	}))

	res, err := d.CopyScheduleWeek(1, fromWeek, toWeek, staffID, false)
	require.NoError(t, err)
	require.Equal(t, 2, res.Created)
	require.Equal(t, 0, res.Skipped)

	_, destShifts, err := d.GetScheduleForWeek(1, toWeek)
	require.NoError(t, err)
	require.Len(t, destShifts, 2, "both source shifts copied into the destination week")
	// The AM shift landed exactly one week later, same time-of-day and metadata.
	var am *Shift
	for i := range destShifts {
		if destShifts[i].Notes == "AM" {
			am = &destShifts[i]
		}
	}
	require.NotNil(t, am)
	require.Equal(t, toWeek.Add(9*time.Hour), am.StartsAt.UTC())
	require.Equal(t, toWeek.Add(17*time.Hour), am.EndsAt.UTC())
	require.Equal(t, 30, am.BreakMinutes)
	require.NotNil(t, am.StaffID)
	require.Equal(t, staffID, *am.StaffID)
}

// TestCopyScheduleWeekOnlyEmptyDays proves idempotent "copy into empty days":
// when the destination already has a shift on a weekday, that weekday's source
// shifts are skipped, and only source shifts landing on still-empty destination
// days are copied. Re-running is safe (no duplication).
func TestCopyScheduleWeekOnlyEmptyDays(t *testing.T) {
	d := newScheduleTestDB(t)
	posID, staffID := seedScheduleBusiness(t, 1)

	fromWeek := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	toWeek := fromWeek.AddDate(0, 0, 7)

	src, err := d.GetOrCreateDraftSchedule(1, fromWeek, staffID)
	require.NoError(t, err)
	// Source: Mon + Tue shifts.
	require.NoError(t, d.CreateShift(&Shift{BusinessID: 1, ScheduleID: src.ID, PositionID: posID,
		StartsAt: fromWeek.Add(9 * time.Hour), EndsAt: fromWeek.Add(17 * time.Hour), CreatedByStaffID: staffID}))
	require.NoError(t, d.CreateShift(&Shift{BusinessID: 1, ScheduleID: src.ID, PositionID: posID,
		StartsAt: fromWeek.AddDate(0, 0, 1).Add(9 * time.Hour), EndsAt: fromWeek.AddDate(0, 0, 1).Add(17 * time.Hour), CreatedByStaffID: staffID}))

	// Destination already has a Monday shift → Monday is a filled day.
	dest, err := d.GetOrCreateDraftSchedule(1, toWeek, staffID)
	require.NoError(t, err)
	require.NoError(t, d.CreateShift(&Shift{BusinessID: 1, ScheduleID: dest.ID, PositionID: posID,
		StartsAt: toWeek.Add(10 * time.Hour), EndsAt: toWeek.Add(18 * time.Hour), CreatedByStaffID: staffID}))

	res, err := d.CopyScheduleWeek(1, fromWeek, toWeek, staffID, true)
	require.NoError(t, err)
	require.Equal(t, 1, res.Created, "only the Tuesday shift copies (Monday already has one)")
	require.Equal(t, 1, res.Skipped, "the Monday source shift is skipped")

	_, destShifts, err := d.GetScheduleForWeek(1, toWeek)
	require.NoError(t, err)
	require.Len(t, destShifts, 2, "the pre-existing Monday shift + the copied Tuesday shift")

	// Re-running copy-into-empty-days is idempotent: now both dest days are filled.
	res2, err := d.CopyScheduleWeek(1, fromWeek, toWeek, staffID, true)
	require.NoError(t, err)
	require.Equal(t, 0, res2.Created, "re-run copies nothing new")
	require.Equal(t, 2, res2.Skipped)
}

// TestCopyScheduleWeekEmptySource proves copying an empty source week is a
// no-op (0 created) rather than an error.
func TestCopyScheduleWeekEmptySource(t *testing.T) {
	d := newScheduleTestDB(t)
	seedScheduleBusiness(t, 1)
	fromWeek := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	toWeek := fromWeek.AddDate(0, 0, 7)
	res, err := d.CopyScheduleWeek(1, fromWeek, toWeek, 0, false)
	require.NoError(t, err)
	require.Equal(t, 0, res.Created)
	require.Equal(t, 0, res.Skipped)
}
