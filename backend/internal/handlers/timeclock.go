package handlers

import (
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/server"
)

// publishTimeclockEntry emits the content-free timeclock.entry SSE nudge (ids +
// status only) so the manager review queue refetches live instead of going stale.
// Gated on timeclock:manage in sse_permissions.go — line staff never receive it.
func publishTimeclockEntry(businessID, entryID uint, status string) {
	events.GetHub().PublishJSON(businessID, "timeclock.entry", gin.H{
		"entry_id": entryID,
		"status":   status,
	})
}

// TimeclockHandler serves the time-clock endpoints (Slice 4). Punch endpoints
// (clock-in/out/break + my timesheet) are always scoped to the caller's OWN
// staff_id (from the gin context, never a body value). The review endpoints
// (list/approve/manual-entry) are manager-gated (timeclock:manage). Staff-facing
// responses expose worked MINUTES/HOURS only — never a dollar field or a pay
// rate. Any labor-$ derived from an approved entry lives only in the
// owner/financial:read-gated accounting labor endpoint.
type TimeclockHandler struct{ db *database.DB }

func NewTimeclockHandler(db *database.DB) *TimeclockHandler { return &TimeclockHandler{db: db} }

// parseBusinessDayRange parses YYYY-MM-DD from/to in the business timezone
// (mirrors LiveFloor). End is exclusive next local midnight so the 'to' calendar
// day is inclusive. Empty from defaults to 8 weeks before now; empty to defaults
// to tomorrow local midnight.
func (h *TimeclockHandler) parseBusinessDayRange(businessID uint, fromStr, toStr string) (from, to time.Time, err error) {
	biz, err := h.db.GetBusinessByID(businessID)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	loc := database.ResolveBusinessLocation(biz)
	now := time.Now().In(loc)
	// Default window: last 8 local weeks through end of today (exclusive end = tomorrow).
	to = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	from = to.AddDate(0, 0, -56)
	if fromStr != "" {
		if t, perr := time.ParseInLocation("2006-01-02", fromStr, loc); perr == nil {
			from = t
		}
	}
	if toStr != "" {
		if t, perr := time.ParseInLocation("2006-01-02", toStr, loc); perr == nil {
			to = t.AddDate(0, 0, 1) // inclusive of the 'to' calendar day
		}
	}
	return from, to, nil
}

// timeEntryDTO is the wire shape for a time entry. It embeds the money-free
// database.TimeEntry row and adds the computed worked minutes/hours (reusing
// database.WorkedMinutes — DRY). There is deliberately NO pay/rate/dollar field
// on this surface (contracts §0 no-staff-dollars).
type timeEntryDTO struct {
	database.TimeEntry
	WorkedMinutes int     `json:"worked_minutes"`
	WorkedHours   float64 `json:"worked_hours"`
}

func toTimeEntryDTO(e database.TimeEntry) timeEntryDTO {
	mins := database.WorkedMinutes(e)
	return timeEntryDTO{
		TimeEntry:     e,
		WorkedMinutes: mins,
		WorkedHours:   math.Round(float64(mins)/60.0*100) / 100,
	}
}

func toTimeEntryDTOs(entries []database.TimeEntry) []timeEntryDTO {
	out := make([]timeEntryDTO, 0, len(entries))
	for _, e := range entries {
		out = append(out, toTimeEntryDTO(e))
	}
	return out
}

// clockInDTO is the optional POST /me/clock-in body; shift_id ties the punch to
// a scheduled shift (best-effort, stored as-is).
type clockInDTO struct {
	ShiftID *uint `json:"shift_id"`
}

// ClockIn opens a punch for the caller's own staff_id. A second clock-in while
// one is open is a 409 (ErrAlreadyClockedIn).
func (h *TimeclockHandler) ClockIn(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	staffID, ok := staffIDFromContext(c)
	if !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Staff identity required")
		return
	}
	var in clockInDTO
	// Body is optional; ignore bind errors on an empty/whitespace body.
	_ = c.ShouldBindJSON(&in)
	entry, err := h.db.ClockIn(businessID, staffID, in.ShiftID)
	if err != nil {
		if errors.Is(err, database.ErrAlreadyClockedIn) {
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "You are already clocked in")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to clock in")
		return
	}
	publishTimeclockEntry(businessID, entry.ID, entry.Status)
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": toTimeEntryDTO(*entry)})
}

// ClockOut closes the caller's open punch and moves it to pending_review. No open
// entry to close is a 409 (ErrNotClockedIn).
func (h *TimeclockHandler) ClockOut(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	staffID, ok := staffIDFromContext(c)
	if !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Staff identity required")
		return
	}
	entry, err := h.db.ClockOut(businessID, staffID)
	if err != nil {
		if errors.Is(err, database.ErrNotClockedIn) {
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "You are not clocked in")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to clock out")
		return
	}
	// A clock-out lands the entry in pending_review — nudge the manager queue.
	publishTimeclockEntry(businessID, entry.ID, entry.Status)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": toTimeEntryDTO(*entry)})
}

