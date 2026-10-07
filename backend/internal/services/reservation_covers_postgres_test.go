//go:build integration_postgres

package services

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/testperf/genesisdb"
)

// TestReservationCoversConcurrentGuestCreates is the Postgres proof that
// concurrent unassigned guest bookings cannot exceed MaxCoversPerSlot.
// Ten party-of-2 creates race the same slot with a cap of 6; exactly three
// commit and the booked covers stay at or under the cap.
func TestReservationCoversConcurrentGuestCreates(t *testing.T) {
	ctx := context.Background()
	pg, err := genesisdb.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	previous := database.GetDB()
	database.SetTestDB(pg.DB)
	t.Cleanup(func() { database.SetTestDB(previous) })

	sqlDB, err := pg.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(20)

	business := &database.Business{
		BusinessId:     "reservation-covers-pg",
		OwnerAddress:   "0x1111111111111111111111111111111111111111",
		Name:           "Covers Race",
		SettlementAddr: "0x2222222222222222222222222222222222222222",
		TippingAddr:    "0x3333333333333333333333333333333333333333",
		Timezone:       "UTC",
		IsActive:       true,
	}
	require.NoError(t, pg.DB.Create(business).Error)

	// A weekday with no hours row is closed. Seed every day so the 18:00
	// slot is inside a configured window whichever weekday tomorrow is.
	for day := 0; day < 7; day++ {
		require.NoError(t, pg.DB.Create(&database.BusinessOperatingHours{
			BusinessID: business.ID,
			DayOfWeek:  day,
			OpenTime:   "11:00",
			CloseTime:  "23:00",
			IsClosed:   false,
		}).Error)
	}

	for i := 0; i < 8; i++ {
		table := &database.Table{
			BusinessID: business.ID,
			TableCode:  fmt.Sprintf("covers-pg-%d", i),
			Name:       fmt.Sprintf("T%d", i),
			Capacity:   8,
			IsActive:   true,
		}
		require.NoError(t, pg.DB.Create(table).Error)
	}

	settings := &database.ReservationSettings{
		BusinessID:           business.ID,
		Enabled:              true,
		MaxAdvanceDays:       30,
		MinAdvanceMinutes:    0,
		MinPartySize:         1,
		MaxPartySize:         20,
		DefaultDuration:      90,
		SlotIntervalMinutes:  30,
		ServiceBufferMinutes: 0,
		MaxCoversPerSlot:     6,
		ApprovalMode:         database.ReservationApprovalAuto,
		ExternalPartnerLinks: database.JSONRawMessage("[]"),
	}
	require.NoError(t, pg.DB.Create(settings).Error)
	// GORM omits false bools that carry default:true, so the Postgres defaults
	// would turn auto-assign and waitlist back on. Force the zeroes.
	require.NoError(t, pg.DB.Model(settings).UpdateColumns(map[string]any{
		"auto_assign_tables":     false,
		"allow_waitlist":         false,
		"max_covers_per_slot":    6,
		"min_advance_minutes":    0,
		"service_buffer_minutes": 0,
		"default_duration":       90,
		"enabled":                true,
	}).Error)

	var stored database.ReservationSettings
	require.NoError(t, pg.DB.Where("business_id = ?", business.ID).First(&stored).Error)
	require.Equal(t, 6, stored.MaxCoversPerSlot)
	require.False(t, stored.AutoAssignTables)
	require.False(t, stored.AllowWaitlist)

	tomorrow := time.Now().UTC().Add(24 * time.Hour)
	slot := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 18, 0, 0, 0, time.UTC)

	service := NewReservationService(pg.DB)
	const racers = 10
	start := make(chan struct{})
	results := make([]*database.TableReservation, racers)
	errs := make([]error, racers)
	var wg sync.WaitGroup
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i], errs[i] = service.CreateReservation(business.ID, CreateReservationInput{
				CustomerName:    fmt.Sprintf("Guest %d", i),
				CustomerPhone:   "5550000000",
				CustomerEmail:   fmt.Sprintf("guest%d@example.test", i),
				PartySize:       2,
				ReservationTime: slot.Format(time.RFC3339),
			}, "customer", true)
		}(i)
	}
	close(start)
	wg.Wait()

	success := 0
	for i, err := range errs {
		if err == nil {
			success++
			require.NotNil(t, results[i])
			require.Nil(t, results[i].TableID)
			require.Equal(t, 2, results[i].PartySize)
			continue
		}
		require.Truef(t, reservationCreateRefusedForCapacity(err), "racer %d: %v", i, err)
	}
	require.Equal(t, 3, success)
	require.LessOrEqual(t, success*2, 6)

	var rows []database.TableReservation
	require.NoError(t, pg.DB.Where(
		"business_id = ? AND status IN ?",
		business.ID,
		[]string{"pending", "confirmed", "seated"},
	).Find(&rows).Error)
	covers := 0
	for _, row := range rows {
		covers += row.PartySize
		require.Nil(t, row.TableID)
	}
	require.Equal(t, 3, len(rows))
	require.LessOrEqual(t, covers, 6)
	require.Equal(t, 6, covers)
}

