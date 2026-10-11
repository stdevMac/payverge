package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/structs"
)

func setupAdminMCPAuthRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	previousStore := session.GlobalStore
	session.GlobalStore = nil
	t.Cleanup(func() { session.GlobalStore = previousStore })

	r := gin.New()
	r.Use(AuthenticationAdminMiddleware())
	probe := func(c *gin.Context) {
		userID, _ := ExtractAdminUserID(c)
		c.JSON(http.StatusOK, gin.H{
			"user_id":           userID,
			"admin_auth_method": c.GetString("admin_auth_method"),
			"token_type":        c.GetString("token_type"),
		})
	}
	// Allowlisted route the MCP server calls. FullPath must match the allowlist key.
	r.GET("/api/v1/admin/system/health", probe)
	r.POST("/api/v1/admin/users/:id/close", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"closed": true, "token_type": c.GetString("token_type")})
	})
	return r
}

func performAdminMCPAuthRequest(r *gin.Engine, token, remoteAddr string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/health", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = remoteAddr
	r.ServeHTTP(w, req)
	return w
}

func TestAuthenticationAdminMiddlewareAcceptsRawMCPTokenFromLoopback(t *testing.T) {
	t.Setenv("PAYVERGE_ADMIN_MCP_TOKEN", "mcp-secret-token")

	r := setupAdminMCPAuthRouter(t)
	w := performAdminMCPAuthRequest(r, "mcp-secret-token", "127.0.0.1:51742")

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.JSONEq(t, `{"user_id":0,"admin_auth_method":"mcp_token","token_type":"admin_mcp"}`, w.Body.String())
}

func TestAuthenticationAdminMiddlewareAcceptsSHA256MCPTokenFromAllowedCIDR(t *testing.T) {
	sum := sha256.Sum256([]byte("mcp-secret-token"))
	t.Setenv("PAYVERGE_ADMIN_MCP_TOKEN_SHA256", hex.EncodeToString(sum[:]))
	t.Setenv("PAYVERGE_ADMIN_MCP_ALLOWED_IPS", "198.51.100.0/24")

	r := setupAdminMCPAuthRouter(t)
	w := performAdminMCPAuthRequest(r, "mcp-secret-token", "198.51.100.23:51742")

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.JSONEq(t, `{"user_id":0,"admin_auth_method":"mcp_token","token_type":"admin_mcp"}`, w.Body.String())
}

func TestAuthenticationAdminMiddlewareRejectsMCPTokenFromUnallowedIP(t *testing.T) {
	t.Setenv("PAYVERGE_ADMIN_MCP_TOKEN", "mcp-secret-token")
	t.Setenv("PAYVERGE_ADMIN_MCP_ALLOWED_IPS", "127.0.0.1/32,::1")

	r := setupAdminMCPAuthRouter(t)
	w := performAdminMCPAuthRequest(r, "mcp-secret-token", "203.0.113.10:51742")

	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "MCP admin token is not allowed from this IP")
}

// TestAuthenticationAdminMiddlewareRejectsForwardedLoopback: a loopback
// address only counts for a direct local connection. A request that names
// 127.0.0.1 in X-Forwarded-For, or reaches a loopback peer with forwarding
// headers (a proxy on the same host), is refused under the default allowlist.
func TestAuthenticationAdminMiddlewareRejectsForwardedLoopback(t *testing.T) {
	t.Setenv("PAYVERGE_ADMIN_MCP_TOKEN", "mcp-secret-token")

	r := setupAdminMCPAuthRouter(t)
	require.NoError(t, r.SetTrustedProxies([]string{"0.0.0.0/0", "::/0"})) // worst case: trust every proxy

	for _, tc := range []struct{ remote, xff string }{
		{"198.51.100.4:4000", "127.0.0.1"},
		{"127.0.0.1:4000", "127.0.0.1"},
		{"127.0.0.1:4000", "203.0.113.10"},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/system/health", nil)
		req.Header.Set("Authorization", "Bearer mcp-secret-token")
		req.Header.Set("X-Forwarded-For", tc.xff)
		req.RemoteAddr = tc.remote
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusForbidden, w.Code, "remote=%s xff=%s: %s", tc.remote, tc.xff, w.Body.String())
	}
}

func TestAuthenticationAdminMiddlewareStillAcceptsAdminJWT(t *testing.T) {
	structs.SecretKey = []byte("test-admin-mcp-auth-secret")

	dsn := "file:admin_mcp_jwt_live?mode=memory&cache=shared"
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gormDB.AutoMigrate(&database.User{}))
	prev := database.GetDB()
	database.SetTestDB(gormDB)
	t.Cleanup(func() { database.SetTestDB(prev) })

	user := database.User{Email: "admin@example.com", Name: "Admin", Role: "admin", Address: "0xAdmin"}
	require.NoError(t, gormDB.Create(&user).Error)

	token, err := GenerateUserToken(user.ID, user.Email, user.Address, "admin")
	require.NoError(t, err)

	r := setupAdminMCPAuthRouter(t)
	w := performAdminMCPAuthRequest(r, token, "203.0.113.10:51742")

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), fmt.Sprintf(`"user_id":%d`, user.ID))
}

func TestAdminMCPTokenRouteAllowlist(t *testing.T) {
	t.Setenv("PAYVERGE_ADMIN_MCP_TOKEN", "mcp-secret-token")
	r := setupAdminMCPAuthRouter(t)

	allowed := performAdminMCPAuthRequest(r, "mcp-secret-token", "127.0.0.1:51742")
	require.Equal(t, http.StatusOK, allowed.Code, allowed.Body.String())
	require.Contains(t, allowed.Body.String(), `"token_type":"admin_mcp"`)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/7/close", nil)
	req.Header.Set("Authorization", "Bearer mcp-secret-token")
	req.RemoteAddr = "127.0.0.1:51742"
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "MCP admin token is not allowed on this route")
	require.Contains(t, w.Body.String(), ErrCodeNotAdmin)
}

func TestAdminMCPAllowlistDoesNotAffectAdminJWT(t *testing.T) {
	structs.SecretKey = []byte("test-admin-mcp-auth-secret")

	dsn := "file:admin_mcp_jwt_allowlist?mode=memory&cache=shared"
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gormDB.AutoMigrate(&database.User{}))
	prev := database.GetDB()
	database.SetTestDB(gormDB)
	t.Cleanup(func() { database.SetTestDB(prev) })

	user := database.User{Email: "admin-allowlist@example.com", Name: "Admin", Role: "admin", Address: "0xAdminAllow"}
	require.NoError(t, gormDB.Create(&user).Error)

	token, err := GenerateUserToken(user.ID, user.Email, user.Address, "admin")
	require.NoError(t, err)

	r := setupAdminMCPAuthRouter(t)

	health := performAdminMCPAuthRequest(r, token, "203.0.113.10:51742")
	require.Equal(t, http.StatusOK, health.Code, health.Body.String())
	require.Contains(t, health.Body.String(), `"token_type":"user"`)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/7/close", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "203.0.113.10:51742"
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), `"token_type":"user"`)
	require.NotContains(t, w.Body.String(), "MCP admin token is not allowed on this route")
}
