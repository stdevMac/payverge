package demo

// #905: the live demo venue advertised delivery hours of `mon 11–22` while the
// business itself is open 11–23 every day. The seeded zone constant has covered
// the full week since #714 — but the seed and its self-heal are keyed on the
// zone NAME ("CABA"), and the pre-v8 demo seeded its zone as "Downtown" with
// `{"mon":"11:00-22:00"}` hours, a USD 4.99 fee and NY boundaries. On any venue
// seeded before the rename the ensure pass simply created a second zone next to
// the legacy one and healed only the new row, leaving the stale Monday-only
// zone active, quoting, and visible in Delivery settings.
//
// A demo venue has exactly one delivery zone. These tests pin that: the legacy
// row is adopted by rename (so delivery orders keep their zone reference) and
// any surplus zone is retired.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func demoZones(t *testing.T, db *gorm.DB, businessID uint) []database.DeliveryZone {
	t.Helper()
	var zones []database.DeliveryZone
	require.NoError(t, db.Where("business_id = ?", businessID).Order("id ASC").Find(&zones).Error)
	return zones
}

// requireCanonicalDemoZone asserts the single surviving zone carries the
// showroom's own truth: full-week hours, CABA boundaries, ARS money.
func requireCanonicalDemoZone(t *testing.T, db *gorm.DB, businessID uint) database.DeliveryZone {
	t.Helper()
	zones := demoZones(t, db, businessID)
	require.Lenf(t, zones, 1, "business %d must keep exactly one delivery zone, got %d", businessID, len(zones))
	zone := zones[0]
	require.Equal(t, demoZoneName, zone.Name, "business %d: surviving zone must be the canonical demo zone", businessID)
	require.True(t, zone.IsActive, "business %d: canonical zone must stay active", businessID)
	require.Truef(t, zoneHoursCoverFullWeek(zone.OperatingHours),
		"business %d: zone hours must cover the full week like the business hours, got %q", businessID, zone.OperatingHours)
	require.Falsef(t, zoneHoursMondayOnly(zone.OperatingHours),
		"business %d: zone still advertises Monday-only delivery: %q", businessID, zone.OperatingHours)
	require.Truef(t, demoZoneCoversCABADemoPostals(zone.Boundaries),
		"business %d: zone boundaries must cover the CABA demo postals, got %q", businessID, zone.Boundaries)
	require.EqualValuesf(t, demoZoneDeliveryFeeCents, zone.DeliveryFee,
		"business %d: zone must quote the seeded AR$2.900 envío, not a legacy USD fee", businessID)
	require.EqualValues(t, demoZoneMinimumOrderCents, zone.MinimumOrderAmount)
	require.Equal(t, demoZoneEstimatedMinutes, zone.EstimatedTime)
	require.Equal(t, demoZoneDescription, zone.Description)
	return zone
}

// A venue seeded before the CABA rename keeps its legacy zone row. The ensure
// must adopt that row instead of stacking a second zone beside it, and the
// adopted row must come out fully canonical.
func TestEnsureAdoptsLegacyDemoZoneInsteadOfStackingASecondOne(t *testing.T) {
	db := newDemoServiceTestDB(t)
	database.InitTestDB(db)
	admin := seedAdmin(t, db, "demo-zone-legacy-adopt@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 5})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	businesses := demoBusinesses(t, db, admin.ID)
	legacyZoneIDs := map[uint]uint{}
	for _, business := range businesses {
		zone := requireCanonicalDemoZone(t, db, business.ID)
		legacyZoneIDs[business.ID] = zone.ID
		// Rewind the row to the pre-v8 shape the live US venues still carry.
		require.NoError(t, db.Model(&database.DeliveryZone{}).
			Where("id = ?", zone.ID).
			Updates(map[string]interface{}{
				"name":                 "Downtown",
				"description":          "Primary demo delivery zone",
				"boundaries":           `{"postal_codes":["100*","101*","112*"],"cities":["New York","Brooklyn"]}`,
				"operating_hours":      `{"mon":"11:00-22:00"}`,
				"delivery_fee":         499,
				"minimum_order_amount": 1500,
				"estimated_time":       45,
			}).Error)
	}

	_, err = svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	for _, business := range businesses {
		zone := requireCanonicalDemoZone(t, db, business.ID)
		require.Equalf(t, legacyZoneIDs[business.ID], zone.ID,
			"business %d: the legacy zone must be adopted in place, not replaced (delivery orders reference its id)", business.ID)

		// The settings snapshot the Delivery tab renders must agree.
		var settings database.DeliverySettings
		require.NoError(t, db.Where("business_id = ?", business.ID).First(&settings).Error)
		var snapshot []zoneSnapshotShape
		require.NoError(t, json.Unmarshal([]byte(settings.DeliveryZones), &snapshot))
		require.Lenf(t, snapshot, 1, "business %d: settings blob must list exactly one zone", business.ID)
		require.Equal(t, demoZoneName, snapshot[0].Name)
		require.InDelta(t, 2900.0, snapshot[0].DeliveryFee, 0.001)
	}
}

// When both a legacy zone and the canonical one exist — the state every venue
// that ran the post-rename ensure is in — the surplus row must be retired and
// its delivery orders repointed at the survivor, not orphaned.
func TestEnsureRetiresSurplusDemoZonesAndKeepsOrdersPointed(t *testing.T) {
	db := newDemoServiceTestDB(t)
	database.InitTestDB(db)
	admin := seedAdmin(t, db, "demo-zone-surplus@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 5})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	businesses := demoBusinesses(t, db, admin.ID)
	strandedOrders := map[uint]uint{}
	for _, business := range businesses {
		legacy := database.DeliveryZone{
			BusinessID:     business.ID,
			Name:           "Downtown",
			Description:    "Primary demo delivery zone",
			Boundaries:     `{"type":"Polygon","coordinates":[]}`,
			DeliveryFee:    499,
			EstimatedTime:  45,
			Priority:       2,
			IsActive:       true,
			OperatingHours: `{"mon":"11:00-22:00"}`,
		}
		require.NoError(t, db.Create(&legacy).Error)

		var order database.DeliveryOrder
		if err := db.Where("business_id = ?", business.ID).First(&order).Error; err == nil {
			require.NoError(t, db.Model(&database.DeliveryOrder{}).
				Where("id = ?", order.ID).Update("zone_id", legacy.ID).Error)
			strandedOrders[business.ID] = order.ID
		}
	}

	_, err = svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	for _, business := range businesses {
		zone := requireCanonicalDemoZone(t, db, business.ID)
		orderID, ok := strandedOrders[business.ID]
		if !ok {
			continue
		}
		var order database.DeliveryOrder
		require.NoError(t, db.First(&order, orderID).Error)
		require.NotNilf(t, order.ZoneID, "business %d: retiring a zone must not orphan delivery order %d", business.ID, orderID)
		require.Equalf(t, zone.ID, *order.ZoneID,
			"business %d: delivery order %d must follow the surviving zone", business.ID, orderID)
	}
}
