package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
)

type reservationSQLCaptureLogger struct {
	logger.Interface
	statements []string
}

func (l *reservationSQLCaptureLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	l.statements = append(l.statements, sql)
	if l.Interface != nil {
		l.Interface.Trace(ctx, begin, fc, err)
	}
}

func TestReservationServiceGetAvailabilityIncludesUnavailableReason(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-availability")
	createTestTable(t, db, business.ID, "T1", 4)
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.MaxCoversPerSlot = 4
	})

	target := nextDayAt(18, 0)
	_, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Alice",
		CustomerPhone:   "5551111111",
		CustomerEmail:   "alice@example.com",
		PartySize:       4,
		ReservationTime: target.Format(time.RFC3339),
		Source:          "staff",
	}, "host", false)
	require.NoError(t, err)

	availability, err := service.GetPublicAvailability(publicBusinessForTest(t, business.ID), target, 2)
	require.NoError(t, err)

	targetISO := target.Format(time.RFC3339)
	var matched *ReservationAvailabilitySlotDTO
	for i := range availability.AvailableSlots {
		if availability.AvailableSlots[i].Time == targetISO {
			matched = &availability.AvailableSlots[i]
			break
		}
	}

	require.NotNil(t, matched)
	assert.Equal(t, 0, matched.AvailableTables)
	assert.Equal(t, "covers_limit", matched.ReasonCode)
}

func TestGetAvailabilityDoesNotAdvertiseSlotsBeyondMaxAdvance(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	now := time.Date(2026, 8, 17, 15, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	business := createTestHospitalityBusiness(t, db, "reservation-max-advance")
	createTestTable(t, db, business.ID, "T-max", 4)
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.MaxAdvanceDays = 30
	})

	tooFar := now.AddDate(0, 0, 400)
	availability, err := service.GetPublicAvailability(publicBusinessForTest(t, business.ID), tooFar, 2)
	require.NoError(t, err)
	require.NotEmpty(t, availability.AvailableSlots, "day is still listed so the guest sees why it is closed")
	for _, slot := range availability.AvailableSlots {
		assert.Equal(t, 0, slot.AvailableTables, slot.Time)
		assert.Equal(t, "reservation_max_advance", slot.ReasonCode, slot.Time)
	}

	_, err = service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Far Guest",
		CustomerPhone:   "5550000000",
		CustomerEmail:   "far@example.com",
		PartySize:       2,
		ReservationTime: tooFar.Format(time.RFC3339),
		Source:          "customer",
	}, "customer", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "days in advance")
}

func TestGetTableOptionsIncludesOccupancyAndExcludesItFromRecommendations(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	now := time.Date(2026, 7, 18, 18, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	business := createTestHospitalityBusiness(t, db, "reservation-table-options")
	require.NoError(t, db.Model(business).Update("timezone", "America/New_York").Error)
	occupied := createTestTable(t, db, business.ID, "Window 1", 4)
	free := createTestTable(t, db, business.ID, "Window 2", 4)
	configureReservationSettings(t, business.ID, nil)

	require.NoError(t, db.Create(&database.Bill{
		BusinessID:     business.ID,
		TableID:        occupied.ID,
		BillNumber:     "occupied-window-1",
		Status:         database.BillStatusOpen,
		SettlementAddr: "settlement",
		TippingAddr:    "tipping",
		CreatedAt:      now.Add(-90 * time.Minute),
	}).Error)

	options, err := service.GetTableOptions(business.ID, now.Add(time.Hour), 120, 2, 0)
	require.NoError(t, err)
	byID := make(map[uint]ReservationTableOptionDTO, len(options))
	for _, option := range options {
		byID[option.ID] = option
	}
	require.Equal(t, database.TableOccupancyOccupied, byID[occupied.ID].OccupancyState)
	require.True(t, byID[occupied.ID].RequiresOccupancyOverride)
	require.False(t, byID[occupied.ID].Recommended)
	require.True(t, byID[free.ID].Recommended)
}

func TestCreateReservationAutoAssignmentSkipsOccupiedCandidate(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	now := time.Date(2026, 7, 18, 18, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	business := createTestHospitalityBusiness(t, db, "reservation-autoassign-occupancy")
	occupied := createTestTable(t, db, business.ID, "Small", 2)
	free := createTestTable(t, db, business.ID, "Free", 4)
	configureReservationSettings(t, business.ID, nil)
	require.NoError(t, db.Create(&database.Bill{
		BusinessID: business.ID, TableID: occupied.ID, BillNumber: "occupied-small",
		Status: database.BillStatusOpen, SettlementAddr: "settlement", TippingAddr: "tipping",
		CreatedAt: now.Add(-time.Hour),
	}).Error)

	created, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName: "Walk-in", PartySize: 2,
		ReservationTime: now.Add(time.Hour).Format(time.RFC3339), Source: "staff",
	}, "host", false)
	require.NoError(t, err)
	require.NotNil(t, created.TableID)
	require.Equal(t, free.ID, *created.TableID)
}

func TestCreateReservationAutoAssignmentSkipsUndersizedTable(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	now := time.Date(2026, 8, 19, 18, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	business := createTestHospitalityBusiness(t, db, "reservation-autoassign-capacity")
	table10 := createTestTable(t, db, business.ID, "Table 10", 2)
	fourTop := createTestTable(t, db, business.ID, "Table 2", 4)
	configureReservationSettings(t, business.ID, nil)

	created, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName: "Party of four", PartySize: 4,
		ReservationTime: now.Add(time.Hour).Format(time.RFC3339), Source: "staff",
	}, "host", false)
	require.NoError(t, err)
	require.NotNil(t, created.TableID)
	require.Equal(t, fourTop.ID, *created.TableID)
	require.NotEqual(t, table10.ID, *created.TableID)

	_, err = service.CreateReservation(business.ID, CreateReservationInput{
		TableID: &table10.ID, CustomerName: "Still a four-top", PartySize: 4,
		ReservationTime: now.Add(2 * time.Hour).Format(time.RFC3339), Source: "staff",
	}, "host", false)
	require.Error(t, err)
	require.NotContains(t, strings.ToLower(err.Error()), "sql")
}

func TestCreateReservationAutoAssignmentHonorsLayoutSeatCount(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	now := time.Date(2026, 8, 19, 18, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	business := createTestHospitalityBusiness(t, db, "reservation-layout-capacity")
	twoTop := createTestTable(t, db, business.ID, "Table 10", 4)
	seats := 2
	require.NoError(t, db.Model(twoTop).Updates(map[string]any{
		"max_capacity":       seats,
		"visible_seat_count": seats,
	}).Error)
	require.NoError(t, db.First(twoTop, twoTop.ID).Error)
	fourTop := createTestTable(t, db, business.ID, "Table 5", 4)
	configureReservationSettings(t, business.ID, nil)

	created, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName: "Layout party", PartySize: 4,
		ReservationTime: now.Add(time.Hour).Format(time.RFC3339), Source: "staff",
	}, "host", false)
	require.NoError(t, err)
	require.NotNil(t, created.TableID)
	require.Equal(t, fourTop.ID, *created.TableID)
	require.NotEqual(t, twoTop.ID, *created.TableID)
}

