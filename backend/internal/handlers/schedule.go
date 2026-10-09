package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// ScheduleHandler serves the weekly schedule + shift endpoints (Slice 2). Reads
// are row-scoped in the service layer: managers/owners (schedule:write) see the
// draft + every shift; plain staff (schedule:read only) see a published week
// with just their own + open shifts. No money fields anywhere in this surface.
type ScheduleHandler struct{ db *database.DB }

func NewScheduleHandler(db *database.DB) *ScheduleHandler { return &ScheduleHandler{db: db} }

// parseWeekStart reads the ?week= query param (a YYYY-MM-DD date or RFC3339
// timestamp) and returns the normalized week_start value matched against
// schedules.week_start. It delegates to parseWeekValue so the GET reader and the
// CreateDraft builder normalize identically — a GET and a POST for the same
// calendar week always resolve to the same row.
func parseWeekStart(c *gin.Context) (time.Time, bool) {
	raw := strings.TrimSpace(c.Query("week"))
	if raw == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "week is required (YYYY-MM-DD)")
		return time.Time{}, false
	}
	ws, err := parseWeekValue(raw)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "week must be YYYY-MM-DD or RFC3339")
		return time.Time{}, false
	}
	return ws, true
}

// callerCanScheduleWrite reports whether the caller holds schedule:write
// (manager/owner) and therefore gets the unscoped operator view. It mirrors the
// server RBAC match semantics (exact, "cat:*", "*:*") so the in-handler scoping
// decision is identical to the route-level gate.
func callerCanScheduleWrite(c *gin.Context) bool {
	perms, allAccess := server.ResolveContextPermissions(c)
	if allAccess {
		return true
	}
	return permMatch(perms, "schedule:write")
}

func permMatch(perms []string, required string) bool {
	for _, p := range perms {
		if p == required || p == "*:*" {
			return true
		}
		if strings.HasSuffix(p, ":*") && strings.HasPrefix(required, strings.TrimSuffix(p, ":*")+":") {
			return true
		}
	}
	return false
}

// Get returns the schedule + shifts for ?week=, row-scoped by the caller's role.
// Envelope: {"success":true,"data":{"schedule":…|null,"shifts":[…]}}.
func (h *ScheduleHandler) Get(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	weekStart, ok := parseWeekStart(c)
	if !ok {
		return
	}

	var (
		sched  *database.Schedule
		shifts []database.Shift
		err    error
	)
	if callerCanScheduleWrite(c) {
		sched, shifts, err = h.db.GetScheduleForWeek(businessID, weekStart)
	} else {
		staffID := c.GetUint("staff_id")
		sched, shifts, err = h.db.GetPublishedScheduleForStaff(businessID, weekStart, staffID)
	}
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load schedule")
		return
	}
	if shifts == nil {
		shifts = []database.Shift{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"schedule": sched, "shifts": shifts}})
}

// scheduleCreateDraftDTO is the POST /schedule body: a week-start date.
type scheduleCreateDraftDTO struct {
	Week string `json:"week"`
}

// CreateDraft lazily creates (or returns) the draft schedule for ?week / body
// week. Write-gated (schedule:write) at the route. Returns the schedule.
func (h *ScheduleHandler) CreateDraft(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	var in scheduleCreateDraftDTO
	_ = c.ShouldBindJSON(&in)
	week := strings.TrimSpace(in.Week)
	if week == "" {
		week = strings.TrimSpace(c.Query("week"))
	}
	weekStart, perr := parseWeekValue(week)
	if perr != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "week is required (YYYY-MM-DD)")
		return
	}
	sched, err := h.db.GetOrCreateDraftSchedule(businessID, weekStart, c.GetUint("staff_id"))
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to create draft schedule")
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": sched})
}

// parseWeekValue parses a YYYY-MM-DD or RFC3339 week string and normalizes it to
// the start of its UTC calendar day (time-of-day truncated). Now that
// (business_id, week_start) is UNIQUE, flooring guarantees a non-midnight RFC3339
// can't key a near-duplicate row, and GET/POST for the same week resolve to one
// row. Shared by CreateDraft and the GET reader's parseWeekStart.
func parseWeekValue(week string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", week); err == nil {
		return weekFloorUTC(t), nil
	}
	t, err := time.Parse(time.RFC3339, week)
	if err != nil {
		return time.Time{}, err
	}
	return weekFloorUTC(t), nil
}

