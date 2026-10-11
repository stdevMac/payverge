package auth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// countPasswordMatches swaps passwordMatches for a wrapper that counts calls
// and still performs the real compare. Cleanup restores the package var.
func countPasswordMatches(t *testing.T) *int {
	t.Helper()
	prev := passwordMatches
	n := 0
	passwordMatches = func(password, hash string) bool {
		n++
		return prev(password, hash)
	}
	t.Cleanup(func() { passwordMatches = prev })
	return &n
}

func TestLoginWithEmail_MissingAccountStillComparesPassword(t *testing.T) {
	h, _, _ := newRegisterSessionHandler(t)
	n := countPasswordMatches(t)

	_, err := h.authService.LoginWithEmail("nobody@example.com", "x")

	require.ErrorIs(t, err, ErrInvalidCredentials)
	require.Equal(t, 1, *n)
}

func TestLoginWithEmail_VerifiedWrongPasswordComparesOnce(t *testing.T) {
	h, _, _ := newRegisterSessionHandler(t)
	seedVerifiedEmailUser(t, h, "owner@example.com", nil)
	n := countPasswordMatches(t)

	_, err := h.authService.LoginWithEmail("owner@example.com", "x")

	require.ErrorIs(t, err, ErrInvalidCredentials)
	require.Equal(t, 1, *n)
}

func TestLoginWithEmail_UnverifiedWrongPasswordIsInvalidCredentials(t *testing.T) {
	setEmailMode(t, "smtp", "required")
	h, _, _ := newRegisterSessionHandler(t)
	registerForModeTest(t, h, "pending@example.com")

	_, err := h.authService.LoginWithEmail("pending@example.com", "wrong-password")

	require.ErrorIs(t, err, ErrInvalidCredentials)
	require.NotErrorIs(t, err, ErrEmailNotVerified)
}
