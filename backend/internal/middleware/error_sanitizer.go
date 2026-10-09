package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/stdevmac/payverge/backend/internal/logger"
)

const (
	publicInternalCode           = "INTERNAL"
	publicServiceUnavailableCode = "SERVICE_UNAVAILABLE"
	publicInternalMessage        = "Internal server error"
	publicUnavailableMessage     = "Service temporarily unavailable"
)

var privateLogSecretPatterns = []struct {
	pattern     *regexp.Regexp
	replacement string
}{
	{regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`), "Bearer [REDACTED]"},
	{regexp.MustCompile(`(?i)\b(?:sk|rk|pk)_(?:live|test)_[A-Za-z0-9_-]+\b`), "[REDACTED]"},
	{regexp.MustCompile(`(?i)\b(?:whsec|cs_live|cs_test)_[A-Za-z0-9_-]+\b`), "[REDACTED]"},
	{regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`), "[REDACTED]"},
	{
		regexp.MustCompile(`(?i)\b([a-z0-9_-]*(?:password|passwd|secret|token|api[_-]?key|authorization)[a-z0-9_-]*)\s*(?:=|:)\s*(?:"[^"]*"|'[^']*'|[^\s,;]+)`),
		`${1}=[REDACTED]`,
	},
}

type publicErrorEnvelope struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Message string `json:"message,omitempty"`
}

// PublicServerErrorAINotConfigured is the 503 an LLM-only route answers when
// no LLM provider is configured on this server.
const PublicServerErrorAINotConfigured = "ai_not_configured"

// AINotConfiguredMessage is the operator-facing explanation shipped with every
// ai_not_configured response. It names both provider paths of the env
// contract: OPENROUTER_API_KEY for the hosted router, LLM_BASE_URL (+
// LLM_API_KEY when the endpoint needs one) for a self-hosted model.
const AINotConfiguredMessage = "AI features are not configured on this server. Ask the operator to configure an LLM provider (OPENROUTER_API_KEY, or LLM_BASE_URL plus LLM_API_KEY for a self-hosted or OpenAI-compatible model) and restart."

// publicServerErrors is the allowlist of 5xx codes whose identity is safe and
// useful to show publicly. The body the sanitizer writes for them is built
// from this table only (server-authored constants), never from the handler's
// own bytes, so an allowlisted code cannot carry private detail out.
var publicServerErrors = map[string]string{
	PublicServerErrorAINotConfigured: AINotConfiguredMessage,
}

// AllowPublicServerError marks the current response as an allowlisted public
// 5xx. Call it before writing the status. Unknown codes, or a request that is
// not behind ErrorSanitizer, are ignored and the response is sanitized as
// usual.
func AllowPublicServerError(c *gin.Context, code string) {
	if _, ok := publicServerErrors[code]; !ok {
		return
	}
	if w, ok := c.Writer.(*publicErrorResponseWriter); ok {
		w.publicCode = code
	}
}

// publicErrorResponseWriter is the final response boundary for server errors.
// It replaces the body of any 5xx response with a small, stable JSON
// envelope and discards later writes for that response. The replacement happens
// when WriteHeader is called, rather than after the handler returns, so it also
// covers panics recovered by Gin and handlers that write only a status.
//
// Successful streaming responses are untouched: SSE and other chunked handlers
// normally commit 2xx before streaming, so they never enter the 5xx branch.
type publicErrorResponseWriter struct {
	gin.ResponseWriter
	sanitized   bool
	privateBody []byte
	publicCode  string
}

