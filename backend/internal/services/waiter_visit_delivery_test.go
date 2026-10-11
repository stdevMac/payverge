package services

import (
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/locales"

	"github.com/stretchr/testify/require"
)

// #903: WaiterDeliveryAnswer used to hand the guest WaiterVisitFacts.DeliveryHint
// verbatim — the model's English system-prompt fragment. That shipped operator
// instructions ("Please encourage them to use the 'Order Delivery' button"),
// an unlabelled bare fee number, and English text in all 21 guest locales; and
// for a marketplace-only venue it produced "I don't have delivery details here"
// on a venue whose Delivery settings already list PedidosYa and Rappi.
//
// The answer now comes from the venue's real delivery configuration, in the
// guest's language, with money pre-formatted by the caller.

// waiterDeliveryGuestLocales is the 21-locale guest tier, mirrored from the
// other waiter copy suites so a new locale fails here too.
var waiterDeliveryGuestLocales = []string{
	"ar", "da", "de", "en", "es", "es-AR", "fr", "hi", "it", "ja", "ko",
	"nl", "no", "pl", "pt", "ru", "sv", "th", "tr", "vi", "zh",
}

// waiterDeliveryPromptProse are phrases that only ever belonged in the model's
// system prompt. None of them may reach a guest.
var waiterDeliveryPromptProse = []string{
	"Please encourage them",
	"Direct guests to",
	"Delivery is ENABLED",
	"delivery partners.",
	"on the page",
}

func TestWaiterDeliveryAnswerUnknownDefersToStaff(t *testing.T) {
	// Settings unreadable: the assistant must not claim the venue does or does
	// not deliver.
	require.Equal(t, deliveryUnknownCopy["en"], WaiterDeliveryAnswer("en", WaiterVisitFacts{}))
	require.Equal(t, deliveryUnknownCopy["es-AR"], WaiterDeliveryAnswer("es-AR", WaiterVisitFacts{}))
}

func TestWaiterDeliveryAnswerOffIsExplicitAndLocalized(t *testing.T) {
	facts := WaiterVisitFacts{DeliveryKnown: true}
	require.Equal(t, deliveryOffCopy["en"], WaiterDeliveryAnswer("en", facts))
	require.Equal(t, deliveryOffCopy["ja"], WaiterDeliveryAnswer("ja", facts))
	// A venue we know does not deliver gets a straight answer, not a punt.
	require.NotEqual(t, deliveryUnknownCopy["en"], WaiterDeliveryAnswer("en", facts))
}

func TestWaiterDeliveryAnswerNamesConfiguredPartners(t *testing.T) {
	// The marketplace-only shape behind #903 / #871.
	facts := WaiterVisitFacts{
		DeliveryKnown:    true,
		DeliveryEnabled:  true,
		DeliveryPartners: []string{"PedidosYa", "Rappi"},
	}
	seen := map[string]bool{}
	for _, locale := range []string{"en", "es", "es-AR", "pt"} {
		got := WaiterDeliveryAnswer(locale, facts)
		require.Contains(t, got, "PedidosYa", locale)
		require.Contains(t, got, "Rappi", locale)
		require.NotEqual(t, deliveryUnknownCopy["en"], got, locale)
		require.NotContains(t, got, "{partners}", locale)
		seen[got] = true
	}
	// es / es-AR / pt must not all be the English sentence.
	require.Greater(t, len(seen), 1, "delivery partner answer is not localized")
}

func TestWaiterDeliveryAnswerInHouseUsesCallerFormattedMoney(t *testing.T) {
	facts := WaiterVisitFacts{
		DeliveryKnown:    true,
		DeliveryEnabled:  true,
		DeliveryInHouse:  true,
		DeliveryFeeLabel: "ARS 2900.00",
	}
	got := WaiterDeliveryAnswer("es-AR", facts)
	require.Contains(t, got, "ARS 2900.00")
	require.NotContains(t, got, "{fee}")
	require.NotEqual(t, deliveryUnknownCopy["es-AR"], got)
}

func TestWaiterDeliveryAnswerInHouseAndPartnersMentionsBoth(t *testing.T) {
	// Showroom venue 142: in-house delivery AND PedidosYa + Rappi.
	facts := WaiterVisitFacts{
		DeliveryKnown:    true,
		DeliveryEnabled:  true,
		DeliveryInHouse:  true,
		DeliveryFeeLabel: "ARS 2900.00",
		DeliveryPartners: []string{"PedidosYa", "Rappi"},
	}
	got := WaiterDeliveryAnswer("es", facts)
	require.Contains(t, got, "ARS 2900.00")
	require.Contains(t, got, "PedidosYa")
	require.Contains(t, got, "Rappi")
	require.NotContains(t, got, "{fee}")
	require.NotContains(t, got, "{partners}")
}

