package auth

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A completed registration must stamp users.activated_at so activation is
// measurable (Task 22 / finding 24).
func TestRegisterWritesActivatedAt(t *testing.T) {
	h, db, _ := newRegisterSessionHandler(t)

	// AutoMigrate picks up new columns on the User model.
	require.NoError(t, db.AutoMigrate(&database.User{}))

	before := time.Now().UTC().Add(-time.Second)
	w, c := postJSON(t, map[string]string{
		"email":    "activated@example.com",
		"password": "password123",
		"name":     "Activated User",
	})
	h.Register(c)
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())

	var user database.User
	require.NoError(t, db.Where("email = ?", "activated@example.com").First(&user).Error)
	require.NotNil(t, user.ActivatedAt, "activated_at must be set on completed registration")
	assert.False(t, user.ActivatedAt.IsZero())
	assert.True(t, !user.ActivatedAt.Before(before), "activated_at must be at/after registration")
	assert.True(t, !user.ActivatedAt.After(time.Now().UTC().Add(time.Second)))
}

// Signup source is persisted when the client supplies it (attribution).
func TestRegisterPersistsSignupSource(t *testing.T) {
	h, db, _ := newRegisterSessionHandler(t)
	require.NoError(t, db.AutoMigrate(&database.User{}))

	w, c := postJSON(t, map[string]string{
		"email":         "sourced@example.com",
		"password":      "password123",
		"name":          "Sourced",
		"signup_source": "concierge",
	})
	h.Register(c)
	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())

	var user database.User
	require.NoError(t, db.Where("email = ?", "sourced@example.com").First(&user).Error)
	assert.Equal(t, "concierge", user.SignupSource)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
}
