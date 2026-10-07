package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
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

// countingEmailProvider records how many messages were dispatched so tests can
// assert a verification email send was attempted.
type countingEmailProvider struct {
	mu    sync.Mutex
	count int
}

func (p *countingEmailProvider) Send(_ context.Context, _ emails.EmailMessage) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.count++
	return nil
}

func (p *countingEmailProvider) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.count
}

func newRegisterSessionHandler(t *testing.T) (*AuthHandler, *gorm.DB, *countingEmailProvider) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	prevSecret := structs.SecretKey
	t.Cleanup(func() { structs.SecretKey = prevSecret })
	structs.SecretKey = []byte("test-secret-key-for-register-session-tests")

	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&database.User{}, &UserAuth{}))

	provider := &countingEmailProvider{}
	es, err := emails.NewEmailServer(provider, "noreply@example.com", "updates@example.com", "../../email/templates")
	require.NoError(t, err)

	return NewAuthHandler(db, es), db, provider
}

// TestRegisterCreatesPendingIdentityWithoutSession pins the verified-email
// boundary: registration reserves the identity and sends verification, but
// cannot mint a normal operator credential.
func TestRegisterCreatesPendingIdentityWithoutSession(t *testing.T) {
	h, _, provider := newRegisterSessionHandler(t)

	w, c := postJSON(t, map[string]string{
		"email":    "founder@example.com",
		"password": "password123",
		"name":     "Founder",
	})
	h.Register(c)

	require.Equal(t, http.StatusCreated, w.Code, "register should return 201")

	var body AuthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	assert.True(t, body.Success)
	assert.Empty(t, body.Token, "register must not issue a normal operator token")
	assert.True(t, body.VerificationRequired)

	// A session cookie must be set, mirroring Login.
	cookies := w.Result().Cookies()
	var hasSession bool
	for _, ck := range cookies {
		if ck.Name == "session_token" && ck.Value != "" {
			hasSession = true
		}
	}
	assert.False(t, hasSession, "registration cannot set a normal session_token cookie")

	// A verification email send was attempted.
	assert.GreaterOrEqual(t, provider.Count(), 1, "verification email should be sent")

	// The user is NOT verified yet.
	var authRec UserAuth
	require.NoError(t, h.db.Where("provider = ? AND provider_user_id = ?", "email", "founder@example.com").First(&authRec).Error)
	assert.False(t, authRec.EmailVerified, "registration must not mark email verified")
}

// TestLoginStillRequiresVerifiedEmail is a regression guard: the existing 403
// path for unverified logins must remain unchanged after pending registration.
func TestLoginStillRequiresVerifiedEmail(t *testing.T) {
	h, _, _ := newRegisterSessionHandler(t)

	// Register leaves the account unverified and issues no session.
	wReg, cReg := postJSON(t, map[string]string{
		"email":    "pending@example.com",
		"password": "password123",
		"name":     "Pending",
	})
	h.Register(cReg)
	require.Equal(t, http.StatusCreated, wReg.Code)

	// A fresh login for the unverified account must still be rejected 403.
	wLogin, cLogin := postJSON(t, map[string]string{
		"email":    "pending@example.com",
		"password": "password123",
	})
	h.Login(cLogin)

	require.Equal(t, http.StatusForbidden, wLogin.Code, "unverified login must stay 403")
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(wLogin.Body.Bytes(), &body))
	assert.Equal(t, "AUTH_FORBIDDEN", body["code"])
	params, ok := body["params"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, params["requires_email_verification"])
	// Sanity: the message references verification.
	assert.True(t, strings.Contains(strings.ToLower(body["error"].(string)), "verif"))
}
