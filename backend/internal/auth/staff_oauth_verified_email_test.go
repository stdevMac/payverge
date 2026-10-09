package auth

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestResolveStaffForGoogleLogin_UnverifiedEmailRejectedEvenWhenStaffExists is
// the core account-takeover regression for the staff Google login flow. The
// primary user flow already refuses to trust an unverified Google email
// (ErrUnverifiedOAuthEmail); the staff callback historically matched a staff row
// by email with no such check. Anyone who creates an *unverified* Google account
// bearing a staff member's email could then sign in as that staff member. The
// resolver must reject the login before the email match can authenticate,
// regardless of whether a matching active staff row exists.
func TestResolveStaffForGoogleLogin_UnverifiedEmailRejectedEvenWhenStaffExists(t *testing.T) {
	activeStaff := &database.Staff{ID: 7, BusinessID: 1, Email: "server@example.com", IsActive: true}
	lookup := func(email string) (*database.Staff, error) { return activeStaff, nil }

	userInfo := &GoogleUserInfo{ID: "attacker-google-id", Email: "server@example.com", VerifiedEmail: false}

	staff, err := resolveStaffForGoogleLogin(userInfo, lookup)
	require.ErrorIs(t, err, ErrStaffOAuthUnverifiedEmail, "unverified Google email must be rejected before matching staff")
	assert.Nil(t, staff, "no staff may be returned for an unverified takeover attempt")
}

// TestResolveStaffForGoogleLogin_VerifiedActiveStaffSucceeds is the positive
// control: a verified Google email matching an active staff member logs in.
func TestResolveStaffForGoogleLogin_VerifiedActiveStaffSucceeds(t *testing.T) {
	activeStaff := &database.Staff{ID: 7, BusinessID: 1, Email: "server@example.com", IsActive: true}
	lookup := func(email string) (*database.Staff, error) {
		assert.Equal(t, "server@example.com", email, "lookup must receive the lower-cased email")
		return activeStaff, nil
	}

	userInfo := &GoogleUserInfo{ID: "g", Email: "Server@Example.com", VerifiedEmail: true}

	staff, err := resolveStaffForGoogleLogin(userInfo, lookup)
	require.NoError(t, err)
	require.NotNil(t, staff)
	assert.Equal(t, uint(7), staff.ID)
}

// TestResolveStaffForGoogleLogin_NoMatchingStaff maps a verified email with no
// staff row to the not-found sentinel (existing ?staff_error=not_found redirect).
func TestResolveStaffForGoogleLogin_NoMatchingStaff(t *testing.T) {
	lookup := func(email string) (*database.Staff, error) { return nil, gorm.ErrRecordNotFound }

	userInfo := &GoogleUserInfo{ID: "g", Email: "ghost@example.com", VerifiedEmail: true}

	staff, err := resolveStaffForGoogleLogin(userInfo, lookup)
	require.ErrorIs(t, err, ErrStaffNotFound)
	assert.Nil(t, staff)
}

// TestResolveStaffForGoogleLogin_InactiveStaff maps a verified email matching an
// inactive staff row to the inactive sentinel (existing ?staff_error=inactive).
func TestResolveStaffForGoogleLogin_InactiveStaff(t *testing.T) {
	lookup := func(email string) (*database.Staff, error) {
		return &database.Staff{ID: 9, Email: "gone@example.com", IsActive: false}, nil
	}

	userInfo := &GoogleUserInfo{ID: "g", Email: "gone@example.com", VerifiedEmail: true}

	staff, err := resolveStaffForGoogleLogin(userInfo, lookup)
	require.ErrorIs(t, err, ErrStaffInactive)
	assert.Nil(t, staff)
}

func TestResolveStaffForGoogleLogin_MultipleMembershipsNeverGuessesScope(t *testing.T) {
	lookup := func(email string) (*database.Staff, error) {
		return nil, database.ErrStaffMembershipSelectionNeeded
	}
	userInfo := &GoogleUserInfo{ID: "g", Email: "multi@example.com", VerifiedEmail: true}

	staff, err := resolveStaffForGoogleLogin(userInfo, lookup)
	require.ErrorIs(t, err, ErrStaffMembershipSelectionNeeded)
	assert.Nil(t, staff)
}

func TestStaffOAuthMembershipSelectionRedirectCarriesSignedHandoffInFragment(t *testing.T) {
	payload := StaffOAuthMembershipSelectionPayload{
		SelectionToken: "short-lived-signed-proof",
		Memberships: []StaffOAuthMembershipChoice{
			{BusinessID: 11, BusinessName: "Cafe One", Role: database.StaffRoleServer},
			{BusinessID: 22, BusinessName: "Cafe Two", Role: database.StaffRoleManager},
		},
	}
	redirect, err := staffOAuthMembershipSelectionRedirect(
		"https://payverge.test/staff/login?redirect=%2Fstaff%2Fhome",
		payload,
	)
	require.NoError(t, err)
	require.NotContains(t, strings.Split(redirect, "#")[0], payload.SelectionToken,
		"selection proof must not enter query logs or referrers")

	parsed, err := url.Parse(redirect)
	require.NoError(t, err)
	encoded := parsed.Fragment
	require.True(t, strings.HasPrefix(encoded, "staff_membership_selection="))
	decoded, err := url.QueryUnescape(strings.TrimPrefix(encoded, "staff_membership_selection="))
	require.NoError(t, err)
	var got StaffOAuthMembershipSelectionPayload
	require.NoError(t, json.Unmarshal([]byte(decoded), &got))
	require.Equal(t, payload, got)
}

// TestResolveStaffForGoogleLogin_NilUserInfo fails closed on a nil identity.
func TestResolveStaffForGoogleLogin_NilUserInfo(t *testing.T) {
	lookup := func(email string) (*database.Staff, error) { return nil, errors.New("must not be called") }

	staff, err := resolveStaffForGoogleLogin(nil, lookup)
	require.ErrorIs(t, err, ErrStaffOAuthUnverifiedEmail)
	assert.Nil(t, staff)
}
