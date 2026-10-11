package crm

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterCustomerInvalidBodyReturnsCodedEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	h := NewHandler(NewService(db))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{"email":"not-an-email"}`)))
	c.Request.Header.Set("Content-Type", "application/json")

	h.RegisterCustomer(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "VALIDATION_INVALID_INPUT", body["code"])
	assert.NotEmpty(t, body["error"])
}

func TestGetProfileNotAuthenticatedReturnsCodedEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	h := NewHandler(NewService(db))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	h.GetCustomerProfile(c) // no customer_id in context

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "AUTH_NOT_AUTHENTICATED", body["code"])
}

func TestGetCustomerDetailsBusinessNotFoundReturnsCodedEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	h := NewHandler(NewService(db))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: "999999"}}

	h.GetCustomerDetails(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "NOT_FOUND", body["code"])
}

func TestPutLoyaltyInvalidBodyReturnsCodedEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	h := NewHandler(NewService(db))
	biz := createCRMTestBusiness(t, db, "biz-loyalty", "0xowner", "Loyalty Co")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader([]byte(`{"points_per_dollar": 500}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: biz.BusinessId}}

	h.PutLoyalty(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "VALIDATION_INVALID_INPUT", body["code"])
}
