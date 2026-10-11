// Package foodcost computes a read-only food-cost / gross-margin lens by
// joining recipe ingredient costs with analytics sale prices. It persists
// nothing and does not change the accounting P&L formula — food-cost is a
// display metric, not a deduction.
package foodcost

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"
)

// ErrUnsupportedPeriod marks an invalid period argument (client error, maps to 400).
// DB/infrastructure failures from Analyze are NOT this error and should map to 500.
var ErrUnsupportedPeriod = errors.New("foodcost: unsupported period")

// ItemStatsProvider is the analytics seam (satisfied by *analytics.AnalyticsService).
type ItemStatsProvider interface {
	GetPopularItems(businessID uint, limit int, period string, loc *time.Location) ([]analytics.ItemStats, error)
}

// ItemStatsWindowProvider is the optional date-bounded seam for custom start/end
// windows (accounting Overview date range). *analytics.AnalyticsService implements it.
type ItemStatsWindowProvider interface {
	GetPopularItemsInWindow(businessID uint, limit int, start, end time.Time) ([]analytics.ItemStats, error)
}

// RecipeCostProvider is the recipe-cost seam (satisfied by *database.DB).
type RecipeCostProvider interface {
	GetRecipeCostsForBusiness(businessID uint) ([]database.RecipeIngredientCost, error)
}

// MenuCogsProvider is the optional plate-level COGS seam (MenuItem.Cogs).
// Satisfied by *database.DB. When absent, only recipe costs are used.
type MenuCogsProvider interface {
	GetMenuItemCogsForBusiness(businessID uint) ([]database.MenuItemCogs, error)
}

// ItemMargin is the per-dish food-cost row. Money fields are dollars.
type ItemMargin struct {
	MenuItemID      string  `json:"menu_item_id"`
	MenuItemName    string  `json:"menu_item_name"`
	UnitCost        float64 `json:"unit_cost"`
	AvgPrice        float64 `json:"avg_price"`
	FoodCostPct     float64 `json:"food_cost_pct"` // 0..1; 0 when AvgPrice == 0
	MarginPerUnit   float64 `json:"margin_per_unit"`
	QtySold         int     `json:"qty_sold"`
	HasCompleteCost bool    `json:"has_complete_cost"`
}

// Report is the aggregate food-cost view for one business+period.
type Report struct {
	Period             string       `json:"period"`
	Items              []ItemMargin `json:"items"` // recipe-mapped items, worst-first
	BlendedFoodCostPct float64      `json:"blended_food_cost_pct"`
	EstimatedCOGS      float64      `json:"estimated_cogs"`
	TotalRevenue       float64      `json:"total_revenue"`
	ItemsMissingCost   int          `json:"items_missing_cost"`
	ItemsWithoutRecipe int          `json:"items_without_recipe"`
	// HasSales is true when the analytics window recorded recognized quantity
	// above zero, including zero-price bundle components. When false, per-item
	// sale figures (avg_price, qty_sold, margin) are structurally zero —
	// surfaces must say "no sales this period" instead of presenting cost
	// math over nothing (#797).
	HasSales bool `json:"has_sales"`
}

// Calculator is the single source of truth for food-cost math.
type Calculator struct {
	recipes RecipeCostProvider
	items   ItemStatsProvider
}

func NewCalculator(recipes RecipeCostProvider, items ItemStatsProvider) *Calculator {
	return &Calculator{recipes: recipes, items: items}
}

// Analyze computes the food-cost report for a business over a period
// (day/week/month). loc aligns the analytics window to the business calendar;
// nil is treated as UTC by the analytics service.
func (c *Calculator) Analyze(businessID uint, period string, loc *time.Location) (Report, error) {
	switch period {
	case "":
		period = "week"
	case "day", "week", "month":
	default:
		return Report{}, fmt.Errorf("%w %q (allowed: day, week, month)", ErrUnsupportedPeriod, period)
	}

	// Analytics uses "today" for the day window.
	analyticsPeriod := period
	if period == "day" {
		analyticsPeriod = "today"
	}

	stats, err := c.items.GetPopularItems(businessID, 0, analyticsPeriod, loc)
	if err != nil {
		return Report{}, fmt.Errorf("foodcost: popular items: %w", err)
	}
	return c.analyzeWithStats(businessID, period, stats)
}

