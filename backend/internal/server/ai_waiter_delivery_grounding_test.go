package server

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// #903: the guest waiter answered "do you deliver?" with WaiterVisitFacts.DeliveryHint
// — the model's English system-prompt fragment — instead of the venue's real
// Delivery settings. This suite drives the shipped fact builder end to end.

const waiterDeliveryPartnerLinksJSON = `[{"name":"PedidosYa","url":"https://www.pedidosya.com.ar","provider_key":"pedidosya"},` +
	`{"name":"Rappi","url":"https://www.rappi.com.ar","provider_key":"rappi"}]`

func waiterDeliveryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := database.GetDB()
	previousService := GetDeliveryService()
	t.Cleanup(func() {
		database.SetTestDB(previousDB)
		SetDeliveryService(previousService)
	})

	gormDB, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"),
		&gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{}, &database.DeliverySettings{}, &database.DeliveryZone{},
		&database.BusinessOperatingHours{}))
	database.SetTestDB(gormDB)
	SetDeliveryService(services.NewDeliveryService(gormDB, nil))
	return gormDB
}

// waiterDeliveryShowroomBusiness mirrors showroom 142: ARS, Buenos Aires, demo
// (so the subscription gate is open the way the seeded venue's is).
func waiterDeliveryShowroomBusiness(t *testing.T, db *gorm.DB) *database.Business {
	t.Helper()
	business := &database.Business{
		BusinessId:      "waiter-delivery-" + t.Name(),
		Name:            "Parrilla Quebracho Azul",
		IsActive:        true,
		IsDemo:          true,
		DefaultCurrency: "ARS",
		Timezone:        "America/Argentina/Buenos_Aires",
	}
	require.NoError(t, db.Create(business).Error)
	return business
}

func TestWaiterDeliveryFactsGroundOnConfiguredPartners(t *testing.T) {
	db := waiterDeliveryTestDB(t)
	business := waiterDeliveryShowroomBusiness(t, db)
	require.NoError(t, db.Create(&database.DeliverySettings{
		BusinessID:             business.ID,
		DeliveryEnabled:        true,
		InHouseDeliveryEnabled: true,
		ThirdPartyEnabled:      true,
		FlatDeliveryFee:        290000, // ARS 2.900,00 in cents
		ExternalPartnerLinks:   database.JSONRawMessage(waiterDeliveryPartnerLinksJSON),
	}).Error)
	waiterDeliveryMatchableZone(t, db, business.ID)

	facts := waiterVisitFactsFromContext(business, nil, "")

	require.True(t, facts.DeliveryKnown)
	require.True(t, facts.DeliveryEnabled)
	require.True(t, facts.DeliveryInHouse)
	require.Equal(t, []string{"PedidosYa", "Rappi"}, facts.DeliveryPartners)
	// Money: int64 cents in the DB, pre-formatted here in the venue currency.
	require.Equal(t, "ARS 2900.00", facts.DeliveryFeeLabel)

	answer := services.WaiterDeliveryAnswer("es-AR", facts)
	require.Contains(t, answer, "PedidosYa")
	require.Contains(t, answer, "Rappi")
	require.Contains(t, answer, "ARS 2900.00")
	require.NotContains(t, answer, "Please encourage them")
	require.NotContains(t, answer, "Direct guests to")
	require.NotContains(t, answer, "Delivery is ENABLED")
}

func TestWaiterDeliveryFactsMarketplaceOnlyVenueStillDelivers(t *testing.T) {
	db := waiterDeliveryTestDB(t)
	business := waiterDeliveryShowroomBusiness(t, db)
	require.NoError(t, db.Create(&database.DeliverySettings{
		BusinessID:           business.ID,
		DeliveryEnabled:      true,
		ThirdPartyEnabled:    true,
		ExternalPartnerLinks: database.JSONRawMessage(waiterDeliveryPartnerLinksJSON),
	}).Error)

	facts := waiterVisitFactsFromContext(business, nil, "")
	require.True(t, facts.DeliveryKnown)
	require.True(t, facts.DeliveryEnabled)
	require.False(t, facts.DeliveryInHouse)
	require.Equal(t, []string{"PedidosYa", "Rappi"}, facts.DeliveryPartners)

	// The bug: a venue with only marketplace partners used to get the
	// "I don't have delivery details here" punt.
	answer := services.WaiterDeliveryAnswer("es", facts)
	require.Contains(t, answer, "PedidosYa")
	require.Contains(t, answer, "Rappi")
}

