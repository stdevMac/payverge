package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func TestGetMenuByBusinessCustomUrl_HidesWeekdayLunchOutsideDaypart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicBusinessHandlerTestDB(t)
	services.ResetPricingCache()
	t.Cleanup(func() {
		database.ResetScheduleNow()
		services.ResetPricingCache()
	})

	business := createPublicBusinessRouteTestBusiness(t, "lunch-daypart", true, true)
	require.NoError(t, database.GetDB().Model(business).Update("timezone", "America/New_York").Error)
	business.Timezone = "America/New_York"

	start, end := 11*60, 15*60
	require.NoError(t, database.GetDB().Create(&database.Offer{
		BusinessID: business.ID, Name: "Weekday Lunch 15% Off",
		Description:  "15% off any order, Monday through Friday, 11am–3pm.",
		DiscountType: "percentage", DiscountValue: 15, IsActive: true, ApplicableTo: "all",
		WeekdayMask: 62, StartMinute: &start, EndMinute: &end,
	}).Error)

	ny, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	offerNames := func(at time.Time) []string {
		database.SetScheduleNow(func() time.Time { return at })
		services.ResetPricingCache()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		GetMenuByBusinessCustomUrl(c)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())

		var resp struct {
			Offers []database.Offer `json:"offers"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		names := make([]string, 0, len(resp.Offers))
		for _, offer := range resp.Offers {
			names = append(names, offer.Name)
			if offer.Name == "Weekday Lunch 15% Off" {
				require.NotNil(t, offer.StartMinute, "guest JSON must include start_minute")
				require.NotNil(t, offer.EndMinute, "guest JSON must include end_minute")
			}
		}
		return names
	}

	require.Contains(t, offerNames(time.Date(2026, 8, 12, 12, 0, 0, 0, ny)), "Weekday Lunch 15% Off")
	require.NotContains(t, offerNames(time.Date(2026, 8, 12, 15, 14, 0, 0, ny)), "Weekday Lunch 15% Off")
}
