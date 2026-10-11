package services

import (
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

func setupReservationSettingsPerfDB(t testing.TB) *gorm.DB {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	gormDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	database.InitTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.BusinessOperatingHours{},
		&database.Table{},
		&database.ReservationSettings{},
		&database.TableReservation{},
	))

	return gormDB
}

func createReservationSettingsPerfBusiness(t testing.TB, gormDB *gorm.DB, slug string) *database.Business {
	t.Helper()

	business := &database.Business{
		BusinessId:          fmt.Sprintf("biz_%s_%d", slug, time.Now().UnixNano()),
		OwnerAddress:        fmt.Sprintf("owner_%s_%d", slug, time.Now().UnixNano()),
		Name:                fmt.Sprintf("Business %s", slug),
		SettlementAddr:      "0xsettlement",
		TippingAddr:         "0xtipping",
		Timezone:            "UTC",
		IsActive:            true,
		BusinessPageEnabled: true,
	}
	require.NoError(t, gormDB.Create(business).Error)

	for day := 0; day < 7; day++ {
		require.NoError(t, gormDB.Create(&database.BusinessOperatingHours{
			BusinessID: business.ID,
			DayOfWeek:  day,
			OpenTime:   "10:00",
			CloseTime:  "22:00",
			IsClosed:   false,
		}).Error)
	}

	settings := &database.ReservationSettings{
		BusinessID:           business.ID,
		Enabled:              true,
		MaxAdvanceDays:       30,
		MinAdvanceMinutes:    0,
		MinPartySize:         1,
		MaxPartySize:         12,
		DefaultDuration:      60,
		SlotIntervalMinutes:  30,
		ServiceBufferMinutes: 15,
		AutoAssignTables:     true,
		ApprovalMode:         database.ReservationApprovalManual,
		AllowWaitlist:        true,
		AllowCancellation:    true,
		ExternalPartnerLinks: database.JSONRawMessage("[]"),
	}
	require.NoError(t, gormDB.Create(settings).Error)

	for i := 0; i < 24; i++ {
		require.NoError(t, gormDB.Create(&database.Table{
			BusinessID: business.ID,
			TableCode:  fmt.Sprintf("SET-PERF-%02d", i),
			Name:       fmt.Sprintf("Settings Perf Table %02d", i),
			Capacity:   2 + (i % 6),
			IsActive:   true,
		}).Error)
	}

	start := time.Now().UTC().AddDate(0, 0, 1).Truncate(24 * time.Hour).Add(10 * time.Hour)
	for day := 0; day < 7; day++ {
		for i := 0; i < 48; i++ {
			tableID := uint((i % 24) + 1)
			require.NoError(t, gormDB.Create(&database.TableReservation{
				BusinessID:       business.ID,
				TableID:          &tableID,
				CustomerName:     fmt.Sprintf("Summary Guest %d-%d", day, i),
				CustomerEmail:    fmt.Sprintf("summary-%d-%d@example.com", day, i),
				PartySize:        2 + (i % 4),
				ReservationTime:  start.AddDate(0, 0, day).Add(time.Duration(i%24) * 30 * time.Minute),
				Duration:         60,
				Status:           "confirmed",
				Source:           "staff",
				ConfirmationCode: fmt.Sprintf("SUMMARY-%d-%d", day, i),
			}).Error)
		}
	}

	return business
}

func TestReservationServicePublicSettingsReusesAvailabilityContext(t *testing.T) {
	gormDB := setupReservationSettingsPerfDB(t)
	service := NewReservationService(gormDB)
	business := createReservationSettingsPerfBusiness(t, gormDB, "settings-query-shape")

	queryCount := 0
	callbackName := "payverge:test_reservation_settings_query_counter"
	require.NoError(t, gormDB.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		queryCount++
	}))
	t.Cleanup(func() {
		_ = gormDB.Callback().Query().Remove(callbackName)
	})

	settings, err := service.GetPublicSettingsForBusiness(publicBusinessForTest(t, business.ID))
	require.NoError(t, err)
	require.NotNil(t, settings)
	assert.LessOrEqual(t, queryCount, 8, "settings availability summary should not reload business/settings/tables or re-query hours/conflicts per lookahead day")
}

func BenchmarkGetPublicReservationSettingsSQLite(b *testing.B) {
	gormDB := setupReservationSettingsPerfDB(b)
	service := NewReservationService(gormDB)
	business := createReservationSettingsPerfBusiness(b, gormDB, "settings-bench")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		settings, err := service.GetPublicSettingsForBusiness(publicBusinessForTest(b, business.ID))
		if err != nil {
			b.Fatal(err)
		}
		if settings.AvailableSlotCount == 0 {
			b.Fatal("expected available slot summary")
		}
	}
}
