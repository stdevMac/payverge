package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigureTrustedProxies_DisablesForwardedHeadersByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	require.NoError(t, ConfigureTrustedProxies(router, ""))

	router.GET("/ip", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "10.0.0.9:1234"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "10.0.0.9", w.Body.String())
}

func TestConfigureTrustedProxies_AllowsConfiguredProxyHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	require.NoError(t, ConfigureTrustedProxies(router, "127.0.0.1"))

	router.GET("/ip", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "1.2.3.4", w.Body.String())
}

func TestConfigureTrustedProxies_RejectsInvalidConfiguration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	err := ConfigureTrustedProxies(router, "not-a-valid-proxy")
	require.Error(t, err)
}

func TestConfigureTrustedPlatform_CloudflareUsesCFConnectingIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	require.NoError(t, ConfigureTrustedProxies(router, ""))
	require.NoError(t, ConfigureTrustedPlatform(router, "cloudflare"))

	router.GET("/ip", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})

	req := httptest.NewRequest(http.MethodGet, "/ip", nil)
	req.RemoteAddr = "10.0.0.9:1234"
	req.Header.Set("CF-Connecting-IP", "203.0.113.10")
	req.Header.Set("X-Forwarded-For", "198.51.100.20")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "203.0.113.10", w.Body.String())
}

func TestConfigureTrustedPlatform_RejectsUnknownValue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	err := ConfigureTrustedPlatform(router, "unknown-cdn")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported trusted platform")
}

func TestTrustedProxiesConfigured_ReportsEmpty(t *testing.T) {
	assert.False(t, TrustedProxiesConfigured(""))
	assert.False(t, TrustedProxiesConfigured("   "))
	assert.True(t, TrustedProxiesConfigured("127.0.0.1"))
	assert.True(t, TrustedProxiesConfigured("10.0.0.0/8, 172.16.0.0/12"))
}
