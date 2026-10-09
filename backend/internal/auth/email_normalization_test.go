package auth

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeEmail(t *testing.T) {
	assert.Equal(t, "owner@example.com", NormalizeEmail("  Owner@Example.COM "))
	assert.Equal(t, "", NormalizeEmail("   "))
}

// Registering with mixed case stores the canonical lowercase identity, and a
// second registration differing only by case is a duplicate (409) — the same
// mailbox must never yield two accounts.
func TestRegisterNormalizesEmailCase(t *testing.T) {
	h, db, _ := newRegisterSessionHandler(t)

	w, c := postJSON(t, map[string]string{
		"email":    "Mixed@Example.COM",
		"password": "password123",
		"name":     "Mixed Case",
	})
	h.Register(c)
	require.Equal(t, http.StatusCreated, w.Code)

	var user database.User
	require.NoError(t, db.Where("email = ?", "mixed@example.com").First(&user).Error)
	var authRec UserAuth
	require.NoError(t, db.Where("provider = ? AND provider_user_id = ?", "email", "mixed@example.com").First(&authRec).Error)

	w2, c2 := postJSON(t, map[string]string{
		"email":    "MIXED@example.com",
		"password": "password123",
		"name":     "Mixed Again",
	})
	h.Register(c2)
	require.Equal(t, http.StatusConflict, w2.Code)
	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &body))
	assert.Equal(t, "VALIDATION_DUPLICATE_EMAIL", body["code"])
}

// A LEGACY mixed-case auth record (created before normalization shipped) must
// still be able to log in with any casing — lookups are case-insensitive, not
// dependent on stored normalization.
func TestLoginResolvesLegacyMixedCaseAuthRecord(t *testing.T) {
	h, db, _ := newRegisterSessionHandler(t)

	hash, err := HashPassword("password123")
	require.NoError(t, err)
	require.NoError(t, db.Create(&database.User{Email: "Legacy@Example.com", AuthMethod: "email", Role: "user", EmailVerified: true}).Error)
	require.NoError(t, db.Create(&UserAuth{
		Provider:       "email",
		ProviderUserID: "Legacy@Example.com",
		PasswordHash:   hash,
		EmailVerified:  true,
		UserID:         1,
	}).Error)

	w, c := postJSON(t, map[string]string{"email": "legacy@example.com", "password": "password123"})
	h.Login(c)
	// Wrong-credentials would be 401; resolving the legacy row yields 200.
	require.Equal(t, http.StatusOK, w.Code)
}

// The loser of a concurrent duplicate registration sees a unique violation
// from the user INSERT (both requests passed the pre-checks). sqlite's
// duplicate-insert error must classify as a unique-constraint error — that is
// the exact error the Register tx branch maps to 409 DUPLICATE_EMAIL.
func TestConcurrentDuplicateRegistrationErrorClassifiesAsUniqueViolation(t *testing.T) {
	_, db, _ := newRegisterSessionHandler(t)

	require.NoError(t, db.Create(&database.User{
		Email: "race@example.com", AuthMethod: "email", Role: "user",
	}).Error)
	err := db.Create(&database.User{
		Email: "race@example.com", AuthMethod: "email", Role: "user",
	}).Error
	require.Error(t, err)
	assert.True(t, database.IsUniqueConstraintError(err),
		"duplicate users.email insert must classify as a unique violation")
}
