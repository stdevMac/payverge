package services

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublicMarketingVenueName_OmitsDemoShowrooms(t *testing.T) {
	require.Equal(t, "", PublicMarketingVenueName("Payverge AI Pro Demo Lounge", false))
	require.Equal(t, "", PublicMarketingVenueName("Neighborhood Bistro", true))
	require.Equal(t, "Neighborhood Bistro", PublicMarketingVenueName("Neighborhood Bistro", false))
}

func TestStripDemoMarketingBranding_RemovesPayvergeHashtagsAndVenue(t *testing.T) {
	got := StripDemoMarketingBranding(
		"Enjoy our Date Night for Two combo: steak, spritz, and tart for two. Perfect for local restaurant guests in New York. #PayvergeAIDemoLounge #DateNight",
	)
	require.NotContains(t, got, "Payverge")
	require.NotContains(t, got, "#PayvergeAIDemoLounge")
	require.Contains(t, got, "#DateNight")
	require.Contains(t, got, "Date Night for Two")

	got = StripDemoMarketingBranding("Tonight at Payverge AI Pro Demo Lounge. #Payverge")
	require.NotContains(t, got, "Payverge")
	require.NotContains(t, got, "#Payverge")
	require.Contains(t, got, "Tonight")
}
