package director_tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubRecipeCosts struct {
	rows []database.RecipeIngredientCost
}

func (s *stubRecipeCosts) GetRecipeCostsForBusiness(_ uint) ([]database.RecipeIngredientCost, error) {
	return s.rows, nil
}

func TestFoodCostAnalysisTool_Metadata(t *testing.T) {
	tool := &FoodCostAnalysisTool{}
	assert.Equal(t, "get_food_cost_analysis", tool.Name())
	assert.NotEmpty(t, tool.HumanLabel("en"))
	assert.NotEmpty(t, tool.HumanLabel("es"))
	schema := tool.Schema()
	require.NotNil(t, schema)
	require.NotNil(t, schema.Properties["period"])
	require.NotNil(t, schema.Properties["limit"])
	require.NotNil(t, schema.Properties["threshold_pct"])
}

func TestFoodCostAnalysisTool_Run(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Food Cost Bistro")

	items := &stubPopularItemsProvider{byPeriod: map[string][]analytics.ItemStats{
		"week": {
			{ItemID: "pizza", ItemName: "Pizza", TotalSold: 20, RecognizedQuantity: 20, Revenue: 200, AveragePrice: 10.0},
			{ItemID: "steak", ItemName: "Steak", TotalSold: 5, RecognizedQuantity: 5, Revenue: 100, AveragePrice: 20.0},
		},
	}}
	recipes := &stubRecipeCosts{rows: []database.RecipeIngredientCost{
		{MenuItemID: "pizza", MenuItemName: "Pizza", QuantityRequired: 1, CostPerUnit: 3.0, HasCost: true},
		{MenuItemID: "steak", MenuItemName: "Steak", QuantityRequired: 1, CostPerUnit: 13.0, HasCost: true},
	}}

	tool := &FoodCostAnalysisTool{Items: items, Recipes: recipes}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	result, err := tool.Run(context.Background(), map[string]any{"period": "week", "limit": float64(5)}, env)
	require.NoError(t, err)
	assert.Equal(t, "week", result.Data["period"])
	assert.InDelta(t, 125.0/300.0, result.Data["blended_food_cost_pct"].(float64), 0.0001)

	itemsOut, ok := result.Data["items"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, itemsOut, 2)
	// Worst-first: Steak (65%) before Pizza (30%).
	assert.Equal(t, "Steak", itemsOut[0]["name"])
	assert.InDelta(t, 0.65, itemsOut[0]["food_cost_pct"].(float64), 0.0001)
}

func TestFoodCostAnalysisTool_IncludesUnsoldPlateCogs(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Unsold Cogs Bistro")
	cats := []database.MenuCategory{{
		ID:   "cat-mains",
		Name: "Mains",
		Items: []database.MenuItem{
			{ID: "demo-bowl", Name: "Harvest Bowl", Price: 18.50, Cogs: 8.50, IsAvailable: true},
			{ID: "demo-steak", Name: "Steak Plate", Price: 24.00, Cogs: 11.00, IsAvailable: true},
		},
	}}
	raw, err := json.Marshal(cats)
	require.NoError(t, err)
	require.NoError(t, db.GetGorm().Create(&database.Menu{
		BusinessID: bizID, Categories: string(raw), IsActive: true, Version: 1,
	}).Error)

	tool := &FoodCostAnalysisTool{}
	env := ToolEnv{
		BusinessID: bizID,
		Locale:     "en",
		DB:         db,
		Analytics:  newTestAnalytics(t, db),
	}
	result, err := tool.Run(context.Background(), map[string]any{"period": "week", "limit": float64(10)}, env)
	require.NoError(t, err)
	itemsOut, ok := result.Data["items"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, itemsOut, 2)
	byName := map[string]map[string]any{}
	for _, row := range itemsOut {
		name, _ := row["name"].(string)
		byName[name] = row
	}
	bowl := byName["Harvest Bowl"]
	require.NotNil(t, bowl)
	assert.InDelta(t, 8.50, bowl["unit_cost"].(float64), 0.0001)
	assert.InDelta(t, 18.50, bowl["avg_price"].(float64), 0.0001)
	assert.InDelta(t, 10.00, bowl["margin_per_unit"].(float64), 0.0001)
	assert.InDelta(t, 8.50/18.50, bowl["food_cost_pct"].(float64), 0.0001)
	steak := byName["Steak Plate"]
	require.NotNil(t, steak)
	assert.InDelta(t, 11.00, steak["unit_cost"].(float64), 0.0001)
	assert.InDelta(t, 24.00, steak["avg_price"].(float64), 0.0001)
	assert.InDelta(t, 13.00, steak["margin_per_unit"].(float64), 0.0001)
}

