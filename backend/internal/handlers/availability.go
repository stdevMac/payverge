package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/server"
)

// AvailabilityHandler serves the per-staff availability + time-off endpoints
// (Slice 3). Availability writes are always scoped to the caller's OWN staff_id
// (from the gin context, never a body value). The time-off list is row-scoped in
// the service by role: approvers (schedule:approve) see the whole queue; plain
// staff see only their own. Decisions are gated on schedule:approve and audited.
// No money fields anywhere in this surface.
type AvailabilityHandler struct{ db *database.DB }

func NewAvailabilityHandler(db *database.DB) *AvailabilityHandler {
	return &AvailabilityHandler{db: db}
}

// callerCanScheduleApprove reports whether the caller holds schedule:approve
// (manager/owner). It mirrors the server RBAC match semantics (exact, "cat:*",
// "*:*") via the shared permMatch helper so the in-handler row-scoping decision
// is identical to the route-level gate.
func callerCanScheduleApprove(c *gin.Context) bool {
	perms, allAccess := server.ResolveContextPermissions(c)
	if allAccess {
		return true
	}
	return permMatch(perms, "schedule:approve")
}

// GetMyAvailability returns the caller's own availability windows.
// Envelope: {"success":true,"data":[…]}.
func (h *AvailabilityHandler) GetMyAvailability(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	staffID, ok := staffIDFromContext(c)
	if !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Staff identity required")
		return
	}
	windows, err := h.db.GetAvailability(businessID, staffID)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load availability")
		return
	}
	if windows == nil {
		windows = []database.StaffAvailability{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": windows})
}

// availabilityWindowDTO is one window in the PUT body.
type availabilityWindowDTO struct {
	Weekday  int    `json:"weekday"`
	StartMin int    `json:"start_min"`
	EndMin   int    `json:"end_min"`
	Kind     string `json:"kind"`
}

// putAvailabilityDTO is the PUT /me/availability body: the full replacement set.
type putAvailabilityDTO struct {
	Windows []availabilityWindowDTO `json:"windows"`
}

// PutMyAvailability replaces the caller's entire availability set. The whole
// batch is validated before any write (reject-before-persist); the caller's own
// staff_id is stamped in the service, so a body can never target another person.
func (h *AvailabilityHandler) PutMyAvailability(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	staffID, ok := staffIDFromContext(c)
	if !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Staff identity required")
		return
	}
	var in putAvailabilityDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	windows := make([]database.StaffAvailability, 0, len(in.Windows))
	for _, w := range in.Windows {
		if w.Weekday < 0 || w.Weekday > 6 {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "weekday must be 0..6")
			return
		}
		if w.StartMin < 0 || w.EndMin > 1440 || w.StartMin >= w.EndMin {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "start_min/end_min must satisfy 0 <= start < end <= 1440")
			return
		}
		if w.Kind != database.AvailabilityKindPreferred && w.Kind != database.AvailabilityKindUnavailable {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "kind must be preferred or unavailable")
			return
		}
		windows = append(windows, database.StaffAvailability{
			Weekday: w.Weekday, StartMin: w.StartMin, EndMin: w.EndMin, Kind: w.Kind,
		})
	}
	saved, err := h.db.ReplaceAvailability(businessID, staffID, windows)
	if err != nil {
		if err == database.ErrAvailabilityInvalid {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid availability window")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to save availability")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": saved})
}

// GetTeamAvailability returns every staff member's recurring availability
// windows for the business, grouped by staff_id, for the manager's schedule-
// builder overlay. Route-gated on schedule:read (manager/owner). Availability is
// a per-weekday recurring pattern, so no date range is needed. No money fields.
// Envelope: {"success":true,"data":{"staff_availabilities":{"<staffId>":[…]}}}.
func (h *AvailabilityHandler) GetTeamAvailability(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	byStaff, err := h.db.GetTeamAvailability(businessID)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load team availability")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"staff_availabilities": byStaff}})
}

// createTimeOffDTO is the POST /me/time-off body.
type createTimeOffDTO struct {
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	Reason   string    `json:"reason"`
}

// CreateMyTimeOff files a pending time-off request for the caller and fans out a
// timeoff.requested SSE so approvers (schedule:approve subscribers) see it live.
func (h *AvailabilityHandler) CreateMyTimeOff(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	staffID, ok := staffIDFromContext(c)
	if !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Staff identity required")
		return
	}
	var in createTimeOffDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
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
	req, err := h.db.CreateTimeOff(businessID, staffID, in.StartsAt, in.EndsAt, in.Reason)
	if err != nil {
		if err == database.ErrTimeOffInvalidRange {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "ends_at must be after starts_at")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to create time-off request")
		return
	}
	events.GetHub().PublishJSON(businessID, "timeoff.requested", gin.H{
		"request_id": req.ID, "staff_id": req.StaffID,
		"starts_at": req.StartsAt, "ends_at": req.EndsAt,
	})
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": req})
}

// ListTimeOff returns time-off requests, row-scoped in the SERVICE by role:
// approvers (schedule:approve) get the whole queue (optionally ?status=
// filtered); plain staff get only their own. Route-gated on schedule:read.
func (h *AvailabilityHandler) ListTimeOff(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	status := strings.TrimSpace(c.Query("status"))
	var scope *uint
	if !callerCanScheduleApprove(c) {
		sid := c.GetUint("staff_id")
		scope = &sid
	}
	list, err := h.db.ListTimeOff(businessID, status, scope)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load time-off requests")
		return
	}
	if list == nil {
		list = []database.TimeOffRequest{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": list})
}

// decideTimeOffDTO is the POST /time-off/:reqId/decision body.
type decideTimeOffDTO struct {
	Approve bool   `json:"approve"`
	Reason  string `json:"reason"`
}

// DecideTimeOff approves or denies a pending request (schedule:approve), then
// fans out a timeoff.decided SSE so the requester (schedule:self subscriber)
// hears the outcome live. A non-pending request is a 409 (illegal transition); a
// cross-tenant/unknown request is a 404.
func (h *AvailabilityHandler) DecideTimeOff(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	reqID, ok := parseParamUint(c, "reqId")
	if !ok {
		return
	}
	var in decideTimeOffDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	updated, err := h.db.DecideTimeOff(businessID, reqID, c.GetUint("staff_id"), in.Approve, in.Reason)
	if err != nil {
		switch err {
		case database.ErrTimeOffNotFound:
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Time-off request not found")
		case database.ErrTimeOffNotPending:
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Time-off request is no longer pending")
		case database.ErrTimeOffExpired:
			server.RespondWithError(c, http.StatusConflict, "time_off_expired", "Time-off request has expired")
		default:
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to decide time-off request")
		}
		return
	}
	events.GetHub().PublishJSON(businessID, "timeoff.decided", gin.H{
		"request_id": updated.ID, "staff_id": updated.StaffID, "status": updated.Status,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": updated})
}
