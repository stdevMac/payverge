package server

import (
	"strings"

	"github.com/stdevmac/payverge/backend/internal/services"
)

// Issue 942: guests state an allergy by NAMING the allergen — "soy alérgico al
// maní", "tengo intolerancia a la lactosa", "I have a peanut allergy" — but
// services.DetectDietaryTags only recognizes the avoidance phrasing ("sin
// maní", "lactose free"). With no dietary tag the deterministic finalizer had
// nothing to filter on, so the guest got either the refuse-all allergen hedge
// or a plain catalog list with no safety notice at all.
//
// This maps a named allergen onto the canonical dietary tag the snapshot can
// already filter on, so the same reviewed machinery (RecommendableEntities'
// allergen-exclusion fallback, the avoid-dish walk-through, the staff
// disclaimer) answers a stated allergy.
//
// Deliberately conservative in two ways:
//   - An avoidance marker is REQUIRED, so "¿tienen algo con maní?" is not read
//     as an allergy.
//   - Only allergens with a dietary tag are mapped. Sesame, egg, fish and the
//     rest have no snapshot-level "free of" signal, so those keep the refusal
//     rather than getting a guess dressed up as a safe answer.
var waiterAllergenAvoidanceTagOrder = []string{"gluten-free", "dairy-free", "nut-free"}

// waiterAvoidanceMarkers are allergy / intolerance / avoidance stems. They are
// matched as plain substrings (not whole tokens) so inflections are covered
// ("alergia", "alérgico", "allergic", "intolerante"), and the list is applied
// regardless of locale: these stems do not collide across languages, and this
// is only ever consulted on a message that already reads as an allergen ask.
var waiterAvoidanceMarkers = []string{
	"allerg", "alerg", "alérg", "alerj",
	"intoleran", "intoler", "unverträg", "unvertrag", "nietoleran", "непереносим",
	"avoid", "evitar", "éviter", "eviter", "vermeiden", "undgå", "unngå", "undvika", "unikać", "избегать",
	"can't have", "cant have", "cannot have", "can't eat", "cant eat", "no puedo comer", "no puedo tomar",
	"حساسية", "एलर्जी", "アレル", "알레르", "แพ้", "dị ứng", "过敏", "不耐",
}

// waiterAllergenAvoidanceNouns supplements services.DietaryTagAvoidedAllergens
// with the words guests actually type. That table is written for matching a
// dish's stored allergen chips, so it misses plurals and the specific tree nuts
// ("nueces" is there, "nuez" is not). Nouns match as whole tokens so "nut"
// cannot fire inside "minuto".
var waiterAllergenAvoidanceNouns = map[string][]string{
	"dairy-free": {"lactosa", "lacteos", "lácteos", "queso", "cheese"},
	"nut-free": {
		"nuez", "nueces", "nuts", "frutos secos", "frutas secas", "manies", "manís",
		"cacahuetes", "cacahuates", "almendra", "almendras", "avellana", "avellanas",
		"pistacho", "pistachos", "anacardo", "anacardos", "castaña", "castañas",
		"peanuts", "almond", "almonds", "cashew", "cashews", "hazelnut", "hazelnuts",
		"pistachio", "pistachios", "walnut", "walnuts", "tree nuts",
	},
}

// waiterMessageStatesAvoidance reports whether the guest framed the message as
// an allergy, intolerance or avoidance rather than a request FOR the ingredient.
func waiterMessageStatesAvoidance(userMessage string) bool {
	lower := strings.ToLower(userMessage)
	if strings.TrimSpace(lower) == "" {
		return false
	}
	for _, marker := range waiterAvoidanceMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// waiterAllergenAvoidanceTags returns the canonical dietary tags implied by the
// allergens a guest named. Empty when the message states no avoidance, or names
// only allergens the snapshot cannot filter on.
func waiterAllergenAvoidanceTags(userMessage string) []string {
	if !waiterMessageStatesAvoidance(userMessage) {
		return nil
	}
	tags := make([]string, 0, len(waiterAllergenAvoidanceTagOrder))
	for _, tag := range waiterAllergenAvoidanceTagOrder {
		nouns := append(append([]string(nil), services.DietaryTagAvoidedAllergens(tag)...), waiterAllergenAvoidanceNouns[tag]...)
		if containsAnyWaiterPhrase(userMessage, nouns) {
			tags = append(tags, tag)
		}
	}
	if len(tags) == 0 {
		return nil
	}
	return tags
}

// waiterDietaryTags is the dietary constraint set a finalizer filters on: the
// tags the guest asked for by name ("sin gluten", "vegetariano") plus the tags
// implied by an allergen they stated ("soy alérgico al maní").
func waiterDietaryTags(locale, userMessage string) []string {
	tags := services.DetectDietaryTags(locale, userMessage)
	seen := make(map[string]struct{}, len(tags)+len(waiterAllergenAvoidanceTagOrder))
	for _, tag := range tags {
		seen[tag] = struct{}{}
	}
	for _, tag := range waiterAllergenAvoidanceTags(userMessage) {
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	return tags
}
