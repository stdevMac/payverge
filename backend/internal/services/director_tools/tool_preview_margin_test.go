package director_tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreviewMarginChangeTool_Metadata(t *testing.T) {
	tool := &PreviewMarginChangeTool{}
	assert.Equal(t, "preview_margin_change", tool.Name())
	assert.Equal(t, "Previewing new margin", tool.HumanLabel("en"))
	assert.Equal(t, "Calculando el margen nuevo", tool.HumanLabel("es"))
	assert.Equal(t, "Calculando el margen nuevo", tool.HumanLabel("es_ar"))
	assert.NotContains(t, tool.HumanLabel("en"), "preview_margin_change")
	assert.NotContains(t, tool.HumanLabel("es"), "preview_margin_change")
	assert.NotEqual(t, tool.HumanLabel("en"), tool.Description())
	assert.Contains(t, tool.Description(), "Does not apply")
	assert.Contains(t, tool.Description(), "1.50")
	assert.Contains(t, tool.Description(), "pass that as value")
	require.NotNil(t, tool.Schema())
}

func TestPreviewMarginChangeTool_LiveShapedBestsellersReturnCurrentAndNew(t *testing.T) {
	// Live 2026-08-21 Franky QA: Steak has plate cost, Demo Spritz does not.
	// The model asked what price to consider instead of computing a preview.
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Live Margin Lounge")
	cats := []database.MenuCategory{{
		ID:   "cat-mains",
		Name: "Mains",
		Items: []database.MenuItem{
			{ID: "demo-steak", Name: "Steak Plate", Price: 42.00, Cogs: 7.52, IsAvailable: true},
			{ID: "demo-spritz", Name: "Demo Spritz", Price: 14.00, IsAvailable: true},
			{ID: "demo-bowl", Name: "Harvest Bowl", Price: 18.50, Cogs: 8.50, IsAvailable: true},
		},
	}}
	raw, err := json.Marshal(cats)
	require.NoError(t, err)
	require.NoError(t, db.GetGorm().Create(&database.Menu{
		BusinessID: bizID, Categories: string(raw), IsActive: true, Version: 1,
	}).Error)

	tool := &PreviewMarginChangeTool{}
	env := ToolEnv{
		BusinessID: bizID,
		Locale:     "en",
		DB:         db,
		Analytics:  newTestAnalytics(t, db),
	}

	res, err := tool.Run(context.Background(), map[string]any{}, env)
	require.NoError(t, err)
	assert.Zero(t, res.ProposalID)
	assert.Equal(t, false, res.Data["applied"])
	assert.InDelta(t, 1.50, res.Data["price_increase"].(float64), 0.001)

	items, ok := res.Data["items"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, items, 2)

	byName := map[string]map[string]any{}
	for _, row := range items {
		name, _ := row["name"].(string)
		byName[name] = row
		assert.Equal(t, false, row["applied"])
		assert.Equal(t, true, row["unit_cost_known"], "%s must have plate cost for the numeric walkthrough", name)
		assert.Contains(t, row, "current_margin")
		assert.Contains(t, row, "new_margin")
		assert.Contains(t, row, "current_food_cost_pct")
		assert.Contains(t, row, "new_food_cost_pct")
	}
	assert.NotContains(t, byName, "Demo Spritz", "must not invent a plate cost for Spritz")

	steak := byName["Steak Plate"]
	require.NotNil(t, steak)
	assert.InDelta(t, 42.00, steak["current_price"].(float64), 0.001)
	assert.InDelta(t, 43.50, steak["proposed_price"].(float64), 0.001)
	assert.InDelta(t, 7.52, steak["unit_cost"].(float64), 0.001)
	assert.InDelta(t, 34.48, steak["current_margin"].(float64), 0.001)
	assert.InDelta(t, 35.98, steak["new_margin"].(float64), 0.001)
	assert.InDelta(t, 7.52/42.00, steak["current_food_cost_pct"].(float64), 0.0001)
	assert.InDelta(t, 7.52/43.50, steak["new_food_cost_pct"].(float64), 0.0001)

	assert.Contains(t, res.Summary, "current margin")
	assert.Contains(t, res.Summary, "new margin")
	assert.Contains(t, res.Summary, "Nothing is queued")
	assert.NotContains(t, res.Summary, "what price")
}

