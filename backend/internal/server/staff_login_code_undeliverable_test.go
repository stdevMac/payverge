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

	"github.com/stdevmac/payverge/backend/internal/database"
)

func postRequestLoginCode(t *testing.T, email string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]string{"email": email})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/staff/request-login-code", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	RequestLoginCode(c)
	return w
}

func TestRequestLoginCode_ReservedLocalDomainIs400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	originalSender := sendStaffLoginCodeEmail
	defer func() { sendStaffLoginCodeEmail = originalSender }()
	sent := false
	sendStaffLoginCodeEmail = func(_ *database.Staff, _ *database.Business, _ string) error {
		sent = true
		return nil
	}

	w := postRequestLoginCode(t, "camila@payverge.local")
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.False(t, sent)

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.Equal(t, ErrCodeInvalidInput, payload["code"])
	assert.NotContains(t, w.Body.String(), "Login code sent")
}

func TestRequestLoginCode_WellFormedUnknownEmailHedgesWithoutRevealing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerLoginCodeHedge", "Hedge Biz")
	staff := createStaffMember(t, business.ID, "real-staff@example.com", "Real Staff")

	originalSender := sendStaffLoginCodeEmail
	defer func() { sendStaffLoginCodeEmail = originalSender }()
	var sentTo string
	sendStaffLoginCodeEmail = func(s *database.Staff, _ *database.Business, _ string) error {
		sentTo = s.Email
		return nil
	}

	missing := postRequestLoginCode(t, "nobody@example.com")
	known := postRequestLoginCode(t, staff.Email)

	assert.Equal(t, http.StatusOK, missing.Code, missing.Body.String())
	assert.Equal(t, http.StatusOK, known.Code, known.Body.String())
	assert.Equal(t, missing.Body.String(), known.Body.String(),
		"registered and unknown well-formed addresses must share one response body")

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(missing.Body.Bytes(), &payload))
	msg, _ := payload["message"].(string)
	assert.Contains(t, msg, "If this address has an account")
	assert.NotContains(t, msg, "Login code sent to your email")
	assert.Equal(t, staff.Email, sentTo)
}

func TestRequestLoginCode_ReservedLocalDoesNotRevealRegisteredStaff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerLocalStaff", "Local Biz")
	_ = createStaffMember(t, business.ID, "camila@payverge.local", "Camila")

	unknown := postRequestLoginCode(t, "ghost@internal.local")
	registered := postRequestLoginCode(t, "camila@payverge.local")

	assert.Equal(t, http.StatusBadRequest, unknown.Code)
	assert.Equal(t, http.StatusBadRequest, registered.Code)
	assert.Equal(t, unknown.Body.String(), registered.Body.String(),
		".local 400 must not be an account-enumeration oracle")
}
