package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/money"
)

const (
	OrderItemTypeMenuItem   = "menu_item"
	OrderItemTypeBundle     = "bundle"
	OrderItemTypeBundleItem = "bundle_item"
	OrderItemTypeDiscount   = "discount"
)

// PromotionInputLine is a normalized request item passed to the pricing engine.
type PromotionInputLine struct {
	Name            string
	MenuItemID      string
	Quantity        int
	UnitPrice       float64
	Options         []database.MenuItemOption
	SpecialRequests string
	ItemType        string
	BundleID        *uint
	ParentBundleID  *uint
	SourceOfferID   *uint
}

// AppliedOffer captures discount amounts applied by a specific offer.
type AppliedOffer struct {
	OfferID uint    `json:"offer_id"`
	Name    string  `json:"name"`
	Amount  float64 `json:"amount"`
}

// PromotionResult contains expanded order lines and discount metadata.
type PromotionResult struct {
	Lines         []database.OrderItem `json:"lines"`
	BaseSubtotal  float64              `json:"base_subtotal"`
	DiscountTotal float64              `json:"discount_total"`
	AppliedOffers []AppliedOffer       `json:"applied_offers"`
}

type menuItemLookup struct {
	ID   string
	Name string
	// Category is the stable category KEY (ID when present, else name) used for
	// id-based category offer matching. CategoryName is the display name, kept so
	// a legacy name-targeted category offer still matches (§3.7 fix 6 BE-first).
	Category     string
	CategoryName string
	Price        float64
	Options      []database.MenuItemOption
	Available    bool
}

func normalizeItemType(itemType string) string {
	switch strings.ToLower(strings.TrimSpace(itemType)) {
	case OrderItemTypeBundle:
		return OrderItemTypeBundle
	case OrderItemTypeBundleItem:
		return OrderItemTypeBundleItem
	case OrderItemTypeDiscount:
		return OrderItemTypeDiscount
	default:
		return OrderItemTypeMenuItem
	}
}

// promotionLineLabel prefers the catalog id so qty/text errors can name the
// offending line when the client omitted menu_item_name.
func promotionLineLabel(line PromotionInputLine) string {
	if id := strings.TrimSpace(line.MenuItemID); id != "" {
		return id
	}
	if name := strings.TrimSpace(line.Name); name != "" {
		return name
	}
	if line.BundleID != nil {
		return fmt.Sprintf("bundle:%d", *line.BundleID)
	}
	return ""
}

func buildMenuLookups(categories []database.MenuCategory) (map[string]menuItemLookup, map[string]menuItemLookup) {
	byID := map[string]menuItemLookup{}
	byName := map[string]menuItemLookup{}

	for _, category := range categories {
		categoryName := strings.TrimSpace(category.Name)
		categoryKey := strings.TrimSpace(category.ID)
		if categoryKey == "" {
			categoryKey = categoryName
		}

		for _, item := range category.Items {
			lookup := menuItemLookup{
				ID:           strings.TrimSpace(item.ID),
				Name:         strings.TrimSpace(item.Name),
				Price:        item.Price,
				Category:     categoryKey,
				CategoryName: categoryName,
				Options:      item.Options,
				Available:    item.IsAvailable,
			}

			if lookup.ID != "" {
				byID[lookup.ID] = lookup
			}
			nameKey := strings.ToLower(lookup.Name)
			if nameKey != "" {
				byName[nameKey] = lookup
			}
		}
	}

	return byID, byName
}

func normalizeBundleRefs(raw string) ([]database.BundleItemRef, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	var refs []database.BundleItemRef
	if err := json.Unmarshal([]byte(trimmed), &refs); err == nil {
		for i := range refs {
			if refs[i].Quantity <= 0 {
				refs[i].Quantity = 1
			}
		}
		return refs, nil
	}

	var ids []string
	if err := json.Unmarshal([]byte(trimmed), &ids); err == nil {
		converted := make([]database.BundleItemRef, 0, len(ids))
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			converted = append(converted, database.BundleItemRef{
				MenuItemID: id,
				Quantity:   1,
			})
		}
		return converted, nil
	}

	return nil, fmt.Errorf("invalid bundle items payload")
}

func majorToMinor(value float64) int64 {
	return int64(math.Round(value * 100))
}

func minorToMajor(value int64) float64 {
	return float64(value) / 100
}

