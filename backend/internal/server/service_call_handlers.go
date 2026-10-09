package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
	operational_alerts "github.com/stdevmac/payverge/backend/internal/services/operational_alerts"

	"github.com/gin-gonic/gin"
)

// serviceCallReasons is the closed enum of guest-selectable reasons. The
// operational_alerts service embeds the reason into staff-facing alert text,
// so this endpoint must never forward guest free text (see
// CreateServiceCallAlert's contract).
var serviceCallReasons = map[string]bool{"water": true, "order": true, "check": true}

// serviceCallCooldown is how long after a resolved call a table must wait
// before it can raise a new one (guest-side anti-spam, on top of the route
// rate limiter).
const serviceCallCooldown = 2 * time.Minute

type createServiceCallRequest struct {
	Reason string `json:"reason"`
}

// CreateServiceCallByTableCode lets a guest at a table raise a service call
// (water / order / check). A second reason while open/acknowledged upserts
// the same alert row (staff copy + metadata) rather than stacking duplicates;
// 429 during the post-resolve cooldown.
//
// POST /guest/table/:code/service-call
func CreateServiceCallByTableCode(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Table code is required"})
		return
	}

	table, business, err := loadPublicGuestTableContext(code)
	if err != nil {
		respondPublicGuestTableLookupError(c, err)
		return
	}

	// Match sibling guest routes: reject guest actions on a suspended or closed business.
	if !database.IsBusinessOperational(business) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "This business is not currently accepting orders",
			"code":  services.OrderErrCodeBusinessUnavailable,
		})
		return
	}

	var req createServiceCallRequest
	if err := c.ShouldBindJSON(&req); err != nil || !serviceCallReasons[req.Reason] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Reason must be one of: water, order, check"})
		return
	}

	svc := operational_alerts.NewService(database.GetDB())
	status, _, resolvedAt, err := svc.GetServiceCallStatus(c.Request.Context(), table.BusinessID, table.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check service call status"})
		return
	}

	switch status {
	case operational_alerts.ServiceCallStatusOpen, operational_alerts.ServiceCallStatusAcknowledged:
		// A call is already in flight. Upsert the new reason onto the same
		// row so staff see the latest ask (water → check) instead of dropping
		// the second chip. Still one alert — no duplicate stack.
		if err := svc.CreateServiceCallAlert(c.Request.Context(), table.BusinessID, table.ID, table.Name, req.Reason); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create service call"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": status, "reason": req.Reason})
		return
	case operational_alerts.ServiceCallStatusResolved:
		if resolvedAt != nil {
			if remaining := serviceCallCooldown - time.Since(*resolvedAt); remaining > 0 {
				retryAfter := int(remaining.Seconds()) + 1
				c.Header("Retry-After", strconv.Itoa(retryAfter))
				c.JSON(http.StatusTooManyRequests, gin.H{
					"error":               "A service call for this table was just resolved. Please wait a moment before calling again.",
					"retry_after_seconds": retryAfter,
				})
				return
			}
		}
	}

	if err := svc.CreateServiceCallAlert(c.Request.Context(), table.BusinessID, table.ID, table.Name, req.Reason); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create service call"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"status": operational_alerts.ServiceCallStatusOpen, "reason": req.Reason})
}

// GetServiceCallStatusByTableCode is the guest poll: the current service-call
// state for the table (none / open / acknowledged / resolved). Narrow
// projection all the way down — see TestServiceCallStatusQueryShape.
//
// GET /guest/table/:code/service-call
func GetServiceCallStatusByTableCode(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Table code is required"})
		return
	}

	table, _, err := loadPublicGuestTableContext(code)
	if err != nil {
		respondPublicGuestTableLookupError(c, err)
		return
	}

	status, reason, _, err := operational_alerts.NewService(database.GetDB()).
		GetServiceCallStatus(c.Request.Context(), table.BusinessID, table.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check service call status"})
		return
	}

	payload := gin.H{"status": status}
	if reason != "" {
		payload["reason"] = reason
	}
	c.JSON(http.StatusOK, payload)
}
