package director_tools

import (
	"context"
	"fmt"

	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services/foodcost"
	"github.com/stdevmac/payverge/backend/internal/services/wastevariance"
)

// WasteVarianceTool reports inventory waste and theoretical-vs-actual usage
// variance for a day/week/month window. All five providers are test seams;
// production registers the tool bare and falls back to env.DB / env.Analytics.
type WasteVarianceTool struct {
	Items     foodcost.ItemStatsProvider
	Recipes   foodcost.RecipeCostProvider
	Movements wastevariance.MovementProvider
	Dims      wastevariance.ItemDimProvider
	Window    wastevariance.WindowProvider
}

// Name is the snake_case function identifier sent to the model.
func (t *WasteVarianceTool) Name() string { return "get_waste_variance" }

// HumanLabel is the localized pill label the UI shows while the tool runs.
func (t *WasteVarianceTool) HumanLabel(locale string) string {
	switch locale {
	case "es", "es_ar":
		return "Revisando tu desperdicio e inventario"
	case "fr":
		return "Vérification des pertes et écarts d'inventaire"
	case "ar":
		return "فحص هدر المخزون والتباينات"
	default:
		return "Checking your inventory waste & variance"
	}
}

// Description is the model-facing tool description sent to the provider.
func (t *WasteVarianceTool) Description() string {
	return "Reports inventory waste and usage variance for a period, valued at cost (all money is dollars). tracked_loss_cost is the total explicitly-tracked loss; loss_by_reason breaks it into reasons (one of: spoilage, count_shrink, manual). Per ingredient: theoretical_usage (what recipes×sales imply), actual_usage (what the inventory ledger shows), variance (actual − theoretical; positive = used more than sales predicted = shrinkage), variance_cost, and tracked_loss_cost. variance_measurable is false when no recipe links the ingredient to menu sales — then theoretical_usage, variance and variance_cost are ABSENT and no variance exists for that ingredient; actual_usage on its own is consumption, not variance. Never state a variance quantity that is not present in this result; read the guidance field. ingredients is capped to the worst-loss items (see limit); use it for 'how much am I wasting / where is my shrinkage' questions."
}

// Schema declares the argument shape the model sees.
func (t *WasteVarianceTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{
		Type: llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{
			"period": {
				Type:        llm.TypeString,
				Description: "Time window: day, week, or month. Default: week.",
				Enum:        []string{"day", "week", "month"},
			},
			"limit": {
				Type:        llm.TypeInteger,
				Description: "Number of worst-loss ingredients to return (1-50, default 5).",
			},
		},
	}
}

