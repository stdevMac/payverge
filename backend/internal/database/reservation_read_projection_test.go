package database

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
)

// reservationReadSQLRecorder captures emitted SQL so the projection of the
// embedded Business on reservation reads can be asserted at the query layer
// (PRELOAD-04). Mirrors orders_list_perf_test.go's column-capture helpers.
type reservationReadSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *reservationReadSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func (r *reservationReadSQLRecorder) statementSelectsFrom(table string, statement string) bool {
	normalized := strings.ToLower(strings.TrimSpace(statement))
	return strings.Contains(normalized, "from `"+table+"`") ||
		strings.Contains(normalized, "from \""+table+"\"") ||
		strings.Contains(normalized, "from "+table)
}

func (r *reservationReadSQLRecorder) selectStarCount(table string) int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.TrimSpace(statement))
		if !strings.HasPrefix(normalized, "select *") {
			continue
		}
		if r.statementSelectsFrom(table, statement) {
			count++
		}
	}
	return count
}

func (r *reservationReadSQLRecorder) businessSelectMentions(column string) bool {
	needleBacktick := "`" + strings.ToLower(column) + "`"
	needleDouble := `"` + strings.ToLower(column) + `"`
	for _, statement := range r.statements {
		if !r.statementSelectsFrom("businesses", statement) {
			continue
		}
		normalized := strings.ToLower(statement)
		if strings.Contains(normalized, needleBacktick) ||
			strings.Contains(normalized, needleDouble) {
			return true
		}
	}
	return false
}

func (r *reservationReadSQLRecorder) selectsFromTable(table string) bool {
	for _, statement := range r.statements {
		if r.statementSelectsFrom(table, statement) {
			return true
		}
	}
	return false
}

func (r *reservationReadSQLRecorder) reset() {
	r.statements = nil
}

func setupReservationReadProjectionDB(t testing.TB, rec *reservationReadSQLRecorder) *gorm.DB {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: rec})
	require.NoError(t, err, "open in-memory database")

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	db = gormDB
	require.NoError(t, db.AutoMigrate(
		&Business{},
		&Table{},
		&TableReservation{},
		&ReservationStatusHistory{},
	))
	return gormDB
}

// seedReservationWithFatBusiness creates a Business whose heavy/sensitive
// columns (Stripe IDs, onboarding blob) are populated alongside the few fields
// reservation readers actually consume, plus a reservation with status history.
func seedReservationWithFatBusiness(t testing.TB, confCode string) (*Business, *TableReservation) {
	t.Helper()

	business := &Business{
		BusinessId:      fmt.Sprintf("resv-proj-%d", time.Now().UnixNano()),
		Name:            "Projected Bistro",
		OwnerAddress:    fmt.Sprintf("0xresvproj%x", time.Now().UnixNano()),
		CustomURL:       "projected-bistro",
		Timezone:        "America/New_York",
		DefaultCurrency: "USD",
		DisplayCurrency: "EUR",
		OnboardingState: JSONRawMessage(`{"secret":"do-not-leak"}`),
		Description:     strings.Repeat("long marketing description ", 64),
		WelcomeMessage:  strings.Repeat("welcome blurb ", 64),
		AboutStory:      strings.Repeat("about story ", 64),
	}
	require.NoError(t, db.Create(business).Error)

	table := &Table{
		BusinessID: business.ID,
		TableCode:  "PRJ-T1",
		Name:       "Patio",
		Capacity:   4,
		QRCode:     strings.Repeat("qr-payload", 128),
		IsActive:   true,
	}
	require.NoError(t, db.Create(table).Error)

	tableID := table.ID
	reservation := &TableReservation{
		BusinessID:       business.ID,
		TableID:          &tableID,
		CustomerName:     "Projection Guest",
		CustomerEmail:    "guest@example.com",
		PartySize:        2,
		ReservationTime:  time.Now().UTC().Add(48 * time.Hour),
		Duration:         90,
		Status:           "confirmed",
		Source:           "customer",
		ConfirmationCode: confCode,
	}
	require.NoError(t, db.Create(reservation).Error)

	require.NoError(t, db.Create(&ReservationStatusHistory{
		ReservationID: reservation.ID,
		Status:        "confirmed",
		ChangedBy:     "owner",
	}).Error)

	return business, reservation
}

func assertBusinessProjectionShape(t *testing.T, rec *reservationReadSQLRecorder, biz *Business, loaded Business) {
	t.Helper()

	// No SELECT * on businesses — the embedded Business must be projected.
	assert.Zero(t, rec.selectStarCount("businesses"),
		"reservation read should project Business columns instead of SELECT *")

	// Consumed columns must be present in the projected SELECT.
	for _, col := range []string{"id", "name", "custom_url", "default_currency", "display_currency", "timezone"} {
		assert.True(t, rec.businessSelectMentions(col),
			"projected Business SELECT must include %q (a reservation consumer reads it)", col)
	}

	// Sensitive/heavy columns must NOT be selected.
	for _, col := range []string{"stripe_customer_id", "stripe_subscription_id", "stripe_payment_method", "onboarding_state", "description", "welcome_message", "about_story"} {
		assert.False(t, rec.businessSelectMentions(col),
			"projected Business SELECT must NOT include %q (over-fetch / leak)", col)
	}

	// The hydrated struct must carry the projected fields...
	assert.Equal(t, biz.Name, loaded.Name)
	assert.Equal(t, biz.CustomURL, loaded.CustomURL)
	assert.Equal(t, biz.Timezone, loaded.Timezone)
	assert.Equal(t, biz.DefaultCurrency, loaded.DefaultCurrency)
	assert.Equal(t, biz.DisplayCurrency, loaded.DisplayCurrency)

	// ...and leave the un-projected sensitive fields zeroed.
	assert.Empty(t, string(loaded.OnboardingState), "onboarding blob must not be hydrated")
	assert.Empty(t, loaded.Description, "marketing description must not be hydrated")
}

