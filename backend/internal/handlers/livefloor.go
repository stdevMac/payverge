package handlers

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
)

// Live-floor row statuses. The board answers "who is actually on the floor right
// now, and who should be?" for a business's day. All money-free.
const (
	liveFloorOnClock   = "on_clock"  // has an open punch (working)
	liveFloorDone      = "done"      // punched in and out already today (came and went)
	liveFloorScheduled = "scheduled" // shift hasn't started (or within grace), no punch yet
	liveFloorLate      = "late"      // shift started (past grace), still no punch
	liveFloorNoShow    = "no_show"   // shift ended, never punched in
)

// liveFloorLateGraceMinutes is how long after a shift's start a staffer who
// hasn't clocked in is still "scheduled" before flipping to "late". A small
// grace avoids flagging someone the instant their shift ticks over.
const liveFloorLateGraceMinutes = 5

// liveFloorRow is one person on (or expected on) the floor. StaffName is filled
// by the handler from a batched name map. ShiftID/ShiftStart/ShiftEnd are nil
// for a staffer clocked in without a scheduled shift. LateMinutes is set only
// for the "late" status. No money fields (staff surface).
type liveFloorRow struct {
	StaffID     uint       `json:"staff_id"`
	StaffName   string     `json:"staff_name"`
	Status      string     `json:"status"`
	ShiftID     *uint      `json:"shift_id,omitempty"`
	ShiftStart  *time.Time `json:"shift_start,omitempty"`
	ShiftEnd    *time.Time `json:"shift_end,omitempty"`
	ClockInAt   *time.Time `json:"clock_in_at,omitempty"`
	LateMinutes int        `json:"late_minutes,omitempty"`
}

// liveFloorSummary is the at-a-glance count the board badges from. Sums to
// len(rows).
type liveFloorSummary struct {
	OnClock   int `json:"on_clock"`
	Scheduled int `json:"scheduled"`
	Late      int `json:"late"`
	NoShow    int `json:"no_show"`
	Done      int `json:"done"`
}

// computeLiveFloor derives the live-floor rows from a day's assigned shifts and
// time entries. Pure (no DB, no wall clock) and driven by an explicit now so it
// is deterministic under test. lateGrace is how long past a shift's start a
// no-punch staffer stays "scheduled" before "late". One row per assigned shift
// plus one row per staffer clocked in WITHOUT a scheduled shift today. Rows are
// ordered deterministically: scheduled shifts by (start, staff), then the
// unscheduled on-clock extras by staff. Money-free.
func computeLiveFloor(shifts []database.Shift, entries []database.TimeEntry, now time.Time, lateGrace time.Duration) []liveFloorRow {
	// Per-staff punch aggregation: the current open punch (if any) and whether
	// any punch was closed today. Open wins for "are they here now".
	type agg struct {
		open   *database.TimeEntry
		closed bool
	}
	byStaff := map[uint]*agg{}
	for i := range entries {
		e := entries[i]
		a := byStaff[e.StaffID]
		if a == nil {
			a = &agg{}
			byStaff[e.StaffID] = a
		}
		if e.ClockOutAt == nil || e.Status == database.TimeEntryStatusOpen {
			ec := e
			a.open = &ec
		} else {
			a.closed = true
		}
	}

	rows := make([]liveFloorRow, 0, len(shifts))
	scheduled := map[uint]bool{}
	for i := range shifts {
		sh := shifts[i]
		if sh.StaffID == nil {
			continue // open (unassigned) shift — nobody on the floor
		}
		staffID := *sh.StaffID
		scheduled[staffID] = true
		shiftID := sh.ID
		row := liveFloorRow{StaffID: staffID, ShiftID: &shiftID, ShiftStart: &sh.StartsAt, ShiftEnd: &sh.EndsAt}
		a := byStaff[staffID]
		switch {
		case a != nil && a.open != nil:
			row.Status = liveFloorOnClock
			row.ClockInAt = &a.open.ClockInAt
		case a != nil && a.closed:
			row.Status = liveFloorDone
		case now.Before(sh.StartsAt):
			row.Status = liveFloorScheduled
		case !now.Before(sh.EndsAt): // now >= end
			row.Status = liveFloorNoShow
		case now.Before(sh.StartsAt.Add(lateGrace)):
			row.Status = liveFloorScheduled // still within grace
		default:
			row.Status = liveFloorLate
			row.LateMinutes = int(now.Sub(sh.StartsAt).Minutes())
		}
		rows = append(rows, row)
	}

	// Anyone clocked in without a scheduled shift today is still on the floor —
	// surface them so the manager's headcount is complete. Collect + sort by
	// staff id for deterministic ordering (map iteration is unordered).
	var extras []uint
	for staffID, a := range byStaff {
		if scheduled[staffID] || a.open == nil {
			continue
		}
		extras = append(extras, staffID)
	}
	sort.Slice(extras, func(i, j int) bool { return extras[i] < extras[j] })
	for _, staffID := range extras {
		a := byStaff[staffID]
		rows = append(rows, liveFloorRow{StaffID: staffID, Status: liveFloorOnClock, ClockInAt: &a.open.ClockInAt})
	}
	return rows
}

