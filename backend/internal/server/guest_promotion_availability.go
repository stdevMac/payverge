package server

import (
	"fmt"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

var venueWideGuestOrderability = map[services.OrderabilityState]struct{}{
	services.OrderabilityBusinessClosed: {},
	services.OrderabilityOrderingOff:    {},
}

// unsellableGuestMenuItemIDs is the 86 / inventory-out set used to hide
// targeted guest offers and bundles. Venue-wide closed / ordering-off states
// are not an item 86 — those already have their own guest banners.
func unsellableGuestMenuItemIDs(
	categories []database.MenuCategory,
	projection map[string]services.Orderability,
) map[string]bool {
	hidden := make(map[string]bool)
	for _, category := range categories {
		for _, item := range category.Items {
			id := strings.TrimSpace(item.ID)
			if id == "" {
				continue
			}
			if !item.IsAvailable {
				hidden[id] = true
				continue
			}
			decision, ok := projection[id]
			if !ok {
				continue
			}
			if _, venueWide := venueWideGuestOrderability[decision.State]; venueWide {
				continue
			}
			if !decision.Orderable {
				hidden[id] = true
			}
		}
	}
	return hidden
}

func liveGuestBundleKeys(bundles []database.Bundle) map[string]bool {
	live := make(map[string]bool, len(bundles)*2)
	for _, bundle := range bundles {
		live[fmt.Sprintf("%d", bundle.ID)] = true
		if name := strings.TrimSpace(bundle.Name); name != "" {
			live[name] = true
		}
	}
	return live
}

// filterOffersByMissingBundles drops bundle-targeted offers whose combo is no
// longer guest-sellable (a Date Night $10-off while steak is 86'd).
func filterOffersByMissingBundles(offers []database.Offer, liveBundles []database.Bundle) []database.Offer {
	if len(offers) == 0 {
		return offers
	}
	live := liveGuestBundleKeys(liveBundles)
	out := make([]database.Offer, 0, len(offers))
	for _, offer := range offers {
		if strings.EqualFold(strings.TrimSpace(offer.ApplicableTo), "bundle") && offer.TargetID != nil {
			target := strings.TrimSpace(*offer.TargetID)
			if target != "" && !live[target] {
				continue
			}
		}
		out = append(out, offer)
	}
	return out
}

// filterGuestLivePromotions hides item-targeted offers and bundles that depend
// on an unsellable (86 / inventory_out) child. Broad / category offers stay.
func filterGuestLivePromotions(
	offers []database.Offer,
	bundles []database.Bundle,
	categories []database.MenuCategory,
	projection map[string]services.Orderability,
) ([]database.Offer, []database.Bundle) {
	hidden := unsellableGuestMenuItemIDs(categories, projection)
	liveBundles := filterBundlesByHiddenItems(bundles, hidden)
	liveOffers := filterOffersByHiddenItems(offers, hidden)
	liveOffers = filterOffersByMissingBundles(liveOffers, liveBundles)
	return liveOffers, liveBundles
}

// cloneGuestMenuCategories copies category/item structs so serve-time 86 stamps
// cannot mutate the shared pricing-cache snapshot. Item option slices stay
// shared; projection only writes InventoryStatus / IsAvailable on the item.
func cloneGuestMenuCategories(categories []database.MenuCategory) []database.MenuCategory {
	if categories == nil {
		return nil
	}
	out := make([]database.MenuCategory, len(categories))
	for i, category := range categories {
		out[i] = category
		if category.Items == nil {
			continue
		}
		items := make([]database.MenuItem, len(category.Items))
		copy(items, category.Items)
		out[i].Items = items
	}
	return out
}

// applyGuestLivePromotions filters guest offers/bundles against the same
// orderability projection the menu payload already computed, so table/menu
// JSON never advertises a combo or item offer the kitchen cannot fire.
// Stamps land on a cloned catalog: the pricing cache is shared across requests
// and must keep the stored manual is_available flag.
func applyGuestLivePromotions(
	business *database.Business,
	menu *database.Menu,
	categories []database.MenuCategory,
	offers []database.Offer,
	bundles []database.Bundle,
) (gin.H, []database.MenuCategory, []database.Offer, []database.Bundle) {
	if business == nil && menu != nil {
		business = guestFallbackBusiness(menu.BusinessID)
	}
	live := cloneGuestMenuCategories(categories)
	projection := projectGuestOrderability(business, live)
	offers, bundles = filterGuestLivePromotions(
		offers,
		bundles,
		live,
		projection,
	)
	return gin.H{"item_orderability": projection}, live, offers, bundles
}
