package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/emails"
)

func TestSendOperationalUpdate_ReturnsServiceUnavailableWhenEmailServerDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalEmailServer := emails.EmailServerInstance
	emails.EmailServerInstance = nil
	defer func() { emails.EmailServerInstance = originalEmailServer }()

	body, err := json.Marshal(map[string]any{
		"recipients":   []string{"ops@example.com"},
		"update_title": "Maintenance",
		"update_intro": "Heads up",
		"update_body":  "We will be down briefly",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	SendOperationalUpdate(c)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "Email service not available")
}

func TestSendPlatformUpdate_ReturnsServiceUnavailableWhenEmailServerDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalEmailServer := emails.EmailServerInstance
	emails.EmailServerInstance = nil
	defer func() { emails.EmailServerInstance = originalEmailServer }()

	body, err := json.Marshal(map[string]any{
		"recipients":   []string{"ops@example.com"},
		"update_title": "New Feature",
		"update_intro": "Released",
		"update_body":  "Feature details",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	SendPlatformUpdate(c)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "Email service not available")
}