// summarizeLiveFloor tallies the rows by status for the board badge.
func summarizeLiveFloor(rows []liveFloorRow) liveFloorSummary {
	var s liveFloorSummary
	for _, r := range rows {
		switch r.Status {
		case liveFloorOnClock:
			s.OnClock++
		case liveFloorScheduled:
			s.Scheduled++
		case liveFloorLate:
			s.Late++
		case liveFloorNoShow:
			s.NoShow++
		case liveFloorDone:
			s.Done++
		}
	}
	return s
}

// LiveFloor returns the manager live-floor board for a business's day (the
// business-local calendar day by default; ?date=YYYY-MM-DD overrides it, parsed
// in the business timezone). It reports who is on the clock, scheduled, running
// late, a no-show, or done — computed from that day's published assigned shifts
// and time entries, relative to the real current time. timeclock:manage gated.
// Money-free. Envelope: {"success":true,"data":{"date","rows","summary"}}.
func (h *TimeclockHandler) LiveFloor(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	biz, err := h.db.GetBusinessByID(businessID)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load business")
		return
	}
	loc := database.ResolveBusinessLocation(biz)
	now := time.Now().In(loc)

	// Day window in the business calendar. ?date overrides the day being viewed;
	// status is still computed relative to the real now.
	dayRef := now
	if q := strings.TrimSpace(c.Query("date")); q != "" {
		if parsed, perr := time.ParseInLocation("2006-01-02", q, loc); perr == nil {
			dayRef = parsed
		}
	}
	dayStart := time.Date(dayRef.Year(), dayRef.Month(), dayRef.Day(), 0, 0, 0, 0, loc)
	dayEnd := dayStart.Add(24 * time.Hour)

	// Only this venue-week's published roster can create late/no-show rows.
	// Leftover published=true shifts from another week's schedule were
	// inventing tardies when Horario showed 0 turnos and nobody was punched in.
	shifts, err := h.publishedShiftsForLiveFloor(businessID, dayStart, dayEnd)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load shifts")
		return
	}
	entries, err := h.db.ListEntriesForDay(businessID, dayStart, dayEnd)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load time entries")
		return
	}

	rows := computeLiveFloor(shifts, entries, now, time.Duration(liveFloorLateGraceMinutes)*time.Minute)

	// Batch-resolve names for every staffer on the board (single query, no N+1).
	ids := make([]uint, 0, len(rows))
	seen := map[uint]bool{}
	for _, r := range rows {
		if !seen[r.StaffID] {
			seen[r.StaffID] = true
			ids = append(ids, r.StaffID)
		}
	}
	names, err := h.db.StaffNamesByIDs(businessID, ids)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load staff")
		return
	}
	for i := range rows {
		rows[i].StaffName = names[rows[i].StaffID]
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"date":    dayStart.Format("2006-01-02"),
		"rows":    rows,
		"summary": summarizeLiveFloor(rows),
	}})
}

// weekStartUTCMidnight is the schedules.week_start value (UTC date) for the
// venue-local calendar day, aligned to weekStartDay (0=Sun..6=Sat).
func weekStartUTCMidnight(localDay time.Time, weekStartDay int) time.Time {
	if weekStartDay < 0 || weekStartDay > 6 {
		weekStartDay = 1
	}
	dow := int(localDay.Weekday())
	back := (dow - weekStartDay + 7) % 7
	start := localDay.AddDate(0, 0, -back)
	return time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
}

func shiftOverlapsDay(sh database.Shift, dayStart, dayEnd time.Time) bool {
	return sh.StartsAt.Before(dayEnd) && sh.EndsAt.After(dayStart)
}

// publishedShiftsForLiveFloor returns assigned, published, filled shifts from
// THIS week's published schedule that overlap [dayStart, dayEnd). An empty or
// draft week yields no scheduled rows (open punches still surface as extras).
func (h *TimeclockHandler) publishedShiftsForLiveFloor(businessID uint, dayStart, dayEnd time.Time) ([]database.Shift, error) {
	weekStartDay := 1
	if settings, err := h.db.GetOrCreateBusinessScheduleSettings(businessID); err == nil && settings != nil {
		weekStartDay = settings.WeekStartDay
	}
	weekStart := weekStartUTCMidnight(dayStart, weekStartDay)
	sched, weekShifts, err := h.db.GetScheduleForWeek(businessID, weekStart)
	if err != nil {
		return nil, err
	}
	if sched == nil || sched.Status != database.ScheduleStatusPublished {
		return nil, nil
	}
	out := make([]database.Shift, 0, len(weekShifts))
	for _, sh := range weekShifts {
		if !sh.Published || sh.StaffID == nil || sh.Status != database.ShiftStatusFilled {
			continue
		}
		if !shiftOverlapsDay(sh, dayStart, dayEnd) {
			continue
		}
		out = append(out, sh)
	}
	return out, nil
}