func TestCreateReservationRequiresExplicitOccupiedOverride(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	now := time.Date(2026, 7, 18, 18, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	business := createTestHospitalityBusiness(t, db, "reservation-explicit-occupancy")
	table := createTestTable(t, db, business.ID, "Occupied", 4)
	configureReservationSettings(t, business.ID, nil)
	require.NoError(t, db.Create(&database.Bill{
		BusinessID: business.ID, TableID: table.ID, BillNumber: "occupied-explicit",
		Status: database.BillStatusOpen, SettlementAddr: "settlement", TippingAddr: "tipping",
		CreatedAt: now.Add(-time.Hour),
	}).Error)

	input := CreateReservationInput{
		TableID: &table.ID, CustomerName: "Operator Guest", PartySize: 2,
		ReservationTime: now.Add(time.Hour).Format(time.RFC3339), Source: "staff",
	}
	_, err := service.CreateReservation(business.ID, input, "host", false)
	require.ErrorIs(t, err, database.ErrReservationTableOccupied)

	input.AllowOccupiedTableOverride = true
	created, err := service.CreateReservation(business.ID, input, "host", false)
	require.NoError(t, err)
	require.Equal(t, table.ID, *created.TableID)
}

func TestReservationConflictLoadingIgnoresStaleActiveReservations(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-conflict-stale")
	createTestTable(t, db, business.ID, "T-stale-1", 4)
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.DefaultDuration = 120
		settings.ServiceBufferMinutes = 0
	})
	ctx, err := service.loadAvailabilityContext(business.ID)
	require.NoError(t, err)

	slot := time.Date(2026, 5, 3, 19, 0, 0, 0, time.UTC)
	stale := database.TableReservation{
		BusinessID:       ctx.business.ID,
		PartySize:        2,
		ReservationTime:  slot.AddDate(0, -6, 0),
		Duration:         120,
		Status:           "confirmed",
		CustomerName:     "Stale Guest",
		ConfirmationCode: "STALE1",
	}
	overlap := database.TableReservation{
		BusinessID:       ctx.business.ID,
		PartySize:        2,
		ReservationTime:  slot.Add(30 * time.Minute),
		Duration:         120,
		Status:           "confirmed",
		CustomerName:     "Overlap Guest",
		ConfirmationCode: "OVERLAP1",
	}
	require.NoError(t, db.Create(&stale).Error)
	require.NoError(t, db.Create(&overlap).Error)

	conflicts, err := service.loadReservationConflicts(ctx, slot, slot.Add(2*time.Hour), 0)
	require.NoError(t, err)

	require.Len(t, conflicts.all, 1)
	require.Equal(t, overlap.ID, conflicts.all[0].ID)
}

func TestReservationConflictLoadingQueryUsesLowerBound(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	sqlLogger := &reservationSQLCaptureLogger{Interface: db.Logger}
	db = db.Session(&gorm.Session{Logger: sqlLogger})
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-conflict-bound")
	createTestTable(t, db, business.ID, "T-bound-1", 4)
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.DefaultDuration = 120
		settings.ServiceBufferMinutes = 15
	})
	ctx, err := service.loadAvailabilityContext(business.ID)
	require.NoError(t, err)

	slot := time.Date(2026, 5, 3, 19, 0, 0, 0, time.UTC)
	_, err = service.loadReservationConflicts(ctx, slot, slot.Add(2*time.Hour), 0)
	require.NoError(t, err)

	var reservationQuery string
	for _, statement := range sqlLogger.statements {
		if strings.Contains(statement, "table_reservations") &&
			strings.Contains(statement, "status IN") &&
			strings.Contains(statement, "reservation_time <") {
			reservationQuery = statement
		}
	}
	require.NotEmpty(t, reservationQuery)
	require.Contains(t, reservationQuery, "reservation_time >=")
}

func TestReservationServiceGetAvailabilityBatchesReservationConflictQueries(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-availability-batched")
	for i := 0; i < 8; i++ {
		createTestTable(t, db, business.ID, fmt.Sprintf("T-batch-%d", i), 4)
	}
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.DefaultDuration = 60
		settings.ServiceBufferMinutes = 0
		settings.SlotIntervalMinutes = 30
	})

	target := nextDayAt(18, 0)
	require.NoError(t, db.Model(&database.BusinessOperatingHours{}).
		Where("business_id = ? AND day_of_week = ?", business.ID, int(target.Weekday())).
		Updates(map[string]interface{}{
			"open_time":  "18:00",
			"close_time": "20:00",
			"is_closed":  false,
		}).Error)

	_, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Seed Guest",
		CustomerPhone:   "5550001111",
		CustomerEmail:   "seed@example.com",
		PartySize:       2,
		ReservationTime: target.Format(time.RFC3339),
		Source:          "staff",
	}, "host", false)
	require.NoError(t, err)

	for i := 0; i < 500; i++ {
		require.NoError(t, db.Create(&database.TableReservation{
			BusinessID:       business.ID,
			CustomerName:     fmt.Sprintf("Stale Guest %d", i),
			CustomerEmail:    fmt.Sprintf("stale-%d@example.com", i),
			PartySize:        2,
			ReservationTime:  target.AddDate(0, -6, 0).Add(time.Duration(i) * time.Minute),
			Duration:         120,
			Status:           "confirmed",
			Source:           "staff",
			ConfirmationCode: fmt.Sprintf("STALE-BATCH-%d", i),
		}).Error)
	}

	queryCount := 0
	callbackName := "payverge:test_reservation_availability_query_counter"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		queryCount++
	}))
	t.Cleanup(func() {
		_ = db.Callback().Query().Remove(callbackName)
	})

	availability, err := service.GetPublicAvailability(publicBusinessForTest(t, business.ID), target, 2)
	require.NoError(t, err)
	require.NotEmpty(t, availability.AvailableSlots)
	assert.LessOrEqual(t, queryCount, 8)
}

func TestReservationServiceCreateReservationWaitlistsWhenFull(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-waitlist")
	createTestTable(t, db, business.ID, "T2", 4)
	configureReservationSettings(t, business.ID, nil)

	target := nextDayAt(19, 0)
	_, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Primary Guest",
		CustomerPhone:   "5551111111",
		CustomerEmail:   "primary@example.com",
		PartySize:       4,
		ReservationTime: target.Format(time.RFC3339),
		Source:          "staff",
	}, "host", false)
	require.NoError(t, err)

	waitlistReservation, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Waitlist Guest",
		CustomerPhone:   "5552222222",
		CustomerEmail:   "waitlist@example.com",
		PartySize:       4,
		ReservationTime: target.Format(time.RFC3339),
		Source:          "customer",
	}, "customer", true)
	require.NoError(t, err)

	assert.Equal(t, "waitlist", waitlistReservation.Status)
	assert.Nil(t, waitlistReservation.TableID)
	require.NotNil(t, waitlistReservation.WaitlistPosition)
	assert.Equal(t, 1, *waitlistReservation.WaitlistPosition)
	require.Len(t, waitlistReservation.StatusHistory, 1)
	assert.Equal(t, "waitlist", waitlistReservation.StatusHistory[0].Status)
}

// connectTelegramForReservationTests satisfies the ShouldEnqueueTelegramNotification
// gate (added for parity with guest orders / manual payments / inventory) so
// reservation enqueue tests exercise the outbox write itself.
func connectTelegramForReservationTests(t *testing.T, db *gorm.DB, businessID uint) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))
	plugin := database.Plugin{Name: "telegram", DisplayName: "Telegram", IsActive: true, Category: "integration"}
	require.NoError(t, db.Create(&plugin).Error)
	require.NoError(t, db.Create(&database.BusinessPlugin{
		BusinessID: businessID,
		PluginID:   plugin.ID,
		IsEnabled:  true,
		Config:     `{"is_connected":true,"chat_id":"12345"}`,
	}).Error)
	ResetTelegramNotificationEligibilityCache()
	t.Cleanup(ResetTelegramNotificationEligibilityCache)
}

func TestReservationServiceCreateReservation_EnqueuesTelegramNotification(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-notify")
	connectTelegramForReservationTests(t, db, business.ID)
	createTestTable(t, db, business.ID, "T-notify", 4)
	configureReservationSettings(t, business.ID, nil)

	target := nextDayAt(19, 30)
	reservation, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Notify Guest",
		CustomerPhone:   "5554441111",
		CustomerEmail:   "notify@example.com",
		PartySize:       2,
		ReservationTime: target.Format(time.RFC3339),
		Source:          "staff",
	}, "host", false)
	require.NoError(t, err)

	var delivery database.PluginNotificationDelivery
	require.NoError(t, db.Where("business_id = ? AND plugin_name = ? AND event_type = ?", business.ID, "telegram", PluginEventReservationCreated).First(&delivery).Error)
	assert.Equal(t, fmt.Sprintf("reservation:%d", reservation.ID), delivery.EventID)
	assert.Equal(t, "Notify Guest", delivery.Payload["customer_name"])
	assert.Equal(t, float64(2), delivery.Payload["party_size"])
}

