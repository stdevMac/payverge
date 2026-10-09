package crm

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestPutLoyaltyHandler_PersistsIndependentEarnAndRedeemRates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupLoyaltyHandlerDB(t)
	seedLoyaltyTestBusiness(t, db, 91)

	body, _ := json.Marshal(map[string]any{
		"enabled":                      true,
		"points_per_dollar":            1.25,
		"redemption_points_per_dollar": 100,
		"tiers":                        []map[string]any{},
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "91"}}
	c.Request = httptest.NewRequest(http.MethodPut, "/businesses/91/crm/loyalty", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.PutLoyalty(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var saved database.LoyaltyProgram
	require.NoError(t, db.Where("business_id = ?", 91).First(&saved).Error)
	require.Equal(t, 1.25, saved.PointsPerDollar)
	require.Equal(t, 100.0, saved.RedemptionPointsPerDollar)
}

func TestPutLoyaltyHandler_RejectsZeroRedeemWhenEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupLoyaltyHandlerDB(t)
	seedLoyaltyTestBusiness(t, db, 92)

	body, _ := json.Marshal(map[string]any{
		"enabled":                      true,
		"points_per_dollar":            1.25,
		"redemption_points_per_dollar": 0,
		"tiers":                        []map[string]any{},
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "92"}}
	c.Request = httptest.NewRequest(http.MethodPut, "/businesses/92/crm/loyalty", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.PutLoyalty(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPutLoyaltyHandler_RejectsOutOfRangeRedeemRate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupLoyaltyHandlerDB(t)
	seedLoyaltyTestBusiness(t, db, 93)

	body, _ := json.Marshal(map[string]any{
		"enabled":                      true,
		"points_per_dollar":            1,
		"redemption_points_per_dollar": 50000,
		"tiers":                        []map[string]any{},
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "93"}}
	c.Request = httptest.NewRequest(http.MethodPut, "/businesses/93/crm/loyalty", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.PutLoyalty(c)
	require.Equal(t, http.StatusBadRequest, w.Code)
}
