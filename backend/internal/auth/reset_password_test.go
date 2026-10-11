package auth

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/session"
)

// TestResetPassword_ReturnsUserIDForSessionRevocation guards the contract the
// handler relies on to revoke a user's sessions after a password reset: the
// service must return the affected user's ID (and zero on failure so the handler
// skips revocation).
func TestResetPassword_ReturnsUserIDForSessionRevocation(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)

	user := &database.User{Email: "reset@example.com"}
	require.NoError(t, db.Create(user).Error)

	oldHash, err := HashPassword("oldpassword123")
	require.NoError(t, err)
	expiry := time.Now().Add(time.Hour)
	authRec := &UserAuth{
		UserID:       user.ID,
		Provider:     "email",
		PasswordHash: oldHash,
		// Reset tokens are stored hashed at rest; seed the hash of the plaintext
		// the caller will present, matching what RequestPasswordReset now persists.
		ResetToken:  HashToken("reset-token-abc"),
		ResetExpiry: &expiry,
	}
	require.NoError(t, db.Create(authRec).Error)

	userID, err := h.authService.ResetPassword("reset-token-abc", "newpassword123")
	require.NoError(t, err)
	assert.Equal(t, user.ID, userID, "must return the affected user's ID so the handler can revoke sessions")

	var reloaded UserAuth
	require.NoError(t, db.First(&reloaded, authRec.ID).Error)
	assert.Empty(t, reloaded.ResetToken, "reset token must be cleared")
	assert.True(t, CheckPassword("newpassword123", reloaded.PasswordHash), "new password must verify")
	assert.False(t, CheckPassword("oldpassword123", reloaded.PasswordHash), "old password must no longer verify")

	// Invalid token: zero id + error, so the handler skips revocation.
	id, err := h.authService.ResetPassword("wrong-token", "whatever123")
	assert.Error(t, err)
	assert.Zero(t, id)
}

func TestResetPassword_RecordsSecurityResetReason(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)
	require.NoError(t, db.AutoMigrate(&session.UserSession{}))

	previousStore := session.GlobalStore
	session.GlobalStore = session.NewStore(db)
	t.Cleanup(func() { session.GlobalStore = previousStore })

	user := &database.User{Email: "reset-handler@example.com"}
	require.NoError(t, db.Create(user).Error)
	expiry := time.Now().Add(time.Hour)
	passwordHash, err := HashPassword("oldpassword123")
	require.NoError(t, err)
	require.NoError(t, db.Create(&UserAuth{
		UserID:       user.ID,
		Provider:     "email",
		PasswordHash: passwordHash,
		ResetToken:   HashToken("handler-reset-token"),
		ResetExpiry:  &expiry,
	}).Error)
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &user.ID,
		TokenHash: session.HashToken("reset-access-token"),
		Provider:  "email",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	w, c := postJSON(t, map[string]string{
		"token":        "handler-reset-token",
		"new_password": "newpassword123",
	})
	h.ResetPassword(c)

	assert.Equal(t, http.StatusOK, w.Code)
	var persisted session.UserSession
	require.NoError(t, db.First(&persisted, sess.ID).Error)
	assert.True(t, persisted.Revoked)
	assert.Equal(t, string(session.RevocationReasonSecurityReset), persisted.RevocationReason)
}

func TestResetPassword_RevokesOnlyUserProviderClass(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)
	require.NoError(t, db.AutoMigrate(&session.UserSession{}))

	previousStore := session.GlobalStore
	session.GlobalStore = session.NewStore(db)
	t.Cleanup(func() { session.GlobalStore = previousStore })

	user := &database.User{Email: "reset-isolation@example.com"}
	require.NoError(t, db.Create(user).Error)
	expiry := time.Now().Add(time.Hour)
	passwordHash, err := HashPassword("oldpassword123")
	require.NoError(t, err)
	require.NoError(t, db.Create(&UserAuth{
		UserID:       user.ID,
		Provider:     "email",
		PasswordHash: passwordHash,
		ResetToken:   HashToken("isolation-reset-token"),
		ResetExpiry:  &expiry,
	}).Error)

	sessions := make(map[string]*session.UserSession)
	for _, provider := range []string{"email", "dynamic_web3", "staff_code", "customer"} {
		sessions[provider], err = session.GlobalStore.Create(session.CreateInput{
			UserID:    &user.ID,
			TokenHash: session.HashToken("reset-isolation-" + provider),
			Provider:  provider,
			ExpiresAt: time.Now().Add(time.Hour),
		})
		require.NoError(t, err)
	}

	w, c := postJSON(t, map[string]string{
		"token":        "isolation-reset-token",
		"new_password": "newpassword123",
	})
	h.ResetPassword(c)
	require.Equal(t, http.StatusOK, w.Code)

	for _, provider := range []string{"email", "dynamic_web3"} {
		var persisted session.UserSession
		require.NoError(t, db.First(&persisted, sessions[provider].ID).Error)
		assert.True(t, persisted.Revoked, "%s belongs to the user identity namespace", provider)
		assert.Equal(t, string(session.RevocationReasonSecurityReset), persisted.RevocationReason)
	}
	for _, provider := range []string{"staff_code", "customer"} {
		var persisted session.UserSession
		require.NoError(t, db.First(&persisted, sessions[provider].ID).Error)
		assert.False(t, persisted.Revoked, "%s shares only the numeric id and must survive", provider)
		assert.Nil(t, persisted.RevokedAt)
		assert.Empty(t, persisted.RevocationReason)
	}
}

func TestResetPassword_RevokesAddressKeyedOperatorSession(t *testing.T) {
	h, db := newAuthEnvelopeHandler(t)
	require.NoError(t, db.AutoMigrate(&session.UserSession{}))

	previousStore := session.GlobalStore
	session.GlobalStore = session.NewStore(db)
	t.Cleanup(func() { session.GlobalStore = previousStore })

	const wallet = "0xabc123abc123abc123abc123abc123abc123abcd"
	user := &database.User{Email: "reset-wallet@example.com", Address: wallet}
	require.NoError(t, db.Create(user).Error)
	expiry := time.Now().Add(time.Hour)
	passwordHash, err := HashPassword("oldpassword123")
	require.NoError(t, err)
	require.NoError(t, db.Create(&UserAuth{
		UserID:       user.ID,
		Provider:     "email",
		PasswordHash: passwordHash,
		ResetToken:   HashToken("wallet-reset-token"),
		ResetExpiry:  &expiry,
	}).Error)

	addressSess, err := session.GlobalStore.Create(session.CreateInput{
		Address:   wallet,
		TokenHash: session.HashToken("reset-address-only"),
		Provider:  "web3",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	require.Nil(t, addressSess.UserID)

	unrelated, err := session.GlobalStore.Create(session.CreateInput{
		Address:   "0xdddddddddddddddddddddddddddddddddddddddd",
		TokenHash: session.HashToken("reset-unrelated-address"),
		Provider:  "web3",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	w, c := postJSON(t, map[string]string{
		"token":        "wallet-reset-token",
		"new_password": "newpassword123",
	})
	h.ResetPassword(c)
	require.Equal(t, http.StatusOK, w.Code)

	var persisted session.UserSession
	require.NoError(t, db.First(&persisted, addressSess.ID).Error)
	assert.True(t, persisted.Revoked)
	assert.Equal(t, string(session.RevocationReasonSecurityReset), persisted.RevocationReason)

	var other session.UserSession
	require.NoError(t, db.First(&other, unrelated.ID).Error)
	assert.False(t, other.Revoked)
}
