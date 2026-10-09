package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func TestPublicGuestOffers_DropsCodedOffersAndOmitsCodeKey(t *testing.T) {
	secret := "SECRET10"
	blank := "   "
	auto := database.Offer{
		ID: 1, BusinessID: 7, Name: "Lunch Special",
		DiscountType: "percentage", DiscountValue: 10, IsActive: true, ApplicableTo: "all",
	}
	coded := auto
	coded.ID = 2
	coded.Name = "Secret Percent"
	coded.Code = &secret
	whitespace := auto
	whitespace.ID = 3
	whitespace.Name = "Blank Code"
	whitespace.Code = &blank

	got := publicGuestOffers([]database.Offer{auto, coded, whitespace})
	require.NotNil(t, got)
	require.Len(t, got, 2)
	require.Equal(t, "Lunch Special", got[0].Name)
	require.Equal(t, "Blank Code", got[1].Name)

	empty := publicGuestOffers(nil)
	require.NotNil(t, empty)
	require.Empty(t, empty)

	body, err := json.Marshal(got)
	require.NoError(t, err)
	require.NotContains(t, string(body), "SECRET10")
	require.NotContains(t, string(body), `"code"`)
	require.Contains(t, string(body), "Lunch Special")
}

func TestGetMenuByBusinessCustomUrl_OmitsPromoCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicBusinessHandlerTestDB(t)
	services.ResetPricingCache()
	t.Cleanup(services.ResetPricingCache)

	business := createPublicBusinessRouteTestBusiness(t, "promo-leak-menu", true, true)
	secret := "SECRET10"
	require.NoError(t, database.GetDB().Create(&database.Offer{
		BusinessID: business.ID, Name: "Lunch Special",
		DiscountType: "percentage", DiscountValue: 10, IsActive: true, ApplicableTo: "all",
		WeekdayMask: 127,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.Offer{
		BusinessID: business.ID, Name: "Secret Percent", Code: &secret,
		DiscountType: "percentage", DiscountValue: 10, IsActive: true, ApplicableTo: "all",
		WeekdayMask: 127,
	}).Error)
	services.ResetPricingCache()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	GetMenuByBusinessCustomUrl(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	body := w.Body.String()
	require.NotContains(t, body, "SECRET10")
	require.NotContains(t, body, `"code"`)
	require.Contains(t, body, "Lunch Special")
	require.NotContains(t, body, "Secret Percent")
}

func TestPublicGuestOffers_JSONHasNoCodeKey(t *testing.T) {
	// Guard the type itself: even a projected offer must not grow a code field.
	body, err := json.Marshal(publicGuestOffer{Name: "Lunch Special"})
	require.NoError(t, err)
	require.False(t, strings.Contains(string(body), `"code"`))
}
