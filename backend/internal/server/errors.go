package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/services"
)

// Auth error codes.
//
// These string literals are the wire contract with the frontend: WS-F1's
// getLocalizedApiError reads `response.data.code` and looks it up in
// apiErrors.json (en/es/es-AR). The literals here MUST stay byte-for-byte equal
// to the JSON keys, or localization silently falls back to the raw English
// `error` string. TestCanonicalErrorCodeVocabulary pins them.
const (
	ErrCodeTokenMissing = "AUTH_TOKEN_MISSING"
	ErrCodeTokenExpired = "AUTH_TOKEN_EXPIRED"
	ErrCodeTokenInvalid = "AUTH_TOKEN_INVALID"
	// ErrCodeRefreshRotated is returned when a refresh request lost a concurrent
	// rotation race (another in-flight refresh already advanced the one-time
	// refresh token). The session is still alive — the FE must NOT clear its
	// session hint or force logout on this code.
	ErrCodeRefreshRotated = "AUTH_REFRESH_ROTATED"
	// ErrCodeSessionUnknown is the canonical code for a revoked/unknown session
	// (renamed from the legacy "AUTH_SESSION_REVOKED" so it matches the FE
	// apiErrors.json key). The guest AI-waiter path also emits this code; the FE
	// isSessionUnknownError() branches on it to silently re-create the session.
	ErrCodeSessionUnknown     = "AUTH_SESSION_UNKNOWN"
	ErrCodeForbidden          = "AUTH_FORBIDDEN"
	ErrCodeNotAdmin           = "AUTH_NOT_ADMIN"
	ErrCodeInsufficientRole   = "AUTH_INSUFFICIENT_ROLE"
	ErrCodeInvalidCredentials = "AUTH_INVALID_CREDENTIALS"
	ErrCodeNotBusinessOwner   = "BIZ_NOT_OWNER"
	ErrCodeStaffNoAccess      = "BIZ_STAFF_NO_ACCESS"
	ErrCodeBusinessNotFound   = "BIZ_NOT_FOUND"
	ErrCodeDuplicateEmail     = "VALIDATION_DUPLICATE_EMAIL"
	ErrCodeWeakPassword       = "VALIDATION_WEAK_PASSWORD"
	ErrCodeInvalidInput       = "VALIDATION_INVALID_INPUT"
	// ErrCodeFieldInvalid pairs with a {"field": "<name>"} params bag — emit via
	// RespondWithFieldError so the FE can interpolate the localized message.
	ErrCodeFieldInvalid = "VALIDATION_FIELD_INVALID"
	// ErrCodeInvalidDateRange marks a reporting window the server cannot honor:
	// an inverted range, half a range, an unparseable date, or an unknown period
	// preset. Split out of VALIDATION_INVALID_INPUT (#930) because these 400s are
	// the ones operators actually hit on money screens, and a generic
	// "invalid input" code left them stranded on the raw English `error` string.
	// Operator-tier: localized in en/es/es-AR apiErrors.json only.
	ErrCodeInvalidDateRange = "VALIDATION_INVALID_DATE_RANGE"
	ErrCodeUserNotFound     = "USER_NOT_FOUND"
	ErrCodeRateLimited      = "RATE_LIMITED"
	ErrCodePaymentFailed    = "PAYMENT_FAILED"
	ErrCodePaymentDeclined  = "PAYMENT_DECLINED"
	ErrCodeConflict         = "CONFLICT"
	ErrCodeNotFound         = "NOT_FOUND"
	ErrCodeServerError      = "SERVER_ERROR"

	// Operator inventory / delivery validation codes (Batch E — FE apiErrors.json).
	ErrCodeInventoryInsufficientStock       = "inventory_insufficient_stock"
	ErrCodeDeliveryFeeNegative              = "delivery_fee_negative"
	ErrCodeDeliveryZoneInvalid              = "delivery_zone_invalid"
	ErrCodeDeliveryPaymentModeInvalid       = "delivery_payment_mode_invalid"
	ErrCodeDeliveryPartnerLinksInvalid      = "delivery_partner_links_invalid"
	ErrCodeDeliveryZoneNameRequired         = "delivery_zone_name_required"
	ErrCodeDeliveryRequiresZoneOrPartner    = "delivery_requires_zone_or_partner"
	ErrCodeDeliveryZonePriorityInvalid      = "delivery_zone_priority_invalid"
	ErrCodeDeliveryZonePriorityDuplicate    = "delivery_zone_priority_duplicate"
	ErrCodeDeliveryZoneEstimatedTimeInvalid = "delivery_zone_estimated_time_invalid"
	ErrCodeDeliveryHoursInvalid             = "delivery_hours_invalid"
	ErrCodeDeliveryTipNegative              = "delivery_tip_negative"

	// Operator bill-lifecycle code. Localized operator-tier under
	// messages/*/billManager.json (voidRefund.errors.kitchenTicketsLive),
	// not in the guest apiErrors.json tree — voiding is manager-PIN only.
	ErrCodeBillVoidKitchenTicketsLive = "bill_void_kitchen_tickets_live"

	// Operator fiscal code: the bill already carries a live fiscal receipt, so a
	// second issue would be a duplicate factura. Localized operator-tier under
	// messages/*/fiscal.json and accounting.json — the fiscal console is en/es/
	// es-AR only, so this never belongs in the 21-locale guest apiErrors tree.
	ErrCodeFiscalReceiptAlreadyIssued = "fiscal_receipt_already_issued"

	ErrCodeInternal           = "INTERNAL"
	ErrCodeNotAuthenticated   = "AUTH_NOT_AUTHENTICATED"
	ErrCodeServiceUnavailable = "SERVICE_UNAVAILABLE"

	// Guest loyalty redeem/undo codes. These are toasted via apiErrors.json on
	// the 21-locale guest tier — do not collapse them onto VALIDATION_INVALID_INPUT.
	ErrCodeInsufficientPoints       = "insufficient_points"
	ErrCodeNoActiveBill             = "no_active_bill"
	ErrCodeNoLoyaltyConnection      = "no_loyalty_connection"
	ErrCodeVisitNotRecorded         = "visit_not_recorded"
	ErrCodeRequestIDRequired        = "request_id_required"
	ErrCodeLoyaltyAlreadyApplied    = "loyalty_discount_already_applied"
	ErrCodeLoyaltyUndoNotOwner      = "loyalty_undo_not_owner"
	ErrCodeLoyaltyNotEnabled        = "loyalty_not_enabled"
	ErrCodeLoyaltyRateNotConfigured = "loyalty_rate_not_configured"
	ErrCodeLoyaltyPointsTooSmall    = "loyalty_points_too_small"
	ErrCodeNoLoyaltyDiscount        = "no_loyalty_discount"
)

