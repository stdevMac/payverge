package services

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
)

type reminderSweepSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *reminderSweepSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func (r *reminderSweepSQLRecorder) businessSelect() string {
	for _, statement := range r.statements {
		normalized := strings.ToLower(statement)
		if strings.Contains(normalized, "from `businesses`") || strings.Contains(normalized, "from \"businesses\"") {
			return statement
		}
	}
	return ""
}

func setupReminderProjectionDB(t testing.TB, recorder logger.Interface) *gorm.DB {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: recorder})
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
		&database.ReservationSettings{},
		&database.TableReservation{},
	))
	return gormDB
}

// getBusinessesWithReservationsEnabled must project only the scalar columns the
// reminder sweep actually reads off the Business row. A SELECT * here hydrates
// the full ~90-column row (onboarding_state JSON, welcome_message, about_story,
// stripe_*, ai_* blobs) for every reservation-enabled business, every hour.
func TestGetBusinessesWithReservationsEnabledProjectsLeanColumns(t *testing.T) {
	recorder := &reminderSweepSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	gormDB := setupReminderProjectionDB(t, recorder)

	business := &database.Business{
		BusinessId:      "biz_projection",
		OwnerAddress:    "owner_projection",
		Name:            "Projection Bistro",
		SettlementAddr:  "0xsettlement",
		TippingAddr:     "0xtipping",
		Phone:           "+15551234567",
		Timezone:        "America/New_York",
		DefaultLanguage: "es",
		Address: database.BusinessAddress{
			Street:     "1 Main St",
			City:       "Townsville",
			State:      "NY",
			PostalCode: "10001",
			Country:    "USA",
		},
		// Heavy fields that MUST NOT be selected by the sweep.
		WelcomeMessage:  strings.Repeat("welcome ", 256),
		AboutStory:      strings.Repeat("story ", 256),
		OnboardingState: database.JSONRawMessage(`{"step":"done"}`),
		IsActive:        true,
	}
	require.NoError(t, gormDB.Create(business).Error)

	settings := &database.ReservationSettings{
		BusinessID:           business.ID,
		Enabled:              true,
		SendReminderEmail:    true,
		MaxAdvanceDays:       30,
		ReminderHoursBefore:  24,
		ExternalPartnerLinks: database.JSONRawMessage("[]"),
	}
	require.NoError(t, gormDB.Create(settings).Error)

	service := NewReservationReminderService()
	businesses, err := service.getBusinessesWithReservationsEnabled()
	require.NoError(t, err)
	require.Len(t, businesses, 1)

	// Functional correctness: every field sendReminderEmail reads is hydrated.
	got := businesses[0]
	assert.Equal(t, business.ID, got.ID)
	assert.Equal(t, "Projection Bistro", got.Name)
	assert.Equal(t, "+15551234567", got.Phone)
	assert.Equal(t, "America/New_York", got.Timezone)
	assert.Equal(t, "es", got.DefaultLanguage)
	assert.Equal(t, "1 Main St", got.Address.Street)
	assert.Equal(t, "Townsville", got.Address.City)
	assert.Equal(t, "NY", got.Address.State)
	assert.Equal(t, "10001", got.Address.PostalCode)
	assert.Equal(t, "USA", got.Address.Country)
	// Heavy blobs are NOT hydrated (proof the projection is lean).
	assert.Empty(t, got.WelcomeMessage, "reminder sweep must not hydrate welcome_message")
	assert.Empty(t, got.AboutStory, "reminder sweep must not hydrate about_story")

	// SQL-shape assertions: lean projection, not SELECT *.
	stmt := recorder.businessSelect()
	require.NotEmpty(t, stmt, "expected a SELECT against businesses")
	lower := strings.ToLower(stmt)
	assert.NotContains(t, lower, "select *", "reminder sweep must not SELECT * the business row")
	assert.NotContains(t, lower, "onboarding_state", "reminder sweep must not select onboarding_state blob")
	assert.NotContains(t, lower, "welcome_message", "reminder sweep must not select welcome_message")
	assert.NotContains(t, lower, "about_story", "reminder sweep must not select about_story")
	// Required columns are present in the projection.
	for _, col := range []string{"name", "phone", "timezone", "default_language", "street", "city", "state", "postal_code", "country"} {
		assert.Contains(t, lower, col, "projection must include column %q", col)
	}
}

// BenchmarkReminderSweepBusinessLoadSQLite measures the per-sweep cost of loading
// reservation-enabled businesses + their settings (the 1 + N read shape, before
// the per-business reservation scan). It exercises the projection (B/op) and the
// settings load (write-on-read elimination).
func BenchmarkReminderSweepBusinessLoadSQLite(b *testing.B) {
	gormDB := setupReminderProjectionDB(b, logger.Default.LogMode(logger.Silent))

	const businessCount = 50
	for i := 0; i < businessCount; i++ {
		business := &database.Business{
			BusinessId:      fmt.Sprintf("biz_bench_%03d", i),
			OwnerAddress:    fmt.Sprintf("owner_bench_%03d", i),
			Name:            fmt.Sprintf("Bench Bistro %03d", i),
			SettlementAddr:  "0xsettlement",
			TippingAddr:     "0xtipping",
			Phone:           "+15550000000",
			Timezone:        "America/New_York",
			DefaultLanguage: "en",
			Address: database.BusinessAddress{
				Street: "1 Main St", City: "Townsville", State: "NY", PostalCode: "10001", Country: "USA",
			},
			// Heavy blobs the old SELECT * dragged along every hour.
			WelcomeMessage:  strings.Repeat("welcome ", 512),
			AboutStory:      strings.Repeat("story ", 512),
			OnboardingState: database.JSONRawMessage(strings.Repeat(`{"k":"v"},`, 64)),
			SocialMedia:     strings.Repeat("social ", 256),
			BannerImages:    strings.Repeat("banner ", 256),
			IsActive:        true,
		}
		require.NoError(b, gormDB.Create(business).Error)
		require.NoError(b, gormDB.Create(&database.ReservationSettings{
			BusinessID:           business.ID,
			Enabled:              true,
			SendReminderEmail:    true,
			MaxAdvanceDays:       0, // would trip needsUpdate → write-on-read in the persist variant
			ReminderHoursBefore:  24,
			ExternalPartnerLinks: database.JSONRawMessage("[]"),
		}).Error)
	}

	service := NewReservationReminderService()

	b.ReportAllocs()
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		businesses, err := service.getBusinessesWithReservationsEnabled()
		if err != nil {
			b.Fatal(err)
		}
		for _, biz := range businesses {
			// Mirror the per-business settings load the sweep performs (read variant).
			if _, err := database.GetReservationSettingsForRead(biz.ID); err != nil {
				b.Fatal(err)
			}
		}
	}
}
