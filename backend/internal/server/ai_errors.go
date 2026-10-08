package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/llm"

	"github.com/gin-gonic/gin"
)

// Stable public AI error codes (Wave 4). Frontend maps these to locale keys.
const (
	AICodeTemporarilyUnavailable  = "ai_temporarily_unavailable"
	AICodeTimeout                 = "ai_timeout"
	AICodeInvalidResponse         = "ai_invalid_response"
	AICodeContextUnavailable      = "ai_context_unavailable"
	AICodeJobConflict             = "ai_job_conflict"
	AICodeConfigurationRequired   = "ai_configuration_required"
	AICodePrivacyPolicy           = "ai_privacy_policy"
	AICodeBudgetExceeded          = "ai_budget_exceeded"
	AICodeRateLimited             = "ai_rate_limited"
	AICodeStructuredOutputInvalid = "ai_structured_output_invalid"
)

// AIPublicError is the only AI error shape allowed in JSON/SSE payloads.
type AIPublicError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
	Retryable bool   `json:"retryable"`
}

// MapAIError classifies an internal error into a stable public AI code.
// It never returns provider/SQL/path text.
func MapAIError(err error) (code string, message string, status int, retryable bool) {
	if err == nil {
		return AICodeTemporarilyUnavailable, "AI is temporarily unavailable.", http.StatusServiceUnavailable, true
	}
	switch {
	case errors.Is(err, llm.ErrStructuredOutput):
		return AICodeStructuredOutputInvalid, "The AI response was incomplete. Retry this step.", http.StatusBadGateway, true
	case errors.Is(err, llm.ErrPrivacyPolicy):
		return AICodePrivacyPolicy, "AI privacy requirements could not be met.", http.StatusServiceUnavailable, false
	case errors.Is(err, llm.ErrBudgetExceeded):
		return AICodeBudgetExceeded, "The AI budget for this business has been reached.", http.StatusPaymentRequired, false
	case errors.Is(err, llm.ErrRateLimited):
		return AICodeRateLimited, "AI is rate limited. Please try again shortly.", http.StatusTooManyRequests, true
	case errors.Is(err, llm.ErrMalformedResponse):
		return AICodeInvalidResponse, "The AI returned an invalid response.", http.StatusBadGateway, true
	case errors.Is(err, llm.ErrAuth):
		return AICodeConfigurationRequired, "AI is not configured correctly.", http.StatusServiceUnavailable, false
	case errors.Is(err, llm.ErrUpstream):
		// Upstream may wrap timeout/context cancellation.
		if isTimeoutErr(err) {
			return AICodeTimeout, "The AI request timed out.", http.StatusGatewayTimeout, true
		}
		return AICodeTemporarilyUnavailable, "AI is temporarily unavailable.", http.StatusBadGateway, true
	case isTimeoutErr(err):
		return AICodeTimeout, "The AI request timed out.", http.StatusGatewayTimeout, true
	case strings.Contains(strings.ToLower(err.Error()), "conflict"):
		return AICodeJobConflict, "Another AI job is already in progress.", http.StatusConflict, true
	default:
		return AICodeTemporarilyUnavailable, "AI is temporarily unavailable.", http.StatusServiceUnavailable, true
	}
}

func isTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "context canceled") ||
		strings.Contains(msg, "context cancelled")
}

// RespondAIError writes a safe AI error envelope and never echoes err.Error().
func RespondAIError(c *gin.Context, err error) {
	code, message, status, retryable := MapAIError(err)
	reqID := c.GetString("request_id")
	if reqID == "" {
		reqID = c.Writer.Header().Get("X-Request-ID")
	}
	// Privacy-safe telemetry only — enums, never raw error text.
	llm.EmitTelemetry(llm.AITelemetryEvent{
		Feature:    "ai",
		Surface:    "http",
		Code:       code,
		ErrorClass: publicErrorClass(code),
	})
	c.JSON(status, AIPublicError{
		Code:      code,
		Message:   message,
		RequestID: reqID,
		Retryable: retryable,
	})
}

func publicErrorClass(code string) string {
	switch code {
	case AICodeTimeout:
		return "timeout"
	case AICodeRateLimited:
		return "rate_limit"
	case AICodeBudgetExceeded:
		return "budget"
	case AICodePrivacyPolicy:
		return "privacy"
	case AICodeJobConflict:
		return "conflict"
	case AICodeInvalidResponse:
		return "invalid_response"
	case AICodeStructuredOutputInvalid:
		return "structured_output"
	case AICodeConfigurationRequired:
		return "configuration"
	default:
		return "unavailable"
	}
}

// AIErrorLeaksSecret reports whether a public payload contains dangerous substrings.
func AIErrorLeaksSecret(body string) bool {
	lower := strings.ToLower(body)
	needles := []string{
		"select ", "insert into", "password", "authorization:",
		"bearer ", "api_key", "-----begin", "/var/", "/users/",
		"postgres://", "sqlstate", "openrouter", "stack trace",
	}
	for _, n := range needles {
		if strings.Contains(lower, n) {
			return true
		}
	}
	return false
}
