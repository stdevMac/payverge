package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/llm"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMapAIError_NeverExposesInternalText(t *testing.T) {
	cases := []error{
		errors.New("pq: ERROR: relation \"foo\" does not exist (SQLSTATE 42P01)"),
		errors.New("open /home/app/secret/key.pem: permission denied"),
		errors.New("open /Users/jdoe/secret/key.pem: permission denied"),
		errors.New("Authorization: Bearer sk-live-abc123"),
		errors.New("dial tcp 10.0.0.5:5432: i/o timeout"),
		llm.ErrMalformedResponse,
		llm.ErrPrivacyPolicy,
		llm.ErrBudgetExceeded,
	}
	for _, err := range cases {
		code, msg, status, _ := MapAIError(err)
		require.NotEmpty(t, code)
		require.NotEmpty(t, msg)
		require.GreaterOrEqual(t, status, 400)
		require.False(t, AIErrorLeaksSecret(msg), "message leaked: %q from %v", msg, err)
		require.False(t, AIErrorLeaksSecret(code))
		require.NotContains(t, msg, err.Error())
	}
}

func TestRespondAIError_JSONEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set("request_id", "req_test_1")

	RespondAIError(c, errors.New("SELECT * FROM users WHERE password='x'"))
	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	var body AIPublicError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, AICodeTemporarilyUnavailable, body.Code)
	require.Equal(t, "req_test_1", body.RequestID)
	require.True(t, body.Retryable)
	require.False(t, AIErrorLeaksSecret(w.Body.String()))
	require.NotContains(t, w.Body.String(), "SELECT")
	require.NotContains(t, w.Body.String(), "password")
}

func TestRespondAIErrorStructuredOutputFailure(t *testing.T) {
	router := gin.New()
	router.GET("/test", func(c *gin.Context) {
		c.Set("request_id", "req-wizard-1")
		RespondAIError(c, fmt.Errorf("%w: unexpected EOF", llm.ErrStructuredOutput))
	})
	request := httptest.NewRequest(http.MethodGet, "/test", nil)
	res := httptest.NewRecorder()
	router.ServeHTTP(res, request)
	require.Equal(t, http.StatusBadGateway, res.Code)
	var body AIPublicError
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.Equal(t, "ai_structured_output_invalid", body.Code)
	require.Equal(t, "req-wizard-1", body.RequestID)
	require.True(t, body.Retryable)
	require.NotContains(t, res.Body.String(), "unexpected EOF")
}

func TestMapAIError_Timeout(t *testing.T) {
	code, _, status, retryable := MapAIError(errors.New("context deadline exceeded"))
	require.Equal(t, AICodeTimeout, code)
	require.Equal(t, http.StatusGatewayTimeout, status)
	require.True(t, retryable)
}
