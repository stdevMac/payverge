package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/logger"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
)

func init() {
	// Ensure the structured logger is available for ErrorSanitizer.
	if logger.Logger == nil {
		logger.InitLogger()
	}
}

func TestErrorSanitizerSanitizesAndLogs500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ErrorSanitizer())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "GORM: connection refused"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	// The private handler detail is logged server-side; only the stable public
	// envelope may cross the HTTP boundary.
	body := w.Body.String()
	if body != `{"error":"Internal server error","code":"INTERNAL"}` {
		t.Fatalf("expected sanitized public body, got: %s", body)
	}
}

func TestErrorSanitizer_ignores200(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ErrorSanitizer())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": "hello"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if body != `{"data":"hello"}` {
		t.Fatalf("expected original body, got: %s", body)
	}
}

func TestErrorSanitizer_passes400Unchanged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ErrorSanitizer())
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	body := w.Body.String()
	if body != `{"error":"invalid input"}` {
		t.Fatalf("expected original body, got: %s", body)
	}
}

func TestErrorSanitizer_passes422PluginUnavailable(t *testing.T) {
	// Guest crypto config rejections are 422 plugin_unavailable so the
	// public 5xx boundary cannot rewrite them to SERVICE_UNAVAILABLE (#527).
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ErrorSanitizer())
	r.POST("/crypto-quote", func(c *gin.Context) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": "This business is not currently accepting this crypto payment method",
			"code":  "plugin_unavailable",
		})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/crypto-quote", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"code":"plugin_unavailable"`) {
		t.Fatalf("expected plugin_unavailable on the wire, got: %s", body)
	}
	if strings.Contains(body, "SERVICE_UNAVAILABLE") {
		t.Fatalf("sanitizer remapped a product 422: %s", body)
	}
}

// A 503 that is designed behaviour (AI not configured) is logged
// at INFO; an unmarked 503 is still an ERROR.
func TestErrorSanitizer_expectedUnavailableLogsBelowError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hook := logtest.NewLocal(logger.Logger)
	defer hook.Reset()

	r := gin.New()
	r.Use(ErrorSanitizer())
	r.GET("/ai", func(c *gin.Context) {
		AllowPublicServerError(c, PublicServerErrorAINotConfigured)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": PublicServerErrorAINotConfigured})
	})
	r.GET("/down", func(c *gin.Context) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "db down"})
	})

	for path, want := range map[string]logrus.Level{"/ai": logrus.InfoLevel, "/down": logrus.ErrorLevel} {
		hook.Reset()
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s: expected 503, got %d", path, w.Code)
		}
		entry := hook.LastEntry()
		if entry == nil || entry.Level != want {
			t.Fatalf("%s: expected a %s log entry, got %+v", path, want, entry)
		}
	}
}

// The error-log sink must carry the route template, not the raw URL with its
// capability token.
func TestErrorSanitizerLogPathHasNoCapabilityToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hook := logtest.NewLocal(logger.Logger)
	defer hook.Reset()

	r := gin.New()
	r.Use(ErrorSanitizer())
	r.GET("/api/v1/guest/bill/:bill_token/split", func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "boom"})
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/guest/bill/BILLSECRET/split", nil))

	entry := hook.LastEntry()
	if entry == nil {
		t.Fatal("expected a log entry")
	}
	if got := entry.Data["path"]; got != "/api/v1/guest/bill/:bill_token/split" {
		t.Fatalf("path = %v, want route template", got)
	}
	for _, v := range entry.Data {
		if s, ok := v.(string); ok && strings.Contains(s, "BILLSECRET") {
			t.Fatalf("token leaked into log field: %q", s)
		}
	}
}
