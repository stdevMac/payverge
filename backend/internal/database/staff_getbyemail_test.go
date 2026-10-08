package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStaffGetByEmail_CaseAndWhitespaceInsensitive locks in the fix for staff
// members who could not receive an email login code (or sign in with Google)
// because their staff row was stored with a mixed-case address. Login and OAuth
// normalize the typed/Google email to lower-case, but the previous `email = ?`
// lookup is case-sensitive on Postgres — so the row was invisible and
// request-login-code reported "record not found" while still returning 200
// (no code was ever sent, and Google login redirected with staff_error=not_found).
func TestStaffGetByEmail_CaseAndWhitespaceInsensitive(t *testing.T) {
	setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&Staff{}))

	// Stored the way a legacy / un-normalized path would have left it: mixed case.
	stored := &Staff{
		BusinessID: 1,
		Email:      "Sample.StaffMember.98@Example.COM",
		Name:       "Sample Staff",
		Role:       "staff",
		IsActive:   true,
	}
	require.NoError(t, db.Create(stored).Error)

	svc := NewStaffService()

	// The normalized (lower-cased) address a login/OAuth caller passes must match.
	got, err := svc.GetByEmail("sample.staffmember.98@example.com")
	require.NoError(t, err, "lower-cased lookup must find a mixed-case stored email")
	require.NotNil(t, got)
	require.Equal(t, stored.ID, got.ID)

	// Stray surrounding whitespace on the query is tolerated too.
	got, err = svc.GetByEmail("  sample.staffmember.98@example.com  ")
	require.NoError(t, err)
	require.Equal(t, stored.ID, got.ID)

	// A genuinely different address still misses (we did not over-broaden matching).
	_, err = svc.GetByEmail("someone.else@example.com")
	require.Error(t, err)
}
