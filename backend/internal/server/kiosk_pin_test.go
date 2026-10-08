package server

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/auththrottle"
	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestVerifyKioskPin_CorrectClearsThrottle: a correct PIN returns nil and clears
// any accumulated failures.
func TestVerifyKioskPin_CorrectClearsThrottle(t *testing.T) {
	db := setupPinThrottleTestDB(t)
	th := auththrottle.New(db, auththrottle.Config{MaxAttempts: 3})
	SetManagerPinThrottle(th)
	t.Cleanup(func() { SetManagerPinThrottle(nil) })

	staff := seedStaffWithPin(t, db, "123456")
	principal := managerPinPrincipal(staff.ID)
	require.NoError(t, th.Record(principal, kioskPinThrottleKind))

	_, err := VerifyKioskPin(staff.ID, "123456")
	require.NoError(t, err)

	locked, _, lerr := th.IsLocked(principal, kioskPinThrottleKind)
	require.NoError(t, lerr)
	assert.False(t, locked, "a correct PIN must clear the kiosk throttle")
}

// TestVerifyKioskPin_WrongPinRecordsAndLocks: MaxAttempts wrong PINs flip the
// kiosk lockout, proving Record runs on the wrong-PIN path.
func TestVerifyKioskPin_WrongPinRecordsAndLocks(t *testing.T) {
	db := setupPinThrottleTestDB(t)
	th := auththrottle.New(db, auththrottle.Config{MaxAttempts: 3})
	SetManagerPinThrottle(th)
	t.Cleanup(func() { SetManagerPinThrottle(nil) })

	staff := seedStaffWithPin(t, db, "123456")
	for i := 0; i < 3; i++ {
		_, err := VerifyKioskPin(staff.ID, "000000")
		require.ErrorIs(t, err, database.ErrPinInvalid)
	}
	locked, _, err := th.IsLocked(managerPinPrincipal(staff.ID), kioskPinThrottleKind)
	require.NoError(t, err)
	assert.True(t, locked, "wrong PINs must accumulate a kiosk lockout")

	// Even the correct PIN is now refused while locked.
	_, err = VerifyKioskPin(staff.ID, "123456")
	require.ErrorIs(t, err, ErrKioskPinLocked)
}

// TestVerifyKioskPin_KindIsolatedFromManagerPin is the security-critical test:
// a kiosk lockout must NOT lock the manager-PIN step-up counter, and vice
// versa. They share one throttle instance but separate kinds.
func TestVerifyKioskPin_KindIsolatedFromManagerPin(t *testing.T) {
	db := setupPinThrottleTestDB(t)
	th := auththrottle.New(db, auththrottle.Config{MaxAttempts: 3})
	SetManagerPinThrottle(th)
	t.Cleanup(func() { SetManagerPinThrottle(nil) })

	staff := seedStaffWithPin(t, db, "123456")
	principal := managerPinPrincipal(staff.ID)

	// Exhaust the KIOSK counter.
	for i := 0; i < 3; i++ {
		_, _ = VerifyKioskPin(staff.ID, "000000")
	}
	kioskLocked, _, err := th.IsLocked(principal, kioskPinThrottleKind)
	require.NoError(t, err)
	require.True(t, kioskLocked)

	// The manager-PIN counter must be untouched.
	mgrLocked, _, err := th.IsLocked(principal, managerPinThrottleKind)
	require.NoError(t, err)
	assert.False(t, mgrLocked, "a kiosk lockout must not lock the manager-PIN step-up")
}

// TestVerifyKioskPin_NoPinSet surfaces ErrPinNotSet without recording an
// attempt (a staffer who never enrolled is not brute-forcing anything).
func TestVerifyKioskPin_NoPinSet(t *testing.T) {
	db := setupPinThrottleTestDB(t)
	th := auththrottle.New(db, auththrottle.Config{MaxAttempts: 3})
	SetManagerPinThrottle(th)
	t.Cleanup(func() { SetManagerPinThrottle(nil) })

	staff := &database.Staff{
		BusinessID: 1, Email: fmt.Sprintf("nopin+%s@example.com", t.Name()),
		Name: "No Pin", Role: database.StaffRoleServer, InvitedBy: "o@example.com", IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(staff).Error)

	_, err := VerifyKioskPin(staff.ID, "123456")
	require.ErrorIs(t, err, database.ErrPinNotSet)

	locked, _, lerr := th.IsLocked(managerPinPrincipal(staff.ID), kioskPinThrottleKind)
	require.NoError(t, lerr)
	assert.False(t, locked, "an unenrolled staffer must not accrue a lockout")
}

// TestVerifyKioskPin_FailsClosedOnBrokenBackend: when the lockout backend
// errors (auth_attempts missing), VerifyKioskPin must DENY even a correct PIN.
func TestVerifyKioskPin_FailsClosedOnBrokenBackend(t *testing.T) {
	db := setupPinThrottleTestDB(t)
	staff := seedStaffWithPin(t, db, "123456")

	brokenDB, err := gorm.Open(
		sqlite.Open(fmt.Sprintf("file:kiosk_broken_%s?mode=memory&cache=shared", t.Name())),
		&gorm.Config{})
	require.NoError(t, err)
	brokenSQL, err := brokenDB.DB()
	require.NoError(t, err)
	brokenSQL.SetMaxOpenConns(1)
	// Intentionally NOT migrating AuthAttempt so IsLocked's SELECT errors.
	SetManagerPinThrottle(auththrottle.New(brokenDB, auththrottle.Config{MaxAttempts: 3}))
	t.Cleanup(func() { SetManagerPinThrottle(nil) })

	_, verr := VerifyKioskPin(staff.ID, "123456") // correct PIN — still denied
	require.ErrorIs(t, verr, ErrKioskPinLocked)
}

// TestVerifyKioskPin_NilThrottleStillVerifies: with no throttle installed
// (dev/tests) verification still works — a nil throttle disables lockout only.
func TestVerifyKioskPin_NilThrottleStillVerifies(t *testing.T) {
	db := setupPinThrottleTestDB(t)
	SetManagerPinThrottle(nil)
	staff := seedStaffWithPin(t, db, "123456")

	_, err := VerifyKioskPin(staff.ID, "123456")
	require.NoError(t, err)
	_, err = VerifyKioskPin(staff.ID, "000000")
	require.ErrorIs(t, err, database.ErrPinInvalid)
}