func optionTotalCents(options []database.MenuItemOption) int64 {
	var total int64
	for _, option := range options {
		total += majorToMinor(option.PriceChange)
	}
	return total
}

func bundleMapByIDAndName(bundles []database.Bundle) (map[uint]database.Bundle, map[string]database.Bundle) {
	byID := make(map[uint]database.Bundle, len(bundles))
	byName := make(map[string]database.Bundle, len(bundles))
	for _, bundle := range bundles {
		byID[bundle.ID] = bundle
		nameKey := strings.ToLower(strings.TrimSpace(bundle.Name))
		if nameKey != "" {
			byName[nameKey] = bundle
		}
	}
	return byID, byName
}

// LoadActivePromotionsForBusiness loads the active offers and bundles that
// should participate in server-side order pricing.
func LoadActivePromotionsForBusiness(business *database.Business) ([]database.Offer, []database.Bundle) {
	if business == nil {
		return []database.Offer{}, []database.Bundle{}
	}

	offers, err := database.GetActiveOffersByBusinessIDAt(business.ID, database.ScheduleNow(), business.Timezone)
	if err != nil {
		offers = []database.Offer{}
	}

	bundles, err := database.GetActiveBundlesByBusinessID(business.ID)
	if err != nil {
		bundles = []database.Bundle{}
	}

	return offers, bundles
}

// ErrPricingDataUnavailable is returned when the pricing snapshot cannot be
// loaded. The wrapped cause is for logs; handlers must not send it to clients.
var ErrPricingDataUnavailable = errors.New("pricing data unavailable")

// PriceOrderInputsByBusinessID resolves menu data and applies auto-apply
// promotions so callers do not need to trust client-supplied pricing. Offers
// that carry a promo code are excluded; pass the guest's code to
// PriceOrderInputsByBusinessIDWithPromo to include a matching one. Reads come
// from the pricing cache (see PricingCacheTTL); callers that mutate pricing
// inputs should invoke InvalidatePricingCache afterwards.
func PriceOrderInputsByBusinessID(businessID uint, input []PromotionInputLine) (PromotionResult, *database.Business, error) {
	return PriceOrderInputsByBusinessIDWithPromo(businessID, input, "")
}

// PriceOrderInputsByBusinessIDWithPromo is PriceOrderInputsByBusinessID with a
// guest promo code. A coded offer applies only when its code matches promoCode
// (case-insensitive, trimmed). An empty promoCode keeps only auto-apply offers.
func PriceOrderInputsByBusinessIDWithPromo(businessID uint, input []PromotionInputLine, promoCode string) (PromotionResult, *database.Business, error) {
	snap, err := getPricingSnapshot(businessID)
	if err != nil {
		return PromotionResult{}, nil, fmt.Errorf("%w: %w", ErrPricingDataUnavailable, err)
	}

	offers := database.FilterOffersActiveAt(snap.offers, database.ScheduleNow(), snap.business.Timezone)
	offers = filterOffersForPromoCode(offers, promoCode)
	result, err := ApplyPromotionsToOrder(snap.categories, snap.bundles, offers, input)
	if err != nil {
		return PromotionResult{}, nil, err
	}
	if err := requirePricedOrderLines(result); err != nil {
		return PromotionResult{}, nil, err
	}

	return result, snap.business, nil
}

// filterOffersForPromoCode keeps auto-apply offers (nil or blank Code) and
// offers whose Code equals promoCode after trimming, compared case-insensitively.
// A blank promoCode keeps only auto-apply offers. The result is a new slice;
// the input, including a cached pricing snapshot, is left unchanged.
func filterOffersForPromoCode(offers []database.Offer, promoCode string) []database.Offer {
	promoCode = strings.TrimSpace(promoCode)
	filtered := make([]database.Offer, 0, len(offers))
	for _, offer := range offers {
		if offer.Code != nil {
			code := strings.TrimSpace(*offer.Code)
			if code != "" && !strings.EqualFold(code, promoCode) {
				continue
			}
		}
		filtered = append(filtered, offer)
	}
	return filtered
}

// errNoValidPricedLines is the shared post-pricing reject used when every
// request line was discarded (for example client-supplied item_type=discount
// rows) and nothing remains to quote or persist.
var errNoValidPricedLines = errors.New("order must contain at least one valid item")

