package services

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestCategoryOffer_MatchesByIdAndLegacyName is the §3.7 fix-6 backend BE-first
// compat guard: a category-scoped offer must match whether its TargetID is the
// category's stable ID (new frontend) OR the category name (legacy offers written
// before the frontend switched to id-based targeting). Both must keep working so
// switching the frontend to id targeting doesn't silently orphan existing offers.
func TestCategoryOffer_MatchesByIdAndLegacyName(t *testing.T) {
	categories := []database.MenuCategory{
		{
			ID:   "cat-main",
			Name: "Mains",
			Items: []database.MenuItem{
				{ID: "item-burger", Name: "Burger", Price: 10, IsAvailable: true},
			},
		},
	}

	lines := []PromotionInputLine{
		{Name: "Burger", MenuItemID: "item-burger", Quantity: 1, UnitPrice: 10, ItemType: OrderItemTypeMenuItem},
	}

	// New id-based targeting.
	byID := []database.Offer{
		{ID: 1, Name: "Mains 10%", DiscountType: "percentage", DiscountValue: 10, ApplicableTo: "category", TargetID: ptrString("cat-main"), IsActive: true},
	}
	resID, err := ApplyPromotionsToOrder(categories, nil, byID, lines)
	if err != nil {
		t.Fatalf("id-target: unexpected error: %v", err)
	}
	if !approxEqual(resID.DiscountTotal, 1.0) {
		t.Fatalf("id-target: expected 1.00 discount, got %.2f", resID.DiscountTotal)
	}

	// Legacy name-based targeting (category has an ID but the offer stored the name).
	byName := []database.Offer{
		{ID: 2, Name: "Mains 10%", DiscountType: "percentage", DiscountValue: 10, ApplicableTo: "category", TargetID: ptrString("Mains"), IsActive: true},
	}
	resName, err := ApplyPromotionsToOrder(categories, nil, byName, lines)
	if err != nil {
		t.Fatalf("name-target: unexpected error: %v", err)
	}
	if !approxEqual(resName.DiscountTotal, 1.0) {
		t.Fatalf("name-target: legacy name-targeted category offer must still match; got %.2f discount", resName.DiscountTotal)
	}
}
