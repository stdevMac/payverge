package server

import (
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestTranslatedMenuResponseOmitsDuplicateArray asserts that the translated-branch
// guest menu response:
//   - carries the translated categories under the canonical "categories" key, and
//   - does NOT emit the redundant "parsed_categories" key that previously doubled
//     the largest guest payload for non-default-language traffic.
//
// The frontend accesses translated categories via the pattern
//
//	menuData.parsed_categories || menuData.categories
//
// so moving the translated array to "categories" and dropping "parsed_categories"
// is backward-compatible: the fallback arm now fires and returns the same data.
func TestTranslatedMenuResponseOmitsDuplicateArray(t *testing.T) {
	resp := buildTranslatedMenuResponse(
		&database.Menu{},
		[]database.MenuCategory{{Name: "Tacos-es"}},
		nil,
		nil,
		"es",
	)

	if _, dup := resp["parsed_categories"]; dup {
		t.Fatal("translated response still ships duplicate parsed_categories")
	}

	cats, ok := resp["categories"].([]database.MenuCategory)
	if !ok {
		t.Fatalf("categories field missing or wrong type: %T", resp["categories"])
	}
	if len(cats) == 0 || cats[0].Name != "Tacos-es" {
		t.Fatalf("categories not translated: %v", cats)
	}
}

// TestTranslatedMenuResponsePreservesAllFields asserts that buildTranslatedMenuResponse
// preserves the "menu", "language", "offers", and "bundles" fields alongside
// the translated "categories", so no other field is inadvertently dropped.
func TestTranslatedMenuResponsePreservesAllFields(t *testing.T) {
	menu := &database.Menu{Version: 3}
	cats := []database.MenuCategory{{Name: "Bebidas-es"}}
	offers := []database.Offer{{Name: "2x1-es"}}
	bundles := []database.Bundle{{Name: "Combo-es"}}

	resp := buildTranslatedMenuResponse(menu, cats, offers, bundles, "es")

	menuPayload, ok := resp["menu"].(gin.H)
	if !ok || menuPayload == nil {
		t.Error("menu field missing from translated response")
	} else if _, dup := menuPayload["categories"]; dup {
		t.Error("menu.categories still duplicates the top-level categories tree")
	}
	if resp["language"] != "es" {
		t.Errorf("language = %v, want es", resp["language"])
	}
	gotOffers, ok := resp["offers"].([]publicGuestOffer)
	if !ok || len(gotOffers) != 1 || gotOffers[0].Name != "2x1-es" {
		t.Errorf("offers field wrong: %v", resp["offers"])
	}
	gotBundles, ok := resp["bundles"].([]database.Bundle)
	if !ok || len(gotBundles) != 1 || gotBundles[0].Name != "Combo-es" {
		t.Errorf("bundles field wrong: %v", resp["bundles"])
	}
}
