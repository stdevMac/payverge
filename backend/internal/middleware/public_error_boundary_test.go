package middleware

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/logger"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type capturedLogHook struct {
	entries []*logrus.Entry
}

func (h *capturedLogHook) Levels() []logrus.Level { return logrus.AllLevels }

func (h *capturedLogHook) Fire(entry *logrus.Entry) error {
	clone := *entry
	clone.Data = make(logrus.Fields, len(entry.Data))
	for key, value := range entry.Data {
		clone.Data[key] = value
	}
	h.entries = append(h.entries, &clone)
	return nil
}

func newPublicErrorBoundaryRouter(handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	// This is the production order: ErrorSanitizer wraps RequestID, so it can
	// attach the generated/validated request ID after the handler returns.
	router.Use(ErrorSanitizer())
	router.Use(RequestID())
	router.GET("/failure", handler)
	return router
}

func decodeErrorBoundaryBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body), recorder.Body.String())
	return body
}

func TestPublicErrorBoundarySanitizesInternalFailureClasses(t *testing.T) {
	testCases := []struct {
		name       string
		status     int
		privateErr string
		wantCode   string
		wantError  string
		forbidden  []string
	}{
		{
			name:       "database",
			status:     http.StatusInternalServerError,
			privateErr: `pq: relation "users" does not exist; SELECT * FROM users WHERE id = 918273`,
			wantCode:   "INTERNAL",
			wantError:  "Internal server error",
			forbidden:  []string{"pq:", "relation", "SELECT", "users", "918273"},
		},
		{
			name:       "provider",
			status:     http.StatusBadGateway,
			privateErr: "stripe request failed: sk_live_launch_secret checkout_session=cs_live_internal_123",
			wantCode:   "SERVICE_UNAVAILABLE",
			wantError:  "Service temporarily unavailable",
			forbidden:  []string{"stripe", "sk_live", "launch_secret", "cs_live_internal_123"},
		},
		{
			name:       "filesystem",
			status:     http.StatusInternalServerError,
			privateErr: "open /srv/payverge/private/config.json: permission denied",
			wantCode:   "INTERNAL",
			wantError:  "Internal server error",
			forbidden:  []string{"/srv/payverge", "config.json", "permission denied"},
		},
		{
			name:       "stack and internal identifier",
			status:     http.StatusInternalServerError,
			privateErr: "panic at handlers.go:417 goroutine 91 business_id=74421",
			wantCode:   "INTERNAL",
			wantError:  "Internal server error",
			forbidden:  []string{"handlers.go", "goroutine", "business_id", "74421"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			router := newPublicErrorBoundaryRouter(func(c *gin.Context) {
				_ = c.Error(errors.New(testCase.privateErr))
				c.JSON(testCase.status, gin.H{
					"error":       testCase.privateErr,
					"stack":       "private stack",
					"internal_id": 74421,
				})
			})

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/failure", nil)
			request.Header.Set(RequestIDHeader, "req-a4-"+strings.ReplaceAll(testCase.name, " ", "-"))
			router.ServeHTTP(recorder, request)

			assert.Equal(t, testCase.status, recorder.Code)
			assert.Equal(t, "application/json; charset=utf-8", recorder.Header().Get("Content-Type"))
			assert.Equal(t, "req-a4-"+strings.ReplaceAll(testCase.name, " ", "-"), recorder.Header().Get(RequestIDHeader))
			body := decodeErrorBoundaryBody(t, recorder)
			assert.Equal(t, map[string]interface{}{
				"code":  testCase.wantCode,
				"error": testCase.wantError,
			}, body)
			for _, forbidden := range testCase.forbidden {
				assert.NotContains(t, recorder.Body.String(), forbidden)
			}
			assert.NotContains(t, recorder.Body.String(), "private stack")
			assert.NotContains(t, recorder.Body.String(), "internal_id")
		})
	}
}

