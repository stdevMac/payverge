package marketing

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// L4-22: why_factor values must be numeric/flag wire tokens — no English prose.
func TestWhyFactorValues_AreNumericOnly(t *testing.T) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	end := now.Add(48 * time.Hour)
	offers := []database.Offer{{
		Name:          "Lunch special",
		DiscountType:  "percentage",
		DiscountValue: 15,
		EndDate:       &end,
		Image:         "https://example.com/x.jpg",
		IsActive:      true,
	}}
	got := offerSuggestions(offers, 1, now)
	require.Len(t, got, 1)
	for _, f := range got[0].WhyFactors {
		require.NotContains(t, strings.ToLower(f.Value), "off", "factor %s value %q", f.Key, f.Value)
		require.NotContains(t, f.Value, "Ends within", "factor %s", f.Key)
		require.NotContains(t, f.Value, "Has a photo", "factor %s", f.Key)
		require.NotContains(t, f.Value, "days", "factor %s value %q", f.Key, f.Value)
	}
	// discount is "15%" not "15% off"
	var disc string
	for _, f := range got[0].WhyFactors {
		if f.Key == "discount" {
			disc = f.Value
		}
		if f.Key == "photo_ready" {
			require.Equal(t, "1", f.Value)
		}
		if f.Key == "ending_soon" {
			require.Equal(t, "7", f.Value) // offerUrgencyWindowDays
		}
	}
	require.Equal(t, "15%", disc)

	s := &CampaignSuggestion{ImageURL: "https://x", Rank: 1}
	applyImageBoost(s)
	require.True(t, hasWhyFactor(s, "photo_ready"))
	for _, f := range s.WhyFactors {
		if f.Key == "photo_ready" {
			require.Equal(t, "1", f.Value)
		}
	}
}
