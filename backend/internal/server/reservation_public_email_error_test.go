package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A guest who books online without a usable email must get a coded validation
// error, not a bare English sentence — the storefront localizes by code and had
// nothing to key on.
func TestRespondGuestReservationEmailRequired_EmitsCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/business/demo/reservations", nil)

	respondGuestReservationEmailRequired(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "reservation_email_required", body["code"])
	assert.Contains(t, body["error"], "email address")
}

// The shared message → code map has to know the message too, so any other path
// that surfaces it still codes the response.
func TestReservationDomainErrorCode_EmailRequired(t *testing.T) {
	assert.Equal(
		t,
		"reservation_email_required",
		reservationDomainErrorCode("a valid email address is required to book online"),
	)
}
