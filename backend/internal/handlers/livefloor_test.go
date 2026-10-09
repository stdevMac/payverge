package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// grace is the shared late grace used across these tests.
const grace = 5 * time.Minute

func mkShift(id, staffID uint, start, end time.Time) database.Shift {
	sp := staffID
	return database.Shift{ID: id, BusinessID: 1, StaffID: &sp, StartsAt: start, EndsAt: end, Status: database.ShiftStatusFilled, Published: true}
}

func openEntry(staffID uint, in time.Time) database.TimeEntry {
	return database.TimeEntry{BusinessID: 1, StaffID: staffID, ClockInAt: in, Status: database.TimeEntryStatusOpen}
}

func closedEntry(staffID uint, in, out time.Time) database.TimeEntry {
	o := out
	return database.TimeEntry{BusinessID: 1, StaffID: staffID, ClockInAt: in, ClockOutAt: &o, Status: database.TimeEntryStatusPendingReview}
}

// rowFor finds the row for a staff id (0 shift rows are unique per staff here).
func rowFor(rows []liveFloorRow, staffID uint) *liveFloorRow {
	for i := range rows {
		if rows[i].StaffID == staffID {
			return &rows[i]
		}
	}
	return nil
}

func TestComputeLiveFloor_OnClock(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	sh := mkShift(1, 10, now.Add(-2*time.Hour), now.Add(6*time.Hour))
	in := now.Add(-105 * time.Minute)
	rows := computeLiveFloor([]database.Shift{sh}, []database.TimeEntry{openEntry(10, in)}, now, grace)
	require.Len(t, rows, 1)
	r := rows[0]
	require.Equal(t, liveFloorOnClock, r.Status)
	require.NotNil(t, r.ClockInAt)
	require.Equal(t, in, *r.ClockInAt)
	require.NotNil(t, r.ShiftID)
}

func TestComputeLiveFloor_Done(t *testing.T) {
	now := time.Date(2026, 6, 30, 18, 0, 0, 0, time.UTC)
	sh := mkShift(1, 10, now.Add(-9*time.Hour), now.Add(-1*time.Hour))
	entry := closedEntry(10, now.Add(-9*time.Hour), now.Add(-1*time.Hour))
	rows := computeLiveFloor([]database.Shift{sh}, []database.TimeEntry{entry}, now, grace)
	require.Len(t, rows, 1)
	require.Equal(t, liveFloorDone, rows[0].Status)
}

func TestComputeLiveFloor_Scheduled_BeforeStart(t *testing.T) {
	now := time.Date(2026, 6, 30, 8, 0, 0, 0, time.UTC)
	sh := mkShift(1, 10, now.Add(1*time.Hour), now.Add(9*time.Hour))
	rows := computeLiveFloor([]database.Shift{sh}, nil, now, grace)
	require.Len(t, rows, 1)
	require.Equal(t, liveFloorScheduled, rows[0].Status)
}

func TestComputeLiveFloor_ScheduledWithinGrace(t *testing.T) {
	now := time.Date(2026, 6, 30, 9, 3, 0, 0, time.UTC) // 3 min after a 09:00 start, grace is 5
	sh := mkShift(1, 10, time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC), time.Date(2026, 6, 30, 17, 0, 0, 0, time.UTC))
	rows := computeLiveFloor([]database.Shift{sh}, nil, now, grace)
	require.Len(t, rows, 1)
	require.Equal(t, liveFloorScheduled, rows[0].Status, "within grace is still scheduled, not late")
}

func TestComputeLiveFloor_Late(t *testing.T) {
	start := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	now := start.Add(20 * time.Minute) // past the 5-min grace, before end
	sh := mkShift(1, 10, start, start.Add(8*time.Hour))
	rows := computeLiveFloor([]database.Shift{sh}, nil, now, grace)
	require.Len(t, rows, 1)
	require.Equal(t, liveFloorLate, rows[0].Status)
	require.Equal(t, 20, rows[0].LateMinutes)
}

func TestComputeLiveFloor_NoShow(t *testing.T) {
	start := time.Date(2026, 6, 30, 9, 0, 0, 0, time.UTC)
	end := start.Add(8 * time.Hour)
	now := end.Add(30 * time.Minute) // shift over, never punched
	sh := mkShift(1, 10, start, end)
	rows := computeLiveFloor([]database.Shift{sh}, nil, now, grace)
	require.Len(t, rows, 1)
	require.Equal(t, liveFloorNoShow, rows[0].Status)
}

