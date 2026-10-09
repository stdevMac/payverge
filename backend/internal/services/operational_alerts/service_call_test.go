package operational_alerts

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestSameSeatingCheckPlease(t *testing.T) {
	stale := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	t.Run("empty table", func(t *testing.T) {
		require.False(t, SameSeatingCheckPlease("check", stale, time.Time{}, false))
	})
	t.Run("newer bill after yesterday's check", func(t *testing.T) {
		require.False(t, SameSeatingCheckPlease("check", stale, stale.Add(26*time.Hour), true))
	})
	t.Run("same-seating unpaid bill", func(t *testing.T) {
		require.True(t, SameSeatingCheckPlease("check", stale, stale.Add(-20*time.Minute), true))
	})
	t.Run("water is never same-seating check", func(t *testing.T) {
		require.False(t, SameSeatingCheckPlease("water", stale, stale.Add(-20*time.Minute), true))
	})
}

func TestServiceCallTTLExemptIsClaimedOnly(t *testing.T) {
	require.False(t, ServiceCallTTLExempt(database.OperationalAlertStatusOpen, "check"))
	require.True(t, ServiceCallTTLExempt(database.OperationalAlertStatusClaimed, "water"))
}

func TestCreateServiceCallAlertUpsertsPerTable(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	require.NoError(t, svc.CreateServiceCallAlert(ctx, business.ID, 5, "Table 5", "water"))

	var alerts []database.OperationalAlert
	require.NoError(t, db.Where("business_id = ? AND alert_type = ?", business.ID, database.OperationalAlertTypeServiceCall).Find(&alerts).Error)
	require.Len(t, alerts, 1)
	first := alerts[0]
	require.Equal(t, database.OperationalAlertStatusOpen, first.Status)
	require.Equal(t, database.OperationalAlertResourceTypeTable, first.ResourceType)
	require.Equal(t, int64(5), first.ResourceID)
	require.Equal(t, database.OperationalAlertPriorityUrgent, first.Priority)
	require.Contains(t, string(first.Metadata), `"reason":"water"`)

	firstEventAt := first.LastEventAt

	// Second call on the same table while still open must upsert, not
	// duplicate.
	require.NoError(t, svc.CreateServiceCallAlert(ctx, business.ID, 5, "Table 5", "order"))

	require.NoError(t, db.Where("business_id = ? AND alert_type = ?", business.ID, database.OperationalAlertTypeServiceCall).Find(&alerts).Error)
	require.Len(t, alerts, 1)
	require.False(t, alerts[0].LastEventAt.Before(firstEventAt))
	require.Contains(t, string(alerts[0].Metadata), `"reason":"order"`)
	require.Contains(t, alerts[0].Body, "order")
}

func TestServiceCallStatusProjection(t *testing.T) {
	db, business, staff := setupServiceTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	status, _, resolvedAt, err := svc.GetServiceCallStatus(ctx, business.ID, 7)
	require.NoError(t, err)
	require.Equal(t, ServiceCallStatusNone, status)
	require.Nil(t, resolvedAt)

	require.NoError(t, svc.CreateServiceCallAlert(ctx, business.ID, 7, "Table 7", "check"))
	status, reason, resolvedAt, err := svc.GetServiceCallStatus(ctx, business.ID, 7)
	require.NoError(t, err)
	require.Equal(t, ServiceCallStatusOpen, status)
	require.Equal(t, "check", reason)
	require.Nil(t, resolvedAt)

	var alert database.OperationalAlert
	require.NoError(t, db.Where("business_id = ? AND alert_type = ? AND resource_id = ?",
		business.ID, database.OperationalAlertTypeServiceCall, 7).First(&alert).Error)

	_, err = svc.ClaimAlertForBusiness(ctx, business.ID, alert.ID, Actor{StaffID: &staff.ID, Name: staff.Name}, "manual")
	require.NoError(t, err)
	status, reason, resolvedAt, err = svc.GetServiceCallStatus(ctx, business.ID, 7)
	require.NoError(t, err)
	require.Equal(t, ServiceCallStatusAcknowledged, status)
	require.Equal(t, "check", reason)
	require.Nil(t, resolvedAt)

	_, err = svc.ResolveAlertByIDForBusiness(ctx, business.ID, alert.ID, Actor{StaffID: &staff.ID, Name: staff.Name}, "handled")
	require.NoError(t, err)
	status, reason, resolvedAt, err = svc.GetServiceCallStatus(ctx, business.ID, 7)
	require.NoError(t, err)
	require.Equal(t, ServiceCallStatusResolved, status)
	require.Equal(t, "check", reason)
	require.NotNil(t, resolvedAt)
}

