package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

func TestIngestErrorIgnoresForgedRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if logger.Logger == nil {
		logger.InitLogger()
	}
	setupErrorLogTestDB(t)

	raw, err := json.Marshal(map[string]any{
		"timestamp": "2026-05-04T00:00:00Z",
		"error":     "boom",
		"component": "checkout",
		"function":  "pay",
		"requestId": "forged-123",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs/error", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set("request_id", "srv-abc")
	NewErrorLogHandler().IngestError(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var stored database.ErrorLog
	require.NoError(t, database.GetDB().Where("request_id = ?", "srv-abc").First(&stored).Error)
	require.Equal(t, "srv-abc", stored.RequestID)
	require.Equal(t, ErrorLogSourceClientUntrusted, stored.Source)

	var forged int64
	require.NoError(t, database.GetDB().Model(&database.ErrorLog{}).Where("request_id = ?", "forged-123").Count(&forged).Error)
	require.Zero(t, forged)
}

func TestErrorLogListAdminMCPExcludesClientUntrusted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if logger.Logger == nil {
		logger.InitLogger()
	}
	setupErrorLogTestDB(t)

	now := time.Now().UTC()
	require.NoError(t, database.StoreErrorLog(context.Background(), &database.ErrorLog{
		Timestamp: now,
		Source:    ErrorLogSourceClientUntrusted,
		Component: "checkout",
		Error:     "client boom",
		Message:   "client boom",
		RequestID: "client-row",
	}))
	require.NoError(t, database.StoreErrorLog(context.Background(), &database.ErrorLog{
		Timestamp: now.Add(time.Second),
		Source:    "backend",
		Component: "webhooks",
		Error:     "server boom",
		Message:   "server boom",
		RequestID: "server-row",
	}))

	mcp := listErrorsAs(t, "admin_mcp", "")
	require.EqualValues(t, 1, mcp.Total)
	require.Len(t, mcp.Errors, 1)
	require.Equal(t, "backend", mcp.Errors[0].Source)
	for _, row := range mcp.Errors {
		require.NotEqual(t, ErrorLogSourceClientUntrusted, row.Source)
	}

	filtered := listErrorsAs(t, "admin_mcp", "source="+ErrorLogSourceClientUntrusted)
	require.EqualValues(t, 0, filtered.Total)
	require.Empty(t, filtered.Errors)

	admin := listErrorsAs(t, "user", "")
	require.EqualValues(t, 2, admin.Total)
	sources := map[string]bool{}
	for _, row := range admin.Errors {
		sources[row.Source] = true
	}
	require.True(t, sources[ErrorLogSourceClientUntrusted], "a normal admin still sees client rows")
	require.True(t, sources["backend"])
}

type errorLogListResponse struct {
	Errors []database.ErrorLog `json:"errors"`
	Total  int64               `json:"total"`
}

func listErrorsAs(t *testing.T, tokenType, rawQuery string) errorLogListResponse {
	t.Helper()
	target := "/api/v1/admin/errors"
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Set("token_type", tokenType)
	NewErrorLogHandler().ListErrors(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp errorLogListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}
