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

func TestHybridAuthentication_FallsThroughRevokedStaffCookieToSessionCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.User{}))

	business := createOwnedBusiness(t, "0xOwnerA", "Hybrid Fallback Biz")
	staff := createStaffMember(t, business.ID, "stale-hybrid@example.com", "Stale Hybrid")
	staffToken := generateStaffTokenWithSession(t, staff)
	require.NoError(t, session.GlobalStore.RevokeByTokenHashWithReason(
		session.HashToken(staffToken),
		session.RevocationReasonUserLogout,
	))

	user := &database.User{Email: "hybrid-owner@example.com", Role: "user", EmailVerified: true}
	require.NoError(t, database.GetDB().Create(user).Error)
	userSess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &user.ID,
		TokenHash: "placeholder",
		Provider:  "web3",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	userToken, err := GenerateUserToken(user.ID, user.Email, "", user.Role, userSess.ID)
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.UpdateTokenHash(userSess.ID, session.HashToken(userToken)))

	router := gin.New()
	router.Use(HybridAuthenticationMiddleware())
	router.GET("/inside/businesses", func(c *gin.Context) {
		assert.Equal(t, "user", c.GetString("token_type"))
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/inside/businesses", nil)
	req.AddCookie(&http.Cookie{Name: "staff_token", Value: staffToken})
	req.AddCookie(&http.Cookie{Name: "session_token", Value: userToken})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
}

func TestHybridAuthentication_StillPrefersLiveStaffCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerA", "Live Staff Biz")
	staff := createStaffMember(t, business.ID, "live-staff@example.com", "Live Staff")
	staffToken := generateStaffTokenWithSession(t, staff)

	router := gin.New()
	router.Use(HybridAuthenticationMiddleware())
	router.GET("/inside/businesses", func(c *gin.Context) {
		assert.Equal(t, "staff", c.GetString("token_type"))
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/inside/businesses", nil)
	req.AddCookie(&http.Cookie{Name: "staff_token", Value: staffToken})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code, w.Body.String())
}
