package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
)

// NotificationsHandler serves the per-staff inbox. Every route is a /me self
// route: self-scoped by the context staff_id, and rejects staff_id==0 (owner
// with no staff row) with 403 — the Slice-9 self-route guard. No money fields.
type NotificationsHandler struct{ db *database.DB }

func NewNotificationsHandler(db *database.DB) *NotificationsHandler {
	return &NotificationsHandler{db: db}
}

// ListMine returns the caller's own inbox, newest first. ?limit (cap 50),
// ?before (keyset id cursor). Envelope: {"success":true,"data":[…]}.
func (h *NotificationsHandler) ListMine(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	staffID, ok := staffIDFromContext(c)
	if !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Staff identity required")
		return
	}
	limit, _ := strconv.Atoi(strings.TrimSpace(c.Query("limit")))
	before64, _ := strconv.ParseUint(strings.TrimSpace(c.Query("before")), 10, 64)
	rows, err := h.db.ListStaffNotifications(businessID, staffID, limit, uint(before64))
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load notifications")
		return
	}
	if rows == nil {
		rows = []database.StaffNotification{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": rows})
}

// UnreadCount returns {"data":{"count":N}} for the caller's own inbox.
func (h *NotificationsHandler) UnreadCount(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	staffID, ok := staffIDFromContext(c)
	if !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Staff identity required")
		return
	}
	n, err := h.db.CountUnreadStaffNotifications(businessID, staffID)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to count notifications")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"count": n}})
}

// markReadDTO is the POST body: either explicit ids or all:true.
type markReadDTO struct {
	IDs []uint `json:"ids"`
	All bool   `json:"all"`
}

// MarkRead stamps read_at on the caller's own rows. {"data":{"updated":N}}.
func (h *NotificationsHandler) MarkRead(c *gin.Context) {
	businessID, ok := parseBusinessID(c)
	if !ok {
		return
	}
	staffID, ok := staffIDFromContext(c)
	if !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Staff identity required")
		return
	}
	var in markReadDTO
	if err := c.ShouldBindJSON(&in); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if !in.All && len(in.IDs) == 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "ids or all is required")
		return
	}
	updated, err := h.db.MarkStaffNotificationsRead(businessID, staffID, in.IDs, in.All)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to update notifications")
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"updated": updated}})
}