// breakDTO is the POST /me/break body.
type breakDTO struct {
	Minutes int `json:"minutes"`
}

// Break adds minutes to the caller's open punch's break total. Invalid/excessive
// break is a 400; no open entry is a 409.
func (h *TimeclockHandler) Break(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	staffID, ok := staffIDFromContext(c)
	if !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Staff identity required")
		return
	}
	var in breakDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	entry, err := h.db.AddBreak(businessID, staffID, in.Minutes)
	if err != nil {
		switch {
		case errors.Is(err, database.ErrInvalidBreak):
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Break minutes must be 0 or more")
		case errors.Is(err, database.ErrBreakExceedsElapsed):
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Break cannot exceed time worked")
		case errors.Is(err, database.ErrNotClockedIn):
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "You are not clocked in")
		default:
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to add break")
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": toTimeEntryDTO(*entry)})
}

// GetMyTimesheet returns the caller's own time entries (hours/minutes only, no
// dollars). With no query params it returns all own entries (back-compat,
// TodayCard). With ?from/?to/?status it returns a bounded date-range read. from/to
// are YYYY-MM-DD; to is inclusive of its calendar day. Default window is the last
// 8 weeks. Envelope: {"success":true,"data":[…]}.
func (h *TimeclockHandler) GetMyTimesheet(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	staffID, ok := staffIDFromContext(c)
	if !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Staff identity required")
		return
	}
	fromStr := strings.TrimSpace(c.Query("from"))
	toStr := strings.TrimSpace(c.Query("to"))
	status := strings.TrimSpace(c.Query("status"))

	var entries []database.TimeEntry
	var err error
	if fromStr == "" && toStr == "" {
		// Back-compat: no range → all own entries (existing behavior, TodayCard).
		entries, err = h.db.ListTimesheet(businessID, staffID)
	} else {
		from, to, perr := h.parseBusinessDayRange(businessID, fromStr, toStr)
		if perr != nil {
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to resolve business timezone")
			return
		}
		entries, err = h.db.ListTimesheetRange(businessID, staffID, from, to, status)
	}
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load timesheet")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": toTimeEntryDTOs(entries)})
}

// ListTimesheets returns the manager review queue (timeclock:manage). status=""
// defaults to pending_review; ?status= filters. Hours/minutes only — labor-$ is
// never on this surface (it lives in the owner-gated accounting labor endpoint).
//
// BE-first pagination: with NO ?offset/?limit param the response is the legacy
// bare-array {data:[…]} (unchanged for old clients). With ?offset or ?limit (and
// optional ?from/?to YYYY-MM-DD window) it returns a bounded page plus a total
// count {data:[…], total, offset, limit} so the UI can page with an honest total.
func (h *TimeclockHandler) ListTimesheets(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	status := strings.TrimSpace(c.Query("status"))
	offsetStr := strings.TrimSpace(c.Query("offset"))
	limitStr := strings.TrimSpace(c.Query("limit"))
	fromStr := strings.TrimSpace(c.Query("from"))
	toStr := strings.TrimSpace(c.Query("to"))

	// Legacy path: no pagination params → the original bare-array shape.
	if offsetStr == "" && limitStr == "" && fromStr == "" && toStr == "" {
		entries, err := h.db.ListForReview(businessID, status)
		if err != nil {
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load timesheets")
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": toTimeEntryDTOs(entries)})
		return
	}

	offset := 0
	if n, err := strconv.Atoi(offsetStr); err == nil && n > 0 {
		offset = n
	}
	limit := 50
	if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
		limit = n
	}
	from, to, perr := h.parseBusinessDayRange(businessID, fromStr, toStr)
	if perr != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to resolve business timezone")
		return
	}
	entries, total, err := h.db.ListForReviewPaged(businessID, status, from, to, offset, limit)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load timesheets")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    toTimeEntryDTOs(entries),
		"total":   total,
		"offset":  offset,
		"limit":   limit,
	})
}

// ApproveTimesheet approves a pending_review entry (timeclock:manage). A
// non-pending entry is a 409 (illegal transition); a cross-tenant/unknown entry
// is a 404. The approver is the caller's own staff_id.
func (h *TimeclockHandler) ApproveTimesheet(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	entryID, ok := parseParamUint(c, "entryId")
	if !ok {
		return
	}
	entry, err := h.db.ApproveTimeEntry(businessID, entryID, c.GetUint("staff_id"))
	if err != nil {
		switch {
		case errors.Is(err, database.ErrTimeEntryNotFound):
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Time entry not found")
		case errors.Is(err, database.ErrTimeEntryNotPendingReview):
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Time entry is not pending review")
		default:
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to approve time entry")
		}
		return
	}
	publishTimeclockEntry(businessID, entry.ID, entry.Status)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": toTimeEntryDTO(*entry)})
}

// rejectEntryDTO is the POST /timesheets/:entryId/reject body.
type rejectEntryDTO struct {
	Reason string `json:"reason"`
}

