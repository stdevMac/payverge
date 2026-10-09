package middleware

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupIdempotencyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&IdempotencyKey{}); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}
	return db
}

func newTestRouter(db *gorm.DB, endpoint string, handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST(endpoint, Idempotency(db, "POST "+endpoint), handler)
	return r
}

func TestIdempotency_NoHeaderFallsThrough(t *testing.T) {
	db := setupIdempotencyTestDB(t)
	calls := 0
	r := newTestRouter(db, "/x", func(c *gin.Context) {
		calls++
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/x", bytes.NewBuffer([]byte(`{"a":1}`)))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("call %d: expected 200, got %d", i, w.Code)
		}
	}
	if calls != 2 {
		t.Fatalf("expected handler to fire twice with no header, got %d", calls)
	}
}

func TestIdempotency_ReplaysCachedResponse(t *testing.T) {
	db := setupIdempotencyTestDB(t)
	calls := 0
	r := newTestRouter(db, "/x", func(c *gin.Context) {
		calls++
		c.JSON(http.StatusCreated, gin.H{"call": calls})
	})

	// First call — handler runs, response cached.
	req1 := httptest.NewRequest("POST", "/x", bytes.NewBuffer([]byte(`{"a":1}`)))
	req1.Header.Set("Idempotency-Key", "test-key-1")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first: expected 201, got %d body=%s", w1.Code, w1.Body.String())
	}

	// Second call same key + body — replays cached response.
	req2 := httptest.NewRequest("POST", "/x", bytes.NewBuffer([]byte(`{"a":1}`)))
	req2.Header.Set("Idempotency-Key", "test-key-1")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusCreated {
		t.Fatalf("second: expected 201 replayed, got %d", w2.Code)
	}
	if calls != 1 {
		t.Fatalf("expected handler to fire only once, got %d calls", calls)
	}
	if w1.Body.String() != w2.Body.String() {
		t.Fatalf("expected identical bodies, got %q vs %q", w1.Body.String(), w2.Body.String())
	}
}

func TestIdempotency_MismatchedBodyReturns409(t *testing.T) {
	db := setupIdempotencyTestDB(t)
	r := newTestRouter(db, "/x", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req1 := httptest.NewRequest("POST", "/x", bytes.NewBuffer([]byte(`{"a":1}`)))
	req1.Header.Set("Idempotency-Key", "test-key-2")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first: expected 200, got %d", w1.Code)
	}

	// Same key, different body.
	req2 := httptest.NewRequest("POST", "/x", bytes.NewBuffer([]byte(`{"a":2}`)))
	req2.Header.Set("Idempotency-Key", "test-key-2")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("expected 409 for mismatched body, got %d body=%s", w2.Code, w2.Body.String())
	}
}

func TestIdempotency_LongKeyRejected(t *testing.T) {
	db := setupIdempotencyTestDB(t)
	r := newTestRouter(db, "/x", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	long := make([]byte, maxIdempotencyKeyLen+1)
	for i := range long {
		long[i] = 'a'
	}
	req := httptest.NewRequest("POST", "/x", bytes.NewBuffer([]byte(`{}`)))
	req.Header.Set("Idempotency-Key", string(long))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for long key, got %d", w.Code)
	}
}

// Regression for the manager-PIN probe→retry flow (bills void/refund):
// the frontend issues a no-PIN probe that 403s pin_required, then retries
// with the PIN header under the SAME Idempotency-Key and identical body.
// The cached 403 must NOT be replayed — auth-gate rejections don't consume
// the key.
func TestIdempotency_AuthRejectionNotCached(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusUnauthorized} {
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			db := setupIdempotencyTestDB(t)
			calls := 0
			r := newTestRouter(db, "/x", func(c *gin.Context) {
				calls++
				if c.GetHeader("X-Manager-Pin") == "" {
					c.JSON(status, gin.H{"code": "pin_required"})
					return
				}
				c.JSON(http.StatusOK, gin.H{"voided": true})
			})

			// Probe: no PIN header → auth-gate rejection.
			req1 := httptest.NewRequest("POST", "/x", bytes.NewBuffer([]byte(`{"reason":"comp"}`)))
			req1.Header.Set("Idempotency-Key", "probe-retry-key")
			w1 := httptest.NewRecorder()
			r.ServeHTTP(w1, req1)
			if w1.Code != status {
				t.Fatalf("probe: expected %d, got %d", status, w1.Code)
			}

			// The rejection must not have consumed the key.
			var count int64
			db.Model(&IdempotencyKey{}).Where("key = ?", "probe-retry-key").Count(&count)
			if count != 0 {
				t.Fatalf("expected auth rejection to release the idempotency row, found %d rows", count)
			}

			// Retry: same key + same body, PIN now in the header → must reach
			// the handler and succeed (not replay the cached 403, not 425).
			req2 := httptest.NewRequest("POST", "/x", bytes.NewBuffer([]byte(`{"reason":"comp"}`)))
			req2.Header.Set("Idempotency-Key", "probe-retry-key")
			req2.Header.Set("X-Manager-Pin", "1234")
			w2 := httptest.NewRecorder()
			r.ServeHTTP(w2, req2)
			if w2.Code != http.StatusOK {
				t.Fatalf("PIN retry: expected 200, got %d body=%s", w2.Code, w2.Body.String())
			}
			if calls != 2 {
				t.Fatalf("expected handler to fire on both probe and retry, got %d calls", calls)
			}

			// The successful mutation IS cached: a network retry of the PIN
			// call replays the 200 without re-running the handler.
			req3 := httptest.NewRequest("POST", "/x", bytes.NewBuffer([]byte(`{"reason":"comp"}`)))
			req3.Header.Set("Idempotency-Key", "probe-retry-key")
			req3.Header.Set("X-Manager-Pin", "1234")
			w3 := httptest.NewRecorder()
			r.ServeHTTP(w3, req3)
			if w3.Code != http.StatusOK {
				t.Fatalf("replay: expected cached 200, got %d", w3.Code)
			}
			if calls != 2 {
				t.Fatalf("expected success to be cached (no third handler call), got %d calls", calls)
			}
		})
	}
}

func TestIdempotency_EndpointScopesKey(t *testing.T) {
	db := setupIdempotencyTestDB(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	calls := 0
	r.POST("/a", Idempotency(db, "POST /a"), func(c *gin.Context) {
		calls++
		c.JSON(http.StatusOK, gin.H{"endpoint": "a"})
	})
	r.POST("/b", Idempotency(db, "POST /b"), func(c *gin.Context) {
		calls++
		c.JSON(http.StatusOK, gin.H{"endpoint": "b"})
	})

	// Same key, different endpoints — both should run.
	for _, path := range []string{"/a", "/b"} {
		req := httptest.NewRequest("POST", path, bytes.NewBuffer([]byte(`{}`)))
		req.Header.Set("Idempotency-Key", "shared-key")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d", path, w.Code)
		}
	}
	if calls != 2 {
		t.Fatalf("expected both handlers to fire (endpoint-scoped), got %d", calls)
	}
}
