package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/session"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type operatorEmailAuthState struct {
	ID             uint `gorm:"primaryKey"`
	UserID         uint `gorm:"index"`
	Provider       string
	ProviderUserID string
	EmailVerified  bool
}

func (operatorEmailAuthState) TableName() string { return "user_auths" }

func seedOperatorEmailVerificationState(t *testing.T, db *gorm.DB, user *database.User, verified bool) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&operatorEmailAuthState{}))
	require.NoError(t, db.Model(&database.User{}).Where("id = ?", user.ID).Update("email_verified", verified).Error)
	result := db.Model(&operatorEmailAuthState{}).
		Where("user_id = ? AND provider = ?", user.ID, "email").
		Updates(map[string]interface{}{
			"provider_user_id": user.Email,
			"email_verified":   verified,
		})
	require.NoError(t, result.Error)
	if result.RowsAffected == 0 {
		require.NoError(t, db.Create(&operatorEmailAuthState{
			UserID: user.ID, Provider: "email", ProviderUserID: user.Email, EmailVerified: verified,
		}).Error)
	}
}

func seedOperatorAccessSession(t *testing.T, user *database.User, provider, accessSeed, refreshRaw string) (*session.UserSession, string) {
	t.Helper()
	uid := user.ID
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID: &uid, TokenHash: session.HashToken(accessSeed), Provider: provider, ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	token, err := GenerateUserToken(user.ID, user.Email, user.Address, user.Role, sess.ID)
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.UpdateTokenHash(sess.ID, session.HashToken(token)))
	if refreshRaw != "" {
		require.NoError(t, session.GlobalStore.RotateRefreshToken(sess.ID, session.HashToken(refreshRaw), time.Now().Add(7*24*time.Hour)))
	}
	return sess, token
}

func TestLegacyUnverifiedOperatorSessionLosesProtectedAccess(t *testing.T) {
	t.Setenv("EMAIL_VERIFICATION", "required")
	for _, tc := range []struct {
		name       string
		middleware gin.HandlerFunc
	}{
		{name: "operator", middleware: AuthenticationMiddleware()},
		{name: "hybrid", middleware: HybridAuthenticationMiddleware()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, user := setupRefreshHandlerTest(t)
			seedOperatorEmailVerificationState(t, db, user, false)
			sess, token := seedOperatorAccessSession(t, user, "email", "legacy-unverified-"+tc.name, "")

			router := gin.New()
			router.Use(tc.middleware)
			router.GET("/inside/probe", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			req := httptest.NewRequest(http.MethodGet, "/inside/probe", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
			var persisted session.UserSession
			require.NoError(t, db.First(&persisted, sess.ID).Error)
			assert.True(t, persisted.Revoked, "a legacy unverified full session must be terminally revoked")
			assert.Equal(t, "email_unverified", persisted.RevocationReason)
		})
	}
}

func TestRefreshRejectsLegacyUnverifiedOperatorSession(t *testing.T) {
	t.Setenv("EMAIL_VERIFICATION", "required")
	db, user := setupRefreshHandlerTest(t)
	seedOperatorEmailVerificationState(t, db, user, false)
	sess, _ := seedOperatorAccessSession(t, user, "email", "unverified-refresh-access", "unverified-refresh")

	w, newRefresh := callRefresh(t, "unverified-refresh")
	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
	assert.Empty(t, newRefresh)
	var persisted session.UserSession
	require.NoError(t, db.First(&persisted, sess.ID).Error)
	assert.True(t, persisted.Revoked)
	assert.Equal(t, "email_unverified", persisted.RevocationReason)
}

func TestVerifiedOperatorSessionStillAccessesAndRefreshes(t *testing.T) {
	t.Setenv("EMAIL_VERIFICATION", "required")
	db, user := setupRefreshHandlerTest(t)
	seedOperatorEmailVerificationState(t, db, user, true)
	_, token := seedOperatorAccessSession(t, user, "email", "verified-access", "verified-refresh")

	router := gin.New()
	router.Use(HybridAuthenticationMiddleware())
	router.GET("/inside/probe", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodGet, "/inside/probe", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	wAccess := httptest.NewRecorder()
	router.ServeHTTP(wAccess, req)
	assert.Equal(t, http.StatusNoContent, wAccess.Code, wAccess.Body.String())

	wRefresh, newRefresh := callRefresh(t, "verified-refresh")
	assert.Equal(t, http.StatusOK, wRefresh.Code, wRefresh.Body.String())
	assert.NotEmpty(t, newRefresh)
}

func probeOperatorSession(t *testing.T, token string) int {
	t.Helper()
	router := gin.New()
	router.Use(HybridAuthenticationMiddleware())
	router.GET("/inside/probe", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodGet, "/inside/probe", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w.Code
}

// With EMAIL_VERIFICATION=off the flags stay false (nothing proved the
// address), so the session boundary only checks that the email identity is
// still bound to the account's current address.
func TestVerificationOffKeepsABoundUnverifiedSession(t *testing.T) {
	t.Setenv("EMAIL_VERIFICATION", "off")
	db, user := setupRefreshHandlerTest(t)
	seedOperatorEmailVerificationState(t, db, user, false)
	sess, token := seedOperatorAccessSession(t, user, "email", "off-bound-access", "off-bound-refresh")

	assert.Equal(t, http.StatusNoContent, probeOperatorSession(t, token))
	wRefresh, newRefresh := callRefresh(t, "off-bound-refresh")
	assert.Equal(t, http.StatusOK, wRefresh.Code, wRefresh.Body.String())
	assert.NotEmpty(t, newRefresh)

	var persisted session.UserSession
	require.NoError(t, db.First(&persisted, sess.ID).Error)
	assert.False(t, persisted.Revoked)
}

func TestVerificationOffRevokesASessionWhoseEmailIdentityIsStale(t *testing.T) {
	t.Setenv("EMAIL_VERIFICATION", "off")
	db, user := setupRefreshHandlerTest(t)
	seedOperatorEmailVerificationState(t, db, user, false)
	sess, token := seedOperatorAccessSession(t, user, "email", "off-stale-access", "")
	require.NoError(t, db.Model(&database.User{}).Where("id = ?", user.ID).Update("email", "moved-"+user.Email).Error)

	assert.Equal(t, http.StatusForbidden, probeOperatorSession(t, token))
	var persisted session.UserSession
	require.NoError(t, db.First(&persisted, sess.ID).Error)
	assert.True(t, persisted.Revoked)
	assert.Equal(t, "email_unverified", persisted.RevocationReason)
}

// Accounts created while verification was off are not grandfathered: turning
// verification back on revokes their sessions until the address is proven.
func TestSwitchingVerificationOnRevokesOffModeSessions(t *testing.T) {
	t.Setenv("EMAIL_VERIFICATION", "off")
	db, user := setupRefreshHandlerTest(t)
	seedOperatorEmailVerificationState(t, db, user, false)
	sess, token := seedOperatorAccessSession(t, user, "email", "off-then-on-access", "")
	assert.Equal(t, http.StatusNoContent, probeOperatorSession(t, token))

	t.Setenv("EMAIL_VERIFICATION", "required")
	assert.Equal(t, http.StatusForbidden, probeOperatorSession(t, token))
	var persisted session.UserSession
	require.NoError(t, db.First(&persisted, sess.ID).Error)
	assert.True(t, persisted.Revoked)
}