func TestWaiterDeliveryFactsIncludeBuiltInMarketplaceToggles(t *testing.T) {
	db := waiterDeliveryTestDB(t)
	business := waiterDeliveryShowroomBusiness(t, db)
	require.NoError(t, db.Create(&database.DeliverySettings{
		BusinessID:           business.ID,
		DeliveryEnabled:      true,
		ThirdPartyEnabled:    true,
		UberEatsEnabled:      true,
		DoordashEnabled:      true,
		ExternalPartnerLinks: database.JSONRawMessage("[]"),
	}).Error)

	facts := waiterVisitFactsFromContext(business, nil, "")
	require.Contains(t, facts.DeliveryPartners, "Uber Eats")
	require.Contains(t, facts.DeliveryPartners, "DoorDash")
}

func TestWaiterDeliveryFactsNoSettingsRowMeansNoDelivery(t *testing.T) {
	db := waiterDeliveryTestDB(t)
	business := waiterDeliveryShowroomBusiness(t, db)

	facts := waiterVisitFactsFromContext(business, nil, "")
	require.True(t, facts.DeliveryKnown, "a missing settings row is a configured answer, not an unreadable one")
	require.False(t, facts.DeliveryEnabled)
	require.Empty(t, facts.DeliveryPartners)

	require.Equal(t, services.WaiterDeliveryAnswer("en", services.WaiterVisitFacts{DeliveryKnown: true}),
		services.WaiterDeliveryAnswer("en", facts))
}

func TestWaiterDeliveryFactsUnknownWhenDeliveryServiceUnset(t *testing.T) {
	db := waiterDeliveryTestDB(t)
	business := waiterDeliveryShowroomBusiness(t, db)
	SetDeliveryService(nil)

	facts := waiterVisitFactsFromContext(business, nil, "")
	require.False(t, facts.DeliveryKnown, "with no delivery service we must not claim the venue does or does not deliver")
	require.False(t, facts.DeliveryEnabled)
}

func TestWaiterDeliveryFactsLapsedSubscriptionHidesInHouseButKeepsPartners(t *testing.T) {
	db := waiterDeliveryTestDB(t)
	// Not a demo showroom and suspended: the in-house checkout path would 402,
	// but the marketplace partners still take the order.
	business := &database.Business{
		BusinessId:      "waiter-delivery-lapsed",
		Name:            "Lapsed Grill",
		IsActive:        true,
		DefaultCurrency: "ARS",
	}
	require.NoError(t, db.Create(business).Error)
	require.NoError(t, db.Create(&database.DeliverySettings{
		BusinessID:             business.ID,
		DeliveryEnabled:        true,
		InHouseDeliveryEnabled: true,
		ThirdPartyEnabled:      true,
		FlatDeliveryFee:        290000,
		ExternalPartnerLinks:   database.JSONRawMessage(waiterDeliveryPartnerLinksJSON),
	}).Error)

	facts := waiterVisitFactsFromContext(business, nil, "")
	require.False(t, facts.DeliveryInHouse)
	require.Equal(t, []string{"PedidosYa", "Rappi"}, facts.DeliveryPartners)

	answer := services.WaiterDeliveryAnswer("es-AR", facts)
	require.Contains(t, answer, "PedidosYa")
	require.NotContains(t, answer, "ARS 2900.00", "lapsed venue must not advertise the in-house fee")
}

// waiterDeliveryMatchableZone seeds the one thing in-house checkout needs to be
// reachable: an active zone whose boundaries can match a guest address.
func waiterDeliveryMatchableZone(t *testing.T, db *gorm.DB, businessID uint) {
	t.Helper()
	require.NoError(t, db.Create(&database.DeliveryZone{
		BusinessID: businessID,
		Name:       "Palermo",
		IsActive:   true,
		Boundaries: `{"postal_codes":["1414","1425"],"cities":["Buenos Aires"]}`,
	}).Error)
}

