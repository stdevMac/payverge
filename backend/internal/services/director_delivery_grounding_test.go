package services

import (
	"encoding/json"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #871: on 142 Parrilla Quebracho Azul the operator asked the Director about
// delivery and was told the venue had no delivery partners and should add one
// in Plugins — while Delivery settings already carried PedidosYa and Rappi
// with a fee of ARS 2.900. The Director snapshot had no delivery block at all,
// so the model was answering from the plugin list, where delivery never
// appears. These tests pin the evidence, not the model.

// seedDirectorDeliverySettings writes one delivery_settings row the way the
// Delivery settings screen would.
func seedDirectorDeliverySettings(t *testing.T, db *database.DB, row database.DeliverySettings) {
	t.Helper()
	require.NoError(t, db.GetGorm().Create(&row).Error)
}

// demoParrillaPartnerLinks is the exact shape the venue's settings carry.
const demoParrillaPartnerLinks = `[{"name":"PedidosYa","url":"https://pedidosya.com.ar/quebracho-azul"},{"name":"Rappi","url":"https://rappi.com.ar/quebracho-azul"}]`

func TestLoadDirectorDeliveryConfigReadsConfiguredMarketplacePartners(t *testing.T) {
	db := setupDirectorGroundingTestDB(t)

	seedDirectorDeliverySettings(t, db, database.DeliverySettings{
		BusinessID:             142,
		DeliveryEnabled:        true,
		InHouseDeliveryEnabled: true,
		ThirdPartyEnabled:      true,
		FlatDeliveryFee:        290000, // ARS 2.900 in cents
		MinimumOrderAmount:     1500000,
		DeliveryRadius:         8,
		ExternalPartnerLinks:   database.JSONRawMessage(demoParrillaPartnerLinks),
	})

	cfg := loadDirectorDeliveryConfig(db.GetGorm(), 142)

	require.True(t, cfg.Known, "the settings row exists, so delivery is readable")
	assert.True(t, cfg.Enabled)
	assert.True(t, cfg.InHouseEnabled)
	assert.True(t, cfg.ThirdPartyEnabled)
	assert.Equal(t, []string{"PedidosYa", "Rappi"}, cfg.Partners,
		"the partners the operator configured must reach the model by name")
	// Money wire contract: cents in the DB, dollars (venue currency) on the wire.
	assert.InDelta(t, 2900.0, cfg.FlatFee, 0.001)
	assert.InDelta(t, 15000.0, cfg.MinimumOrder, 0.001)
	assert.InDelta(t, 8.0, cfg.RadiusKm, 0.001)
}

func TestLoadDirectorDeliveryConfigTreatsMissingRowAsConfiguredNoDelivery(t *testing.T) {
	db := setupDirectorGroundingTestDB(t)

	cfg := loadDirectorDeliveryConfig(db.GetGorm(), 999)

	assert.True(t, cfg.Known, "a venue that never configured delivery is a known answer, not an unreadable one")
	assert.False(t, cfg.Enabled)
	assert.Empty(t, cfg.Partners)
}

func TestLoadDirectorDeliveryConfigReportsUnknownWithoutADatabase(t *testing.T) {
	cfg := loadDirectorDeliveryConfig(nil, 142)

	assert.False(t, cfg.Known, "an unreadable config must never be reported as 'no delivery'")
	assert.NotNil(t, cfg.Partners)
}

func TestDirectorDeliveryPartnersUnionsBuiltInTogglesWithoutDuplicating(t *testing.T) {
	partners := directorDeliveryPartners(
		[]byte(`[{"name":"uber eats"},{"name":"Rappi"}]`),
		true,  // Uber Eats toggle — already present as a link, must not repeat
		true,  // DoorDash
		false, // Grubhub
	)

	assert.Equal(t, []string{"uber eats", "Rappi", "DoorDash"}, partners)
}

func TestDirectorDeliveryPartnersSurvivesUnparseableLinks(t *testing.T) {
	partners := directorDeliveryPartners([]byte(`not json`), false, false, false)
	assert.Empty(t, partners, "a malformed links blob must not panic or leak raw JSON into the prompt")
}

// TestDirectorDeliveryProseNamesPartnersAndForbidsThePluginsAnswer is the RED
// proof for the reported answer: the prose must name PedidosYa and Rappi, price
// the fee in ARS, and explicitly block the "you have no delivery partners, go to
// Plugins" reply the model produced.
func TestDirectorDeliveryProseNamesPartnersAndForbidsThePluginsAnswer(t *testing.T) {
	prose := directorDeliveryProse(directorDeliveryConfig{
		Known:             true,
		Enabled:           true,
		InHouseEnabled:    true,
		ThirdPartyEnabled: true,
		Partners:          []string{"PedidosYa", "Rappi"},
		FlatFee:           2900,
		MinimumOrder:      15000,
		RadiusKm:          8,
	}, "ARS")

	assert.Contains(t, prose, "PedidosYa")
	assert.Contains(t, prose, "Rappi")
	assert.Contains(t, prose, "ARS 2900.00", "the fee must be priced in the venue currency, never a bare $")
	assert.NotContains(t, prose, "$")
	assert.Contains(t, prose, "ALREADY configured")
	assert.Contains(t, prose, "Plugins", "the prose must name the wrong answer in order to forbid it")
}

func TestDirectorDeliveryProseDistinguishesOffFromUnreadable(t *testing.T) {
	off := directorDeliveryProse(directorDeliveryConfig{Known: true, Enabled: false}, "USD")
	assert.Contains(t, off, "turned off")

	unknown := directorDeliveryProse(directorDeliveryConfig{}, "USD")
	assert.Contains(t, unknown, "could not be read")
	assert.NotContains(t, unknown, "turned off",
		"an unreadable config must never be stated as 'delivery is off'")
}

func TestDirectorDeliveryProseSaysSoWhenNoPartnerIsConfigured(t *testing.T) {
	prose := directorDeliveryProse(directorDeliveryConfig{
		Known:          true,
		Enabled:        true,
		InHouseEnabled: true,
		FlatFee:        5,
	}, "USD")

	assert.Contains(t, prose, "no marketplace partner is configured")
	assert.NotContains(t, prose, "ALREADY configured")
}

// TestBuildContextGroundsConfiguredDeliveryPartners drives the real
// buildContext: the delivery evidence must ride in the marshalled snapshot the
// model actually reads.
func TestBuildContextGroundsConfiguredDeliveryPartners(t *testing.T) {
	db := setupDirectorGroundingTestDB(t)
	service := NewDirectorConsoleService(db, analytics.NewAnalyticsService(db), nil, nil)

	business := database.Business{
		Name:            "Parrilla Quebracho Azul",
		SettlementAddr:  "s-871-ctx",
		TippingAddr:     "t-871-ctx",
		DefaultCurrency: "ARS",
		DisplayCurrency: "ARS",
		Timezone:        "America/Argentina/Buenos_Aires",
	}
	require.NoError(t, db.GetGorm().Create(&business).Error)

	seedDirectorDeliverySettings(t, db, database.DeliverySettings{
		BusinessID:             business.ID,
		DeliveryEnabled:        true,
		InHouseDeliveryEnabled: true,
		ThirdPartyEnabled:      true,
		FlatDeliveryFee:        290000,
		DeliveryRadius:         8,
		ExternalPartnerLinks:   database.JSONRawMessage(demoParrillaPartnerLinks),
	})

	ctx, err := service.buildContext(DirectorAskRequest{
		BusinessID: business.ID,
		Message:    "¿Tenemos delivery con PedidosYa?",
		Locale:     "es",
		ActiveTab:  "overview",
	}, &business)
	require.NoError(t, err)

	require.True(t, ctx.Delivery.Known)
	assert.True(t, ctx.Delivery.Enabled)
	assert.Equal(t, []string{"PedidosYa", "Rappi"}, ctx.Delivery.Partners)
	assert.InDelta(t, 2900.0, ctx.Delivery.FlatFee, 0.001)
	assert.Contains(t, ctx.DeliverySummary, "PedidosYa")
	assert.Contains(t, ctx.DeliverySummary, "ARS 2900.00")

	blob, err := json.Marshal(ctx)
	require.NoError(t, err)
	assert.Contains(t, string(blob), "PedidosYa", "the partner names must survive into the model payload")
	assert.Contains(t, string(blob), "Rappi")
}

// TestBuildContextReportsNoDeliveryWhenTheVenueNeverConfiguredIt guards the
// other direction: adding delivery grounding must not make every venue claim
// it delivers.
func TestBuildContextReportsNoDeliveryWhenTheVenueNeverConfiguredIt(t *testing.T) {
	db := setupDirectorGroundingTestDB(t)
	service := NewDirectorConsoleService(db, analytics.NewAnalyticsService(db), nil, nil)

	business := database.Business{Name: "Dine-in Only", SettlementAddr: "s-871-none", TippingAddr: "t-871-none"}
	require.NoError(t, db.GetGorm().Create(&business).Error)

	ctx, err := service.buildContext(DirectorAskRequest{
		BusinessID: business.ID,
		Message:    "Do we deliver?",
		Locale:     "en",
	}, &business)
	require.NoError(t, err)

	assert.True(t, ctx.Delivery.Known)
	assert.False(t, ctx.Delivery.Enabled)
	assert.Contains(t, ctx.DeliverySummary, "turned off")
}

// TestDirectorDeliveryGroundingSurvivesTheOutputGuard proves the new keys are
// not scrubbed by the deterministic PII guard — a grounding field the guard
// strips is grounding the model never sees.
func TestDirectorDeliveryGroundingSurvivesTheOutputGuard(t *testing.T) {
	for _, key := range []string{"delivery", "delivery_summary", "partners", "flat_fee", "in_house_enabled"} {
		assert.False(t, directorKeyIsSensitive(key), "%q must not be scrubbed out of the Director payload", key)
	}
}
