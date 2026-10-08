package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newIdempotencyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&IdempotencyKey{}))
	return db
}

// driveOnce runs one request through the Idempotency middleware + a handler that
// returns the given status, and reports the observed status.
func driveOnce(t *testing.T, db *gorm.DB, key string, handlerStatus int) int {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/close", Idempotency(db, "bill_close"), func(c *gin.Context) {
		c.JSON(handlerStatus, gin.H{"status": handlerStatus})
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/close", http.NoBody)
	req.Header.Set(IdempotencyHeaderName, key)
	r.ServeHTTP(w, req)
	return w.Code
}

// A transient 500 must NOT be cached: the retry must re-run the handler and be
// able to succeed (200), not replay the cached 500.
func TestIdempotency_DoesNotCache5xx(t *testing.T) {
	db := newIdempotencyTestDB(t)
	require.Equal(t, http.StatusInternalServerError, driveOnce(t, db, "k-5xx", http.StatusInternalServerError))
	// Same key, handler now succeeds — the claimed row must have been deleted.
	require.Equal(t, http.StatusOK, driveOnce(t, db, "k-5xx", http.StatusOK),
		"a cached 5xx replayed and permanently blocked the retry")

	var rows []IdempotencyKey
	require.NoError(t, db.Where("key = ?", "k-5xx").Find(&rows).Error)
	require.Len(t, rows, 1, "only the successful 200 row should remain")
	require.Equal(t, http.StatusOK, rows[0].ResponseStatus)
}

// A terminal 4xx (e.g. 422 validation) is still cached and replayed — that
// outcome is deterministic and safe to dedup.
func TestIdempotency_StillCaches4xx(t *testing.T) {
	db := newIdempotencyTestDB(t)
	require.Equal(t, http.StatusUnprocessableEntity, driveOnce(t, db, "k-4xx", http.StatusUnprocessableEntity))
	require.Equal(t, http.StatusUnprocessableEntity, driveOnce(t, db, "k-4xx", http.StatusOK),
		"a terminal 4xx must replay from cache (handler must NOT re-run)")
}
