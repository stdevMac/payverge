package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func serveMedia(r http.Handler, method string) int {
	req := httptest.NewRequest(method, "/media/businesses/1/menu_items/a.png", nil)
	req.RemoteAddr = "203.0.113.7:1000"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

// TestRegisterMediaRoutesRateLimited: /media is exempt from the global API
// limiter, so registerMediaRoutes must put its own per-IP limiter in front of
// the handler, shared by GET and HEAD and configurable from the environment.
func TestRegisterMediaRoutesRateLimited(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv(envMediaRateLimitRPM, "1")
	t.Setenv(envMediaRateLimitBurst, "1")

	r := gin.New()
	registerMediaRoutes(r)

	if code := serveMedia(r, http.MethodGet); code == http.StatusTooManyRequests {
		t.Fatalf("first request got 429; the burst must admit it")
	}
	if code := serveMedia(r, http.MethodGet); code != http.StatusTooManyRequests {
		t.Fatalf("GET past the media budget got %d, want 429", code)
	}
	if code := serveMedia(r, http.MethodHead); code != http.StatusTooManyRequests {
		t.Fatalf("HEAD must share the GET bucket; got %d, want 429", code)
	}
}

// TestRegisterMediaRoutesRateLimitDisabled: MEDIA_RATE_LIMIT_REQUESTS_PER_MINUTE=0
// turns the limiter off.
func TestRegisterMediaRoutesRateLimitDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv(envMediaRateLimitRPM, "0")
	t.Setenv(envMediaRateLimitBurst, "")

	r := gin.New()
	registerMediaRoutes(r)
	for i := 0; i < 20; i++ {
		if code := serveMedia(r, http.MethodGet); code == http.StatusTooManyRequests {
			t.Fatalf("request #%d got 429 with the media limiter disabled", i)
		}
	}
}
