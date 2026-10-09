package demo

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Guard against a hard-coded upstream image host creeping back into the seed:
// every seeded photo resolves through the configured public store, so a fork
// that configures nothing never hot-links the original project's CDN.
func TestDemoAssetURLsNeverPointAtTheUpstreamCDN(t *testing.T) {
	t.Setenv("PUBLIC_URL", "")

	var seeded []string
	for id := range menuImages {
		seeded = append(seeded, menuImage(id))
	}
	for id := range promoImages {
		seeded = append(seeded, promoImage(id))
	}
	for _, p := range profiles() {
		seeded = append(seeded, p.Hero)
	}
	for _, u := range seeded {
		require.NotEmpty(t, u)
		require.NotContains(t, u, "payverge.io")
		require.True(t, strings.HasSuffix(u, ".jpg"), u)
		require.True(t, isOwnHostedDemoAsset(u), "%s must count as own-hosted", u)
	}
	require.False(t, isOwnHostedDemoAsset("https://images.unsplash.com/photo-1.jpg"))
}

func TestDemoImageHelpersReturnEmptyForUnknownIDs(t *testing.T) {
	require.Empty(t, menuImage("no-such-dish"))
	require.Empty(t, promoImage("no-such-promo"))
}