func (w *publicErrorResponseWriter) WriteHeader(status int) {
	if status < http.StatusInternalServerError {
		w.ResponseWriter.WriteHeader(status)
		return
	}

	if w.sanitized {
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(status)
	w.sanitized = true
	_, _ = w.ResponseWriter.Write(marshalPublicErrorWithCode(status, w.publicCode))
}

func (w *publicErrorResponseWriter) Write(body []byte) (int, error) {
	if w.Status() >= http.StatusInternalServerError {
		w.capturePrivateBody(body)
		if !w.sanitized {
			w.WriteHeader(w.Status())
		}
		return len(body), nil
	}
	return w.ResponseWriter.Write(body)
}

func (w *publicErrorResponseWriter) WriteString(body string) (int, error) {
	if w.Status() >= http.StatusInternalServerError {
		w.capturePrivateBody([]byte(body))
		if !w.sanitized {
			w.WriteHeader(w.Status())
		}
		return len(body), nil
	}
	return w.ResponseWriter.WriteString(body)
}

func (w *publicErrorResponseWriter) WriteHeaderNow() {
	if w.Status() >= http.StatusInternalServerError && !w.sanitized {
		w.WriteHeader(w.Status())
		return
	}
	w.ResponseWriter.WriteHeaderNow()
}

func (w *publicErrorResponseWriter) capturePrivateBody(body []byte) {
	// Private diagnostic logging must stay bounded even when a faulty handler
	// attempts to return a large dump. Eight KiB is enough to retain the useful
	// error envelope without allowing an unbounded per-request allocation.
	const maxPrivateBodyBytes = 8 << 10
	remaining := maxPrivateBodyBytes - len(w.privateBody)
	if remaining <= 0 || len(body) == 0 {
		return
	}
	if len(body) > remaining {
		body = body[:remaining]
	}
	w.privateBody = append(w.privateBody, body...)
}

func marshalPublicErrorWithCode(status int, code string) []byte {
	if message, ok := publicServerErrors[code]; ok {
		body, err := json.Marshal(publicErrorEnvelope{Error: code, Code: code, Message: message})
		if err == nil {
			return body
		}
	}
	return marshalPublicError(status)
}

func marshalPublicError(status int) []byte {
	envelope := publicErrorEnvelope{
		Error: publicInternalMessage,
		Code:  publicInternalCode,
	}
	if status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable ||
		status == http.StatusGatewayTimeout {
		envelope.Error = publicUnavailableMessage
		envelope.Code = publicServiceUnavailableCode
	}

	body, err := json.Marshal(envelope)
	if err != nil {
		// The envelope contains constants only. Keep a literal fail-safe anyway so
		// a future type change cannot reopen the public error leak.
		return []byte(`{"error":"Internal server error","code":"INTERNAL"}`)
	}
	return body
}

// ErrorSanitizer is the global public error boundary. Public/domain 4xx
// responses pass through unchanged. Every 5xx response is reduced to a stable
// code and safe message, while private failure detail is retained only in a
// structured server log correlated by request ID.
func ErrorSanitizer() gin.HandlerFunc {
	return func(c *gin.Context) {
		wrapped := &publicErrorResponseWriter{ResponseWriter: c.Writer}
		c.Writer = wrapped

		defer func() {
			if recovered := recover(); recovered != nil {
				logPrivateServerError(c, http.StatusInternalServerError, fmt.Sprint(recovered))
				panic(recovered)
			}

			status := c.Writer.Status()
			if status >= http.StatusInternalServerError {
				privateDetail := ""
				if len(c.Errors) > 0 {
					privateDetail = c.Errors.String()
				} else if len(wrapped.privateBody) > 0 {
					privateDetail = string(wrapped.privateBody)
				}
				logPrivateServerError(c, status, privateDetail)
			}
		}()

		c.Next()
	}
}

func logPrivateServerError(c *gin.Context, status int, privateDetail string) {
	requestID := c.GetString(RequestIDKey)
	if requestID == "" {
		requestID = c.Writer.Header().Get(RequestIDHeader)
	}

	entry := logger.Logger.WithFields(logrus.Fields{
		"request_id":    requestID,
		"method":        c.Request.Method,
		"path":          SafeRequestPath(c),
		"status":        status,
		"private_error": sanitizePrivateLogDetail(privateDetail),
	})
	if isExpectedUnavailable(c, status) {
		// AI-off instances answer these routes with 503 by
		// design; logging them at ERROR would be noise on every visit.
		entry.Info("feature not configured on this server")
		return
	}
	entry.Error("server request failed")
}

func isExpectedUnavailable(c *gin.Context, status int) bool {
	if status != http.StatusServiceUnavailable {
		return false
	}
	w, ok := c.Writer.(*publicErrorResponseWriter)
	return ok && w.publicCode == PublicServerErrorAINotConfigured
}

func sanitizePrivateLogDetail(detail string) string {
	for _, secretPattern := range privateLogSecretPatterns {
		detail = secretPattern.pattern.ReplaceAllString(detail, secretPattern.replacement)
	}
	return detail
}
