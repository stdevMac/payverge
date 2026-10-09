package director_tools

import (
	"context"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginStatusTool_Name(t *testing.T) {
	tool := &PluginStatusTool{}
	assert.Equal(t, "get_plugin_status", tool.Name())
}

func TestPluginStatusTool_HumanLabel(t *testing.T) {
	tool := &PluginStatusTool{}
	assert.NotEmpty(t, tool.HumanLabel("en"))
	assert.NotEmpty(t, tool.HumanLabel("es"))
}

func TestPluginStatusTool_Schema(t *testing.T) {
	tool := &PluginStatusTool{}
	schema := tool.Schema()
	require.NotNil(t, schema)
	require.NotNil(t, schema.Properties["category"])
}

func seedPluginRows(t *testing.T, db *database.DB, bizID uint) {
	t.Helper()
	gormDB := db.GetGorm()

	plugins := []database.Plugin{
		{Name: "stripe", DisplayName: "Stripe Integration", Category: database.PluginCategoryPayment, IsActive: true},
		{Name: "loyalty", DisplayName: "Loyalty", Category: database.PluginCategoryMarketing, IsActive: true},
		{Name: "kpi", DisplayName: "KPI Dashboard", Category: database.PluginCategoryAnalytics, IsActive: true},
		{Name: "reports", DisplayName: "Daily Reports", Category: database.PluginCategoryReporting, IsActive: true},
	}
	for i := range plugins {
		require.NoError(t, gormDB.Create(&plugins[i]).Error)
	}

	// Enable the first two plugins for this business.
	require.NoError(t, gormDB.Create(&database.BusinessPlugin{
		BusinessID: bizID, PluginID: plugins[0].ID, IsEnabled: true,
	}).Error)
	require.NoError(t, gormDB.Create(&database.BusinessPlugin{
		BusinessID: bizID, PluginID: plugins[1].ID, IsEnabled: true,
	}).Error)
}

func TestPluginStatusTool_Run(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Plugins Bistro")
	seedPluginRows(t, db, bizID)

	tool := &PluginStatusTool{}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	result, err := tool.Run(context.Background(), map[string]any{"category": "all"}, env)
	require.NoError(t, err)

	enabled, ok := result.Data["enabled"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, enabled, 2)

	recommended, ok := result.Data["recommended"].([]map[string]any)
	require.True(t, ok)
	// kpi (analytics) and reports (reporting) are the two not-enabled
	// plugins from the platform list.
	require.Len(t, recommended, 2)
	assert.Contains(t, result.Summary, "2")
}

func TestPluginStatusTool_FiltersByCategory(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Plugins Category Filter Bistro")
	seedPluginRows(t, db, bizID)

	tool := &PluginStatusTool{}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	result, err := tool.Run(context.Background(), map[string]any{"category": "analytics"}, env)
	require.NoError(t, err)

	recommended := result.Data["recommended"].([]map[string]any)
	require.Len(t, recommended, 1)
	assert.Equal(t, "kpi", recommended[0]["name"])
}

func TestPluginStatusTool_DefaultsToAll(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Default Category Plugins")
	seedPluginRows(t, db, bizID)

	tool := &PluginStatusTool{}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	result, err := tool.Run(context.Background(), map[string]any{}, env)
	require.NoError(t, err)
	assert.Equal(t, "all", result.Data["category"])
}

func TestPluginStatusTool_ValidatesCategory(t *testing.T) {
	db := newTestDB(t)
	bizID := createTestBusiness(t, db, "Bad Plugin Category")
	tool := &PluginStatusTool{}
	env := ToolEnv{BusinessID: bizID, Locale: "en", DB: db}

	_, err := tool.Run(context.Background(), map[string]any{"category": "bogus"}, env)
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "category"))
}