// TestReservationCoversConcurrentPartySizeEdits proves UpdateReservation
// rechecks covers under the business lock. Five confirmed party-of-1 bookings
// share a slot capped at 10 covers; all five race to grow to 3. Each
// pre-transaction read sees 4+3 <= 10, but only two edits fit (3+3+1+1+1=9;
// a third gives 11), so exactly two must commit.
func TestReservationCoversConcurrentPartySizeEdits(t *testing.T) {
	ctx := context.Background()
	pg, err := genesisdb.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	previous := database.GetDB()
	database.SetTestDB(pg.DB)
	t.Cleanup(func() { database.SetTestDB(previous) })

	sqlDB, err := pg.DB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(20)

	business := &database.Business{
		BusinessId:     "reservation-covers-edit-pg",
		OwnerAddress:   "0x1111111111111111111111111111111111111111",
		Name:           "Covers Edit Race",
		SettlementAddr: "0x2222222222222222222222222222222222222222",
		TippingAddr:    "0x3333333333333333333333333333333333333333",
		Timezone:       "UTC",
		IsActive:       true,
	}
	require.NoError(t, pg.DB.Create(business).Error)
	for day := 0; day < 7; day++ {
		require.NoError(t, pg.DB.Create(&database.BusinessOperatingHours{
			BusinessID: business.ID,
			DayOfWeek:  day,
			OpenTime:   "11:00",
			CloseTime:  "23:00",
		}).Error)
	}
	for i := 0; i < 8; i++ {
		require.NoError(t, pg.DB.Create(&database.Table{
			BusinessID: business.ID,
			TableCode:  fmt.Sprintf("covers-edit-pg-%d", i),
			Name:       fmt.Sprintf("T%d", i),
			Capacity:   8,
			IsActive:   true,
		}).Error)
	}
	settings := &database.ReservationSettings{
		BusinessID:           business.ID,
		Enabled:              true,
		MaxAdvanceDays:       30,
		MinPartySize:         1,
		MaxPartySize:         20,
		DefaultDuration:      90,
		SlotIntervalMinutes:  30,
		MaxCoversPerSlot:     10,
		ApprovalMode:         database.ReservationApprovalAuto,
		ExternalPartnerLinks: database.JSONRawMessage("[]"),
	}
	require.NoError(t, pg.DB.Create(settings).Error)
	require.NoError(t, pg.DB.Model(settings).UpdateColumns(map[string]any{
		"auto_assign_tables":     false,
		"allow_waitlist":         false,
		"max_covers_per_slot":    10,
		"min_advance_minutes":    0,
		"service_buffer_minutes": 0,
		"default_duration":       90,
		"enabled":                true,
	}).Error)

	tomorrow := time.Now().UTC().Add(24 * time.Hour)
	slot := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 18, 0, 0, 0, time.UTC)

	const racers = 5
	ids := make([]uint, racers)
	for i := 0; i < racers; i++ {
		row := &database.TableReservation{
			BusinessID:       business.ID,
			CustomerName:     fmt.Sprintf("Guest %d", i),
			PartySize:        1,
			ReservationTime:  slot,
			Duration:         90,
			Status:           "confirmed",
			Source:           "staff",
			ConfirmationCode: fmt.Sprintf("COVERS-EDIT-%d", i),
		}
		require.NoError(t, pg.DB.Create(row).Error)
		ids[i] = row.ID
	}

	service := NewReservationService(pg.DB)
	start := make(chan struct{})
	errs := make([]error, racers)
	var wg sync.WaitGroup
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			party := 3
			_, _, errs[i] = service.UpdateReservation(business.ID, ids[i], UpdateReservationInput{
				PartySize: &party,
			}, "staff")
		}(i)
	}
	close(start)
	wg.Wait()

	success := 0
	for i, err := range errs {
		if err == nil {
			success++
			continue
		}
		require.Truef(t, reservationCreateRefusedForCapacity(err), "racer %d: %v", i, err)
	}
	require.Equal(t, 2, success)

	var rows []database.TableReservation
	require.NoError(t, pg.DB.Where("business_id = ? AND status IN ?", business.ID,
		[]string{"pending", "confirmed", "seated"}).Find(&rows).Error)
	covers := 0
	for _, row := range rows {
		covers += row.PartySize
	}
	require.Equal(t, 9, covers)
}

func reservationCreateRefusedForCapacity(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no longer available") || strings.Contains(msg, "no availability")
}