func TestReservationServiceTransitionCheckInCreatesBill(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-seat")
	table := createTestTable(t, db, business.ID, "T3", 4)
	configureReservationSettings(t, business.ID, nil)

	target := nextDayAt(20, 0)
	reservation, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Seated Guest",
		CustomerPhone:   "5553333333",
		CustomerEmail:   "seated@example.com",
		PartySize:       4,
		ReservationTime: target.Format(time.RFC3339),
		Source:          "staff",
	}, "host", false)
	require.NoError(t, err)
	require.NotNil(t, reservation.TableID)
	assert.Equal(t, table.ID, *reservation.TableID)

	pullReservationIntoArrivalWindow(t, db, reservation.ID)

	updated, err := service.TransitionReservation(business.ID, reservation.ID, "check_in", ReservationTransitionInput{}, "host")
	require.NoError(t, err)
	assert.Equal(t, "seated", updated.Status)
	assert.NotNil(t, updated.SeatedAt)

	bill, _, err := database.GetOpenBillByTableID(table.ID)
	require.NoError(t, err)
	assert.Equal(t, business.ID, bill.BusinessID)
}

// #704 T5 / bill 1132 / ticket 1123: reservation check-in and Live View POST
// /seat (reservation branch) used to CreateBill over leftover in_kitchen.
// Walk-in Liberar/seat already refuse; check-in must return the same error
// and must not open a new check.
func TestReservationServiceCheckIn_RefusesInKitchenOnAbandoned1132T5(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-t5-leftover")
	table := createTestTable(t, db, business.ID, "T5", 4)
	configureReservationSettings(t, business.ID, nil)

	target := nextDayAt(20, 0)
	reservation, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Next Party",
		CustomerPhone:   "5551132112",
		CustomerEmail:   "t5@example.com",
		PartySize:       2,
		ReservationTime: target.Format(time.RFC3339),
		Source:          "staff",
	}, "host", false)
	require.NoError(t, err)
	require.NotNil(t, reservation.TableID)
	assert.Equal(t, table.ID, *reservation.TableID)

	orphaned := &database.Bill{
		BusinessID:  business.ID,
		TableID:     table.ID,
		BillNumber:  "1132",
		Status:      database.BillStatusAbandoned,
		TotalAmount: 0,
		Items:       "[]",
	}
	require.NoError(t, db.Create(orphaned).Error)
	require.NoError(t, db.Create(&database.Order{
		BillID:      orphaned.ID,
		BusinessID:  business.ID,
		OrderNumber: "G86-36604192",
		Status:      database.OrderStatusInKitchen,
		CreatedBy:   "guest",
		Items:       `[{"id":"tea-1","menu_item_name":"Iced Tea","price":5,"quantity":1,"subtotal":5}]`,
	}).Error)

	pullReservationIntoArrivalWindow(t, db, reservation.ID)

	updated, err := service.TransitionReservation(business.ID, reservation.ID, "check_in", ReservationTransitionInput{}, "host")
	require.ErrorIs(t, err, database.ErrFloorLiveKitchenTickets)
	require.Nil(t, updated)

	var seated database.TableReservation
	require.NoError(t, db.First(&seated, reservation.ID).Error)
	assert.NotEqual(t, "seated", seated.Status, "must not mark seated when leftover kitchen blocks the bill")

	var opened int64
	require.NoError(t, db.Model(&database.Bill{}).
		Where("table_id = ? AND status IN ?", table.ID, database.ActiveBillStatuses()).
		Count(&opened).Error)
	assert.Equal(t, int64(0), opened, "CreateBill must not open a new check over leftover in_kitchen")
}

func TestReservationServiceCheckIn_RefusesPendingOnAbandonedTable(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-t1-pending")
	table := createTestTable(t, db, business.ID, "T1", 4)
	configureReservationSettings(t, business.ID, nil)

	target := nextDayAt(20, 0)
	reservation, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Queue Party",
		CustomerPhone:   "5551139113",
		CustomerEmail:   "t1@example.com",
		PartySize:       2,
		ReservationTime: target.Format(time.RFC3339),
		Source:          "staff",
	}, "host", false)
	require.NoError(t, err)

	orphaned := &database.Bill{
		BusinessID:  business.ID,
		TableID:     table.ID,
		BillNumber:  "1139",
		Status:      database.BillStatusAbandoned,
		TotalAmount: 0,
		Items:       "[]",
	}
	require.NoError(t, db.Create(orphaned).Error)
	require.NoError(t, db.Create(&database.Order{
		BillID:      orphaned.ID,
		BusinessID:  business.ID,
		OrderNumber: "G86-1129",
		Status:      database.OrderStatusPending,
		CreatedBy:   "guest",
		Items:       "[]",
	}).Error)

	pullReservationIntoArrivalWindow(t, db, reservation.ID)

	_, err = service.TransitionReservation(business.ID, reservation.ID, "check_in", ReservationTransitionInput{}, "host")
	require.ErrorIs(t, err, database.ErrFloorPendingOrders)
	require.NotErrorIs(t, err, database.ErrFloorLiveKitchenTickets)

	var opened int64
	require.NoError(t, db.Model(&database.Bill{}).
		Where("table_id = ? AND status IN ?", table.ID, database.ActiveBillStatuses()).
		Count(&opened).Error)
	assert.Equal(t, int64(0), opened)
}

func TestReservationServiceTransitionReservation_EnqueuesStatusNotification(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-status-notify")
	connectTelegramForReservationTests(t, db, business.ID)
	createTestTable(t, db, business.ID, "T-status", 4)
	configureReservationSettings(t, business.ID, nil)

	target := nextDayAt(20, 0)
	reservation, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Status Guest",
		CustomerPhone:   "5554442222",
		CustomerEmail:   "status@example.com",
		PartySize:       2,
		ReservationTime: target.Format(time.RFC3339),
		Source:          "staff",
	}, "host", false)
	require.NoError(t, err)

	pullReservationIntoArrivalWindow(t, db, reservation.ID)

	updated, err := service.TransitionReservation(business.ID, reservation.ID, "check_in", ReservationTransitionInput{}, "host")
	require.NoError(t, err)

	var delivery database.PluginNotificationDelivery
	// EventID is now keyed per-transition (…:status:<status>:<historyID>) so a
	// re-entered status isn't silently deduped; match by the stable prefix.
	require.NoError(t, db.Where("business_id = ? AND plugin_name = ? AND event_type = ? AND event_id LIKE ?",
		business.ID, "telegram", PluginEventReservationStatusChanged,
		fmt.Sprintf("reservation:%d:status:%s:%%", reservation.ID, updated.Status)).First(&delivery).Error)
	assert.Equal(t, "Status Guest", delivery.Payload["customer_name"])
	assert.Equal(t, updated.Status, delivery.Payload["status"])
}

// TestReservationServiceTransition_ReinstateAfterCancelReNotifies guards the N1
// fix: a status re-entered after leaving it (confirm → cancel → confirm) must
// produce a SECOND Telegram notification, not be silently deduped by a key that
// only encodes the status value.
func TestReservationServiceTransition_ReinstateAfterCancelReNotifies(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-reinstate-notify")
	connectTelegramForReservationTests(t, db, business.ID)
	createTestTable(t, db, business.ID, "T-reinstate", 4)
	configureReservationSettings(t, business.ID, nil)

	target := nextDayAt(20, 0)
	reservation, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Reinstate Guest",
		CustomerPhone:   "5554443333",
		CustomerEmail:   "reinstate@example.com",
		PartySize:       2,
		ReservationTime: target.Format(time.RFC3339),
		Source:          "staff",
	}, "host", false)
	require.NoError(t, err)

	// Two full cancel→confirm cycles guarantee two REAL confirmed transitions
	// regardless of the auto-assigned starting status.
	for i := 0; i < 2; i++ {
		_, err = service.TransitionReservation(business.ID, reservation.ID, "cancel", ReservationTransitionInput{}, "host")
		require.NoError(t, err)
		_, err = service.TransitionReservation(business.ID, reservation.ID, "confirm", ReservationTransitionInput{}, "host")
		require.NoError(t, err)
	}

	// Both "confirmed" transitions must have produced a distinct delivery row.
	var confirmedDeliveries int64
	require.NoError(t, db.Model(&database.PluginNotificationDelivery{}).
		Where("business_id = ? AND plugin_name = ? AND event_type = ? AND event_id LIKE ?",
			business.ID, "telegram", PluginEventReservationStatusChanged,
			fmt.Sprintf("reservation:%d:status:confirmed:%%", reservation.ID)).
		Count(&confirmedDeliveries).Error)
	assert.Equal(t, int64(2), confirmedDeliveries, "reinstated 'confirmed' must re-notify, not dedup")
}

