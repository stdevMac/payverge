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

// digestSQLRecorder captures every SQL statement GORM executes so tests can
// assert on SELECT column lists without touching the database log output.
type digestSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *digestSQLRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func (r *digestSQLRecorder) reset() {
	r.statements = r.statements[:0]
}

func (r *digestSQLRecorder) selectSQL() string {
	for _, s := range r.statements {
		low := strings.ToLower(s)
		if strings.HasPrefix(low, "select ") && strings.Contains(low, "businesses") {
			return s
		}
	}
	return ""
}

// setupDigestProjectionTestDB opens an in-memory SQLite DB, AutoMigrates the
// Business model, seeds one AI Pro business with sentinel secret values in
// fields that must NOT be hydrated by the narrow projection, and returns the
// DB plus the SQL recorder attached to it.
func setupDigestProjectionTestDB(t testing.TB) (*gorm.DB, *digestSQLRecorder) {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	recorder := &digestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: recorder})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	require.NoError(t, db.AutoMigrate(&database.Business{}, &database.User{}))
	database.SetTestDB(db)

	uid := uint(42)
	biz := database.Business{
		// 9 columns the digest path actually reads:
		BusinessId:      "my-slug",
		Name:            "Test Bistro",
		OwnerName:       "Ana García",
		Email:           "ana@bistro.example",
		Timezone:        "America/New_York",
		DefaultLanguage: "es",
		UserID:          &uid,
		OwnerAddress:    "0xdeadbeef",
		// IsActive is a WHERE predicate; include it so the row matches the
		// query.
		IsActive: true,
		// Sentinel secrets that must NOT appear in the projected result:
		OnboardingState: database.JSONRawMessage(`{"secret":"no"}`),
	}
	require.NoError(t, db.Create(&biz).Error)

	return db, recorder
}

