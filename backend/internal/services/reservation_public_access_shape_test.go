package services

import (
	"context"
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

type publicReservationSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *publicReservationSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
	if r.Interface != nil {
		r.Interface.Trace(ctx, begin, fc, err)
	}
}

func (r *publicReservationSQLRecorder) reset() {
	r.statements = nil
}

func (r *publicReservationSQLRecorder) matching(needles ...string) []string {
	var out []string
	for _, statement := range r.statements {
		normalized := strings.ToLower(statement)
		ok := true
		for _, needle := range needles {
			if !strings.Contains(normalized, strings.ToLower(needle)) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, statement)
		}
	}
	return out
}

func setupPublicReservationAccessShapeDB(t *testing.T) (*gorm.DB, *publicReservationSQLRecorder) {
	t.Helper()

	recorder := &publicReservationSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	db := setupHospitalityServiceTestDB(t)
	db = db.Session(&gorm.Session{Logger: recorder})
	database.InitTestDB(db)
	return db, recorder
}

func seedPublicReservationAccessShapeBusiness(t *testing.T, db *gorm.DB, slug string) *database.Business {
	t.Helper()

	business := createTestHospitalityBusiness(t, db, slug)
	createTestTable(t, db, business.ID, "T-"+slug, 4)
	configureReservationSettings(t, business.ID, func(settings *database.ReservationSettings) {
		settings.DefaultDuration = 60
		settings.ServiceBufferMinutes = 15
		settings.SlotIntervalMinutes = 30
		settings.MinAdvanceMinutes = 0
	})
	require.NoError(t, db.Model(&database.BusinessOperatingHours{}).
		Where("business_id = ?", business.ID).
		Updates(map[string]interface{}{
			"open_time":  "10:00",
			"close_time": "22:00",
			"is_closed":  false,
		}).Error)
	return business
}

// TestLoadReservationConflictsOmitsGuestPII is the #558 projection gate:
// availability conflict reads must not SELECT guest PII columns.
func TestLoadReservationConflictsOmitsGuestPII(t *testing.T) {
	db, recorder := setupPublicReservationAccessShapeDB(t)
	service := NewReservationService(db)
	business := seedPublicReservationAccessShapeBusiness(t, db, "resv-conflict-pii")

	slot := time.Now().UTC().AddDate(0, 0, 1).Truncate(time.Hour)
	secretEmail := fmt.Sprintf("pii-%d@example.com", time.Now().UnixNano())
	require.NoError(t, db.Create(&database.TableReservation{
		BusinessID:       business.ID,
		CustomerName:     "Secret Guest",
		CustomerEmail:    secretEmail,
		CustomerPhone:    "5550009999",
		PartySize:        2,
		ReservationTime:  slot,
		Duration:         60,
		Status:           "confirmed",
		Source:           "staff",
		ConfirmationCode: "PII-SHAPE-1",
		SpecialRequests:  "allergy peanuts",
		Notes:            "vip notes must stay in the database",
	}).Error)

	ctx, err := service.loadAvailabilityContext(business.ID)
	require.NoError(t, err)

	recorder.reset()
	conflicts, err := service.loadReservationConflicts(ctx, slot, slot.Add(2*time.Hour), 0)
	require.NoError(t, err)
	require.Len(t, conflicts.all, 1)
	assert.Empty(t, conflicts.all[0].CustomerEmail)
	assert.Empty(t, conflicts.all[0].CustomerPhone)
	assert.Empty(t, conflicts.all[0].SpecialRequests)
	assert.Empty(t, conflicts.all[0].Notes)
	assert.Equal(t, 2, conflicts.all[0].PartySize)

	queries := recorder.matching("table_reservations")
	require.NotEmpty(t, queries)
	for _, statement := range queries {
		normalized := strings.ToLower(statement)
		assert.NotContains(t, normalized, "select *",
			"conflict read must project scheduling columns, not SELECT *: %s", statement)
		for _, col := range []string{"customer_email", "customer_phone", "special_requests", "notes"} {
			assert.NotContains(t, normalized, col,
				"conflict read must not select %s: %s", col, statement)
		}
	}
}

