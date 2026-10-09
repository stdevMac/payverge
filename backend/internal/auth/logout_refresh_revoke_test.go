package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/session"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SEC-6 / #303: logout with only refresh_token present must revoke the session
// so /auth/refresh cannot mint a new access token.
func TestLogout_RevokesViaRefreshTokenAlone(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)
	require.NoError(t, db.AutoMigrate(&session.UserSession{}))

	previousStore := session.GlobalStore
	session.GlobalStore = session.NewStore(db)
	t.Cleanup(func() { session.GlobalStore = previousStore })

	refreshRaw := "refresh-only-logout-token"
	uid := uint(42)
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &uid,
		TokenHash: session.HashToken("already-expired-access"),
		Provider:  "email",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.RotateRefreshToken(
		sess.ID,
		session.HashToken(refreshRaw),
		time.Now().Add(7*24*time.Hour),
	))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/logout", nil)
	// Intentionally omit session_token / Authorization — only refresh remains.
	c.Request.AddCookie(&http.Cookie{Name: "refresh_token", Value: refreshRaw})
	h.Logout(c)

	assert.Equal(t, http.StatusOK, w.Code)
	_, err = session.GlobalStore.ValidateRefreshToken(session.HashToken(refreshRaw))
	require.Error(t, err, "refresh token must be unusable after logout")

	var persisted session.UserSession
	require.NoError(t, db.First(&persisted, sess.ID).Error)
	assert.True(t, persisted.Revoked)
	assert.Equal(t, string(session.RevocationReasonUserLogout), persisted.RevocationReason)
}
