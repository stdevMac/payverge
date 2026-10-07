package auth

import (
	"net/http"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/session"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedVerifiedEmailUser creates a users row + verified email UserAuth so Login
// passes credential + verification checks and reaches the deleted_at gate.
func seedVerifiedEmailUser(t *testing.T, h *AuthHandler, email string, deletedAt *time.Time) {
	t.Helper()
	hash, err := HashPassword("password123")
	require.NoError(t, err)

	user := database.User{Email: email, Name: "Owner", Role: "user", EmailVerified: true, DeletedAt: deletedAt}
	require.NoError(t, h.db.Create(&user).Error)
	require.NoError(t, h.db.Create(&UserAuth{
		UserID: user.ID, Provider: "email", ProviderUserID: email,
		PasswordHash: hash, EmailVerified: true,
	}).Error)
}

// TestLogin_DeletedAccountIsRejected locks P1-11: correct credentials on a
// soft-deleted user must NOT sign in — deletion was previously enforced by
// nothing, so a "deleted" account could keep using the product.
func TestLogin_DeletedAccountIsRejected(t *testing.T) {
	h, _ := newAuthEnvelopeHandler(t)

	prevStore := session.GlobalStore
	session.GlobalStore = nil
	t.Cleanup(func() { session.GlobalStore = prevStore })

	deletedAt := time.Now().UTC()
	seedVerifiedEmailUser(t, h, "gone@example.com", &deletedAt)

	w, c := postJSON(t, map[string]string{"email": "gone@example.com", "password": "password123"})
	h.Login(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "AUTH_FORBIDDEN")
	assert.Contains(t, w.Body.String(), "scheduled for deletion")
}

// TestLogin_LiveAccountStillSignsIn is the control: the gate must not misfire
// on accounts without deleted_at.
func TestLogin_LiveAccountStillSignsIn(t *testing.T) {
	h, _ := newAuthEnvelopeHandler(t)

	prevStore := session.GlobalStore
	session.GlobalStore = nil
	t.Cleanup(func() { session.GlobalStore = prevStore })

	seedVerifiedEmailUser(t, h, "alive@example.com", nil)

	w, c := postJSON(t, map[string]string{"email": "alive@example.com", "password": "password123"})
	h.Login(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"success":true`)
}
