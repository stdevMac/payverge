package services

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestGetDeliverySettingsDTO_PublicOmitsOpsFields pins #292: unauthenticated
// delivery-settings must not leak capacity, auto-assign, dispatch instructions,
// or online-payment availability.
func TestGetDeliverySettingsDTO_PublicOmitsOpsFields(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	require.NoError(t, svc.db.Where("business_id = ?", businessID).Delete(&database.DeliverySettings{}).Error)
	settings := database.DeliverySettings{
		BusinessID:              businessID,
		DeliveryEnabled:         true,
		InHouseDeliveryEnabled:  true,
		MaxConcurrentDeliveries: 7,
		AutoAssignDrivers:       true,
		DeliveryInstructions:    "Use the dispatch board; do not share with guests.",
		PaymentMode:             string(database.DeliveryPaymentOnline),
		ExternalPartnerLinks:    database.JSONRawMessage(`[]`),
		DeliveryZones:           database.JSONRawMessage(`[]`),
	}
	require.NoError(t, svc.db.Create(&settings).Error)

	zone := database.DeliveryZone{
		BusinessID:          businessID,
		Name:                "Downtown",
		DeliveryFee:         299,
		MinimumOrderAmount:  1500,
		EstimatedTime:       25,
		Priority:            3,
		CutoffBufferMinutes: 15,
		IsActive:            true,
		Boundaries:          `{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,0]]]}`,
	}
	require.NoError(t, svc.db.Create(&zone).Error)

	publicDTO, err := svc.GetDeliverySettingsDTO(businessID, true)
	require.NoError(t, err)

	raw, err := json.Marshal(publicDTO)
	require.NoError(t, err)
	var publicMap map[string]any
	require.NoError(t, json.Unmarshal(raw, &publicMap))

	assert.NotContains(t, publicMap, "auto_assign_drivers")
	assert.NotContains(t, publicMap, "max_concurrent_deliveries")
	assert.NotContains(t, publicMap, "online_payment_available")
	assert.NotContains(t, publicMap, "live_order_count")
	assert.NotContains(t, publicMap, "delivery_instructions")
	assert.Equal(t, true, publicMap["delivery_enabled"])
	assert.Equal(t, false, publicMap["in_house_delivery_enabled"],
		"GeoJSON-only zones cannot match an address; do not advertise in-house delivery")
	assert.NotEmpty(t, publicMap["payment_mode"])

	zones, ok := publicMap["zones"].([]any)
	require.True(t, ok)
	require.Len(t, zones, 1)
	zoneMap := zones[0].(map[string]any)
	assert.NotContains(t, zoneMap, "boundaries")
	assert.Equal(t, float64(0), zoneMap["priority"])
	assert.Equal(t, float64(0), zoneMap["cutoff_buffer_minutes"])

	operatorDTO, err := svc.GetDeliverySettingsDTO(businessID, false)
	require.NoError(t, err)
	assert.True(t, operatorDTO.AutoAssignDrivers)
	assert.True(t, operatorDTO.InHouseDeliveryEnabled,
		"operator settings keep the stored in-house flag; only the public projection hides it")
	assert.Equal(t, 7, operatorDTO.MaxConcurrentDeliveries)
	assert.Equal(t, "Use the dispatch board; do not share with guests.", operatorDTO.DeliveryInstructions)
	require.Len(t, operatorDTO.Zones, 1)
	assert.NotEmpty(t, operatorDTO.Zones[0].Boundaries)
	assert.Equal(t, 3, operatorDTO.Zones[0].Priority)
}