// ErrorResponse is the structured error response returned by API endpoints.
type ErrorResponse struct {
	Error  string                 `json:"error"`
	Code   string                 `json:"code,omitempty"`
	Params map[string]interface{} `json:"params,omitempty"`
}

// RespondWithError writes a JSON error response with an optional error code.
func RespondWithError(c *gin.Context, status int, code string, message string) {
	c.JSON(status, ErrorResponse{Error: message, Code: code})
}

// defaultBindErrorMessage is the product-safe copy for failed ShouldBindJSON /
// ShouldBindQuery calls. Gin validator dumps look like
// `Key: 'InviteStaffRequest.Email' Error:Field validation...` and must never
// reach operators or guests (FIND-030/031/032).
const defaultBindErrorMessage = "Please check the form and try again."

// IsRawValidatorDump reports gin/validator binding dumps that are not product copy.
func IsRawValidatorDump(detail string) bool {
	s := strings.TrimSpace(detail)
	if s == "" {
		return false
	}
	return strings.Contains(s, "Key: '") ||
		strings.Contains(s, "Error:Field validation") ||
		strings.Contains(s, "failed on the '") ||
		strings.Contains(s, `binding:"`)
}

// PublicBindErrorMessage returns a client-safe message for a bind failure.
// Raw gin dumps and JSON type-error internals always collapse to the default.
func PublicBindErrorMessage(err error) string {
	if err == nil {
		return defaultBindErrorMessage
	}
	s := err.Error()
	if IsRawValidatorDump(s) {
		return defaultBindErrorMessage
	}
	// json.SyntaxError / unmarshal type errors still leak Go type names or
	// parser internals (unexpected EOF on truncated bodies).
	lower := strings.ToLower(s)
	if strings.Contains(s, "json:") ||
		strings.Contains(s, "cannot unmarshal") ||
		strings.Contains(s, "invalid character") ||
		strings.Contains(lower, "unexpected eof") ||
		strings.Contains(lower, "unexpected end of json") {
		return defaultBindErrorMessage
	}
	if len(s) == 0 || len(s) > 200 {
		return defaultBindErrorMessage
	}
	return s
}

// RespondBindError writes VALIDATION_INVALID_INPUT with product-safe copy.
// Prefer this over `c.JSON(400, gin.H{"error": err.Error()})` after ShouldBind*.
func RespondBindError(c *gin.Context, err error) {
	RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, PublicBindErrorMessage(err))
}

// RespondWithErrorParams writes a JSON error response with interpolation params.
func RespondWithErrorParams(c *gin.Context, status int, code string, message string, params map[string]interface{}) {
	c.JSON(status, ErrorResponse{Error: message, Code: code, Params: params})
}

// RespondWithOrderabilityConflict preserves the stable operator inventory code
// while returning the per-item decisions needed by clients to refresh stale
// carts and explain which physical item (including a bundle child) is blocked.
func RespondWithOrderabilityConflict(c *gin.Context, unavailable *services.ItemNotOrderableError) {
	c.JSON(http.StatusConflict, gin.H{
		"code":  ErrCodeInventoryInsufficientStock,
		"error": unavailable.Error(),
		"details": gin.H{
			"items": unavailable.Items,
		},
	})
}

// RespondWithFieldError writes a VALIDATION_FIELD_INVALID envelope carrying a
// {"field": "<name>"} params bag so the frontend can render a localized,
// field-specific message via apiErrors.json interpolation.
func RespondWithFieldError(c *gin.Context, status int, field string, message string) {
	c.JSON(status, ErrorResponse{
		Error:  message,
		Code:   ErrCodeFieldInvalid,
		Params: map[string]interface{}{"field": field},
	})
}

// MapDeliveryActionError maps a delivery accept/reject service error to a safe
// (status, code, client-message) triple. User-meaningful sentinels surface a
// helpful message; everything else (raw DB/gorm errors) collapses to a single
// generic message so internal table/column/SQL detail never reaches the client.
// Callers must log the underlying err server-side before responding.
//
// This is the single safe-message chokepoint shared by both delivery-cancel
// routes: handlers.UpdateOrderStatus (PUT /status delivery branch) and
// server.CancelOrder (PATCH .../cancel).
func MapDeliveryActionError(err error) (int, string, string) {
	switch {
	case errors.Is(err, services.ErrDeliveryOrderNotFound):
		return http.StatusNotFound, ErrCodeNotFound, "Delivery order not found"
	case errors.Is(err, services.ErrDeliveryInvalidState):
		return http.StatusBadRequest, ErrCodeInvalidInput, "Delivery is not in a state that allows this action"
	default:
		return http.StatusInternalServerError, ErrCodeInternal, "Could not process the delivery action"
	}
}