func TestGetServiceCallStatusTreatsPastSLAAsNone(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	require.NoError(t, svc.CreateServiceCallAlert(ctx, business.ID, 3, "Table 3", "water"))
	stale := time.Now().Add(-ServiceCallTTL - time.Minute)
	require.NoError(t, db.Model(&database.OperationalAlert{}).
		Where("business_id = ? AND alert_type = ? AND resource_id = ?",
			business.ID, database.OperationalAlertTypeServiceCall, 3).
		Update("last_event_at", stale).Error)

	status, reason, resolvedAt, err := svc.GetServiceCallStatus(ctx, business.ID, 3)
	require.NoError(t, err)
	require.Equal(t, ServiceCallStatusNone, status)
	require.Empty(t, reason)
	require.Nil(t, resolvedAt)
}

func TestGetServiceCallStatusKeepsLiveCallInsideSLA(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	require.NoError(t, svc.CreateServiceCallAlert(ctx, business.ID, 4, "Table 4", "order"))
	fresh := time.Now().Add(-30 * time.Minute)
	require.NoError(t, db.Model(&database.OperationalAlert{}).
		Where("business_id = ? AND alert_type = ? AND resource_id = ?",
			business.ID, database.OperationalAlertTypeServiceCall, 4).
		Update("last_event_at", fresh).Error)

	status, reason, resolvedAt, err := svc.GetServiceCallStatus(ctx, business.ID, 4)
	require.NoError(t, err)
	require.Equal(t, ServiceCallStatusOpen, status)
	require.Equal(t, "order", reason)
	require.Nil(t, resolvedAt)
}

func TestListAlertsOmitsStaleServiceCalls(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	require.NoError(t, svc.CreateServiceCallAlert(ctx, business.ID, 8, "Table 8", "water"))
	require.NoError(t, svc.CreateServiceCallAlert(ctx, business.ID, 9, "Table 9", "check"))
	stale := time.Now().Add(-2 * time.Hour)
	require.NoError(t, db.Model(&database.OperationalAlert{}).
		Where("business_id = ? AND resource_id = ?", business.ID, 8).
		Update("last_event_at", stale).Error)

	alerts, err := svc.ListActiveAlerts(ctx, business.ID, nil, []database.OperationalAlertType{
		database.OperationalAlertTypeServiceCall,
	})
	require.NoError(t, err)
	require.Len(t, alerts, 1)
	require.Equal(t, int64(9), alerts[0].ResourceID)
}

func TestExpireStaleServiceCallsResolvesPastSLA(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	require.NoError(t, svc.CreateServiceCallAlert(ctx, business.ID, 11, "Table 11", "water"))
	require.NoError(t, svc.CreateServiceCallAlert(ctx, business.ID, 12, "Table 12", "order"))
	stale := time.Now().Add(-ServiceCallTTL - time.Second)
	require.NoError(t, db.Model(&database.OperationalAlert{}).
		Where("business_id = ? AND resource_id = ?", business.ID, 11).
		Update("last_event_at", stale).Error)

	n, err := svc.ExpireStaleServiceCalls(ctx, time.Now())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	var staleRow, liveRow database.OperationalAlert
	require.NoError(t, db.Where("resource_id = ?", 11).First(&staleRow).Error)
	require.NoError(t, db.Where("resource_id = ?", 12).First(&liveRow).Error)
	require.Equal(t, database.OperationalAlertStatusResolved, staleRow.Status)
	require.NotNil(t, staleRow.ResolvedAt)
	require.Equal(t, database.OperationalAlertStatusOpen, liveRow.Status)

	var event database.OperationalAlertEvent
	require.NoError(t, db.Where("alert_id = ? AND event_type = ?",
		staleRow.ID, database.OperationalAlertEventTypeResolved).First(&event).Error)
	require.Contains(t, string(event.Metadata), ServiceCallExpireReason)

	// Second sweep is a no-op.
	n, err = svc.ExpireStaleServiceCalls(ctx, time.Now())
	require.NoError(t, err)
	require.Equal(t, 0, n)
}

