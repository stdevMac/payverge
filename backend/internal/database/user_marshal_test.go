package database

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUserMarshalJSON_SlimWhenPartialProjection locks decision-14 / L6-15:
// list-path actors loaded with Select("id, name, email") must not emit nested
// notification_preferences zeros or invented auth fields.
func TestUserMarshalJSON_SlimWhenPartialProjection(t *testing.T) {
	u := User{ID: 7, Name: "Owner Ada", Email: "ada@example.com"}
	// CreatedAt zero → partial projection heuristic.
	raw, err := json.Marshal(u)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	assert.Equal(t, float64(7), m["id"])
	assert.Equal(t, "Owner Ada", m["name"])
	assert.Equal(t, "ada@example.com", m["email"])
	assert.NotContains(t, m, "notification_preferences")
	assert.NotContains(t, m, "auth_method")
	assert.NotContains(t, m, "email_verified")
	assert.NotContains(t, m, "role")
}

func TestUserMarshalJSON_FullWhenCreatedAtSet(t *testing.T) {
	u := User{
		ID: 7, Name: "Owner Ada", Email: "ada@example.com",
		Role: "user", AuthMethod: "email", CreatedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	}
	raw, err := json.Marshal(u)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	assert.Equal(t, "user", m["role"])
	assert.Equal(t, "email", m["auth_method"])
	assert.Contains(t, m, "created_at")
}
