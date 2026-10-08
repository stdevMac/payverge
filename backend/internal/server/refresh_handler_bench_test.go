package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// BenchmarkRefreshTokenRotation measures the hot POST /auth/refresh rotation
// path (cookie parse -> session lookup -> user lookup -> CAS rotate -> token
// mint). Perf-gate baseline/delta anchor for the durable-session merge
// (CLAUDE.md "Backend Performance Gate"). Each iteration performs a real
// rotation and feeds the newly minted refresh cookie into the next one.
func BenchmarkRefreshTokenRotation(b *testing.B) {
	gin.SetMode(gin.TestMode)
	_, user := setupRefreshHandlerTest(b)
	seedRefreshSession(b, user.ID, "bench-refresh-0")

	refresh := "bench-refresh-0"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
		req.AddCookie(&http.Cookie{Name: "refresh_token", Value: refresh})
		c.Request = req

		RefreshToken(c)

		if w.Code != http.StatusOK {
			b.Fatalf("refresh failed with status %d: %s", w.Code, w.Body.String())
		}
		next := ""
		for _, ck := range w.Result().Cookies() {
			if ck.Name == "refresh_token" && ck.Value != "" {
				next = ck.Value
			}
		}
		if next == "" {
			b.Fatal("no rotated refresh cookie issued")
		}
		refresh = next
	}
}