func requirePricedOrderLines(result PromotionResult) error {
	if len(result.Lines) == 0 {
		return errNoValidPricedLines
	}
	return nil
}

// PromotionLinesSubtotal returns the billable subtotal represented by the
// expanded promotion result lines, including discount rows.
func PromotionLinesSubtotal(lines []database.OrderItem) float64 {
	var subtotalCents int64
	for _, line := range lines {
		subtotalCents += majorToMinor(line.Subtotal)
	}
	return minorToMajor(subtotalCents)
}

// PromotionLinesToBillItems converts authoritative order lines into bill items.
func PromotionLinesToBillItems(lines []database.OrderItem) []database.BillItem {
	billItems := make([]database.BillItem, 0, len(lines))
	stamp := time.Now().UTC()
	for _, line := range lines {
		itemType := strings.TrimSpace(line.ItemType)
		if itemType == "" {
			itemType = OrderItemTypeMenuItem
		}

		menuItemID := strings.TrimSpace(line.MenuItemID)
		if menuItemID == "" {
			menuItemID = line.MenuItemName
		}

		billItems = append(billItems, database.BillItem{
			// Assign a real UUID NOW so the JSON snapshot stored on
			// bills.items matches the relational bill_items row. The
			// synthetic order-line ID ("item-foo-1234") is not a valid UUID;
			// leaving it empty makes Postgres backfill the row but leaves
			// "" in the JSON, which breaks itemized splitting downstream.
			ID:                 database.NormalizeBillItemUUID(""),
			MenuItemID:         menuItemID,
			Name:               line.MenuItemName,
			Price:              line.Price,
			Quantity:           line.Quantity,
			Options:            line.Options,
			ItemType:           itemType,
			BundleID:           line.BundleID,
			ParentBundleID:     line.ParentBundleID,
			BundleOccurrenceID: line.BundleOccurrenceID,
			SourceOfferID:      line.SourceOfferID,
			Subtotal:           line.Subtotal,
			CreatedAt:          stamp,
		})
	}
	return billItems
}

func resolveMenuItemOptions(itemName string, selected []database.MenuItemOption, available []database.MenuItemOption) ([]database.MenuItemOption, error) {
	if len(selected) == 0 {
		return []database.MenuItemOption{}, nil
	}

	byID := make(map[string]database.MenuItemOption, len(available))
	byName := make(map[string]database.MenuItemOption, len(available))
	for _, option := range available {
		optionID := strings.TrimSpace(option.ID)
		if optionID != "" {
			byID[optionID] = option
		}
		nameKey := strings.ToLower(strings.TrimSpace(option.Name))
		if nameKey != "" {
			byName[nameKey] = option
		}
	}

	resolved := make([]database.MenuItemOption, 0, len(selected))
	for _, option := range selected {
		if match, ok := byID[strings.TrimSpace(option.ID)]; ok {
			resolved = append(resolved, match)
			continue
		}
		if match, ok := byName[strings.ToLower(strings.TrimSpace(option.Name))]; ok {
			resolved = append(resolved, match)
			continue
		}
		return nil, NewOrderValidationError(OrderErrCodeOptionNotFound,
			"option '%s' not found for menu item '%s'", option.Name, itemName)
	}

	return resolved, nil
}

