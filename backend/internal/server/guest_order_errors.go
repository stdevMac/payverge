package server

import (
	"errors"
	"net/http"

	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

// respondGuestOrderValidationError maps pricing/validation failures from the
// promotion engine onto the structured guest error contract: existing JSON
// error body + machine-readable "code" the guest FE translates (Stage 3 G-1).
// Untyped errors keep the legacy {"error": ...} shape with no code.
func respondGuestOrderValidationError(c *gin.Context, err error) {
	var ve *services.OrderValidationError
	if errors.As(err, &ve) {
		// Closed Mode / ordering toggles are conflicts with the dining room state
		// (not form validation) — 409 matches guest FE handling for paused orders.
		status := http.StatusBadRequest
		if ve.Code == services.OrderErrCodeBusinessClosed ||
			ve.Code == services.OrderErrCodeOrderingDisabled ||
			ve.Code == services.OrderErrCodeBillNotOpen {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": ve.Message, "code": ve.Code})
		return
	}
	// Never surface raw internals to guests.
	c.JSON(http.StatusBadRequest, gin.H{
		"error": "Please check the form and try again.",
		"code":  ErrCodeInvalidInput,
	})
}