// #903-B: the canonical public storefront gate (delivery_v1.go) carries TWO
// suppressions — a lapsed subscription, and in-house delivery with no active
// zone that can match an address (#714). The waiter mirrored only the first, so
// a solvent venue whose own storefront hides delivery still had the assistant
// promise "Yes — we deliver. The delivery fee is ARS 2900.00."
func TestWaiterDeliveryFactsHideInHouseWhenNoZoneCanMatchAnAddress(t *testing.T) {
	db := waiterDeliveryTestDB(t)
	business := waiterDeliveryShowroomBusiness(t, db)
	require.NoError(t, db.Create(&database.DeliverySettings{
		BusinessID:             business.ID,
		DeliveryEnabled:        true,
		InHouseDeliveryEnabled: true,
		ThirdPartyEnabled:      true,
		FlatDeliveryFee:        290000,
		ExternalPartnerLinks:   database.JSONRawMessage(waiterDeliveryPartnerLinksJSON),
	}).Error)
	// No zone rows at all: nothing an address can land in.

	facts := waiterVisitFactsFromContext(business, nil, "")

	require.True(t, facts.DeliveryKnown)
	require.True(t, facts.DeliveryEnabled, "the marketplace partners still take the order")
	require.False(t, facts.DeliveryInHouse,
		"the storefront hides in-house delivery with no matchable zone; the waiter must agree")
	require.Empty(t, facts.DeliveryFeeLabel, "no reachable in-house channel means no priced promise")

	answer := services.WaiterDeliveryAnswer("es", facts)
	require.Contains(t, answer, "PedidosYa")
	require.NotContains(t, answer, "2900", "the in-house fee must not be quoted")
}

func TestWaiterDeliveryFactsHideInHouseWhenTheOnlyZoneHasEmptyBoundaries(t *testing.T) {
	db := waiterDeliveryTestDB(t)
	business := waiterDeliveryShowroomBusiness(t, db)
	require.NoError(t, db.Create(&database.DeliverySettings{
		BusinessID:             business.ID,
		DeliveryEnabled:        true,
		InHouseDeliveryEnabled: true,
		ThirdPartyEnabled:      true,
		FlatDeliveryFee:        290000,
		ExternalPartnerLinks:   database.JSONRawMessage(waiterDeliveryPartnerLinksJSON),
	}).Error)
	// A half-finished zone: active, but its boundary set matches nothing.
	require.NoError(t, db.Create(&database.DeliveryZone{
		BusinessID: business.ID,
		Name:       "Draft zone",
		IsActive:   true,
		Boundaries: `{}`,
	}).Error)

	facts := waiterVisitFactsFromContext(business, nil, "")

	require.False(t, facts.DeliveryInHouse,
		"an empty boundary set matches no address, so in-house delivery is not reachable")
	require.Empty(t, facts.DeliveryFeeLabel)
	require.Equal(t, []string{"PedidosYa", "Rappi"}, facts.DeliveryPartners)
}

func TestWaiterDeliveryFactsKeepInHouseWhenAnActiveZoneCanMatch(t *testing.T) {
	db := waiterDeliveryTestDB(t)
	business := waiterDeliveryShowroomBusiness(t, db)
	require.NoError(t, db.Create(&database.DeliverySettings{
		BusinessID:             business.ID,
		DeliveryEnabled:        true,
		InHouseDeliveryEnabled: true,
		FlatDeliveryFee:        290000,
		ExternalPartnerLinks:   database.JSONRawMessage("[]"),
	}).Error)
	waiterDeliveryMatchableZone(t, db, business.ID)

	facts := waiterVisitFactsFromContext(business, nil, "")

	require.True(t, facts.DeliveryInHouse, "a covered venue still delivers in house")
	require.Equal(t, "ARS 2900.00", facts.DeliveryFeeLabel)
	require.Contains(t, services.WaiterDeliveryAnswer("es", facts), "ARS 2900.00")
}
