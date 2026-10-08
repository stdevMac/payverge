package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/auththrottle"
	"github.com/stdevmac/payverge/backend/internal/database"
)

// setupPinThrottleTestDB returns an in-memory SQLite DB migrated with the
// tables needed for manager-PIN throttle tests (Staff, CompVoidAudit,
// AuthAttempt). Each call gets a unique DSN to avoid shared-cache bleed
// across parallel tests.
func setupPinThrottleTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:pin_throttle_%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Staff{},
		&database.CompVoidAudit{},
		&database.AuthAttempt{},
	))
	return gormDB
}

// seedStaffWithPin creates an active Staff row in the current test DB and sets
// its manager PIN to `pin`. Returns the created Staff.
func seedStaffWithPin(t *testing.T, _ *gorm.DB, pin string) *database.Staff {
	t.Helper()
	staff := &database.Staff{
		BusinessID: 1,
		Email:      fmt.Sprintf("staff+%s@example.com", t.Name()),
		Name:       "Test Staff",
		Role:       database.StaffRoleManager,
		InvitedBy:  "owner@example.com",
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(staff).Error)
	require.NoError(t, database.SetStaffPin(staff.ID, pin))
	return staff
}

// stubStaffContext returns a Gin handler that stamps the context with
// token_type="staff" and staff_id=id, mimicking what the auth middleware does.
// This is the same pattern used in staff_handlers_test.go.
func stubStaffContext(id uint) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_id", id)
		c.Next()
	}
}

// TestRequireManagerPIN_LockedStaffRejectedBeforeVerify asserts that a staff
// identity that is locked out is rejected with 429/RATE_LIMITED even when
// the supplied PIN header is correct. This proves the lockout gate runs before
// bcrypt verification.
func TestRequireManagerPIN_LockedStaffRejectedBeforeVerify(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPinThrottleTestDB(t)
	th := auththrottle.New(db, auththrottle.Config{MaxAttempts: 3})
	SetManagerPinThrottle(th)
	t.Cleanup(func() { SetManagerPinThrottle(nil) })

	staff := seedStaffWithPin(t, db, "123456")
	principal := managerPinPrincipal(staff.ID)
	for i := 0; i < 3; i++ {
		require.NoError(t, th.Record(principal, managerPinThrottleKind))
	}

	r := gin.New()
	r.POST("/bills/:bill_id/void", stubStaffContext(staff.ID), RequireManagerPIN("bill", "void"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/bills/1/void", nil)
	req.Header.Set("X-Manager-Pin", "123456") // correct PIN — must still be rejected
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Contains(t, w.Body.String(), "RATE_LIMITED")
}

// TestRequireManagerPIN_WrongPinAccumulatesLockout asserts that MaxAttempts
// wrong-PIN requests through the real middleware flip IsLocked to true, proving
// that Record is called on the actual wrong-PIN path.
func TestRequireManagerPIN_WrongPinAccumulatesLockout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPinThrottleTestDB(t)
	th := auththrottle.New(db, auththrottle.Config{MaxAttempts: 3})
	SetManagerPinThrottle(th)
	t.Cleanup(func() { SetManagerPinThrottle(nil) })

	staff := seedStaffWithPin(t, db, "123456")
	r := gin.New()
	r.POST("/bills/:bill_id/void", stubStaffContext(staff.ID), RequireManagerPIN("bill", "void"), func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/bills/1/void", nil)
		req.Header.Set("X-Manager-Pin", "000000") // wrong
		r.ServeHTTP(httptest.NewRecorder(), req)
	}
	locked, _, err := th.IsLocked(managerPinPrincipal(staff.ID), managerPinThrottleKind)
	require.NoError(t, err)
	assert.True(t, locked)
}

// TestRequireManagerPIN_FailsClosedWhenThrottleBackendErrors locks in the
// fail-closed fix: when the PIN lockout backend errors (auth_attempts missing),
// the middleware must DENY with 429 rather than wave the privileged action
// through unthrottled. The request supplies the CORRECT PIN, so a fail-open
// regression would return 200.
func TestRequireManagerPIN_FailsClosedWhenThrottleBackendErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupPinThrottleTestDB(t)
	staff := seedStaffWithPin(t, db, "123456")

	// Throttle backed by a DB WITHOUT the auth_attempts table -> IsLocked errors.
	brokenDB, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:pin_broken_%s?mode=memory&cache=shared", t.Name())),
		&gorm.Config{})
	require.NoError(t, err)
	brokenSQL, err := brokenDB.DB()
	require.NoError(t, err)
	brokenSQL.SetMaxOpenConns(1)
	// Intentionally NOT migrating AuthAttempt so IsLocked's SELECT errors.
	SetManagerPinThrottle(auththrottle.New(brokenDB, auththrottle.Config{MaxAttempts: 3}))
	t.Cleanup(func() { SetManagerPinThrottle(nil) })

	r := gin.New()
	r.POST("/bills/:bill_id/void", stubStaffContext(staff.ID), RequireManagerPIN("bill", "void"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/bills/1/void", nil)
	req.Header.Set("X-Manager-Pin", "123456") // correct PIN — must STILL be denied
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code,
		"PIN throttle backend error must fail CLOSED (429) even with the correct PIN; body: %s", w.Body.String())
	assert.Contains(t, w.Body.String(), "RATE_LIMITED")
}
