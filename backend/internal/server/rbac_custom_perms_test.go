package server

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupCustomPermsTestDB spins up an in-memory SQLite DB with the Staff table
// migrated and wires it as the package-level DB so RBACMiddleware{db:
// database.NewDB()} resolves to it via GetGorm(). It returns the raw *gorm.DB so
// tests can register query/SQL-capture callbacks for access-shape assertions.
func setupCustomPermsTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(&database.Staff{}))

	return gormDB
}

// newCustomPermsRBACMiddleware builds an RBACMiddleware bound to the current
// package-level test DB. Must be called AFTER setupCustomPermsTestDB so NewDB()
// captures the swapped connection.
func newCustomPermsRBACMiddleware() *RBACMiddleware {
	return &RBACMiddleware{db: database.NewDB()}
}

func newTestGinContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c
}

// TestGetCustomPermissions_UsesContextNoQuery proves the hot path: when
// hydrateLiveStaffContext has stashed the raw custom_permissions JSON in the gin
// context, getCustomPermissions parses it WITHOUT issuing any DB query.
func TestGetCustomPermissions_UsesContextNoQuery(t *testing.T) {
	gormDB := setupCustomPermsTestDB(t)
	r := newCustomPermsRBACMiddleware()

	queryCount := 0
	require.NoError(t, gormDB.Callback().Query().Before("gorm:query").Register("count_custom_perms_ctx_hit", func(_ *gorm.DB) {
		queryCount++
	}))

	c := newTestGinContext(t)
	c.Set("staff_id", uint(1))
	c.Set("staff_custom_permissions", `["orders:write"]`)

	perms := r.getCustomPermissions(c, uint(1))

	assert.Equal(t, []string{"orders:write"}, perms)
	assert.Equal(t, 0, queryCount, "context-hit path must NOT issue any DB query")
}

// TestGetCustomPermissions_EmptyContextValueNoQuery proves that a present-but-empty
// context value (staff with no custom perms) returns nil and still issues ZERO
// queries — the empty string is an authoritative "no custom perms", not a miss.
func TestGetCustomPermissions_EmptyContextValueNoQuery(t *testing.T) {
	gormDB := setupCustomPermsTestDB(t)
	r := newCustomPermsRBACMiddleware()

	queryCount := 0
	require.NoError(t, gormDB.Callback().Query().Before("gorm:query").Register("count_custom_perms_empty_ctx", func(_ *gorm.DB) {
		queryCount++
	}))

	c := newTestGinContext(t)
	c.Set("staff_id", uint(1))
	c.Set("staff_custom_permissions", "")

	perms := r.getCustomPermissions(c, uint(1))

	assert.Nil(t, perms, "empty context value means no custom perms")
	assert.Equal(t, 0, queryCount, "empty context value must NOT trigger a fallback query")
}

// TestGetCustomPermissions_ContextMissFallsBackToNarrowQuery proves the fail-safe:
// when the context key is absent (a caller that did not run
// hydrateLiveStaffContext), getCustomPermissions falls back to a SINGLE query
// that selects ONLY custom_permissions (+ pk) — never the full staff row.
func TestGetCustomPermissions_ContextMissFallsBackToNarrowQuery(t *testing.T) {
	gormDB := setupCustomPermsTestDB(t)
	r := newCustomPermsRBACMiddleware()

	staff := database.Staff{
		BusinessID:        7,
		Email:             "narrow@example.test",
		Name:              "Narrow Query Staff",
		Role:              database.StaffRoleServer,
		IsActive:          true,
		InvitedBy:         "0xowner",
		CustomPermissions: `["bills:write","menu:read"]`,
	}
	require.NoError(t, gormDB.Create(&staff).Error)

	queryCount := 0
	var capturedSQL string
	require.NoError(t, gormDB.Callback().Query().Before("gorm:query").Register("count_custom_perms_miss", func(_ *gorm.DB) {
		queryCount++
	}))
	require.NoError(t, gormDB.Callback().Query().After("gorm:query").Register("capture_custom_perms_sql", func(db *gorm.DB) {
		capturedSQL = db.Statement.SQL.String()
	}))

	c := newTestGinContext(t)
	c.Set("staff_id", staff.ID)
	// staff_custom_permissions intentionally NOT set — simulates a context miss.

	perms := r.getCustomPermissions(c, staff.ID)

	require.Equal(t, []string{"bills:write", "menu:read"}, perms, "fallback must return the staff's stored custom perms")
	assert.Equal(t, 1, queryCount, "context miss must issue exactly ONE fallback query")

	lower := strings.ToLower(capturedSQL)
	assert.Contains(t, lower, "custom_permissions", "fallback query must select custom_permissions")
	// Narrow-projection proof: the full-row columns must NOT appear in the SELECT.
	assert.NotContains(t, lower, "email", "fallback query must NOT select the full staff row (email leaked)")
	assert.NotContains(t, lower, "invited_by", "fallback query must NOT select the full staff row (invited_by leaked)")
	assert.NotContains(t, lower, "pin_hash", "fallback query must NOT select the full staff row (pin_hash leaked)")
}

// BenchmarkGetStaffPermissions_Before measures the legacy full-row fallback path
// (no context value present) — every call hits the DB. Recorded as the M2
// baseline.
func BenchmarkGetStaffPermissions_Before(b *testing.B) {
	gormDB := benchSetupCustomPermsDB(b)
	r := &RBACMiddleware{db: database.NewDB()}

	staff := database.Staff{
		BusinessID:        7,
		Email:             "bench-before@example.test",
		Name:              "Bench Before",
		Role:              database.StaffRoleServer,
		IsActive:          true,
		InvitedBy:         "0xowner",
		CustomPermissions: `["bills:write","menu:read"]`,
	}
	require.NoError(b, gormDB.Create(&staff).Error)

	gin.SetMode(gin.ReleaseMode)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("staff_id", staff.ID)
		// No staff_custom_permissions — forces the fallback query every call.
		if perms := r.getCustomPermissions(c, staff.ID); len(perms) != 2 {
			b.Fatalf("expected 2 perms, got %d", len(perms))
		}
	}
}

// BenchmarkGetStaffPermissions_After measures the optimized hot path: the raw
// custom_permissions JSON is hydrated in context (as hydrateLiveStaffContext
// does), so resolution is a pure in-memory parse with ZERO queries.
func BenchmarkGetStaffPermissions_After(b *testing.B) {
	gormDB := benchSetupCustomPermsDB(b)
	r := &RBACMiddleware{db: database.NewDB()}

	queryCount := 0
	require.NoError(b, gormDB.Callback().Query().Before("gorm:query").Register("count_custom_perms_after_bench", func(_ *gorm.DB) {
		queryCount++
	}))

	gin.SetMode(gin.ReleaseMode)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("staff_id", uint(1))
		c.Set("staff_custom_permissions", `["bills:write","menu:read"]`)
		if perms := r.getCustomPermissions(c, uint(1)); len(perms) != 2 {
			b.Fatalf("expected 2 perms, got %d", len(perms))
		}
	}
	b.StopTimer()
	if queryCount != 0 {
		b.Fatalf("After path must issue zero queries, got %d", queryCount)
	}
}

func benchSetupCustomPermsDB(b *testing.B) *gorm.DB {
	b.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", b.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(b, err)
	sqlDB, err := gormDB.DB()
	require.NoError(b, err)
	sqlDB.SetMaxOpenConns(1)
	b.Cleanup(func() { require.NoError(b, sqlDB.Close()) })
	database.SetTestDB(gormDB)
	require.NoError(b, gormDB.AutoMigrate(&database.Staff{}))
	return gormDB
}
