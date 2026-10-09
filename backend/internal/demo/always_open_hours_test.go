package demo

// The public demo serves visitors in every timezone. Seeding the venues with
// Buenos Aires 11:00-23:00 hours made ResolveOrderability answer
// business_closed from 02:00 to 14:00 UTC, so half the day no guest could
// order. Demo venues are now open around the clock, and a venue still carrying
// the old seed (a restored snapshot) is healed by the startup ensure.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func requireDemoVenueOpenAroundTheClock(t *testing.T, business database.Business) {
	t.Helper()
	db := database.GetDB()
	loc, err := time.LoadLocation(business.Timezone)
	require.NoError(t, err)
	// Every hour of every weekday in the venue's own timezone, including the
	// 03:00 Buenos Aires slot the old seed refused and both sides of midnight.
	start := time.Date(2026, 7, 5, 0, 0, 0, 0, loc) // a Sunday
	for h := 0; h < 7*24; h++ {
		for _, minute := range []int{0, 59} {
			at := start.Add(time.Duration(h)*time.Hour + time.Duration(minute)*time.Minute)
			require.Truef(t, services.BusinessOpenAt(db, &business, at),
				"business %d must be open at %s", business.ID, at.Format("Mon 15:04 MST"))
		}
	}
}

func TestDemoVenuesAreOpenAroundTheClock(t *testing.T) {
	db := newDemoServiceTestDB(t)
	database.InitTestDB(db)
	admin := seedAdmin(t, db, "demo-always-open@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 3})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	for _, business := range demoBusinesses(t, db, admin.ID) {
		var hours []database.BusinessOperatingHours
		require.NoError(t, db.Where("business_id = ?", business.ID).Find(&hours).Error)
		require.Len(t, hours, 7)
		for _, row := range hours {
			require.False(t, row.IsClosed)
			require.Equal(t, demoOpenTime, row.OpenTime)
			require.Equal(t, demoCloseTime, row.CloseTime)
		}
		require.Equal(t, "America/Argentina/Buenos_Aires", business.Timezone)
		requireDemoVenueOpenAroundTheClock(t, business)

		// Delivery zone hours advertise the same always-open window.
		zones := demoZones(t, db, business.ID)
		require.Len(t, zones, 1)
		var zoneHours map[string]string
		require.NoError(t, json.Unmarshal([]byte(zones[0].OperatingHours), &zoneHours))
		require.Len(t, zoneHours, 7)
		for day, window := range zoneHours {
			require.Equalf(t, demoOpenTime+"-"+demoCloseTime, window, "zone %s window", day)
		}
	}
}

func TestEnsureHealsLegacyDemoHoursButKeepsOperatorEdits(t *testing.T) {
	db := newDemoServiceTestDB(t)
	database.InitTestDB(db)
	admin := seedAdmin(t, db, "demo-hours-heal@example.com")

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-seed", BaselineDays: 3})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	businesses := demoBusinesses(t, db, admin.ID)
	require.GreaterOrEqual(t, len(businesses), 2)
	legacy, edited := businesses[0], businesses[1]

	// Rewind one venue (and its zone) to the pre-1.0 seed a restored snapshot
	// carries.
	require.NoError(t, db.Model(&database.BusinessOperatingHours{}).
		Where("business_id = ?", legacy.ID).
		Updates(map[string]interface{}{"open_time": legacyDemoOpenTime, "close_time": legacyDemoCloseTime}).Error)
	require.NoError(t, db.Model(&database.DeliveryZone{}).
		Where("business_id = ?", legacy.ID).
		Update("operating_hours", `{"mon":"11:00-23:00","tue":"11:00-23:00","wed":"11:00-23:00","thu":"11:00-23:00","fri":"11:00-23:00","sat":"11:00-23:00","sun":"11:00-23:00"}`).Error)
	at3am := time.Date(2026, 7, 7, 3, 0, 0, 0, mustLoadBuenosAires(t))
	require.False(t, services.BusinessOpenAt(db, &legacy, at3am), "precondition: legacy seed is closed at 03:00")

	// The other venue's operator closed Mondays: not the legacy signature.
	require.NoError(t, db.Model(&database.BusinessOperatingHours{}).
		Where("business_id = ?", edited.ID).
		Updates(map[string]interface{}{"open_time": legacyDemoOpenTime, "close_time": legacyDemoCloseTime}).Error)
	require.NoError(t, db.Model(&database.BusinessOperatingHours{}).
		Where("business_id = ? AND day_of_week = ?", edited.ID, 1).
		Update("is_closed", true).Error)

	_, err = svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	requireDemoVenueOpenAroundTheClock(t, legacy)
	zones := demoZones(t, db, legacy.ID)
	require.Len(t, zones, 1)
	require.Equal(t, demoZoneOperatingHours, zones[0].OperatingHours)

	var editedHours []database.BusinessOperatingHours
	require.NoError(t, db.Where("business_id = ?", edited.ID).Order("day_of_week").Find(&editedHours).Error)
	require.Len(t, editedHours, 7)
	for _, row := range editedHours {
		require.Equal(t, legacyDemoOpenTime, row.OpenTime, "operator-edited hours must survive the ensure")
		require.Equal(t, legacyDemoCloseTime, row.CloseTime)
		require.Equal(t, row.DayOfWeek == 1, row.IsClosed)
	}
}

func mustLoadBuenosAires(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	require.NoError(t, err)
	return loc
}
