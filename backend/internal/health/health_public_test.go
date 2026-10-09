package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicHandler_OmitsDetailWithoutToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv(detailTokenEnv, "")

	detailedCalled := false
	detailed := func(c *gin.Context) {
		detailedCalled = true
		c.JSON(http.StatusOK, gin.H{
			"status": "healthy",
			"uptime": "1h",
			"checks": []any{},
		})
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)

	PublicHandler(openMemoryDB(t), detailed)(c)

	require.Equal(t, http.StatusOK, w.Code)
	assert.False(t, detailedCalled)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "ok", body["status"])
	assert.NotContains(t, body, "uptime")
	assert.NotContains(t, body, "checks")
	assert.NotContains(t, body, "version")
}

func TestPublicHandler_ReturnsDetailWithBearerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv(detailTokenEnv, "secret-health-token")

	detailed := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy", "uptime": "1h"})
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	c.Request.Header.Set("Authorization", "Bearer secret-health-token")

	PublicHandler(nil, detailed)(c)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "healthy", body["status"])
	assert.Equal(t, "1h", body["uptime"])
}

func TestPublicHandler_RejectsWrongToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv(detailTokenEnv, "secret-health-token")

	detailedCalled := false
	detailed := func(c *gin.Context) {
		detailedCalled = true
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	c.Request.Header.Set("Authorization", "Bearer wrong")

	PublicHandler(openMemoryDB(t), detailed)(c)

	require.Equal(t, http.StatusOK, w.Code)
	assert.False(t, detailedCalled)
	assert.JSONEq(t, `{"status":"ok"}`, w.Body.String())
}

func TestPublicHandler_ReportsDatabaseAvailability(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv(detailTokenEnv, "")

	detailed := func(c *gin.Context) {
		t.Fatal("detailed handler must not run without a token")
	}

	t.Run("open", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		PublicHandler(openMemoryDB(t), detailed)(c)
		require.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"status":"ok"}`, w.Body.String())
	})

	t.Run("closed", func(t *testing.T) {
		db := openMemoryDB(t)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		PublicHandler(db, detailed)(c)
		require.Equal(t, http.StatusServiceUnavailable, w.Code)
		assert.JSONEq(t, `{"status":"unavailable"}`, w.Body.String())
	})

	t.Run("nil", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		PublicHandler(nil, detailed)(c)
		require.Equal(t, http.StatusServiceUnavailable, w.Code)
		assert.JSONEq(t, `{"status":"unavailable"}`, w.Body.String())
	})
}

func TestPublicReadinessHandler_QueryTokenDoesNotUnlockDetail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv(detailTokenEnv, "secret-health-token")

	state := ReadinessState{DB: openMemoryDB(t)}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/health/ready?token=secret-health-token", nil)
	PublicReadinessHandler(state)(c)

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"status":"ready"}`, w.Body.String())

	w = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil)
	c.Request.Header.Set("Authorization", "Bearer secret-health-token")
	PublicReadinessHandler(state)(c)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "ready", body["status"])
	assert.Contains(t, body, "checks")
}

func TestPublicReadinessHandler_OmitsChecksWithoutToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_ = os.Unsetenv(detailTokenEnv)

	state := ReadinessState{
		DB: nil, // nil DB → not ready
		Components: []ComponentResult{
			{Component: "jwt", Status: "ok"},
		},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil)

	PublicReadinessHandler(state)(c)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "not ready", body["status"])
	assert.NotContains(t, body, "checks")
}

func TestLivenessHandler_MinimalPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil)

	LivenessHandler()(c)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "alive", body["status"])
	assert.NotContains(t, body, "timestamp")
}
