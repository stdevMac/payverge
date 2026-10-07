package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGoogleUserInfoParsesVerifiedEmailTag confirms the GoogleUserInfo struct's
// JSON tag matches the field name returned by the legacy v2 userinfo endpoint
// (https://www.googleapis.com/oauth2/v2/userinfo), which emits "verified_email".
// If this tag ever drifts, VerifiedEmail would silently parse as false and the
// merge gate below would lock out every legitimate Google user.
func TestGoogleUserInfoParsesVerifiedEmailTag(t *testing.T) {
	raw := []byte(`{"id":"123","email":"alice@example.com","verified_email":true,"name":"Alice"}`)
	info, err := parseGoogleUserInfo(raw)
	require.NoError(t, err)
	assert.True(t, info.VerifiedEmail, "verified_email:true from the v2 endpoint must populate VerifiedEmail")

	rawFalse := []byte(`{"id":"123","email":"alice@example.com","verified_email":false,"name":"Alice"}`)
	infoFalse, err := parseGoogleUserInfo(rawFalse)
	require.NoError(t, err)
	assert.False(t, infoFalse.VerifiedEmail, "verified_email:false must populate VerifiedEmail=false")
}

// TestGoogleMerge_UnverifiedEmail_DoesNotTakeOverExistingAccount is the core
// account-takeover regression. A new Google identity whose email Google reports
// as NOT verified must NOT be linked into a pre-existing account that shares the
// email. The existing account must be left untouched and the resolver must
// signal a rejection.
func TestGoogleMerge_UnverifiedEmail_DoesNotTakeOverExistingAccount(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)

	// Pre-existing victim account, registered via email/password.
	victim := &database.User{Email: "victim@example.com", Name: "Victim", AuthMethod: "email", Role: "user"}
	require.NoError(t, db.Create(victim).Error)
	emailAuth := &UserAuth{UserID: victim.ID, Provider: "email", ProviderUserID: "victim@example.com", EmailVerified: true}
	require.NoError(t, db.Create(emailAuth).Error)

	// Attacker controls a Google account with the victim's email but Google
	// has NOT verified it.
	userInfo := &GoogleUserInfo{
		ID:            "attacker-google-id",
		Email:         "victim@example.com",
		VerifiedEmail: false,
		Name:          "Attacker",
	}
	authRecord := &UserAuth{Provider: "google", ProviderUserID: userInfo.ID, EmailVerified: false}

	_, _, err := h.resolveGoogleCallbackUserWithInvite(context.Background(), userInfo, authRecord, true /*isNewUser*/, false /*isLinking*/, 0, "")
	require.ErrorIs(t, err, ErrUnverifiedOAuthEmail, "unverified Google email must be rejected, not merged")

	// The Google auth record must NOT have been linked to the victim.
	var googleAuthCount int64
	require.NoError(t, db.Model(&UserAuth{}).
		Where("provider = ? AND provider_user_id = ?", "google", "attacker-google-id").
		Count(&googleAuthCount).Error)
	assert.Zero(t, googleAuthCount, "no Google auth row may be created for an unverified takeover attempt")

	// The victim's account is untouched: still exactly one auth (email), name intact.
	var victimAuths []UserAuth
	require.NoError(t, db.Where("user_id = ?", victim.ID).Find(&victimAuths).Error)
	assert.Len(t, victimAuths, 1, "victim must still have exactly one (email) auth method")
	assert.Equal(t, "email", victimAuths[0].Provider)

	var reloaded database.User
	require.NoError(t, db.First(&reloaded, victim.ID).Error)
	assert.Equal(t, "Victim", reloaded.Name, "victim name must be untouched")
}

// TestGoogleMerge_VerifiedEmail_LinksIntoExistingAccount is the positive
// control: a Google identity whose email IS verified still links into the
// pre-existing email account, exactly as before the gate.
func TestGoogleMerge_VerifiedEmail_LinksIntoExistingAccount(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)

	existing := &database.User{Email: "owner@example.com", Name: "Owner", AuthMethod: "email", Role: "user"}
	require.NoError(t, db.Create(existing).Error)
	emailAuth := &UserAuth{UserID: existing.ID, Provider: "email", ProviderUserID: "owner@example.com", EmailVerified: true}
	require.NoError(t, db.Create(emailAuth).Error)

	userInfo := &GoogleUserInfo{
		ID:            "owner-google-id",
		Email:         "owner@example.com",
		VerifiedEmail: true,
		Name:          "Owner",
	}
	authRecord := &UserAuth{Provider: "google", ProviderUserID: userInfo.ID, EmailVerified: true}

	user, isNew, err := h.resolveGoogleCallbackUserWithInvite(context.Background(), userInfo, authRecord, true /*isNewUser*/, false /*isLinking*/, 0, "")
	require.NoError(t, err)
	assert.False(t, isNew, "linking into an existing account is not a new user")
	assert.Equal(t, existing.ID, user.ID, "verified Google login must resolve to the existing account")

	// The Google auth row is now linked to the existing user.
	var googleAuth UserAuth
	require.NoError(t, db.Where("provider = ? AND provider_user_id = ?", "google", "owner-google-id").First(&googleAuth).Error)
	assert.Equal(t, existing.ID, googleAuth.UserID, "Google auth must be linked to the existing account")

	// Two auth methods now: email + google.
	var auths []UserAuth
	require.NoError(t, db.Where("user_id = ?", existing.ID).Find(&auths).Error)
	assert.Len(t, auths, 2)
}

