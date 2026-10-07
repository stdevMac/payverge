package server

import (
	"encoding/json"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func strptr(s string) *string { return &s }

func TestFilterOffersByHiddenItems(t *testing.T) {
	offers := []database.Offer{
		{Name: "Weekday Lunch 15% Off", ApplicableTo: "all"},
		{Name: "$5 Off the Steak Plate", ApplicableTo: "item", TargetID: strptr("demo-steak")},
		{Name: "Bowl Combo", ApplicableTo: "item", TargetID: strptr("demo-bowl")},
		{Name: "Category Deal", ApplicableTo: "category", TargetID: strptr("mains")},
	}
	hidden := map[string]bool{"demo-steak": true}

	out := filterOffersByHiddenItems(offers, hidden)

	if len(out) != 3 {
		t.Fatalf("expected 3 offers (steak-targeted dropped), got %d: %+v", len(out), out)
	}
	for _, o := range out {
		if o.ApplicableTo == "item" && o.TargetID != nil && hidden[*o.TargetID] {
			t.Errorf("offer targeting hidden item leaked: %q", o.Name)
		}
	}
}

func TestFilterBundlesByHiddenItems(t *testing.T) {
	dateNight, _ := json.Marshal([]database.BundleItemRef{
		{MenuItemID: "demo-steak", Name: "Steak Plate", Quantity: 1},
		{MenuItemID: "demo-cocktail", Name: "Demo Spritz", Quantity: 2},
	})
	drinksOnly, _ := json.Marshal([]database.BundleItemRef{
		{MenuItemID: "demo-cocktail", Name: "Demo Spritz", Quantity: 2},
	})
	bundles := []database.Bundle{
		{Name: "Date Night for Two", Items: string(dateNight)},
		{Name: "Spritz Pair", Items: string(drinksOnly)},
		{Name: "Malformed", Items: "not-json"},
	}
	hidden := map[string]bool{"demo-steak": true}

	out := filterBundlesByHiddenItems(bundles, hidden)

	// Date Night (contains steak) dropped; Spritz Pair kept; malformed kept.
	if len(out) != 2 {
		t.Fatalf("expected 2 bundles, got %d: %+v", len(out), out)
	}
	for _, b := range out {
		if b.Name == "Date Night for Two" {
			t.Errorf("bundle containing hidden item leaked: %q", b.Name)
		}
	}
}
