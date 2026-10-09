package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #577: the guest prompts that failed on production must classify as
// recommendation / pairing / alternative intent in every affected locale.
func TestWaiterDiscoveryIntentDetectorsCoverProductionFailures(t *testing.T) {
	tests := []struct {
		name    string
		locale  string
		message string
		detect  func(string, string) bool
	}{
		{"en whats good today", "en", "What's good today?", DetectRecommendationIntent},
		{"en what do you recommend", "en", "What do you recommend?", DetectRecommendationIntent},
		{"en what should i order", "en", "What should I order?", DetectRecommendationIntent},
		{"en pairs with", "en", "What pairs with the Harvest Bowl?", DetectPairingIntent},
		{"en goes well with", "en", "What goes well with the burger?", DetectPairingIntent},
		{"en alternative", "en", "The Steak Plate is unavailable — what's a good alternative?", DetectAlternativeIntent},
		{"en instead", "en", "Can I have something else instead?", DetectAlternativeIntent},
		{"es-AR recomendas", "es-AR", "¿Qué plato vegetariano me recomendás de este menú?", DetectRecommendationIntent},
		{"es-AR que me sugeris", "es-AR", "¿Qué me sugerís?", DetectRecommendationIntent},
		{"es recomiendas", "es", "¿Qué me recomiendas?", DetectRecommendationIntent},
		{"fr conseillez", "fr", "Que me conseillez-vous avec le Harvest Bowl ?", DetectPairingIntent},
		{"fr recommandez", "fr", "Que recommandez-vous ?", DetectRecommendationIntent},
		{"de empfehlen", "de", "Was empfehlen Sie?", DetectRecommendationIntent},
		{"it consigliate", "it", "Cosa mi consigliate?", DetectRecommendationIntent},
		{"pt recomenda", "pt", "O que você recomenda?", DetectRecommendationIntent},
		{"ja osusume", "ja", "おすすめは何ですか", DetectRecommendationIntent},
		{"en sold out", "en", "What's sold out tonight? Anything 86'd I should skip?", DetectSoldOutIntent},
		{"es 86", "es", "che esta 86 el asado?", DetectSoldOutIntent},
		{"en available now", "en", "what's available right now?", DetectRecommendationIntent},
		{"en actually available", "en", "what's good that's actually available right now?", DetectRecommendationIntent},
		{"en available skip 86", "en", "what's good that's actually available right now? nothing 86'd please", DetectRecommendationIntent},
		{"en skip 86 constraint", "en", "what's good that's actually available right now? nothing 86'd please", DetectSoldOutSkipConstraint},
		{"es-AR tables", "es-AR", "qué mesas están libres?", DetectTablesIntent},
		{"en tables spanish phrasing", "en", "qué mesas están libres?", DetectTablesIntent},
		{"es service call", "es", "llama al mozo, agua", DetectServiceCallIntent},
		{"en hours", "en", "When do you close?", DetectHoursIntent},
		{"en bill", "en", "can I get the bill please?", DetectBillIntent},
		{"es english bill phrasing", "es", "can I get the bill please?", DetectBillIntent},
		{"es-AR english bill phrasing", "es-AR", "can I get the bill please?", DetectBillIntent},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.True(t, tc.detect(tc.locale, tc.message), "expected intent for %q", tc.message)
		})
	}
}

// Ordinary menu questions must NOT be swept into a discovery intent — otherwise
// "Can I order the Seasonal Soup?" would answer with alternatives instead of a
// straight availability answer.
func TestWaiterDiscoveryIntentDetectorsIgnoreOrdinaryMessages(t *testing.T) {
	for _, message := range []string{
		"Can I order the Seasonal Soup?",
		"Tell me about the Harvest Bowl",
		"I would like to know your hours",
		"Show me the full menu",
		"Is the Steak Plate available?",
	} {
		assert.False(t, DetectRecommendationIntent("en", message), "recommendation: %q", message)
		assert.False(t, DetectPairingIntent("en", message), "pairing: %q", message)
		assert.False(t, DetectAlternativeIntent("en", message), "alternative: %q", message)
	}
}

func TestDetectSoldOutIntentIgnoresSkipConstraintOnAvailabilityAsk(t *testing.T) {
	listing := "What's sold out tonight? Anything 86'd I should skip?"
	availability := "what's good that's actually available right now? nothing 86'd please"

	assert.True(t, DetectSoldOutIntent("en", listing))
	assert.False(t, DetectSoldOutSkipConstraint("en", listing))

	assert.False(t, DetectSoldOutIntent("en", availability))
	assert.True(t, DetectSoldOutSkipConstraint("en", availability))
	assert.True(t, DetectRecommendationIntent("en", availability))
	assert.True(t, DetectRecommendationIntent("en", "what's available right now?"))
	assert.True(t, DetectSoldOutIntent("es", "che esta 86 el asado?"))
}

func TestDetectDietaryTagsReturnsCanonicalIDs(t *testing.T) {
	tests := []struct {
		locale  string
		message string
		want    []string
	}{
		{"en", "Do you have anything vegetarian?", []string{"vegetarian"}},
		{"en", "I'm looking for a vegan main", []string{"vegan"}},
		{"en", "Anything gluten free?", []string{"gluten-free"}},
		{"es-AR", "¿Qué plato vegetariano me recomendás de este menú?", []string{"vegetarian"}},
		{"es", "¿Tienen algo vegano?", []string{"vegan"}},
		{"fr", "Avez-vous un plat végétarien ?", []string{"vegetarian"}},
		{"de", "Haben Sie etwas vegetarisches?", []string{"vegetarian"}},
		{"en", "What's good today?", nil},
	}
	for _, tc := range tests {
		t.Run(tc.locale+" "+tc.message, func(t *testing.T) {
			got := DetectDietaryTags(tc.locale, tc.message)
			if len(tc.want) == 0 {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, tc.want, got)
			}
			for _, tag := range got {
				canonical, ok := CanonicalDietaryTagID(tag)
				require.True(t, ok, "detector emitted non-canonical tag %q", tag)
				assert.Equal(t, tag, canonical)
			}
		})
	}
}

// "vegan" implies "vegetarian"; asking for a vegan dish must not require BOTH
// tags on the item (which would filter out every vegan-only-tagged dish).
func TestDetectDietaryTagsVeganSubsumesVegetarian(t *testing.T) {
	assert.Equal(t, []string{"vegan"}, DetectDietaryTags("en", "a vegan vegetarian plate please"))
}

func TestDietaryTagAvoidedAllergensExcludesGlutenDishes(t *testing.T) {
	assert.True(t, DietaryAskAvoidsAllergens([]string{"gluten-free"}))
	assert.False(t, DietaryAskAvoidsAllergens([]string{"vegetarian"}))
	assert.True(t, EntityContainsAvoidedAllergen([]string{"gluten", "sesame"}, []string{"gluten-free"}))
	assert.False(t, EntityContainsAvoidedAllergen([]string{"sesame"}, []string{"gluten-free"}))
	assert.False(t, EntityContainsAvoidedAllergen(nil, []string{"gluten-free"}))
}

// #583: the deterministic reply language follows the session locale, so an
// unknown locale must still fall back deterministically instead of panicking or
// silently matching another family's vocabulary.
func TestWaiterDiscoveryUnknownLocaleFallsBackToEnglish(t *testing.T) {
	assert.True(t, DetectRecommendationIntent("xx-YY", "What do you recommend?"))
	assert.False(t, DetectRecommendationIntent("xx-YY", strings.Repeat("z", 20)))
}