func TestComputeLiveFloor_OnClockWithoutScheduledShift(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	// Staff 10 is scheduled; staff 99 clocked in with no shift on the board.
	sh := mkShift(1, 10, now.Add(-2*time.Hour), now.Add(6*time.Hour))
	entries := []database.TimeEntry{openEntry(10, now.Add(-2*time.Hour)), openEntry(99, now.Add(-30*time.Minute))}
	rows := computeLiveFloor([]database.Shift{sh}, entries, now, grace)
	require.Len(t, rows, 2)
	extra := rowFor(rows, 99)
	require.NotNil(t, extra, "an unscheduled clocked-in staffer must still appear on the floor")
	require.Equal(t, liveFloorOnClock, extra.Status)
	require.Nil(t, extra.ShiftID, "the unscheduled row has no shift")
}

func TestComputeLiveFloor_OpenShiftIgnored(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	open := database.Shift{ID: 5, BusinessID: 1, StaffID: nil, StartsAt: now.Add(1 * time.Hour), EndsAt: now.Add(5 * time.Hour), Status: database.ShiftStatusOpen, Published: true}
	rows := computeLiveFloor([]database.Shift{open}, nil, now, grace)
	require.Empty(t, rows, "an unassigned (open) shift puts nobody on the floor")
}

func TestComputeLiveFloor_OpenPunchBeatsClosed(t *testing.T) {
	now := time.Date(2026, 6, 30, 15, 0, 0, 0, time.UTC)
	sh := mkShift(1, 10, now.Add(-6*time.Hour), now.Add(2*time.Hour))
	// A morning punch closed, then a fresh open punch — they're back on the clock.
	entries := []database.TimeEntry{
		closedEntry(10, now.Add(-6*time.Hour), now.Add(-3*time.Hour)),
		openEntry(10, now.Add(-1*time.Hour)),
	}
	rows := computeLiveFloor([]database.Shift{sh}, entries, now, grace)
	require.Len(t, rows, 1)
	require.Equal(t, liveFloorOnClock, rows[0].Status, "an open punch beats an earlier closed one")
}

func TestComputeLiveFloor_DeterministicOrder(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	// Two scheduled shifts (input order preserved from the query's start-asc) plus
	// two unscheduled on-clock extras, which must come after and be staff-id sorted.
	shifts := []database.Shift{
		mkShift(1, 10, now.Add(-3*time.Hour), now.Add(5*time.Hour)),
		mkShift(2, 20, now.Add(-1*time.Hour), now.Add(7*time.Hour)),
	}
	entries := []database.TimeEntry{
		openEntry(10, now.Add(-3*time.Hour)),
		openEntry(88, now.Add(-20*time.Minute)),
		openEntry(55, now.Add(-10*time.Minute)),
	}
	rows := computeLiveFloor(shifts, entries, now, grace)
	require.Len(t, rows, 4)
	require.Equal(t, uint(10), rows[0].StaffID)
	require.Equal(t, uint(20), rows[1].StaffID)
	require.Equal(t, uint(55), rows[2].StaffID, "extras sorted by staff id")
	require.Equal(t, uint(88), rows[3].StaffID)
}

