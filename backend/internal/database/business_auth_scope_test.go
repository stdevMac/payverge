package database

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedAuthScopeFatBusiness creates a Business whose sensitive/heavy columns
// (onboarding jsonb blob, text blobs) are populated alongside the
// auth-scope columns the middleware actually reads, so the projection test can
// prove GetBusinessAuthScopeByIdOrBusinessId never hydrates the blobs.
func seedAuthScopeFatBusiness(t testing.TB) *Business {
	t.Helper()
	uid := uint(4242)

	business := &Business{
		BusinessId:   fmt.Sprintf("auth-scope-%d", time.Now().UnixNano()),
		Name:         "Auth Scope Diner",
		OwnerAddress: "0xAUTHSCOPEowner",
		UserID:       &uid,
		IsActive:     true,
		// Heavy / sensitive columns that must NOT reach the auth context value.
		OnboardingState: JSONRawMessage(`{"secret":"do-not-leak"}`),
		Description:     "heavy text blob that the auth path never reads",
		WelcomeMessage:  "another heavy blob",
	}
	require.NoError(t, db.Create(business).Error)
	return business
}

// TestGetBusinessAuthScope_ProjectsAuthColumnsNotBlobs is the load-bearing
// access-shape guard for M1 (OF-01/MW-02): the auth middleware loader must read
// only the narrow auth/lifecycle columns and never the jsonb
// onboarding blob / text blobs, for BOTH the numeric-id and slug resolution
// paths.
func TestGetBusinessAuthScope_ProjectsAuthColumnsNotBlobs(t *testing.T) {
	rec := &publicBizSQLRecorder{}
	setupPublicBizProjectionDB(t, rec)
	business := seedAuthScopeFatBusiness(t)

	// Contrast / RED proof: the FULL loader DOES hydrate the secrets — this is
	// what makes the projection load-bearing.
	full, err := GetBusinessByIdOrBusinessId(fmt.Sprintf("%d", business.ID))
	require.NoError(t, err)
	require.NotEmpty(t, string(full.OnboardingState), "full loader must carry onboarding blob (contrast)")

	assertProjected := func(label, identifier string) {
		rec.reset()
		got, err := GetBusinessAuthScopeByIdOrBusinessId(identifier)
		require.NoError(t, err, label)
		require.NotNil(t, got, label)

		// Query shape: no SELECT * over businesses, and the projected SELECT must
		// NOT mention the sensitive/heavy columns.
		assert.Zero(t, rec.businessSelectStarCount(), "%s: must not SELECT * the business row", label)
		assert.False(t, rec.businessSelectMentions("stripe_customer_id"), "%s: must not project stripe_customer_id", label)
		assert.False(t, rec.businessSelectMentions("onboarding_state"), "%s: must not project onboarding_state", label)
		assert.False(t, rec.businessSelectMentions("description"), "%s: must not project description blob", label)
		// And it MUST project every auth/subscription column the consumers read.
		for _, col := range businessAuthScopeColumns {
			assert.True(t, rec.businessSelectMentions(col), "%s: projection must include %q", label, col)
		}

		// Hydrated struct: auth/subscription fields present...
		assert.Equal(t, business.ID, got.ID, label)
		assert.Equal(t, "0xAUTHSCOPEowner", got.OwnerAddress, label)
		require.NotNil(t, got.UserID, label)
		assert.Equal(t, uint(4242), *got.UserID, label)

		// ...and the blobs/Stripe IDs are NEVER hydrated.
		assert.Empty(t, string(got.OnboardingState), "%s: onboarding_state blob must not be hydrated", label)
		assert.Empty(t, got.Description, "%s: description blob must not be hydrated", label)
	}

	assertProjected("numeric-id path", fmt.Sprintf("%d", business.ID))
	assertProjected("slug path", business.BusinessId)
}

// TestGetBusinessAuthScope_LockStateParity proves the projected struct computes
// the SAME admin lifecycle lock as a full-row load across the lock matrix —
// the auth-path projection must lose nothing the lock gate reads. (If a lock
// column were dropped from the projection, a date/bool would zero and the
// lock state would diverge — this test would catch it.)
func TestGetBusinessAuthScope_LockStateParity(t *testing.T) {
	rec := &publicBizSQLRecorder{}
	setupPublicBizProjectionDB(t, rec)

	past := time.Now().Add(-72 * time.Hour)

	cases := []struct {
		name     string
		inactive bool
		closedAt *time.Time
		isDemo   bool
	}{
		{"active", false, nil, false},
		{"suspended", true, nil, false},
		{"closed", false, &past, false},
		{"suspended and closed", true, &past, false},
		{"demo suspended", true, nil, true},
	}

	for i, tc := range cases {
		uid := uint(1000 + i)
		biz := &Business{
			BusinessId:      fmt.Sprintf("lockparity-%d-%d", i, time.Now().UnixNano()),
			Name:            "Lock Parity",
			OwnerAddress:    fmt.Sprintf("0xlock%d", i),
			UserID:          &uid,
			IsActive:        true,
			IsDemo:          tc.isDemo,
			ClosedAt:        tc.closedAt,
			OnboardingState: JSONRawMessage(`{"x":1}`),
		}
		require.NoError(t, db.Create(biz).Error)
		if tc.inactive {
			require.NoError(t, db.Model(&Business{}).Where("id = ?", biz.ID).UpdateColumn("is_active", false).Error)
		}

		full, err := GetBusinessByIdOrBusinessId(fmt.Sprintf("%d", biz.ID))
		require.NoError(t, err, tc.name)
		projected, err := GetBusinessAuthScopeByIdOrBusinessId(fmt.Sprintf("%d", biz.ID))
		require.NoError(t, err, tc.name)

		assert.Equal(t, IsBusinessOperational(full), IsBusinessOperational(projected),
			"operational state must match between full and projected load for %q", tc.name)
		assert.Equal(t, AdminBusinessStatus(full), AdminBusinessStatus(projected),
			"admin status must match between full and projected load for %q", tc.name)
	}
}

// BenchmarkGetBusinessByIdOrBusinessIdFull is the M1 before-baseline: the full
// ~94-column row load the auth middleware previously did on every request.
func BenchmarkGetBusinessByIdOrBusinessIdFull(b *testing.B) {
	rec := &publicBizSQLRecorder{}
	setupPublicBizProjectionDB(b, rec)
	business := seedAuthScopeFatBusiness(b)
	id := fmt.Sprintf("%d", business.ID)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := GetBusinessByIdOrBusinessId(id); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkGetBusinessAuthScope is the M1 after-measurement: the narrow
// 14-column projection the auth middleware now does (includes demo_owner_user_id).
func BenchmarkGetBusinessAuthScope(b *testing.B) {
	rec := &publicBizSQLRecorder{}
	setupPublicBizProjectionDB(b, rec)
	business := seedAuthScopeFatBusiness(b)
	id := fmt.Sprintf("%d", business.ID)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := GetBusinessAuthScopeByIdOrBusinessId(id); err != nil {
			b.Fatal(err)
		}
	}
}
