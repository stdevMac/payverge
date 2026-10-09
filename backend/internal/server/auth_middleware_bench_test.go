package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
)

// BenchmarkAuthMiddleware measures the per-request cost of the user
// AuthenticationMiddleware: cookie/header token extraction, JWT verification,
// and validateSession (which short-circuits when session.GlobalStore is nil,
// as it is in this bench). A 200 from the trivial handler proves the
// middleware passed.
//
// The testcontainer + auto-migrate are kept because we insert a real User row
// the token references — even though the middleware itself doesn't load the
// user, future iterations may add that check, and a real DB makes the
// benchmark surface match production startup ordering.
func BenchmarkAuthMiddleware(b *testing.B) {
	// GetSecretKey() panics if JWT_SECRET_KEY is unset. We seed a deterministic
	// dev-only value so the bench mirrors prod startup ordering without needing
	// real secrets. b.Setenv handles cleanup after the bench.
	b.Setenv("JWT_SECRET_KEY", "perf-bench-jwt-secret-key-do-not-use-in-prod")
	setupServerBenchmarkDB(b)

	user := database.User{
		Email:   "perf-bench-user@example.test",
		Address: "0x000000000000000000000000000000000000bEEF",
		Role:    "user",
	}
	if err := database.GetDB().Create(&user).Error; err != nil {
		b.Fatalf("create user: %v", err)
	}

	token, err := server.GenerateUserToken(user.ID, user.Email, user.Address, user.Role)
	if err != nil {
		b.Fatalf("generate token: %v", err)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(server.AuthenticationMiddleware())
	r.GET("/protected", func(c *gin.Context) { c.Status(http.StatusOK) })

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("got %d, body=%s", w.Code, w.Body.String())
		}
	}
}