// ApplyPromotionsToOrder expands bundle lines and appends discount lines from eligible offers.
func ApplyPromotionsToOrder(
	categories []database.MenuCategory,
	bundles []database.Bundle,
	offers []database.Offer,
	input []PromotionInputLine,
) (PromotionResult, error) {
	byID, byName := buildMenuLookups(categories)
	bundlesByID, bundlesByName := bundleMapByIDAndName(bundles)

	// B-4: bound the request before any per-line work.
	if len(input) > MaxOrderLines {
		return PromotionResult{}, NewOrderValidationError(OrderErrCodeTooManyItems,
			"order cannot contain more than %d lines", MaxOrderLines)
	}

	expanded := make([]database.OrderItem, 0, len(input)+8)
	billableIndexes := make([]int, 0, len(input))

	addBillableIndex := func() {
		billableIndexes = append(billableIndexes, len(expanded)-1)
	}

	for _, line := range input {
		quantity := line.Quantity
		lineLabel := promotionLineLabel(line)
		if quantity <= 0 {
			return PromotionResult{}, fmt.Errorf("quantity must be greater than zero for '%s'", lineLabel)
		}
		if quantity > MaxOrderItemQuantity {
			return PromotionResult{}, NewOrderValidationError(OrderErrCodeQuantityExceeded,
				"quantity for '%s' exceeds the maximum of %d per line", lineLabel, MaxOrderItemQuantity)
		}
		if utf8.RuneCountInString(line.SpecialRequests) > MaxOrderTextLen {
			return PromotionResult{}, NewOrderValidationError(OrderErrCodeTextTooLong,
				"special requests for '%s' exceed %d characters", lineLabel, MaxOrderTextLen)
		}

		itemType := normalizeItemType(line.ItemType)
		if itemType == OrderItemTypeBundle || line.BundleID != nil {
			var bundle database.Bundle
			foundBundle := false

			if line.BundleID != nil {
				if resolved, ok := bundlesByID[*line.BundleID]; ok {
					bundle = resolved
					foundBundle = true
				}
			} else {
				// Name lookup is only for id-less requests. A supplied bundle_id
				// is the identity — the client name cannot select a different bundle.
				nameKey := strings.ToLower(strings.TrimSpace(line.Name))
				if resolved, ok := bundlesByName[nameKey]; ok {
					bundle = resolved
					foundBundle = true
				}
			}
			if !foundBundle {
				return PromotionResult{}, NewOrderValidationError(OrderErrCodeBundleNotFound,
					"bundle '%s' not found", line.Name)
			}

			bundleID := bundle.ID
			parentSubtotalCents := majorToMinor(bundle.Price) * int64(quantity)
			parent := database.OrderItem{
				ID:            fmt.Sprintf("bundle-%d-%d", bundle.ID, time.Now().UnixNano()),
				ItemType:      OrderItemTypeBundle,
				MenuItemID:    fmt.Sprintf("bundle:%d", bundle.ID),
				BundleID:      &bundleID,
				MenuItemName:  bundle.Name,
				Quantity:      quantity,
				Price:         bundle.Price,
				Options:       []database.MenuItemOption{},
				Subtotal:      minorToMajor(parentSubtotalCents),
				SourceOfferID: line.SourceOfferID,
			}
			parent.BundleOccurrenceID = parent.ID
			expanded = append(expanded, parent)
			addBillableIndex()

			refs, err := normalizeBundleRefs(bundle.Items)
			if err != nil {
				return PromotionResult{}, fmt.Errorf("failed to parse bundle items for '%s': %w", bundle.Name, err)
			}
			for _, ref := range refs {
				refQty := ref.Quantity
				if refQty <= 0 {
					refQty = 1
				}

				refID := strings.TrimSpace(ref.MenuItemID)
				var resolved menuItemLookup
				ok := false
				if refID != "" {
					resolved, ok = byID[refID]
				} else {
					resolved, ok = byName[strings.ToLower(strings.TrimSpace(ref.Name))]
				}
				if !ok {
					label := strings.TrimSpace(ref.Name)
					if label == "" {
						label = refID
					}
					return PromotionResult{}, NewOrderValidationError(OrderErrCodeItemNotFound,
						"bundle item '%s' not found in menu", label)
				}
				if !resolved.Available {
					return PromotionResult{}, NewOrderValidationError(OrderErrCodeItemUnavailable,
						"menu item '%s' is currently unavailable", resolved.Name)
				}

				childQty := refQty * quantity
				child := database.OrderItem{
					ID:           fmt.Sprintf("bundle-item-%d-%s-%d", bundle.ID, resolved.ID, time.Now().UnixNano()),
					ItemType:     OrderItemTypeBundleItem,
					MenuItemID:   resolved.ID,
					MenuItemName: resolved.Name,
					Quantity:     childQty,
					// Catalog unit price for kitchen/costing display. Subtotal
					// stays 0 so Price×Qty paths never bill included components.
					Price:              resolved.Price,
					Options:            []database.MenuItemOption{},
					Subtotal:           0, // informational line, price accounted on parent bundle line
					BundleID:           &bundleID,
					ParentBundleID:     &bundleID,
					BundleOccurrenceID: parent.BundleOccurrenceID,
				}
				expanded = append(expanded, child)
			}

			continue
		}

		if itemType == OrderItemTypeDiscount {
			// Existing discount lines are ignored at request level because discounts are auto-computed.
			continue
		}

		menuItemID := strings.TrimSpace(line.MenuItemID)
		var resolved menuItemLookup
		found := false
		if menuItemID != "" {
			resolved, found = byID[menuItemID]
		}
		// Stale/forged IDs used to skip orderability because the quote keyed
		// the gate on the unresolved ID (#540). Fall back to catalog name.
		if !found {
			resolved, found = byName[strings.ToLower(strings.TrimSpace(line.Name))]
		}
		if !found {
			label := strings.TrimSpace(line.Name)
			if label == "" {
				label = menuItemID
			}
			if label == "" {
				return PromotionResult{}, errors.New("menu item name is required")
			}
			return PromotionResult{}, NewOrderValidationError(OrderErrCodeItemNotFound,
				"menu item '%s' not found", label)
		}
		// Request menu_item_name is discarded once the catalog line is resolved.
		name := strings.TrimSpace(resolved.Name)
		if name == "" {
			return PromotionResult{}, errors.New("menu item name is required")
		}
		if resolved.ID != "" {
			menuItemID = resolved.ID
		}
		// B-3: 86'd items are not orderable — uniform server-side reject for
		// guest AND operator creates (staff re-toggle availability if needed).
		if !resolved.Available {
			return PromotionResult{}, NewOrderValidationError(OrderErrCodeItemUnavailable,
				"menu item '%s' is currently unavailable", name)
		}

		resolvedOptions, err := resolveMenuItemOptions(name, line.Options, resolved.Options)
		if err != nil {
			return PromotionResult{}, err
		}

		unitPrice := resolved.Price

		unitPriceCents := majorToMinor(unitPrice)
		subtotalCents := (unitPriceCents + optionTotalCents(resolvedOptions)) * int64(quantity)
		item := database.OrderItem{
			ID:              fmt.Sprintf("item-%s-%d", strings.ToLower(strings.ReplaceAll(name, " ", "-")), time.Now().UnixNano()),
			ItemType:        OrderItemTypeMenuItem,
			MenuItemID:      menuItemID,
			MenuItemName:    name,
			Quantity:        quantity,
			Price:           unitPrice,
			Options:         resolvedOptions,
			SpecialRequests: line.SpecialRequests,
			Subtotal:        minorToMajor(subtotalCents),
			BundleID:        line.BundleID,
			ParentBundleID:  line.ParentBundleID,
			SourceOfferID:   line.SourceOfferID,
		}
		expanded = append(expanded, item)
		addBillableIndex()
	}

	var baseSubtotalCents int64
	for _, index := range billableIndexes {
		baseSubtotalCents += majorToMinor(expanded[index].Subtotal)
	}

	discountLines := make([]database.OrderItem, 0, len(offers))
	appliedOffers := make([]AppliedOffer, 0, len(offers))
	remainingCapCents := baseSubtotalCents
	now := time.Now()

	for _, offer := range offers {
		if remainingCapCents <= 0 {
			break
		}
		if !offer.IsActive {
			continue
		}
		if offer.StartDate != nil && offer.StartDate.After(now) {
			continue
		}
		if offer.EndDate != nil && offer.EndDate.Before(now) {
			continue
		}

		var eligibleSubtotalCents int64
		var eligibleQty int
		targetID := ""
		if offer.TargetID != nil {
			targetID = strings.TrimSpace(*offer.TargetID)
		}

		for _, index := range billableIndexes {
			line := expanded[index]
			lineSubtotalCents := majorToMinor(line.Subtotal)
			if lineSubtotalCents <= 0 {
				continue
			}

			match := false
			switch offer.ApplicableTo {
			case "all", "":
				match = true
			case "item":
				// Item offers match the resolved catalog id only. The request
				// (or persisted) display name is never an eligibility key.
				catalogID := strings.TrimSpace(line.MenuItemID)
				match = line.ItemType == OrderItemTypeMenuItem &&
					catalogID != "" && catalogID == targetID
			case "bundle":
				match = line.ItemType == OrderItemTypeBundle &&
					line.BundleID != nil &&
					strconv.FormatUint(uint64(*line.BundleID), 10) == targetID
			case "category":
				if line.ItemType != OrderItemTypeMenuItem {
					break
				}
				categoryKey := ""
				categoryName := ""
				if lookup, ok := byID[line.MenuItemID]; ok {
					categoryKey = lookup.Category
					categoryName = lookup.CategoryName
				} else if lookup, ok := byName[strings.ToLower(strings.TrimSpace(line.MenuItemName))]; ok {
					categoryKey = lookup.Category
					categoryName = lookup.CategoryName
				}
				// Match by the stable category ID (what the dashboard writes) or
				// the category name (what API callers may send as target_id).
				match = strings.EqualFold(strings.TrimSpace(categoryKey), targetID) ||
					(categoryName != "" && strings.EqualFold(strings.TrimSpace(categoryName), targetID))
			}

			if match {
				eligibleSubtotalCents += lineSubtotalCents
				if line.Quantity > 0 {
					eligibleQty += line.Quantity
				}
			}
		}

		if eligibleSubtotalCents <= 0 {
			continue
		}

		var rawDiscountCents int64
		switch offer.DiscountType {
		case "percentage":
			basisPoints := int64(math.Round(offer.DiscountValue * 100))
			if basisPoints < 0 {
				continue
			}
			if basisPoints > 10000 {
				basisPoints = 10000
			}
			rawDiscountCents = percentageDiscount(eligibleSubtotalCents, basisPoints)
		case "fixed":
			// A check-wide offer ("all", or the legacy empty scope the offer
			// form persisted by default) is "$X off the check" — one discount
			// per check. Only plate-scoped offers scale by qualifying quantity.
			if offer.ApplicableTo == "all" || offer.ApplicableTo == "" {
				rawDiscountCents = majorToMinor(offer.DiscountValue)
			} else {
				rawDiscountCents = majorToMinor(offer.DiscountValue) * int64(eligibleQty)
			}
		default:
			continue
		}

		if rawDiscountCents <= 0 {
			continue
		}

		appliedDiscountCents := rawDiscountCents
		if appliedDiscountCents > remainingCapCents {
			appliedDiscountCents = remainingCapCents
		}
		if appliedDiscountCents > eligibleSubtotalCents {
			appliedDiscountCents = eligibleSubtotalCents
		}
		if appliedDiscountCents <= 0 {
			continue
		}

		remainingCapCents -= appliedDiscountCents
		appliedDiscount := minorToMajor(appliedDiscountCents)
		offerID := offer.ID
		discountLine := database.OrderItem{
			ID:         fmt.Sprintf("discount-%d-%d", offer.ID, time.Now().UnixNano()),
			ItemType:   OrderItemTypeDiscount,
			MenuItemID: fmt.Sprintf("offer:%d", offer.ID),
			// Store plain offer name — localized "OFERTAS APLICADAS" heading
			// already carries context (NEW-13). Do not bake English "Offer:" in.
			MenuItemName:  offer.Name,
			Quantity:      1,
			Price:         -appliedDiscount,
			Options:       []database.MenuItemOption{},
			Subtotal:      -appliedDiscount,
			SourceOfferID: &offerID,
		}
		discountLines = append(discountLines, discountLine)
		appliedOffers = append(appliedOffers, AppliedOffer{
			OfferID: offer.ID,
			Name:    offer.Name,
			Amount:  appliedDiscount,
		})
	}

	lines := append(expanded, discountLines...)
	var discountTotalCents int64
	for _, line := range discountLines {
		discountTotalCents += -majorToMinor(line.Subtotal)
	}

	return PromotionResult{
		Lines:         lines,
		BaseSubtotal:  minorToMajor(baseSubtotalCents),
		DiscountTotal: minorToMajor(discountTotalCents),
		AppliedOffers: appliedOffers,
	}, nil
}

// ComputeBillTotals calculates tax/service/total from a post-discount subtotal.
func ComputeBillTotals(subtotal, taxRate, serviceFeeRate float64) (float64, float64, float64) {
	taxCents, serviceFeeCents, totalCents := BillTotalsCents(
		majorToMinor(subtotal),
		taxRate,
		serviceFeeRate,
	)
	return minorToMajor(taxCents), minorToMajor(serviceFeeCents), minorToMajor(totalCents)
}

// BillTotalsCents is the cent-exact companion to ComputeBillTotals for callers
// that already hold a canonical subtotal.
func BillTotalsCents(subtotalCents int64, taxRate, serviceFeeRate float64) (int64, int64, int64) {
	return money.BillTotalsCents(subtotalCents, taxRate, serviceFeeRate)
}
