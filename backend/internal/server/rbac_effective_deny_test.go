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

// setupEffectiveDenyTestDB migrates Staff + StaffPermissionDeny for effective-permission
// and access-shape tests. Uses a unique in-memory DSN per test.
func setupEffectiveDenyTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:eff-deny-%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(&database.Staff{}, &database.StaffPermissionDeny{}))
	return gormDB
}

func newEffectiveDenyRBAC() *RBACMiddleware {
	return &RBACMiddleware{db: database.NewDB()}
}

// staffCtx builds a gin context that mimics hydrateLiveStaffContext for staff RBAC checks.
func staffCtx(t *testing.T, role database.StaffRole, staffID uint, customJSON string, denies []string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("token_type", "staff")
	c.Set("staff_role", string(role))
	c.Set("staff_id", staffID)
	// Present keys = authoritative (even when empty), matching hydrateLiveStaffContext.
	c.Set("staff_custom_permissions", customJSON)
	if denies == nil {
		denies = []string{}
	}
	c.Set("staff_permission_denies", denies)
	return c
}

// --- Task 10: four core effective-permission cases ---

// 1) Role grant + explicit deny of the same permission => DENIED.
func TestEffectivePermissions_RoleGrantPlusExplicitDeny_Denied(t *testing.T) {
	_ = setupEffectiveDenyTestDB(t)
	r := newEffectiveDenyRBAC()

	// Manager role includes bills:read by default.
	c := staffCtx(t, database.StaffRoleManager, 1, "", []string{"bills:read"})

	allowed := r.checkStaffPermissions(c, []string{"bills:read"})
	assert.False(t, allowed, "role grant + explicit deny of the same permission must DENY")
}

// 2) Explicit user grant of a permission the role lacks, no deny => ALLOWED.
func TestEffectivePermissions_CustomGrantNoDeny_Allowed(t *testing.T) {
	_ = setupEffectiveDenyTestDB(t)
	r := newEffectiveDenyRBAC()

	// Kitchen has no analytics:sales; grant it via custom permissions.
	c := staffCtx(t, database.StaffRoleKitchen, 2, `["analytics:sales"]`, nil)

	allowed := r.checkStaffPermissions(c, []string{"analytics:sales"})
	assert.True(t, allowed, "explicit user grant without deny must ALLOW")
}

// 3) An "owner-only by default" money permission (bills:refund) is NOT a role
// default — a plain manager is denied — but IS delegable: an owner may grant it
// to a specific manager, making it effective. An explicit deny still overrides.
// (Payverge's documented separation-of-duties model keeps a delegation escape
// hatch; nothing is non-delegable today — see server.OwnerOnlyPermissions.)
func TestEffectivePermissions_OwnerOnlyByDefault_DelegableByGrant(t *testing.T) {
	_ = setupEffectiveDenyTestDB(t)
	r := newEffectiveDenyRBAC()

	// Plain manager, no grant: bills:refund is not a role default → denied.
	plain := staffCtx(t, database.StaffRoleManager, 3, `[]`, nil)
	assert.False(t, r.checkStaffPermissions(plain, []string{"bills:refund"}),
		"bills:refund is owner-only by default; a plain manager must be denied")

	// Owner delegates bills:refund to this manager via custom grant → effective.
	granted := staffCtx(t, database.StaffRoleManager, 3, `["bills:refund"]`, nil)
	assert.True(t, r.checkStaffPermissions(granted, []string{"bills:refund"}),
		"an owner may delegate bills:refund to a specific manager via custom grant")

	// An explicit deny overrides even a delegated grant.
	deniedToo := staffCtx(t, database.StaffRoleManager, 3, `["bills:refund"]`, []string{"bills:refund"})
	assert.False(t, r.checkStaffPermissions(deniedToo, []string{"bills:refund"}),
		"an explicit deny overrides even a delegated grant")
}