func TestPublicErrorBoundaryAddsBodyWhenHandlerOnlyWrites5xxStatus(t *testing.T) {
	router := newPublicErrorBoundaryRouter(func(c *gin.Context) {
		c.Status(http.StatusServiceUnavailable)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/failure", nil))

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Equal(t, map[string]interface{}{
		"code":  "SERVICE_UNAVAILABLE",
		"error": "Service temporarily unavailable",
	}, decodeErrorBoundaryBody(t, recorder))
}

func TestPublicErrorBoundaryPreservesUsefulDomain4xx(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(ErrorSanitizer())
	router.Use(RequestID())
	router.POST("/validation", func(c *gin.Context) {
		var input struct {
			PartySize int `json:"party_size" binding:"required,min=1"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error":  "Party size must be at least 1",
				"code":   "VALIDATION_INVALID_INPUT",
				"params": gin.H{"field": "party_size"},
			})
			return
		}
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/validation", strings.NewReader(`{"party_size":0}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
	assert.Equal(t, map[string]interface{}{
		"error":  "Party size must be at least 1",
		"code":   "VALIDATION_INVALID_INPUT",
		"params": map[string]interface{}{"field": "party_size"},
	}, decodeErrorBoundaryBody(t, recorder))
}

func TestPublicErrorBoundarySanitizesRecoveredPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.RecoveryWithWriter(io.Discard))
	router.Use(ErrorSanitizer())
	router.Use(RequestID())
	router.GET("/panic", func(*gin.Context) {
		panic("panic stack /srv/private/handler.go internal_id=99117")
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panic", nil))

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.Equal(t, map[string]interface{}{
		"code":  "INTERNAL",
		"error": "Internal server error",
	}, decodeErrorBoundaryBody(t, recorder))
	assert.NotContains(t, recorder.Body.String(), "/srv/private")
	assert.NotContains(t, recorder.Body.String(), "99117")
}

func TestPublicErrorBoundaryLogsPrivateDetailWithRequestID(t *testing.T) {
	previousLogger := logger.Logger
	testLogger := logrus.New()
	testLogger.SetOutput(io.Discard)
	hook := &capturedLogHook{}
	testLogger.AddHook(hook)
	logger.Logger = testLogger
	t.Cleanup(func() { logger.Logger = previousLogger })

	const privateDetail = "dial tcp db.internal:5432 password=private-provider-value"
	router := newPublicErrorBoundaryRouter(func(c *gin.Context) {
		// This intentionally models a legacy handler that forgot c.Error. The
		// boundary must still retain the rejected body for private diagnostics.
		c.JSON(http.StatusInternalServerError, gin.H{"error": privateDetail})
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/failure", nil)
	request.Header.Set(RequestIDHeader, "req-a4-private-log")
	router.ServeHTTP(recorder, request)

	require.Len(t, hook.entries, 1)
	entry := hook.entries[0]
	assert.Equal(t, "req-a4-private-log", entry.Data["request_id"])
	assert.Equal(t, http.StatusInternalServerError, entry.Data["status"])
	assert.Equal(t, http.MethodGet, entry.Data["method"])
	assert.Equal(t, "/failure", entry.Data["path"])
	assert.Contains(t, entry.Data["private_error"], "dial tcp db.internal:5432")
	assert.Contains(t, entry.Data["private_error"], "[REDACTED]")
	assert.NotContains(t, entry.Data["private_error"], "private-provider-value")
	assert.NotContains(t, recorder.Body.String(), privateDetail)
}

func TestSanitizePrivateLogDetailRedactsCredentials(t *testing.T) {
	// Assemble the documented fake access-key shape at runtime so the repository
	// secret scanner can keep treating any contiguous key-shaped source text as
	// a release-blocking finding.
	awsAccessKey := "AKIA" + "IOSFODNN7EXAMPLE"
	detail := "Bearer eyJhbGciOiJIUzI1NiJ9.payload.signature " +
		"sk_live_launch_secret whsec_webhook_private " +
		"password=hunter2 api_key:abc123 aws_secret_access_key=aws-private " +
		awsAccessKey

	sanitized := sanitizePrivateLogDetail(detail)

	assert.NotContains(t, sanitized, "eyJhbGci")
	assert.NotContains(t, sanitized, "launch_secret")
	assert.NotContains(t, sanitized, "webhook_private")
	assert.NotContains(t, sanitized, "hunter2")
	assert.NotContains(t, sanitized, "abc123")
	assert.NotContains(t, sanitized, "aws-private")
	assert.NotContains(t, sanitized, awsAccessKey)
	assert.Contains(t, sanitized, "[REDACTED]")
}
