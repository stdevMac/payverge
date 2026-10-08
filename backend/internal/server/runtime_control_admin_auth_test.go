package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/runtimecontrol"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/structs"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Lives in server, not runtimecontrol: runtimecontrol tests cannot import this
// package. fiscal now imports runtimecontrol, and guest/operator fiscal
// handlers import fiscal, so a runtimecontrol → server test import is a cycle.
func TestAdminControlSurfaceRequiresAdminAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("PAYVERGE_ADMIN_MCP_TOKEN", "runtime-admin-secret")
	t.Setenv("PAYVERGE_ADMIN_MCP_ALLOWED_IPS", "127.0.0.1/32")
	previousStore := session.GlobalStore
	session.GlobalStore = nil
	t.Cleanup(func() { session.GlobalStore = previousStore })

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared&_busy_timeout=5000"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&runtimecontrol.Control{}, &runtimecontrol.AuditEvent{}))

	structs.SecretKey = []byte("test-runtime-control-admin-secret")
	userDB, err := gorm.Open(sqlite.Open("file:"+t.Name()+"_users?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, userDB.AutoMigrate(&database.User{}))
	prev := database.GetDB()
	database.SetTestDB(userDB)
	t.Cleanup(func() { database.SetTestDB(prev) })
	admin := database.User{Email: "runtime-admin@example.com", Name: "Admin", Role: "admin", Address: "0xRuntimeAdmin"}
	require.NoError(t, userDB.Create(&admin).Error)
	token, err := GenerateUserToken(admin.ID, admin.Email, admin.Address, "admin")
	require.NoError(t, err)

	h := runtimecontrol.NewHandler(runtimecontrol.New(db))
	r := gin.New()
	group := r.Group("/api/v1/admin", AuthenticationAdminMiddleware())
	group.PUT("/runtime-controls/:key", h.Update)

	body := []byte(`{"enabled":false,"owner":"platform","reason":"drill","expires_at":"2099-01-01T00:00:00Z"}`)
	unauthorized := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/runtime-controls/payments_enabled", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(unauthorized, req)
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)

	// The MCP token is not a full admin. This route is outside its allowlist.
	mcp := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/admin/runtime-controls/payments_enabled", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer runtime-admin-secret")
	req.RemoteAddr = "127.0.0.1:1234"
	r.ServeHTTP(mcp, req)
	require.Equal(t, http.StatusForbidden, mcp.Code, mcp.Body.String())
	require.Contains(t, mcp.Body.String(), "MCP admin token is not allowed on this route")

	authorized := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/api/v1/admin/runtime-controls/payments_enabled", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "203.0.113.10:1234"
	r.ServeHTTP(authorized, req)
	require.Equal(t, http.StatusNoContent, authorized.Code, authorized.Body.String())
}