// 4) An unrelated permission is unaffected by a deny on a different permission.
func TestEffectivePermissions_UnrelatedPermissionUnaffectedByDeny(t *testing.T) {
	_ = setupEffectiveDenyTestDB(t)
	r := newEffectiveDenyRBAC()

	// Deny menu:write; bills:read (role grant for manager) must still be allowed.
	c := staffCtx(t, database.StaffRoleManager, 4, "", []string{"menu:write"})

	assert.False(t, r.checkStaffPermissions(c, []string{"menu:write"}),
		"denied permission itself must be denied")
	assert.True(t, r.checkStaffPermissions(c, []string{"bills:read"}),
		"unrelated permission must remain allowed when a different permission is denied")
}

// Wildcard deny: deny menu:delete while role has concrete menu:*-style coverage via
// hasPermission wildcards / full menu grants. Manager has menu:read and menu:write;
// we simulate role-level menu:* via a custom grant of menu:* and deny menu:delete.
func TestEffectivePermissions_WildcardDeny_SpecificDeniedOthersAllowed(t *testing.T) {
	_ = setupEffectiveDenyTestDB(t)
	r := newEffectiveDenyRBAC()

	// Server role has menu:read only (not menu:delete). Grant menu:* custom, deny menu:delete.
	c := staffCtx(t, database.StaffRoleServer, 5, `["menu:*"]`, []string{"menu:delete"})

	assert.False(t, r.checkStaffPermissions(c, []string{"menu:delete"}),
		"deny of menu:delete must block even when menu:* is granted")
	assert.True(t, r.checkStaffPermissions(c, []string{"menu:read"}),
		"menu:read must remain allowed when only menu:delete is denied")
}

// Access shape: with denies hydrated in context, permission checks issue ZERO DB queries
// (same hot-path contract as custom grants). Context miss falls back to ONE staff_id query.
func TestGetPermissionDenies_ContextHitZeroQueries(t *testing.T) {
	gormDB := setupEffectiveDenyTestDB(t)
	r := newEffectiveDenyRBAC()

	queryCount := 0
	require.NoError(t, gormDB.Callback().Query().Before("gorm:query").Register("count_deny_ctx_hit", func(_ *gorm.DB) {
		queryCount++
	}))

	c := staffCtx(t, database.StaffRoleServer, 1, "", []string{"orders:write"})
	denies := r.getPermissionDenies(c, uint(1))

	assert.Equal(t, []string{"orders:write"}, denies)
	assert.Equal(t, 0, queryCount, "context-hit deny path must NOT issue any DB query")
}

func TestGetPermissionDenies_EmptyContextValueNoQuery(t *testing.T) {
	gormDB := setupEffectiveDenyTestDB(t)
	r := newEffectiveDenyRBAC()

	queryCount := 0
	require.NoError(t, gormDB.Callback().Query().Before("gorm:query").Register("count_deny_empty_ctx", func(_ *gorm.DB) {
		queryCount++
	}))

	c := staffCtx(t, database.StaffRoleServer, 1, "", []string{})
	denies := r.getPermissionDenies(c, uint(1))

	assert.Empty(t, denies)
	assert.Equal(t, 0, queryCount, "empty deny slice in context must NOT trigger a fallback query")
}

func TestGetPermissionDenies_ContextMissFallsBackToSingleStaffIDQuery(t *testing.T) {
	gormDB := setupEffectiveDenyTestDB(t)
	r := newEffectiveDenyRBAC()

	staff := database.Staff{
		BusinessID: 7,
		Email:      "deny-narrow@example.test",
		Name:       "Deny Narrow",
		Role:       database.StaffRoleServer,
		IsActive:   true,
		InvitedBy:  "0xowner",
	}
	require.NoError(t, gormDB.Create(&staff).Error)
	require.NoError(t, gormDB.Create(&database.StaffPermissionDeny{
		BusinessID: staff.BusinessID,
		StaffID:    staff.ID,
		Permission: "bills:close",
		CreatedBy:  "0xowner",
	}).Error)
	require.NoError(t, gormDB.Create(&database.StaffPermissionDeny{
		BusinessID: staff.BusinessID,
		StaffID:    staff.ID,
		Permission: "menu:write",
		CreatedBy:  "0xowner",
	}).Error)

	queryCount := 0
	var capturedSQL string
	require.NoError(t, gormDB.Callback().Query().Before("gorm:query").Register("count_deny_miss", func(_ *gorm.DB) {
		queryCount++
	}))
	require.NoError(t, gormDB.Callback().Query().After("gorm:query").Register("capture_deny_sql", func(db *gorm.DB) {
		capturedSQL = db.Statement.SQL.String()
	}))

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("staff_id", staff.ID)
	// staff_permission_denies intentionally NOT set — context miss.

	denies := r.getPermissionDenies(c, staff.ID)

	require.ElementsMatch(t, []string{"bills:close", "menu:write"}, denies)
	assert.Equal(t, 1, queryCount, "context miss must issue exactly ONE fallback query")

	lower := strings.ToLower(capturedSQL)
	assert.Contains(t, lower, "staff_permission_denies", "fallback must query staff_permission_denies")
	assert.Contains(t, lower, "staff_id", "fallback must be keyed by staff_id")
}

