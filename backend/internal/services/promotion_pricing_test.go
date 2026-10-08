package services

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func ptrUint(v uint) *uint {
	return &v
}

func ptrString(v string) *string {
	return &v
}

func approxEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.0001
}

func buildBundleJSON(t *testing.T, refs []database.BundleItemRef) string {
	t.Helper()
	payload, err := json.Marshal(refs)
	if err != nil {
		t.Fatalf("failed to marshal refs: %v", err)
	}
	return string(payload)
}

func TestApplyPromotionsToOrder_StackingAcrossScopes(t *testing.T) {
	categories := []database.MenuCategory{
		{
			ID:   "cat-main",
			Name: "Mains",
			Items: []database.MenuItem{
				{ID: "item-burger", Name: "Burger", Price: 12, IsAvailable: true},
				{ID: "item-fries", Name: "Fries", Price: 6, IsAvailable: true},
			},
		},
		{
			ID:   "cat-drinks",
			Name: "Drinks",
			Items: []database.MenuItem{
				{ID: "item-cola", Name: "Cola", Price: 4, IsAvailable: true},
			},
		},
	}

	bundles := []database.Bundle{
		{
			ID:       10,
			Name:     "Combo",
			Price:    15,
			IsActive: true,
			Items: buildBundleJSON(t, []database.BundleItemRef{
				{MenuItemID: "item-burger", Quantity: 1},
				{MenuItemID: "item-cola", Quantity: 1},
			}),
		},
	}

	offers := []database.Offer{
		{ID: 1, Name: "All 10%", DiscountType: "percentage", DiscountValue: 10, ApplicableTo: "all", IsActive: true},
		{ID: 2, Name: "Burger $2", DiscountType: "fixed", DiscountValue: 2, ApplicableTo: "item", TargetID: ptrString("item-burger"), IsActive: true},
		{ID: 3, Name: "Bundle 20%", DiscountType: "percentage", DiscountValue: 20, ApplicableTo: "bundle", TargetID: ptrString("10"), IsActive: true},
		{ID: 4, Name: "Mains $1", DiscountType: "fixed", DiscountValue: 1, ApplicableTo: "category", TargetID: ptrString("cat-main"), IsActive: true},
	}

	result, err := ApplyPromotionsToOrder(categories, bundles, offers, []PromotionInputLine{
		{Name: "Burger", MenuItemID: "item-burger", Quantity: 1, UnitPrice: 12, ItemType: OrderItemTypeMenuItem},
		{Name: "Combo", Quantity: 1, UnitPrice: 15, ItemType: OrderItemTypeBundle, BundleID: ptrUint(10)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !approxEqual(result.BaseSubtotal, 27) {
		t.Fatalf("expected base subtotal 27, got %.2f", result.BaseSubtotal)
	}
	if !approxEqual(result.DiscountTotal, 8.7) {
		t.Fatalf("expected discount total 8.70, got %.2f", result.DiscountTotal)
	}
	if len(result.AppliedOffers) != 4 {
		t.Fatalf("expected 4 applied offers, got %d", len(result.AppliedOffers))
	}

	discountLines := 0
	bundleChildren := 0
	bundleOccurrence := ""
	for _, line := range result.Lines {
		if line.ItemType == OrderItemTypeBundle {
			bundleOccurrence = line.BundleOccurrenceID
			if bundleOccurrence == "" {
				t.Fatal("bundle parent must carry an occurrence id")
			}
		}
		if line.ItemType == OrderItemTypeDiscount {
			discountLines++
		}
		if line.ItemType == OrderItemTypeBundleItem {
			bundleChildren++
			if line.BundleOccurrenceID == "" || line.BundleOccurrenceID != bundleOccurrence {
				t.Fatalf("bundle child occurrence %q must match parent %q", line.BundleOccurrenceID, bundleOccurrence)
			}
			if line.Subtotal != 0 {
				t.Fatalf("expected bundle child subtotal 0, got %.2f", line.Subtotal)
			}
		}
	}
	if discountLines != 4 {
		t.Fatalf("expected 4 discount lines, got %d", discountLines)
	}
	if bundleChildren != 2 {
		t.Fatalf("expected 2 bundle child lines, got %d", bundleChildren)
	}
}

func TestApplyPromotionsToOrder_BundleChildrenCarryCatalogPrice(t *testing.T) {
	categories := []database.MenuCategory{
		{
			ID: "cat-main", Name: "Mains",
			Items: []database.MenuItem{
				{ID: "demo-steak", Name: "Steak Plate", Price: 42, IsAvailable: true},
				{ID: "demo-dessert", Name: "Chocolate Tart", Price: 9, IsAvailable: true},
			},
		},
		{
			ID: "cat-drinks", Name: "Drinks",
			Items: []database.MenuItem{
				{ID: "demo-cocktail", Name: "Demo Spritz", Price: 12, IsAvailable: true},
			},
		},
	}
	bundles := []database.Bundle{{
		ID: 7, Name: "Date Night for Two", Price: 68, IsActive: true,
		Items: buildBundleJSON(t, []database.BundleItemRef{
			{MenuItemID: "demo-steak", Quantity: 1},
			{MenuItemID: "demo-cocktail", Quantity: 2},
			{MenuItemID: "demo-dessert", Quantity: 1},
		}),
	}}

	result, err := ApplyPromotionsToOrder(categories, bundles, nil, []PromotionInputLine{
		{Name: "Date Night for Two", Quantity: 1, UnitPrice: 68, ItemType: OrderItemTypeBundle, BundleID: ptrUint(7)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !approxEqual(result.BaseSubtotal, 68) {
		t.Fatalf("bundle parent must still bill 68, got %.2f", result.BaseSubtotal)
	}

	prices := map[string]float64{}
	for _, line := range result.Lines {
		if line.ItemType != OrderItemTypeBundleItem {
			continue
		}
		if line.Subtotal != 0 {
			t.Fatalf("bundle child %s subtotal must stay 0, got %.2f", line.MenuItemName, line.Subtotal)
		}
		prices[line.MenuItemName] = line.Price
	}
	if prices["Steak Plate"] != 42 {
		t.Fatalf("steak child catalog price, got %.2f", prices["Steak Plate"])
	}
	if prices["Demo Spritz"] != 12 {
		t.Fatalf("spritz child catalog price, got %.2f", prices["Demo Spritz"])
	}
	if prices["Chocolate Tart"] != 9 {
		t.Fatalf("tart child catalog price, got %.2f", prices["Chocolate Tart"])
	}

	billItems := PromotionLinesToBillItems(result.Lines)
	var childSubtotal float64
	for _, item := range billItems {
		if item.ItemType == OrderItemTypeBundleItem {
			if item.Price <= 0 {
				t.Fatalf("bill child %s must keep catalog price, got %.2f", item.Name, item.Price)
			}
			childSubtotal += item.Subtotal
		}
	}
	if childSubtotal != 0 {
		t.Fatalf("bundle children must not contribute to bill subtotal, got %.2f", childSubtotal)
	}
}

func TestApplyPromotionsToOrder_IgnoresBundleCurrencyField(t *testing.T) {
	categories := []database.MenuCategory{
		{
			ID:   "cat-main",
			Name: "Mains",
			Items: []database.MenuItem{
				{ID: "item-burger", Name: "Burger", Price: 12, IsAvailable: true},
			},
		},
	}
	bundles := []database.Bundle{
		{
			ID:       10,
			Name:     "Combo",
			Price:    15,
			Currency: "EUR",
			IsActive: true,
			Items: buildBundleJSON(t, []database.BundleItemRef{
				{MenuItemID: "item-burger", Quantity: 1},
			}),
		},
	}

	result, err := ApplyPromotionsToOrder(categories, bundles, nil, []PromotionInputLine{
		{Name: "Combo", Quantity: 1, UnitPrice: 15, ItemType: OrderItemTypeBundle, BundleID: ptrUint(10)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !approxEqual(result.BaseSubtotal, 15) {
		t.Fatalf("stored majors must stay 15 regardless of Currency=EUR, got %.2f", result.BaseSubtotal)
	}
	for _, line := range result.Lines {
		if line.ItemType == OrderItemTypeBundle && !approxEqual(line.Price, 15) {
			t.Fatalf("bundle parent price must stay 15, got %.2f", line.Price)
		}
	}
}

func TestApplyPromotionsToOrder_RepeatedBundleLinesGetDistinctOccurrences(t *testing.T) {
	categories := []database.MenuCategory{
		{
			ID:   "cat-main",
			Name: "Mains",
			Items: []database.MenuItem{
				{ID: "item-burger", Name: "Burger", Price: 12, IsAvailable: true},
			},
		},
		{
			ID:   "cat-drinks",
			Name: "Drinks",
			Items: []database.MenuItem{
				{ID: "item-cola", Name: "Cola", Price: 4, IsAvailable: true},
			},
		},
	}
	bundles := []database.Bundle{
		{
			ID:       10,
			Name:     "Combo",
			Price:    15,
			IsActive: true,
			Items: buildBundleJSON(t, []database.BundleItemRef{
				{MenuItemID: "item-burger", Quantity: 1},
				{MenuItemID: "item-cola", Quantity: 1},
			}),
		},
	}

	result, err := ApplyPromotionsToOrder(categories, bundles, nil, []PromotionInputLine{
		{Name: "Combo", Quantity: 1, UnitPrice: 15, ItemType: OrderItemTypeBundle, BundleID: ptrUint(10)},
		{Name: "Combo", Quantity: 1, UnitPrice: 15, ItemType: OrderItemTypeBundle, BundleID: ptrUint(10)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	childCounts := make(map[string]int)
	parentOccurrences := make(map[string]struct{})
	for _, line := range result.Lines {
		switch line.ItemType {
		case OrderItemTypeBundle:
			if line.BundleOccurrenceID == "" {
				t.Fatal("bundle parent must carry an occurrence id")
			}
			parentOccurrences[line.BundleOccurrenceID] = struct{}{}
		case OrderItemTypeBundleItem:
			childCounts[line.BundleOccurrenceID]++
		}
	}
	if len(parentOccurrences) != 2 {
		t.Fatalf("expected two distinct bundle occurrences, got %d", len(parentOccurrences))
	}
	for occurrence := range parentOccurrences {
		if childCounts[occurrence] != 2 {
			t.Fatalf("expected occurrence %q to own 2 children, got %d", occurrence, childCounts[occurrence])
		}
	}
}

func TestApplyPromotionsToOrder_AdditiveCap(t *testing.T) {
	categories := []database.MenuCategory{
		{ID: "cat-main", Name: "Mains", Items: []database.MenuItem{{ID: "item-soup", Name: "Soup", Price: 10, IsAvailable: true}}},
	}
	offers := []database.Offer{
		{ID: 1, Name: "Fixed 8", DiscountType: "fixed", DiscountValue: 8, ApplicableTo: "all", IsActive: true},
		{ID: 2, Name: "Fixed 7", DiscountType: "fixed", DiscountValue: 7, ApplicableTo: "all", IsActive: true},
	}

	result, err := ApplyPromotionsToOrder(categories, nil, offers, []PromotionInputLine{{
		Name: "Soup", MenuItemID: "item-soup", Quantity: 1, UnitPrice: 10,
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !approxEqual(result.BaseSubtotal, 10) {
		t.Fatalf("expected base subtotal 10, got %.2f", result.BaseSubtotal)
	}
	if !approxEqual(result.DiscountTotal, 10) {
		t.Fatalf("expected capped discount total 10, got %.2f", result.DiscountTotal)
	}
	if len(result.AppliedOffers) != 2 {
		t.Fatalf("expected 2 applied offers, got %d", len(result.AppliedOffers))
	}
	if !approxEqual(result.AppliedOffers[1].Amount, 2) {
		t.Fatalf("expected second offer capped to 2, got %.2f", result.AppliedOffers[1].Amount)
	}
}

// #946: a fixed item offer is once per qualifying plate (quantity), not once per check.
func TestApplyPromotionsToOrder_FixedOfferAppliesOncePerQualifyingPlate(t *testing.T) {
	bifeCatalog := []database.MenuCategory{{
		ID: "cat-main", Name: "Mains",
		Items: []database.MenuItem{
			{ID: "item-bife", Name: "Bife", Price: 34000, IsAvailable: true},
			{ID: "item-cheap-bife", Name: "Cheap Bife", Price: 1000, IsAvailable: true},
		},
	}}
	bifeOffer := []database.Offer{{
		ID: 1, Name: "AR$ 3.000 menos en el bife", DiscountType: "fixed", DiscountValue: 3000,
		ApplicableTo: "item", TargetID: ptrString("item-bife"), IsActive: true,
	}}
	cheapOffer := []database.Offer{{
		ID: 2, Name: "AR$ 3.000 menos en el bife", DiscountType: "fixed", DiscountValue: 3000,
		ApplicableTo: "item", TargetID: ptrString("item-cheap-bife"), IsActive: true,
	}}

	t.Run("qty1", func(t *testing.T) {
		result, err := ApplyPromotionsToOrder(bifeCatalog, nil, bifeOffer, []PromotionInputLine{{
			Name: "Bife", MenuItemID: "item-bife", Quantity: 1, ItemType: OrderItemTypeMenuItem,
		}})
		require.NoError(t, err)
		require.InDelta(t, 34000, result.BaseSubtotal, 0.001)
		require.InDelta(t, 3000, result.DiscountTotal, 0.001, "one bife must take 3000 off")
	})

	t.Run("qty2", func(t *testing.T) {
		result, err := ApplyPromotionsToOrder(bifeCatalog, nil, bifeOffer, []PromotionInputLine{{
			Name: "Bife", MenuItemID: "item-bife", Quantity: 2, ItemType: OrderItemTypeMenuItem,
		}})
		require.NoError(t, err)
		require.InDelta(t, 68000, result.BaseSubtotal, 0.001)
		require.InDelta(t, 6000, result.DiscountTotal, 0.001, "qty 2 must take 6000 off, not 3000 once per check")
	})

	t.Run("twoLines", func(t *testing.T) {
		result, err := ApplyPromotionsToOrder(bifeCatalog, nil, bifeOffer, []PromotionInputLine{
			{Name: "Bife", MenuItemID: "item-bife", Quantity: 1, ItemType: OrderItemTypeMenuItem},
			{Name: "Bife", MenuItemID: "item-bife", Quantity: 1, ItemType: OrderItemTypeMenuItem},
		})
		require.NoError(t, err)
		require.InDelta(t, 68000, result.BaseSubtotal, 0.001)
		require.InDelta(t, 6000, result.DiscountTotal, 0.001, "two bife lines must take 6000 off, not 3000 once per check")
	})

	t.Run("capAtEligibleSubtotal", func(t *testing.T) {
		result, err := ApplyPromotionsToOrder(bifeCatalog, nil, cheapOffer, []PromotionInputLine{{
			Name: "Cheap Bife", MenuItemID: "item-cheap-bife", Quantity: 2, ItemType: OrderItemTypeMenuItem,
		}})
		require.NoError(t, err)
		require.InDelta(t, 2000, result.BaseSubtotal, 0.001)
		require.InDelta(t, 2000, result.DiscountTotal, 0.001, "2×3000 off must recap to the 2000 eligible subtotal")
	})
}

// #946 guard: check-wide fixed offers ("all", and the legacy empty scope the
// offer form persisted by default) are "$X off the check". They must not scale
// with plate count, or "$8 off" becomes $24 on a three-plate order.
func TestApplyPromotionsToOrder_FixedCheckWideOfferDoesNotScaleWithPlates(t *testing.T) {
	catalog := []database.MenuCategory{{
		ID: "cat-main", Name: "Mains",
		Items: []database.MenuItem{
			{ID: "item-bife", Name: "Bife", Price: 34000, IsAvailable: true},
			{ID: "item-chips", Name: "Chips", Price: 300, IsAvailable: true},
			{ID: "item-water", Name: "Water", Price: 5, IsAvailable: true},
		},
	}}

	for _, scope := range []string{"all", ""} {
		scope := scope
		name := scope
		if name == "" {
			name = "legacy-empty"
		}
		t.Run(name, func(t *testing.T) {
			offers := []database.Offer{{
				ID: 1, Name: "$8 off the check", DiscountType: "fixed", DiscountValue: 8,
				ApplicableTo: scope, IsActive: true,
			}}

			t.Run("singlePlate", func(t *testing.T) {
				result, err := ApplyPromotionsToOrder(catalog, nil, offers, []PromotionInputLine{{
					Name: "Bife", MenuItemID: "item-bife", Quantity: 1, ItemType: OrderItemTypeMenuItem,
				}})
				require.NoError(t, err)
				require.InDelta(t, 34000, result.BaseSubtotal, 0.001)
				require.InDelta(t, 8, result.DiscountTotal, 0.001, "one plate takes $8 off the check")
			})

			t.Run("threePlatesSameLine", func(t *testing.T) {
				result, err := ApplyPromotionsToOrder(catalog, nil, offers, []PromotionInputLine{{
					Name: "Bife", MenuItemID: "item-bife", Quantity: 3, ItemType: OrderItemTypeMenuItem,
				}})
				require.NoError(t, err)
				require.InDelta(t, 102000, result.BaseSubtotal, 0.001)
				require.InDelta(t, 8, result.DiscountTotal, 0.001,
					"$8 off the check must stay $8 on 3 plates, not become $24")
			})

			t.Run("threePlatesThreeLines", func(t *testing.T) {
				result, err := ApplyPromotionsToOrder(catalog, nil, offers, []PromotionInputLine{
					{Name: "Bife", MenuItemID: "item-bife", Quantity: 1, ItemType: OrderItemTypeMenuItem},
					{Name: "Bife", MenuItemID: "item-bife", Quantity: 1, ItemType: OrderItemTypeMenuItem},
					{Name: "Chips", MenuItemID: "item-chips", Quantity: 1, ItemType: OrderItemTypeMenuItem},
				})
				require.NoError(t, err)
				require.InDelta(t, 68300, result.BaseSubtotal, 0.001)
				require.InDelta(t, 8, result.DiscountTotal, 0.001,
					"$8 off the check must stay $8 across 3 lines")
			})

			t.Run("capsAtEligibleSubtotal", func(t *testing.T) {
				result, err := ApplyPromotionsToOrder(catalog, nil, offers, []PromotionInputLine{{
					Name: "Water", MenuItemID: "item-water", Quantity: 1, ItemType: OrderItemTypeMenuItem,
				}})
				require.NoError(t, err)
				require.InDelta(t, 5, result.BaseSubtotal, 0.001)
				require.InDelta(t, 5, result.DiscountTotal, 0.001,
					"$8 off a $5 check must recap to the eligible subtotal")
			})
		})
	}
}

func TestApplyPromotionsToOrder_IgnoresInactiveAndOutOfWindowOffers(t *testing.T) {
	categories := []database.MenuCategory{
		{ID: "cat-main", Name: "Mains", Items: []database.MenuItem{{ID: "item-rice", Name: "Rice", Price: 20, IsAvailable: true}}},
	}
	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)
	tomorrow := now.Add(24 * time.Hour)

	offers := []database.Offer{
		{ID: 1, Name: "Inactive", DiscountType: "percentage", DiscountValue: 90, ApplicableTo: "all", IsActive: false},
		{ID: 2, Name: "Expired", DiscountType: "percentage", DiscountValue: 80, ApplicableTo: "all", IsActive: true, EndDate: &yesterday},
		{ID: 3, Name: "Future", DiscountType: "percentage", DiscountValue: 70, ApplicableTo: "all", IsActive: true, StartDate: &tomorrow},
		{ID: 4, Name: "Valid", DiscountType: "percentage", DiscountValue: 10, ApplicableTo: "all", IsActive: true},
	}

	result, err := ApplyPromotionsToOrder(categories, nil, offers, []PromotionInputLine{{
		Name: "Rice", MenuItemID: "item-rice", Quantity: 1, UnitPrice: 20,
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.AppliedOffers) != 1 {
		t.Fatalf("expected 1 applied offer, got %d", len(result.AppliedOffers))
	}
	if result.AppliedOffers[0].OfferID != 4 {
		t.Fatalf("expected valid offer ID 4, got %d", result.AppliedOffers[0].OfferID)
	}
	if !approxEqual(result.DiscountTotal, 2) {
		t.Fatalf("expected discount total 2.00, got %.2f", result.DiscountTotal)
	}
}

func TestApplyPromotionsToOrder_IgnoresLunchOfferOutsideDaypart(t *testing.T) {
	categories := []database.MenuCategory{
		{ID: "cat-main", Name: "Mains", Items: []database.MenuItem{{ID: "item-rice", Name: "Rice", Price: 20, IsAvailable: true}}},
	}
	start, end := 11*60, 15*60
	offers := []database.Offer{{
		ID: 1, Name: "Weekday Lunch 15% Off", DiscountType: "percentage", DiscountValue: 15,
		ApplicableTo: "all", IsActive: true, WeekdayMask: 62, StartMinute: &start, EndMinute: &end,
	}}
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	inWindow := database.FilterOffersActiveAt(offers, time.Date(2026, 8, 12, 12, 0, 0, 0, ny), "America/New_York")
	if len(inWindow) != 1 {
		t.Fatalf("expected lunch offer at 12:00 NY, got %d", len(inWindow))
	}
	outOfWindow := database.FilterOffersActiveAt(offers, time.Date(2026, 8, 12, 15, 14, 0, 0, ny), "America/New_York")
	if len(outOfWindow) != 0 {
		t.Fatalf("expected no lunch offer at 15:14 NY, got %d", len(outOfWindow))
	}
	result, err := ApplyPromotionsToOrder(categories, nil, outOfWindow, []PromotionInputLine{{
		Name: "Rice", MenuItemID: "item-rice", Quantity: 1, UnitPrice: 20,
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.AppliedOffers) != 0 {
		t.Fatalf("lunch offer must not auto-apply at 15:14 NY, applied %d", len(result.AppliedOffers))
	}
}

func TestComputeBillTotals_AppliesTaxAndServiceToDiscountedSubtotal(t *testing.T) {
	tax, service, total := ComputeBillTotals(80, 10, 5)
	if !approxEqual(tax, 8) {
		t.Fatalf("expected tax 8.00, got %.2f", tax)
	}
	if !approxEqual(service, 4) {
		t.Fatalf("expected service 4.00, got %.2f", service)
	}
	if !approxEqual(total, 92) {
		t.Fatalf("expected total 92.00, got %.2f", total)
	}
}

// REV-7 / NEW-13: discount line name must be the plain offer name (no "Offer:" prefix).
func TestApplyPromotionsToOrder_OfferLineNameUnprefixed(t *testing.T) {
	categories := []database.MenuCategory{
		{
			ID:   "cat-main",
			Name: "Mains",
			Items: []database.MenuItem{
				{ID: "item-burger", Name: "Burger", Price: 12, IsAvailable: true},
			},
		},
	}
	offers := []database.Offer{
		{
			ID:            42,
			Name:          "Happy Hour 10%",
			DiscountType:  "percentage",
			DiscountValue: 10,
			ApplicableTo:  "all",
			IsActive:      true,
		},
	}

	result, err := ApplyPromotionsToOrder(categories, nil, offers, []PromotionInputLine{
		{Name: "Burger", MenuItemID: "item-burger", Quantity: 1, UnitPrice: 12, ItemType: OrderItemTypeMenuItem},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var discount *database.OrderItem
	for i := range result.Lines {
		line := &result.Lines[i]
		if line.ItemType == OrderItemTypeDiscount {
			discount = line
			break
		}
	}
	if discount == nil {
		t.Fatal("expected a discount line from the offer")
	}
	if discount.MenuItemName != "Happy Hour 10%" {
		t.Fatalf("MenuItemName = %q, want plain offer name %q", discount.MenuItemName, "Happy Hour 10%")
	}
	if len(discount.MenuItemName) >= 6 && discount.MenuItemName[:6] == "Offer:" {
		t.Fatalf("MenuItemName must not start with Offer: prefix, got %q", discount.MenuItemName)
	}
	if len(result.AppliedOffers) != 1 || result.AppliedOffers[0].Name != "Happy Hour 10%" {
		t.Fatalf("AppliedOffers name = %#v, want Happy Hour 10%%", result.AppliedOffers)
	}
}

func demoTeaSteakCatalog() []database.MenuCategory {
	return []database.MenuCategory{
		{
			ID:   "mains",
			Name: "Mains",
			Items: []database.MenuItem{
				{ID: "demo-steak", Name: "Steak Plate", Price: 42, IsAvailable: true},
			},
		},
		{
			ID:   "drinks",
			Name: "Drinks",
			Items: []database.MenuItem{
				{ID: "demo-tea", Name: "Iced Tea", Price: 5, IsAvailable: true},
			},
		},
	}
}

func firstMenuLine(t *testing.T, result PromotionResult) database.OrderItem {
	t.Helper()
	for _, line := range result.Lines {
		if line.ItemType == OrderItemTypeMenuItem || line.ItemType == "" {
			return line
		}
	}
	t.Fatal("expected a menu item line")
	return database.OrderItem{}
}

// #534: after menu_item_id resolves, the request name is discarded. Pricing
// and the persisted/quoted name both come from the catalog.
func TestApplyPromotionsToOrder_DiscardsClientNameAfterCatalogResolve(t *testing.T) {
	result, err := ApplyPromotionsToOrder(demoTeaSteakCatalog(), nil, nil, []PromotionInputLine{{
		Name: "Steak Plate", MenuItemID: "demo-tea", Quantity: 1, UnitPrice: 42,
		ItemType: OrderItemTypeMenuItem,
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	line := firstMenuLine(t, result)
	if line.MenuItemID != "demo-tea" {
		t.Fatalf("MenuItemID = %q, want demo-tea", line.MenuItemID)
	}
	if line.MenuItemName != "Iced Tea" {
		t.Fatalf("MenuItemName = %q, want catalog Iced Tea (client sent Steak Plate)", line.MenuItemName)
	}
	if !approxEqual(line.Price, 5) || !approxEqual(line.Subtotal, 5) {
		t.Fatalf("price/subtotal = %.2f/%.2f, want tea 5.00", line.Price, line.Subtotal)
	}
	if !approxEqual(result.BaseSubtotal, 5) {
		t.Fatalf("BaseSubtotal = %.2f, want 5.00", result.BaseSubtotal)
	}
}

// #534: item-scoped offers match the resolved catalog id only. A forged
// menu_item_name must not claim another item's discount.
func TestApplyPromotionsToOrder_ItemOfferMatchesCatalogIDNotClientName(t *testing.T) {
	steakOffer := []database.Offer{{
		ID: 1, Name: "$5 Off the Steak Plate", DiscountType: "fixed", DiscountValue: 5,
		ApplicableTo: "item", TargetID: ptrString("demo-steak"), IsActive: true,
	}}
	nameTargeted := []database.Offer{{
		ID: 2, Name: "Steak name target", DiscountType: "fixed", DiscountValue: 5,
		ApplicableTo: "item", TargetID: ptrString("Steak Plate"), IsActive: true,
	}}

	teaAsSteak, err := ApplyPromotionsToOrder(demoTeaSteakCatalog(), nil, steakOffer, []PromotionInputLine{{
		Name: "Steak Plate", MenuItemID: "demo-tea", Quantity: 1, ItemType: OrderItemTypeMenuItem,
	}})
	if err != nil {
		t.Fatalf("id-targeted tea: unexpected error: %v", err)
	}
	if len(teaAsSteak.AppliedOffers) != 0 || !approxEqual(teaAsSteak.DiscountTotal, 0) {
		t.Fatalf("steak-id offer must not apply to tea with forged name, applied=%v discount=%.2f",
			teaAsSteak.AppliedOffers, teaAsSteak.DiscountTotal)
	}

	teaNameTarget, err := ApplyPromotionsToOrder(demoTeaSteakCatalog(), nil, nameTargeted, []PromotionInputLine{{
		Name: "Steak Plate", MenuItemID: "demo-tea", Quantity: 1, ItemType: OrderItemTypeMenuItem,
	}})
	if err != nil {
		t.Fatalf("name-targeted tea: unexpected error: %v", err)
	}
	if len(teaNameTarget.AppliedOffers) != 0 || !approxEqual(teaNameTarget.DiscountTotal, 0) {
		t.Fatalf("name-targeted steak offer must not apply to tea, applied=%v discount=%.2f",
			teaNameTarget.AppliedOffers, teaNameTarget.DiscountTotal)
	}

	honestSteak, err := ApplyPromotionsToOrder(demoTeaSteakCatalog(), nil, steakOffer, []PromotionInputLine{{
		Name: "Iced Tea", MenuItemID: "demo-steak", Quantity: 1, ItemType: OrderItemTypeMenuItem,
	}})
	if err != nil {
		t.Fatalf("honest steak: unexpected error: %v", err)
	}
	line := firstMenuLine(t, honestSteak)
	if line.MenuItemName != "Steak Plate" {
		t.Fatalf("forged tea name on steak must still persist as Steak Plate, got %q", line.MenuItemName)
	}
	if len(honestSteak.AppliedOffers) != 1 || !approxEqual(honestSteak.DiscountTotal, 5) {
		t.Fatalf("steak-id offer must still apply to steak, applied=%v discount=%.2f",
			honestSteak.AppliedOffers, honestSteak.DiscountTotal)
	}
}

// After a bundle_id resolves, the request name is discarded the same way as
// menu items. A cheap combo must not print as the expensive combo.
func TestApplyPromotionsToOrder_DiscardsClientBundleNameAfterCatalogResolve(t *testing.T) {
	categories := []database.MenuCategory{
		{ID: "mains", Name: "Mains", Items: []database.MenuItem{
			{ID: "demo-steak", Name: "Steak Plate", Price: 42, IsAvailable: true},
			{ID: "demo-tea", Name: "Iced Tea", Price: 5, IsAvailable: true},
		}},
	}
	bundles := []database.Bundle{
		{
			ID: 10, Name: "Tea Combo", Price: 8, IsActive: true,
			Items: buildBundleJSON(t, []database.BundleItemRef{
				{MenuItemID: "demo-tea", Name: "Steak Plate", Quantity: 1},
			}),
		},
		{
			ID: 20, Name: "Steak Dinner", Price: 50, IsActive: true,
			Items: buildBundleJSON(t, []database.BundleItemRef{
				{MenuItemID: "demo-steak", Name: "Steak Plate", Quantity: 1},
			}),
		},
	}

	result, err := ApplyPromotionsToOrder(categories, bundles, nil, []PromotionInputLine{{
		Name: "Steak Dinner", Quantity: 1, ItemType: OrderItemTypeBundle, BundleID: ptrUint(10),
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !approxEqual(result.BaseSubtotal, 8) {
		t.Fatalf("BaseSubtotal = %.2f, want tea combo 8.00", result.BaseSubtotal)
	}
	var parent, child *database.OrderItem
	for i := range result.Lines {
		switch result.Lines[i].ItemType {
		case OrderItemTypeBundle:
			parent = &result.Lines[i]
		case OrderItemTypeBundleItem:
			child = &result.Lines[i]
		}
	}
	if parent == nil || parent.MenuItemName != "Tea Combo" {
		t.Fatalf("bundle parent name = %v, want catalog Tea Combo", parent)
	}
	if !approxEqual(parent.Price, 8) {
		t.Fatalf("bundle parent price = %.2f, want 8.00", parent.Price)
	}
	if child == nil {
		t.Fatal("expected a bundle child line")
	}
	if child.MenuItemName != "Iced Tea" || child.MenuItemID != "demo-tea" {
		t.Fatalf("bundle child must be catalog Iced Tea, got id=%q name=%q", child.MenuItemID, child.MenuItemName)
	}

	_, err = ApplyPromotionsToOrder(categories, bundles, nil, []PromotionInputLine{{
		Name: "Steak Dinner", Quantity: 1, ItemType: OrderItemTypeBundle, BundleID: ptrUint(999),
	}})
	if err == nil {
		t.Fatal("unknown bundle_id must not fall back to the client bundle name")
	}

	nameOnly, err := ApplyPromotionsToOrder(demoTeaSteakCatalog(), nil, nil, []PromotionInputLine{{
		Name: "Iced Tea", Quantity: 1, ItemType: OrderItemTypeMenuItem,
	}})
	if err != nil {
		t.Fatalf("name-only lookup: unexpected error: %v", err)
	}
	line := firstMenuLine(t, nameOnly)
	if line.MenuItemID != "demo-tea" || line.MenuItemName != "Iced Tea" || !approxEqual(line.Price, 5) {
		t.Fatalf("name-only lookup must still resolve catalog tea, got id=%q name=%q price=%.2f",
			line.MenuItemID, line.MenuItemName, line.Price)
	}

	remapped, err := ApplyPromotionsToOrder(demoTeaSteakCatalog(), nil, nil, []PromotionInputLine{{
		Name: "Steak Plate", MenuItemID: "missing-id", Quantity: 1, ItemType: OrderItemTypeMenuItem,
	}})
	if err != nil {
		t.Fatalf("stale menu_item_id must remap via catalog name (#540): %v", err)
	}
	line = firstMenuLine(t, remapped)
	if line.MenuItemID != "demo-steak" || line.MenuItemName != "Steak Plate" || !approxEqual(line.Price, 42) {
		t.Fatalf("stale id remapped to steak, got id=%q name=%q price=%.2f",
			line.MenuItemID, line.MenuItemName, line.Price)
	}

	_, err = ApplyPromotionsToOrder(demoTeaSteakCatalog(), nil, nil, []PromotionInputLine{{
		Name: "Definitely Not On The Menu", MenuItemID: "missing-id", Quantity: 1, ItemType: OrderItemTypeMenuItem,
	}})
	if err == nil {
		t.Fatal("unknown id + unknown name must still be item_not_found")
	}
}
