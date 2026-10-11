package trustpilot

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetFeaturesOnlyAdvertisesImplementedCapabilities(t *testing.T) {
	plugin := &TrustpilotPlugin{}

	var features []string
	require.NoError(t, json.Unmarshal([]byte(plugin.GetFeatures()), &features))

	require.ElementsMatch(t, []string{
		"Review Link Generator",
		"Business Profile Link",
	}, features)

	// Unimplemented capabilities must not be advertised.
	for _, banned := range []string{"Review Display", "Review Widgets", "Customer Invitations", "Review Collection"} {
		require.NotContains(t, features, banned)
	}
}
