package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestUpdateTranslation_RequiresBusinessID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body := map[string]interface{}{
		"entity_type":     "menu_item",
		"entity_id":       1,
		"field_name":      "name",
		"language_code":   "es",
		"translated_text": "Hamburguesa",
	}
	jsonBody, _ := json.Marshal(body)
	c.Request = httptest.NewRequest(http.MethodPut, "/translations", bytes.NewBuffer(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler := &CurrencyHandler{}
	handler.UpdateTranslation(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateTranslation_RejectsUnauthorizedUser(t *testing.T) {
	handler := setupCurrencyHandlerTestDB(t)
	business := createCurrencyTestBusiness(t, "0x1111111111111111111111111111111111111111")

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Set user context as a different user
	c.Set("address", "0xUnauthorizedUser")

	body := map[string]interface{}{
		"entity_type":     "menu_item",
		"entity_id":       1,
		"field_name":      "name",
		"language_code":   "es",
		"translated_text": "Hamburguesa",
		"business_id":     business.ID,
	}
	jsonBody, _ := json.Marshal(body)
	c.Request = httptest.NewRequest(http.MethodPut, "/translations", bytes.NewBuffer(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateTranslation(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestUpdateTranslation_FailsGracefullyWithoutDatabase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body := map[string]interface{}{
		"entity_type":     "menu_item",
		"entity_id":       1,
		"field_name":      "name",
		"language_code":   "es",
		"translated_text": "Hamburguesa",
		"business_id":     1,
	}
	jsonBody, _ := json.Marshal(body)
	c.Request = httptest.NewRequest(http.MethodPut, "/translations", bytes.NewBuffer(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler := &CurrencyHandler{}
	handler.UpdateTranslation(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestUpdateTranslation_RejectsUnknownBusiness(t *testing.T) {
	handler := setupCurrencyHandlerTestDB(t)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("address", "0xUnauthorizedUser")

	body := map[string]interface{}{
		"entity_type":     "menu_item",
		"entity_id":       1,
		"field_name":      "name",
		"language_code":   "es",
		"translated_text": "Hamburguesa",
		"business_id":     999,
	}
	jsonBody, _ := json.Marshal(body)
	c.Request = httptest.NewRequest(http.MethodPut, "/translations", bytes.NewBuffer(jsonBody))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.UpdateTranslation(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "Business not found")
}
