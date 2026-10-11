package services

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeGeneratedMenu_DropsHallucinatedEnumsAndBadPrices(t *testing.T) {
	menu := &GeneratedMenu{
		Currency: "$",
		Categories: []database.MenuCategory{{
			Name: "Mains",
			Items: []database.MenuItem{
				{
					Name:        "Veggie Bowl",
					Price:       12.99,
					Allergens:   []string{"gluten", "unicorn-dust", "DAIRY"},
					DietaryTags: []string{"vegan", "keto"},
				},
				{Name: "Free Lunch", Price: 0},
				{Name: "Gold Plate", Price: 250000},
				{Name: "Burger", Price: 9.5},
			},
		}},
	}

	report := sanitizeGeneratedMenu(menu)

	item := menu.Categories[0].Items[0]
	// Case-folded DAIRY rewrites to dairy; only the hallucinated ID drops.
	assert.Equal(t, []string{"gluten", "dairy"}, item.Allergens, "fake allergen dropped; case folded")
	assert.Equal(t, []string{"vegan"}, item.DietaryTags, "non-canonical dietary tag dropped")

	assert.Len(t, menu.Categories[0].Items, 2)
	names := []string{menu.Categories[0].Items[0].Name, menu.Categories[0].Items[1].Name}
	assert.ElementsMatch(t, []string{"Veggie Bowl", "Burger"}, names)

	assert.Equal(t, 1, report.DroppedAllergens)
	assert.Equal(t, 1, report.DroppedDietaryTags)
	assert.Equal(t, 2, report.DroppedItems)
}

func TestCanonicalEnumSets_AreExact(t *testing.T) {
	assert.Len(t, canonicalAllergenIDs, 14)
	assert.Len(t, canonicalDietaryTagIDs, 7)
	assert.Contains(t, canonicalAllergenIDs, "treenuts")
	assert.Contains(t, canonicalDietaryTagIDs, "low-sodium")
}