// AnalyzeWindow computes the food-cost report for an explicit half-open
// [start, end) window (same SQL shape as period presets). Requires the items
// provider to implement ItemStatsWindowProvider.
func (c *Calculator) AnalyzeWindow(businessID uint, start, end time.Time) (Report, error) {
	if !end.After(start) {
		return Report{}, fmt.Errorf("%w: end must be after start", ErrUnsupportedPeriod)
	}
	wp, ok := c.items.(ItemStatsWindowProvider)
	if !ok {
		return Report{}, errors.New("foodcost: items provider does not support custom windows")
	}
	stats, err := wp.GetPopularItemsInWindow(businessID, 0, start, end)
	if err != nil {
		return Report{}, fmt.Errorf("foodcost: popular items window: %w", err)
	}
	label := start.UTC().Format("2006-01-02") + "/" + end.UTC().Format("2006-01-02")
	return c.analyzeWithStats(businessID, label, stats)
}

func (c *Calculator) analyzeWithStats(businessID uint, period string, stats []analytics.ItemStats) (Report, error) {
	recipeRows, err := c.recipes.GetRecipeCostsForBusiness(businessID)
	if err != nil {
		return Report{}, fmt.Errorf("foodcost: recipe costs: %w", err)
	}

	// Aggregate ingredient lines into a per-menu-item unit cost.
	type agg struct {
		name     string
		unitCost float64
		complete bool
	}
	perItem := make(map[string]*agg)
	for _, r := range recipeRows {
		a := perItem[r.MenuItemID]
		if a == nil {
			a = &agg{name: r.MenuItemName, complete: true}
			perItem[r.MenuItemID] = a
		}
		if r.MenuItemName != "" {
			a.name = r.MenuItemName
		}
		a.unitCost += r.QuantityRequired * r.CostPerUnit
		if !r.HasCost {
			a.complete = false
		}
	}

	// Plate-level COGS (MenuItem.Cogs) fills coverage gaps for dishes that
	// have no inventory recipe yet. Recipe-mapped items win — never overwrite
	// a recipe-derived unit cost with the simpler plate field.
	if mcp, ok := c.recipes.(MenuCogsProvider); ok {
		manual, mErr := mcp.GetMenuItemCogsForBusiness(businessID)
		if mErr != nil {
			return Report{}, fmt.Errorf("foodcost: menu cogs: %w", mErr)
		}
		for _, m := range manual {
			if m.UnitCost <= 0 {
				continue
			}
			if _, hasRecipe := perItem[m.MenuItemID]; hasRecipe {
				continue
			}
			perItem[m.MenuItemID] = &agg{
				name:     m.MenuItemName,
				unitCost: m.UnitCost,
				complete: true,
			}
		}
	}

	// analytics.GetPopularItems keys ItemStats by bill_items.name ("Use name
	// as ID since we aggregated by name"), while recipes key by MenuItemID —
	// so an ID-only lookup never matches in production and the whole report
	// reads 0% / $0. Match by ID first (tests, any future ID-keyed source),
	// then fall back to the recipe's MenuItemName.
	statByID := make(map[string]analytics.ItemStats, len(stats))
	for _, s := range stats {
		statByID[s.ItemID] = s
	}
	// A stat is CLAIMED by the first recipe item that resolves to it, so two
	// distinct MenuItemIDs sharing a display name (analytics aggregates
	// bill_items by name, emitting ONE name-keyed stat) can't both absorb the
	// same sales into the blended COGS/revenue — that would double-count the dish
	// and push food-cost total_revenue above the analytics revenue for the window.
	// Later duplicates report zero sold (they still appear as items). matchedStatKeys
	// doubles as the "this stat had a recipe" set for the coverage-gap count below.
	matchedStatKeys := make(map[string]bool, len(stats))
	lookupStat := func(itemID, itemName string) (analytics.ItemStats, bool) {
		if s, ok := statByID[itemID]; ok && !matchedStatKeys[itemID] {
			matchedStatKeys[itemID] = true
			return s, true
		}
		if s, ok := statByID[itemName]; ok && !matchedStatKeys[itemName] {
			matchedStatKeys[itemName] = true
			return s, true
		}
		return analytics.ItemStats{}, false
	}

	// Items is initialized non-nil so an empty report (a business with no
	// recipe-mapped items) serializes as "items":[] rather than "items":null.
	// The FE reads .length over it, and a null crashes the whole accounting tab.
	// Mirrors the wastevariance calculator's Ingredients contract.
	report := Report{Period: period, Items: []ItemMargin{}}
	// Any recognized quantity makes it a sales window, including a zero-price
	// bundle component whose revenue sits on the combo parent.
	for _, s := range stats {
		if s.RecognizedQuantity > 0 {
			report.HasSales = true
			break
		}
	}
	var weightedCost, weightedRevenue float64
	// Iterate MenuItemIDs in a stable order so the single-claim of a shared
	// name-keyed stat is deterministic (map order is randomized in Go).
	itemIDs := make([]string, 0, len(perItem))
	for itemID := range perItem {
		itemIDs = append(itemIDs, itemID)
	}
	sort.Strings(itemIDs)
	for _, itemID := range itemIDs {
		a := perItem[itemID]
		s, sold := lookupStat(itemID, a.name)
		m := ItemMargin{
			MenuItemID:      itemID,
			MenuItemName:    a.name,
			UnitCost:        a.unitCost,
			HasCompleteCost: a.complete,
		}
		recognizedQty := 0.0
		if sold {
			m.AvgPrice = s.AveragePrice
			m.QtySold = s.TotalSold
			recognizedQty = s.RecognizedQuantity
		}
		// Margin only exists where a sale price exists. An unsold item has
		// AvgPrice 0 = "no sales", not "sold for free" — deriving
		// MarginPerUnit = 0 - unitCost painted negative margins across
		// no-sales windows (#797).
		if m.AvgPrice > 0 {
			m.MarginPerUnit = m.AvgPrice - a.unitCost
			m.FoodCostPct = a.unitCost / m.AvgPrice
		}
		report.Items = append(report.Items, m)

		// A complete recipe contributes ingredient cost for every recognized
		// unit, even when the line's own price is 0 (combo component). Revenue
		// still requires a positive average price and uses the float quantity.
		if a.complete && recognizedQty > 0 {
			weightedCost += a.unitCost * recognizedQty
			if m.AvgPrice > 0 {
				weightedRevenue += m.AvgPrice * recognizedQty
			}
		}
	}

	// Items that sold but have no recipe at all → coverage gap. They still
	// belong in total_revenue so the headline % is of all sales, not the
	// recipe-mapped slice (regression of #181 / #490).
	for _, s := range stats {
		if matchedStatKeys[s.ItemID] {
			continue
		}
		report.ItemsWithoutRecipe++
		name := s.ItemName
		if name == "" {
			name = s.ItemID
		}
		m := ItemMargin{
			MenuItemID:      s.ItemID,
			MenuItemName:    name,
			AvgPrice:        s.AveragePrice,
			QtySold:         s.TotalSold,
			HasCompleteCost: false,
		}
		if m.AvgPrice > 0 {
			m.FoodCostPct = 0
			m.MarginPerUnit = m.AvgPrice
		}
		report.Items = append(report.Items, m)
		if s.RecognizedQuantity > 0 && m.AvgPrice > 0 {
			weightedRevenue += m.AvgPrice * s.RecognizedQuantity
		}
	}

	// Sold SKUs with unit cost 0 or unknown: incomplete recipe, no recipe,
	// or a computed unitCost <= 0. Unsold rows (Salad) do not increment.
	// ItemsWithoutRecipe stays a separate coverage-gap counter.
	for _, it := range report.Items {
		if it.QtySold > 0 && (!it.HasCompleteCost || it.UnitCost <= 0) {
			report.ItemsMissingCost++
		}
	}

	report.EstimatedCOGS = weightedCost
	report.TotalRevenue = weightedRevenue
	if weightedRevenue > 0 {
		report.BlendedFoodCostPct = weightedCost / weightedRevenue
	}

	sort.SliceStable(report.Items, func(i, j int) bool {
		if report.Items[i].FoodCostPct != report.Items[j].FoodCostPct {
			return report.Items[i].FoodCostPct > report.Items[j].FoodCostPct
		}
		return report.Items[i].MenuItemID < report.Items[j].MenuItemID
	})
	return report, nil
}
