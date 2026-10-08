package crm

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestComputePointsEarned(t *testing.T) {
	program := &database.LoyaltyProgram{Enabled: true, PointsPerDollar: 1}
	require.Equal(t, 5, computePointsEarned(500, program))

	premium := &database.LoyaltyProgram{Enabled: true, PointsPerDollar: 1.5}
	require.Equal(t, 75, computePointsEarned(5000, premium))

	disabled := &database.LoyaltyProgram{Enabled: false, PointsPerDollar: 1}
	require.Equal(t, 0, computePointsEarned(500, disabled))

	require.Equal(t, 0, computePointsEarned(500, nil))
}

func TestComputeTier(t *testing.T) {
	program := &database.LoyaltyProgram{
		Enabled: true,
		Tiers: []database.LoyaltyTier{
			{Name: "Bronze", MinLifetimeSpentCents: 0, SortOrder: 0},
			{Name: "Silver", MinLifetimeSpentCents: 20000, SortOrder: 1},
			{Name: "Gold", MinLifetimeSpentCents: 45000, SortOrder: 2},
		},
	}
	require.Equal(t, "Bronze", computeTier(0, program))
	require.Equal(t, "Bronze", computeTier(19999, program))
	require.Equal(t, "Silver", computeTier(20000, program))
	require.Equal(t, "Silver", computeTier(30000, program))
	require.Equal(t, "Gold", computeTier(45000, program))
	require.Equal(t, "Gold", computeTier(999999, program))
	require.Equal(t, "", computeTier(100, &database.LoyaltyProgram{Tiers: nil}))
	require.Equal(t, "", computeTier(100, nil))
}

func TestValidateTierLadderRejectsOutOfRangeThresholds(t *testing.T) {
	const want = "loyalty tier spend thresholds must be between 0 and 1,000,000,000.00"
	for _, cents := range []int64{-1, 100_000_000_001} {
		err := validateTierLadder([]database.LoyaltyTier{{
			Name:                  "Bronze",
			MinLifetimeSpentCents: cents,
			SortOrder:             0,
		}})
		require.EqualError(t, err, want)
	}
	for _, cents := range []int64{0, 100_000_000_000} {
		err := validateTierLadder([]database.LoyaltyTier{{
			Name:                  "Bronze",
			MinLifetimeSpentCents: cents,
			SortOrder:             0,
		}})
		require.NoError(t, err)
	}
}

func TestValidateTierLadderUsesCanonicalUnicodeNameNormalization(t *testing.T) {
	tests := []struct {
		name  string
		left  string
		right string
		want  error
	}{
		{
			name:  "trims Unicode whitespace handled by Go strings.TrimSpace",
			left:  "\u0085Gold\u0085",
			right: "Gold",
			want:  errors.New("loyalty tiers must have unique names"),
		},
		{
			name:  "uses simple lowercase for dotted I",
			left:  "İ",
			right: "i",
			want:  errors.New("loyalty tiers must have unique names"),
		},
		{
			name:  "uses simple lowercase without Greek final sigma context",
			left:  "ΟΣ",
			right: "οσ",
			want:  errors.New("loyalty tiers must have unique names"),
		},
		{
			name:  "does not trim FEFF because Go strings.TrimSpace preserves it",
			left:  "\ufeffGold",
			right: "Gold",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tiers := []database.LoyaltyTier{
				{Name: tt.left, MinLifetimeSpentCents: 0, SortOrder: 0},
				{Name: tt.right, MinLifetimeSpentCents: 100, SortOrder: 1},
			}
			err := validateTierLadder(tiers)
			if tt.want == nil {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tt.want.Error())
		})
	}
}
