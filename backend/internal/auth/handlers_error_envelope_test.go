package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/structs"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type noopEmailProvider struct{}

func (noopEmailProvider) Send(_ context.Context, _ emails.EmailMessage) error { return nil }

func newAuthEnvelopeHandler(t *testing.T) (*AuthHandler, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	prevSecret := structs.SecretKey
	t.Cleanup(func() { structs.SecretKey = prevSecret })
	structs.SecretKey = []byte("test-secret-key-for-auth-envelope-tests")

	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&database.User{}, &UserAuth{}))

	es, err := emails.NewEmailServer(noopEmailProvider{}, "noreply@example.com", "updates@example.com", "../../email/templates")
	require.NoError(t, err)

	return NewAuthHandler(db, es), db
}

func postJSON(t *testing.T, body interface{}) (*httptest.ResponseRecorder, *gin.Context) {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	return w, c
}

func TestLoginEmailNotVerifiedReturnsParamsEnvelope(t *testing.T) {
	h, _ := newAuthEnvelopeHandler(t)

	// Register creates an unverified email auth record; persist it so Login finds it.
	authRecord, _, err := h.authService.RegisterWithEmail("unverified@example.com", "password123", "Pat")
	require.NoError(t, err)
	require.NoError(t, h.authService.CreateAuth(authRecord))

	w, c := postJSON(t, map[string]string{"email": "unverified@example.com", "password": "password123"})
	h.Login(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.NotEmpty(t, body["error"])
	assert.Equal(t, "AUTH_FORBIDDEN", body["code"])
	params, ok := body["params"].(map[string]interface{})
	require.True(t, ok, "verification fields must live under params")
	assert.Equal(t, true, params["requires_email_verification"])
}

func TestLoginInvalidCredentialsReturnsCodedEnvelope(t *testing.T) {
	h, _ := newAuthEnvelopeHandler(t)

	w, c := postJSON(t, map[string]string{"email": "nobody@example.com", "password": "whatever"})
	h.Login(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	// H1: bad email/password now carries the canonical AUTH_INVALID_CREDENTIALS
	// code (was AUTH_TOKEN_INVALID) so the FE localizes it distinctly.
	assert.Equal(t, "AUTH_INVALID_CREDENTIALS", body["code"])
}

func TestGoogleCallbackInvalidStateReturnsCodedEnvelope(t *testing.T) {
	h, _ := newAuthEnvelopeHandler(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/google/callback?code=x&state=bogus", nil)

	h.GoogleCallback(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "VALIDATION_INVALID_INPUT", body["code"])
}

func TestLinkWalletNotAuthenticatedReturnsCodedEnvelope(t *testing.T) {
	h, _ := newAuthEnvelopeHandler(t)
	w, c := postJSON(t, map[string]string{"address": "0xabc", "message": "m", "signature": "s"})
	// no user_id in context
	h.LinkWallet(c)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "AUTH_NOT_AUTHENTICATED", body["code"])
}

func TestVerifyEmailInvalidTokenReturnsCodedEnvelope(t *testing.T) {
	h, _ := newAuthEnvelopeHandler(t)
	w, c := postJSON(t, map[string]string{"token": "does-not-exist"})
	h.VerifyEmail(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "VALIDATION_INVALID_INPUT", body["code"])
}