func TestExpireStaleServiceCallsExpiresEmptyTableCheckPlease(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	require.NoError(t, svc.CreateServiceCallAlert(ctx, business.ID, 21, "Table 21", "check"))
	stale := time.Now().Add(-2 * time.Hour)
	require.NoError(t, db.Model(&database.OperationalAlert{}).
		Where("resource_id = ?", 21).
		Update("last_event_at", stale).Error)

	n, err := svc.ExpireStaleServiceCalls(ctx, time.Now())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	status, reason, _, err := svc.GetServiceCallStatus(ctx, business.ID, 21)
	require.NoError(t, err)
	require.Equal(t, ServiceCallStatusNone, status)
	require.Empty(t, reason)

	var row database.OperationalAlert
	require.NoError(t, db.Where("resource_id = ?", 21).First(&row).Error)
	require.Equal(t, database.OperationalAlertStatusResolved, row.Status)
}

func TestExpireStaleServiceCallsKeepsSameSeatingCheckPlease(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.Bill{}))
	svc := NewService(db)
	ctx := context.Background()

	require.NoError(t, svc.CreateServiceCallAlert(ctx, business.ID, 28, "Table 28", "check"))
	stale := time.Now().Add(-2 * time.Hour)
	require.NoError(t, db.Model(&database.OperationalAlert{}).
		Where("resource_id = ?", 28).
		Update("last_event_at", stale).Error)
	require.NoError(t, db.Create(&database.Bill{
		BusinessID:     business.ID,
		TableID:        28,
		BillNumber:     "OPEN-28-" + t.Name(),
		Status:         database.BillStatusOpen,
		SettlementAddr: "settle",
		TippingAddr:    "tip",
		Items:          "[]",
		CreatedAt:      stale.Add(-15 * time.Minute),
	}).Error)

	n, err := svc.ExpireStaleServiceCalls(ctx, time.Now())
	require.NoError(t, err)
	require.Equal(t, 0, n)

	status, reason, _, err := svc.GetServiceCallStatus(ctx, business.ID, 28)
	require.NoError(t, err)
	require.Equal(t, ServiceCallStatusOpen, status)
	require.Equal(t, "check", reason)
}

func TestExpireStaleServiceCallsLeftoverKitchenIsNotAKeep(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.Bill{}, &database.Order{}))
	svc := NewService(db)
	ctx := context.Background()

	require.NoError(t, svc.CreateServiceCallAlert(ctx, business.ID, 33, "Table 33", "check"))
	stale := time.Now().Add(-2 * time.Hour)
	require.NoError(t, db.Model(&database.OperationalAlert{}).
		Where("resource_id = ?", 33).
		Update("last_event_at", stale).Error)
	abandoned := database.Bill{
		BusinessID:     business.ID,
		TableID:        33,
		BillNumber:     "ABANDON-33-" + t.Name(),
		Status:         database.BillStatusAbandoned,
		SettlementAddr: "settle",
		TippingAddr:    "tip",
		Items:          "[]",
		CreatedAt:      stale.Add(-time.Hour),
	}
	require.NoError(t, db.Create(&abandoned).Error)
	require.NoError(t, db.Create(&database.Order{
		BillID:      abandoned.ID,
		BusinessID:  business.ID,
		OrderNumber: "K-33-" + t.Name(),
		Status:      database.OrderStatusInKitchen,
		Items:       "[]",
	}).Error)

	n, err := svc.ExpireStaleServiceCalls(ctx, time.Now())
	require.NoError(t, err)
	require.Equal(t, 1, n, "leftover-kitchen host-stand occupied is not TTL occupancy")

	status, _, _, err := svc.GetServiceCallStatus(ctx, business.ID, 33)
	require.NoError(t, err)
	require.Equal(t, ServiceCallStatusNone, status)
}

