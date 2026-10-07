package server

import (
	"strconv"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"

	"github.com/gin-gonic/gin"
)

// ExtractStaffIDFromContext returns a non-nil *uint when the current gin
// context is authenticated as a staff member. Owner / user / web3 / empty
// token contexts return nil so downstream service-layer attribution stays
// untouched (matching the original semantic that analytics only count
// staff-driven closures).
//
// This helper exists so payment handlers can pass staff attribution *into*
// the service call that performs the status transition, instead of relying
// on a post-hoc UPDATE. The post-hoc pattern was unsafe: a bill auto-closed
// by a Web3 webhook (no staff) could later be misattributed to any staff
// member who hit an endpoint that ran the stamp helper.
func ExtractStaffIDFromContext(c *gin.Context) *uint {
	if c == nil {
		return nil
	}
	tokenTypeAny, _ := c.Get("token_type")
	tokenType, _ := tokenTypeAny.(string)
	if tokenType != "staff" {
		return nil
	}

	staffIDAny, exists := c.Get("staff_id")
	if !exists {
		return nil
	}

	switch v := staffIDAny.(type) {
	case uint:
		if v > 0 {
			u := v
			return &u
		}
	case uint64:
		if v > 0 {
			u := uint(v)
			return &u
		}
	case int:
		if v > 0 {
			u := uint(v)
			return &u
		}
	case int64:
		if v > 0 {
			u := uint(v)
			return &u
		}
	case float64:
		if v > 0 {
			u := uint(v)
			return &u
		}
	case string:
		if n, err := strconv.ParseUint(v, 10, 32); err == nil && n > 0 {
			u := uint(n)
			return &u
		}
	}
	return nil
}

// StampBillClosedByStaff sets bill.ClosedByStaffID (and closed_at if missing)
// from the current staff context when the bill has transitioned to the
// "closed" terminal state (unpaid closure via CloseBill). The "paid"
// transition is now handled atomically inside the service layer
// (applyBillPaymentAmounts) using ExtractStaffIDFromContext, so this helper
// only covers the no-payment CloseBill path.
//
// This function is defensive and idempotent:
//   - No-op if the caller isn't a staff token (owners leave the field nil so
//     analytics reflect that the closure wasn't initiated by a staff member).
//   - No-op if the bill hasn't transitioned to "closed" yet.
//   - Only stamps closed_by_staff_id when currently NULL — concurrent closures
//     don't overwrite the first stamp.
//   - Only stamps closed_at when currently NULL so we don't clobber an earlier
//     closure timestamp.
//
// Restricting the UPDATE to status=closed (not paid) closes the
// misattribution gap: a bill auto-closed by a Web3 webhook (no staff) in the
// "paid" state can no longer be silently claimed by a staff member who later
// hits any endpoint that happens to call this helper.
func StampBillClosedByStaff(c *gin.Context, billID uint) {
	if c == nil || billID == 0 {
		return
	}

	staffIDPtr := ExtractStaffIDFromContext(c)
	if staffIDPtr == nil {
		return
	}
	staffID := *staffIDPtr

	db := database.GetDB()
	if db == nil {
		return
	}

	// Single atomic UPDATE: sets closed_by_staff_id and stamps closed_at in one
	// round-trip. COALESCE preserves any existing closed_at so we never clobber
	// an earlier timestamp. The NULL guard on closed_by_staff_id ensures the
	// first staff member to land the UPDATE wins — racing closures don't
	// overwrite each other. Restricted to status=closed (paid-path attribution
	// is owned by the service layer).
	result := db.Exec(
		`UPDATE bills
		    SET closed_by_staff_id = ?,
		        closed_at = COALESCE(closed_at, ?)
		  WHERE id = ?
		    AND closed_by_staff_id IS NULL
		    AND status = ?`,
		staffID,
		time.Now(),
		billID,
		database.BillStatusClosed,
	)
	if result.Error != nil {
		logger.Logger.Errorf("bill-attribution: StampBillClosedByStaff update failed for bill %d staff %d: %v", billID, staffID, result.Error)
	}
}
