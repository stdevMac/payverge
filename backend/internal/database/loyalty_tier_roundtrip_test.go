package database

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLoyaltyTier_MinLifetimeSpentCentsRoundTrip guards against float truncation
// in the dollars↔cents round-trip. MarshalJSON emits MinLifetimeSpentCents/100 as
// a dollar float; UnmarshalJSON must multiply back with rounding, since e.g.
// 10057¢ → 100.57 → 100.57*100 == 10056.9999 would truncate to 10056¢, corrupting
// the tier threshold on every save/load cycle.
func TestLoyaltyTier_MinLifetimeSpentCentsRoundTrip(t *testing.T) {
	for _, cents := range []int64{0, 57, 226, 10057, 19999, 99999} {
		tier := LoyaltyTier{Name: "Tier", MinLifetimeSpentCents: cents}
		raw, err := json.Marshal(tier)
		require.NoError(t, err)

		var back LoyaltyTier
		require.NoError(t, json.Unmarshal(raw, &back))
		require.Equal(t, cents, back.MinLifetimeSpentCents,
			"dollars↔cents round-trip must preserve the exact cent value")
	}
}
