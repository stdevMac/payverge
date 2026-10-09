package demo

// Live-verified regression: the demo seed wrote the delivery zone with GeoJSON
// boundaries (`{"type":"Polygon","coordinates":[]}`), but the real zone matcher
// (services.zoneMatchesAddress) only understands
// `{"postal_codes":[...],"cities":[...]}`. GeoJSON unmarshals to empty
// postal_codes/cities, which matches NOTHING — so on the production demo
// business no address on earth passed the delivery address check and every
// prospect dead-ended at "outside this business's delivery area".
//
// These tests exercise the REAL matcher through the exported QuoteDelivery
// path and assert that:
//   1. a freshly seeded demo zone matches the demo venues' own CABA addresses
//      (Defensa 9148 C1065 San Telmo, Costa Rica 9602 C1414 Palermo),
//   2. the ensure pass self-heals existing rows that still carry the broken
//      GeoJSON boundaries or legacy NY postals (production demo rows must fix
//      themselves without manual SQL), and
//   3. the DeliverySettings.DeliveryZones snapshot blob is rewritten into the
//      DTO shape the settings surface expects (delivery_fee, boundaries,
//      is_active — not the legacy `{"name":...,"fee":...}` stub).
import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

type zoneBoundariesShape struct {
	PostalCodes []string `json:"postal_codes"`
	Cities      []string `json:"cities"`
}

type zoneSnapshotShape struct {
	Name        string              `json:"name"`
	DeliveryFee float64             `json:"delivery_fee"`
	Boundaries  zoneBoundariesShape `json:"boundaries"`
	IsActive    bool                `json:"is_active"`
}

// neutralizeQuoteGates removes the time-of-day and capacity gates from
// QuoteDelivery so the test outcome depends only on zone matching, never on
// the wall clock the test happens to run at.
func neutralizeQuoteGates(t *testing.T, db *gorm.DB, businessID uint) {
	t.Helper()
	// Explicit overnight-equal 24h custom window (00:00–00:00). Empty custom
	// hours fail CLOSED at quote time, and "23:59" ends at 23:59:00 so the last
	// wall-clock minute of the day still rejected quotes.
	require.NoError(t, db.Model(&database.DeliverySettings{}).
		Where("business_id = ?", businessID).
		Updates(map[string]interface{}{
			"delivery_hours_same_as_business": false,
			"delivery_start_time":             "00:00",
			"delivery_end_time":               "00:00",
			"max_concurrent_deliveries":       0,
		}).Error)
}

func demoBusinesses(t *testing.T, db *gorm.DB, adminID uint) []database.Business {
	t.Helper()
	var businesses []database.Business
	require.NoError(t, db.Where("demo_owner_user_id = ?", adminID).Find(&businesses).Error)
	require.NotEmpty(t, businesses)
	return businesses
}