func TestExpireStaleServiceCallsExpiresCheckPleaseOnNewerBill(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.Bill{}))
	svc := NewService(db)
	ctx := context.Background()

	require.NoError(t, svc.CreateServiceCallAlert(ctx, business.ID, 29, "Table 29", "check"))
	stale := time.Now().Add(-26 * time.Hour)
	require.NoError(t, db.Model(&database.OperationalAlert{}).
		Where("resource_id = ?", 29).
		Update("last_event_at", stale).Error)
	require.NoError(t, db.Create(&database.Bill{
		BusinessID:     business.ID,
		TableID:        29,
		BillNumber:     "NEW-29-" + t.Name(),
		Status:         database.BillStatusOpen,
		SettlementAddr: "settle",
		TippingAddr:    "tip",
		Items:          "[]",
		CreatedAt:      time.Now().Add(-10 * time.Minute),
	}).Error)

	n, err := svc.ExpireStaleServiceCalls(ctx, time.Now())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	status, _, _, err := svc.GetServiceCallStatus(ctx, business.ID, 29)
	require.NoError(t, err)
	require.Equal(t, ServiceCallStatusNone, status)
}

func TestExpireStaleServiceCallsKeepsClaimed(t *testing.T) {
	db, business, staff := setupServiceTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	require.NoError(t, svc.CreateServiceCallAlert(ctx, business.ID, 22, "Table 22", "water"))
	var alert database.OperationalAlert
	require.NoError(t, db.Where("resource_id = ?", 22).First(&alert).Error)
	_, err := svc.ClaimAlertForBusiness(ctx, business.ID, alert.ID, Actor{StaffID: &staff.ID, Name: staff.Name}, "test")
	require.NoError(t, err)
	stale := time.Now().Add(-2 * time.Hour)
	require.NoError(t, db.Model(&database.OperationalAlert{}).
		Where("id = ?", alert.ID).
		Update("last_event_at", stale).Error)

	n, err := svc.ExpireStaleServiceCalls(ctx, time.Now())
	require.NoError(t, err)
	require.Equal(t, 0, n)

	status, _, _, err := svc.GetServiceCallStatus(ctx, business.ID, 22)
	require.NoError(t, err)
	require.Equal(t, ServiceCallStatusAcknowledged, status)
}

func TestExpireStaleServiceCallsKeepsOpenBillTable(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.Bill{}))
	svc := NewService(db)
	ctx := context.Background()

	require.NoError(t, svc.CreateServiceCallAlert(ctx, business.ID, 23, "Table 23", "water"))
	stale := time.Now().Add(-2 * time.Hour)
	require.NoError(t, db.Model(&database.OperationalAlert{}).
		Where("resource_id = ?", 23).
		Update("last_event_at", stale).Error)
	require.NoError(t, db.Create(&database.Bill{
		BusinessID:     business.ID,
		TableID:        23,
		BillNumber:     "OPEN-23-" + t.Name(),
		Status:         database.BillStatusPartial,
		SettlementAddr: "settle",
		TippingAddr:    "tip",
		Items:          "[]",
		CreatedAt:      stale.Add(-30 * time.Minute),
	}).Error)

	n, err := svc.ExpireStaleServiceCalls(ctx, time.Now())
	require.NoError(t, err)
	require.Equal(t, 0, n)

	var row database.OperationalAlert
	require.NoError(t, db.Where("resource_id = ?", 23).First(&row).Error)
	require.Equal(t, database.OperationalAlertStatusOpen, row.Status)
}

