package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/server"
)

// FIND-033 residual: handlers package bind failures must not leak gin validator dumps.
func TestHandlersRespondBindError_NoValidatorDump(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		name     string
		setup    func(r *gin.Engine)
		method   string
		path     string
		body     string
		wantCode int
	}{
		{
			name: "plugin create invalid json type",
			setup: func(r *gin.Engine) {
				h := NewPluginHandlers(nil, nil)
				r.POST("/admin/plugins", h.CreatePlugin)
			},
			method:   http.MethodPost,
			path:     "/admin/plugins",
			body:     `{"name": 123}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name: "delivery create order type mismatch",
			setup: func(r *gin.Engine) {
				h := NewDeliveryHandler(nil)
				r.POST("/businesses/:id/delivery/orders", func(c *gin.Context) {
					c.Set("token_type", "staff")
					h.CreateDeliveryOrder(c)
				})
			},
			method:   http.MethodPost,
			path:     "/businesses/1/delivery/orders",
			body:     `{"delivery_fee": "not-a-number"}`,
			wantCode: http.StatusBadRequest,
		},
		{
			name: "fiscal update settings truncated json",
			setup: func(r *gin.Engine) {
				h := &FiscalHandlers{}
				r.PUT("/businesses/:id/fiscal/settings", h.UpdateSettings)
			},
			method:   http.MethodPut,
			path:     "/businesses/1/fiscal/settings",
			body:     `{`,
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			tc.setup(r)
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			body := w.Body.String()
			assert.NotContains(t, body, "Key:")
			assert.NotContains(t, body, "Field validation")
			assert.NotContains(t, body, "cannot unmarshal")
			assert.NotContains(t, body, `binding:"`)

			var resp server.ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, server.ErrCodeInvalidInput, resp.Code)
			assert.Equal(t, "Please check the form and try again.", resp.Error)
		})
	}
}