func TestWaiterDeliveryAnswerEnabledButUnconfiguredDefersToStaff(t *testing.T) {
	// Delivery toggled on with no in-house fee and no partner: promising a
	// channel the guest cannot find would be worse than deferring to staff.
	facts := WaiterVisitFacts{DeliveryKnown: true, DeliveryEnabled: true}
	require.Equal(t, deliveryUnknownCopy["en"], WaiterDeliveryAnswer("en", facts))
}

func TestWaiterDeliveryAnswerNeverEmitsOperatorPromptProse(t *testing.T) {
	shapes := []WaiterVisitFacts{
		{},
		{DeliveryKnown: true},
		{DeliveryKnown: true, DeliveryEnabled: true},
		{DeliveryKnown: true, DeliveryEnabled: true, DeliveryInHouse: true, DeliveryFeeLabel: "$5.00"},
		{DeliveryKnown: true, DeliveryEnabled: true, DeliveryPartners: []string{"PedidosYa", "Rappi"}},
		{DeliveryKnown: true, DeliveryEnabled: true, DeliveryInHouse: true, DeliveryFeeLabel: "$5.00",
			DeliveryPartners: []string{"Uber Eats"}},
	}
	for _, locale := range waiterDeliveryGuestLocales {
		for i, facts := range shapes {
			got := WaiterDeliveryAnswer(locale, facts)
			require.NotEmpty(t, got, "locale=%s shape=%d", locale, i)
			for _, prose := range waiterDeliveryPromptProse {
				require.NotContains(t, got, prose, "locale=%s shape=%d leaked prompt prose", locale, i)
			}
		}
	}
}

func TestWaiterDeliveryAnswerDropsBlankPartnerNames(t *testing.T) {
	facts := WaiterVisitFacts{
		DeliveryKnown:    true,
		DeliveryEnabled:  true,
		DeliveryPartners: []string{"  ", "Rappi", ""},
	}
	got := WaiterDeliveryAnswer("en", facts)
	require.Contains(t, got, "Rappi")
	require.NotContains(t, got, ", ,")
	require.NotContains(t, got, " ,")
}

func TestWaiterDeliveryCopyCoversEveryGuestLocale(t *testing.T) {
	maps := map[string]map[string]string{
		"deliveryOffCopy":      deliveryOffCopy,
		"deliveryInHouseCopy":  deliveryInHouseCopy,
		"deliveryPartnersCopy": deliveryPartnersCopy,
		"deliveryBothCopy":     deliveryBothCopy,
	}
	for name, m := range maps {
		for _, code := range waiterDeliveryGuestLocales {
			value, ok := m[code]
			require.Truef(t, ok, "%s is missing locale %q", name, code)
			require.NotEmptyf(t, strings.TrimSpace(value), "%s[%q] is empty", name, code)
		}
		require.Lenf(t, m, len(waiterDeliveryGuestLocales), "%s has locales outside the guest tier", name)
	}
	for _, code := range waiterDeliveryGuestLocales {
		require.Containsf(t, deliveryInHouseCopy[code], "{fee}", "deliveryInHouseCopy[%q] lost its {fee} slot", code)
		require.Containsf(t, deliveryPartnersCopy[code], "{partners}", "deliveryPartnersCopy[%q] lost its {partners} slot", code)
		require.Containsf(t, deliveryBothCopy[code], "{fee}", "deliveryBothCopy[%q] lost its {fee} slot", code)
		require.Containsf(t, deliveryBothCopy[code], "{partners}", "deliveryBothCopy[%q] lost its {partners} slot", code)
	}
	// The registry is the source of truth for who the guest tier is.
	for _, locale := range locales.GuestLocales() {
		require.Containsf(t, waiterDeliveryGuestLocales, locale.Canonical,
			"guest locale %q is in the registry but not in the delivery copy suite", locale.Canonical)
	}
}

// #903-A: neutral `es` is tuteo and `es-AR` is the voseo override layer. The
// delivery copy shipped the Rioplatense "podés" in BOTH, so every non-Argentine
// Spanish guest was addressed in a regional register — the split the rest of
// this file gets right (consulta / consultá).
func TestWaiterDeliveryNeutralSpanishIsTuteoAndArgentineKeepsVoseo(t *testing.T) {
	maps := map[string]map[string]string{
		"deliveryOffCopy":      deliveryOffCopy,
		"deliveryInHouseCopy":  deliveryInHouseCopy,
		"deliveryPartnersCopy": deliveryPartnersCopy,
		"deliveryBothCopy":     deliveryBothCopy,
	}
	for name, m := range maps {
		for _, voseo := range []string{"podés", "tenés", "querés", "hacés", "consultá", "sos "} {
			require.NotContainsf(t, m["es"], voseo,
				"%s[\"es\"] must be neutral tuteo, not Rioplatense %q", name, voseo)
		}
	}
	require.Contains(t, deliveryBothCopy["es"], "puedes pedir",
		"neutral es addresses the guest as tú")
	require.Contains(t, deliveryBothCopy["es-AR"], "podés pedir",
		"the es-AR override layer keeps voseo")
}