func TestGetDeliverySettingsDTO_PublicKeepsInHouseWhenZoneIsMatchable(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	require.NoError(t, svc.db.Where("business_id = ?", businessID).Delete(&database.DeliverySettings{}).Error)
	require.NoError(t, svc.db.Where("business_id = ?", businessID).Delete(&database.DeliveryZone{}).Error)
	require.NoError(t, svc.db.Create(&database.DeliverySettings{
		BusinessID:             businessID,
		DeliveryEnabled:        true,
		InHouseDeliveryEnabled: true,
		ExternalPartnerLinks:   database.JSONRawMessage(`[]`),
		DeliveryZones:          database.JSONRawMessage(`[]`),
	}).Error)
	require.NoError(t, svc.db.Create(&database.DeliveryZone{
		BusinessID:    businessID,
		Name:          "Downtown",
		DeliveryFee:   499,
		EstimatedTime: 32,
		IsActive:      true,
		Boundaries:    `{"postal_codes":["100*","101*"],"cities":["New York"]}`,
	}).Error)

	publicDTO, err := svc.GetDeliverySettingsDTO(businessID, true)
	require.NoError(t, err)
	assert.True(t, publicDTO.InHouseDeliveryEnabled)
}

func TestGetDeliverySettingsDTO_PublicClosedDisablesInHouse(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	require.NoError(t, svc.db.Model(&database.Business{}).Where("id = ?", businessID).
		Update("closed_at", time.Now().Add(-time.Hour)).Error)
	require.NoError(t, svc.db.Where("business_id = ?", businessID).Delete(&database.DeliverySettings{}).Error)
	settings := database.DeliverySettings{
		BusinessID:             businessID,
		DeliveryEnabled:        true,
		InHouseDeliveryEnabled: true,
		ThirdPartyEnabled:      true,
		ExternalPartnerLinks:   database.JSONRawMessage(`[]`),
		DeliveryZones:          database.JSONRawMessage(`[]`),
	}
	require.NoError(t, svc.db.Create(&settings).Error)

	publicDTO, err := svc.GetDeliverySettingsDTO(businessID, true)
	require.NoError(t, err)
	assert.True(t, publicDTO.DeliveryEnabled)
	assert.False(t, publicDTO.InHouseDeliveryEnabled)

	operatorDTO, err := svc.GetDeliverySettingsDTO(businessID, false)
	require.NoError(t, err)
	assert.True(t, operatorDTO.InHouseDeliveryEnabled)
}

const (
	publicDeliveryUberKey     = "ueats_live_SECRET_do_not_select"
	publicDeliveryDoorDashKey = "dd_live_SECRET_do_not_select"
	publicDeliveryGrubhubKey  = "gh_live_SECRET_do_not_select"
	publicDeliveryGeoJSON     = `{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,1],[0,0],[0,0],[1,0],[1,1],[0,1],[0,0]]]}`
)

