package server

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestIsReservedPublicSlug(t *testing.T) {
	for _, s := range []string{"admin", "api", "demo", "b", "internal", "demo-lounge", "admin-1", "b-x", "API", "Demo"} {
		require.True(t, database.IsReservedPublicSlug(s), s)
	}
	for _, s := range []string{"payverge-core-demo-kitchen", "my-restaurant", "aurora", "demoed", "cab", ""} {
		require.False(t, database.IsReservedPublicSlug(s), s)
	}
}

func TestValidateCustomURL_RejectsReserved(t *testing.T) {
	err := validateCustomURL("demo-admin-1-ai-pro", 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "reserved")

	// Format-valid, non-reserved, no DB hit for empty DB is fine — availability
	// check may still need DB. Skip taken check by only asserting reserved path.
	err = validateCustomURL("a", 0)
	require.Error(t, err) // min length
}
