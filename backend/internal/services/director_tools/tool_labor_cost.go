package director_tools

import (
	"context"
	"fmt"

	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services/foodcost"
	"github.com/stdevmac/payverge/backend/internal/services/labor"
)

// LaborCostTool reports labor cost as a % of sales and composes prime cost
// (food-cost% + labor%) for a week|month window. All seams are test seams;
// production registers the tool bare and falls back to env.DB / env.Analytics.
type LaborCostTool struct {
	Payroll labor.PayrollProvider
	Sales   labor.SalesProvider
	Window  labor.WindowProvider
	Recipes foodcost.RecipeCostProvider
	Items   foodcost.ItemStatsProvider
}

func (t *LaborCostTool) Name() string { return "get_labor_cost" }

func (t *LaborCostTool) HumanLabel(locale string) string {
	switch locale {
	case "es", "es_ar":
		return "Revisando tu costo laboral y prime cost"
	case "fr":
		return "Vérification du coût de main-d'œuvre et du prime cost"
	case "ar":
		return "فحص تكلفة العمالة والتكلفة الأساسية"
	default:
		return "Checking your labor & prime cost"
	}
}

func (t *LaborCostTool) Description() string {
	return "Reports labor cost as a percentage of sales for a period (all money is dollars). labor_cost is prorated paid-payroll spend over the window; net_sales is revenue excluding tips; labor_cost_pct = labor_cost / net_sales (0 when there are no sales). food_cost_pct is the blended food cost ratio; prime_cost_pct = labor_cost_pct + food_cost_pct (the operator's prime-cost heuristic — approximate when recipe coverage is sparse, since food_cost_pct is measured over recipe-mapped sales only while labor_cost_pct is over total net sales). has_data is false when no paid payroll runs overlap the window — say so rather than reporting 0% labor. Use for 'what's my labor cost / prime cost / am I overstaffed' questions. period is week or month only (labor cannot be resolved for a single day)."
}

func (t *LaborCostTool) Schema() *llm.JSONSchema {
	return &llm.JSONSchema{
		Type: llm.TypeObject,
		Properties: map[string]*llm.JSONSchema{
			"period": {
				Type:        llm.TypeString,
				Description: "Time window: week or month. Default: week.",
				Enum:        []string{"week", "month"},
			},
		},
	}
}

// normalizeLaborPeriod accepts only week|month (default week) — labor cannot be
// resolved for a single day, unlike the menu tools.
func normalizeLaborPeriod(args map[string]any) (string, error) {
	raw, ok := args["period"]
	if !ok {
		return "week", nil
	}
	str, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("get_labor_cost: period must be a string, got %T", raw)
	}
	switch str {
	case "":
		return "week", nil
	case "week", "month":
		return str, nil
	default:
		return "", fmt.Errorf("get_labor_cost: unsupported period %q (allowed: week, month)", str)
	}
}

func (t *LaborCostTool) Run(_ context.Context, args map[string]any, env ToolEnv) (ToolResult, error) {
	payroll := t.Payroll
	if payroll == nil && env.DB != nil {
		payroll = env.DB
	}
	sales := t.Sales
	if sales == nil && env.Analytics != nil {
		sales = env.Analytics
	}
	window := t.Window
	if window == nil && env.Analytics != nil {
		window = env.Analytics
	}
	recipes := t.Recipes
	if recipes == nil && env.DB != nil {
		recipes = env.DB
	}
	items := t.Items
	if items == nil && env.Analytics != nil {
		items = env.Analytics
	}
	if payroll == nil || sales == nil || window == nil || recipes == nil || items == nil {
		return ToolResult{}, fmt.Errorf("get_labor_cost: providers not wired")
	}

	period, err := normalizeLaborPeriod(args)
	if err != nil {
		return ToolResult{}, err
	}

	rep, err := labor.NewCalculator(payroll, sales, window).Analyze(env.BusinessID, period, env.Location)
	if err != nil {
		return ToolResult{}, fmt.Errorf("get_labor_cost: %w", err)
	}

	// Compose prime cost; food-cost failure is non-fatal (food contributes 0).
	// prime_cost_pct is the food%+labor% operator heuristic — approximate when
	// recipe coverage is sparse (food% is over recipe-mapped sales only). Advisory.
	foodCostPct := 0.0
	if fc, ferr := foodcost.NewCalculator(recipes, items).Analyze(env.BusinessID, period, env.Location); ferr == nil {
		foodCostPct = fc.BlendedFoodCostPct
	}
	primeCostPct := rep.LaborCostPct + foodCostPct

	return ToolResult{
		Summary: fmt.Sprintf("Labor %.0f%% of sales; prime cost %.0f%%", rep.LaborCostPct*100, primeCostPct*100),
		Data: map[string]any{
			"period":            rep.Period,
			"labor_cost":        rep.LaborCost,
			"net_sales":         rep.NetSales,
			"labor_cost_pct":    rep.LaborCostPct,
			"payroll_run_count": rep.PayrollRunCount,
			"has_data":          rep.HasData,
			"food_cost_pct":     foodCostPct,
			"prime_cost_pct":    primeCostPct,
		},
	}, nil
}
