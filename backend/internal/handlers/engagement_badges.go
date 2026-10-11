package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
)

// EngagementBadgesHandler serves the staff-nav badge counts for the buried
// engagement surfaces (unacked announcements + pending checklists) so the More
// menu and bottom nav can nudge staff toward work they'd otherwise miss. It is a
// /me self route: self-scoped by the context staff_id, and rejects staff_id==0
// (an owner with no staff row) with 403 — the Slice-9 self-route guard. Money-free.
type EngagementBadgesHandler struct{ db *database.DB }

func NewEngagementBadgesHandler(db *database.DB) *EngagementBadgesHandler {
	return &EngagementBadgesHandler{db: db}
}

// Badges returns the caller's own "needs attention" counts. Two cheap counts
// (unacked require_ack announcements resolved by audience, plus not-yet-complete
// checklist runs assigned to the caller). Envelope:
// {"success":true,"data":{"unacked_announcements":N,"pending_checklists":M}}.
func (h *EngagementBadgesHandler) Badges(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	staffID, ok := staffIDFromContext(c)
	if !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Staff identity required")
		return
	}

	unacked, err := h.db.CountUnackedAnnouncements(businessID, staffID, callerStaffRole(c))
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load badges")
		return
	}
	pending, err := h.db.CountPendingChecklistRuns(businessID, staffID)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load badges")
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"unacked_announcements": unacked,
		"pending_checklists":    pending,
	}})
}