// Run resolves providers, validates args, runs the waste-variance calculator,
// and shapes the report into a ToolResult. Read-only; ProposalID stays zero.
func (t *WasteVarianceTool) Run(_ context.Context, args map[string]any, env ToolEnv) (ToolResult, error) {
	items := t.Items
	if items == nil && env.Analytics != nil {
		items = env.Analytics
	}
	recipes := t.Recipes
	if recipes == nil && env.DB != nil {
		recipes = env.DB
	}
	movements := t.Movements
	if movements == nil && env.DB != nil {
		movements = env.DB
	}
	dims := t.Dims
	if dims == nil && env.DB != nil {
		dims = env.DB
	}
	window := t.Window
	if window == nil && env.Analytics != nil {
		window = env.Analytics
	}
	if items == nil || recipes == nil || movements == nil || dims == nil || window == nil {
		return ToolResult{}, fmt.Errorf("get_waste_variance: providers not wired")
	}

	period, err := normalizeMenuPeriod("get_waste_variance", args)
	if err != nil {
		return ToolResult{}, err
	}
	limit, err := normalizeMenuLimit("get_waste_variance", args)
	if err != nil {
		return ToolResult{}, err
	}

	calc := wastevariance.NewCalculator(recipes, items, movements, dims, window)
	report, err := calc.Analyze(env.BusinessID, period, env.Location)
	if err != nil {
		return ToolResult{}, fmt.Errorf("get_waste_variance: %w", err)
	}

	// Cap the per-ingredient list to bound per-call token cost. The calculator
	// sorts Ingredients by tracked_loss_cost DESC so the slice surfaces the
	// worst-loss items. The report-level aggregates below are the full population.
	n := limit
	if n > len(report.Ingredients) {
		n = len(report.Ingredients)
	}
	ingredients := make([]map[string]any, 0, n)
	for _, iv := range report.Ingredients[:n] {
		row := map[string]any{
			"inventory_item_id":   iv.InventoryItemID,
			"name":                iv.Name,
			"unit":                iv.Unit,
			"actual_usage":        iv.ActualUsage,
			"tracked_loss_cost":   iv.TrackedLossCost,
			"has_recipe":          iv.HasRecipe,
			"variance_measurable": iv.HasRecipe,
		}
		if iv.HasRecipe {
			row["theoretical_usage"] = iv.TheoreticalUsage
			row["variance"] = iv.Variance
			row["variance_cost"] = iv.VarianceCost
		} else {
			// #872: an ingredient with no recipe has UNKNOWN theoretical usage,
			// not zero. Emitting theoretical_usage: 0 and variance: 0 next to a
			// real actual_usage handed the model an arithmetic invitation, and
			// it took it — reporting the raw consumption figure as a variance
			// on an ingredient whose variance cannot be computed at all. The
			// accounting UI already renders these cells as "—"; the model was
			// the only consumer still being shown fabricated zeroes.
			row["variance_unavailable_reason"] = "no recipe links this ingredient to a menu item, so theoretical usage is unknown and no variance can be computed for it"
		}
		ingredients = append(ingredients, row)
	}

	measurable := 0
	for _, iv := range report.Ingredients {
		if iv.HasRecipe {
			measurable++
		}
	}

	return ToolResult{
		Summary: fmt.Sprintf("$%.2f tracked loss; %d ingredients with variance", report.TrackedLossCost, len(report.Ingredients)),
		Data: map[string]any{
			"period":                 period,
			"tracked_loss_cost":      report.TrackedLossCost,
			"total_variance_cost":    report.TotalVarianceCost,
			"theoretical_usage_cost": report.TheoreticalUsageCost,
			"loss_by_reason":         report.LossByReason,
			"items_without_recipe":   report.ItemsWithoutRecipe,
			"measurable_ingredients": measurable,
			"sparse":                 report.Sparse,
			"ingredients":            ingredients,
			"guidance":               wasteVarianceGuidance(len(report.Ingredients), measurable, report.ItemsWithoutRecipe),
		},
	}, nil
}

// wasteVarianceGuidance is the anti-fabrication instruction that ships with
// every waste/variance result (#872).
//
// The calculator was never wrong — it simply had nothing to say about
// ingredients with no recipe, and silence read as zero. This states, in the
// tool result itself, when a variance figure exists and when stating one at
// all would be an invention.
func wasteVarianceGuidance(total, measurable, withoutRecipe int) string {
	const neverInvent = " Quote only variance numbers that appear in this result — never estimate, round up, or infer a quantity that is not here."

	if total == 0 {
		return "No ingredient recorded usage or loss in this window, so there is NO waste or variance to report. Say the data is not there yet." + neverInvent
	}
	if measurable == 0 {
		return fmt.Sprintf("None of the %d ingredients in this window has a recipe linking it to menu sales, so theoretical usage is unknown and NO variance can be computed for any of them. Say the variance cannot be calculated yet and that recipes are needed first — actual_usage is consumption, not variance.", total) + neverInvent
	}
	if withoutRecipe > 0 {
		return fmt.Sprintf("Variance is computable for %d of %d ingredients. State a variance figure ONLY for ingredients where variance_measurable is true; for the other %d there is no variance to report, only tracked loss and consumption.", measurable, total, withoutRecipe) + neverInvent
	}
	return fmt.Sprintf("Variance is computable for all %d ingredients in this window.", total) + neverInvent
}