func TestReservationServiceTransitionCheckInPreservesMalformedActiveBillError(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-seat-malformed-bill")
	table := createTestTable(t, db, business.ID, "T3B", 4)
	configureReservationSettings(t, business.ID, nil)

	target := nextDayAt(20, 30)
	reservation, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Malformed Bill Guest",
		CustomerPhone:   "5553334444",
		CustomerEmail:   "malformed-bill@example.com",
		PartySize:       4,
		ReservationTime: target.Format(time.RFC3339),
		Source:          "staff",
	}, "host", false)
	require.NoError(t, err)
	require.NotNil(t, reservation.TableID)
	assert.Equal(t, table.ID, *reservation.TableID)

	require.NoError(t, db.Create(&database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     "B-reservation-malformed-active",
		Status:         database.BillStatusOpen,
		Items:          "{",
		SettlementAddr: business.SettlementAddr,
		TippingAddr:    business.TippingAddr,
	}).Error)

	_, err = service.TransitionReservation(business.ID, reservation.ID, "check_in", ReservationTransitionInput{}, "host")
	require.Error(t, err)

	reloadedReservation, err := database.GetReservationByBusinessAndID(business.ID, reservation.ID)
	require.NoError(t, err)
	assert.Equal(t, "confirmed", reloadedReservation.Status)

	var activeBillCount int64
	require.NoError(t, db.Model(&database.Bill{}).
		Where("table_id = ? AND status IN ?", table.ID, database.ActiveBillStatuses()).
		Count(&activeBillCount).Error)
	assert.EqualValues(t, 1, activeBillCount)
}

func TestReservationServiceCreateReservation_SanitizesGuestOnlyFields(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-guest-sanitize")
	table := createTestTable(t, db, business.ID, "T-guest-sanitize", 4)
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.AutoAssignTables = false
		settings.ApprovalMode = database.ReservationApprovalManual
	})

	target := nextDayAt(18, 30)
	reservation, err := service.CreateReservation(business.ID, CreateReservationInput{
		TableID:         &table.ID,
		CustomerName:    "Guest Sanitized",
		CustomerPhone:   "5552220000",
		CustomerEmail:   "guest-sanitized@example.com",
		PartySize:       2,
		ReservationTime: target.Format(time.RFC3339),
		SpecialRequests: "Window seat",
		Notes:           "Internal manager note",
		Source:          "staff",
	}, "customer", true)
	require.NoError(t, err)

	assert.Nil(t, reservation.TableID)
	assert.Equal(t, "customer", reservation.Source)
	assert.Equal(t, "customer", reservation.CreatedBy)
	assert.Equal(t, "Window seat", reservation.SpecialRequests)
	assert.Empty(t, reservation.Notes)
}

func TestReservationServiceRejectsCrossBusinessAccess(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	businessOne := createTestHospitalityBusiness(t, db, "reservation-scope-a")
	businessTwo := createTestHospitalityBusiness(t, db, "reservation-scope-b")
	createTestTable(t, db, businessOne.ID, "T4", 4)
	configureReservationSettings(t, businessOne.ID, nil)
	configureReservationSettings(t, businessTwo.ID, nil)

	target := nextDayAt(17, 0)
	reservation, err := service.CreateReservation(businessOne.ID, CreateReservationInput{
		CustomerName:    "Scoped Guest",
		CustomerPhone:   "5554444444",
		CustomerEmail:   "scope@example.com",
		PartySize:       2,
		ReservationTime: target.Format(time.RFC3339),
		Source:          "staff",
	}, "host", false)
	require.NoError(t, err)

	_, err = service.TransitionReservation(businessTwo.ID, reservation.ID, "check_in", ReservationTransitionInput{}, "host")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reservation not found")
}

func TestReservationServiceGetPublicSettingsIncludesBookableSlotSummary(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-public-summary")
	createTestTable(t, db, business.ID, "T5", 4)
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.DefaultDuration = 30
		settings.ServiceBufferMinutes = 0
		settings.SlotIntervalMinutes = 30
	})

	require.NoError(t, db.Model(&database.BusinessOperatingHours{}).
		Where("business_id = ?", business.ID).
		Update("is_closed", true).Error)

	target := nextDayAt(18, 0)
	targetWeekday := int(target.Weekday())
	require.NoError(t, db.Model(&database.BusinessOperatingHours{}).
		Where("business_id = ? AND day_of_week = ?", business.ID, targetWeekday).
		Updates(map[string]interface{}{
			"is_closed":  false,
			"open_time":  "18:00",
			"close_time": "19:00",
		}).Error)

	_, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Summary Guest",
		CustomerPhone:   "5555555555",
		CustomerEmail:   "summary@example.com",
		PartySize:       4,
		ReservationTime: target.Format(time.RFC3339),
		Source:          "staff",
	}, "host", false)
	require.NoError(t, err)

	publicSettings, err := service.GetPublicSettingsForBusiness(publicBusinessForTest(t, business.ID))
	require.NoError(t, err)
	require.NotNil(t, publicSettings.NextAvailableSlot)
	assert.Equal(t, 1, publicSettings.AvailableSlotCount)
	assert.Equal(t, target.Add(30*time.Minute).Format(time.RFC3339), *publicSettings.NextAvailableSlot)
}

func TestReservationServiceGetPublicSettings_ClosedOmitsSlots(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-closed-public")
	require.NoError(t, db.Model(business).Update("closed_at", time.Now().Add(-time.Hour)).Error)
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.Enabled = true
	})

	operator, err := service.GetSettings(business.ID)
	require.NoError(t, err)
	assert.True(t, operator.Enabled, "operator settings must keep the stored enabled flag")

	publicSettings, err := service.GetPublicSettingsForBusiness(publicBusinessForTest(t, business.ID))
	require.NoError(t, err)
	assert.False(t, publicSettings.Enabled)
	assert.Nil(t, publicSettings.NextAvailableSlot)
	assert.Equal(t, 0, publicSettings.AvailableSlotCount)
}

func TestReservationServiceGetSettingsPreservesExplicitZeroValueControls(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-zero-controls")

	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.MinAdvanceMinutes = 0
		settings.ServiceBufferMinutes = 0
		settings.HoldDurationMinutes = 0
		settings.NoShowGraceMinutes = 0
		settings.ReminderHoursBefore = 0
	})

	settings, err := service.GetSettings(business.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, settings.MinAdvanceMinutes)
	assert.Equal(t, 0, settings.ServiceBufferMinutes)
	assert.Equal(t, 0, settings.HoldDurationMinutes)
	assert.Equal(t, 0, settings.NoShowGraceMinutes)
	assert.Equal(t, 0, settings.ReminderHoursBefore)
}

func TestReservationServiceUpdateSettingsRejectsNegativeOperationalControls(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-settings-validation")
	configureReservationSettings(t, business.ID, nil)

	negativeOne := -1
	cases := []struct {
		name    string
		input   UpdateReservationSettingsInput
		wantErr string
	}{
		{
			name:    "max covers per slot",
			input:   UpdateReservationSettingsInput{MaxCoversPerSlot: &negativeOne},
			wantErr: "max_covers_per_slot cannot be negative",
		},
		{
			name:    "cancellation deadline",
			input:   UpdateReservationSettingsInput{CancellationDeadline: &negativeOne},
			wantErr: "cancellation_deadline cannot be negative",
		},
		{
			name:    "reminder hours before",
			input:   UpdateReservationSettingsInput{ReminderHoursBefore: &negativeOne},
			wantErr: "reminder_hours_before cannot be negative",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.UpdateSettings(business.ID, tc.input)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestReservationServiceUpdateReservation_PreservesFieldChangesWhenTransitioningStatus(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-status-update")
	createTestTable(t, db, business.ID, "T6", 4)
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.ApprovalMode = database.ReservationApprovalManual
	})

	target := nextDayAt(18, 30)
	reservation, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Pending Guest",
		CustomerPhone:   "5556666666",
		CustomerEmail:   "pending@example.com",
		PartySize:       2,
		ReservationTime: target.Format(time.RFC3339),
		Source:          "customer",
	}, "customer", true)
	require.NoError(t, err)
	require.Equal(t, "pending", reservation.Status)

	updatedName := "Confirmed Guest"
	updatedReservation, _, err := service.UpdateReservation(business.ID, reservation.ID, UpdateReservationInput{
		CustomerName: &updatedName,
		Status:       stringPtr("confirmed"),
	}, "host")
	require.NoError(t, err)
	assert.Equal(t, "confirmed", updatedReservation.Status)
	assert.Equal(t, updatedName, updatedReservation.CustomerName)
}

