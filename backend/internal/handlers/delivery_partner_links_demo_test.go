package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/demomode"
	"github.com/stdevmac/payverge/backend/internal/services"
)

const storedDeliveryPartnerLinks = `[{"name":"DoorDash","provider_key":"doordash","url":"https://www.doordash.com/store/y"}]`

const changedDeliveryPartnerLinks = `{"external_partner_links":[{"name":"Uber Eats","provider_key":"ubereats","url":"https://www.ubereats.com/store/x"}]}`

func putDeliverySettings(t *testing.T, businessID uint, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", businessID)}}
	c.Request = httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("token_type", "staff")
	NewDeliveryHandler(services.NewDeliveryService(database.GetDB(), nil)).UpdateDeliverySettings(c)
	return w
}

func seedDeliveryPartnerLinks(t *testing.T, businessID uint) {
	t.Helper()
	require.NoError(t, database.GetDB().AutoMigrate(&database.DeliverySettings{}, &database.DeliveryZone{}))
	require.NoError(t, database.GetDB().Create(&database.DeliverySettings{
		BusinessID:                  businessID,
		DeliveryHoursSameAsBusiness: true,
		EstimatedPrepTime:           30,
		ExternalPartnerLinks:        database.JSONRawMessage(storedDeliveryPartnerLinks),
	}).Error)
}

func deliveryPartnerLinksJSON(t *testing.T, businessID uint) string {
	t.Helper()
	var row database.DeliverySettings
	require.NoError(t, database.GetDB().Where("business_id = ?", businessID).First(&row).Error)
	return string(row.ExternalPartnerLinks)
}

func TestUpdateDeliverySettings_DemoModeFreezesPartnerLinks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupHandlerTestDB(t)
	config.SetDemoModeForTesting(t, true)
	biz := createTestBusiness(t)
	seedDeliveryPartnerLinks(t, biz.ID)

	changed := putDeliverySettings(t, biz.ID, changedDeliveryPartnerLinks)
	require.Equal(t, http.StatusForbidden, changed.Code, changed.Body.String())
	assert.Contains(t, changed.Body.String(), demomode.ErrorCode)
	assert.Contains(t, changed.Body.String(), string(demomode.KindStorefront))
	stored := deliveryPartnerLinksJSON(t, biz.ID)
	assert.Contains(t, stored, "https://www.doordash.com/store/y")
	assert.NotContains(t, stored, "ubereats.com")

	unchanged := putDeliverySettings(t, biz.ID, `{"external_partner_links":`+storedDeliveryPartnerLinks+`}`)
	require.Equal(t, http.StatusOK, unchanged.Code, unchanged.Body.String())
	assert.Contains(t, deliveryPartnerLinksJSON(t, biz.ID), "https://www.doordash.com/store/y")

	omitted := putDeliverySettings(t, biz.ID, `{"estimated_prep_time":25}`)
	require.Equal(t, http.StatusOK, omitted.Code, omitted.Body.String())
	assert.Contains(t, deliveryPartnerLinksJSON(t, biz.ID), "https://www.doordash.com/store/y")
	assert.NotContains(t, deliveryPartnerLinksJSON(t, biz.ID), "ubereats.com")
}

func TestUpdateDeliverySettings_DemoModeOffSavesPartnerLinks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupHandlerTestDB(t)
	config.SetDemoModeForTesting(t, false)
	biz := createTestBusiness(t)
	seedDeliveryPartnerLinks(t, biz.ID)

	saved := putDeliverySettings(t, biz.ID, changedDeliveryPartnerLinks)
	require.Equal(t, http.StatusOK, saved.Code, saved.Body.String())
	assert.Contains(t, deliveryPartnerLinksJSON(t, biz.ID), "https://www.ubereats.com/store/x")
	assert.Contains(t, saved.Body.String(), "https://www.ubereats.com/store/x")
}