func TestGetReservationByConfirmationCodeProjectsBusiness(t *testing.T) {
	rec := &reservationReadSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupReservationReadProjectionDB(t, rec)

	biz, _ := seedReservationWithFatBusiness(t, "PRJ-CONF-1")

	rec.reset()
	loaded, err := GetReservationByConfirmationCode("PRJ-CONF-1")
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assertBusinessProjectionShape(t, rec, biz, loaded.Business)

	// Guest confirmation-code path does not render the status timeline; the lean
	// reader must not hydrate StatusHistory at all.
	assert.False(t, rec.selectsFromTable("reservation_status_histories"),
		"guest confirmation-code read must not load StatusHistory")
	assert.Empty(t, loaded.StatusHistory, "guest read must not hydrate StatusHistory")
}

func TestGetReservationByIDProjectsBusinessAndKeepsStatusHistory(t *testing.T) {
	rec := &reservationReadSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupReservationReadProjectionDB(t, rec)

	biz, reservation := seedReservationWithFatBusiness(t, "PRJ-CONF-2")

	rec.reset()
	loaded, err := GetReservationByBusinessAndID(biz.ID, reservation.ID)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assertBusinessProjectionShape(t, rec, biz, loaded.Business)

	// Operator detail renders the status timeline — it must stay loaded.
	assert.True(t, rec.selectsFromTable("reservation_status_histories"),
		"operator read must load StatusHistory for the timeline")
	assert.NotEmpty(t, loaded.StatusHistory, "operator read must hydrate StatusHistory")
}

func TestGetReservationByBusinessAndIDProjectsBusiness(t *testing.T) {
	rec := &reservationReadSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupReservationReadProjectionDB(t, rec)

	biz, reservation := seedReservationWithFatBusiness(t, "PRJ-CONF-3")

	rec.reset()
	loaded, err := GetReservationByBusinessAndID(biz.ID, reservation.ID)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assertBusinessProjectionShape(t, rec, biz, loaded.Business)
	assert.True(t, rec.selectsFromTable("reservation_status_histories"),
		"operator scoped read must load StatusHistory for the timeline")
}

// TestUpdateReservationAfterProjectedReadKeepsBusinessIntact guards the
// cancel-by-code / transition path: it loads the reservation via the projected
// guest reader and then saves it through UpdateReservationStatusGuardedTx
// (the live transition write). GORM's Save must not write the partially-hydrated
// Business association back over the real row (which would blank Stripe IDs /
// onboarding state).
func TestUpdateReservationAfterProjectedReadKeepsBusinessIntact(t *testing.T) {
	rec := &reservationReadSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupReservationReadProjectionDB(t, rec)

	biz, _ := seedReservationWithFatBusiness(t, "PRJ-SAVE-1")

	loaded, err := GetReservationByConfirmationCode("PRJ-SAVE-1")
	require.NoError(t, err)
	require.NotNil(t, loaded)
	// Sanity: the projected read left the sensitive fields zeroed.

	// Mutate + persist exactly as the transition path does.
	prior := loaded.Status
	loaded.Status = "cancelled"
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return UpdateReservationStatusGuardedTx(tx, loaded, prior, 0, ReservationWriteGuards{})
	}))

	// Re-read the Business directly; the un-projected columns must be intact.
	var after Business
	require.NoError(t, db.First(&after, biz.ID).Error)
	assert.Equal(t, `{"secret":"do-not-leak"}`, string(after.OnboardingState),
		"Save of a projected-Business reservation must not blank the onboarding blob")
	assert.NotEmpty(t, after.Description,
		"Save of a projected-Business reservation must not blank the description")
	// And the mutation itself landed.
	var reloaded TableReservation
	require.NoError(t, db.First(&reloaded, loaded.ID).Error)
	assert.Equal(t, "cancelled", reloaded.Status)
}

// BenchmarkGetReservationByConfirmationCode measures the guest confirmation-code
// read over a fully-related reservation (fat Business + table + history).
func BenchmarkGetReservationByConfirmationCode(b *testing.B) {
	rec := &reservationReadSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupReservationReadProjectionDB(b, rec)
	seedReservationWithFatBusiness(b, "PRJ-BENCH-CONF")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := GetReservationByConfirmationCode("PRJ-BENCH-CONF"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGetReservationByBusinessAndID measures the operator detail read
// (projected Business, retained status timeline) over a fully-related reservation.
func BenchmarkGetReservationByBusinessAndID(b *testing.B) {
	rec := &reservationReadSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupReservationReadProjectionDB(b, rec)
	biz, reservation := seedReservationWithFatBusiness(b, "PRJ-BENCH-ID")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := GetReservationByBusinessAndID(biz.ID, reservation.ID); err != nil {
			b.Fatal(err)
		}
	}
}
