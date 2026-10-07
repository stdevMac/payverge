package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// A demo venue once showed two different addresses: the /b storefront
// showed "16 Demo Market St" while the /t table home showed
// "18 Demo Market St". Root cause was stale seed DATA (see
// internal/demo TestEnsureRepairsStaleDemoBusinessAddress) — but this test
// pins the code-side invariant that made that diagnosis possible: the two
// public read paths (GetBusinessByCustomURL → publicBusinessProjection and
// GetTableByCodePublic → buildPublicGuestBusinessResponse) must serialize the SAME
// canonical business address columns, byte-for-byte equal, so they can never
// drift apart for one business row (e.g. one path switching to a location
// table or a cached copy).
func TestPublicStorefrontAndTableHomeServeSameAddress(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicGuestTableHandlerTestDB(t)
	services.DisablePricingCacheForTest(t)

	const tableCode = "ADDR1234"
	business := &database.Business{
		BusinessId:          "public-address-consistency",
		Name:                "Address Consistency Lounge",
		CustomURL:           "address-consistency-lounge",
		OwnerAddress:        "0xAddressOwner",
		BusinessPageEnabled: true,
		IsActive:            true,
		Timezone:            "UTC",
		Address: database.BusinessAddress{
			Street:     "18 Demo Market St",
			City:       "New York",
			State:      "NY",
			PostalCode: "10013",
			Country:    "US",
		},
	}
	require.NoError(t, database.GetDB().Create(business).Error)
	require.NoError(t, database.GetDB().Model(business).Updates(map[string]interface{}{
		"business_page_enabled": true,
		"is_active":             true,
	}).Error)
	table := &database.Table{
		BusinessID: business.ID,
		Name:       "Table 3",
		TableCode:  tableCode,
		Capacity:   4,
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)

	// Storefront path: GET /business/:customUrl
	storefrontRec := httptest.NewRecorder()
	storefrontCtx, _ := gin.CreateTestContext(storefrontRec)
	storefrontCtx.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
	storefrontCtx.Request = httptest.NewRequest(http.MethodGet, "/business/"+business.CustomURL, nil)
	GetBusinessByCustomURL(storefrontCtx)
	require.Equal(t, http.StatusOK, storefrontRec.Code, "storefront lookup must succeed: %s", storefrontRec.Body.String())

	var storefrontBody map[string]any
	require.NoError(t, json.Unmarshal(storefrontRec.Body.Bytes(), &storefrontBody))
	storefrontAddress, ok := storefrontBody["address"].(map[string]any)
	require.True(t, ok, "storefront payload must include an address object")

	// Table home path: GET /guest/table/:code
	tableRec := httptest.NewRecorder()
	tableCtx, _ := gin.CreateTestContext(tableRec)
	tableCtx.Params = gin.Params{{Key: "code", Value: tableCode}}
	tableCtx.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/table/%s", tableCode), nil)
	GetTableByCodePublic(tableCtx)
	require.Equal(t, http.StatusOK, tableRec.Code, "table home lookup must succeed: %s", tableRec.Body.String())

	var tableBody map[string]any
	require.NoError(t, json.Unmarshal(tableRec.Body.Bytes(), &tableBody))
	tableBusiness, ok := tableBody["business"].(map[string]any)
	require.True(t, ok, "table home payload must include a business object")
	tableAddress, ok := tableBusiness["address"].(map[string]any)
	require.True(t, ok, "table home business payload must include an address object")

	require.Equal(t, storefrontAddress, tableAddress,
		"storefront (/b via /business/:customUrl) and table home (/t via /guest/table/:code) must serve the same canonical business address")
	require.Equal(t, "18 Demo Market St", storefrontAddress["street"],
		"both public paths must read businesses.street verbatim")
}
