package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SEC-4 / #301: password reset must rate-limit per email like verification
// resend, and rate-limited requests must not invalidate the prior token.
func TestRequestPasswordReset_PerEmailRateLimitPreservesPriorToken(t *testing.T) {
	_, db := newAuthEnvelopeHandler(t)
	svc := NewAuthService(db)

	authRecord, _, err := svc.RegisterWithEmail("reset-limit@example.com", "password123", "Reset")
	require.NoError(t, err)
	authRecord.UserID = 7
	require.NoError(t, svc.CreateAuth(authRecord))

	first, err := svc.RequestPasswordReset("reset-limit@example.com")
	require.NoError(t, err)
	require.NotEmpty(t, first)

	var stored UserAuth
	require.NoError(t, db.Where("provider = ? AND LOWER(provider_user_id) = LOWER(?)", "email", "reset-limit@example.com").First(&stored).Error)
	firstHash := stored.ResetToken
	assert.Equal(t, HashToken(first), firstHash)

	// Exhaust the 3/hour budget (first already consumed one).
	for i := 0; i < 2; i++ {
		tok, err := svc.RequestPasswordReset("reset-limit@example.com")
		require.NoError(t, err)
		require.NotEmpty(t, tok)
	}

	// Fourth request is rate-limited: empty token, prior hash untouched.
	blocked, err := svc.RequestPasswordReset("reset-limit@example.com")
	require.NoError(t, err)
	assert.Empty(t, blocked)

	require.NoError(t, db.Where("provider = ? AND LOWER(provider_user_id) = LOWER(?)", "email", "reset-limit@example.com").First(&stored).Error)
	assert.NotEmpty(t, stored.ResetToken, "rate-limited request must not clear the prior token")
	// The third successful request rotated the hash; blocked must not change it again.
	thirdHash := stored.ResetToken
	assert.NotEqual(t, "", thirdHash)

	blockedAgain, err := svc.RequestPasswordReset("reset-limit@example.com")
	require.NoError(t, err)
	assert.Empty(t, blockedAgain)
	require.NoError(t, db.Where("provider = ? AND LOWER(provider_user_id) = LOWER(?)", "email", "reset-limit@example.com").First(&stored).Error)
	assert.Equal(t, thirdHash, stored.ResetToken, "further rate-limited requests must preserve the stored hash")
}

func TestRequestPasswordReset_UnknownEmailStillConsumesBudget(t *testing.T) {
	_, db := newAuthEnvelopeHandler(t)
	svc := NewAuthService(db)

	for i := 0; i < 3; i++ {
		tok, err := svc.RequestPasswordReset("nobody@example.com")
		require.ErrorIs(t, err, ErrUserNotFound)
		assert.Empty(t, tok)
	}
	tok, err := svc.RequestPasswordReset("nobody@example.com")
	require.NoError(t, err, "rate-limited unknown emails return nil error for anti-enumeration")
	assert.Empty(t, tok)

	_ = db // keep fixture helper side effects consistent
}
