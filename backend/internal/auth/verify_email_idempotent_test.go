package auth

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Verification links are single-use credentials. A replay must fail with the
// same generic response as an unknown token and cannot mint another session.
func TestVerifyEmailServiceRejectsConsumedToken(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)
	svc := h.authService
	user := database.User{Email: "idem@example.com", Name: "Idem", Role: "user", AuthMethod: "email"}
	require.NoError(t, db.Create(&user).Error)

	authRecord, plaintext, err := svc.RegisterWithEmail(user.Email, "password123", user.Name)
	require.NoError(t, err)
	authRecord.UserID = user.ID
	require.NoError(t, svc.CreateAuth(authRecord))

	// First use verifies.
	require.NoError(t, verifyEmailToken(svc, plaintext))

	// Second use is indistinguishable from an unknown token.
	err = verifyEmailToken(svc, plaintext)
	assert.ErrorIs(t, err, ErrTokenInvalid)
}

func TestVerifyEmailHandlerRejectsConsumedToken(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)
	user := database.User{Email: "twice@example.com", Name: "Tw", Role: "user", AuthMethod: "email"}
	require.NoError(t, db.Create(&user).Error)

	authRecord, plaintext, err := h.authService.RegisterWithEmail(user.Email, "password123", user.Name)
	require.NoError(t, err)
	authRecord.UserID = user.ID
	require.NoError(t, h.authService.CreateAuth(authRecord))

	w1, c1 := postJSON(t, map[string]string{"token": plaintext})
	h.VerifyEmail(c1)
	require.Equal(t, http.StatusOK, w1.Code)

	// Second click: generic invalid-token response, with no verification oracle.
	w2, c2 := postJSON(t, map[string]string{"token": plaintext})
	h.VerifyEmail(c2)
	require.Equal(t, http.StatusBadRequest, w2.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &body))
	assert.Equal(t, "VALIDATION_INVALID_INPUT", body["code"])
	assert.NotContains(t, body, "already_verified")
}

// Anti-enumeration lock: an UNKNOWN token's response must be byte-for-byte the
// same generic envelope as before this change (see also
// TestVerifyEmailInvalidTokenReturnsCodedEnvelope).
func TestVerifyEmailUnknownTokenStaysGeneric(t *testing.T) {
	h, _ := newAuthEnvelopeHandler(t)
	w, c := postJSON(t, map[string]string{"token": "never-issued"})
	h.VerifyEmail(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "VALIDATION_INVALID_INPUT", body["code"])
	assert.NotContains(t, body, "already_verified")
}
