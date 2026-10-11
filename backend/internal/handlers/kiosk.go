package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
)

// KioskHandler serves the shared-terminal clock-in surface (Phase 4b). The
// device runs under a manager/owner session (timeclock:manage); individual
// staff identify with a PIN to toggle their OWN punch. No independent staff
// session is minted — the PIN attributes the punch to the staffer, nothing
// more. Every response is money-free (contracts §0 no-staff-dollars).
type KioskHandler struct{ db *database.DB }

func NewKioskHandler(db *database.DB) *KioskHandler { return &KioskHandler{db: db} }

// Roster returns the active staff of the business for the terminal to render as
// name tiles, each annotated with whether they can clock in (has_pin) and their
// current punch state (on_clock / clock_in_at). Money-free.
func (h *KioskHandler) Roster(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	roster, err := h.db.KioskRoster(businessID)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load roster")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"roster": roster}})
}

// kioskPunchDTO is the POST /kiosk/punch body: the staffer selected on the tile
// grid plus the PIN they entered. The backend decides clock-in vs clock-out
// from the staffer's current open-entry state (a true toggle) — the terminal
// never has to track state.
type kioskPunchDTO struct {
	StaffID uint   `json:"staff_id"`
	Pin     string `json:"pin"`
}

const (
	kioskActionClockedIn  = "clocked_in"
	kioskActionClockedOut = "clocked_out"
)

// kioskPunchResult is the money-free envelope returned by a successful punch.
type kioskPunchResult struct {
	Action  string       `json:"action"`
	StaffID uint         `json:"staff_id"`
	Name    string       `json:"name"`
	Entry   timeEntryDTO `json:"entry"`
}

// Punch toggles the selected staffer's clock state after verifying their PIN.
//
// Tenant safety: the staffer is loaded scoped to the business + active FIRST
// (database.VerifyStaffPin is NOT tenant-scoped), so an unknown / foreign /
// inactive staff id is a 404 before any PIN check runs.
func (h *KioskHandler) Punch(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	var in kioskPunchDTO
	if err := c.ShouldBindJSON(&in); err != nil || in.StaffID == 0 || in.Pin == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "staff_id and pin are required")
		return
	}

	staff, err := h.db.GetKioskStaff(businessID, in.StaffID)
	if err != nil {
		if errors.Is(err, database.ErrStaffNotFound) {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Staff member not found")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load staff member")
		return
	}
	if !staff.HasPin {
		server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "This staff member has not set a PIN yet")
		return
	}

	if _, verr := server.VerifyKioskPin(in.StaffID, in.Pin); verr != nil {
		switch {
		case errors.Is(verr, server.ErrKioskPinLocked):
			server.RespondWithError(c, http.StatusTooManyRequests, server.ErrCodeRateLimited, "Too many incorrect PIN attempts. Try again later.")
		case errors.Is(verr, database.ErrPinInvalid), errors.Is(verr, database.ErrPinNotSet):
			server.RespondWithError(c, http.StatusForbidden, server.ErrCodePinInvalid, "Incorrect PIN")
		default:
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to verify PIN")
		}
		return
	}

	// Toggle: an open punch → clock out; otherwise → clock in. The ClockIn /
	// ClockOut CAS guards mean that even a race that flips state between the
	// read and the write surfaces as a conflict (retryable) rather than a
	// double punch.
	action := kioskActionClockedIn
	var entry *database.TimeEntry
	if _, oerr := h.db.GetOpenEntry(businessID, in.StaffID); oerr == nil {
		action = kioskActionClockedOut
		entry, err = h.db.ClockOut(businessID, in.StaffID)
	} else if errors.Is(oerr, database.ErrNotClockedIn) {
		entry, err = h.db.ClockIn(businessID, in.StaffID, nil)
	} else {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to read clock state")
		return
	}
	if err != nil {
		if errors.Is(err, database.ErrAlreadyClockedIn) || errors.Is(err, database.ErrNotClockedIn) {
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Clock state changed, please try again")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to record punch")
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": kioskPunchResult{
		Action:  action,
		StaffID: staff.StaffID,
		Name:    staff.Name,
		Entry:   toTimeEntryDTO(*entry),
	}})
}
