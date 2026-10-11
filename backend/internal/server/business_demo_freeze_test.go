package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/demomode"
)

func putBusinessForDemoTest(t *testing.T, business *database.Business, body map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", business.OwnerAddress)
	UpdateBusiness(c)
	return w
}

func TestUpdateBusiness_DemoModeFreezesStorefrontIdentityAndLinks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	config.SetDemoModeForTesting(t, true)

	cases := map[string]interface{}{
		"name":                "Phishing Bistro",
		"logo":                "https://evil.example/logo.png",
		"settlement_address":  "0x3333333333333333333333333333333333333333",
		"tipping_address":     "0x4444444444444444444444444444444444444444",
		"custom_url":          "free-crypto",
		"website":             "https://evil.example",
		"social_media":        `{"instagram":"https://evil.example"}`,
		"banner_images":       `["https://evil.example/banner.png"]`,
		"default_qr_logo_url": "https://evil.example/qr.png",
		"phone":               "+1 555 0100",
		"address":             map[string]string{"street": "1 Elsewhere St", "city": "Nowhere"},
	}
	i := 0
	for field, value := range cases {
		i++
		t.Run(field, func(t *testing.T) {
			business := createBusinessHandlerTestBusiness(t, "0xOwnerA", fmt.Sprintf("biz-demo-freeze-%d", i))
			w := putBusinessForDemoTest(t, business, map[string]interface{}{field: value})

			require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
			var body struct {
				Error  string `json:"error"`
				Code   string `json:"code"`
				Params struct {
					Kind string `json:"kind"`
				} `json:"params"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, demomode.ErrorCode, body.Code)
			assert.Equal(t, string(demomode.KindStorefront), body.Params.Kind)
			assert.Contains(t, body.Error, "public demo")

			var persisted database.Business
			require.NoError(t, database.GetDB().First(&persisted, business.ID).Error)
			assert.Equal(t, business.Name, persisted.Name)
			assert.Equal(t, business.CustomURL, persisted.CustomURL)
			assert.Equal(t, business.SettlementAddr, persisted.SettlementAddr)
			assert.Equal(t, business.TippingAddr, persisted.TippingAddr)
			assert.Equal(t, business.Website, persisted.Website)
			assert.Equal(t, business.SocialMedia, persisted.SocialMedia)
			assert.Equal(t, business.BannerImages, persisted.BannerImages)
			assert.Equal(t, business.DefaultQRLogoURL, persisted.DefaultQRLogoURL)
		})
	}
}

// The settings form re-sends every field on save; unchanged identity values
// must not block the editable ones.
func TestUpdateBusiness_DemoModeAllowsOtherSettingsWithUnchangedIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	config.SetDemoModeForTesting(t, true)

	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "biz-demo-freeze-ok")
	w := putBusinessForDemoTest(t, business, map[string]interface{}{
		"name":                        business.Name,
		"custom_url":                  business.CustomURL,
		"settlement_address":          business.SettlementAddr,
		"tipping_address":             business.TippingAddr,
		"default_qr_logo_url":         business.DefaultQRLogoURL,
		"description":                 "Neighbourhood grill",
		"welcome_message":             "Hola",
		"default_qr_foreground_color": "#222222",
		"timezone":                    "America/Argentina/Buenos_Aires",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var persisted database.Business
	require.NoError(t, database.GetDB().First(&persisted, business.ID).Error)
	assert.Equal(t, "Neighbourhood grill", persisted.Description)
	assert.Equal(t, "Hola", persisted.WelcomeMessage)
	assert.Equal(t, "#222222", persisted.DefaultQRForegroundColor)
	assert.Equal(t, "America/Argentina/Buenos_Aires", persisted.Timezone)
	assert.Equal(t, business.Name, persisted.Name)
}

func TestUpdateBusiness_DemoModeCapsStorefrontText(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	config.SetDemoModeForTesting(t, true)

	long := strings.Repeat("é", demoStorefrontTextMaxRunes+1)
	for i, field := range []string{"description", "welcome_message", "about_story", "ai_special_instructions"} {
		business := createBusinessHandlerTestBusiness(t, "0xOwnerA", fmt.Sprintf("biz-demo-text-%d", i))
		w := putBusinessForDemoTest(t, business, map[string]interface{}{field: long})
		require.Equal(t, http.StatusForbidden, w.Code, "%s: %s", field, w.Body.String())
		assert.Contains(t, w.Body.String(), "longer than 500 characters", field)
	}

	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "biz-demo-text-ok")
	w := putBusinessForDemoTest(t, business, map[string]interface{}{
		"description": strings.Repeat("é", demoStorefrontTextMaxRunes),
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

// A long text that is already stored (seeded, or written before the cap)
// must not ride through as "unchanged": the cap applies whenever a capped
// field is sent.
func TestUpdateBusiness_DemoModeCapsUnchangedLongText(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	config.SetDemoModeForTesting(t, true)

	long := strings.Repeat("x", demoStorefrontTextMaxRunes+1)
	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "biz-demo-text-resend")
	require.NoError(t, database.GetDB().Model(business).Update("description", long).Error)
	business.Description = long

	w := putBusinessForDemoTest(t, business, map[string]interface{}{"description": long})
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

	// Other short free-text settings are bounded too.
	w = putBusinessForDemoTest(t, business, map[string]interface{}{"default_qr_text_font": long})
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
}

func TestUpdateBusiness_IdentityStaysEditableWithoutDemoMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	config.SetDemoModeForTesting(t, false)

	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "biz-demo-off")
	w := putBusinessForDemoTest(t, business, map[string]interface{}{
		"name":        "Renamed",
		"website":     "https://example.com",
		"description": strings.Repeat("a", demoStorefrontTextMaxRunes+1),
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var persisted database.Business
	require.NoError(t, database.GetDB().First(&persisted, business.ID).Error)
	assert.Equal(t, "Renamed", persisted.Name)
	assert.Equal(t, "https://example.com", persisted.Website)
}

// Every UpdateBusinessRequest field needs a DEMO_MODE decision: allowlisted
// (editable) or explicitly frozen. A new field that is in neither map fails
// here, and until it gets a decision the handler refuses it whenever set.
func TestDemoBusinessFieldPolicyCoversEveryRequestField(t *testing.T) {
	typ := reflect.TypeOf(UpdateBusinessRequest{})
	seen := map[string]bool{}
	for i := 0; i < typ.NumField(); i++ {
		tag := businessRequestFieldTag(typ.Field(i))
		seen[tag] = true
		_, editable := demoEditableBusinessFields[tag]
		_, frozen := demoFrozenBusinessFields[tag]
		if !editable && !frozen {
			t.Errorf("UpdateBusinessRequest.%s (%q) is neither allowlisted nor frozen for DEMO_MODE; add it to demoEditableBusinessFields or demoFrozenBusinessFields", typ.Field(i).Name, tag)
		}
		if editable && frozen {
			t.Errorf("%q is both allowlisted and frozen", tag)
		}
	}
	for tag := range demoEditableBusinessFields {
		if !seen[tag] {
			t.Errorf("allowlisted %q is not an UpdateBusinessRequest field", tag)
		}
	}
	for tag := range demoFrozenBusinessFields {
		if !seen[tag] {
			t.Errorf("frozen %q is not an UpdateBusinessRequest field", tag)
		}
	}
	for _, tag := range []string{"name", "logo", "address", "phone", "website", "social_media", "banner_images", "custom_url", "settlement_address", "tipping_address", "default_qr_logo_url"} {
		if _, editable := demoEditableBusinessFields[tag]; editable {
			t.Errorf("%q must never be on the DEMO_MODE allowlist", tag)
		}
	}
}

func demoSettingsRequest(t *testing.T, business *database.Business, body interface{}, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", business.OwnerAddress)
	handler(c)
	return w
}

func TestHospitalityAndFeatures_DemoModeCapStorefrontText(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	config.SetDemoModeForTesting(t, true)
	long := strings.Repeat("x", demoStorefrontTextMaxRunes+1)

	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "biz-demo-hosp")
	w := demoSettingsRequest(t, business, map[string]interface{}{"welcome_message": long}, UpdateBusinessHospitalitySettings)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), demomode.ErrorCode)
	w = demoSettingsRequest(t, business, map[string]interface{}{"about_story": long}, UpdateBusinessHospitalitySettings)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	w = demoSettingsRequest(t, business, map[string]interface{}{"welcome_message": "Hola", "show_gallery": true}, UpdateBusinessHospitalitySettings)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Re-sending a long value already stored is refused too (no "unchanged" pass).
	require.NoError(t, database.GetDB().Model(business).Update("welcome_message", long).Error)
	w = demoSettingsRequest(t, business, map[string]interface{}{"welcome_message": long}, UpdateBusinessHospitalitySettings)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

	for _, feature := range []map[string]interface{}{
		{"title": "Patio", "description": long},
		{"title": long},
		{"title": "Patio", "icon": long},
	} {
		w = demoSettingsRequest(t, business, []map[string]interface{}{feature}, UpdateBusinessSpecialFeatures)
		require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	}
	// The staff test DB does not migrate business_special_features, so a
	// short feature reaches the store and fails there; it only must not be
	// refused by the demo cap.
	w = demoSettingsRequest(t, business, []map[string]interface{}{{"title": "Patio", "description": "Shaded"}}, UpdateBusinessSpecialFeatures)
	require.NotEqual(t, http.StatusForbidden, w.Code, w.Body.String())
}

// Table QR codes and the bulk "apply to all" branding write the same outbound
// QR logo URL as default_qr_logo_url; each path refuses a new URL in demo.
func TestTableQRLogoPaths_DemoModeRefuseNewLogoURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTableHandlerTestDB(t)
	config.SetDemoModeForTesting(t, true)
	business := createTableHandlerBusiness(t, "demoqr")
	table := createTableHandlerRecord(t, business.ID, "Patio")
	const evil = "https://evil.example/qr.png"

	asOwner := func(h gin.HandlerFunc) gin.HandlerFunc {
		return func(c *gin.Context) {
			c.Set("address", business.OwnerAddress)
			h(c)
		}
	}
	router := gin.New()
	router.PUT("/inside/businesses/:id/tables/:tableId", asOwner(UpdateTable))
	router.PUT("/inside/tables/:id", asOwner(UpdateTableDetails))
	router.POST("/inside/businesses/:id/tables/qr-branding", asOwner(ApplyQRBrandingToAllTables))

	tableURL := "/inside/businesses/" + business.BusinessId + "/tables/" + strconv.Itoa(int(table.ID))
	detailsURL := "/inside/tables/" + strconv.Itoa(int(table.ID))
	brandingURL := "/inside/businesses/" + business.BusinessId + "/tables/qr-branding"

	refused := []struct{ name, method, path string }{
		{"UpdateTable", http.MethodPut, tableURL},
		{"UpdateTableDetails", http.MethodPut, detailsURL},
		{"ApplyQRBrandingToAllTables", http.MethodPost, brandingURL},
	}
	for _, tc := range refused {
		w := performTableHandlerRequest(t, router, tc.method, tc.path, map[string]any{"qr_logo_url": evil, "qr_foreground_color": "#000000"})
		require.Equal(t, http.StatusForbidden, w.Code, "%s: %s", tc.name, w.Body.String())
		assert.Contains(t, w.Body.String(), demomode.ErrorCode, tc.name)
	}
	reloaded, err := database.GetTableByID(table.ID)
	require.NoError(t, err)
	assert.Empty(t, reloaded.QRLogoURL)
	var persisted database.Business
	require.NoError(t, database.GetDB().First(&persisted, business.ID).Error)
	assert.Empty(t, persisted.DefaultQRLogoURL)

	// Colors and an empty logo stay editable.
	w := performTableHandlerRequest(t, router, http.MethodPut, detailsURL, map[string]any{"qr_logo_url": "", "qr_foreground_color": "#111111"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	w = performTableHandlerRequest(t, router, http.MethodPost, brandingURL, map[string]any{"qr_foreground_color": "#222222"})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

// A visitor must not take the shared public storefront offline; re-sending the
// stored value (the settings form does) still passes.
func TestUpdateBusiness_DemoModeFreezesBusinessPageEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	config.SetDemoModeForTesting(t, true)

	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "biz-demo-freeze-page")
	w := putBusinessForDemoTest(t, business, map[string]interface{}{"business_page_enabled": !business.BusinessPageEnabled})
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())

	var persisted database.Business
	require.NoError(t, database.GetDB().First(&persisted, business.ID).Error)
	assert.Equal(t, business.BusinessPageEnabled, persisted.BusinessPageEnabled)

	w = putBusinessForDemoTest(t, business, map[string]interface{}{"business_page_enabled": business.BusinessPageEnabled})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}
