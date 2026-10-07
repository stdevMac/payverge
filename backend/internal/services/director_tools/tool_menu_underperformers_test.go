package director_tools

import (
	"context"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/analytics"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMenuUnderperformersTool_Name(t *testing.T) {
	tool := &MenuUnderperformersTool{}
	assert.Equal(t, "get_menu_underperformers", tool.Name())
}

func TestMenuUnderperformersTool_HumanLabel(t *testing.T) {
	tool := &MenuUnderperformersTool{}
	assert.NotEmpty(t, tool.HumanLabel("en"))
	assert.NotEmpty(t, tool.HumanLabel("es"))
}

func TestMenuUnderperformersTool_Schema(t *testing.T) {
	tool := &MenuUnderperformersTool{}
	schema := tool.Schema()
	require.NotNil(t, schema)
	require.NotNil(t, schema.Properties["period"])
	require.NotNil(t, schema.Properties["limit"])
}

func TestMenuUnderperformersTool_Run(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Underperformers Bistro")

	// Median price across this set: prices sorted = [3, 7, 12, 12, 15, 32];
	// median = (12+12)/2 = 12. Items with AveragePrice > 12 are
	// "premium" tier: Truffle Fries (15) and Wagyu Burger (32).
	// Bottom-by-revenue among the premium tier, asc by revenue:
	// Truffle Fries ($210) then Wagyu Burger ($320).
	provider := &stubPopularItemsProvider{
		byPeriod: map[string][]analytics.ItemStats{
			"week": {
				{ItemName: "Espresso", TotalSold: 30, Revenue: 90.00, AveragePrice: 3.0},
				{ItemName: "House Salad", TotalSold: 22, Revenue: 154.00, AveragePrice: 7.0},
				{ItemName: "Tiramisu", TotalSold: 8, Revenue: 96.00, AveragePrice: 12.0},
				{ItemName: "House Red", TotalSold: 5, Revenue: 60.00, AveragePrice: 12.0},
				{ItemName: "Truffle Fries", TotalSold: 14, Revenue: 210.00, AveragePrice: 15.0},
				{ItemName: "Wagyu Burger", TotalSold: 10, Revenue: 320.00, AveragePrice: 32.0},
			},
		},
	}

	tool := &MenuUnderperformersTool{Items: provider}
	env := ToolEnv{
		BusinessID: bizID,
		Locale:     "en",
		DB:         db,
	}

	result, err := tool.Run(context.Background(), map[string]any{"period": "week", "limit": float64(5)}, env)
	require.NoError(t, err)

	items, ok := result.Data["items"].([]map[string]any)
	require.True(t, ok, "items should be []map[string]any")
	require.Len(t, items, 2, "only items priced above median should remain")

	// Cheaper ones excluded.
	names := []string{
		items[0]["name"].(string),
		items[1]["name"].(string),
	}
	assert.NotContains(t, names, "Espresso")
	assert.NotContains(t, names, "House Salad")
	assert.NotContains(t, names, "Tiramisu")
	assert.NotContains(t, names, "House Red")

	// Sorted asc by revenue (worst first within the premium tier).
	assert.Equal(t, "Truffle Fries", items[0]["name"])
	assert.Equal(t, "Wagyu Burger", items[1]["name"])
	assert.InDelta(t, 15.0, items[0]["unit_price"].(float64), 0.001)

	assert.Equal(t, "week", result.Data["period"])
	assert.Equal(t, 5, result.Data["limit"])
	assert.Contains(t, result.Summary, "2")
}

func TestMenuUnderperformersTool_DefaultsToWeekAndFiveItems(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Default Underperformers Bistro")

	provider := &stubPopularItemsProvider{
		byPeriod: map[string][]analytics.ItemStats{
			"week": {
				{ItemName: "A", Revenue: 10, AveragePrice: 4},
				{ItemName: "B", Revenue: 20, AveragePrice: 8},
				{ItemName: "C", Revenue: 30, AveragePrice: 12},
				{ItemName: "D", Revenue: 40, AveragePrice: 16},
				{ItemName: "E", Revenue: 50, AveragePrice: 20},
				{ItemName: "F", Revenue: 60, AveragePrice: 24},
			},
		},
	}

	tool := &MenuUnderperformersTool{Items: provider}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	result, err := tool.Run(context.Background(), map[string]any{}, env)
	require.NoError(t, err)
	assert.Equal(t, "week", result.Data["period"])
	assert.Equal(t, 5, result.Data["limit"])
}

func TestMenuUnderperformersTool_ValidatesPeriod(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Bad Period Underperformers")

	tool := &MenuUnderperformersTool{Items: &stubPopularItemsProvider{}}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	_, err := tool.Run(context.Background(), map[string]any{"period": "year"}, env)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "period"))
}