// TestGoogleMerge_VerifiedEmail_NoExistingAccount_CreatesNewUser confirms the
// brand-new-user path still works for a verified Google email with no
// pre-existing account.
func TestGoogleMerge_VerifiedEmail_NoExistingAccount_CreatesNewUser(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)

	userInfo := &GoogleUserInfo{
		ID:            "fresh-google-id",
		Email:         "fresh@example.com",
		VerifiedEmail: true,
		Name:          "Fresh",
	}
	authRecord := &UserAuth{Provider: "google", ProviderUserID: userInfo.ID, EmailVerified: true}

	user, isNew, err := h.resolveGoogleCallbackUserWithInvite(context.Background(), userInfo, authRecord, true /*isNewUser*/, false /*isLinking*/, 0, "")
	require.NoError(t, err)
	assert.True(t, isNew, "a fresh account with no prior match is a new user")
	assert.NotZero(t, user.ID)
	assert.Equal(t, "fresh@example.com", user.Email)

	var googleAuth UserAuth
	require.NoError(t, db.Where("provider = ? AND provider_user_id = ?", "google", "fresh-google-id").First(&googleAuth).Error)
	assert.Equal(t, user.ID, googleAuth.UserID)
}

func TestGoogleCallbackUser_RejectsArchivedAccount(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)

	deletedAt := time.Now().UTC()
	archived := &database.User{
		Email: "archived-google@example.com", Name: "Archived", Role: "user",
		AuthMethod: "google", DeletedAt: &deletedAt,
	}
	require.NoError(t, db.Create(archived).Error)
	authRecord := &UserAuth{UserID: archived.ID, Provider: "google", ProviderUserID: "archived-google-id", EmailVerified: true}
	require.NoError(t, db.Create(authRecord).Error)

	info := &GoogleUserInfo{
		ID: "archived-google-id", Email: archived.Email, VerifiedEmail: true, Name: "Restored?", Picture: "https://example.test/pic.png",
	}

	user, isNew, err := h.resolveGoogleCallbackUserWithInvite(context.Background(), info, authRecord, false, false, 0, "")
	require.ErrorIs(t, err, ErrArchivedAccount)
	assert.False(t, isNew)
	assert.Zero(t, user.ID)

	var persisted database.User
	require.NoError(t, db.First(&persisted, archived.ID).Error)
	assert.NotNil(t, persisted.DeletedAt)
	assert.Equal(t, "Archived", persisted.Name)
	assert.Empty(t, persisted.Picture, "archived admission must not update the user")
}

func TestGoogleMerge_RejectsArchivedExistingEmailAccount(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)

	deletedAt := time.Now().UTC()
	archived := &database.User{
		Email: "archived-merge@example.com", Name: "Archived Merge", Role: "user",
		AuthMethod: "email", DeletedAt: &deletedAt,
	}
	require.NoError(t, db.Create(archived).Error)
	require.NoError(t, db.Create(&UserAuth{
		UserID: archived.ID, Provider: "email", ProviderUserID: archived.Email, EmailVerified: true,
	}).Error)

	info := &GoogleUserInfo{
		ID: "new-google-id", Email: archived.Email, VerifiedEmail: true, Name: "Google Name",
	}
	authRecord := &UserAuth{Provider: "google", ProviderUserID: info.ID, EmailVerified: true}

	user, isNew, err := h.resolveGoogleCallbackUserWithInvite(context.Background(), info, authRecord, true, false, 0, "")
	require.ErrorIs(t, err, ErrArchivedAccount)
	assert.False(t, isNew)
	assert.Zero(t, user.ID)

	var googleAuthCount int64
	require.NoError(t, db.Model(&UserAuth{}).
		Where("provider = ? AND provider_user_id = ?", "google", info.ID).
		Count(&googleAuthCount).Error)
	assert.Zero(t, googleAuthCount, "archived merge must not attach a Google identity")

	var persisted database.User
	require.NoError(t, db.First(&persisted, archived.ID).Error)
	assert.NotNil(t, persisted.DeletedAt)
	assert.Equal(t, "Archived Merge", persisted.Name)
}

// TestCreateOrUpdateOAuthUser_HonorsVerifiedFlag proves the service no longer
// hardcodes EmailVerified=true: a new OAuth auth record reflects the verified
// flag the caller passed.
func TestCreateOrUpdateOAuthUser_HonorsVerifiedFlag(t *testing.T) {
	_, db := newAuthEnvelopeHandler(t)
	svc := NewAuthService(db)

	verified, isNew, err := svc.CreateOrUpdateOAuthUser("google", "g-verified", "v@example.com", "V", true)
	require.NoError(t, err)
	assert.True(t, isNew)
	assert.True(t, verified.EmailVerified, "verified flag must propagate to the auth record")

	unverified, isNew, err := svc.CreateOrUpdateOAuthUser("google", "g-unverified", "u@example.com", "U", false)
	require.NoError(t, err)
	assert.True(t, isNew)
	assert.False(t, unverified.EmailVerified, "unverified flag must propagate to the auth record")
}
