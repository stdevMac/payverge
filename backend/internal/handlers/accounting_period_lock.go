package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/accounting"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetPeriodLock GET /accounting/period-lock — current books-closed state.
func (h *AccountingHandler) GetPeriodLock(c *gin.Context) {
	businessID, _, ok := h.loadBusiness(c)
	if !ok {
		return
	}
	lt, err := accounting.GetLockedThrough(h.db.GetGorm(), businessID)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load period lock")
		return
	}
	var through *string
	if lt != nil {
		s := lt.Format("2006-01-02")
		through = &s
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"locked_through": through,
		},
	})
}

type periodLockRequest struct {
	// LockedThrough YYYY-MM-DD; empty/null reopens books fully.
	LockedThrough *string `json:"locked_through"`
	Note          string  `json:"note"`
}

// PostPeriodLock POST /accounting/period-lock — owner-only append-only lock/unlock.
func (h *AccountingHandler) PostPeriodLock(c *gin.Context) {
	businessID, business, ok := h.loadBusiness(c)
	if !ok {
		return
	}
	// Owner-only (stricter than financial:write).
	if !server.CheckBusinessOwnership(c, business) {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Only the business owner can close or reopen the books")
		return
	}
	var req periodLockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	note := strings.TrimSpace(req.Note)
	if note == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "note is required")
		return
	}
	var lockedThrough *time.Time
	if req.LockedThrough != nil {
		// Present-but-empty is invalid for close (L6-24). Clients that mean
		// "reopen" must send JSON null / omit the field — not "".
		s := strings.TrimSpace(*req.LockedThrough)
		if s == "" {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "locked_through date is required to close the books")
			return
		}
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "locked_through must be YYYY-MM-DD")
			return
		}
		lockedThrough = &d
	}
	row := database.AccountingPeriodLock{
		BusinessID:    businessID,
		LockedThrough: lockedThrough,
		Note:          note,
	}
	if uid, ok := c.Get("user_id"); ok {
		if id, ok := uid.(uint); ok {
			row.LockedByUserID = &id
		}
	}
	// Take the books lock so a guarded write that is checking the period
	// right now either commits first or sees this lock.
	if err := h.db.GetGorm().Transaction(func(tx *gorm.DB) error {
		if err := accounting.LockBooksTx(tx, businessID); err != nil {
			return err
		}
		return tx.Create(&row).Error
	}); err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to record period lock")
		return
	}
	var through *string
	if lockedThrough != nil {
		s := lockedThrough.Format("2006-01-02")
		through = &s
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{"locked_through": through, "note": note}})
}

// respondPeriodLocked maps *PeriodLockedError to 409 period_locked. Any other
// non-nil error (e.g. the lock lookup itself failed) responds 500 — the check
// must fail closed, never let a mutation proceed unverified.
func respondPeriodLocked(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	var pe *accounting.PeriodLockedError
	if errors.As(err, &pe) {
		payload := gin.H{"error": pe.Error(), "code": "period_locked"}
		if pe.LockedThrough != nil {
			payload["locked_through"] = pe.LockedThrough.Format("2006-01-02")
		}
		c.JSON(http.StatusConflict, payload)
		return true
	}
	if errors.Is(err, accounting.ErrPeriodLocked) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "code": "period_locked"})
		return true
	}
	server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to verify accounting period lock")
	return true
}
