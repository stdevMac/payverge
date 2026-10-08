package director_tools

import (
	"context"
	"fmt"

	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services/foodcost"
	"github.com/stdevmac/payverge/backend/internal/services/menuengineering"
)

// MenuEngineeringTool classifies every recipe-mapped menu item into the classic
// menu-engineering quadrants (Star / Plowhorse / Puzzle / Dog) from margin
// (food cost %) and popularity (units sold). The model uses it to answer "which
// dishes should I reprice, promote, or cut", then chains propose_price_change
// using a plowhorse's suggested_price to act.
//
// Items/Recipes are test seams; production registers the tool bare and falls
// back to env.Analytics (items) and env.DB (recipes).
type MenuEngineeringTool struct {
	Items   foodcost.ItemStatsProvider
	Recipes foodcost.RecipeCostProvider
}

// Name is the snake_case function identifier sent to the model.
func (t *MenuEngineeringTool) Name() string { return "get_menu_engineering" }

// HumanLabel is the localized pill label the UI shows while the tool runs.
func (t *MenuEngineeringTool) HumanLabel(locale string) string {
	switch locale {
	case "es", "es_ar":
		return "Analizando el menú por margen y popularidad"
	case "fr":
		return "Analyse du menu par marge et popularité"
	case "ar":
		return "تحليل القائمة حسب الهامش والشعبية"
	default:
		return "Mapping your menu by margin and popularity"
	}
}

// Description is the model-facing tool description sent to the provider.
func (t *MenuEngineeringTool) Description() string {
	return "Classifies every recipe-mapped menu item into menu-engineering quadrants from margin (food cost %) and popularity (units sold): Star (high margin + popular), Plowhorse (low margin + popular), Puzzle (high margin + unpopular), Dog (low margin + unpopular). Each dish carries recommended_action (one of: protect, reprice_up, promote, cut) and suggested_price (a target price; non-zero only for plowhorses, 0 elsewhere). dishes is capped to the worst-margin items (see limit); quadrant_counts is the full-population tally. Call for 'which dishes should I reprice/promote/cut' or menu-profitability questions. To act on a plowhorse, chain into propose_price_change with its suggested_price."
}

// Schema declares the argument shape the model sees.
func (t *MenuEngineeringTool) Schema() *llm.JSONSchema {
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
				Description: "Number of dishes to return (1-50), worst-margin first. Default: 5.",
			},
		},
	}
}

// Run computes the food-cost report, classifies it into quadrants, and shapes
// the per-quadrant counts plus the per-dish list for the model. Read-only:
// ProposalID stays zero.
func (t *MenuEngineeringTool) Run(_ context.Context, args map[string]any, env ToolEnv) (ToolResult, error) {
	items := t.Items
	if items == nil && env.Analytics != nil {
		items = env.Analytics
	}
	recipes := t.Recipes
	if recipes == nil && env.DB != nil {
		recipes = env.DB
	}
	if items == nil || recipes == nil {
		return ToolResult{}, fmt.Errorf("get_menu_engineering: providers not wired")
	}

	period, err := normalizeMenuPeriod("get_menu_engineering", args)
	if err != nil {
		return ToolResult{}, err
	}
	limit, err := normalizeMenuLimit("get_menu_engineering", args)
	if err != nil {
		return ToolResult{}, err
	}

	calc := foodcost.NewCalculator(recipes, items)
	report, err := calc.Analyze(env.BusinessID, period, env.Location)
	if err != nil {
		return ToolResult{}, fmt.Errorf("get_menu_engineering: %w", err)
	}
	me := menuengineering.Classify(report)

	// Cap the per-dish list to bound per-call token cost. Classify orders Dishes
	// worst-food-cost-first, so the capped slice surfaces the highest-food-cost
	// act-on candidates (plowhorses/dogs). quadrant_counts below stays the
	// full-population tally regardless of the cap.
	dishes := make([]map[string]any, 0, limit)
	for _, d := range me.Dishes {
		if len(dishes) >= limit {
			break
		}
		dishes = append(dishes, map[string]any{
			"menu_item_id":       d.MenuItemID,
			"name":               d.MenuItemName,
			"quadrant":           string(d.Quadrant),
			"recommended_action": d.Action,
			"food_cost_pct":      d.FoodCostPct,
			"qty_sold":           d.QtySold,
			"avg_price":          d.AvgPrice,
			"suggested_price":    d.SuggestedPrice,
		})
	}
	counts := map[string]int{}
	for _, r := range me.Rollups {
		counts[string(r.Quadrant)] = r.Count
	}

	return ToolResult{
		Summary: fmt.Sprintf("%d stars, %d plowhorses, %d puzzles, %d dogs across recipe-mapped items",
			counts["star"], counts["plowhorse"], counts["puzzle"], counts["dog"]),
		// All *_pct values are 0..1 fractions on one uniform scale (consistent
		// with get_food_cost_analysis) so the model never mis-compares scales.
		Data: map[string]any{
			"period":               period,
			"median_food_cost_pct": me.MedianFoodCostPct,
			"median_qty_sold":      me.MedianQtySold,
			"quadrant_counts":      counts,
			"items_needing_cost":   me.ItemsNeedingCost,
			"sparse":               me.Sparse,
			"dishes":               dishes,
		},
	}, nil
}
