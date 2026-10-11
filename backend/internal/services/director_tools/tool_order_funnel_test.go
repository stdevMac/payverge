package director_tools

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrderFunnelTool_Name(t *testing.T) {
	tool := &OrderFunnelTool{}
	assert.Equal(t, "get_order_funnel", tool.Name())
}

func TestOrderFunnelTool_HumanLabel(t *testing.T) {
	tool := &OrderFunnelTool{}
	assert.NotEmpty(t, tool.HumanLabel("en"))
	assert.NotEmpty(t, tool.HumanLabel("es"))
}

func TestOrderFunnelTool_Schema(t *testing.T) {
	tool := &OrderFunnelTool{}
	schema := tool.Schema()
	require.NotNil(t, schema)
	require.NotNil(t, schema.Properties["period"])
}

func TestOrderFunnelTool_Run(t *testing.T) {
	db := newTestDB(t)
	bizID := seedOrdersForFunnel(t, db, map[string]int{
		"pending":   5,
		"approved":  12,
		"ready":     3,
		"delivered": 80,
		"cancelled": 2,
	})

	tool := &OrderFunnelTool{}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	result, err := tool.Run(context.Background(), map[string]any{"period": "week"}, env)
	require.NoError(t, err)

	statuses, ok := result.Data["statuses"].(map[string]int)
	require.True(t, ok, "statuses should be map[string]int")

	assert.Equal(t, 5, statuses["pending"])
	assert.Equal(t, 12, statuses["approved"])
	assert.Equal(t, 3, statuses["ready"])
	assert.Equal(t, 80, statuses["delivered"])
	assert.Equal(t, 2, statuses["cancelled"])

	assert.Equal(t, 102, result.Data["total"])
	// 2 cancelled / 102 total = ~1.96%
	assert.InDelta(t, 1.96, result.Data["cancellation_rate"], 0.05)
	assert.Equal(t, "week", result.Data["period"])
	assert.NotEmpty(t, result.Summary)
}

func TestOrderFunnelTool_DefaultsToWeek(t *testing.T) {
	db := newTestDB(t)
	bizID := seedOrdersForFunnel(t, db, map[string]int{
		"approved": 1,
	})

	tool := &OrderFunnelTool{}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	result, err := tool.Run(context.Background(), map[string]any{}, env)
	require.NoError(t, err)
	assert.Equal(t, "week", result.Data["period"])
}

func TestOrderFunnelTool_ValidatesPeriod(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Bad Period Funnel")

	tool := &OrderFunnelTool{}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	_, err := tool.Run(context.Background(), map[string]any{"period": "century"}, env)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "period"))
}

func TestOrderFunnelTool_EmptyBusiness(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "No Orders Bistro")

	tool := &OrderFunnelTool{}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	result, err := tool.Run(context.Background(), map[string]any{"period": "day"}, env)
	require.NoError(t, err)

	assert.Equal(t, 0, result.Data["total"])
	assert.InDelta(t, 0.0, result.Data["cancellation_rate"], 0.001)
}