func TestReservationServiceUpdateReservation_StatusOnlyConfirmSkipsAdvanceWindow(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-status-only-confirm")
	createTestTable(t, db, business.ID, "T6B", 4)
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.ApprovalMode = database.ReservationApprovalManual
		settings.MinAdvanceMinutes = 0
	})

	target := nextDayAt(19, 0)
	reservation, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Pending Inside Window",
		CustomerPhone:   "5556667777",
		CustomerEmail:   "inside-window@example.com",
		PartySize:       2,
		ReservationTime: target.Format(time.RFC3339),
		Source:          "customer",
	}, "customer", true)
	require.NoError(t, err)
	require.Equal(t, "pending", reservation.Status)

	require.NoError(t, db.Model(&database.ReservationSettings{}).
		Where("business_id = ?", business.ID).
		Update("min_advance_minutes", 48*60).Error)

	updatedReservation, _, err := service.UpdateReservation(business.ID, reservation.ID, UpdateReservationInput{
		Status: stringPtr("confirmed"),
	}, "host")
	require.NoError(t, err)
	assert.Equal(t, "confirmed", updatedReservation.Status)
	assert.Equal(t, reservation.ReservationTime, updatedReservation.ReservationTime)
}

func TestReservationServiceTransitionReservation_RejectsUnknownStatus(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-invalid-transition")
	createTestTable(t, db, business.ID, "T7", 4)
	configureReservationSettings(t, business.ID, nil)

	target := nextDayAt(21, 0)
	reservation, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Transition Guest",
		CustomerPhone:   "5557777777",
		CustomerEmail:   "transition@example.com",
		PartySize:       2,
		ReservationTime: target.Format(time.RFC3339),
		Source:          "staff",
	}, "host", false)
	require.NoError(t, err)

	_, err = service.TransitionReservation(business.ID, reservation.ID, "mystery_state", ReservationTransitionInput{}, "host")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported reservation transition")

	reloaded, err := database.GetReservationByBusinessAndID(business.ID, reservation.ID)
	require.NoError(t, err)
	assert.Equal(t, reservation.Status, reloaded.Status)
}

func TestReservationServiceCancelReservationByCode_RejectsClosedCancellationWindow(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-public-cancel-deadline")
	createTestTable(t, db, business.ID, "T9", 4)
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.AllowCancellation = true
		settings.CancellationDeadline = 48
	})

	target := nextDayAt(18, 0)
	reservation, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Late Cancellation Guest",
		CustomerPhone:   "5559999999",
		CustomerEmail:   "late-cancel@example.com",
		PartySize:       2,
		ReservationTime: target.Format(time.RFC3339),
		Source:          "customer",
	}, "customer", true)
	require.NoError(t, err)

	_, _, err = service.CancelReservationByCode(reservation.ConfirmationCode, "customer")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be cancelled online")
}

func TestReservationServiceCancelReservationByCode_IsIdempotentForCancelledReservations(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-public-cancelled")
	createTestTable(t, db, business.ID, "T10", 4)
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.AllowCancellation = true
		settings.CancellationDeadline = 1
	})

	target := nextDayAt(19, 0)
	reservation, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Cancelled Guest",
		CustomerPhone:   "5551010101",
		CustomerEmail:   "cancelled@example.com",
		PartySize:       2,
		ReservationTime: target.Format(time.RFC3339),
		Source:          "customer",
	}, "customer", true)
	require.NoError(t, err)

	cancelledReservation, err := service.TransitionReservation(business.ID, reservation.ID, "cancel", ReservationTransitionInput{
		Reason: "Cancelled by customer",
	}, "customer")
	require.NoError(t, err)
	require.Equal(t, "cancelled", cancelledReservation.Status)

	reloadedReservation, changed, err := service.CancelReservationByCode(reservation.ConfirmationCode, "customer")
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, "cancelled", reloadedReservation.Status)
}

