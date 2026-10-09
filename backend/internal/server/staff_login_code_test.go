package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestRequestLoginCode_CreatesCodeWithUsedFalse verifies that newly created
// login codes are stored with Used=false so they are immediately usable.
// Previously the code was created with Used=true and then flipped via
// ActivateCodeForStaff; if that second step failed the code was permanently
// unusable.
func TestRequestLoginCode_CreatesCodeWithUsedFalse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createOwnedBusiness(t, "0xOwnerLoginCode", "LoginCode Biz")
	staff := createStaffMember(t, business.ID, "logincode@example.com", "LoginCode Staff")

	// Override the email sender so the test never hits the real mail server.
	origEmail := sendStaffLoginCodeEmail
	sendStaffLoginCodeEmail = func(s *database.Staff, b *database.Business, code string) error {
		return nil
	}
	defer func() { sendStaffLoginCodeEmail = origEmail }()

	body, err := json.Marshal(map[string]string{
		"email": staff.Email,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/staff/login-code", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	RequestLoginCode(c)

	assert.Equal(t, http.StatusOK, w.Code, "expected 200 OK from RequestLoginCode")

	// The code persisted in the DB must be active (Used=false).
	var code database.StaffLoginCode
	err = database.GetDB().
		Where("staff_id = ?", staff.ID).
		Order("id DESC").
		First(&code).Error
	require.NoError(t, err, "expected a login code to exist in the database")

	assert.False(t, code.Used, "login code must be created with Used=false so it can be consumed during login")
}

// TestRequestLoginCode_EmailSendLimitIsThreePerHour: four requests for one
// address all return the same generic 200, and only the first three send mail.
// A different address still sends.
func TestRequestLoginCode_EmailSendLimitIsThreePerHour(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	origGuard := staffLoginCodeSendGuard
	staffLoginCodeSendGuard = newStaffLoginClientGuard(time.Hour, 3)
	t.Cleanup(func() { staffLoginCodeSendGuard = origGuard })

	business := createOwnedBusiness(t, "0xOwnerLoginLimit", "Login Limit Biz")
	limited := createStaffMember(t, business.ID, "limit-a@example.com", "Limit A")
	other := createStaffMember(t, business.ID, "limit-b@example.com", "Limit B")

	origEmail := sendStaffLoginCodeEmail
	sends := map[string]int{}
	sendStaffLoginCodeEmail = func(s *database.Staff, _ *database.Business, _ string) error {
		sends[s.Email]++
		return nil
	}
	t.Cleanup(func() { sendStaffLoginCodeEmail = origEmail })

	post := func(email string) *httptest.ResponseRecorder {
		body, err := json.Marshal(map[string]string{"email": email})
		require.NoError(t, err)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/staff/login-code", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		RequestLoginCode(c)
		return w
	}

	var firstBody string
	for i := 0; i < 4; i++ {
		w := post(limited.Email)
		require.Equal(t, http.StatusOK, w.Code, "request %d: %s", i+1, w.Body.String())
		if i == 0 {
			firstBody = w.Body.String()
		} else {
			require.Equal(t, firstBody, w.Body.String())
		}
	}
	require.Equal(t, 3, sends[limited.Email])

	w := post(other.Email)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, firstBody, w.Body.String())
	require.Equal(t, 1, sends[other.Email])
}