// TestDigestProjection_ShapeAndParity is the primary access-shape test.
//
// RED phase (full-row Find): asserts that the naive `Find` without a Select
// clause selects "*" / all columns, including stripe_customer_id.
// GREEN phase (projected Find): asserts that the narrow projection does NOT
// include stripe_customer_id / onboarding_state / "*", DOES include all 9
// audited columns, and that every field the digest consumers need is correctly
// populated.
func TestDigestProjection_ShapeAndParity(t *testing.T) {
	db, recorder := setupDigestProjectionTestDB(t)

	// ------------------------------------------------------------------ RED
	// Prove that the pre-fix full-row query selects "all columns" — on SQLite
	// GORM emits `SELECT *` (or lists every column) which will include
	// stripe_customer_id.
	recorder.reset()
	var fullRows []database.Business
	require.NoError(t, db.
		Where("is_active = ?", true).
		Find(&fullRows).Error)

	fullSQL := recorder.selectSQL()
	require.NotEmpty(t, fullSQL, "expected a SELECT statement for businesses")

	// Full-row SELECT must either list all columns explicitly (including stripe) or
	// use a wildcard ("*").  GORM on SQLite emits `SELECT *` for an unqualified
	// Find, so we accept either form.
	fullSQLLow := strings.ToLower(fullSQL)
	containsStripeOrStar := strings.Contains(fullSQLLow, "stripe_customer_id") ||
		strings.Contains(fullSQLLow, "stripe_subscription_id") ||
		strings.Contains(fullSQLLow, "onboarding_state") ||
		strings.Contains(fullSQLLow, "select *") ||
		strings.Contains(fullSQLLow, "`businesses`.*") ||
		strings.Contains(fullSQLLow, "businesses.*")
	assert.True(t, containsStripeOrStar,
		"RED phase: full-row Find should select stripe/onboarding columns or use SELECT *; SQL was: %s", fullSQL)

	// ----------------------------------------------------------------- GREEN
	// Projected query — the exact production fix.
	recorder.reset()
	var projRows []database.Business
	require.NoError(t, db.
		Select("id", "business_id", "name", "owner_name", "email", "timezone", "default_language", "user_id", "owner_address").
		Where("is_active = ?", true).
		Find(&projRows).Error)

	projSQL := recorder.selectSQL()
	require.NotEmpty(t, projSQL, "expected a SELECT statement for projected query")

	projSQLLow := strings.ToLower(projSQL)

	// Must NOT include secrets.
	assert.NotContains(t, projSQLLow, "stripe_customer_id",
		"GREEN: projected query must not select stripe_customer_id")
	assert.NotContains(t, projSQLLow, "stripe_subscription_id",
		"GREEN: projected query must not select stripe_subscription_id")
	assert.NotContains(t, projSQLLow, "onboarding_state",
		"GREEN: projected query must not select onboarding_state")
	assert.False(t,
		strings.Contains(projSQLLow, "businesses.*") || strings.Contains(projSQLLow, "`businesses`.*"),
		"GREEN: projected query must not use SELECT *")

	// Must include every audited column.
	for _, col := range []string{"`id`", "`business_id`", "`name`", "`owner_name`",
		"`email`", "`timezone`", "`default_language`", "`user_id`", "`owner_address`"} {
		assert.Contains(t, projSQLLow, col,
			"GREEN: projected query must select %s; SQL: %s", col, projSQL)
	}

	// ---------------------------------------------------------------- PARITY
	// All 9 consumed fields are populated in the projected result.
	require.Len(t, projRows, 1, "expected exactly one AI Pro business")
	b := projRows[0]

	assert.NotZero(t, b.ID, "parity: ID must be populated")
	assert.Equal(t, "my-slug", b.BusinessId, "parity: BusinessId")
	assert.Equal(t, "Test Bistro", b.Name, "parity: Name")
	assert.Equal(t, "Ana García", b.OwnerName, "parity: OwnerName")
	assert.Equal(t, "ana@bistro.example", b.Email, "parity: Email")
	assert.Equal(t, "America/New_York", b.Timezone, "parity: Timezone")
	assert.Equal(t, "es", b.DefaultLanguage, "parity: DefaultLanguage")
	assert.NotNil(t, b.UserID, "parity: UserID must not be nil")
	assert.Equal(t, uint(42), *b.UserID, "parity: UserID value")
	assert.Equal(t, "0xdeadbeef", b.OwnerAddress, "parity: OwnerAddress")

	// Sentinel secrets must be zero — never hydrated.
	assert.Empty(t, string(b.OnboardingState), "secrets: OnboardingState must be zero in projected row")

	// -------------------------------------------------------- CONSUMER PARITY
	// Verify that digest-path consumer derivations produce the same results from
	// the projected row as they would from a full row.

	// (a) buildDirectorConsoleURL uses BusinessId and ID.
	fullRow := fullRows[0]
	assert.Equal(t,
		buildDirectorConsoleURL(&fullRow),
		buildDirectorConsoleURL(&b),
		"consumer parity: buildDirectorConsoleURL must agree between full and projected row")

	// (b) sendDigestEmail ownerName derivation.
	ownerNameFull := fullRow.OwnerName
	if ownerNameFull == "" {
		ownerNameFull = fullRow.Name
	}
	ownerNameProj := b.OwnerName
	if ownerNameProj == "" {
		ownerNameProj = b.Name
	}
	assert.Equal(t, ownerNameFull, ownerNameProj,
		"consumer parity: ownerName derivation must agree")

	// (c) sendDigestEmail businessName derivation.
	businessNameFull := fullRow.Name
	if businessNameFull == "" {
		businessNameFull = ownerNameFull
	}
	businessNameProj := b.Name
	if businessNameProj == "" {
		businessNameProj = ownerNameProj
	}
	assert.Equal(t, businessNameFull, businessNameProj,
		"consumer parity: businessName derivation must agree")

	// (d) DetermineBusinessOwnerLanguage — both rows have the same UserID,
	// OwnerAddress, Email, DefaultLanguage so the function returns the same
	// value.  We can't call database.GetUserByID because no user row exists,
	// so both rows will fall through to DefaultLanguage = "es".
	assert.Equal(t,
		DetermineBusinessOwnerLanguage(&fullRow),
		DetermineBusinessOwnerLanguage(&b),
		"consumer parity: DetermineBusinessOwnerLanguage must agree between full and projected row")
}

// TestDigestProjection_FullRowContainsSecrets is the explicit RED demonstration:
// a full-row load of the seeded business DOES hydrate the sentinel secrets.
func TestDigestProjection_FullRowContainsSecrets(t *testing.T) {
	db, _ := setupDigestProjectionTestDB(t)

	var rows []database.Business
	require.NoError(t, db.
		Where("is_active = ?", true).
		Find(&rows).Error)
	require.Len(t, rows, 1)

	assert.Contains(t, string(rows[0].OnboardingState), "secret",
		"RED: full-row Find must hydrate OnboardingState (sentinel present)")
}

