package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestRespondGuestOrderValidationError_EmitsCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	respondGuestOrderValidationError(c, services.NewOrderValidationError(
		services.OrderErrCodeItemUnavailable, "menu item '%s' is currently unavailable", "Burger"))

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), `"code":"item_unavailable"`)
	assert.Contains(t, w.Body.String(), "Burger")
}

func TestRespondGuestOrderValidationError_PlainErrorUsesSafeEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	respondGuestOrderValidationError(c, assert.AnError)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	body := w.Body.String()
	// Untyped errors use the product-safe bind envelope (no raw err.Error()).
	assert.Contains(t, body, `"code":"`+ErrCodeInvalidInput+`"`)
	assert.Contains(t, body, "Please check the form and try again.")
	assert.NotContains(t, body, assert.AnError.Error())
}

func TestRespondGuestOrderValidationError_BusinessClosedConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	respondGuestOrderValidationError(c, services.NewOrderValidationError(
		services.OrderErrCodeBusinessClosed, "business is closed"))

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), `"code":"business_closed"`)
}
