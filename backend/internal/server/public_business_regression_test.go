package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func setupPublicBusinessHandlerTestDB(t *testing.T) {
	t.Helper()

	setupStaffHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.Menu{},
		&database.Table{},
		&database.TableReservation{},
		&database.BusinessOperatingHours{},
		&database.ReservationSettings{},
		&database.Offer{},
		&database.Bundle{},
	))
}

func createPublicBusinessRouteTestBusiness(t testing.TB, customURL string, pageEnabled, isActive bool) *database.Business {
	t.Helper()

	business := &database.Business{
		BusinessId:           fmt.Sprintf("public-%s", customURL),
		Name:                 "Public Route Business",
		CustomURL:            customURL,
		OwnerAddress:         "0xPublicOwner",
		SettlementAddr:       "0x1111111111111111111111111111111111111111",
		TippingAddr:          "0x2222222222222222222222222222222222222222",
		BusinessPageEnabled:  pageEnabled,
		IsActive:             isActive,
		GoogleReviewsEnabled: true,
		GooglePlaceID:        "test-place-id",
		GoogleBusinessName:   "Public Route Business",
		GoogleBusinessURL:    "https://maps.example.com/business",
		GoogleReviewLink:     "https://maps.example.com/business/reviews",
		Timezone:             "UTC",
	}
	require.NoError(t, database.GetDB().Create(business).Error)
	require.NoError(t, database.GetDB().Model(business).Updates(map[string]interface{}{
		"business_page_enabled": pageEnabled,
		"is_active":             isActive,
	}).Error)
	return business
}