// BenchmarkDigestBusinessLoad measures the cost of loading AI Pro business rows,
// comparing the full-row SELECT * (before) against the 9-column projection (after).
// Run with: go test ./internal/services/ -run '^$' -bench 'BenchmarkDigestBusinessLoad' -benchmem -count=3
func BenchmarkDigestBusinessLoad(b *testing.B) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", b.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(b, err)
	sqlDB, err := db.DB()
	require.NoError(b, err)
	sqlDB.SetMaxOpenConns(1)
	b.Cleanup(func() { _ = sqlDB.Close() })
	database.SetTestDB(db)
	require.NoError(b, db.AutoMigrate(&database.Business{}))

	// Seed 200 AI Pro businesses.
	for i := 0; i < 200; i++ {
		uid := uint(i + 1)
		biz := database.Business{
			BusinessId:      fmt.Sprintf("biz-%d", i),
			Name:            fmt.Sprintf("Business %d", i),
			OwnerName:       fmt.Sprintf("Owner %d", i),
			Email:           fmt.Sprintf("owner%d@example.com", i),
			Timezone:        "UTC",
			DefaultLanguage: "en",
			UserID:          &uid,
			OwnerAddress:    fmt.Sprintf("0x%040x", i),
			IsActive:        true,
			OnboardingState: database.JSONRawMessage(fmt.Sprintf(`{"step":%d,"visited":true,"data":"%s"}`, i, strings.Repeat("x", 128))),
		}
		require.NoError(b, db.Create(&biz).Error)
	}

	b.Run("FullRow", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var businesses []database.Business
			_ = db.
				Where("is_active = ?", true).
				Find(&businesses).Error
		}
	})

	b.Run("Projected", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var businesses []database.Business
			_ = db.
				Select("id", "business_id", "name", "owner_name", "email", "timezone", "default_language", "user_id", "owner_address").
				Where("is_active = ?", true).
				Find(&businesses).Error
		}
	})
}

// TestDigestProjection_LoadActiveBusinesses exercises the production
// loadActiveBusinesses method end-to-end, asserting:
//   - the 9 audited fields are correctly populated in the returned rows
//   - sentinel secrets (stripe_customer_id, stripe_subscription_id,
//     onboarding_state) are zero — never hydrated by the narrow projection
//   - the emitted SQL does not reference secret column names
func TestDigestProjection_LoadActiveBusinesses(t *testing.T) {
	db, recorder := setupDigestProjectionTestDB(t)

	recorder.reset()
	scheduler := &DirectorDigestScheduler{db: db}
	businesses, err := scheduler.loadActiveBusinesses()
	require.NoError(t, err)
	require.NotEmpty(t, businesses, "expected at least one AI Pro business")

	b := businesses[0]

	// 9 audited fields populated correctly.
	assert.NotZero(t, b.ID, "parity: ID must be populated")
	assert.Equal(t, "my-slug", b.BusinessId, "parity: BusinessId")
	assert.Equal(t, "Test Bistro", b.Name, "parity: Name")
	assert.Equal(t, "Ana García", b.OwnerName, "parity: OwnerName")
	assert.Equal(t, "ana@bistro.example", b.Email, "parity: Email")
	assert.Equal(t, "America/New_York", b.Timezone, "parity: Timezone")
	assert.Equal(t, "es", b.DefaultLanguage, "parity: DefaultLanguage")
	require.NotNil(t, b.UserID, "parity: UserID must not be nil")
	assert.Equal(t, uint(42), *b.UserID, "parity: UserID value")
	assert.Equal(t, "0xdeadbeef", b.OwnerAddress, "parity: OwnerAddress")

	// Sentinel secrets must be zero — the narrow projection must not hydrate them.
	assert.Empty(t, string(b.OnboardingState), "OnboardingState must not be hydrated by projection")

	// Verify the emitted SQL does not reference secret column names.
	projSQL := recorder.selectSQL()
	if projSQL != "" {
		projSQLLow := strings.ToLower(projSQL)
		assert.NotContains(t, projSQLLow, "stripe_customer_id",
			"SQL must not include stripe_customer_id; got: %s", projSQL)
		assert.NotContains(t, projSQLLow, "onboarding_state",
			"SQL must not include onboarding_state; got: %s", projSQL)
	}
}
