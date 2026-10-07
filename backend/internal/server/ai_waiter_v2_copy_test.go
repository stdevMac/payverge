package server

import (
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/locales"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #577/#583: every guest locale must have native deterministic finalizer copy.
// A missing entry silently answers the guest in English (or degrades the whole
// menu answer), which is exactly how French and the other 18 locales lost
// recommendations, pairings and item descriptions.
func TestWaiterFinalizerCopyCoversEveryGuestLocale(t *testing.T) {
	for _, locale := range locales.GuestLocales() {
		t.Run(locale.Canonical, func(t *testing.T) {
			entry, ok := waiterFinalizerCopyByLocale[locale.Canonical]
			require.True(t, ok, "no waiter finalizer copy for guest locale %s", locale.Canonical)
			for field, value := range map[string]string{
				"recommendIntro":    entry.recommendIntro,
				"fullMenuIntro":     entry.fullMenuIntro,
				"available":         entry.available,
				"unavailable":       entry.unavailable,
				"helpAdd":           entry.helpAdd,
				"addLabel":          entry.addLabel,
				"safeFallback":      entry.safeFallback,
				"socialReply":       entry.socialReply,
				"pairingIntro":      entry.pairingIntro,
				"alternativesIntro": entry.alternativesIntro,
				"pickIntro":         entry.pickIntro,
				"pickPairing":       entry.pickPairing,
				"avoidAllergen":     entry.avoidAllergen,
				"billTip":           entry.billTip,
				"billSplit":         entry.billSplit,
			} {
				assert.NotEmpty(t, strings.TrimSpace(value), "%s.%s is empty", locale.Canonical, field)
			}
			assert.Contains(t, entry.pairingIntro, waiterItemPlaceholder, "pairingIntro must carry the item placeholder")
			assert.Contains(t, entry.alternativesIntro, waiterItemPlaceholder, "alternativesIntro must carry the item placeholder")
			assert.Contains(t, entry.pickIntro, waiterItemPlaceholder, "pickIntro must carry the item placeholder")
			assert.Contains(t, entry.pickPairing, waiterItemPlaceholder, "pickPairing must carry the item placeholder")
			assert.Contains(t, entry.avoidAllergen, waiterItemPlaceholder, "avoidAllergen must carry the item placeholder")
			assert.Contains(t, entry.avoidAllergen, waiterListPlaceholder, "avoidAllergen must carry the allergen-list placeholder")
			assert.Contains(t, entry.billTip, waiterPercentPlaceholder, "billTip must carry the percent placeholder")
			assert.Contains(t, entry.billTip, waiterAmountPlaceholder, "billTip must carry the amount placeholder")
			assert.Contains(t, entry.billTip, waiterTotalPlaceholder, "billTip must carry the total placeholder")
			assert.Contains(t, entry.billSplit, waiterCountPlaceholder, "billSplit must carry the guest-count placeholder")
			assert.Contains(t, entry.billSplit, waiterAmountPlaceholder, "billSplit must carry the per-guest amount placeholder")
		})
	}
}

func TestWaiterFinalizerCopyFallsBackDeterministically(t *testing.T) {
	assert.Equal(t, waiterFinalizerCopyByLocale["en"], waiterFinalizerCopy("xx-YY"))
	assert.Equal(t, waiterFinalizerCopyByLocale["es-AR"], waiterFinalizerCopy("es-AR"))
	assert.Equal(t, waiterFinalizerCopyByLocale["fr"], waiterFinalizerCopy("fr"))
}

// Menu names are operator-controlled and can contain % or brace characters;
// substitution must be literal, never format-string interpretation.
func TestWaiterCopyWithItemSubstitutesLiterally(t *testing.T) {
	template := waiterFinalizerCopyByLocale["en"].alternativesIntro
	out := waiterCopyWithItem(template, "100% Beef {item} Burger")
	assert.Contains(t, out, "100% Beef")
	assert.NotContains(t, out, waiterItemPlaceholder+" is not available")
	assert.Equal(t, "no placeholder here", waiterCopyWithItem("no placeholder here", "X"))
}
