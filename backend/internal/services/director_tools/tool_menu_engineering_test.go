package director_tools

import (
	"context"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMenuEngineeringTool_Metadata(t *testing.T) {
	tool := &MenuEngineeringTool{}
	assert.Equal(t, "get_menu_engineering", tool.Name())
	assert.NotEmpty(t, tool.HumanLabel("en"))
	assert.NotEmpty(t, tool.HumanLabel("es"))
	assert.NotEmpty(t, tool.Description())
	schema := tool.Schema()
	require.NotNil(t, schema)
	require.NotNil(t, schema.Properties["period"])
	require.NotNil(t, schema.Properties["limit"])
}

// TestMenuEngineeringTool_FallsBackToEnvProviders mirrors the food-cost tool's
// env-fallback test: the bare struct exercises the env.Analytics (items) and
// env.DB (recipes) fallback. A recipe with a costed inventory item is a
// complete-cost dish even with no sale, so it lands in the classifier output —
// proving the full Data shape without seeding the analytics popular-items CTE.
func TestMenuEngineeringTool_FallsBackToEnvProviders(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Menu Engineering Bistro")

	// Seed one recipe-mapped menu item with a real ingredient cost (the env.DB
	// RecipeCostProvider reads InventoryItem.CostPerUnit via GetRecipeCostsForBusiness).
	ing := database.InventoryItem{BusinessID: bizID, Name: "pizza-ing", Unit: "kg", CostPerUnit: 3.0}
	require.NoError(t, db.GetGorm().Create(&ing).Error)
	require.NoError(t, db.GetGorm().Create(&database.InventoryRecipe{
		BusinessID: bizID, MenuItemID: "pizza", MenuItemName: "Pizza",
		InventoryItemID: ing.ID, QuantityRequired: 1,
	}).Error)

	tool := &MenuEngineeringTool{} // bare → env fallback
	env := ToolEnv{
		BusinessID: bizID,
		Locale:     "en",
		DB:         db,
		Analytics:  newTestAnalytics(t, db),
		Location:   time.UTC,
	}

	result, err := tool.Run(context.Background(), map[string]any{"period": "week"}, env)
	require.NoError(t, err)
	assert.Equal(t, "week", result.Data["period"])

	counts, ok := result.Data["quadrant_counts"].(map[string]int)
	require.True(t, ok, "quadrant_counts must be map[string]int")
	require.NotNil(t, counts)

	dishes, ok := result.Data["dishes"].([]map[string]any)
	require.True(t, ok, "dishes must be []map[string]any")
	require.NotEmpty(t, dishes)
	for _, d := range dishes {
		require.Contains(t, d, "quadrant")
		require.Contains(t, d, "recommended_action")
		require.Contains(t, d, "suggested_price")
	}

	// Scalar rollups the model and UI rely on.
	require.Contains(t, result.Data, "median_food_cost_pct")
	require.Contains(t, result.Data, "median_qty_sold")
	require.Contains(t, result.Data, "sparse")
	require.Contains(t, result.Data, "items_needing_cost")
}

// TestMenuEngineeringTool_ClassifiesQuadrants uses the stub provider seams to
// drive a realistic 4-dish menu through the classifier and proves the plowhorse
// carries a non-zero suggested_price — the value Sage chains into
// propose_price_change to act.
func TestMenuEngineeringTool_ClassifiesQuadrants(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Quadrant Bistro")

	items := &stubPopularItemsProvider{byPeriod: map[string][]analytics.ItemStats{
		"week": {
			{ItemID: "salad", ItemName: "Salad", TotalSold: 30, Revenue: 300, AveragePrice: 10.0},    // fc 0.20, popular  -> star
			{ItemID: "ribeye", ItemName: "Ribeye", TotalSold: 25, Revenue: 500, AveragePrice: 20.0},  // fc 0.60, popular  -> plowhorse
			{ItemID: "souffle", ItemName: "Souffle", TotalSold: 5, Revenue: 40, AveragePrice: 8.0},   // fc 0.25, niche    -> puzzle
			{ItemID: "lobster", ItemName: "Lobster", TotalSold: 4, Revenue: 160, AveragePrice: 40.0}, // fc 0.55, niche    -> dog
		},
	}}
	recipes := &stubRecipeCosts{rows: []database.RecipeIngredientCost{
		{MenuItemID: "salad", MenuItemName: "Salad", QuantityRequired: 1, CostPerUnit: 2.0, HasCost: true},
		{MenuItemID: "ribeye", MenuItemName: "Ribeye", QuantityRequired: 1, CostPerUnit: 12.0, HasCost: true},
		{MenuItemID: "souffle", MenuItemName: "Souffle", QuantityRequired: 1, CostPerUnit: 2.0, HasCost: true},
		{MenuItemID: "lobster", MenuItemName: "Lobster", QuantityRequired: 1, CostPerUnit: 22.0, HasCost: true},
	}}

	tool := &MenuEngineeringTool{Items: items, Recipes: recipes}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	result, err := tool.Run(context.Background(), map[string]any{"period": "week"}, env)
	require.NoError(t, err)

	counts := result.Data["quadrant_counts"].(map[string]int)
	assert.Equal(t, 1, counts["star"])
	assert.Equal(t, 1, counts["plowhorse"])
	assert.Equal(t, 1, counts["puzzle"])
	assert.Equal(t, 1, counts["dog"])

	dishes := result.Data["dishes"].([]map[string]any)
	require.Len(t, dishes, 4)

	var plowhorse map[string]any
	for _, d := range dishes {
		if d["quadrant"] == "plowhorse" {
			plowhorse = d
		}
	}
	require.NotNil(t, plowhorse, "expected a plowhorse dish")
	assert.Equal(t, "reprice_up", plowhorse["recommended_action"])
	// UnitCost 12 / median food-cost 0.40 = 30; max(avg 20, 30) = 30.
	assert.InDelta(t, 30.0, plowhorse["suggested_price"].(float64), 0.0001)
}

// TestMenuEngineeringTool_BoundsDishesByLimit proves the emitted dishes list is
// capped to limit (token discipline) while quadrant_counts stays the full
// population — mirroring the food-cost tool's "worst N + full aggregate" shape.
func TestMenuEngineeringTool_BoundsDishesByLimit(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Bounded Bistro")

	items := &stubPopularItemsProvider{byPeriod: map[string][]analytics.ItemStats{
		"week": {
			{ItemID: "i1", ItemName: "I1", TotalSold: 30, Revenue: 300, AveragePrice: 10.0}, // fc 0.20
			{ItemID: "i2", ItemName: "I2", TotalSold: 25, Revenue: 500, AveragePrice: 20.0}, // fc 0.60
			{ItemID: "i3", ItemName: "I3", TotalSold: 5, Revenue: 40, AveragePrice: 8.0},    // fc 0.25
			{ItemID: "i4", ItemName: "I4", TotalSold: 4, Revenue: 160, AveragePrice: 40.0},  // fc 0.55
			{ItemID: "i5", ItemName: "I5", TotalSold: 20, Revenue: 300, AveragePrice: 15.0}, // fc 0.333
			{ItemID: "i6", ItemName: "I6", TotalSold: 10, Revenue: 120, AveragePrice: 12.0}, // fc 0.583
		},
	}}
	recipes := &stubRecipeCosts{rows: []database.RecipeIngredientCost{
		{MenuItemID: "i1", MenuItemName: "I1", QuantityRequired: 1, CostPerUnit: 2.0, HasCost: true},
		{MenuItemID: "i2", MenuItemName: "I2", QuantityRequired: 1, CostPerUnit: 12.0, HasCost: true},
		{MenuItemID: "i3", MenuItemName: "I3", QuantityRequired: 1, CostPerUnit: 2.0, HasCost: true},
		{MenuItemID: "i4", MenuItemName: "I4", QuantityRequired: 1, CostPerUnit: 22.0, HasCost: true},
		{MenuItemID: "i5", MenuItemName: "I5", QuantityRequired: 1, CostPerUnit: 5.0, HasCost: true},
		{MenuItemID: "i6", MenuItemName: "I6", QuantityRequired: 1, CostPerUnit: 7.0, HasCost: true},
	}}

	tool := &MenuEngineeringTool{Items: items, Recipes: recipes}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	result, err := tool.Run(context.Background(), map[string]any{"period": "week", "limit": float64(2)}, env)
	require.NoError(t, err)

	dishes := result.Data["dishes"].([]map[string]any)
	require.LessOrEqual(t, len(dishes), 2)
	require.Len(t, dishes, 2)
	// Worst-margin first: highest food-cost dish (i2, 0.60) leads the capped list.
	assert.Equal(t, "I2", dishes[0]["name"])

	// quadrant_counts is the full population (all 6 classified), not the capped 2.
	counts := result.Data["quadrant_counts"].(map[string]int)
	total := 0
	for _, c := range counts {
		total += c
	}
	assert.Equal(t, 6, total)
}

func TestMenuEngineeringTool_ValidatesPeriod(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Bad Period Menu Engineering")
	tool := &MenuEngineeringTool{Items: &stubPopularItemsProvider{}, Recipes: &stubRecipeCosts{}}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}
	_, err := tool.Run(context.Background(), map[string]any{"period": "fortnight"}, env)
	require.Error(t, err)
}
