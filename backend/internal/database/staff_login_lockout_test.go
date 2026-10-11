package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestStaffLoginLockout locks in the email-code brute-force protection: after
// StaffLoginMaxFailedAttempts failed verifications the staff member is locked
// out, outstanding codes are burned, and a successful reset clears the state.
func TestStaffLoginLockout(t *testing.T) {
	setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&Staff{}, &StaffLoginCode{}))

	staff := &Staff{BusinessID: 1, Email: "s@x.com", Name: "S", Role: "staff", IsActive: true}
	require.NoError(t, db.Create(staff).Error)

	// An outstanding, still-valid code that should be burned on lockout.
	require.NoError(t, db.Create(&StaffLoginCode{
		StaffID: staff.ID, Code: "123456", ExpiresAt: time.Now().Add(10 * time.Minute),
	}).Error)

	svc := NewStaffService()

	// First N-1 failures do not lock.
	for i := 0; i < StaffLoginMaxFailedAttempts-1; i++ {
		locked, err := svc.RegisterFailedLoginAttempt(staff.ID)
		require.NoError(t, err)
		require.False(t, locked, "attempt %d should not lock yet", i+1)
	}

	// The threshold attempt locks.
	locked, err := svc.RegisterFailedLoginAttempt(staff.ID)
	require.NoError(t, err)
	require.True(t, locked)

	isLocked, until, err := svc.IsLoginLocked(staff.ID)
	require.NoError(t, err)
	require.True(t, isLocked)
	require.NotNil(t, until)

	// Outstanding code was burned.
	var unused int64
	require.NoError(t, db.Model(&StaffLoginCode{}).
		Where("staff_id = ? AND used = ?", staff.ID, false).Count(&unused).Error)
	require.Equal(t, int64(0), unused)

	// Reset clears the lock and counter.
	require.NoError(t, svc.ResetLoginAttempts(staff.ID))
	isLocked, _, err = svc.IsLoginLocked(staff.ID)
	require.NoError(t, err)
	require.False(t, isLocked)
}

// TestReserveLoginAttempt_CeilingLockAndExpiry: reservations are claimed per
// identity before the code is checked, the one reaching the ceiling locks the
// row, a locked row refuses every further claim, an expired lock restarts the
// count, and a multi-membership claim is all-or-nothing.
func TestReserveLoginAttempt_CeilingLockAndExpiry(t *testing.T) {
	setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&Staff{}, &StaffLoginCode{}))
	a := &Staff{BusinessID: 1, Email: "r@x.com", Name: "R", Role: "staff", IsActive: true}
	b := &Staff{BusinessID: 2, Email: "r@x.com", Name: "R", Role: "staff", IsActive: true}
	require.NoError(t, db.Create(a).Error)
	require.NoError(t, db.Create(b).Error)
	require.NoError(t, db.Create(&StaffLoginCode{
		StaffID: a.ID, Code: "123456", ExpiresAt: time.Now().Add(10 * time.Minute),
	}).Error)
	svc := NewStaffService()
	ids := []uint{a.ID, b.ID}

	for i := 0; i < StaffLoginMaxFailedAttempts; i++ {
		ok, err := svc.ReserveLoginAttempt(ids)
		require.NoError(t, err)
		require.True(t, ok, "attempt %d", i+1)
	}
	locked, _, err := svc.IsLoginLocked(a.ID)
	require.NoError(t, err)
	require.True(t, locked, "the attempt reaching the ceiling locks the identity")
	ok, err := svc.ReserveLoginAttempt(ids)
	require.NoError(t, err)
	require.False(t, ok, "a locked identity refuses further attempts")

	require.NoError(t, svc.BurnCodesIfLoginLocked(ids))
	var code StaffLoginCode
	require.NoError(t, db.Where("staff_id = ?", a.ID).First(&code).Error)
	require.True(t, code.Used, "codes are burned once locked")

	// Lock expires: the count restarts at one instead of re-locking.
	past := time.Now().Add(-time.Minute)
	require.NoError(t, db.Model(&Staff{}).Where("id IN ?", ids).Update("login_code_locked_until", past).Error)
	ok, err = svc.ReserveLoginAttempt(ids)
	require.NoError(t, err)
	require.True(t, ok)
	var fresh Staff
	require.NoError(t, db.First(&fresh, a.ID).Error)
	require.Equal(t, 1, fresh.LoginCodeFailedAttempts)
	require.Nil(t, fresh.LoginCodeLockedUntil)

	// All-or-nothing: one locked membership refuses the claim for the email
	// and leaves the other membership's count untouched.
	future := time.Now().Add(time.Minute)
	require.NoError(t, db.Model(&Staff{}).Where("id = ?", b.ID).Update("login_code_locked_until", future).Error)
	ok, err = svc.ReserveLoginAttempt(ids)
	require.NoError(t, err)
	require.False(t, ok)
	require.NoError(t, db.First(&fresh, a.ID).Error)
	require.Equal(t, 1, fresh.LoginCodeFailedAttempts, "a refused claim rolls back")
}