func TestPreviewMarginChangeTool_NamedTwoDollarBumpDoesNotWriteMenu(t *testing.T) {
	db := newTestDB(t)
	bizID := createHarvestBowlMenu(t, db, "Named Two Dollar Preview")
	tool := &PreviewMarginChangeTool{}
	res, err := tool.Run(context.Background(),
		map[string]any{"item_name": "Harvest Bowl", "value": float64(2)},
		ToolEnv{BusinessID: bizID, DB: db, Analytics: newTestAnalytics(t, db)},
	)
	require.NoError(t, err)
	assert.Zero(t, res.ProposalID)
	assert.Equal(t, false, res.Data["applied"])
	assert.InDelta(t, 2.00, res.Data["price_increase"].(float64), 0.001)
	items := res.Data["items"].([]map[string]any)
	require.Len(t, items, 1)
	assert.Equal(t, "Harvest Bowl", items[0]["name"])
	assert.InDelta(t, 18.50, items[0]["current_price"].(float64), 0.001)
	assert.InDelta(t, 20.50, items[0]["proposed_price"].(float64), 0.001)
	assert.InDelta(t, 10.00, items[0]["current_margin"].(float64), 0.001)
	assert.InDelta(t, 12.00, items[0]["new_margin"].(float64), 0.001)
	assert.NotContains(t, res.Summary, "11.50")

	_, cats, err := database.GetMenuByBusinessID(bizID)
	require.NoError(t, err)
	found := false
	for _, cat := range cats {
		for _, item := range cat.Items {
			if item.Name == "Harvest Bowl" {
				found = true
				assert.InDelta(t, 18.50, item.Price, 0.001, "preview must not write the menu")
			}
		}
	}
	assert.True(t, found)
}

func TestPreviewMarginChangeTool_NamedItemUsesDefaultBump(t *testing.T) {
	db := newTestDB(t)
	bizID := createHarvestBowlMenu(t, db, "Named Preview Bistro")
	tool := &PreviewMarginChangeTool{}
	res, err := tool.Run(context.Background(),
		map[string]any{"item_name": "Harvest Bowl"},
		ToolEnv{BusinessID: bizID, DB: db, Analytics: newTestAnalytics(t, db)},
	)
	require.NoError(t, err)
	items := res.Data["items"].([]map[string]any)
	require.Len(t, items, 1)
	assert.Equal(t, "Harvest Bowl", items[0]["name"])
	assert.InDelta(t, 10.00, items[0]["current_margin"].(float64), 0.001)
	assert.InDelta(t, 11.50, items[0]["new_margin"].(float64), 0.001)
	assert.InDelta(t, 8.50/18.50, items[0]["current_food_cost_pct"].(float64), 0.0001)
	assert.InDelta(t, 8.50/20.00, items[0]["new_food_cost_pct"].(float64), 0.0001)
}

func TestPreviewMarginChangeTool_PrefersCostedBestsellers(t *testing.T) {
	sold := []marginPreviewDish{
		{Name: "Demo Spritz", Price: 14, Units: 163, HasSales: true},
		{Name: "Steak Plate", Price: 42, Cost: 7.52, Units: 158, HasSales: true, HasCost: true},
		{Name: "Harvest Bowl", Price: 18.50, Cost: 8.50, HasCost: true},
	}
	got := selectMarginPreviewDishes(sold, nil, 2)
	require.Len(t, got, 2)
	assert.Equal(t, "Steak Plate", got[0].Name)
	assert.Equal(t, "Harvest Bowl", got[1].Name)
}
