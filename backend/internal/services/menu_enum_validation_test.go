package services

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeMenuCategories_DropsBadPricesAndAllergens(t *testing.T) {
	cats := []database.MenuCategory{{
		Name: "Mains",
		Items: []database.MenuItem{
			{Name: "ok", Price: 12.5, Allergens: []string{"gluten", "bogus"}, DietaryTags: []string{"vegan", "nope"}},
			{Name: "zero", Price: 0},
			{Name: "negative", Price: -3},
			{Name: "huge", Price: 10000},
		},
	}}
	out, rep := SanitizeMenuCategories(cats)
	require.Len(t, out[0].Items, 1)
	assert.Equal(t, "ok", out[0].Items[0].Name)
	assert.Equal(t, []string{"gluten"}, out[0].Items[0].Allergens)
	assert.Equal(t, []string{"vegan"}, out[0].Items[0].DietaryTags)
	assert.Equal(t, 3, rep.DroppedItems)
	assert.Equal(t, 1, rep.DroppedAllergens)
	assert.Equal(t, 1, rep.DroppedDietaryTags)
}

// Near-canonical extract/LLM IDs must rewrite to the frontend ALLERGENS set
// rather than silently disappearing (audit: peanuts/tree_nuts/sulfites).
func TestSanitizeMenuCategories_NormalizesNearCanonicalAllergens(t *testing.T) {
	cats := []database.MenuCategory{{
		Name: "Mains",
		Items: []database.MenuItem{{
			Name:  "Pad Thai",
			Price: 14.5,
			Allergens: []string{
				"peanuts", // → peanut
				"unicorn_dust",
				"gluten",
				"tree_nuts", // → treenuts
				"sulfites",  // → so2
				"peanut",    // already canonical; dedupe with peanuts
				"DAIRY",     // case fold
				"Soy",       // → soya
			},
			DietaryTags: []string{"gluten_free", "VEGAN", "keto"},
		}},
	}}
	out, rep := SanitizeMenuCategories(cats)
	require.Len(t, out[0].Items, 1)
	item := out[0].Items[0]
	assert.Equal(t, []string{"peanut", "gluten", "treenuts", "so2", "dairy", "soya"}, item.Allergens)
	assert.Equal(t, []string{"gluten-free", "vegan"}, item.DietaryTags)
	assert.Equal(t, 1, rep.DroppedAllergens, "only unicorn_dust is a true drop")
	assert.Equal(t, 1, rep.DroppedDietaryTags, "only keto is a true drop")
	assert.Equal(t, 0, rep.DroppedItems)
	require.NotEmpty(t, rep.Retained)
	assert.Contains(t, rep.Retained, MenuSanitizeDecision{
		CategoryIndex:  0,
		CategoryName:   "Mains",
		ItemIndex:      0,
		ItemName:       "Pad Thai",
		Field:          "allergens",
		Value:          "peanuts",
		CanonicalValue: "peanut",
		Reason:         "alias_normalized",
	})
	require.NotEmpty(t, rep.Dropped)
	assert.Contains(t, rep.Dropped, MenuSanitizeDecision{
		CategoryIndex: 0,
		CategoryName:  "Mains",
		ItemIndex:     0,
		ItemName:      "Pad Thai",
		Field:         "allergens",
		Value:         "unicorn_dust",
		Reason:        "unknown_enum_value",
	})
}

func TestDemoSeedMenuEnumsAreCanonical(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/demo_seed.sql")
	require.NoError(t, err)

	menuBlock := regexp.MustCompile(`(?s)CREATE TEMP TABLE demo_seed_menu_items .*?SELECT jsonb_agg`).Find(raw)
	require.NotEmpty(t, menuBlock, "demo menu fixture block must remain discoverable")
	pairs := regexp.MustCompile(`(?:true|false),'(\[[^']*\])'(?:::jsonb)?,'(\[[^']*\])'(?:::jsonb)?`).FindAllSubmatch(menuBlock, -1)
	require.NotEmpty(t, pairs, "demo fixture must expose allergen/tag arrays")

	for _, pair := range pairs {
		var allergens, tags []string
		require.NoError(t, json.Unmarshal(pair[1], &allergens))
		require.NoError(t, json.Unmarshal(pair[2], &tags))
		for _, value := range allergens {
			canonical, ok := resolveAllergenID(value)
			assert.True(t, ok, "demo allergen %q is unknown", value)
			assert.Equal(t, value, canonical, "demo allergen %q is an alias, not canonical", value)
		}
		for _, value := range tags {
			canonical, ok := resolveDietaryTagID(value)
			assert.True(t, ok, "demo dietary tag %q is unknown", value)
			assert.Equal(t, value, canonical, "demo dietary tag %q is an alias, not canonical", value)
		}
	}
}

func TestResolveAllergenID_Aliases(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"peanut", "peanut", true},
		{"peanuts", "peanut", true},
		{" Peanuts ", "peanut", true},
		{"tree_nuts", "treenuts", true},
		{"tree-nuts", "treenuts", true},
		{"Tree Nuts", "treenuts", true},
		{"sulfites", "so2", true},
		{"sulphites", "so2", true},
		{"so2", "so2", true},
		{"unicorn_dust", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := resolveAllergenID(c.in)
		assert.Equal(t, c.ok, ok, "in=%q", c.in)
		assert.Equal(t, c.want, got, "in=%q", c.in)
	}
}