func TestReservationServiceGetPublicReservationDetailsByCode_ReturnsGuestActions(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-public-details")
	table := createTestTable(t, db, business.ID, "T11", 4)
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.ApprovalMode = database.ReservationApprovalManual
		settings.AllowCancellation = true
		settings.CancellationDeadline = 4
	})

	target := nextDayAt(18, 0)
	reservation, err := service.CreateReservation(business.ID, CreateReservationInput{
		TableID:         &table.ID,
		CustomerName:    "Public Detail Guest",
		CustomerPhone:   "5551212121",
		CustomerEmail:   "public-detail@example.com",
		PartySize:       2,
		ReservationTime: target.Format(time.RFC3339),
		SpecialRequests: "Window seat",
		Source:          "customer",
	}, "customer", true)
	require.NoError(t, err)
	require.Equal(t, "pending", reservation.Status)

	details, err := service.GetPublicReservationDetailsByCode(reservation.ConfirmationCode)
	require.NoError(t, err)
	assert.Equal(t, business.Name, details.BusinessName)
	assert.Equal(t, business.CustomURL, details.BusinessCustomURL)
	assert.True(t, details.CanCancel)
	assert.Equal(t, "Window seat", details.Reservation.SpecialRequests)
	assert.Equal(t, table.Name, details.Reservation.TableName)
	// Guest confirmation UI formats reservation_time in venue wall-clock.
	assert.Equal(t, business.Timezone, details.BusinessTimezone)

	raw, err := json.Marshal(details)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"table_code"`)
	assert.NotContains(t, string(raw), `"qr_code"`)
}

func stringPtr(value string) *string {
	return &value
}

func TestReservationServiceStaffCreateSkipsMinAdvanceWindow(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-staff-min-advance")
	createTestTable(t, db, business.ID, "T-walkin-1", 4)
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		// Larger than any bookable horizon so every guest attempt trips it.
		settings.MinAdvanceMinutes = 2_000_000
		settings.MaxAdvanceDays = 3650
	})

	target := nextDayAt(12, 0)

	// Guests still respect the advance window…
	_, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Guest Too Soon",
		CustomerPhone:   "5550000001",
		CustomerEmail:   "soon@example.com",
		PartySize:       2,
		ReservationTime: target.Format(time.RFC3339),
		Source:          "customer",
	}, "customer", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "in advance")

	// …but staff booking a walk-in must not be blocked by it.
	created, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Walk-in Guest",
		CustomerPhone:   "5550000002",
		CustomerEmail:   "walkin@example.com",
		PartySize:       2,
		ReservationTime: target.Format(time.RFC3339),
		Source:          "staff",
	}, "host", false)
	require.NoError(t, err)
	assert.Equal(t, "confirmed", created.Status)
}

func TestReservationServiceUpdateUnchangedTimingSkipsWindowRevalidation(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-update-unchanged")
	createTestTable(t, db, business.ID, "T-edit-1", 4)
	configureReservationSettings(t, business.ID, nil)

	// Seed a reservation that already happened — its time can no longer pass
	// the advance-window validation, but editing contact details must work.
	past := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Minute)
	reservation := database.TableReservation{
		BusinessID:       business.ID,
		CustomerName:     "Past Guest",
		CustomerPhone:    "5551112222",
		CustomerEmail:    "past@example.com",
		PartySize:        2,
		ReservationTime:  past,
		Duration:         90,
		Status:           "confirmed",
		Source:           "staff",
		ConfirmationCode: "PASTEDIT1",
	}
	require.NoError(t, db.Create(&reservation).Error)

	// Mirrors the dashboard edit modal: every field is sent, all unchanged
	// except the phone number.
	sameTime := past.Format(time.RFC3339)
	updated, _, err := service.UpdateReservation(business.ID, reservation.ID, UpdateReservationInput{
		CustomerPhone:   stringPtr("5559998888"),
		ReservationTime: &sameTime,
		PartySize:       intPtr(2),
		Duration:        intPtr(90),
	}, "host")
	require.NoError(t, err)
	assert.Equal(t, "5559998888", updated.CustomerPhone)
	assert.True(t, updated.ReservationTime.Equal(past))
}

func TestReservationServiceUpdatePartySizeStillValidatesCapacity(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-update-capacity")
	createTestTable(t, db, business.ID, "T-cap-1", 4)
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.MaxPartySize = 8
	})

	target := nextDayAt(13, 0)
	created, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Resize Guest",
		CustomerPhone:   "5553337777",
		CustomerEmail:   "resize@example.com",
		PartySize:       2,
		ReservationTime: target.Format(time.RFC3339),
		Source:          "staff",
	}, "host", false)
	require.NoError(t, err)

	_, _, err = service.UpdateReservation(business.ID, created.ID, UpdateReservationInput{
		PartySize: intPtr(20),
	}, "host")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "party size")
}

func intPtr(value int) *int {
	return &value
}

func TestReservationServiceGetPublicReservationDetailsByCodeNormalizesCase(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-public-code-case")
	createTestTable(t, db, business.ID, "T-code-case", 4)
	configureReservationSettings(t, business.ID, nil)

	reservation, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Case Lookup Guest",
		CustomerPhone:   "5555390001",
		CustomerEmail:   "case-lookup@example.com",
		PartySize:       2,
		ReservationTime: nextDayAt(18, 0).Format(time.RFC3339),
		Source:          "customer",
	}, "customer", true)
	require.NoError(t, err)
	require.NotEmpty(t, reservation.ConfirmationCode)

	for _, code := range []string{
		reservation.ConfirmationCode,
		strings.ToLower(reservation.ConfirmationCode),
		"  " + strings.ToLower(reservation.ConfirmationCode) + "  ",
	} {
		details, err := service.GetPublicReservationDetailsByCode(code)
		require.NoError(t, err, "code %q", code)
		assert.Equal(t, reservation.ID, details.Reservation.ID, "code %q", code)
		assert.Equal(t, reservation.ConfirmationCode, details.Reservation.ConfirmationCode, "code %q", code)
	}
}

func TestReservationServiceCancelReservationByCodeNormalizesCase(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-public-cancel-case")
	createTestTable(t, db, business.ID, "T-cancel-case", 4)
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.AllowCancellation = true
		settings.CancellationDeadline = 0
	})

	reservation, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Case Cancel Guest",
		CustomerPhone:   "5555390002",
		CustomerEmail:   "case-cancel@example.com",
		PartySize:       2,
		ReservationTime: nextDayAt(19, 0).Format(time.RFC3339),
		Source:          "customer",
	}, "customer", true)
	require.NoError(t, err)

	cancelled, changed, err := service.CancelReservationByCode("  "+strings.ToLower(reservation.ConfirmationCode)+"  ", "customer")
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Equal(t, "cancelled", cancelled.Status)
	assert.Equal(t, reservation.ID, cancelled.ID)
}

func TestReservationServiceCreateRetriesCollidingConfirmationCodes(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-code-collision")
	createTestTable(t, db, business.ID, "T-code-1", 4)
	createTestTable(t, db, business.ID, "T-code-2", 4)
	configureReservationSettings(t, business.ID, nil)

	existing := database.TableReservation{
		BusinessID:       business.ID,
		CustomerName:     "First Holder",
		PartySize:        2,
		ReservationTime:  nextDayAt(17, 0),
		Duration:         90,
		Status:           "confirmed",
		Source:           "staff",
		ConfirmationCode: "C0LL1DE00001",
	}
	require.NoError(t, db.Create(&existing).Error)

	originalGenerator := reservationCodeGenerator
	t.Cleanup(func() { reservationCodeGenerator = originalGenerator })
	calls := 0
	reservationCodeGenerator = func() (string, error) {
		calls++
		if calls == 1 {
			return "C0LL1DE00001", nil
		}
		return "FRESHC0DE001", nil
	}

	created, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Second Guest",
		CustomerPhone:   "5556660000",
		CustomerEmail:   "second@example.com",
		PartySize:       2,
		ReservationTime: nextDayAt(19, 0).Format(time.RFC3339),
		Source:          "staff",
	}, "host", false)
	require.NoError(t, err)
	assert.Equal(t, "FRESHC0DE001", created.ConfirmationCode)
	assert.GreaterOrEqual(t, calls, 2, "generator must be retried after the collision")
}

func TestReservationServiceCreateRetriesCollidingConfirmationCodesCaseInsensitive(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-code-case-collision")
	createTestTable(t, db, business.ID, "T-code-case-1", 4)
	createTestTable(t, db, business.ID, "T-code-case-2", 4)
	configureReservationSettings(t, business.ID, nil)

	existing := database.TableReservation{
		BusinessID:       business.ID,
		CustomerName:     "Upper Holder",
		PartySize:        2,
		ReservationTime:  nextDayAt(17, 0),
		Duration:         90,
		Status:           "confirmed",
		Source:           "staff",
		ConfirmationCode: "C0LL1DE00001",
	}
	require.NoError(t, db.Create(&existing).Error)

	originalGenerator := reservationCodeGenerator
	t.Cleanup(func() { reservationCodeGenerator = originalGenerator })
	calls := 0
	reservationCodeGenerator = func() (string, error) {
		calls++
		if calls == 1 {
			return "  c0ll1de00001  ", nil
		}
		return "  freshc0de001  ", nil
	}

	created, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Second Guest",
		CustomerPhone:   "5556660001",
		CustomerEmail:   "second-case@example.com",
		PartySize:       2,
		ReservationTime: nextDayAt(19, 0).Format(time.RFC3339),
		Source:          "staff",
	}, "host", false)
	require.NoError(t, err)
	assert.Equal(t, "FRESHC0DE001", created.ConfirmationCode)
	assert.GreaterOrEqual(t, calls, 2, "generator must retry after a case-insensitive collision")
}

func TestGenerateReservationCodeIsLongUppercaseHex(t *testing.T) {
	code, err := generateReservationCode()
	require.NoError(t, err)
	assert.Len(t, code, 12)
	assert.Regexp(t, "^[0-9A-F]+$", code)
}

func TestReservationServiceAvailabilityDoesNotAdvertiseUnbookableTailSlots(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-tail-slots")
	createTestTable(t, db, business.ID, "T-tail-1", 4)
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.DefaultDuration = 90
		settings.ServiceBufferMinutes = 15
		settings.SlotIntervalMinutes = 30
	})

	target := nextDayAt(18, 0)
	require.NoError(t, db.Model(&database.BusinessOperatingHours{}).
		Where("business_id = ? AND day_of_week = ?", business.ID, int(target.Weekday())).
		Updates(map[string]interface{}{
			"open_time":  "18:00",
			"close_time": "20:00",
			"is_closed":  false,
		}).Error)

	availability, err := service.GetPublicAvailability(publicBusinessForTest(t, business.ID), target, 2)
	require.NoError(t, err)
	require.NotEmpty(t, availability.AvailableSlots)

	slotByTime := map[string]ReservationAvailabilitySlotDTO{}
	for _, slot := range availability.AvailableSlots {
		slotByTime[slot.Time] = slot
	}

	first, ok := slotByTime[target.UTC().Format(time.RFC3339)]
	require.True(t, ok)
	assert.Greater(t, first.AvailableTables, 0, "18:00 fits 90+15 minutes before the 20:00 close and must stay bookable")

	for _, offset := range []time.Duration{30 * time.Minute, 60 * time.Minute, 90 * time.Minute} {
		when := target.Add(offset)
		slot, ok := slotByTime[when.UTC().Format(time.RFC3339)]
		require.True(t, ok, "slot %s should still be listed", when)
		assert.Equal(t, 0, slot.AvailableTables, "slot %s cannot fit duration+buffer before close and must not be advertised", when)
		assert.Equal(t, "outside_operating_window", slot.ReasonCode, "slot %s", when)
		assert.False(t, slot.Recommended, "slot %s", when)
	}

	// Availability and the create path must agree: the tail slot is rejected today,
	// so it must never be advertised as bookable.
	_, err = service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Tail Guest",
		CustomerPhone:   "5553334444",
		CustomerEmail:   "tail@example.com",
		PartySize:       2,
		ReservationTime: target.Add(30 * time.Minute).Format(time.RFC3339),
		Source:          "customer",
	}, "customer", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "operating hours")
}

func TestApplySparseRecommendations(t *testing.T) {
	t.Parallel()

	t.Run("equivalent open day has no recommendation", func(t *testing.T) {
		slots := []ReservationAvailabilitySlotDTO{
			{Time: "2026-06-20T17:00:00Z", AvailableTables: 10, Recommended: true},
			{Time: "2026-06-20T17:30:00Z", AvailableTables: 10, Recommended: true},
			{Time: "2026-06-20T18:00:00Z", AvailableTables: 10, Recommended: true},
		}
		applySparseRecommendations(slots)
		for _, slot := range slots {
			assert.False(t, slot.Recommended, slot.Time)
		}
	})

	t.Run("differing capacity recommends soonest max-table slot only", func(t *testing.T) {
		slots := []ReservationAvailabilitySlotDTO{
			{Time: "2026-06-20T17:00:00Z", AvailableTables: 9},
			{Time: "2026-06-20T17:30:00Z", AvailableTables: 10},
			{Time: "2026-06-20T18:00:00Z", AvailableTables: 10},
			{Time: "2026-06-20T18:30:00Z", AvailableTables: 8},
		}
		applySparseRecommendations(slots)
		var recommended []string
		for _, slot := range slots {
			if slot.Recommended {
				recommended = append(recommended, slot.Time)
			}
		}
		assert.Equal(t, []string{"2026-06-20T17:30:00Z"}, recommended)
	})

	t.Run("constrained and waitlist windows stay unmarked", func(t *testing.T) {
		slots := []ReservationAvailabilitySlotDTO{
			{Time: "2026-06-20T16:00:00Z", AvailableTables: 0, ReasonCode: "covers_limit", Recommended: true},
			{Time: "2026-06-20T16:30:00Z", AvailableTables: 0, ReasonCode: "outside_operating_window", Recommended: true},
			{Time: "2026-06-20T17:00:00Z", AvailableTables: 2, Recommended: true},
		}
		applySparseRecommendations(slots)
		for _, slot := range slots {
			assert.False(t, slot.Recommended, slot.Time)
		}
	})

	t.Run("single open slot is not recommended", func(t *testing.T) {
		slots := []ReservationAvailabilitySlotDTO{
			{Time: "2026-06-20T19:00:00Z", AvailableTables: 4, Recommended: true},
		}
		applySparseRecommendations(slots)
		assert.False(t, slots[0].Recommended)
	})
}

func TestReservationServiceAvailabilityRecommendationsAreSparse(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-sparse-rec")
	createTestTable(t, db, business.ID, "T-rec-1", 4)
	createTestTable(t, db, business.ID, "T-rec-2", 4)
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.SlotIntervalMinutes = 30
		settings.DefaultDuration = 60
	})

	target := nextDayAt(18, 0)
	availability, err := service.GetPublicAvailability(publicBusinessForTest(t, business.ID), target, 2)
	require.NoError(t, err)
	require.NotEmpty(t, availability.AvailableSlots)

	open := 0
	recommended := 0
	for _, slot := range availability.AvailableSlots {
		if slot.AvailableTables > 0 {
			open++
		}
		if slot.Recommended {
			recommended++
			assert.Greater(t, slot.AvailableTables, 0)
		}
	}
	require.Greater(t, open, 1)
	assert.Zero(t, recommended, "equivalent full-capacity day must not badge every open slot")

	_, err = service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Hold One Table",
		CustomerPhone:   "5550002222",
		CustomerEmail:   "hold@example.com",
		PartySize:       2,
		ReservationTime: target.Format(time.RFC3339),
		Source:          "staff",
	}, "host", false)
	require.NoError(t, err)

	mixed, err := service.GetPublicAvailability(publicBusinessForTest(t, business.ID), target, 2)
	require.NoError(t, err)
	recommended = 0
	var recommendedTime string
	var recommendedTables int
	maxOpenTables := 0
	for _, slot := range mixed.AvailableSlots {
		if slot.AvailableTables > maxOpenTables {
			maxOpenTables = slot.AvailableTables
		}
		if slot.Recommended {
			recommended++
			recommendedTime = slot.Time
			recommendedTables = slot.AvailableTables
		}
	}
	if maxOpenTables == 0 {
		t.Fatal("expected at least one open slot after holding one table")
	}
	minOpenTables := maxOpenTables
	for _, slot := range mixed.AvailableSlots {
		if slot.AvailableTables > 0 && slot.AvailableTables < minOpenTables {
			minOpenTables = slot.AvailableTables
		}
	}
	if minOpenTables == maxOpenTables {
		assert.Zero(t, recommended)
		return
	}
	require.Equal(t, 1, recommended)
	assert.Equal(t, maxOpenTables, recommendedTables)
	assert.NotEmpty(t, recommendedTime)
}

func TestReservationServiceApprovalModeControlsGuestStatus(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "approval-mode-status")
	createTestTable(t, db, business.ID, "T-appr-1", 4)
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.ApprovalMode = database.ReservationApprovalManual
	})

	pendingRes, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName: "Manual Guest", CustomerEmail: "manual@example.com", CustomerPhone: "5550001111",
		PartySize: 2, ReservationTime: nextDayAt(12, 0).Format(time.RFC3339), Source: "customer",
	}, "customer", true)
	require.NoError(t, err)
	assert.Equal(t, "pending", pendingRes.Status, "manual mode: guest bookings await business approval")

	// Staff bookings are never gated by approval mode.
	staffRes, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName: "Walk In", PartySize: 2,
		ReservationTime: nextDayAt(15, 0).Format(time.RFC3339), Source: "staff",
	}, "host", false)
	require.NoError(t, err)
	assert.Equal(t, "confirmed", staffRes.Status)

	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.ApprovalMode = database.ReservationApprovalAuto
	})
	autoRes, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName: "Auto Guest", CustomerEmail: "auto@example.com", CustomerPhone: "5550002222",
		PartySize: 2, ReservationTime: nextDayAt(18, 0).Format(time.RFC3339), Source: "customer",
	}, "customer", true)
	require.NoError(t, err)
	assert.Equal(t, "confirmed", autoRes.Status, "auto mode: instant confirmation")
}

func TestReservationServiceUpdateSettingsRejectsUnknownApprovalMode(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "approval-mode-validate")
	configureReservationSettings(t, business.ID, nil)

	bad := "whenever"
	_, err := service.UpdateSettings(business.ID, UpdateReservationSettingsInput{ApprovalMode: &bad})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "approval_mode")
}

func TestReservationServiceUpdateReservation_ReturnsPriorSnapshot(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-prior-snapshot")
	createTestTable(t, db, business.ID, "T6P", 4)
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.ApprovalMode = database.ReservationApprovalManual
	})

	target := nextDayAt(18, 30)
	reservation, err := service.CreateReservation(business.ID, CreateReservationInput{
		CustomerName:    "Prior Guest",
		CustomerPhone:   "5557777777",
		CustomerEmail:   "prior@example.com",
		PartySize:       2,
		ReservationTime: target.Format(time.RFC3339),
		Source:          "customer",
	}, "customer", true)
	require.NoError(t, err)
	require.Equal(t, "pending", reservation.Status)

	updated, prior, err := service.UpdateReservation(business.ID, reservation.ID, UpdateReservationInput{
		Status: stringPtr("confirmed"),
	}, "host")
	require.NoError(t, err)
	require.NotNil(t, prior)
	// The prior snapshot comes from the same row read the mutation used, so
	// callers can branch on the true pre-update state without a separate
	// (racy) read of their own.
	assert.Equal(t, "pending", prior.Status)
	assert.Equal(t, "confirmed", updated.Status)
	assert.Equal(t, reservation.PartySize, prior.PartySize)
	assert.True(t, prior.ReservationTime.Equal(reservation.ReservationTime))
}

func TestReservationNotificationPayload_IncludesDashboardURL(t *testing.T) {
	t.Setenv("PUBLIC_URL", "https://payverge.io")
	payload := reservationNotificationPayload(&database.TableReservation{
		ID:         3,
		BusinessID: 7,
		Status:     "pending",
	})
	require.Equal(t, "https://payverge.io/business/7/dashboard?tab=reservations", payload["dashboard_url"])
}

func TestGetAvailabilityNetsReservedAndOccupiedTables(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	now := time.Date(2026, 8, 11, 17, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	business := createTestHospitalityBusiness(t, db, "reservation-overbook-net")
	tables := make([]*database.Table, 0, 9)
	for i := 1; i <= 9; i++ {
		tables = append(tables, createTestTable(t, db, business.ID, fmt.Sprintf("T%d", i), 4))
	}
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.DefaultDuration = 120
		settings.ServiceBufferMinutes = 15
		settings.SlotIntervalMinutes = 30
		settings.MaxCoversPerSlot = 48
		settings.MinAdvanceMinutes = 0
	})

	slot1900 := time.Date(2026, 8, 11, 19, 0, 0, 0, time.UTC)
	tableID := tables[1].ID
	require.NoError(t, db.Create(&database.TableReservation{
		BusinessID:       business.ID,
		TableID:          &tableID,
		CustomerName:     "Demo Reservation",
		PartySize:        2,
		ReservationTime:  slot1900,
		Duration:         120,
		Status:           "confirmed",
		ConfirmationCode: "DEMO1900",
		Source:           "staff",
	}).Error)
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&database.Bill{
			BusinessID:     business.ID,
			TableID:        tables[i+3].ID,
			BillNumber:     fmt.Sprintf("open-bill-%d", i),
			Status:         database.BillStatusOpen,
			SettlementAddr: "settlement",
			TippingAddr:    "tipping",
			CreatedAt:      now.Add(-45 * time.Minute),
		}).Error)
	}

	availability, err := service.GetPublicAvailability(publicBusinessForTest(t, business.ID), slot1900, 2)
	require.NoError(t, err)

	byTime := map[string]ReservationAvailabilitySlotDTO{}
	for _, slot := range availability.AvailableSlots {
		byTime[slot.Time] = slot
	}

	matched, ok := byTime[slot1900.Format(time.RFC3339)]
	require.True(t, ok, "19:00 slot must be present")
	// 9 tables − 1 reserved − 3 occupied = 5 ready (not a flat 9).
	assert.Equal(t, 5, matched.AvailableTables)
	assert.Equal(t, "available", matched.ReasonCode)

	early, ok := byTime[time.Date(2026, 8, 11, 18, 30, 0, 0, time.UTC).Format(time.RFC3339)]
	require.True(t, ok)
	// 18:30 overlaps the 19:00 booking (120m) and near-term occupancy.
	assert.Equal(t, 5, early.AvailableTables)
	assert.NotEqual(t, matched.AvailableTables, 9)
}

func TestGetAvailabilitySkipsSplitShiftGapAndHonorsKitchenClose(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	now := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	business := createTestHospitalityBusiness(t, db, "reservation-split-kitchen")
	createTestTable(t, db, business.ID, "Main", 4)
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.DefaultDuration = 90
		settings.ServiceBufferMinutes = 0
		settings.SlotIntervalMinutes = 30
		settings.MinAdvanceMinutes = 0
	})

	day := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	require.NoError(t, db.Where("business_id = ?", business.ID).Delete(&database.BusinessOperatingHours{}).Error)
	kitchen := "21:00"
	require.NoError(t, db.Create(&database.BusinessOperatingHours{
		BusinessID: business.ID, DayOfWeek: int(day.Weekday()),
		OpenTime: "11:00", CloseTime: "15:00", IsClosed: false,
	}).Error)
	require.NoError(t, db.Create(&database.BusinessOperatingHours{
		BusinessID: business.ID, DayOfWeek: int(day.Weekday()),
		OpenTime: "18:00", CloseTime: "23:00", KitchenCloseTime: &kitchen, IsClosed: false,
	}).Error)

	availability, err := service.GetPublicAvailability(publicBusinessForTest(t, business.ID), day, 2)
	require.NoError(t, err)
	times := make([]string, 0, len(availability.AvailableSlots))
	openTimes := make([]string, 0)
	for _, slot := range availability.AvailableSlots {
		times = append(times, slot.Time)
		if slot.AvailableTables > 0 {
			openTimes = append(openTimes, slot.Time)
		}
	}
	assert.NotContains(t, times, time.Date(2026, 8, 11, 16, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"afternoon gap between lunch and dinner must not be sold")
	assert.Contains(t, openTimes, time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC).Format(time.RFC3339))
	assert.Contains(t, openTimes, time.Date(2026, 8, 11, 18, 0, 0, 0, time.UTC).Format(time.RFC3339))
	// 21:00 kitchen close: 19:30 + 90m ends at 21:00 → bookable; 20:00 + 90m ends 21:30 → not.
	assert.Contains(t, openTimes, time.Date(2026, 8, 11, 19, 30, 0, 0, time.UTC).Format(time.RFC3339))
	assert.NotContains(t, openTimes, time.Date(2026, 8, 11, 20, 0, 0, 0, time.UTC).Format(time.RFC3339))
}

func TestGetAvailabilityHonorsHolidayExceptionClosure(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-holiday")
	createTestTable(t, db, business.ID, "Main", 4)
	configureReservationSettings(t, business.ID, nil)

	day := nextDayAt(19, 0)
	require.NoError(t, db.Create(&database.BusinessOperatingException{
		BusinessID:    business.ID,
		ExceptionDate: time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC),
		IsClosed:      true,
		Label:         "Private event",
	}).Error)

	availability, err := service.GetPublicAvailability(publicBusinessForTest(t, business.ID), day, 2)
	require.NoError(t, err)
	assert.Empty(t, availability.AvailableSlots)
}

// TestReservationServiceUpdateSettings_PartialToggleKeepsLinksAndFields guards
// the pointer-field partial update: toggling one control must not reset the
// partner links or any other stored setting to its zero value.
func TestReservationServiceUpdateSettings_PartialToggleKeepsLinksAndFields(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := createTestHospitalityBusiness(t, db, "reservation-partial-toggle")

	links := `[{"name":"Partner","url":"https://partner.example/book"}]`
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.Enabled = true
		settings.MaxPartySize = 9
		settings.MinAdvanceMinutes = 45
		settings.ApprovalMode = "manual"
		settings.AllowWaitlist = true
		settings.ReminderHoursBefore = 6
		settings.ExternalPartnerLinks = database.JSONRawMessage(links)
	})

	disabled := false
	updated, err := service.UpdateSettings(business.ID, UpdateReservationSettingsInput{Enabled: &disabled})
	require.NoError(t, err)
	assert.False(t, updated.Enabled)

	reloaded, err := service.GetSettings(business.ID)
	require.NoError(t, err)
	assert.False(t, reloaded.Enabled)
	assert.Equal(t, 9, reloaded.MaxPartySize)
	assert.Equal(t, 45, reloaded.MinAdvanceMinutes)
	assert.Equal(t, "manual", reloaded.ApprovalMode)
	assert.True(t, reloaded.AllowWaitlist)
	assert.Equal(t, 6, reloaded.ReminderHoursBefore)
	assert.JSONEq(t, links, string(reloaded.ExternalPartnerLinks))

	enabled := true
	_, err = service.UpdateSettings(business.ID, UpdateReservationSettingsInput{Enabled: &enabled})
	require.NoError(t, err)
	reloaded, err = service.GetSettings(business.ID)
	require.NoError(t, err)
	assert.True(t, reloaded.Enabled)
	assert.JSONEq(t, links, string(reloaded.ExternalPartnerLinks))
	assert.Equal(t, "manual", reloaded.ApprovalMode)
}

// publicBusinessForTest loads the storefront projection the public reservation
// handlers pass to GetPublicSettingsForBusiness / GetPublicAvailability.
func publicBusinessForTest(tb testing.TB, businessID uint) *database.Business {
	tb.Helper()
	business, err := database.GetPublicBusinessByID(businessID)
	require.NoError(tb, err)
	return business
}
