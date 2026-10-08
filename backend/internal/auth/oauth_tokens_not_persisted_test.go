package auth

import (
	"reflect"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Provider access and refresh tokens are used in memory during the Google
// callback and must not be stored on user_auths.
func TestCreateOrUpdateOAuthUser_DoesNotPersistProviderTokens(t *testing.T) {
	_, db := newAuthEnvelopeHandler(t)
	svc := NewAuthService(db)

	authRecord, isNew, err := svc.CreateOrUpdateOAuthUser(
		"google", "g-no-tokens", "oauth-notokens@example.com", "OAuth No Tokens", true,
	)
	require.NoError(t, err)
	require.True(t, isNew)

	user := database.User{
		Email:      "oauth-notokens@example.com",
		Name:       "OAuth No Tokens",
		Role:       "user",
		AuthMethod: "google",
	}
	require.NoError(t, db.Create(&user).Error)
	authRecord.UserID = user.ID
	require.NoError(t, svc.CreateAuth(authRecord))

	var userCount int64
	require.NoError(t, db.Model(&database.User{}).Where("email = ?", user.Email).Count(&userCount).Error)
	assert.Equal(t, int64(1), userCount)

	var stored UserAuth
	require.NoError(t, db.Where("provider = ? AND provider_user_id = ?", "google", "g-no-tokens").First(&stored).Error)
	assert.Equal(t, user.ID, stored.UserID)
	assert.True(t, stored.EmailVerified)

	for _, col := range []string{"access_token", "refresh_token", "token_expiry"} {
		assert.Falsef(t, db.Migrator().HasColumn(&UserAuth{}, col), "user_auths must not have column %s", col)
	}

	typ := reflect.TypeOf(UserAuth{})
	_, hasAccess := typ.FieldByName("AccessToken")
	_, hasRefresh := typ.FieldByName("RefreshToken")
	_, hasExpiry := typ.FieldByName("TokenExpiry")
	assert.False(t, hasAccess)
	assert.False(t, hasRefresh)
	assert.False(t, hasExpiry)
}