func TestGetPermissionDenies_FallbackStoreFailureDeniesAll(t *testing.T) {
	gormDB := setupEffectiveDenyTestDB(t)
	r := newEffectiveDenyRBAC()
	require.NoError(t, gormDB.Migrator().DropTable(&database.StaffPermissionDeny{}))

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("token_type", "staff")
	c.Set("staff_role", string(database.StaffRoleManager))
	c.Set("staff_id", uint(91))
	c.Set("staff_custom_permissions", `[]`)
	// Deliberately omit staff_permission_denies to exercise the fallback query.

	assert.False(t, r.checkStaffPermissions(c, []string{"bills:read"}),
		"a deny-store error must not restore a role grant")
	assert.Equal(t, []string{"*:*"}, r.getPermissionDenies(c, uint(91)))
}

// checkStaffPermissions on the hot path (grants + denies in context) must not
// issue per-permission-check queries — zero queries for N required permissions.
func TestCheckStaffPermissions_HotPathNoNPlusOne(t *testing.T) {
	gormDB := setupEffectiveDenyTestDB(t)
	r := newEffectiveDenyRBAC()

	queryCount := 0
	require.NoError(t, gormDB.Callback().Query().Before("gorm:query").Register("count_check_hot", func(_ *gorm.DB) {
		queryCount++
	}))

	c := staffCtx(t, database.StaffRoleManager, 9, `["crm:export"]`, []string{"menu:write"})

	// Several required perms — must not scale queries with required-permission count.
	require.True(t, r.checkStaffPermissions(c, []string{"bills:read", "orders:read", "crm:export"}))
	require.False(t, r.checkStaffPermissions(c, []string{"menu:write"}))
	require.False(t, r.checkStaffPermissions(c, []string{"bills:refund"})) // not granted (owner-only by default)

	assert.Equal(t, 0, queryCount, "hot-path checkStaffPermissions must issue zero DB queries")
}

func BenchmarkCheckStaffPermissions_HotPathWithDenies(b *testing.B) {
	dsn := fmt.Sprintf("file:bench-deny-%s?mode=memory&cache=shared", b.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(b, err)
	sqlDB, err := gormDB.DB()
	require.NoError(b, err)
	sqlDB.SetMaxOpenConns(1)
	b.Cleanup(func() { require.NoError(b, sqlDB.Close()) })
	database.SetTestDB(gormDB)
	require.NoError(b, gormDB.AutoMigrate(&database.Staff{}, &database.StaffPermissionDeny{}))

	r := &RBACMiddleware{db: database.NewDB()}
	queryCount := 0
	require.NoError(b, gormDB.Callback().Query().Before("gorm:query").Register("bench_deny_q", func(_ *gorm.DB) {
		queryCount++
	}))

	gin.SetMode(gin.ReleaseMode)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleManager))
		c.Set("staff_id", uint(1))
		c.Set("staff_custom_permissions", `["crm:export"]`)
		c.Set("staff_permission_denies", []string{"menu:write", "plugins:config"})
		if !r.checkStaffPermissions(c, []string{"bills:read", "crm:export"}) {
			b.Fatal("expected allow")
		}
		if r.checkStaffPermissions(c, []string{"menu:write"}) {
			b.Fatal("expected deny")
		}
	}
	b.StopTimer()
	if queryCount != 0 {
		b.Fatalf("hot path must issue zero queries, got %d", queryCount)
	}
}
