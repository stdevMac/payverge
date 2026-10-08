package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/session"
)

// useFailingSessionStore installs a session store whose database is closed
// after a row is written, so the next revoke hits a dead connection. Mirrors
// TestSignOut_PartialFailureUsesCanonicalErrorEnvelope.
func useFailingSessionStore(t *testing.T, in session.CreateInput) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gormDB.AutoMigrate(&session.UserSession{}))

	previousStore := session.GlobalStore
	store := session.NewStore(gormDB)
	session.GlobalStore = store
	t.Cleanup(func() { session.GlobalStore = previousStore })

	_, err = store.Create(in)
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
}

func TestLogout_PartialFailureUsesCanonicalErrorEnvelope(t *testing.T) {
	h, _ := newAuthEnvelopeHandler(t)
	rawToken := "raw-logout-token"
	useFailingSessionStore(t, session.CreateInput{
		TokenHash: session.HashToken(rawToken),
		Provider:  "email",
		ExpiresAt: time.Now().Add(time.Hour),
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/logout", nil)
	c.Request.AddCookie(&http.Cookie{Name: "session_token", Value: rawToken})
	h.Logout(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "sign_out_partial")
	assert.NotEmpty(t, w.Header().Values("Set-Cookie"), "local cookies must still be cleared")
}

func TestResetPassword_RevokeFailureReturnsSignOutPartial(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)

	user := &database.User{Email: "reset-revoke-fail@example.com"}
	require.NoError(t, db.Create(user).Error)
	expiry := time.Now().Add(time.Hour)
	passwordHash, err := HashPassword("oldpassword123")
	require.NoError(t, err)
	require.NoError(t, db.Create(&UserAuth{
		UserID:       user.ID,
		Provider:     "email",
		PasswordHash: passwordHash,
		ResetToken:   HashToken("revoke-fail-reset-token"),
		ResetExpiry:  &expiry,
	}).Error)

	// Session rows live in a different database from the reset token, so closing
	// it fails revocation only. The password change itself still commits.
	useFailingSessionStore(t, session.CreateInput{
		UserID:    &user.ID,
		TokenHash: session.HashToken("reset-revoke-fail-access"),
		Provider:  "email",
		ExpiresAt: time.Now().Add(time.Hour),
	})

	w, c := postJSON(t, map[string]string{
		"token":        "revoke-fail-reset-token",
		"new_password": "newpassword123",
	})
	h.ResetPassword(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "sign_out_partial")

	var reloaded UserAuth
	require.NoError(t, db.Where("user_id = ?", user.ID).First(&reloaded).Error)
	assert.Empty(t, reloaded.ResetToken, "password reset must commit before the revoke failure")
	assert.True(t, CheckPassword("newpassword123", reloaded.PasswordHash))
}