func TestExpireStaleServiceCallsKeepsZeroLastEventAt(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	require.NoError(t, svc.CreateServiceCallAlert(ctx, business.ID, 24, "Table 24", "water"))
	require.NoError(t, db.Model(&database.OperationalAlert{}).
		Where("resource_id = ?", 24).
		Update("last_event_at", time.Time{}).Error)

	n, err := svc.ExpireStaleServiceCalls(ctx, time.Now())
	require.NoError(t, err)
	require.Equal(t, 0, n)

	require.False(t, IsStaleServiceCall(time.Time{}, time.Now()))
	var row database.OperationalAlert
	require.NoError(t, db.Where("resource_id = ?", 24).First(&row).Error)
	require.Equal(t, database.OperationalAlertStatusOpen, row.Status)
}

func TestGetServiceCallStatusEmptyTableStaleCheckIsNone(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	require.NoError(t, svc.CreateServiceCallAlert(ctx, business.ID, 25, "Table 25", "check"))
	require.NoError(t, db.Model(&database.OperationalAlert{}).
		Where("resource_id = ?", 25).
		Update("last_event_at", time.Now().Add(-2*time.Hour)).Error)

	status, reason, _, err := svc.GetServiceCallStatus(ctx, business.ID, 25)
	require.NoError(t, err)
	require.Equal(t, ServiceCallStatusNone, status)
	require.Empty(t, reason)
}

func TestGetServiceCallStatusKeepsSameSeatingCheckPlease(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.Bill{}))
	svc := NewService(db)
	ctx := context.Background()

	stale := time.Now().Add(-2 * time.Hour)
	require.NoError(t, db.Create(&database.Bill{
		BusinessID:     business.ID,
		TableID:        30,
		BillNumber:     "OPEN-30-" + t.Name(),
		Status:         database.BillStatusPartial,
		SettlementAddr: "settle",
		TippingAddr:    "tip",
		Items:          "[]",
		CreatedAt:      stale.Add(-20 * time.Minute),
	}).Error)
	require.NoError(t, svc.CreateServiceCallAlert(ctx, business.ID, 30, "Table 30", "check"))
	require.NoError(t, db.Model(&database.OperationalAlert{}).
		Where("resource_id = ?", 30).
		Update("last_event_at", stale).Error)

	status, reason, _, err := svc.GetServiceCallStatus(ctx, business.ID, 30)
	require.NoError(t, err)
	require.Equal(t, ServiceCallStatusOpen, status)
	require.Equal(t, "check", reason)
}

func TestGetServiceCallStatusNewBillDoesNotInheritStaleCheck(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.Bill{}))
	svc := NewService(db)
	ctx := context.Background()

	require.NoError(t, svc.CreateServiceCallAlert(ctx, business.ID, 31, "Table 31", "check"))
	stale := time.Now().Add(-26 * time.Hour)
	require.NoError(t, db.Model(&database.OperationalAlert{}).
		Where("resource_id = ?", 31).
		Update("last_event_at", stale).Error)
	require.NoError(t, db.Create(&database.Bill{
		BusinessID:     business.ID,
		TableID:        31,
		BillNumber:     "NEW-31-" + t.Name(),
		Status:         database.BillStatusOpen,
		SettlementAddr: "settle",
		TippingAddr:    "tip",
		Items:          "[]",
		CreatedAt:      time.Now().Add(-5 * time.Minute),
	}).Error)

	status, reason, _, err := svc.GetServiceCallStatus(ctx, business.ID, 31)
	require.NoError(t, err)
	require.Equal(t, ServiceCallStatusNone, status)
	require.Empty(t, reason)
}

