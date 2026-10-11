package director_actions

import (
	"fmt"
	"math"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// currencySymbol maps an ISO 4217 code to a display glyph, falling back to the
// code itself (e.g. "AED", "THB") so proposal copy reads in the business's own
// currency instead of a hardcoded "$". (audit C6)
func currencySymbol(code string) string {
	switch code {
	case "USD":
		return "$"
	case "EUR":
		return "€"
	case "GBP":
		return "£"
	case "JPY", "CNY":
		return "¥"
	case "CAD":
		return "C$"
	case "AUD":
		return "A$"
	default:
		// Less common currencies (AED, THB, INR…) read as "AED 50.00".
		return code + " "
	}
}

// resolvePriceCurrency normalizes the optional variadic currency to a non-empty
// ISO code, defaulting to USD so existing callers (apply path, tests) are
// unaffected.
func resolvePriceCurrency(currency []string) string {
	if len(currency) > 0 && currency[0] != "" {
		return currency[0]
	}
	return "USD"
}

// ComputePriceChange deep-copies categories, applies the price change to the
// targeted items, enforces caps, and returns the mutated copy + a preview diff.
//
// Returns: (mutated, preview, warnings, requiresReconfirm, err)
//   - err is non-nil if any resulting price would be <= 0 (hard reject).
//   - requiresReconfirm is true if any per-item swing exceeds PriceReconfirmSwingPct.
//   - The input categories slice is never mutated.
//
// currency is an optional ISO 4217 code used only for human-readable preview
// copy (summary/error); when omitted it defaults to USD. (audit C6)
func ComputePriceChange(categories []database.MenuCategory, p PriceChangeParams, currency ...string) (
	[]database.MenuCategory, ActionPreview, []string, bool, error,
) {
	cur := resolvePriceCurrency(currency)
	copy := deepCopyCategories(categories)
	var warnings []string
	var requiresReconfirm bool
	var examples []PreviewExample
	affectedCount := 0

	for ci := range copy {
		cat := &copy[ci]
		inCategoryScope := p.Scope == "all" ||
			(strings.HasPrefix(p.Scope, "category:") && strings.TrimPrefix(p.Scope, "category:") == cat.ID)

		for ii := range cat.Items {
			item := &cat.Items[ii]
			inScope := inCategoryScope ||
				(strings.HasPrefix(p.Scope, "item:") && strings.TrimPrefix(p.Scope, "item:") == item.ID)
			if !inScope {
				continue
			}

			oldPrice := item.Price
			var newPrice float64
			switch p.Mode {
			case "percent":
				delta := oldPrice * (p.Value / 100.0)
				if p.Direction == "up" {
					newPrice = oldPrice + delta
				} else {
					newPrice = oldPrice - delta
				}
			case "flat":
				if p.Direction == "up" {
					newPrice = oldPrice + p.Value
				} else {
					newPrice = oldPrice - p.Value
				}
			}

			// Round to 2 decimal places.
			newPrice = math.Round(newPrice*100) / 100

			// Hard reject: resulting price <= 0.
			if newPrice <= 0 {
				return nil, ActionPreview{}, nil, false,
					fmt.Errorf("price change would result in price <= %s0 for item %q (%.2f → %.2f)", currencySymbol(cur), item.Name, oldPrice, newPrice)
			}

			// Per-item swing check for reconfirm.
			swing := math.Abs(newPrice-oldPrice) / oldPrice * 100
			if swing > PriceReconfirmSwingPct {
				requiresReconfirm = true
			}

			item.Price = newPrice
			affectedCount++

			if len(examples) < maxPreviewExamples {
				examples = append(examples, PreviewExample{
					Name:   item.Name,
					Before: oldPrice,
					After:  newPrice,
				})
			}
		}
	}

	if affectedCount == 0 {
		warnings = append(warnings, fmt.Sprintf("No items matched scope %q", p.Scope))
	}

	// Blast-radius gate (audit L4-16): a menu-wide price change touching more
	// than one item always needs explicit reconfirmation, whatever the swing —
	// so a scope the owner never asked for can't apply on a single click.
	if p.Scope == "all" && affectedCount > 1 {
		requiresReconfirm = true
	}

	preview := ActionPreview{
		AffectedCount: affectedCount,
		Examples:      examples,
		Summary:       priceChangeSummary(p, affectedCount, examples, cur),
	}
	return copy, preview, warnings, requiresReconfirm, nil
}

func priceChangeSummary(p PriceChangeParams, count int, examples []PreviewExample, currency string) string {
	if count == 0 {
		return "No items affected."
	}
	dir := "raised"
	if p.Direction == "down" {
		dir = "lowered"
	}
	var magnitude string
	if p.Mode == "percent" {
		magnitude = fmt.Sprintf("%.4g%%", p.Value)
	} else {
		magnitude = fmt.Sprintf("%s%.2f", currencySymbol(currency), p.Value)
	}
	scope := "all items"
	if strings.HasPrefix(p.Scope, "category:") {
		scope = fmt.Sprintf("category %s", strings.TrimPrefix(p.Scope, "category:"))
	} else if strings.HasPrefix(p.Scope, "item:") {
		scope = fmt.Sprintf("item %s", strings.TrimPrefix(p.Scope, "item:"))
	}
	return fmt.Sprintf("Prices %s by %s for %s (%d item(s) affected).", dir, magnitude, scope, count)
}

// PriceChangeTitle returns a deterministic English title for a price change
// proposal. currency is an optional ISO 4217 code for the magnitude glyph;
// defaults to USD. (audit C6)
func PriceChangeTitle(p PriceChangeParams, currency ...string) string {
	cur := resolvePriceCurrency(currency)
	dir := "Raise"
	if p.Direction == "down" {
		dir = "Lower"
	}
	var magnitude string
	if p.Mode == "percent" {
		magnitude = fmt.Sprintf("%.4g%%", p.Value)
	} else {
		magnitude = fmt.Sprintf("%s%.2f flat", currencySymbol(cur), p.Value)
	}
	scope := "all prices"
	if strings.HasPrefix(p.Scope, "category:") {
		scope = fmt.Sprintf("category %s prices", strings.TrimPrefix(p.Scope, "category:"))
	} else if strings.HasPrefix(p.Scope, "item:") {
		scope = fmt.Sprintf("item %s price", strings.TrimPrefix(p.Scope, "item:"))
	}
	return fmt.Sprintf("%s %s by %s", dir, scope, magnitude)
}

// PriceChangeTitleWithPreview uses the operator-facing item name from the
// dry-run examples instead of the internal menu item id (e.g. "Harvest Bowl"
// not "item demo-bowl").
func PriceChangeTitleWithPreview(p PriceChangeParams, preview ActionPreview, currency ...string) string {
	return replaceItemIDWithDisplayName(PriceChangeTitle(p, currency...), p.Scope, preview)
}

// PriceChangeDescription returns a deterministic English description for a price change preview.
func PriceChangeDescription(preview ActionPreview) string {
	if preview.AffectedCount == 0 {
		return "No items would be affected."
	}
	return fmt.Sprintf("Applies to %d item(s). %s", preview.AffectedCount, preview.Summary)
}
