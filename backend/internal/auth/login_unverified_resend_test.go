package auth

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Unverified-login resends must flow through AuthService.ResendVerification —
// the 3/hour verificationLimiter owns ALL token rotation. Before this fix,
// every unverified login rotated the token unconditionally, invalidating the
// link the user was just emailed (P3).
func TestLoginUnverifiedRespectsResendLimiter(t *testing.T) {
	h, db, _ := newRegisterSessionHandler(t)

	w, c := postJSON(t, map[string]string{
		"email": "slow@example.com", "password": "password123", "name": "Slow",
	})
	h.Register(c)
	require.Equal(t, http.StatusCreated, w.Code)

	tokenHash := func() string {
		var rec UserAuth
		require.NoError(t, db.Where("provider = ? AND LOWER(provider_user_id) = LOWER(?)", "email", "slow@example.com").
			First(&rec).Error)
		return rec.VerificationToken
	}

	last := tokenHash()
	rotations := 0
	for i := 0; i < 6; i++ {
		wl, cl := postJSON(t, map[string]string{
			"email": "slow@example.com", "password": "password123",
		})
		h.Login(cl)
		require.Equal(t, http.StatusForbidden, wl.Code)
		if hsh := tokenHash(); hsh != last {
			rotations++
			last = hsh
		}
	}
	assert.LessOrEqual(t, rotations, 3,
		"token rotation on unverified login must be capped by the 3/hour resend limiter (was: unlimited)")
	assert.Greater(t, rotations, 0,
		"within the limiter budget a fresh link IS still issued")
}