// RejectTimesheet rejects a pending_review entry (timeclock:manage), returning it
// to the staffer with the manager's reason. A non-pending entry is a 409; a
// cross-tenant/unknown entry is a 404.
func (h *TimeclockHandler) RejectTimesheet(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	entryID, ok := parseParamUint(c, "entryId")
	if !ok {
		return
	}
	var in rejectEntryDTO
	_ = c.ShouldBindJSON(&in) // reason is optional
	entry, err := h.db.RejectTimeEntry(businessID, entryID, c.GetUint("staff_id"), in.Reason)
	if err != nil {
		switch {
		case errors.Is(err, database.ErrTimeEntryNotFound):
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Time entry not found")
		case errors.Is(err, database.ErrTimeEntryNotPendingReview):
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Time entry is not pending review")
		default:
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to reject time entry")
		}
		return
	}
	publishTimeclockEntry(businessID, entry.ID, entry.Status)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": toTimeEntryDTO(*entry)})
}

// editEntryDTO is the PATCH /timesheets/:entryId body (manager correction of a
// mispunch: adjust clock-in/out + break + note).
type editEntryDTO struct {
	ClockInAt    time.Time  `json:"clock_in_at"`
	ClockOutAt   *time.Time `json:"clock_out_at"`
	BreakMinutes int        `json:"break_minutes"`
	Note         string     `json:"note"`
}

// EditTimesheet applies a manager correction to a NON-approved entry
// (timeclock:manage): adjust clock-in/out/break/note. The entry stays reviewable
// (a rejected entry returns to pending). An approved entry is locked (409). The
// original values are preserved in the RBAC audit trail server-side.
func (h *TimeclockHandler) EditTimesheet(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	entryID, ok := parseParamUint(c, "entryId")
	if !ok {
		return
	}
	var in editEntryDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if in.ClockInAt.IsZero() || in.ClockOutAt == nil || in.ClockOutAt.IsZero() {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "clock_in_at and clock_out_at are required")
		return
	}
	entry, err := h.db.EditTimeEntry(businessID, entryID, c.GetUint("staff_id"), in.ClockInAt, in.ClockOutAt, in.BreakMinutes, in.Note)
	if err != nil {
		switch {
		case errors.Is(err, database.ErrTimeEntryNotFound):
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Time entry not found")
		case errors.Is(err, database.ErrTimeEntryNotEditable):
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Time entry is already approved and cannot be edited")
		case errors.Is(err, database.ErrTimeEntryInvalidRange):
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "clock_out_at must be after clock_in_at")
		case errors.Is(err, database.ErrInvalidBreak):
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Break minutes must be 0 or more")
		case errors.Is(err, database.ErrBreakExceedsElapsed):
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Break cannot exceed time worked")
		default:
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to edit time entry")
		}
		return
	}
	publishTimeclockEntry(businessID, entry.ID, entry.Status)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": toTimeEntryDTO(*entry)})
}

// manualEntryDTO is the POST /time-entries body (manager-authored entry for a
// staff member who forgot to punch).
type manualEntryDTO struct {
	StaffID      uint       `json:"staff_id"`
	ShiftID      *uint      `json:"shift_id"`
	ClockInAt    time.Time  `json:"clock_in_at"`
	ClockOutAt   *time.Time `json:"clock_out_at"`
	BreakMinutes int        `json:"break_minutes"`
	Note         string     `json:"note"`
}

// CreateManualEntry files a manager-authored entry straight into pending_review
// (timeclock:manage). Validates the staff/range/break before any write.
func (h *TimeclockHandler) CreateManualEntry(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	var in manualEntryDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if in.StaffID == 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "staff_id is required")
		return
	}
	if in.ClockInAt.IsZero() || in.ClockOutAt == nil || in.ClockOutAt.IsZero() {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "clock_in_at and clock_out_at are required")
		return
	}
	entry, err := h.db.CreateManualEntry(businessID, in.StaffID, in.ShiftID, in.ClockInAt, in.ClockOutAt, in.BreakMinutes, in.Note)
	if err != nil {
		switch {
		case errors.Is(err, database.ErrTimeEntryInvalidRange):
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "clock_out_at must be after clock_in_at")
		case errors.Is(err, database.ErrInvalidBreak):
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Break minutes must be 0 or more")
		case errors.Is(err, database.ErrBreakExceedsElapsed):
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Break cannot exceed time worked")
		case errors.Is(err, database.ErrStaffNotFound):
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Staff member not found")
		case errors.Is(err, database.ErrTimeEntryOpenPunchBlocksManual):
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Staff is still clocked in; clock out before filing a manual entry")
		case errors.Is(err, database.ErrTimeEntryOverlap):
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "This time window overlaps an existing entry for this staff member")
		default:
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to create time entry")
		}
		return
	}
	publishTimeclockEntry(businessID, entry.ID, entry.Status)
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": toTimeEntryDTO(*entry)})
}
