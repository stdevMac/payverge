package director_tools

import (
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// plateCostForItem returns the dollars-per-plate cost for a menu item.
// Recipe ingredient costs win; plate-level Cogs fills the gap. ok is false
// when no complete cost is on file — callers must not invent a margin.
func plateCostForItem(env ToolEnv, itemID, itemName string) (float64, bool) {
	if env.DB == nil {
		return 0, false
	}
	itemID = strings.TrimSpace(itemID)
	itemName = strings.TrimSpace(itemName)

	if rows, err := env.DB.GetRecipeCostsForBusiness(env.BusinessID); err == nil {
		var cost float64
		matched := false
		complete := true
		for _, r := range rows {
			if (itemID != "" && r.MenuItemID == itemID) || (itemName != "" && strings.EqualFold(r.MenuItemName, itemName)) {
				matched = true
				cost += r.QuantityRequired * r.CostPerUnit
				if !r.HasCost {
					complete = false
				}
			}
		}
		if matched && complete && cost > 0 {
			return cost, true
		}
	}

	if cogs, err := env.DB.GetMenuItemCogsForBusiness(env.BusinessID); err == nil {
		for _, c := range cogs {
			if c.UnitCost <= 0 {
				continue
			}
			if (itemID != "" && c.MenuItemID == itemID) || (itemName != "" && strings.EqualFold(c.MenuItemName, itemName)) {
				return c.UnitCost, true
			}
		}
	}
	return 0, false
}

// menuCardPrices returns live menu-card prices keyed by item ID and lowercased
// name. Used when a food-cost window has no sales so avg_price / food-cost %
// can still be computed from the card instead of being reported as missing.
func menuCardPrices(businessID uint) (byID, byName map[string]float64) {
	byID = map[string]float64{}
	byName = map[string]float64{}
	_, cats, err := database.GetMenuByBusinessID(businessID)
	if err != nil {
		return
	}
	for _, cat := range cats {
		for _, item := range cat.Items {
			if item.Price <= 0 {
				continue
			}
			if item.ID != "" {
				byID[item.ID] = item.Price
			}
			if name := strings.ToLower(strings.TrimSpace(item.Name)); name != "" {
				byName[name] = item.Price
			}
		}
	}
	return
}

func lookupMenuCardPrice(byID, byName map[string]float64, itemID, itemName string) float64 {
	if p := byID[itemID]; p > 0 {
		return p
	}
	if p := byName[strings.ToLower(strings.TrimSpace(itemName))]; p > 0 {
		return p
	}
	return 0
}

func previewFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}
