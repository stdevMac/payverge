package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// WaiterRuntimeContext is the shared, channel-agnostic snapshot used to ground
// both the web AI Waiter and WhatsApp waiter on the same menu/stock/promo state.
type WaiterRuntimeContext struct {
	BusinessID          uint
	BusinessName        string
	BusinessDescription string
	BusinessAddress     string
	AIName              string
	AIPriority          string
	SpecialInstructions string
	Locale              string
	MenuJSON            string
	OffersJSON          string
	BundlesJSON         string
	ReservationContext  string
	DeliveryContext     string
	DisplayCategories   []database.MenuCategory
	DisplayBundles      []database.Bundle
	DisplayOffers       []database.Offer
}

// BuildWaiterRuntimeContext assembles orderable, stock-grounded menu context for
// a business. Locale is recorded for callers; full translation remains the
// responsibility of channel adapters that already own translation tables
// (server package). The critical parity contract is: same orderable items,
// stock exclusions, active offers/bundles, reservation and delivery state.
func BuildWaiterRuntimeContext(
	_ context.Context,
	business *database.Business,
	locale string,
) (WaiterRuntimeContext, error) {
	if business == nil {
		return WaiterRuntimeContext{}, fmt.Errorf("business is required")
	}
	_, categories, err := database.GetMenuByBusinessID(business.ID)
	if err != nil {
		return WaiterRuntimeContext{}, fmt.Errorf("menu not found: %w", err)
	}

	offers, oerr := database.GetActiveOffersByBusinessIDAt(business.ID, database.ScheduleNow(), business.Timezone)
	if oerr != nil {
		log.Printf("waiter context: offers load business=%d: %v", business.ID, oerr)
		offers = []database.Offer{}
	}
	bundles, berr := database.GetActiveBundlesByBusinessID(business.ID)
	if berr != nil {
		log.Printf("waiter context: bundles load business=%d: %v", business.ID, berr)
		bundles = []database.Bundle{}
	}

	displayCategories := categories
	displayOffers := offers
	displayBundles := bundles
	projection := ProjectOrderability(
		business,
		categories,
		OrderabilityContextGuest,
		BusinessOpenAt(database.GetDB(), business, time.Now()),
	)
	hidden := make(map[string]bool)
	for itemID, decision := range projection {
		if !decision.Orderable {
			hidden[itemID] = true
		}
	}
	if len(hidden) > 0 {
		displayCategories = filterWaiterMenuByHidden(displayCategories, hidden)
		displayOffers = filterWaiterOffersByHidden(displayOffers, hidden)
		displayBundles = filterWaiterBundlesByHidden(displayBundles, hidden)
	}

	menuJSON, _ := json.Marshal(displayCategories)
	offersJSON, _ := json.Marshal(displayOffers)
	bundlesJSON, _ := json.Marshal(displayBundles)

	reservationContext := "Reservations are DISABLED. Guests cannot book tables through the AI."
	if resvSettings, rerr := database.GetReservationSettings(business.ID); rerr == nil && resvSettings != nil && resvSettings.Enabled && database.IsBusinessOperational(business) {
		reservationContext = fmt.Sprintf(
			"Reservations are ENABLED. Guests can book for parties of %d to %d people.",
			resvSettings.MinPartySize, resvSettings.MaxPartySize,
		)
	}

	deliveryContext := ""
	var delSettings database.DeliverySettings
	if derr := database.GetDB().Where("business_id = ?", business.ID).First(&delSettings).Error; derr == nil {
		// Keep partner-link copy on a non-operational storefront, but do not
		// advertise in-house checkout the quote/create path will refuse.
		if !database.IsBusinessOperational(business) {
			delSettings.InHouseDeliveryEnabled = false
		}
		deliveryContext = buildWhatsAppDeliveryHint(&delSettings)
	}

	aiName := strings.TrimSpace(business.AiSettings.AiName)
	if aiName == "" {
		aiName = "Sage"
	}

	addr := business.Address
	businessAddress := fmt.Sprintf("%s, %s, %s %s, %s",
		addr.Street, addr.City, addr.State, addr.PostalCode, addr.Country)

	return WaiterRuntimeContext{
		BusinessID:          business.ID,
		BusinessName:        business.Name,
		BusinessDescription: business.Description,
		BusinessAddress:     businessAddress,
		AIName:              aiName,
		AIPriority:          business.AiSettings.AiPriority,
		SpecialInstructions: business.AiSettings.SpecialInstructions,
		Locale:              locale,
		MenuJSON:            string(menuJSON),
		OffersJSON:          string(offersJSON),
		BundlesJSON:         string(bundlesJSON),
		ReservationContext:  reservationContext,
		DeliveryContext:     deliveryContext,
		DisplayCategories:   displayCategories,
		DisplayBundles:      displayBundles,
		DisplayOffers:       displayOffers,
	}, nil
}

func filterWaiterMenuByHidden(cats []database.MenuCategory, hidden map[string]bool) []database.MenuCategory {
	if len(hidden) == 0 {
		return cats
	}
	out := make([]database.MenuCategory, 0, len(cats))
	for _, cat := range cats {
		kept := make([]database.MenuItem, 0, len(cat.Items))
		for _, item := range cat.Items {
			if hidden[strings.TrimSpace(item.ID)] {
				continue
			}
			kept = append(kept, item)
		}
		if len(kept) == 0 {
			continue
		}
		cat.Items = kept
		out = append(out, cat)
	}
	return out
}

func filterWaiterOffersByHidden(offers []database.Offer, hidden map[string]bool) []database.Offer {
	if len(hidden) == 0 {
		return offers
	}
	out := make([]database.Offer, 0, len(offers))
	for _, offer := range offers {
		if offer.ApplicableTo == "item" && offer.TargetID != nil && hidden[strings.TrimSpace(*offer.TargetID)] {
			continue
		}
		out = append(out, offer)
	}
	return out
}

func filterWaiterBundlesByHidden(bundles []database.Bundle, hidden map[string]bool) []database.Bundle {
	if len(hidden) == 0 {
		return bundles
	}
	out := make([]database.Bundle, 0, len(bundles))
	for _, bundle := range bundles {
		var refs []database.BundleItemRef
		if err := json.Unmarshal([]byte(bundle.Items), &refs); err != nil {
			out = append(out, bundle)
			continue
		}
		blocked := false
		for _, ref := range refs {
			if hidden[strings.TrimSpace(ref.MenuItemID)] {
				blocked = true
				break
			}
		}
		if !blocked {
			out = append(out, bundle)
		}
	}
	return out
}
