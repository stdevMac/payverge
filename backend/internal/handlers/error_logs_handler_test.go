package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

// setupErrorLogTestDB creates an in-memory SQLite DB scoped to ErrorLog tests
// so the package's other test setup helpers don't pull in unrelated tables.
func setupErrorLogTestDB(t *testing.T) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)

	require.NoError(t, gormDB.AutoMigrate(&database.ErrorLog{}))
}

// TestIngestError_DefaultsMetadataForJSONBColumn locks in the fix for the
// production 500: the ErrorLog.Metadata field is declared `gorm:"type:jsonb"`,
// and PostgreSQL rejects an empty string ("") with `pq: invalid input syntax
// for type json`. SQLite happily stores ""; this test asserts that
// StoreErrorLog has coerced Metadata to a valid JSON document ("{}") before
// the row hits the driver, so a fresh PostgreSQL deployment would not
// re-trigger the regression.
func TestIngestError_DefaultsMetadataForJSONBColumn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if logger.Logger == nil {
		logger.InitLogger()
	}
	setupErrorLogTestDB(t)

	h := NewErrorLogHandler()

	body := map[string]any{
		"timestamp": "2026-05-04T00:00:00Z",
		"error":     "boom",
		"component": "ConvertingBusinessLandingPage",
		"function":  "useEffect",
		"requestId": "forged-meta",
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/error", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set("request_id", "req-meta")
	h.IngestError(c)

	require.Equal(t, http.StatusOK, w.Code, "expected 200, body: %s", w.Body.String())

	var stored database.ErrorLog
	require.NoError(t, database.GetDB().
		Where("request_id = ?", "req-meta").
		First(&stored).Error)
	require.Equal(t, "{}", stored.Metadata,
		"Metadata must be coerced to valid JSON so the jsonb column accepts the row")
	require.Equal(t, ErrorLogSourceClientUntrusted, stored.Source)
	require.Equal(t, "req-meta", stored.RequestID)
}

// TestIngestError_AcceptsFrontendShape posts the exact payload shape the
// frontend logError client emits and expects a 200. Reproduces a production
// 500 the client hit while rendering a public business page.
func TestIngestError_AcceptsFrontendShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if logger.Logger == nil {
		logger.InitLogger()
	}
	setupErrorLogTestDB(t)

	h := NewErrorLogHandler()

	body := map[string]any{
		"timestamp":      "2026-05-04T00:00:00Z",
		"error":          "Network error",
		"component":      "ConvertingBusinessLandingPage",
		"function":       "useEffect",
		"requestId":      "test-req",
		"additionalInfo": map[string]any{"path": "/b/demo"},
	}
	raw, err := json.Marshal(body)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/error", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	c, _ := gin.CreateTestContext(w)
	c.Request = req
	h.IngestError(c)

	require.Equal(t, http.StatusOK, w.Code, "expected 200, body: %s", w.Body.String())
}

func postErrorLog(t *testing.T, raw []byte, serverRequestID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/error", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	if serverRequestID != "" {
		c.Set("request_id", serverRequestID)
	}
	NewErrorLogHandler().IngestError(c)
	return w
}

// M-logs: the public ingest endpoint must not persist unbounded
// additionalInfo blobs; oversized objects are replaced by a marker.
func TestIngestError_CapsAdditionalInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if logger.Logger == nil {
		logger.InitLogger()
	}
	setupErrorLogTestDB(t)

	big := make(map[string]any)
	for i := 0; i < 40; i++ {
		big["k"+strconv.Itoa(i)] = strings.Repeat("x", 300)
	}
	raw, err := json.Marshal(map[string]any{
		"error": "boom", "requestId": "forged-big-info", "additionalInfo": big,
	})
	require.NoError(t, err)
	require.Less(t, int64(len(raw)), MaxErrorLogBodyBytes, "fixture must pass the body limit")

	w := postErrorLog(t, raw, "req-big-info")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var stored database.ErrorLog
	require.NoError(t, database.GetDB().Where("request_id = ?", "req-big-info").First(&stored).Error)
	require.Equal(t, true, stored.AdditionalInfo["truncated"])
	encoded, err := json.Marshal(stored.AdditionalInfo)
	require.NoError(t, err)
	require.LessOrEqual(t, len(encoded), maxErrorLogAdditionalInfoBytes)

	// Small objects are stored unchanged.
	raw, err = json.Marshal(map[string]any{
		"error": "boom", "requestId": "forged-small-info", "additionalInfo": map[string]any{"path": "/b/x"},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, postErrorLog(t, raw, "req-small-info").Code)
	var small database.ErrorLog
	require.NoError(t, database.GetDB().Where("request_id = ?", "req-small-info").First(&small).Error)
	require.Equal(t, "/b/x", small.AdditionalInfo["path"])
}

func TestIngestError_RejectsOversizedBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if logger.Logger == nil {
		logger.InitLogger()
	}
	setupErrorLogTestDB(t)

	raw, err := json.Marshal(map[string]any{
		"error": "boom", "requestId": "req-huge", "additionalInfo": map[string]any{"blob": strings.Repeat("y", int(MaxErrorLogBodyBytes))},
	})
	require.NoError(t, err)
	w := postErrorLog(t, raw, "")
	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)

	var count int64
	require.NoError(t, database.GetDB().Model(&database.ErrorLog{}).Count(&count).Error)
	require.Zero(t, count)
}