// TestGetPublicSettingsDoesNotNPlusOnePerDay is the #558 day-loop gate:
// the 7-day settings summary must not issue per-day operating-hours or
// reservation-conflict queries.
func TestGetPublicSettingsDoesNotNPlusOnePerDay(t *testing.T) {
	db, recorder := setupPublicReservationAccessShapeDB(t)
	service := NewReservationService(db)
	business := seedPublicReservationAccessShapeBusiness(t, db, "resv-settings-nplusone")

	start := time.Now().UTC().AddDate(0, 0, 1).Truncate(24 * time.Hour).Add(12 * time.Hour)
	for day := 0; day < 7; day++ {
		require.NoError(t, db.Create(&database.TableReservation{
			BusinessID:       business.ID,
			CustomerName:     fmt.Sprintf("Day Guest %d", day),
			CustomerEmail:    fmt.Sprintf("day-%d@example.com", day),
			CustomerPhone:    "5551112222",
			PartySize:        2,
			ReservationTime:  start.AddDate(0, 0, day),
			Duration:         60,
			Status:           "confirmed",
			Source:           "staff",
			ConfirmationCode: fmt.Sprintf("NPLUS-%d", day),
			SpecialRequests:  "window",
			Notes:            "internal",
		}).Error)
	}

	recorder.reset()
	settings, err := service.GetPublicSettingsForBusiness(publicBusinessForTest(t, business.ID))
	require.NoError(t, err)
	require.NotNil(t, settings)

	hoursByDay := recorder.matching("business_operating_hours", "day_of_week =")
	assert.Empty(t, hoursByDay,
		"operating hours must be loaded once for all days, not queried per weekday: %v", hoursByDay)

	hoursAll := recorder.matching("from", "business_operating_hours")
	assert.LessOrEqual(t, len(hoursAll), 1,
		"public settings should issue at most one operating-hours read, got %d: %v", len(hoursAll), hoursAll)

	conflictQueries := recorder.matching("table_reservations", "status")
	assert.LessOrEqual(t, len(conflictQueries), 1,
		"public settings should issue one windowed conflicts read, not one per day, got %d: %v",
		len(conflictQueries), conflictQueries)

	for _, statement := range conflictQueries {
		normalized := strings.ToLower(statement)
		assert.NotContains(t, normalized, "select *",
			"conflicts query must not SELECT *: %s", statement)
		for _, col := range []string{"customer_email", "customer_phone", "special_requests", "notes"} {
			assert.NotContains(t, normalized, col,
				"conflicts query must not select %s: %s", col, statement)
		}
	}
}

// TestGetPublicSettingsDoesNotWriteOnRead is the service-level twin of the
// handler write-on-read gate: GetPublicSettings must not persist normalize.
func TestGetPublicSettingsDoesNotWriteOnRead(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := seedPublicReservationAccessShapeBusiness(t, db, "resv-settings-nowrite")

	require.NoError(t, db.Model(&database.ReservationSettings{}).
		Where("business_id = ?", business.ID).
		Update("max_advance_days", 0).Error)

	writes := 0
	createName := "payverge:test_public_settings_fail_create"
	updateName := "payverge:test_public_settings_fail_update"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(createName, func(tx *gorm.DB) {
		writes++
		t.Errorf("unexpected INSERT against %s during GetPublicSettings", tx.Statement.Table)
	}))
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(updateName, func(tx *gorm.DB) {
		writes++
		t.Errorf("unexpected UPDATE against %s during GetPublicSettings", tx.Statement.Table)
	}))
	t.Cleanup(func() {
		_ = db.Callback().Create().Remove(createName)
		_ = db.Callback().Update().Remove(updateName)
	})

	settings, err := service.GetPublicSettingsForBusiness(publicBusinessForTest(t, business.ID))
	require.NoError(t, err)
	require.NotNil(t, settings)
	assert.Equal(t, 30, settings.MaxAdvanceDays)
	assert.Equal(t, 0, writes)

	var persisted database.ReservationSettings
	require.NoError(t, db.Where("business_id = ?", business.ID).First(&persisted).Error)
	assert.Equal(t, 0, persisted.MaxAdvanceDays)
}

func TestGetAvailabilityDoesNotWriteOnRead(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	service := NewReservationService(db)
	business := seedPublicReservationAccessShapeBusiness(t, db, "resv-avail-nowrite")

	require.NoError(t, db.Model(&database.ReservationSettings{}).
		Where("business_id = ?", business.ID).
		Update("max_advance_days", 0).Error)

	writes := 0
	createName := "payverge:test_public_avail_fail_create"
	updateName := "payverge:test_public_avail_fail_update"
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(createName, func(tx *gorm.DB) {
		writes++
		t.Errorf("unexpected INSERT against %s during GetAvailability", tx.Statement.Table)
	}))
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(updateName, func(tx *gorm.DB) {
		writes++
		t.Errorf("unexpected UPDATE against %s during GetAvailability", tx.Statement.Table)
	}))
	t.Cleanup(func() {
		_ = db.Callback().Create().Remove(createName)
		_ = db.Callback().Update().Remove(updateName)
	})

	target := time.Now().UTC().AddDate(0, 0, 1).Truncate(time.Hour)
	_, err := service.GetPublicAvailability(publicBusinessForTest(t, business.ID), target, 2)
	require.NoError(t, err)
	assert.Equal(t, 0, writes)

	var persisted database.ReservationSettings
	require.NoError(t, db.Where("business_id = ?", business.ID).First(&persisted).Error)
	assert.Equal(t, 0, persisted.MaxAdvanceDays)
}
