package ops_tools_test

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/agents"
	"github.com/stdevmac/payverge/backend/internal/agents/ops_tools"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestOpsReadOnlyStatePluginStatusIsScopedAndSecretFree(t *testing.T) {
	db := setupOpsPluginStatusDB(t)
	stripe := database.Plugin{Name: "stripe", DisplayName: "Stripe", Category: database.PluginCategoryPayment, IsActive: true}
	paypal := database.Plugin{Name: "paypal", DisplayName: "PayPal", Category: database.PluginCategoryPayment, IsActive: true}
	foreign := database.Plugin{Name: "foreign-provider", DisplayName: "Foreign", Category: database.PluginCategoryPayment, IsActive: true}
	for _, plugin := range []*database.Plugin{&stripe, &paypal, &foreign} {
		require.NoError(t, db.GetGorm().Create(plugin).Error)
	}
	require.NoError(t, db.GetGorm().Create(&database.BusinessPlugin{
		BusinessID: 7, PluginID: stripe.ID, IsEnabled: true,
		Config: `{"secret_key":"sk_test_never_return","provider_payload":{"merchant":"private"}}`, LastStatus: "ok",
	}).Error)
	require.NoError(t, db.GetGorm().Create(&database.BusinessPlugin{
		BusinessID: 7, PluginID: paypal.ID, IsEnabled: false, Config: `{"client_secret":"hidden"}`,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&database.BusinessPlugin{
		BusinessID: 99, PluginID: foreign.ID, IsEnabled: true, Config: `{"token":"foreign-secret"}`, LastStatus: "ok",
	}).Error)

	writes := installOpsWriteCounter(t, db.GetGorm())
	tool := &ops_tools.PluginStatusTool{}
	result, err := tool.Run(context.Background(), nil, agents.ToolEnv{BusinessID: 7, DB: db})
	require.NoError(t, err)
	assert.Equal(t, "get_plugin_status", tool.Name())
	assert.Equal(t, 0, *writes)

	plugins, ok := result.Data["plugins"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, plugins, 2)
	sort.Slice(plugins, func(i, j int) bool { return plugins[i]["name"].(string) < plugins[j]["name"].(string) })
	assert.Equal(t, map[string]any{"name": "paypal", "enabled": false, "connected": false, "status": "unknown"}, plugins[0])
	assert.Equal(t, map[string]any{"name": "stripe", "enabled": true, "connected": true, "status": "ok"}, plugins[1])
	assert.NotContains(t, fmt.Sprint(result.Data), "secret")
	assert.NotContains(t, fmt.Sprint(result.Data), "provider_payload")
	assert.NotContains(t, fmt.Sprint(result.Data), "foreign-provider")
}

func TestOpsNoMutationRegistryExposesOnlyReadToolsAndSupportCommunication(t *testing.T) {
	registry := ops_tools.NewRegistry()
	allowedSideEffect := map[string]bool{"create_support_escalation": true}
	for _, declaration := range registry.Declarations() {
		name := declaration.Name
		for _, prefix := range []string{"create_", "update_", "delete_", "enable_", "disable_", "publish_", "charge_", "refund_"} {
			if len(name) >= len(prefix) && name[:len(prefix)] == prefix {
				assert.True(t, allowedSideEffect[name], "non-read Ops tool registered: %s", name)
			}
		}
	}
	_, ok := registry.Get("get_plugin_status")
	assert.True(t, ok)
}

func setupOpsPluginStatusDB(t *testing.T) *database.DB {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:ops-plugin-status-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(&database.Plugin{}, &database.BusinessPlugin{}))
	return database.GetDBWrapper()
}
