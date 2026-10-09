package director_tools

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/analytics"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMenuTopItemsTool_Name(t *testing.T) {
	tool := &MenuTopItemsTool{}
	assert.Equal(t, "get_menu_top_items", tool.Name())
}

func TestMenuTopItemsTool_HumanLabel(t *testing.T) {
	tool := &MenuTopItemsTool{}
	assert.NotEmpty(t, tool.HumanLabel("en"))
	assert.NotEmpty(t, tool.HumanLabel("es"))
}

func TestMenuTopItemsTool_Schema(t *testing.T) {
	tool := &MenuTopItemsTool{}
	schema := tool.Schema()
	require.NotNil(t, schema)
	require.NotNil(t, schema.Properties["period"])
	require.NotNil(t, schema.Properties["limit"])
}

func TestMenuTopItemsTool_Run(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Top Items Bistro")

	// The real service returns rows already sorted by revenue DESC (via SQL
	// ORDER BY). The stub mirrors that pre-sorted order so the tool does not
	// need to re-sort in Go.
	provider := &stubPopularItemsProvider{
		byPeriod: map[string][]analytics.ItemStats{
			"week": {
				{ItemName: "Wagyu Burger", TotalSold: 10, Revenue: 320.00, AveragePrice: 32.0},
				{ItemName: "Truffle Fries", TotalSold: 14, Revenue: 210.00, AveragePrice: 15.0},
				{ItemName: "House Salad", TotalSold: 22, Revenue: 154.00, AveragePrice: 7.0},
				{ItemName: "Tiramisu", TotalSold: 8, Revenue: 96.00, AveragePrice: 12.0},
				{ItemName: "Espresso", TotalSold: 30, Revenue: 90.00, AveragePrice: 3.0},
				{ItemName: "House Red", TotalSold: 5, Revenue: 60.00, AveragePrice: 12.0},
			},
		},
	}

	tool := &MenuTopItemsTool{Items: provider}
	env := ToolEnv{
		BusinessID: bizID,
		Locale:     "en",
		DB:         db,
	}

	result, err := tool.Run(context.Background(), map[string]any{"period": "week", "limit": float64(3)}, env)
	require.NoError(t, err)

	assert.Equal(t, "week", result.Data["period"])
	assert.Equal(t, 3, result.Data["limit"])

	items, ok := result.Data["items"].([]map[string]any)
	require.True(t, ok, "items should be []map[string]any")
	require.Len(t, items, 3)

	// Sorted by revenue desc: Wagyu (320) > Fries (210) > Salad (154).
	assert.Equal(t, "Wagyu Burger", items[0]["name"])
	assert.InDelta(t, 320.0, items[0]["revenue"].(float64), 0.001)
	assert.Equal(t, "Truffle Fries", items[1]["name"])
	assert.Equal(t, "House Salad", items[2]["name"])

	assert.Contains(t, result.Summary, "3")
	assert.Contains(t, result.Summary, "week")
}

func TestMenuTopItemsTool_DefaultsToWeekAndTenItems(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Default Top Items Bistro")

	rows := make([]analytics.ItemStats, 0, 12)
	for i := 0; i < 12; i++ {
		rows = append(rows, analytics.ItemStats{
			ItemName: fmt.Sprintf("Item-%d", i),
			Revenue:  float64(120 - i*10),
		})
	}
	provider := &stubPopularItemsProvider{
		byPeriod: map[string][]analytics.ItemStats{"week": rows},
	}

	tool := &MenuTopItemsTool{Items: provider}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	result, err := tool.Run(context.Background(), map[string]any{}, env)
	require.NoError(t, err)
	assert.Equal(t, "week", result.Data["period"])
	assert.Equal(t, 10, result.Data["limit"])
	items := result.Data["items"].([]map[string]any)
	assert.Len(t, items, 10)
}

func TestMenuTopItemsTool_ValidatesPeriod(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Bad Period Top Items")

	tool := &MenuTopItemsTool{Items: &stubPopularItemsProvider{}}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	_, err := tool.Run(context.Background(), map[string]any{"period": "fortnight"}, env)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "period"))
}

func TestMenuTopItemsTool_ValidatesLimit(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Bad Limit Top Items")

	tool := &MenuTopItemsTool{Items: &stubPopularItemsProvider{}}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	_, err := tool.Run(context.Background(), map[string]any{"limit": float64(0)}, env)
	require.Error(t, err)

	_, err = tool.Run(context.Background(), map[string]any{"limit": float64(51)}, env)
	require.Error(t, err)
}