// TestDemoDeliveryZoneMatchesDemoBusinessAddress walks the seeded zone through
// the real services matcher: both demo venues' own CABA addresses must resolve
// to a zone, and a far-away address must not. OrderSubtotal is ARS pesos
// (float64 wire shape); the seeded minimum order is AR$15.000.
func TestDemoDeliveryZoneMatchesDemoBusinessAddress(t *testing.T) {
	db := newDemoServiceTestDB(t)
	database.InitTestDB(db)
	admin := seedAdmin(t, db, "demo-zone-match@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 30})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	deliverySvc := services.NewDeliveryService(db, nil)
	for _, business := range demoBusinesses(t, db, admin.ID) {
		neutralizeQuoteGates(t, db, business.ID)

		for _, addr := range []database.DeliveryAddress{
			// Bodegón Mesa Larga (San Telmo) and Parrilla Quebracho Azul (Palermo).
			{Street: "Defensa 9148", City: "Buenos Aires", State: "CABA", PostalCode: "C1065", Country: "AR"},
			{Street: "Costa Rica 9602", City: "Buenos Aires", State: "CABA", PostalCode: "C1414", Country: "AR"},
		} {
			quote, err := deliverySvc.QuoteDelivery(business.ID, services.DeliveryQuoteRequest{
				OrderSubtotal:   60000,
				DeliveryAddress: addr,
			})
			require.NoError(t, err)
			require.NotEqual(t, "zone_unavailable", quote.ReasonCode,
				"business %d: the demo venues' OWN address %s must be inside the seeded delivery zone", business.ID, addr.PostalCode)
			require.NotNil(t, quote.Zone, "business %d: quote must resolve a zone for %s", business.ID, addr.PostalCode)
			require.True(t, quote.Eligible, "business %d: quote for %s should be eligible, got reason %q (%s)",
				business.ID, addr.PostalCode, quote.ReasonCode, quote.Message)
		}

		// City-only match (no postal code) must also work for CABA walk-ins.
		cityQuote, err := deliverySvc.QuoteDelivery(business.ID, services.DeliveryQuoteRequest{
			OrderSubtotal: 60000,
			DeliveryAddress: database.DeliveryAddress{
				Street: "Av. Corrientes 1500", City: "CABA", Country: "AR",
			},
		})
		require.NoError(t, err)
		require.NotNil(t, cityQuote.Zone, "business %d: CABA city match must resolve a zone", business.ID)

		// Negative control: a Córdoba address stays outside the zone.
		miss, err := deliverySvc.QuoteDelivery(business.ID, services.DeliveryQuoteRequest{
			OrderSubtotal: 60000,
			DeliveryAddress: database.DeliveryAddress{
				Street: "Av. Colón 500", City: "Córdoba", PostalCode: "X5000", Country: "AR",
			},
		})
		require.NoError(t, err)
		require.Equal(t, "zone_unavailable", miss.ReasonCode,
			"business %d: a Córdoba address must NOT match the demo zone", business.ID)
	}
}

func TestDemoDeliveryZoneHoursCoverFullWeek(t *testing.T) {
	db := newDemoServiceTestDB(t)
	database.InitTestDB(db)
	admin := seedAdmin(t, db, "demo-zone-hours@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 30})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	for _, business := range demoBusinesses(t, db, admin.ID) {
		var zone database.DeliveryZone
		require.NoError(t, db.Where("business_id = ?", business.ID).First(&zone).Error)
		require.False(t, zoneHoursMondayOnly(zone.OperatingHours),
			"business %d: seeded zone hours must not be Monday-only, got %q", business.ID, zone.OperatingHours)
		require.Contains(t, zone.OperatingHours, `"wed"`)
		var settings database.DeliverySettings
		require.NoError(t, db.Where("business_id = ?", business.ID).First(&settings).Error)
		require.NotContains(t, settings.DeliveryInstructions, "dispatch board")
		require.True(t, zoneHoursCoverFullWeek(zone.OperatingHours),
			"business %d: seeded zone hours must cover the full week, got %q", business.ID, zone.OperatingHours)
		require.True(t, demoZoneCoversCABADemoPostals(zone.Boundaries),
			"business %d: seeded boundaries must cover C10*/C14* CABA, got %q", business.ID, zone.Boundaries)
	}
}

func TestEnsureSelfHealsMondayOnlyZoneHoursAndEnglishSeedNote(t *testing.T) {
	db := newDemoServiceTestDB(t)
	database.InitTestDB(db)
	admin := seedAdmin(t, db, "demo-zone-hours-heal@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 30})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	for _, business := range demoBusinesses(t, db, admin.ID) {
		require.NoError(t, db.Model(&database.DeliveryZone{}).
			Where("business_id = ?", business.ID).
			Update("operating_hours", `{"mon":"11:00-22:00"}`).Error)
		require.NoError(t, db.Model(&database.DeliverySettings{}).
			Where("business_id = ?", business.ID).
			Update("delivery_instructions", demoOperatorDispatchSeed).Error)
	}

	_, err = svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	for _, business := range demoBusinesses(t, db, admin.ID) {
		var zone database.DeliveryZone
		require.NoError(t, db.Where("business_id = ?", business.ID).First(&zone).Error)
		require.False(t, zoneHoursMondayOnly(zone.OperatingHours),
			"business %d: ensure must heal Monday-only hours, got %q", business.ID, zone.OperatingHours)
		var settings database.DeliverySettings
		require.NoError(t, db.Where("business_id = ?", business.ID).First(&settings).Error)
		require.NotEqual(t, demoOperatorDispatchSeed, settings.DeliveryInstructions)
		require.False(t, isOperatorDispatchSeedNote(settings.DeliveryInstructions))
	}
}

func TestEnsureSelfHealsMatchableButWrongDemoZonePostalsAndPartialHours(t *testing.T) {
	db := newDemoServiceTestDB(t)
	database.InitTestDB(db)
	admin := seedAdmin(t, db, "demo-zone-wrong-postals@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 30})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	// Production-shaped miss: matcher JSON that matches SOMETHING (so the
	// GeoJSON heal would no-op) but not the CABA demo postals — a supplied
	// postal that misses the zone cannot fall back to the city list — plus
	// Tuesday-only hours that the Monday-only detector would also miss.
	for _, business := range demoBusinesses(t, db, admin.ID) {
		require.NoError(t, db.Model(&database.DeliveryZone{}).
			Where("business_id = ?", business.ID).
			Updates(map[string]interface{}{
				"boundaries":      `{"postal_codes":["9999"],"cities":["Buenos Aires"]}`,
				"operating_hours": `{"tue":"11:00-22:00"}`,
			}).Error)
		require.NoError(t, db.Model(&database.DeliverySettings{}).
			Where("business_id = ?", business.ID).
			Update("delivery_instructions", "Use the delivery dispatch board for assignment.").Error)
	}

	_, err = svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	deliverySvc := services.NewDeliveryService(db, nil)
	for _, business := range demoBusinesses(t, db, admin.ID) {
		var zone database.DeliveryZone
		require.NoError(t, db.Where("business_id = ?", business.ID).First(&zone).Error)
		require.True(t, demoZoneCoversCABADemoPostals(zone.Boundaries),
			"business %d: heal must cover C10*/C14*, got %q", business.ID, zone.Boundaries)
		require.True(t, zoneHoursCoverFullWeek(zone.OperatingHours),
			"business %d: heal must expand Tuesday-only hours, got %q", business.ID, zone.OperatingHours)
		var settings database.DeliverySettings
		require.NoError(t, db.Where("business_id = ?", business.ID).First(&settings).Error)
		require.False(t, isOperatorDispatchSeedNote(settings.DeliveryInstructions))

		neutralizeQuoteGates(t, db, business.ID)
		for _, addr := range []database.DeliveryAddress{
			{Street: "Defensa 9148", City: "Buenos Aires", State: "CABA", PostalCode: "C1065", Country: "AR"},
			{Street: "Costa Rica 9602", City: "Buenos Aires", State: "CABA", PostalCode: "C1414", Country: "AR"},
			{Street: "Av. de Mayo 800", City: "Buenos Aires", State: "CABA", PostalCode: "C1084", Country: "AR"},
		} {
			quote, err := deliverySvc.QuoteDelivery(business.ID, services.DeliveryQuoteRequest{
				OrderSubtotal:   60000,
				DeliveryAddress: addr,
			})
			require.NoError(t, err)
			require.True(t, quote.Eligible,
				"business %d: healed zone must quote %s, got %q (%s)",
				business.ID, addr.PostalCode, quote.ReasonCode, quote.Message)
			require.NotEqual(t, "zone_unavailable", quote.ReasonCode)
		}
	}
}

// TestEnsureSelfHealsBrokenDemoZoneBoundaries simulates the production rows
// seeded before the fix: GeoJSON zone boundaries plus the legacy settings
// blob. A re-run of the ensure pass must rewrite both in place.
func TestEnsureSelfHealsBrokenDemoZoneBoundaries(t *testing.T) {
	db := newDemoServiceTestDB(t)
	database.InitTestDB(db)
	admin := seedAdmin(t, db, "demo-zone-heal@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 30})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	businesses := demoBusinesses(t, db, admin.ID)

	// Corrupt every demo business back to the pre-fix production shape.
	for _, business := range businesses {
		require.NoError(t, db.Model(&database.DeliveryZone{}).
			Where("business_id = ?", business.ID).
			Update("boundaries", `{"type":"Polygon","coordinates":[]}`).Error)
		require.NoError(t, db.Model(&database.DeliverySettings{}).
			Where("business_id = ?", business.ID).
			Update("delivery_zones", database.JSONRawMessage(`[{"name":"Downtown","fee":4.99}]`)).Error)
	}

	// The next ensure pass must heal the stored rows without manual SQL.
	_, err = svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	deliverySvc := services.NewDeliveryService(db, nil)
	for _, business := range businesses {
		var zone database.DeliveryZone
		require.NoError(t, db.Where("business_id = ?", business.ID).First(&zone).Error)

		var boundaries zoneBoundariesShape
		require.NoError(t, json.Unmarshal([]byte(zone.Boundaries), &boundaries),
			"business %d: healed boundaries must be matcher JSON, got %q", business.ID, zone.Boundaries)
		require.NotEmpty(t, boundaries.PostalCodes,
			"business %d: healed boundaries must carry postal codes", business.ID)
		require.Contains(t, boundaries.PostalCodes, "C10*",
			"business %d: healed boundaries must cover the San Telmo C10xx prefix", business.ID)
		require.Contains(t, boundaries.Cities, "Buenos Aires",
			"business %d: healed boundaries must cover the demo city", business.ID)

		// The settings snapshot blob must be rewritten into the DTO shape,
		// resynced from the healed zone row (legacy Downtown stub replaced).
		var settings database.DeliverySettings
		require.NoError(t, db.Where("business_id = ?", business.ID).First(&settings).Error)
		var snapshot []zoneSnapshotShape
		require.NoError(t, json.Unmarshal([]byte(settings.DeliveryZones), &snapshot),
			"business %d: delivery_zones blob must decode as a zone DTO array", business.ID)
		require.Len(t, snapshot, 1)
		require.Equal(t, "CABA", snapshot[0].Name)
		require.InDelta(t, 2900.0, snapshot[0].DeliveryFee, 0.001,
			"business %d: blob must carry delivery_fee in ARS pesos (AR$2.900)", business.ID)
		require.True(t, snapshot[0].IsActive)
		require.NotEmpty(t, snapshot[0].Boundaries.PostalCodes,
			"business %d: blob boundaries must be matcher-compatible", business.ID)

		// End-to-end: after healing, the printed demo address quotes again.
		neutralizeQuoteGates(t, db, business.ID)
		quote, err := deliverySvc.QuoteDelivery(business.ID, services.DeliveryQuoteRequest{
			OrderSubtotal: 60000,
			DeliveryAddress: database.DeliveryAddress{
				Street: "Defensa 9148", City: "Buenos Aires", State: "CABA",
				PostalCode: "C1065", Country: "AR",
			},
		})
		require.NoError(t, err)
		require.True(t, quote.Eligible,
			"business %d: healed zone must quote the demo address, got %q (%s)",
			business.ID, quote.ReasonCode, quote.Message)
	}
}
