package auth

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVerificationTokenHashedAtRest proves the email-verification token is
// stored as a hash, never as the plaintext that is emailed to the user. A read
// of the user_auths table (backup leak, replica, injection elsewhere) must not
// expose a live, usable verification token.
func TestVerificationTokenHashedAtRest(t *testing.T) {
	_, db := newAuthEnvelopeHandler(t)
	svc := NewAuthService(db)
	user := database.User{Email: "verify@example.com", Name: "Ver", Role: "user", AuthMethod: "email"}
	require.NoError(t, db.Create(&user).Error)

	authRecord, plaintext, err := svc.RegisterWithEmail(user.Email, "password123", user.Name)
	require.NoError(t, err)
	authRecord.UserID = user.ID
	require.NotEmpty(t, plaintext, "RegisterWithEmail must return the plaintext token to email")
	require.NoError(t, svc.CreateAuth(authRecord))

	var stored UserAuth
	require.NoError(t, db.Where("provider = ? AND provider_user_id = ?", "email", "verify@example.com").First(&stored).Error)
	assert.NotEqual(t, plaintext, stored.VerificationToken, "verification token must not be stored in plaintext")
	assert.Equal(t, HashToken(plaintext), stored.VerificationToken, "verification token must be stored as its hash")

	// The at-rest value must NOT itself be usable as a verification token.
	require.Error(t, verifyEmailToken(svc, stored.VerificationToken), "the stored hash must not be a usable token")
	// The plaintext token verifies.
	require.NoError(t, verifyEmailToken(svc, plaintext))
}

// TestRotateVerificationTokenHashedAtRest covers the resend path.
func TestRotateVerificationTokenHashedAtRest(t *testing.T) {
	_, db := newAuthEnvelopeHandler(t)
	svc := NewAuthService(db)
	user := database.User{Email: "resend@example.com", Name: "Re", Role: "user", AuthMethod: "email"}
	require.NoError(t, db.Create(&user).Error)

	authRecord, _, err := svc.RegisterWithEmail(user.Email, "password123", user.Name)
	require.NoError(t, err)
	authRecord.UserID = user.ID
	require.NoError(t, svc.CreateAuth(authRecord))

	plaintext, err := svc.RotateVerificationToken("resend@example.com")
	require.NoError(t, err)
	require.NotEmpty(t, plaintext)

	var stored UserAuth
	require.NoError(t, db.Where("provider = ? AND provider_user_id = ?", "email", "resend@example.com").First(&stored).Error)
	assert.Equal(t, HashToken(plaintext), stored.VerificationToken, "rotated token must be stored as its hash")
	require.NoError(t, verifyEmailToken(svc, plaintext))
}

// TestResetTokenHashedAtRest proves the password-reset token is stored as a hash.
// A leaked reset token grants account takeover, so it must never sit in the DB
// in plaintext.
func TestResetTokenHashedAtRest(t *testing.T) {
	_, db := newAuthEnvelopeHandler(t)
	svc := NewAuthService(db)

	authRecord, _, err := svc.RegisterWithEmail("reset@example.com", "password123", "Reset")
	require.NoError(t, err)
	authRecord.UserID = 42 // ResetPassword returns this for session revocation
	require.NoError(t, svc.CreateAuth(authRecord))

	plaintext, err := svc.RequestPasswordReset("reset@example.com")
	require.NoError(t, err)
	require.NotEmpty(t, plaintext)

	var stored UserAuth
	require.NoError(t, db.Where("provider = ? AND provider_user_id = ?", "email", "reset@example.com").First(&stored).Error)
	assert.NotEqual(t, plaintext, stored.ResetToken, "reset token must not be stored in plaintext")
	assert.Equal(t, HashToken(plaintext), stored.ResetToken, "reset token must be stored as its hash")

	// The stored hash is not itself a usable reset token.
	_, err = svc.ResetPassword(stored.ResetToken, "newpassword123")
	require.Error(t, err, "the stored hash must not be a usable reset token")

	// The plaintext token resets the password and returns the user id.
	uid, err := svc.ResetPassword(plaintext, "newpassword123")
	require.NoError(t, err)
	assert.Equal(t, uint(42), uid)
}
