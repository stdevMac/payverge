package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const createBusinessIdempotencyKey = "workspace-create-owner-42-attempt-1"

func stubCreateBusinessSideEffects(t *testing.T) {
	t.Helper()

	originalAdminEmail := sendAdminNewSignupEmailHook
	originalOnboardingEmail := sendCreateBusinessOnboardingEmail
	t.Cleanup(func() {
		sendAdminNewSignupEmailHook = originalAdminEmail
		sendCreateBusinessOnboardingEmail = originalOnboardingEmail
	})

	sendAdminNewSignupEmailHook = func(*database.Business) error { return nil }
	sendCreateBusinessOnboardingEmail = func(*database.Business) error { return nil }
}

func setupCreateBusinessIdempotencyTestDB(t *testing.T) {
	t.Helper()
	db := setupStaffHandlerTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.BusinessCreationRequest{}))
}

func createBusinessIdempotencyRequest(t *testing.T, key, name string) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(map[string]interface{}{
		"name":                  name,
		"default_currency":      "AED",
		"display_currency":      "AED",
		"default_language":      "en",
		"source_language":       "en",
		"timezone":              "Asia/Dubai",
		"tax_rate":              0,
		"service_fee_rate":      0,
		"tax_inclusive":         false,
		"service_inclusive":     false,
		"settlement_address":    "",
		"tipping_address":       "",
		"business_page_enabled": true,
		"address": map[string]string{
			"street":      "1 Main Street",
			"city":        "Dubai",
			"state":       "Dubai",
			"postal_code": "00000",
			"country":     "AE",
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/inside/businesses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Idempotency-Key", key)
	c.Set("address", "0xOwnerIdempotencyContract")
	CreateBusiness(c)
	return w
}

func decodeCreatedBusiness(t *testing.T, response *httptest.ResponseRecorder) database.Business {
	t.Helper()

	var business database.Business
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &business))
	return business
}

func TestCreateBusinessIdempotency_SameKeyAndPayloadReturnsOriginalWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCreateBusinessIdempotencyTestDB(t)
	stubCreateBusinessSideEffects(t)

	first := createBusinessIdempotencyRequest(t, createBusinessIdempotencyKey, "Idempotent Cafe")
	second := createBusinessIdempotencyRequest(t, createBusinessIdempotencyKey, "Idempotent Cafe")

	require.Equal(t, http.StatusCreated, first.Code)
	require.Equal(t, http.StatusCreated, second.Code)
	firstBusiness := decodeCreatedBusiness(t, first)
	secondBusiness := decodeCreatedBusiness(t, second)
	require.Equal(t, firstBusiness.ID, secondBusiness.ID,
		"an exact Idempotency-Key replay must return the originally created workspace")

	var count int64
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("owner_address = ?", "0xOwnerIdempotencyContract").Count(&count).Error)
	require.EqualValues(t, 1, count, "an exact replay must persist one workspace")
}

func TestCreateBusinessIdempotency_SameKeyDifferentPayloadConflicts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCreateBusinessIdempotencyTestDB(t)
	stubCreateBusinessSideEffects(t)

	first := createBusinessIdempotencyRequest(t, createBusinessIdempotencyKey, "Original Cafe")
	conflict := createBusinessIdempotencyRequest(t, createBusinessIdempotencyKey, "Different Cafe")

	require.Equal(t, http.StatusCreated, first.Code)
	require.Equal(t, http.StatusConflict, conflict.Code,
		"reusing an Idempotency-Key for a different canonical payload must return 409")

	var count int64
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("owner_address = ?", "0xOwnerIdempotencyContract").Count(&count).Error)
	require.EqualValues(t, 1, count, "a key/payload conflict must not create another workspace")
}

func TestCreateBusinessIdempotency_ResponseLossRetryCannotDuplicate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCreateBusinessIdempotencyTestDB(t)
	stubCreateBusinessSideEffects(t)

	// Simulate the server committing successfully while the client loses the
	// first response, then retries the identical request with the same key.
	_ = createBusinessIdempotencyRequest(t, createBusinessIdempotencyKey, "Response Loss Cafe")
	retry := createBusinessIdempotencyRequest(t, createBusinessIdempotencyKey, "Response Loss Cafe")

	require.Equal(t, http.StatusCreated, retry.Code)
	var count int64
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("owner_address = ?", "0xOwnerIdempotencyContract").Count(&count).Error)
	require.EqualValues(t, 1, count,
		"a response-loss retry must recover the committed workspace instead of inserting a duplicate")
}