func TestListAlertsOmitsEmptyTableStaleCheckKeepsClaimed(t *testing.T) {
	db, business, staff := setupServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.Bill{}))
	svc := NewService(db)
	ctx := context.Background()

	require.NoError(t, svc.CreateServiceCallAlert(ctx, business.ID, 26, "Table 26", "check"))
	require.NoError(t, svc.CreateServiceCallAlert(ctx, business.ID, 27, "Table 27", "water"))
	require.NoError(t, svc.CreateServiceCallAlert(ctx, business.ID, 32, "Table 32", "check"))
	var water database.OperationalAlert
	require.NoError(t, db.Where("resource_id = ?", 27).First(&water).Error)
	_, err := svc.ClaimAlertForBusiness(ctx, business.ID, water.ID, Actor{StaffID: &staff.ID, Name: staff.Name}, "test")
	require.NoError(t, err)

	stale := time.Now().Add(-2 * time.Hour)
	require.NoError(t, db.Model(&database.OperationalAlert{}).
		Where("business_id = ? AND alert_type = ?", business.ID, database.OperationalAlertTypeServiceCall).
		Update("last_event_at", stale).Error)
	require.NoError(t, db.Create(&database.Bill{
		BusinessID:     business.ID,
		TableID:        32,
		BillNumber:     "OPEN-32-" + t.Name(),
		Status:         database.BillStatusOpen,
		SettlementAddr: "settle",
		TippingAddr:    "tip",
		Items:          "[]",
		CreatedAt:      stale.Add(-time.Hour),
	}).Error)

	alerts, err := svc.ListActiveAlerts(ctx, business.ID, nil, []database.OperationalAlertType{
		database.OperationalAlertTypeServiceCall,
	})
	require.NoError(t, err)
	require.Len(t, alerts, 2)
	ids := map[int64]bool{}
	for _, alert := range alerts {
		ids[alert.ResourceID] = true
	}
	require.True(t, ids[27], "claimed water stays")
	require.True(t, ids[32], "same-seating check-please stays")
	require.False(t, ids[26], "empty-table stale check-please must leave the live list")
}

func TestCreateServiceCallAfterSLAStartsFreshRow(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	require.NoError(t, svc.CreateServiceCallAlert(ctx, business.ID, 15, "Table 15", "water"))
	var first database.OperationalAlert
	require.NoError(t, db.Where("resource_id = ?", 15).First(&first).Error)
	stale := time.Now().Add(-2 * time.Hour)
	require.NoError(t, db.Model(&first).Update("last_event_at", stale).Error)

	require.NoError(t, svc.CreateServiceCallAlert(ctx, business.ID, 15, "Table 15", "check"))

	var alerts []database.OperationalAlert
	require.NoError(t, db.Where("business_id = ? AND alert_type = ? AND resource_id = ?",
		business.ID, database.OperationalAlertTypeServiceCall, 15).
		Order("id ASC").Find(&alerts).Error)
	require.Len(t, alerts, 2)
	require.Equal(t, database.OperationalAlertStatusResolved, alerts[0].Status)
	require.Equal(t, database.OperationalAlertStatusOpen, alerts[1].Status)
	require.Contains(t, string(alerts[1].Metadata), `"reason":"check"`)
	require.True(t, alerts[1].LastEventAt.After(stale.Add(time.Hour)))
}

func TestServiceCallSettingsSelfHeal(t *testing.T) {
	db, business, _ := setupServiceTestDB(t)
	svc := NewService(db)
	ctx := context.Background()

	// Simulate a business row created before service_call existed: default
	// settings for every OTHER known type, but service_call left zero-value.
	stale := database.BusinessAlertSettings{
		BusinessID:                  business.ID,
		Enabled:                     true,
		BrowserNotificationsEnabled: true,
		SoundEnabled:                true,
		Volume:                      0.8,
		RepeatIntervalSeconds:       8,
		EventSettings: database.OperationalAlertEventSettings{
			OrderNew: database.AlertTypeSetting{Enabled: true, Repeating: true, Priority: database.OperationalAlertPriorityUrgent},
		},
	}
	require.NoError(t, db.Create(&stale).Error)

	settings, err := svc.GetSettings(ctx, business.ID)
	require.NoError(t, err)
	require.True(t, settings.EventSettings.ServiceCall.Enabled)
	require.True(t, settings.EventSettings.ServiceCall.Repeating)
	require.Equal(t, database.OperationalAlertPriorityUrgent, settings.EventSettings.ServiceCall.Priority)
}
