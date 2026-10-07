package server

import (
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/assistantcontract"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func waiterAllergenNoticeCount(response assistantcontract.Response) int {
	count := 0
	for _, notice := range response.Notices {
		if notice.ID == "allergen-staff-confirmation" {
			count++
		}
	}
	return count
}

// Issue 942: a guest who NAMES the allergen ("soy alérgico al maní", "tengo
// intolerancia a la lactosa") instead of using the "sin X" phrasing the
// dietary detector requires got the refuse-all hedge — no safe pick, no dairy
// walk-through — even though the carta carries allergen chips.
func TestFinalizeWaiterV2_NamedAllergenGetsSafePicksNotARefusal(t *testing.T) {
	t.Run("spanish peanut allergy", func(t *testing.T) {
		snapshot := allergenChipSnapshot(t, "es")
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-942-mani", Locale: "es", Mode: "ordering",
			UserMessage: "soy alérgico al maní, ¿qué me recomendás?", Snapshot: snapshot,
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.NotEqual(t, services.AllergenRefusal("es"), strings.TrimSpace(response.Answer.Content))
		assert.NotContains(t, response.Answer.Content, "No puedo confirmar los alérgenos")
		// A safe pick is a grounded, tappable entity — not prose.
		require.NotEmpty(t, response.Entities)
		// The safety posture survives: staff confirmation, exactly once.
		assert.Contains(t, response.Answer.Content, services.AllergenDisclaimer("es"))
		assert.Equal(t, 1, waiterAllergenNoticeCount(response))
	})

	t.Run("lactose intolerance walks through the dairy dishes", func(t *testing.T) {
		snapshot := allergenChipSnapshot(t, "es")
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-942-lactosa", Locale: "es", Mode: "ordering",
			UserMessage: "tengo intolerancia a la lactosa", Snapshot: snapshot,
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.NotEqual(t, services.AllergenRefusal("es"), strings.TrimSpace(response.Answer.Content))
		// The dairy dishes are named as skips, with their own localized chips.
		assert.Contains(t, response.Answer.Content, "Lácteos")
		lower := strings.ToLower(response.Answer.Content)
		assert.True(t,
			strings.Contains(lower, "provoleta") || strings.Contains(lower, "flan casero"),
			"expected a dairy dish to be named as a skip, got %q", response.Answer.Content)
		// A dish that lists dairy must never be the pick, nor a tap-to-add card.
		entityIDs := make([]string, 0, len(response.Entities))
		for _, entity := range response.Entities {
			entityIDs = append(entityIDs, entity.ID)
		}
		assert.NotContains(t, entityIDs, "menu_item:demo-provoleta")
		assert.NotContains(t, entityIDs, "menu_item:demo-flan")
		assert.Equal(t, 1, waiterAllergenNoticeCount(response))
	})

	t.Run("english peanut allergy", func(t *testing.T) {
		snapshot := allergenChipSnapshot(t, "en")
		response, err := FinalizeWaiterV2(WaiterFinalizeInput{
			ResponseID: "waiter-942-peanut-en", Locale: "en", Mode: "ordering",
			UserMessage: "I have a peanut allergy, what can I eat?", Snapshot: snapshot,
		})
		require.NoError(t, err)
		requireValidWaiterV2(t, response)
		assert.NotEqual(t, services.AllergenRefusal("en"), strings.TrimSpace(response.Answer.Content))
		require.NotEmpty(t, response.Entities)
		assert.Contains(t, response.Answer.Content, services.AllergenDisclaimer("en"))
	})
}

// The refusal is still the right answer for an allergen the snapshot cannot
// filter on: nothing in the menu model says which dishes are sesame-free.
func TestFinalizeWaiterV2_UnmappableAllergenKeepsTheRefusal(t *testing.T) {
	snapshot := allergenChipSnapshot(t, "es")
	response, err := FinalizeWaiterV2(WaiterFinalizeInput{
		ResponseID: "waiter-942-sesamo", Locale: "es", Mode: "ordering",
		UserMessage: "soy alérgico al sésamo", Snapshot: snapshot,
	})
	require.NoError(t, err)
	requireValidWaiterV2(t, response)
	assert.Contains(t, response.Answer.Content, services.AllergenRefusal("es"))
	assert.Empty(t, response.Entities)
	assert.Equal(t, 1, waiterAllergenNoticeCount(response))
}

func TestWaiterAllergenAvoidanceTags(t *testing.T) {
	for _, tc := range []struct {
		name    string
		message string
		want    []string
	}{
		{"spanish peanut allergy", "soy alérgico al maní", []string{"nut-free"}},
		{"spanish walnut allergy", "tengo alergia a la nuez", []string{"nut-free"}},
		{"spanish lactose intolerance", "soy intolerante a la lactosa", []string{"dairy-free"}},
		{"spanish celiac phrasing without the tag", "tengo alergia al gluten", []string{"gluten-free"}},
		{"english peanut allergy", "I have a peanut allergy", []string{"nut-free"}},
		{"english avoid dairy", "I need to avoid milk", []string{"dairy-free"}},
		{"two allergens", "soy alérgico al maní y a la lactosa", []string{"dairy-free", "nut-free"}},
		// Naming an ingredient is not stating an allergy — a guest asking FOR
		// peanut sauce must not be answered with peanut-free picks.
		{"positive ask is not an avoidance", "¿tienen algo con maní?", nil},
		{"avoidance without a named allergen", "soy alérgico, ¿qué me recomendás?", nil},
		// "nut" must not fire inside "minuto".
		{"allergen word inside another word", "tengo alergia, ¿tardan un minuto?", nil},
		// Allergens with no snapshot-level "free of" signal keep the refusal.
		{"unmappable allergen", "soy alérgico al sésamo", nil},
		{"empty", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, waiterAllergenAvoidanceTags(tc.message))
		})
	}
}

// waiterDietaryTags must not disturb the tags the guest asked for by name.
func TestWaiterDietaryTagsMergesStatedAllergensWithoutLosingAskedTags(t *testing.T) {
	assert.Equal(t, []string{"vegetarian"}, waiterDietaryTags("es", "¿qué opción vegetarian tienen?"))
	assert.Equal(t, []string{"gluten-free"}, waiterDietaryTags("es", "¿qué hay sin gluten?"))
	assert.Equal(t, []string{"gluten-free", "nut-free"},
		waiterDietaryTags("es", "algo sin gluten, y soy alérgico al maní"))
	assert.Empty(t, waiterDietaryTags("es", "¿qué me recomendás?"))
}
