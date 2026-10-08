package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestNewErrorCodeConstants(t *testing.T) {
	assert.Equal(t, "INTERNAL", ErrCodeInternal)
	assert.Equal(t, "AUTH_NOT_AUTHENTICATED", ErrCodeNotAuthenticated)
	assert.Equal(t, "SERVICE_UNAVAILABLE", ErrCodeServiceUnavailable)
}

// TestCanonicalErrorCodeVocabulary pins the H1 codes to the exact UPPER_SNAKE
// strings the frontend localizes via apiErrors.json (WS-F1). A drift here
// silently breaks localization (translateApiError falls back to the raw English
// `error` string), so these literals are part of the wire contract.
func TestCanonicalErrorCodeVocabulary(t *testing.T) {
	cases := map[string]string{
		"AUTH_TOKEN_MISSING":               ErrCodeTokenMissing,
		"AUTH_TOKEN_EXPIRED":               ErrCodeTokenExpired,
		"AUTH_TOKEN_INVALID":               ErrCodeTokenInvalid,
		"AUTH_REFRESH_ROTATED":             ErrCodeRefreshRotated,
		"AUTH_SESSION_UNKNOWN":             ErrCodeSessionUnknown,
		"AUTH_FORBIDDEN":                   ErrCodeForbidden,
		"AUTH_NOT_ADMIN":                   ErrCodeNotAdmin,
		"AUTH_INSUFFICIENT_ROLE":           ErrCodeInsufficientRole,
		"AUTH_INVALID_CREDENTIALS":         ErrCodeInvalidCredentials,
		"BIZ_NOT_OWNER":                    ErrCodeNotBusinessOwner,
		"BIZ_STAFF_NO_ACCESS":              ErrCodeStaffNoAccess,
		"BIZ_NOT_FOUND":                    ErrCodeBusinessNotFound,
		"USER_NOT_FOUND":                   ErrCodeUserNotFound,
		"NOT_FOUND":                        ErrCodeNotFound,
		"VALIDATION_DUPLICATE_EMAIL":       ErrCodeDuplicateEmail,
		"VALIDATION_WEAK_PASSWORD":         ErrCodeWeakPassword,
		"VALIDATION_INVALID_INPUT":         ErrCodeInvalidInput,
		"VALIDATION_FIELD_INVALID":         ErrCodeFieldInvalid,
		"VALIDATION_INVALID_DATE_RANGE":    ErrCodeInvalidDateRange,
		"RATE_LIMITED":                     ErrCodeRateLimited,
		"PAYMENT_FAILED":                   ErrCodePaymentFailed,
		"PAYMENT_DECLINED":                 ErrCodePaymentDeclined,
		"CONFLICT":                         ErrCodeConflict,
		"SERVER_ERROR":                     ErrCodeServerError,
		"INTERNAL":                         ErrCodeInternal,
		"AUTH_NOT_AUTHENTICATED":           ErrCodeNotAuthenticated,
		"SERVICE_UNAVAILABLE":              ErrCodeServiceUnavailable,
		"insufficient_points":              ErrCodeInsufficientPoints,
		"no_active_bill":                   ErrCodeNoActiveBill,
		"no_loyalty_connection":            ErrCodeNoLoyaltyConnection,
		"visit_not_recorded":               ErrCodeVisitNotRecorded,
		"request_id_required":              ErrCodeRequestIDRequired,
		"loyalty_discount_already_applied": ErrCodeLoyaltyAlreadyApplied,
		"loyalty_undo_not_owner":           ErrCodeLoyaltyUndoNotOwner,
		"loyalty_not_enabled":              ErrCodeLoyaltyNotEnabled,
		"loyalty_rate_not_configured":      ErrCodeLoyaltyRateNotConfigured,
		"loyalty_points_too_small":         ErrCodeLoyaltyPointsTooSmall,
		"no_loyalty_discount":              ErrCodeNoLoyaltyDiscount,
	}
	for want, got := range cases {
		assert.Equal(t, want, got, "canonical code constant drifted from the FE vocabulary")
	}
}

// TestSessionUnknownCodeRenamed guards the rename of the legacy
// AUTH_SESSION_REVOKED literal to the canonical AUTH_SESSION_UNKNOWN.
func TestSessionUnknownCodeRenamed(t *testing.T) {
	assert.Equal(t, "AUTH_SESSION_UNKNOWN", ErrCodeSessionUnknown)
}

// TestRespondWithFieldErrorIncludesFieldParam verifies the named-field helper
// emits VALIDATION_FIELD_INVALID with a {"field": "<name>"} params bag so the
// FE can interpolate the localized message.
func TestRespondWithFieldErrorIncludesFieldParam(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	RespondWithFieldError(c, http.StatusBadRequest, "email", "Email is invalid")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "Email is invalid", body["error"])
	assert.Equal(t, "VALIDATION_FIELD_INVALID", body["code"])
	params, ok := body["params"].(map[string]interface{})
	assert.True(t, ok, "params must serialize under the \"params\" key")
	assert.Equal(t, "email", params["field"])
}

func TestRespondWithErrorParamsSerializesParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	RespondWithErrorParams(c, http.StatusForbidden, ErrCodeForbidden, "nope", map[string]interface{}{
		"requires_email_verification": true,
	})

	assert.Equal(t, http.StatusForbidden, w.Code)
	var body map[string]interface{}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "nope", body["error"])
	assert.Equal(t, "AUTH_FORBIDDEN", body["code"])
	params, ok := body["params"].(map[string]interface{})
	assert.True(t, ok, "params must serialize under the \"params\" key")
	assert.Equal(t, true, params["requires_email_verification"])
}