// TestGetDeliverySettingsDTO_PublicAccessShape pins #567: the unauthenticated
// delivery-settings read must project its three aggregates instead of hydrating
// full rows (GeoJSON boundaries that were then nilled, partner API keys, and
// the ~140-column business row) and must not pull plugin features/config_schema
// just to resolve payment_mode.
func TestGetDeliverySettingsDTO_PublicAccessShape(t *testing.T) {
	svc, businessID := newDeliveryTestService(t)
	rec := &sqlRecorder{}
	logged := svc.db.Session(&gorm.Session{Logger: rec})
	svc.db = logged
	database.InitTestDB(logged)

	require.NoError(t, logged.AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))
	require.NoError(t, logged.Model(&database.Business{}).Where("id = ?", businessID).Updates(map[string]any{
		"onboarding_state": `{"secret":"do-not-hydrate"}`,
		"settlement_addr":  "",
	}).Error)

	require.NoError(t, logged.Where("business_id = ?", businessID).Delete(&database.DeliverySettings{}).Error)
	settings := database.DeliverySettings{
		BusinessID:              businessID,
		DeliveryEnabled:         true,
		InHouseDeliveryEnabled:  true,
		ThirdPartyEnabled:       true,
		MaxConcurrentDeliveries: 9,
		AutoAssignDrivers:       true,
		DeliveryInstructions:    "Dispatch-only: do not leak.",
		PaymentMode:             string(database.DeliveryPaymentOnline),
		UberEatsAPIKey:          publicDeliveryUberKey,
		DoordashAPIKey:          publicDeliveryDoorDashKey,
		GrubhubAPIKey:           publicDeliveryGrubhubKey,
		ExternalPartnerLinks:    database.JSONRawMessage(`[{"name":"Uber Eats","url":"https://ubereats.com","provider_key":"ubereats"}]`),
		DeliveryZones:           database.JSONRawMessage(`[{"name":"legacy-snapshot","boundaries":` + publicDeliveryGeoJSON + `}]`),
	}
	require.NoError(t, logged.Create(&settings).Error)

	require.NoError(t, logged.Create(&database.DeliveryZone{
		BusinessID:          businessID,
		Name:                "Downtown",
		DeliveryFee:         299,
		MinimumOrderAmount:  1500,
		EstimatedTime:       25,
		Priority:            3,
		CutoffBufferMinutes: 15,
		IsActive:            true,
		Boundaries:          publicDeliveryGeoJSON,
		OperatingHours:      `{"mon":"10-22"}`,
	}).Error)
	inactive := database.DeliveryZone{
		BusinessID: businessID,
		Name:       "Inactive suburb",
		IsActive:   false,
		Boundaries: `{"type":"Polygon","coordinates":[[[9,9],[8,8],[7,7],[9,9]]]}`,
	}
	require.NoError(t, logged.Create(&inactive).Error)
	require.NoError(t, logged.Model(&inactive).UpdateColumn("is_active", false).Error)

	plugin := database.Plugin{
		Name:         "stripe-delivery-shape",
		DisplayName:  "Stripe",
		Category:     "payment",
		IsActive:     true,
		Features:     `["card","apple_pay","secret_feature"]`,
		ConfigSchema: `{"secret_key":{"type":"string"},"webhook_secret":{"type":"string"}}`,
	}
	require.NoError(t, logged.Create(&plugin).Error)
	require.NoError(t, logged.Create(&database.BusinessPlugin{
		BusinessID: businessID,
		PluginID:   plugin.ID,
		IsEnabled:  true,
		Config:     `{"secret_key":"sk_live_DO_NOT_HYDRATE"}`,
	}).Error)

	rec.sqls = nil
	publicDTO, err := svc.GetDeliverySettingsDTO(businessID, true)
	require.NoError(t, err)
	require.NotNil(t, publicDTO)

	assert.False(t, queriesSelectStarFrom(rec.sqls, "businesses"),
		"public delivery-settings must not SELECT * FROM businesses; queries: %v", rec.sqls)
	assert.False(t, queriesSelectStarFrom(rec.sqls, "delivery_settings"),
		"public delivery-settings must not SELECT * FROM delivery_settings; queries: %v", rec.sqls)
	assert.False(t, queriesSelectStarFrom(rec.sqls, "delivery_zones"),
		"public delivery-settings must not SELECT * FROM delivery_zones; queries: %v", rec.sqls)
	// Guest zone list still omits boundaries. A separate coverage probe may
	// read matcher JSON server-side so in-house is not advertised when no
	// address can match — those bytes must never appear in the DTO.
	for _, secretCol := range []string{"uber_eats_api_key", "doordash_api_key", "grubhub_api_key"} {
		assert.False(t, queriesSelectListMention(rec.sqls, "delivery_settings", secretCol),
			"public settings query must not select %s; queries: %v", secretCol, rec.sqls)
	}
	assert.False(t, queriesSelectListMention(rec.sqls, "delivery_settings", "delivery_zones"),
		"public settings query must not hydrate the unused delivery_zones JSON snapshot; queries: %v", rec.sqls)
	assert.False(t, anyQueryMentions(rec.sqls, "config_schema"),
		"public payment-mode probe must not read plugin config_schema; queries: %v", rec.sqls)
	assert.False(t, anyQueryMentions(rec.sqls, "p.features"),
		"public payment-mode probe must not read plugin features; queries: %v", rec.sqls)
	assert.False(t, queriesSelectListMention(rec.sqls, "businesses", "owner_address"),
		"public business loader must not select owner_address; queries: %v", rec.sqls)
	assert.False(t, queriesSelectListMention(rec.sqls, "businesses", "onboarding_state"),
		"public business loader must not select onboarding_state; queries: %v", rec.sqls)

	assert.True(t, queriesHaveLimitOn(rec.sqls, "delivery_zones"),
		"public zones query must be bounded; queries: %v", rec.sqls)

	raw, err := json.Marshal(publicDTO)
	require.NoError(t, err)
	body := string(raw)
	assert.NotContains(t, body, publicDeliveryUberKey)
	assert.NotContains(t, body, publicDeliveryDoorDashKey)
	assert.NotContains(t, body, publicDeliveryGrubhubKey)
	assert.NotContains(t, body, "uber_eats_api_key")
	assert.NotContains(t, body, "doordash_api_key")
	assert.NotContains(t, body, "grubhub_api_key")
	assert.NotContains(t, body, "sk_live_DO_NOT_HYDRATE")

	var publicMap map[string]any
	require.NoError(t, json.Unmarshal(raw, &publicMap))
	assert.Equal(t, true, publicMap["delivery_enabled"])
	assert.Equal(t, string(database.DeliveryPaymentOnline), publicMap["payment_mode"])
	zones, ok := publicMap["zones"].([]any)
	require.True(t, ok)
	require.Len(t, zones, 1)
	zoneMap := zones[0].(map[string]any)
	assert.NotContains(t, zoneMap, "boundaries")
	assert.NotContains(t, body, `"coordinates"`)
	assert.Equal(t, false, publicMap["in_house_delivery_enabled"],
		"GeoJSON-only Downtown zone must not advertise in-house delivery")
	assert.Equal(t, "Downtown", zoneMap["name"])
	assert.Equal(t, float64(0), zoneMap["priority"])
	assert.Equal(t, float64(0), zoneMap["cutoff_buffer_minutes"])
}