// weekFloorUTC truncates t to midnight of its UTC calendar day.
func weekFloorUTC(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// shiftCreateDTO is the POST /shifts body. staff_id omitted/null = open shift.
type shiftCreateDTO struct {
	ScheduleID   uint      `json:"schedule_id"`
	PositionID   uint      `json:"position_id"`
	StaffID      *uint     `json:"staff_id"`
	StartsAt     time.Time `json:"starts_at"`
	EndsAt       time.Time `json:"ends_at"`
	BreakMinutes int       `json:"break_minutes"`
	Notes        string    `json:"notes"`
}

// CreateShift validates the whole DTO (reject-before-persist), confirms the
// schedule belongs to the business, and inserts the shift. Write-gated.
func (h *ScheduleHandler) CreateShift(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	var in shiftCreateDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if in.ScheduleID == 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "schedule_id is required")
		return
	}
	if in.PositionID == 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "position_id is required")
		return
	}
	if in.StartsAt.IsZero() || in.EndsAt.IsZero() {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "starts_at and ends_at are required")
		return
	}
	if !in.EndsAt.After(in.StartsAt) {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "ends_at must be after starts_at")
		return
	}
	if in.BreakMinutes < 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "break_minutes must be >= 0")
		return
	}
	// Confirm the schedule is in this business before creating a shift under it.
	if _, err := h.db.GetScheduleByID(businessID, in.ScheduleID); err != nil {
		if err == database.ErrScheduleNotFound {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Schedule not found")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load schedule")
		return
	}

	sh := &database.Shift{
		BusinessID: businessID, ScheduleID: in.ScheduleID, PositionID: in.PositionID,
		StaffID: in.StaffID, StartsAt: in.StartsAt, EndsAt: in.EndsAt,
		BreakMinutes: in.BreakMinutes, Notes: in.Notes, CreatedByStaffID: c.GetUint("staff_id"),
	}
	if err := h.db.CreateShift(sh); err != nil {
		h.respondShiftServiceErr(c, err)
		return
	}
	// A shift created into an ALREADY-published schedule and assigned is a live
	// change the staff should hear about immediately (SSE registered in 2.5).
	if sh.Published && sh.StaffID != nil {
		events.GetHub().PublishJSON(businessID, "shift.assigned", gin.H{
			"shift_id": sh.ID, "staff_id": *sh.StaffID, "starts_at": sh.StartsAt,
		})
		notifyStaff(h.db, businessID, []uint{*sh.StaffID}, "shift.assigned",
			services.PushKeyShiftAssigned,
			services.PushArgs{ShiftDate: sh.StartsAt.Format("2006-01-02")},
			"/staff/home?tab=schedule")
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": sh})
}

// copyWeekDTO is the POST /schedule/copy-week body. from_week/to_week are
// YYYY-MM-DD (or RFC3339); only_empty_days=true copies only into destination
// weekdays that have no shift yet (idempotent "fill the gaps").
type copyWeekDTO struct {
	FromWeek      string `json:"from_week"`
	ToWeek        string `json:"to_week"`
	OnlyEmptyDays bool   `json:"only_empty_days"`
}

// CopyWeek copies every shift from from_week into to_week in ONE transactional
// server call — the replacement for the client's N serial per-shift POSTs that
// lost the copy button on any partial failure. Copied shifts land in the
// destination DRAFT (unpublished) so the operator reviews + publishes them; the
// copy never goes live on its own. Returns {created, skipped}. Write-gated.
func (h *ScheduleHandler) CopyWeek(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	var in copyWeekDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	fromWeek, ferr := parseWeekValue(strings.TrimSpace(in.FromWeek))
	if ferr != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "from_week is required (YYYY-MM-DD)")
		return
	}
	toWeek, terr := parseWeekValue(strings.TrimSpace(in.ToWeek))
	if terr != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "to_week is required (YYYY-MM-DD)")
		return
	}
	if fromWeek.Equal(toWeek) {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "from_week and to_week must differ")
		return
	}
	res, err := h.db.CopyScheduleWeek(businessID, fromWeek, toWeek, c.GetUint("staff_id"), in.OnlyEmptyDays)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to copy week")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": res})
}

// shiftUpdateDTO is the PATCH /shifts/:shiftId body. All fields optional; only
// the keys sent are touched. staff_id semantics: nil = unchanged, 0 = unassign
// (open the shift / SQL NULL), >0 = assign (valid staff ids are >= 1).
type shiftUpdateDTO struct {
	PositionID   *uint      `json:"position_id"`
	StaffID      *int       `json:"staff_id"`
	StartsAt     *time.Time `json:"starts_at"`
	EndsAt       *time.Time `json:"ends_at"`
	BreakMinutes *int       `json:"break_minutes"`
	Notes        *string    `json:"notes"`
}

