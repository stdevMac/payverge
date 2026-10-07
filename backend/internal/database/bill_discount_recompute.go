package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/money"

	"gorm.io/gorm"
)

// billItemOfferID resolves the offer a discount line was derived from, via
// source_offer_id or the legacy "offer:<id>" menu_item_id marker.
func billItemOfferID(item *BillItem) (uint, bool) {
	if item.ItemType != "discount" {
		return 0, false
	}
	if item.SourceOfferID != nil && *item.SourceOfferID != 0 {
		return *item.SourceOfferID, true
	}
	if rest, ok := strings.CutPrefix(strings.TrimSpace(item.MenuItemID), "offer:"); ok {
		if id, err := strconv.ParseUint(rest, 10, 64); err == nil && id != 0 {
			return uint(id), true
		}
	}
	return 0, false
}

func billLineIsDiscountBasis(item *BillItem) bool {
	return item.ItemType == "menu_item" || item.ItemType == "bundle"
}

// billLineCategoryLookup maps a bill line back to its menu category for
// category-targeted offers. Mirrors buildMenuLookups in the promotion pricing
// service: the key is the category ID when present, else the category name.
type billLineCategoryLookup func(menuItemID, name string) (categoryKey, categoryName string)

func buildBillCategoryLookupTx(tx *gorm.DB, businessID uint) (billLineCategoryLookup, error) {
	type categoryRef struct {
		key  string
		name string
	}
	byID := map[string]categoryRef{}
	byName := map[string]categoryRef{}

	var menu Menu
	if err := tx.Where("business_id = ? AND is_active = ?", businessID, true).First(&menu).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// No menu: category-targeted offers simply match nothing.
			return func(string, string) (string, string) { return "", "" }, nil
		}
		return nil, fmt.Errorf("failed to load menu for discount recompute: %w", err)
	}
	var categories []MenuCategory
	if strings.TrimSpace(menu.Categories) != "" {
		if err := json.Unmarshal([]byte(menu.Categories), &categories); err != nil {
			return nil, fmt.Errorf("failed to parse menu for discount recompute: %w", err)
		}
	}
	for _, category := range categories {
		name := strings.TrimSpace(category.Name)
		key := strings.TrimSpace(category.ID)
		if key == "" {
			key = name
		}
		ref := categoryRef{key: key, name: name}
		for _, item := range category.Items {
			if id := strings.TrimSpace(item.ID); id != "" {
				byID[id] = ref
			}
			if n := strings.ToLower(strings.TrimSpace(item.Name)); n != "" {
				byName[n] = ref
			}
		}
	}
	return func(menuItemID, name string) (string, string) {
		if ref, ok := byID[strings.TrimSpace(menuItemID)]; ok {
			return ref.key, ref.name
		}
		if ref, ok := byName[strings.ToLower(strings.TrimSpace(name))]; ok {
			return ref.key, ref.name
		}
		return "", ""
	}, nil
}

// offerMatchesBillLine mirrors the targeting rules in ApplyPromotionsToOrder so
// a recomputed discount uses the same eligibility as the original application.
func offerMatchesBillLine(offer *Offer, item *BillItem, categoryOf billLineCategoryLookup) bool {
	targetID := ""
	if offer.TargetID != nil {
		targetID = strings.TrimSpace(*offer.TargetID)
	}
	switch offer.ApplicableTo {
	case "all", "":
		return true
	case "item":
		catalogID := strings.TrimSpace(item.MenuItemID)
		return item.ItemType == "menu_item" && catalogID != "" && catalogID == targetID
	case "bundle":
		return item.ItemType == "bundle" &&
			item.BundleID != nil &&
			strconv.FormatUint(uint64(*item.BundleID), 10) == targetID
	case "category":
		if item.ItemType != "menu_item" {
			return false
		}
		key, name := categoryOf(item.MenuItemID, item.Name)
		return strings.EqualFold(strings.TrimSpace(key), targetID) ||
			(name != "" && strings.EqualFold(strings.TrimSpace(name), targetID))
	}
	return false
}

// offerScopeIsCheckWide reports whether the offer targets the whole check
// rather than specific plates. "" is the legacy scope the offer form persisted
// by default, so it is treated exactly like "all" (mirrors the eligibility
// switch above and ApplyPromotionsToOrder).
func offerScopeIsCheckWide(applicableTo string) bool {
	return applicableTo == "all" || applicableTo == ""
}

// offerRawDiscountCents mirrors the discount math in ApplyPromotionsToOrder.
// qualifyingQty is the sum of Quantity on matching billable lines; percentage
// offers ignore it because eligibleCents already scales with quantity, and
// check-wide fixed offers ignore it because "$X off the check" is one discount
// per check no matter how many plates qualify.
func offerRawDiscountCents(offer *Offer, eligibleCents int64, qualifyingQty int) int64 {
	switch offer.DiscountType {
	case "percentage":
		basisPoints := int64(math.Round(offer.DiscountValue * 100))
		if basisPoints < 0 {
			return 0
		}
		if basisPoints > 10000 {
			basisPoints = 10000
		}
		return (eligibleCents*basisPoints + 5000) / 10000
	case "fixed":
		if qualifyingQty <= 0 {
			return 0
		}
		if offerScopeIsCheckWide(offer.ApplicableTo) {
			return money.CentsFromMajor(offer.DiscountValue)
		}
		return money.CentsFromMajor(offer.DiscountValue) * int64(qualifyingQty)
	}
	return 0
}

