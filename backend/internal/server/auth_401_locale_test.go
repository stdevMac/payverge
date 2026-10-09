package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAuthMiddlewares_401HonorsAcceptLanguage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mws := map[string]gin.HandlerFunc{
		"hybrid": HybridAuthenticationMiddleware(),
		"staff":  StaffAuthenticationMiddleware(),
		"admin":  AuthenticationAdminMiddleware(),
		"user":   AuthenticationMiddleware(),
	}

	for name, mw := range mws {
		t.Run(name+"/es-AR", func(t *testing.T) {
			r := gin.New()
			r.Use(mw)
			r.GET("/probe", func(c *gin.Context) { c.Status(http.StatusNoContent) })

			req := httptest.NewRequest(http.MethodGet, "/probe", nil)
			req.Header.Set("Accept-Language", "es-AR")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
			var payload map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
			require.Equal(t, ErrCodeTokenMissing, payload["code"])
			msg, _ := payload["error"].(string)
			require.NotEqual(t, "Missing authentication token", msg)
			require.Contains(t, msg, "Iniciá")
		})

		t.Run(name+"/en", func(t *testing.T) {
			r := gin.New()
			r.Use(mw)
			r.GET("/probe", func(c *gin.Context) { c.Status(http.StatusNoContent) })

			req := httptest.NewRequest(http.MethodGet, "/probe", nil)
			req.Header.Set("Accept-Language", "en")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusUnauthorized, w.Code)
			var payload map[string]interface{}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
			require.Equal(t, ErrCodeTokenMissing, payload["code"])
			require.Equal(t, "Missing authentication token", payload["error"])
		})
	}
}