func TestFoodCostAnalysisTool_FallsBackToEnvProviders(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Env Fallback Bistro")
	// No Items/Recipes on the struct; env.Analytics provides items, env.DB recipes.
	tool := &FoodCostAnalysisTool{}
	env := ToolEnv{
		BusinessID: bizID,
		Locale:     "en",
		DB:         db,
		Analytics:  newTestAnalytics(t, db),
		Location:   time.UTC,
	}
	// With no recipes/sales seeded, Analyze returns an empty report (no error).
	result, err := tool.Run(context.Background(), map[string]any{}, env)
	require.NoError(t, err)
	assert.Equal(t, "week", result.Data["period"])
}

func TestFoodCostAnalysisTool_ValidatesPeriod(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Bad Period Food Cost")
	tool := &FoodCostAnalysisTool{Items: &stubPopularItemsProvider{}, Recipes: &stubRecipeCosts{}}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}
	_, err := tool.Run(context.Background(), map[string]any{"period": "fortnight"}, env)
	require.Error(t, err)
}

func TestFoodCostAnalysisTool_RejectsOutOfRangeThreshold(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Out Of Range Threshold")
	tool := &FoodCostAnalysisTool{Items: &stubPopularItemsProvider{}, Recipes: &stubRecipeCosts{}}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}
	_, err := tool.Run(context.Background(), map[string]any{"threshold_pct": float64(250)}, env)
	require.Error(t, err)
}

func TestFoodCostAnalysisTool_OverThresholdCountsAllButListCapped(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Over Threshold Bistro")

	// Three complete+sold dishes, all over a 20% threshold: 30%, 40%, 65%.
	items := &stubPopularItemsProvider{byPeriod: map[string][]analytics.ItemStats{
		"week": {
			{ItemID: "pizza", ItemName: "Pizza", TotalSold: 20, RecognizedQuantity: 20, Revenue: 200, AveragePrice: 10.0},
			{ItemID: "burger", ItemName: "Burger", TotalSold: 10, RecognizedQuantity: 10, Revenue: 100, AveragePrice: 10.0},
			{ItemID: "steak", ItemName: "Steak", TotalSold: 5, RecognizedQuantity: 5, Revenue: 100, AveragePrice: 20.0},
		},
	}}
	recipes := &stubRecipeCosts{rows: []database.RecipeIngredientCost{
		{MenuItemID: "pizza", MenuItemName: "Pizza", QuantityRequired: 1, CostPerUnit: 3.0, HasCost: true},   // 30%
		{MenuItemID: "burger", MenuItemName: "Burger", QuantityRequired: 1, CostPerUnit: 4.0, HasCost: true}, // 40%
		{MenuItemID: "steak", MenuItemName: "Steak", QuantityRequired: 1, CostPerUnit: 13.0, HasCost: true},  // 65%
	}}

	tool := &FoodCostAnalysisTool{Items: items, Recipes: recipes}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	result, err := tool.Run(context.Background(), map[string]any{
		"period":        "week",
		"limit":         float64(2),
		"threshold_pct": float64(20),
	}, env)
	require.NoError(t, err)
	// Counter sees ALL three over-threshold dishes even though the list is capped.
	assert.Equal(t, 3, result.Data["items_over_threshold"].(int))
	require.Len(t, result.Data["items"].([]map[string]any), 2)
}