func TestGetBusinessByCustomURL_DemoStorefrontStill200(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicBusinessHandlerTestDB(t)
	services.DisablePricingCacheForTest(t)

	business := createPublicBusinessRouteTestBusiness(t, "payverge-core-demo-kitchen", true, true)
	require.NoError(t, database.GetDB().Model(business).Updates(map[string]interface{}{
		"is_demo":     true,
		"kind":        database.BusinessKindDemo,
		"description": "A Payverge showcase restaurant with realistic operational demo data.",
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	GetBusinessByCustomURL(c)

	require.Equal(t, http.StatusOK, w.Code, "demo storefronts must still 200")
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, true, body["is_demo"])
	assert.Equal(t, string(database.BusinessKindDemo), body["kind"])
}

func TestListPublishedStorefronts_IncludesPublishedDemosOmitsTestKind(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicBusinessHandlerTestDB(t)

	real := createPublicBusinessRouteTestBusiness(t, "real-bistro", true, true)
	demo := createPublicBusinessRouteTestBusiness(t, "payverge-core-demo-kitchen", true, true)
	require.NoError(t, database.GetDB().Model(demo).Updates(map[string]interface{}{
		"is_demo": true,
		"kind":    database.BusinessKindDemo,
	}).Error)
	fixture := createPublicBusinessRouteTestBusiness(t, "ci-test-kitchen", true, true)
	require.NoError(t, database.GetDB().Model(fixture).Updates(map[string]interface{}{
		"kind": database.BusinessKindTest,
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/business/storefronts", nil)
	ListPublishedStorefronts(c)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Storefronts []database.StorefrontSlug `json:"storefronts"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	urls := make([]string, 0, len(body.Storefronts))
	for _, slug := range body.Storefronts {
		urls = append(urls, slug.CustomURL)
	}
	assert.Contains(t, urls, real.CustomURL)
	assert.Contains(t, urls, demo.CustomURL)
	assert.NotContains(t, urls, fixture.CustomURL)
}

func TestGetMenuByBusinessCustomUrl_RejectsDisabledBusinessPage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicBusinessHandlerTestDB(t)
	business := createPublicBusinessRouteTestBusiness(t, "disabled-menu", false, true)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	GetMenuByBusinessCustomUrl(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetPublicReservationSettings_RejectsInactiveBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicBusinessHandlerTestDB(t)
	business := createPublicBusinessRouteTestBusiness(t, "inactive-reservation", true, false)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	GetPublicReservationSettings(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetPublicBusinessGoogleReviews_RejectsDisabledBusinessPage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicBusinessHandlerTestDB(t)
	business := createPublicBusinessRouteTestBusiness(t, "disabled-google", false, true)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	GetPublicBusinessGoogleReviews(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetMenuByBusinessCustomUrl_ReturnsServerErrorOnLookupFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalLookup := getPublicBusinessByCustomURL
	defer func() {
		getPublicBusinessByCustomURL = originalLookup
	}()

	getPublicBusinessByCustomURL = func(string) (*database.Business, error) {
		return nil, fmt.Errorf("database unavailable")
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "customUrl", Value: "broken-business"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	GetMenuByBusinessCustomUrl(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// TestGetMenuByBusinessCustomUrl_OmitsEmbeddedBusinessOnBundlesAndOffers
// is the endpoint-level regression for the SEC-2 follow-up leak: the
// public guest menu serves []Bundle and []Offer on an unauthenticated
// path. Both types embed the full Business struct; without the fix
// (json:"-") the zero-value Business was always emitted, advertising
// stripe_*, owner_*, settlement_address, tipping_address, etc. and
// turning any future Preload("Business") into a real leak. The model-
// level tests in models_json_business_embed_test.go already catch the
// populated case; this test exercises the wire path through the
// menuDataForBusiness seam, injecting a snapshot whose Offer/Bundle
// embed is densely populated. Without the fix the response body
// carries every sensitive key; with the fix the key is omitted
// entirely. (X-2)
func TestGetMenuByBusinessCustomUrl_OmitsEmbeddedBusinessOnBundlesAndOffers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicBusinessHandlerTestDB(t)
	services.ResetPricingCache()

	business := createPublicBusinessRouteTestBusiness(t, "menu-leak", true, true)

	// Hand-craft a snapshot with a populated embedded Business on both
	// the Bundle and the Offer. The seam is the same package-level var
	// the handler calls, so swapping it in routes the populated struct
	// through the production JSON serialization path.
	original := menuDataForBusiness
	defer func() { menuDataForBusiness = original }()
	menuDataForBusiness = func(businessID uint) (*database.Menu, []database.MenuCategory, []database.Offer, []database.Bundle, error) {
		require.Equal(t, business.ID, businessID)
		populated := database.Business{
			ID:             business.ID,
			BusinessId:     business.BusinessId,
			Name:           "Populated",
			OwnerAddress:   "0xOWNER",
			OwnerName:      "Jane Owner",
			Email:          "owner@leak.test",
			Phone:          "+15551234567",
			SettlementAddr: "0xSETTLE",
			TippingAddr:    "0xTIP",
		}
		offer := database.Offer{
			ID:            1,
			BusinessID:    business.ID,
			Name:          "Populated Offer",
			DiscountType:  "percentage",
			DiscountValue: 10,
			IsActive:      true,
			Business:      populated,
		}
		bundle := database.Bundle{
			ID:         1,
			BusinessID: business.ID,
			Name:       "Populated Bundle",
			Items:      "[]",
			IsActive:   true,
			Business:   populated,
		}
		return &database.Menu{BusinessID: business.ID, Categories: "[]", IsActive: true},
			nil, []database.Offer{offer}, []database.Bundle{bundle}, nil
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	GetMenuByBusinessCustomUrl(c)

	require.Equal(t, http.StatusOK, w.Code)

	body := w.Body.String()
	assert.NotContains(t, body, "cus_LEAK", "response body leaked stripe_customer_id value")
	assert.NotContains(t, body, "sub_LEAK", "response body leaked stripe_subscription_id value")
	assert.NotContains(t, body, "0xOWNER", "response body leaked owner_address value")
	assert.NotContains(t, body, "Jane Owner", "response body leaked owner_name value")
	assert.NotContains(t, body, "owner@leak.test", "response body leaked email value")
	assert.NotContains(t, body, "0xSETTLE", "response body leaked settlement_address value")
	assert.NotContains(t, body, "0xTIP", "response body leaked tipping_address value")
	assert.NotContains(t, body, `"stripe_customer_id"`, "response carried stripe_customer_id key")
	assert.NotContains(t, body, `"stripe_subscription_id"`, "response carried stripe_subscription_id key")
	assert.NotContains(t, body, `"owner_address"`, "response carried owner_address key")
	assert.NotContains(t, body, `"settlement_address"`, "response carried settlement_address key")

	var resp map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &resp))

	bundles, ok := resp["bundles"].([]any)
	require.True(t, ok, "response missing bundles array")
	require.Len(t, bundles, 1, "expected exactly the populated bundle")
	offers, ok := resp["offers"].([]any)
	require.True(t, ok, "response missing offers array")
	require.Len(t, offers, 1, "expected exactly the populated offer")
}
