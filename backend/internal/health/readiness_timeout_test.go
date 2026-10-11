package health

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// slowDriver is a database/sql driver whose every connection attempt blocks
// until its context is cancelled, simulating a saturated pool that can never
// hand out a connection in time.
type slowDriver struct{}

func (slowDriver) Open(_ string) (driver.Conn, error) { return nil, errors.New("use OpenConnector") }

type slowConnector struct{ dsn string }

func (c slowConnector) Connect(ctx context.Context) (driver.Conn, error) {
	// Block until the caller's context expires — exactly what a saturated
	// lib/pq pool does when all 25 connections are busy.
	<-ctx.Done()
	return nil, ctx.Err()
}
func (c slowConnector) Driver() driver.Driver { return slowDriver{} }

func init() {
	sql.Register("slowdb", slowDriver{})
}

// buildSlowGormDB opens a *gorm.DB backed by slowConnector without triggering
// an automatic Ping (which would also block).
func buildSlowGormDB(t *testing.T) *gorm.DB {
	t.Helper()
	sqlDB := sql.OpenDB(slowConnector{dsn: "unused"})
	gdb, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		DisableAutomaticPing: true,
	})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	return gdb
}

// TestCheckDatabaseCtxRespectsDeadline asserts that checkDatabaseCtx returns
// "unhealthy" promptly when the supplied context expires, proving that
// PingContext (not bare Ping) is used — the bare Ping ignores the context and
// would block forever on a saturated pool (EXT-4).
func TestCheckDatabaseCtxRespectsDeadline(t *testing.T) {
	gdb := buildSlowGormDB(t)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	result := checkDatabaseCtx(ctx, gdb)
	elapsed := time.Since(start)

	if result.Status != "unhealthy" {
		t.Fatalf("expected unhealthy, got %q (message: %s)", result.Status, result.Message)
	}
	// Should return within ~200ms of the 100ms deadline (not hang for seconds).
	if elapsed > 500*time.Millisecond {
		t.Fatalf("checkDatabaseCtx blocked %v; expected to return near the 100ms deadline", elapsed)
	}
}

// TestReadinessFailsFastOnUnreachableDB asserts the readiness probe returns
// promptly (well under the default driver/pool block) with 503 when the DB is
// unreachable, proving PingContext + a 2s deadline replaced the bare Ping that
// could hang on a saturated pool (EXT-4).
func TestReadinessFailsFastOnUnreachableDB(t *testing.T) {
	gin.SetMode(gin.TestMode)

	gdb := buildSlowGormDB(t)

	r := gin.New()
	r.GET("/ready", ReadinessHandler(ReadinessState{DB: gdb}))
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	req = req.WithContext(context.Background())
	w := httptest.NewRecorder()

	start := time.Now()
	r.ServeHTTP(w, req)
	elapsed := time.Since(start)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 for unreachable DB, got %d", w.Code)
	}
	// The handler derives a 2s deadline; allow 5s for test overhead.
	if elapsed > 5*time.Second {
		t.Fatalf("readiness blocked %v; expected fail-fast under the 2s probe ceiling", elapsed)
	}
}
