package auth

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

// Test-only fixture credentials for a locally seeded email/password admin.
const (
	localAdminEmail    = "admin@local.test"
	localAdminPassword = "Saffron-Lantern-Harbor-73!"
)

// seedLocalAdminAsDeployDoes mirrors a local admin seed after
// sanitize-equivalent redaction: insert users(role=admin) + user_auths
// with a bcrypt hash produced by HashPassword (same as cmd/hash-password).
func seedLocalAdminAsDeployDoes(t *testing.T, h *AuthHandler) {
	t.Helper()

	// Sanitize redacts password hashes to '[redacted]' and emails to
	// redacted-<id>@example.invalid. Start from that world, then re-seed.
	require.NoError(t, h.db.Where("1 = 1").Delete(&UserAuth{}).Error)
	require.NoError(t, h.db.Where("1 = 1").Delete(&database.User{}).Error)

	hash, err := HashPassword(localAdminPassword)
	require.NoError(t, err)
	require.True(t, len(hash) > 0 && hash[0] == '$', "hash must look like bcrypt")

	user := database.User{
		Email:         localAdminEmail,
		Name:          "Local Admin",
		Role:          "admin",
		AuthMethod:    "email",
		EmailVerified: true,
	}
	require.NoError(t, h.db.Create(&user).Error)
	require.NoError(t, h.db.Create(&UserAuth{
		UserID:         user.ID,
		Provider:       "email",
		ProviderUserID: localAdminEmail,
		PasswordHash:   hash,
		EmailVerified:  true,
	}).Error)
}

func TestLocalAdminSeed_LoginWithDocumentedPassword(t *testing.T) {
	h, _ := newAuthEnvelopeHandler(t)
	seedLocalAdminAsDeployDoes(t, h)

	// Drive the real LoginWithEmail path used by POST /auth/login.
	authRec, err := h.authService.LoginWithEmail(localAdminEmail, localAdminPassword)
	require.NoError(t, err)
	require.NotNil(t, authRec)
	assert.Equal(t, "email", authRec.Provider)
	assert.True(t, authRec.EmailVerified)
	assert.True(t, CheckPassword(localAdminPassword, authRec.PasswordHash))

	var user database.User
	require.NoError(t, h.db.First(&user, authRec.UserID).Error)
	assert.Equal(t, "admin", user.Role)
	assert.Equal(t, localAdminEmail, user.Email)
	assert.True(t, user.EmailVerified)
	assert.Nil(t, user.DeletedAt)
}

func TestLocalAdminSeed_WrongPasswordRejected(t *testing.T) {
	h, _ := newAuthEnvelopeHandler(t)
	seedLocalAdminAsDeployDoes(t, h)

	_, err := h.authService.LoginWithEmail(localAdminEmail, "wrong-password")
	require.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestLocalAdminSeed_RedactedHashCannotLogin(t *testing.T) {
	// Proves why seed must run AFTER sanitize-clone.sql.
	h, _ := newAuthEnvelopeHandler(t)

	user := database.User{
		Email:         localAdminEmail,
		Name:          "Local Admin",
		Role:          "admin",
		AuthMethod:    "email",
		EmailVerified: true,
	}
	require.NoError(t, h.db.Create(&user).Error)
	require.NoError(t, h.db.Create(&UserAuth{
		UserID:         user.ID,
		Provider:       "email",
		ProviderUserID: localAdminEmail,
		PasswordHash:   "[redacted]", // post-sanitize value from sanitize-clone.sql
		EmailVerified:  true,
	}).Error)

	_, err := h.authService.LoginWithEmail(localAdminEmail, localAdminPassword)
	require.ErrorIs(t, err, ErrInvalidCredentials)
}

func TestHashPassword_CompatibleWithCmdHashPassword(t *testing.T) {
	// cmd/hash-password is a thin wrapper around HashPassword; this locks the
	// cost/format contract the seed script depends on.
	hash, err := HashPassword(localAdminPassword)
	require.NoError(t, err)
	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(localAdminPassword))
	require.NoError(t, err)
}
