package director_tools

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"
)

// --- local fakes (mirror internal/services/labor/calculator_test.go) ---

type fakeLaborWindow struct{ start, end time.Time }

func (f *fakeLaborWindow) ParsePeriodWindow(_ string, _ *time.Location) (time.Time, time.Time, error) {
	return f.start, f.end, nil
}

type fakeLaborPayroll struct{ runs []database.PayrollRunCost }

func (f *fakeLaborPayroll) GetPaidPayrollRunsOverlapping(_ uint, _, _ time.Time) ([]database.PayrollRunCost, error) {
	return f.runs, nil
}

type fakeLaborSales struct {
	summary *analytics.PaymentWindowSummary
}

func (f *fakeLaborSales) GetPaymentWindowSummary(_ uint, _, _ time.Time) (*analytics.PaymentWindowSummary, error) {
	return f.summary, nil
}

// foodcost seams
type fakeFoodRecipes struct {
	rows []database.RecipeIngredientCost
}

func (f *fakeFoodRecipes) GetRecipeCostsForBusiness(_ uint) ([]database.RecipeIngredientCost, error) {
	return f.rows, nil
}

type fakeFoodItems struct{ stats []analytics.ItemStats }

func (f *fakeFoodItems) GetPopularItems(_ uint, _ int, _ string, _ *time.Location) ([]analytics.ItemStats, error) {
	return f.stats, nil
}

func laborDay(n int) time.Time {
	return time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, n)
}

// fully-wired tool whose seams produce a deterministic report: labor_cost_pct=0.30
// (run [day0,day7] $600 over $2000 sales) and food_cost_pct=0.30 ($3 cost / $10
// price across 5 sold) → prime_cost_pct=0.60.
func wiredLaborTool() *LaborCostTool {
	return &LaborCostTool{
		Window:  &fakeLaborWindow{start: laborDay(0), end: laborDay(7)},
		Payroll: &fakeLaborPayroll{runs: []database.PayrollRunCost{{ID: 1, PeriodStart: laborDay(0), PeriodEnd: laborDay(7), GrossTotal: 60000}}},
		Sales:   &fakeLaborSales{summary: &analytics.PaymentWindowSummary{TotalRevenue: 2000, TotalTips: 300}},
		Recipes: &fakeFoodRecipes{rows: []database.RecipeIngredientCost{{MenuItemID: "burger", MenuItemName: "Burger", QuantityRequired: 1, CostPerUnit: 3, HasCost: true}}},
		Items:   &fakeFoodItems{stats: []analytics.ItemStats{{ItemID: "burger", ItemName: "Burger", TotalSold: 5, RecognizedQuantity: 5, AveragePrice: 10}}},
	}
}

func TestLaborCostTool_Name(t *testing.T) {
	assert.Equal(t, "get_labor_cost", (&LaborCostTool{}).Name())
}

func TestLaborCostTool_RejectsDay(t *testing.T) {
	// Seams must be non-nil so Run reaches normalizeLaborPeriod (provider
	// resolution happens first). The period error fires before any calc runs.
	_, err := wiredLaborTool().Run(context.Background(), map[string]any{"period": "day"}, ToolEnv{BusinessID: 1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported period")
}

func TestLaborCostTool_PrimeCostComposition(t *testing.T) {
	res, err := wiredLaborTool().Run(context.Background(), map[string]any{"period": "week"}, ToolEnv{BusinessID: 1})
	require.NoError(t, err)

	laborPct := res.Data["labor_cost_pct"].(float64)
	foodPct := res.Data["food_cost_pct"].(float64)
	primePct := res.Data["prime_cost_pct"].(float64)

	assert.InDelta(t, 0.30, laborPct, 0.0001)
	assert.InDelta(t, 0.30, foodPct, 0.0001)
	// Value-driven invariant: prime is exactly labor + food.
	assert.InDelta(t, laborPct+foodPct, primePct, 1e-9)
	assert.InDelta(t, 0.60, primePct, 0.0001)

	// Report fields flow through unchanged.
	assert.Equal(t, "week", res.Data["period"])
	assert.InDelta(t, 600.0, res.Data["labor_cost"].(float64), 0.01)
	assert.InDelta(t, 2000.0, res.Data["net_sales"].(float64), 0.01)
	assert.Equal(t, 1, res.Data["payroll_run_count"].(int))
	assert.True(t, res.Data["has_data"].(bool))

	assert.Equal(t, "Labor 30% of sales; prime cost 60%", res.Summary)
}

func TestLaborCostTool_HasDataFalseWhenNoRuns(t *testing.T) {
	// No payroll runs overlap → has_data false, labor 0% (do not invent a number).
	tool := wiredLaborTool()
	tool.Payroll = &fakeLaborPayroll{runs: nil}
	res, err := tool.Run(context.Background(), map[string]any{}, ToolEnv{BusinessID: 1})
	require.NoError(t, err)
	assert.False(t, res.Data["has_data"].(bool))
	assert.Equal(t, 0.0, res.Data["labor_cost_pct"].(float64))
	// food still composes; prime == labor + food invariant holds.
	assert.InDelta(t,
		res.Data["labor_cost_pct"].(float64)+res.Data["food_cost_pct"].(float64),
		res.Data["prime_cost_pct"].(float64), 1e-9)
}