// TestLiveFloorEndpoint exercises the full handler path: it seeds a business
// (UTC), a named staffer with a published shift for today plus an open punch,
// and asserts the endpoint resolves the staff name, returns the {date,rows,
// summary} envelope, marks the punched-in staffer on_clock, and never leaks a
// dollar sign (money-free staff surface).
func TestLiveFloorEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, gdb.AutoMigrate(&database.Business{}, &database.Staff{}, &database.Schedule{}, &database.Shift{}, &database.TimeEntry{}, &database.BusinessScheduleSettings{}))
	prev := database.GetDB()
	database.SetTestDB(gdb)
	t.Cleanup(func() { database.SetTestDB(prev) })

	require.NoError(t, gdb.Create(&database.Business{ID: 1, BusinessId: "biz-1", Timezone: "UTC"}).Error)
	require.NoError(t, gdb.Create(&database.Staff{ID: 10, BusinessID: 1, Email: "dana@biz.test", Name: "Dana", Role: "server", IsActive: true}).Error)

	// A shift anchored at today-midday (UTC) is always inside today's window.
	midday := time.Now().UTC().Truncate(24 * time.Hour).Add(12 * time.Hour)
	weekStart := weekStartUTCMidnight(midday, 1)
	require.NoError(t, gdb.Create(&database.Schedule{
		ID: 1, BusinessID: 1, WeekStart: weekStart, Status: database.ScheduleStatusPublished,
	}).Error)
	sp := uint(10)
	require.NoError(t, gdb.Create(&database.Shift{
		BusinessID: 1, ScheduleID: 1, PositionID: 1, StaffID: &sp,
		StartsAt: midday, EndsAt: midday.Add(8 * time.Hour),
		Status: database.ShiftStatusFilled, Published: true, CreatedByStaffID: 1,
	}).Error)
	require.NoError(t, gdb.Create(&database.TimeEntry{
		BusinessID: 1, StaffID: 10, ClockInAt: midday, Status: database.TimeEntryStatusOpen,
	}).Error)

	h := NewTimeclockHandler(database.GetDBWrapper())
	r := gin.New()
	r.GET("/b/:id/live-floor", h.LiveFloor)
	req, _ := http.NewRequest(http.MethodGet, "/b/1/live-floor", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NotContains(t, w.Body.String(), "$", "live-floor board must be money-free")

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Date    string           `json:"date"`
			Rows    []liveFloorRow   `json:"rows"`
			Summary liveFloorSummary `json:"summary"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	require.NotEmpty(t, resp.Data.Date)
	require.Len(t, resp.Data.Rows, 1)
	require.Equal(t, "Dana", resp.Data.Rows[0].StaffName, "the handler must resolve the staff name")
	require.Equal(t, liveFloorOnClock, resp.Data.Rows[0].Status)
	require.Equal(t, 1, resp.Data.Summary.OnClock)
	require.False(t, strings.Contains(strings.ToLower(w.Body.String()), "rate"), "no pay-rate field on the board")
}

func TestWeekStartUTCMidnight(t *testing.T) {
	// Wednesday 19 Aug 2026 in New York.
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	wed := time.Date(2026, 8, 19, 15, 0, 0, 0, loc)
	sun := weekStartUTCMidnight(wed, 0)
	require.Equal(t, time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC), sun)
	mon := weekStartUTCMidnight(wed, 1)
	require.Equal(t, time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC), mon)
}

func TestPublishedShiftsForLiveFloor_IgnoresOtherWeekLeftovers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, err := gdb.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, gdb.AutoMigrate(&database.Business{}, &database.Staff{}, &database.Schedule{}, &database.Shift{}, &database.TimeEntry{}, &database.BusinessScheduleSettings{}))
	prev := database.GetDB()
	database.SetTestDB(gdb)
	t.Cleanup(func() { database.SetTestDB(prev) })

	require.NoError(t, gdb.Create(&database.Business{ID: 1, BusinessId: "biz-1", Timezone: "UTC"}).Error)
	require.NoError(t, gdb.Create(&database.Staff{ID: 10, BusinessID: 1, Email: "hana@biz.test", Name: "Hana Host", Role: "host", IsActive: true}).Error)

	now := time.Date(2026, 8, 19, 15, 0, 0, 0, time.UTC)
	thisWeek := weekStartUTCMidnight(now, 1) // Mon 17
	lastWeek := thisWeek.AddDate(0, 0, -7)
	require.NoError(t, gdb.Create(&database.Schedule{
		ID: 1, BusinessID: 1, WeekStart: lastWeek, Status: database.ScheduleStatusPublished,
	}).Error)
	// Current week exists but is an empty draft — Horario shows 0 turnos.
	require.NoError(t, gdb.Create(&database.Schedule{
		ID: 2, BusinessID: 1, WeekStart: thisWeek, Status: database.ScheduleStatusDraft,
	}).Error)
	sp := uint(10)
	require.NoError(t, gdb.Create(&database.Shift{
		BusinessID: 1, ScheduleID: 1, PositionID: 1, StaffID: &sp,
		StartsAt: now.Add(-2 * time.Hour), EndsAt: now.Add(6 * time.Hour),
		Status: database.ShiftStatusFilled, Published: true, CreatedByStaffID: 1,
	}).Error)

	h := NewTimeclockHandler(database.GetDBWrapper())
	dayStart := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	dayEnd := dayStart.Add(24 * time.Hour)
	shifts, err := h.publishedShiftsForLiveFloor(1, dayStart, dayEnd)
	require.NoError(t, err)
	require.Empty(t, shifts, "leftover published shifts from another week must not invent late rows")

	rows := computeLiveFloor(shifts, nil, now, grace)
	require.Empty(t, rows)
}

func TestSummarizeLiveFloor(t *testing.T) {
	rows := []liveFloorRow{
		{StaffID: 1, Status: liveFloorOnClock},
		{StaffID: 2, Status: liveFloorOnClock},
		{StaffID: 3, Status: liveFloorScheduled},
		{StaffID: 4, Status: liveFloorLate},
		{StaffID: 5, Status: liveFloorNoShow},
		{StaffID: 6, Status: liveFloorDone},
	}
	s := summarizeLiveFloor(rows)
	require.Equal(t, 2, s.OnClock)
	require.Equal(t, 1, s.Scheduled)
	require.Equal(t, 1, s.Late)
	require.Equal(t, 1, s.NoShow)
	require.Equal(t, 1, s.Done)
}
