package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func forwardingWarnRequest(h gin.HandlerFunc, remote, xff string) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(h)
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remote
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	r.ServeHTTP(httptest.NewRecorder(), req)
}

// FBE-2: with the loopback-only default, a proxy on a container network must
// not degrade per-IP limits silently.
func TestWarnOnUntrustedForwardingPeer(t *testing.T) {
	var calls int
	logf := func(string, ...any) { calls++ }
	h := WarnOnUntrustedForwardingPeer(logf)

	forwardingWarnRequest(h, "127.0.0.1:5000", "203.0.113.9")    // loopback proxy: trusted by default
	forwardingWarnRequest(h, "203.0.113.7:5000", "198.51.100.1") // public peer: not a proxy shape
	forwardingWarnRequest(h, "172.18.0.5:5000", "")              // private peer without XFF
	assert.Equal(t, 0, calls)

	forwardingWarnRequest(h, "172.18.0.5:5000", "203.0.113.9")
	forwardingWarnRequest(h, "10.0.0.2:5000", "203.0.113.9")
	assert.Equal(t, 1, calls, "warns exactly once")
}
