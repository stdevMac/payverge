package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/structs"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAdminLiveRoleDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:admin_live_role_%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	prev := database.GetDB()
	database.SetTestDB(gormDB)
	t.Cleanup(func() { database.SetTestDB(prev) })

	require.NoError(t, gormDB.AutoMigrate(&database.User{}, &session.UserSession{}))

	previousStore := session.GlobalStore
	session.GlobalStore = nil // skip session store; focus on live role check
	t.Cleanup(func() { session.GlobalStore = previousStore })

	structs.SecretKey = []byte("test-admin-live-role-secret")
	return gormDB
}

func setupAdminLiveRoleRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AuthenticationAdminMiddleware())
	r.GET("/admin/probe", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"user_id": c.MustGet("user_id"),
			"role":    c.GetString("role"),
		})
	})
	return r
}

// SEC-7 / #304: JWT role=admin must not grant admin access after live demotion.
func TestAuthenticationAdminMiddleware_RejectsDemotedAdminJWT(t *testing.T) {
	db := setupAdminLiveRoleDB(t)
	user := database.User{Email: "was-admin@example.com", Name: "Was Admin", Role: "user", Address: "0xDemoted"}
	require.NoError(t, db.Create(&user).Error)

	token, err := GenerateUserToken(user.ID, user.Email, user.Address, "admin")
	require.NoError(t, err)

	r := setupAdminLiveRoleRouter(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/probe", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "forbidden")
}

func TestAuthenticationAdminMiddleware_AcceptsLiveAdminJWT(t *testing.T) {
	db := setupAdminLiveRoleDB(t)
	user := database.User{Email: "still-admin@example.com", Name: "Admin", Role: "admin", Address: "0xAdminLive"}
	require.NoError(t, db.Create(&user).Error)

	token, err := GenerateUserToken(user.ID, user.Email, user.Address, "admin")
	require.NoError(t, err)

	r := setupAdminLiveRoleRouter(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/probe", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"role":"admin"`)
	require.Contains(t, w.Body.String(), fmt.Sprintf(`"user_id":%d`, user.ID))
}

func TestAuthenticationAdminMiddleware_RejectsMissingUser(t *testing.T) {
	setupAdminLiveRoleDB(t)

	token, err := GenerateUserToken(99999, "ghost@example.com", "0xGhost", "admin")
	require.NoError(t, err)

	r := setupAdminLiveRoleRouter(t)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/probe", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "forbidden")
}