func queriesSelectStarFrom(sqls []string, table string) bool {
	for _, sql := range sqls {
		normalized := strings.ToLower(strings.Join(strings.Fields(sql), " "))
		if !strings.HasPrefix(normalized, "select *") {
			continue
		}
		if strings.Contains(normalized, "from `"+table+"`") ||
			strings.Contains(normalized, `from "`+table+`"`) ||
			strings.Contains(normalized, "from "+table+" ") ||
			strings.HasSuffix(normalized, "from "+table) {
			return true
		}
	}
	return false
}

func queriesSelectListMention(sqls []string, table, column string) bool {
	col := strings.ToLower(column)
	table = strings.ToLower(table)
	for _, sql := range sqls {
		normalized := strings.ToLower(strings.Join(strings.Fields(sql), " "))
		if !strings.HasPrefix(normalized, "select ") {
			continue
		}
		from := strings.Index(normalized, " from ")
		if from < 0 {
			continue
		}
		fromClause := normalized[from:]
		if !strings.Contains(fromClause, "`"+table+"`") &&
			!strings.Contains(fromClause, `"`+table+`"`) &&
			!strings.Contains(fromClause, " "+table+" ") &&
			!strings.Contains(fromClause, " "+table) {
			continue
		}
		selectList := normalized[:from]
		if strings.Contains(selectList, "`"+col+"`") ||
			strings.Contains(selectList, `"`+col+`"`) ||
			strings.Contains(selectList, "."+col) ||
			strings.Contains(selectList, " "+col+",") ||
			strings.HasSuffix(selectList, " "+col) {
			return true
		}
	}
	return false
}