// recomputeBillOfferDiscountsTx rewrites offer-derived discount lines against
// the current state of the lines they were computed from (L2-3: a derived
// discount must never stay frozen while the lines under it change).
//
// Each discount line recomputes within its own order cohort — the bill lines
// sharing its order_id. Offers are applied per order at pricing time, so
// re-deriving every line from the whole bill would double-count an offer that
// appears on several orders. Percentage offers re-derive from the cohort's
// eligible subtotal; plate-scoped fixed offers scale by qualifying quantity
// while check-wide ones stay at their configured value; all re-cap to
// what the cohort can still absorb, and a line whose discount falls to zero is
// dropped. Discount lines with no resolvable offer (legacy or manual) are left
// exactly as they are.
func recomputeBillOfferDiscountsTx(tx *gorm.DB, businessID uint, items []BillItem) ([]BillItem, error) {
	offerIDSet := map[uint]struct{}{}
	for i := range items {
		if id, ok := billItemOfferID(&items[i]); ok {
			offerIDSet[id] = struct{}{}
		}
	}
	if len(offerIDSet) == 0 {
		return items, nil
	}

	offerIDs := make([]uint, 0, len(offerIDSet))
	for id := range offerIDSet {
		offerIDs = append(offerIDs, id)
	}
	var offerRows []Offer
	if err := tx.Where("business_id = ? AND id IN ?", businessID, offerIDs).Find(&offerRows).Error; err != nil {
		return nil, fmt.Errorf("failed to load offers for discount recompute: %w", err)
	}
	offers := make(map[uint]*Offer, len(offerRows))
	needsCategories := false
	for i := range offerRows {
		offers[offerRows[i].ID] = &offerRows[i]
		if offerRows[i].ApplicableTo == "category" {
			needsCategories = true
		}
	}

	categoryOf := billLineCategoryLookup(func(string, string) (string, string) { return "", "" })
	if needsCategories {
		lookup, err := buildBillCategoryLookupTx(tx, businessID)
		if err != nil {
			return nil, err
		}
		categoryOf = lookup
	}

	cohortKey := func(orderID *uint) uint {
		if orderID == nil {
			return 0
		}
		return *orderID
	}

	// Remaining discountable capacity per cohort, consumed in bill-line order so
	// stacked offers can never discount more than the cohort's items are worth.
	remainingCap := map[uint]int64{}
	for i := range items {
		if !billLineIsDiscountBasis(&items[i]) {
			continue
		}
		if cents := money.CentsFromMajor(items[i].Subtotal); cents > 0 {
			remainingCap[cohortKey(items[i].OrderID)] += cents
		}
	}

	result := make([]BillItem, 0, len(items))
	for i := range items {
		item := items[i]
		offerID, traceable := billItemOfferID(&item)
		if !traceable {
			result = append(result, item)
			continue
		}
		cohort := cohortKey(item.OrderID)

		offer, known := offers[offerID]
		if !known {
			// The offer row is gone (or belongs to another business): the frozen
			// amount cannot be re-derived. Keep the line but still consume cohort
			// capacity so later recomputed lines cannot overdraw the bill.
			consumed := -money.CentsFromMajor(item.Subtotal)
			if consumed > 0 {
				if cap := remainingCap[cohort]; consumed > cap {
					consumed = cap
				}
				remainingCap[cohort] -= consumed
			}
			result = append(result, item)
			continue
		}

		var eligibleCents int64
		var eligibleQty int
		for j := range items {
			line := &items[j]
			if !billLineIsDiscountBasis(line) || cohortKey(line.OrderID) != cohort {
				continue
			}
			cents := money.CentsFromMajor(line.Subtotal)
			if cents <= 0 {
				continue
			}
			if offerMatchesBillLine(offer, line, categoryOf) {
				eligibleCents += cents
				if line.Quantity > 0 {
					eligibleQty += line.Quantity
				}
			}
		}

		discountCents := offerRawDiscountCents(offer, eligibleCents, eligibleQty)
		if discountCents > eligibleCents {
			discountCents = eligibleCents
		}
		if cap := remainingCap[cohort]; discountCents > cap {
			discountCents = cap
		}
		if discountCents <= 0 {
			// Nothing left to discount in this cohort — drop the line.
			continue
		}
		remainingCap[cohort] -= discountCents

		amount := money.MajorFromCents(discountCents)
		item.Price = -amount
		item.Subtotal = -amount
		item.Quantity = 1
		result = append(result, item)
	}

	return result, nil
}
