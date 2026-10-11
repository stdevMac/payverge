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

func TestLogout_RecordsUserLogoutReason(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)
	require.NoError(t, db.AutoMigrate(&session.UserSession{}))

	previousStore := session.GlobalStore
	session.GlobalStore = session.NewStore(db)
	t.Cleanup(func() { session.GlobalStore = previousStore })

	rawToken := "email-logout-token"
	sess, err := session.GlobalStore.Create(session.CreateInput{
		TokenHash: session.HashToken(rawToken),
		Provider:  "email",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/logout", nil)
	c.Request.AddCookie(&http.Cookie{Name: "session_token", Value: rawToken})
	h.Logout(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var persisted session.UserSession
	require.NoError(t, db.First(&persisted, sess.ID).Error)
	assert.True(t, persisted.Revoked)
	assert.Equal(t, string(session.RevocationReasonUserLogout), persisted.RevocationReason)
}
