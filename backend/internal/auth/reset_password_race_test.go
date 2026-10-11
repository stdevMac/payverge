package auth

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func seedResetToken(t *testing.T, email, token string) (*AuthHandler, *UserAuth) {
	t.Helper()
	h, db := newAuthEnvelopeHandler(t)
	user := &database.User{Email: email}
	require.NoError(t, db.Create(user).Error)
	oldHash, err := HashPassword("oldpassword123")
	require.NoError(t, err)
	expiry := time.Now().Add(time.Hour)
	rec := &UserAuth{
		UserID:       user.ID,
		Provider:     "email",
		PasswordHash: oldHash,
		ResetToken:   HashToken(token),
		ResetExpiry:  &expiry,
	}
	require.NoError(t, db.Create(rec).Error)
	return h, rec
}

// Low (reset race): a token consumed between the lookup and the write (a
// concurrent reset) must not be honoured a second time, and the competing
// password must survive.
func TestResetPassword_TokenConsumedConcurrentlyIsNotReused(t *testing.T) {
	h, rec := seedResetToken(t, "race@example.com", "race-token")
	db := h.db

	competingHash, err := HashPassword("competing-password-1")
	require.NoError(t, err)
	resetPasswordAfterLookupHook = func() {
		// Another request wins the race and consumes the token.
		require.NoError(t, db.Model(&UserAuth{}).Where("id = ?", rec.ID).Updates(map[string]interface{}{
			"password_hash": competingHash,
			"reset_token":   "",
			"reset_expiry":  nil,
		}).Error)
	}
	t.Cleanup(func() { resetPasswordAfterLookupHook = nil })

	id, err := h.authService.ResetPassword("race-token", "attacker-password-2")
	assert.ErrorIs(t, err, ErrTokenInvalid)
	assert.Zero(t, id)

	var reloaded UserAuth
	require.NoError(t, db.First(&reloaded, rec.ID).Error)
	assert.True(t, CheckPassword("competing-password-1", reloaded.PasswordHash), "the winning reset must not be overwritten")
}

func TestResetPassword_ConcurrentResetsWithOneTokenSucceedOnce(t *testing.T) {
	h, rec := seedResetToken(t, "race-many@example.com", "race-many-token")

	const n = 6
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := h.authService.ResetPassword("race-many-token", "newpassword123")
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			assert.ErrorIs(t, err, ErrTokenInvalid)
		}
	}
	assert.Equal(t, 1, successes, "one reset token must change the password exactly once")

	var reloaded UserAuth
	require.NoError(t, h.db.First(&reloaded, rec.ID).Error)
	assert.Empty(t, reloaded.ResetToken)
	assert.Nil(t, reloaded.ResetExpiry)
}

func TestResetPassword_ExpiredTokenStillReportsExpired(t *testing.T) {
	h, rec := seedResetToken(t, "expired@example.com", "expired-token")
	past := time.Now().Add(-time.Minute)
	require.NoError(t, h.db.Model(&UserAuth{}).Where("id = ?", rec.ID).Update("reset_expiry", past).Error)

	id, err := h.authService.ResetPassword("expired-token", "newpassword123")
	assert.ErrorIs(t, err, ErrTokenExpired)
	assert.Zero(t, id)

	_, err = h.authService.ResetPassword("", "newpassword123")
	assert.ErrorIs(t, err, ErrTokenInvalid)
}
