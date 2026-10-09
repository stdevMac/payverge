package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupPromoHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Table{},
		&database.Offer{},
	))
	return gormDB
}

func ptr[T any](v T) *T { return &v }

// newPromoRouter returns a gin engine wired for ValidatePromoCode tests.
func newPromoRouter() *gin.Engine {
	r := gin.New()
	r.POST("/guest/table/:code/validate-promo", ValidatePromoCode)
	return r
}

// postPromo is a test helper that issues a POST to the validate-promo endpoint.
func postPromo(t *testing.T, router *gin.Engine, tableCode, promoCode string) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(map[string]string{"code": promoCode})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/guest/table/"+tableCode+"/validate-promo", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// seedPromoFixture creates a business, a table, and an offer with the given code/dates.
func seedPromoFixture(
	t *testing.T,
	tableCode string,
	offerCode *string,
	isActive bool,
	startDate *time.Time,
	endDate *time.Time,
) (*database.Business, *database.Table, *database.Offer) {
	t.Helper()

	business := createOwnedBusiness(t, "0xPromoOwner", "Promo Biz "+tableCode)

	table := &database.Table{
		BusinessID: business.ID,
		Name:       "T-Promo",
		TableCode:  tableCode,
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)

	offer := &database.Offer{
		BusinessID:    business.ID,
		Name:          "Happy Hour",
		DiscountType:  "percentage",
		DiscountValue: 15.0,
		ApplicableTo:  "all",
		IsActive:      isActive,
		Code:          offerCode,
		StartDate:     startDate,
		EndDate:       endDate,
	}
	require.NoError(t, database.GetDB().Create(offer).Error)

	return business, table, offer
}

// TestValidatePromo_ValidCodeReturns200 verifies that a matching, active promo
// code returns 200 with the offer details.
func TestValidatePromo_ValidCodeReturns200(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPromoHandlerTestDB(t)

	_, _, offer := seedPromoFixture(t, "tbl-valid-200", ptr("SAVE15"), true, nil, nil)

	router := newPromoRouter()
	w := postPromo(t, router, "tbl-valid-200", "SAVE15")

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	offerResp, ok := resp["offer"].(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, float64(offer.ID), offerResp["id"])
	assert.Equal(t, offer.Name, offerResp["name"])
	assert.Equal(t, offer.DiscountType, offerResp["discount_type"])
	assert.Equal(t, offer.DiscountValue, offerResp["discount_value"])
	assert.Equal(t, offer.ApplicableTo, offerResp["applicable_to"])
}

// TestValidatePromo_InvalidCodeReturns404 checks that a code that doesn't
// match any offer returns 404 with an appropriate error message.
func TestValidatePromo_InvalidCodeReturns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPromoHandlerTestDB(t)

	seedPromoFixture(t, "tbl-invalid-404", ptr("REAL10"), true, nil, nil)

	router := newPromoRouter()
	w := postPromo(t, router, "tbl-invalid-404", "FAKECODE")

	require.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp["error"], "Invalid or expired code")
}

// TestValidatePromo_ExpiredOfferReturns404 verifies that an offer whose
// end_date is in the past is rejected.
func TestValidatePromo_ExpiredOfferReturns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPromoHandlerTestDB(t)

	past := time.Now().UTC().Add(-24 * time.Hour)
	seedPromoFixture(t, "tbl-expired-404", ptr("OLD10"), true, nil, &past)

	router := newPromoRouter()
	w := postPromo(t, router, "tbl-expired-404", "OLD10")

	require.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp["error"], "Invalid or expired code")
}

// TestValidatePromo_CaseInsensitiveMatch ensures that "save15", "SAVE15",
// and "Save15" all resolve to the same offer.
func TestValidatePromo_CaseInsensitiveMatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPromoHandlerTestDB(t)

	_, _, offer := seedPromoFixture(t, "tbl-case-200", ptr("SUMMER20"), true, nil, nil)

	router := newPromoRouter()

	for _, variant := range []string{"summer20", "Summer20", "SUMMER20"} {
		w := postPromo(t, router, "tbl-case-200", variant)
		require.Equal(t, http.StatusOK, w.Code, "variant %q should match", variant)

		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		offerResp := resp["offer"].(map[string]interface{})
		assert.Equal(t, float64(offer.ID), offerResp["id"])
	}
}

// TestValidatePromo_UnknownTableReturns404 verifies that an unrecognised table
// code returns 404 before any offer lookup is attempted.
func TestValidatePromo_UnknownTableReturns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPromoHandlerTestDB(t)

	router := newPromoRouter()
	w := postPromo(t, router, "tbl-does-not-exist", "ANY")

	require.Equal(t, http.StatusNotFound, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Contains(t, resp["error"], "Table not found")
}

// TestValidatePromo_MissingBodyReturns400 ensures that an empty/missing JSON
// body returns a 400 Bad Request.
func TestValidatePromo_MissingBodyReturns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPromoHandlerTestDB(t)

	seedPromoFixture(t, "tbl-bad-req", ptr("CODE"), true, nil, nil)

	router := newPromoRouter()

	req := httptest.NewRequest(http.MethodPost, "/guest/table/tbl-bad-req/validate-promo", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestValidatePromo_FutureStartDateReturns404 verifies that an offer whose
// start_date is in the future is not yet valid.
func TestValidatePromo_FutureStartDateReturns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPromoHandlerTestDB(t)

	future := time.Now().UTC().Add(24 * time.Hour)
	seedPromoFixture(t, "tbl-future-404", ptr("FUTURE10"), true, &future, nil)

	router := newPromoRouter()
	w := postPromo(t, router, "tbl-future-404", "FUTURE10")

	require.Equal(t, http.StatusNotFound, w.Code)
}

// TestValidatePromo_SuspendedBusinessIsUnavailable verifies that a suspended
// business's guest promo route is gated with the shared 403
// business_unavailable contract, matching sibling guest routes.
func TestValidatePromo_SuspendedBusinessIsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPromoHandlerTestDB(t)

	business, _, _ := seedPromoFixture(t, "tbl-inactive-402", ptr("SAVE15"), true, nil, nil)
	business.IsActive = false
	require.NoError(t, database.GetDB().Save(business).Error)

	router := newPromoRouter()
	w := postPromo(t, router, "tbl-inactive-402", "SAVE15")

	require.Equal(t, http.StatusForbidden, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "business_unavailable", resp["code"])
}
