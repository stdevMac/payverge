package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsRawValidatorDump(t *testing.T) {
	assert.True(t, IsRawValidatorDump(
		"Key: 'InviteStaffRequest.Email' Error:Field validation for 'Email' failed on the 'required' tag",
	))
	assert.False(t, IsRawValidatorDump("Customer name is required"))
}

func TestPublicBindErrorMessage_SanitizesGinDumps(t *testing.T) {
	ginDump := errors.New("Key: 'CreateReservationInput.CustomerName' Error:Field validation for 'CustomerName' failed on the 'required' tag")
	assert.Equal(t, defaultBindErrorMessage, PublicBindErrorMessage(ginDump))
	assert.Equal(t, defaultBindErrorMessage, PublicBindErrorMessage(errors.New(`json: cannot unmarshal number into Go value of type string`)))
	assert.Equal(t, defaultBindErrorMessage, PublicBindErrorMessage(errors.New("unexpected EOF")))
	assert.Equal(t, "slot already taken", PublicBindErrorMessage(errors.New("slot already taken")))
}

func TestRespondBindError_Envelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	RespondBindError(c, errors.New("Key: 'X.Y' Error:Field validation for 'Y' failed on the 'required' tag"))
	require.Equal(t, http.StatusBadRequest, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, ErrCodeInvalidInput)
	assert.Contains(t, body, defaultBindErrorMessage)
	assert.NotContains(t, body, "Key:")
	assert.NotContains(t, body, "Field validation")
}