func anyQueryMentions(sqls []string, needle string) bool {
	n := strings.ToLower(needle)
	for _, sql := range sqls {
		if strings.Contains(strings.ToLower(sql), n) {
			return true
		}
	}
	return false
}

func queriesHaveLimitOn(sqls []string, table string) bool {
	for _, sql := range sqls {
		normalized := strings.ToLower(strings.Join(strings.Fields(sql), " "))
		if !strings.HasPrefix(normalized, "select ") {
			continue
		}
		if !(strings.Contains(normalized, "from `"+table+"`") ||
			strings.Contains(normalized, `from "`+table+`"`) ||
			strings.Contains(normalized, "from "+table+" ") ||
			strings.Contains(normalized, "from "+table)) {
			continue
		}
		if strings.Contains(normalized, " limit ") {
			return true
		}
	}
	return false
}

func setupPublicDeliverySettingsBench(tb testing.TB) (*DeliveryService, uint) {
	tb.Helper()

	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared",
		strings.NewReplacer("/", "_", " ", "_").Replace(tb.Name()), time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(tb, err)
	sqlDB, err := db.DB()
	require.NoError(tb, err)
	sqlDB.SetMaxOpenConns(1)
	tb.Cleanup(func() { _ = sqlDB.Close() })

	database.InitTestDB(db)
	require.NoError(tb, db.AutoMigrate(
		&database.Business{},
		&database.DeliverySettings{},
		&database.DeliveryZone{},
		&database.Plugin{},
		&database.BusinessPlugin{},
	))

	business := &database.Business{
		BusinessId:          fmt.Sprintf("pub-deliv-bench-%d", time.Now().UnixNano()),
		OwnerAddress:        fmt.Sprintf("owner-pub-deliv-%d", time.Now().UnixNano()),
		Name:                "Public Delivery Bench",
		SettlementAddr:      "0xsettlement",
		Timezone:            "UTC",
		IsActive:            true,
		BusinessPageEnabled: true,
		OnboardingState:     database.JSONRawMessage(`{"secret":"do-not-hydrate"}`),
	}
	require.NoError(tb, db.Create(business).Error)

	settings := database.DeliverySettings{
		BusinessID:             business.ID,
		DeliveryEnabled:        true,
		InHouseDeliveryEnabled: true,
		PaymentMode:            string(database.DeliveryPaymentOnline),
		UberEatsAPIKey:         publicDeliveryUberKey,
		DoordashAPIKey:         publicDeliveryDoorDashKey,
		GrubhubAPIKey:          publicDeliveryGrubhubKey,
		ExternalPartnerLinks:   database.JSONRawMessage(`[{"name":"Uber Eats","url":"https://ubereats.com","provider_key":"ubereats"}]`),
		DeliveryZones:          database.JSONRawMessage(`[{"name":"legacy-snapshot","boundaries":` + publicDeliveryGeoJSON + `}]`),
	}
	require.NoError(tb, db.Create(&settings).Error)
	for i := 0; i < 8; i++ {
		require.NoError(tb, db.Create(&database.DeliveryZone{
			BusinessID:         business.ID,
			Name:               fmt.Sprintf("Zone %d", i),
			DeliveryFee:        299,
			MinimumOrderAmount: 1500,
			EstimatedTime:      20 + i,
			Priority:           i,
			IsActive:           true,
			Boundaries:         publicDeliveryGeoJSON,
		}).Error)
	}

	return NewDeliveryService(db, nil), business.ID
}

func BenchmarkGetDeliverySettingsDTOPublicSQLite(b *testing.B) {
	svc, businessID := setupPublicDeliverySettingsBench(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dto, err := svc.GetDeliverySettingsDTO(businessID, true)
		if err != nil {
			b.Fatal(err)
		}
		if dto == nil || len(dto.Zones) == 0 {
			b.Fatal("expected public zones")
		}
	}
}
