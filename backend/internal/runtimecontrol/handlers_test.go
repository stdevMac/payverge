package runtimecontrol

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUpdateHandlerAcceptsValidControlChange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(New(newRuntimeControlTestDB(t)))
	r := gin.New()
	r.PUT("/runtime-controls/:key", h.Update)

	body := []byte(`{"enabled":false,"owner":"platform","reason":"drill","expires_at":"2099-01-01T00:00:00Z"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/runtime-controls/payments_enabled", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
}
