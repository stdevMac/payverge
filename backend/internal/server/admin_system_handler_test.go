package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/adminruntime"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAdminSystemHealthDB(t *testing.T) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file:admin_system_health?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(gormDB)
}

func TestGetAdminSystemHealth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAdminSystemHealthDB(t)
	adminruntime.SetWorker("test_worker", "running", "ok", true)

	router := gin.New()
	router.GET("/admin/system/health", GetAdminSystemHealth)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin/system/health", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var resp adminSystemHealthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "healthy", resp.Status)
	require.NotEmpty(t, resp.Checks)
	require.NotEmpty(t, resp.Workers)
}
