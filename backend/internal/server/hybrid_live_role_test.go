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
)

// seedHybridRoleSession creates a live session for user and returns a bearer
// token whose role claim is claimRole (which may differ from users.role to
// model a token minted before a demotion).
func seedHybridRoleSession(t testing.TB, user *database.User, claimRole, seed string) (*session.UserSession, string) {
	t.Helper()
	uid := user.ID
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID: &uid, TokenHash: session.HashToken(seed), Provider: "email", ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	token, err := GenerateUserToken(user.ID, user.Email, user.Address, claimRole, sess.ID)
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.UpdateTokenHash(sess.ID, session.HashToken(token)))
	return sess, token
}

func hybridRoleProbe(t testing.TB, token string) (*httptest.ResponseRecorder, interface{}) {
	t.Helper()
	var seenRole interface{}
	router := gin.New()
	router.Use(HybridAuthenticationMiddleware())
	router.GET("/inside/probe", func(c *gin.Context) {
		seenRole, _ = c.Get("role")
		c.Status(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/inside/probe", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w, seenRole
}

// M-role: a JWT minted while the user was an admin must not keep platform
// admin powers on /inside after users.role is demoted, and every session of
// the demoted user must be revoked so refresh cannot keep the stale token
// family alive.
func TestHybridAuth_DemotedAdminClaimIsNotTrusted(t *testing.T) {
	db, user := setupRefreshHandlerTest(t)
	sess, token := seedHybridRoleSession(t, user, "admin", "demoted-admin-access")
	otherSess, _ := seedHybridRoleSession(t, user, "admin", "demoted-admin-other-device")

	// users.role is "user" (setupRefreshHandlerTest) while the claim says admin.
	w, role := hybridRoleProbe(t, token)
	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
	assert.NotEqual(t, "admin", role, "stale admin claim must never reach handlers")

	for _, id := range []uint{sess.ID, otherSess.ID} {
		var persisted session.UserSession
		require.NoError(t, db.First(&persisted, id).Error)
		assert.True(t, persisted.Revoked, "session %d of a demoted admin must be revoked", id)
		assert.Equal(t, string(session.RevocationReasonRoleChanged), persisted.RevocationReason)
	}

	// The revoked token stays dead on the next request.
	w2, _ := hybridRoleProbe(t, token)
	assert.Equal(t, http.StatusUnauthorized, w2.Code)
}

func TestHybridAuth_LiveAdminKeepsRole(t *testing.T) {
	db, user := setupRefreshHandlerTest(t)
	require.NoError(t, db.Model(&database.User{}).Where("id = ?", user.ID).Update("role", "admin").Error)
	_, token := seedHybridRoleSession(t, user, "admin", "live-admin-access")

	w, role := hybridRoleProbe(t, token)
	assert.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	assert.Equal(t, "admin", role)
}

func TestHybridAuth_NonAdminClaimNeedsNoLookup(t *testing.T) {
	_, user := setupRefreshHandlerTest(t)
	_, token := seedHybridRoleSession(t, user, "user", "plain-user-access")

	w, role := hybridRoleProbe(t, token)
	assert.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
	assert.Equal(t, "user", role)
}

// The session-cookie path (no bearer) goes through applyHybridUserOrWeb3Context
// and must apply the same live-role rule.
func TestHybridAuth_SessionCookieDemotedAdminIsRevoked(t *testing.T) {
	db, user := setupRefreshHandlerTest(t)
	sess, token := seedHybridRoleSession(t, user, "admin", "demoted-admin-cookie")

	router := gin.New()
	router.Use(HybridAuthenticationMiddleware())
	var seenRole interface{}
	router.GET("/inside/probe", func(c *gin.Context) {
		seenRole, _ = c.Get("role")
		c.Status(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, "/inside/probe", nil)
	req.AddCookie(&http.Cookie{Name: "session_token", Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
	assert.NotEqual(t, "admin", seenRole)
	var persisted session.UserSession
	require.NoError(t, db.First(&persisted, sess.ID).Error)
	assert.True(t, persisted.Revoked)
}

// Refresh must mint the live users.role, never the role of the token being
// refreshed.
func TestRefreshToken_MintsLiveRoleAfterDemotion(t *testing.T) {
	db, user := setupRefreshHandlerTest(t)
	require.NoError(t, db.Model(&database.User{}).Where("id = ?", user.ID).Update("role", "admin").Error)
	seedRefreshSession(t, user.ID, "demotion-refresh")
	require.NoError(t, db.Model(&database.User{}).Where("id = ?", user.ID).Update("role", "user").Error)

	w, _ := callRefresh(t, "demotion-refresh")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var minted string
	for _, ck := range w.Result().Cookies() {
		if ck.Name == "session_token" && ck.Value != "" {
			minted = ck.Value
		}
	}
	require.NotEmpty(t, minted, "refresh must set a new session_token cookie")
	claims, err := VerifyToken(minted)
	require.NoError(t, err)
	assert.Equal(t, "user", claims["role"], "refresh must not re-mint a stale admin role")
}

// BenchmarkHybridAuthUserToken measures HybridAuthenticationMiddleware for a
// bearer user token. role=user takes no live-role lookup; role=admin pays one
// narrow users.role read (M-role).
func BenchmarkHybridAuthUserToken(b *testing.B) {
	for _, tc := range []struct {
		name string
		role string
	}{{"role=user", "user"}, {"role=admin", "admin"}} {
		b.Run(tc.name, func(b *testing.B) {
			db, user := setupRefreshHandlerTest(b)
			require.NoError(b, db.Model(&database.User{}).Where("id = ?", user.ID).Update("role", tc.role).Error)
			_, token := seedHybridRoleSession(b, user, tc.role, "bench-"+tc.role)
			gin.SetMode(gin.ReleaseMode)
			router := gin.New()
			router.Use(HybridAuthenticationMiddleware())
			router.GET("/inside/probe", func(c *gin.Context) { c.Status(http.StatusNoContent) })
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				req := httptest.NewRequest(http.MethodGet, "/inside/probe", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				if w.Code != http.StatusNoContent {
					b.Fatalf("got %d: %s", w.Code, w.Body.String())
				}
			}
		})
	}
}
