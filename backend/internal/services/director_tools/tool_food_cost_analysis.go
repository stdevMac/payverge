package director_tools

import (
	"context"
	"fmt"
	"sort"

	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services/foodcost"
)

// defaultFoodCostThresholdPct is the food-cost fraction (0..1) above which a
// dish is flagged "high" when the caller omits threshold_pct. 0.35 == 35%.
const defaultFoodCostThresholdPct = 0.35

// FoodCostAnalysisTool returns per-dish food-cost %, unit cost, and gross
// margin for menu items that have a recipe, plus a sales-weighted blended
// food-cost % for the window. The model uses it to answer "is my food cost
// high?" and to pick repricing candidates, then chains into
// propose_price_change to act on a flagged item.
//
// Items/Recipes are test seams; production registers the tool bare and falls
// back to env.Analytics (items) and env.DB (recipes).
type FoodCostAnalysisTool struct {
	Items   foodcost.ItemStatsProvider
	Recipes foodcost.RecipeCostProvider
}

// Name is the snake_case function identifier sent to the model.
func (t *FoodCostAnalysisTool) Name() string { return "get_food_cost_analysis" }

// HumanLabel is the localized pill label the UI shows while the tool runs.
func (t *FoodCostAnalysisTool) HumanLabel(locale string) string {
	switch locale {
	case "es", "es_ar":
		return "Analizando el costo de insumos por plato"
	case "fr":
		return "Analyse du coût des ingrédients par plat"
	case "ar":
		return "تحليل تكلفة المكونات لكل طبق"
	default:
		return "Analyzing food cost by dish"
	}
}

// Description is the model-facing tool description sent to the provider.
func (t *FoodCostAnalysisTool) Description() string {
	return "Returns per-dish food cost % (ingredient cost ÷ sale price), unit cost, and gross margin for menu items that have a recipe, plus a sales-weighted blended food cost % for the window. Call for 'is my food cost high', margin/profitability questions, or repricing candidates. To act on a flagged item, chain into propose_price_change."
}

// Schema declares the argument shape the model sees.
func (t *FoodCostAnalysisTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{
		Type: llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{
			"period": {
				Type:        llm.TypeString,
				Description: "Time window. One of: day, week, month. Default: week.",
				Enum:        []string{"day", "week", "month"},
			},
			"limit": {
				Type:        llm.TypeInteger,
				Description: "Number of worst-offender items to return (1-50). Default: 5.",
			},
			"threshold_pct": {
				Type:        llm.TypeNumber,
				Description: "Food-cost % (0-100) above which an item is 'high'. Default: 35.",
			},
		},
	}
}

// Run validates args, computes the food-cost report via foodcost.Calculator,
// and shapes the worst-offenders list plus an over-threshold count for the
// model. Read-only: ProposalID stays zero.
func (t *FoodCostAnalysisTool) Run(_ context.Context, args map[string]any, env ToolEnv) (ToolResult, error) {
	items := t.Items
	if items == nil && env.Analytics != nil {
		items = env.Analytics
	}
	recipes := t.Recipes
	if recipes == nil && env.DB != nil {
		recipes = env.DB
	}
	if items == nil || recipes == nil {
		return ToolResult{}, fmt.Errorf("get_food_cost_analysis: providers not wired")
	}

	period, err := normalizeMenuPeriod("get_food_cost_analysis", args)
	if err != nil {
		return ToolResult{}, err
	}
	limit, err := normalizeMenuLimit("get_food_cost_analysis", args)
	if err != nil {
		return ToolResult{}, err
	}
	// threshold_pct arrives as a 0-100 percentage; store it as a 0..1 fraction
	// so it lives on the same scale as report.BlendedFoodCostPct and each item's
	// FoodCostPct. The model sends numbers as float64 via JSON; mirror
	// normalizeMenuLimit's wider coercion (int/int64/float32/float64).
	threshold := defaultFoodCostThresholdPct
	if raw, ok := args["threshold_pct"]; ok {
		switch v := raw.(type) {
		case int:
			threshold = float64(v) / 100.0
		case int64:
			threshold = float64(v) / 100.0
		case float32:
			threshold = float64(v) / 100.0
		case float64:
			threshold = v / 100.0
		default:
			return ToolResult{}, fmt.Errorf("get_food_cost_analysis: threshold_pct must be a number, got %T", raw)
		}
		if threshold < 0 || threshold > 1.0 {
			return ToolResult{}, fmt.Errorf("get_food_cost_analysis: threshold_pct out of range (0-100)")
		}
	}

	calc := foodcost.NewCalculator(recipes, items)
	report, err := calc.Analyze(env.BusinessID, period, env.Location)
	if err != nil {
		return ToolResult{}, fmt.Errorf("get_food_cost_analysis: %w", err)
	}

	// Complete plate cost is enough. When the window has no sales, fill
	// avg_price from the live menu card so food-cost % and unit margin are
	// still available instead of an empty items list.
	priceByID, priceByName := menuCardPrices(env.BusinessID)
	type scored struct {
		id, name           string
		unitCost, avgPrice float64
		foodPct, margin    float64
		qty                int
	}
	rows := make([]scored, 0, len(report.Items))
	for _, m := range report.Items {
		if !m.HasCompleteCost || m.UnitCost <= 0 {
			continue
		}
		avg := m.AvgPrice
		if avg <= 0 {
			avg = lookupMenuCardPrice(priceByID, priceByName, m.MenuItemID, m.MenuItemName)
		}
		if avg <= 0 {
			continue
		}
		rows = append(rows, scored{
			id:       m.MenuItemID,
			name:     m.MenuItemName,
			unitCost: m.UnitCost,
			avgPrice: avg,
			foodPct:  m.UnitCost / avg,
			margin:   avg - m.UnitCost,
			qty:      m.QtySold,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].foodPct != rows[j].foodPct {
			return rows[i].foodPct > rows[j].foodPct
		}
		return rows[i].margin < rows[j].margin
	})
	overThreshold := 0
	worst := make([]map[string]any, 0, limit)
	for _, r := range rows {
		if r.foodPct > threshold {
			overThreshold++
		}
		if len(worst) < limit {
			worst = append(worst, map[string]any{
				"menu_item_id":    r.id,
				"name":            r.name,
				"unit_cost":       r.unitCost,
				"avg_price":       r.avgPrice,
				"food_cost_pct":   r.foodPct,
				"margin_per_unit": r.margin,
				"qty_sold":        r.qty,
			})
		}
	}

	return ToolResult{
		Summary: fmt.Sprintf("Blended food cost %.0f%% across recipe-mapped items", report.BlendedFoodCostPct*100),
		// All *_pct values (blended_food_cost_pct, threshold_pct, and each
		// item's food_cost_pct) are fractions in 0..1 on a single uniform scale
		// so the model never mis-compares a 0.65 food cost against a 35 cutoff.
		Data: map[string]any{
			"period":                period,
			"blended_food_cost_pct": report.BlendedFoodCostPct,
			"estimated_cogs":        report.EstimatedCOGS,
			"total_revenue":         report.TotalRevenue,
			"items_missing_cost":    report.ItemsMissingCost,
			"items_without_recipe":  report.ItemsWithoutRecipe,
			"threshold_pct":         threshold,
			"items_over_threshold":  overThreshold,
			"items":                 worst,
		},
	}, nil
}
