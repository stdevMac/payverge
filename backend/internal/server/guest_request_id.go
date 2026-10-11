package server

import (
	"fmt"
	"net/http"
	"strings"

	requestmiddleware "github.com/stdevmac/payverge/backend/internal/middleware"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

// ErrCodeRequestIDInvalid is returned when a guest order's X-Request-Id is
// present but unusable (longer than services.MaxGuestCheckoutRequestIDLen).
// The guest web client always sends a UUID, so this is an API-client error
// and is not localized in apiErrors.json.
const ErrCodeRequestIDInvalid = "request_id_invalid"

// Machine-readable "error" values for the guest order request-id contract
// (docs/api/guest-orders.md). "code" carries the stable vocabulary the guest
// frontend localizes; "message" is the English explanation for API clients.
const (
	guestOrderErrMissingRequestID = "missing_request_id"
	guestOrderErrInvalidRequestID = "invalid_request_id"
)

// guestOrderRequestID reads the client-supplied X-Request-Id that
// POST /guest/table/:code/order uses as its idempotency key. A retry with the
// same id replays the original order instead of creating a second one, so the
// id must come from the client: the middleware-generated trace id cannot be
// used. On a missing or over-long id it writes a 400 and returns false.
func guestOrderRequestID(c *gin.Context) (string, bool) {
	requestID := strings.TrimSpace(c.GetHeader(requestmiddleware.RequestIDHeader))
	switch {
	case requestID == "":
		c.JSON(http.StatusBadRequest, gin.H{
			"error": guestOrderErrMissingRequestID,
			"code":  ErrCodeRequestIDRequired,
			"message": "The X-Request-Id header is required. Send a new unique id " +
				"(a UUID is recommended) for each order and reuse it when retrying " +
				"that order, so the order is created at most once.",
		})
		return "", false
	case len(requestID) > services.MaxGuestCheckoutRequestIDLen:
		c.JSON(http.StatusBadRequest, gin.H{
			"error": guestOrderErrInvalidRequestID,
			"code":  ErrCodeRequestIDInvalid,
			"message": fmt.Sprintf("The X-Request-Id header must be at most %d bytes "+
				"(a UUID is 36). Send a shorter unique id for each order.",
				services.MaxGuestCheckoutRequestIDLen),
		})
		return "", false
	}
	return requestID, true
}