// UpdateShift applies a whitelisted partial update, reject-before-persist.
func (h *ScheduleHandler) UpdateShift(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	shiftID, ok := parseParamUint(c, "shiftId")
	if !ok {
		return
	}
	var in shiftUpdateDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	fields := map[string]interface{}{}
	if in.PositionID != nil {
		if *in.PositionID == 0 {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "position_id must be > 0")
			return
		}
		fields["position_id"] = *in.PositionID
	}
	if in.StaffID != nil {
		switch {
		case *in.StaffID == 0:
			fields["staff_id"] = nil // explicit unassign -> SQL NULL
		case *in.StaffID < 0:
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "staff_id must be >= 0")
			return
		default:
			fields["staff_id"] = uint(*in.StaffID)
		}
	}
	if in.StartsAt != nil {
		if in.StartsAt.IsZero() {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "starts_at is invalid")
			return
		}
		fields["starts_at"] = *in.StartsAt
	}
	if in.EndsAt != nil {
		if in.EndsAt.IsZero() {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "ends_at is invalid")
			return
		}
		fields["ends_at"] = *in.EndsAt
	}
	if in.BreakMinutes != nil {
		if *in.BreakMinutes < 0 {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "break_minutes must be >= 0")
			return
		}
		fields["break_minutes"] = *in.BreakMinutes
	}
	if in.Notes != nil {
		fields["notes"] = *in.Notes
	}
	if len(fields) == 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "no fields to update")
		return
	}
	if err := h.db.UpdateShiftFields(businessID, shiftID, fields); err != nil {
		h.respondShiftServiceErr(c, err)
		return
	}
	// Live change to a published shift -> notify the affected scope.
	if sh, err := h.db.GetShiftByID(businessID, shiftID); err == nil && sh.Published {
		payload := gin.H{"shift_id": sh.ID, "starts_at": sh.StartsAt}
		if sh.StaffID != nil {
			payload["staff_id"] = *sh.StaffID
		}
		events.GetHub().PublishJSON(businessID, "shift.updated", payload)
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// DeleteShift hard-deletes a shift, tenant-scoped. Write-gated.
func (h *ScheduleHandler) DeleteShift(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	shiftID, ok := parseParamUint(c, "shiftId")
	if !ok {
		return
	}
	if err := h.db.DeleteShift(businessID, shiftID); err != nil {
		h.respondShiftServiceErr(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// respondShiftServiceErr maps schedule/shift service errors to HTTP responses.
func (h *ScheduleHandler) respondShiftServiceErr(c *gin.Context, err error) {
	switch err {
	case database.ErrShiftInvalidRange:
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "ends_at must be after starts_at")
	case database.ErrShiftNotFound:
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Shift not found")
	case database.ErrPositionNotFound:
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Position not found")
	case database.ErrStaffNotFound:
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Staff not found")
	default:
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to save shift")
	}
}

// Publish flips a draft schedule to published (transactional, idempotent) and
// fans out: one schedule.published SSE, a shift.assigned SSE per assigned shift,
// and a per-staff "your week" notification enqueued on the outbox. A re-publish
// of an already-published schedule is a 409 (no events, no duplicate
// notifications). Publish-gated (schedule:publish) at the route.
//
// NOTE: the outbox delivery worker does NOT yet honor per-user mute or the
// BusinessScheduleSettings quiet-hours window for these events — only the
// shift-reminder scheduler enforces quiet hours today. Quiet-hours/mute
// enforcement for outbox-dispatched schedule events is a cross-cutting follow-up
// (not in scope for this slice); do not assume it is guaranteed here.
func (h *ScheduleHandler) Publish(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	scheduleID, ok := parseParamUint(c, "scheduleId")
	if !ok {
		return
	}
	sched, shifts, err := h.db.PublishSchedule(businessID, scheduleID, c.GetUint("staff_id"))
	if err != nil {
		if err == database.ErrScheduleNotDraft {
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Schedule is already published")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to publish schedule")
		return
	}

	events.GetHub().PublishJSON(businessID, "schedule.published", gin.H{
		"schedule_id": sched.ID, "week_start": sched.WeekStart,
	})

	byStaff := map[uint]int{}
	seen := make(map[uint]struct{})
	distinct := make([]uint, 0)
	for i := range shifts {
		sh := &shifts[i]
		if sh.StaffID == nil {
			continue
		}
		events.GetHub().PublishJSON(businessID, "shift.assigned", gin.H{
			"shift_id": sh.ID, "staff_id": *sh.StaffID, "starts_at": sh.StartsAt,
		})
		byStaff[*sh.StaffID]++
		if _, ok := seen[*sh.StaffID]; !ok {
			seen[*sh.StaffID] = struct{}{}
			distinct = append(distinct, *sh.StaffID)
		}
	}
	notifyStaff(h.db, businessID, distinct, "schedule.published",
		services.PushKeySchedulePublished,
		services.PushArgs{WeekOf: sched.WeekStart.Format("2006-01-02")},
		"/staff/home?tab=schedule")

	// Operator Telegram: one roll-up per publish (not per staffer — the assigned
	// staff are reminded on their own channel via notifyStaff above). Gated so no
	// dead outbox rows accrue for businesses without Telegram / with the event off.
	if services.ShouldEnqueueTelegramNotification(businessID, services.PluginEventSchedulePublished) {
		totalShifts := 0
		for _, count := range byStaff {
			totalShifts += count
		}
		if _, _, err := services.EnqueuePluginNotification(services.PluginNotificationEvent{
			BusinessID: businessID,
			EventType:  services.PluginEventSchedulePublished,
			EventID:    fmt.Sprintf("schedule:%d:published", sched.ID),
			Payload: map[string]interface{}{
				"week_start":  sched.WeekStart,
				"shift_count": totalShifts,
				"staff_count": len(distinct),
			},
			CreatedAt: time.Now().UTC(),
		}, "telegram"); err != nil {
			log.Printf("Failed to enqueue Telegram schedule.published notification for business_id=%d schedule_id=%d: %v", businessID, sched.ID, err)
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"schedule": sched, "shifts": shifts}})
}
