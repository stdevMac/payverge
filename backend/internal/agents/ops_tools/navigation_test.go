package ops_tools_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/agents"
	"github.com/stdevmac/payverge/backend/internal/agents/ops_tools"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestNavigateToTabDirectorUnlockedForOperationalBusiness(t *testing.T) {
	tool := ops_tools.NavigateToTabTool{}
	res, err := tool.Run(context.Background(), map[string]any{"tab": "director-console"}, agents.ToolEnv{
		BusinessID: 42, IsStaffUser: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Data["locked"] != false {
		t.Fatalf("expected unlocked: no plan gates a tab, got %#v", res.Data)
	}
}

func TestNavigateToTabLockedWhenSuspended(t *testing.T) {
	tool := ops_tools.NavigateToTabTool{}
	res, err := tool.Run(context.Background(), map[string]any{"tab": "director-console"}, agents.ToolEnv{
		BusinessID: 42, IsStaffUser: true, IsSuspended: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Data["locked"] != true || res.Data["hidden"] != true {
		t.Fatalf("expected locked and hidden for suspended staff, got %#v", res.Data)
	}
}

func TestOpsReadOnlyStateBusinessContextReturnsAIFactsWithoutWrites(t *testing.T) {
	db := setupOpsNavigationDB(t)
	biz := &database.Business{
		BusinessId: "ops-context", Name: "Context Cafe", OwnerAddress: "0x",
		SettlementAddr: "0x1", TippingAddr: "0x2", IsActive: true,
		AiSettings: database.BusinessAiSettings{AiEnabled: true},
	}
	require.NoError(t, db.GetGorm().Create(biz).Error)

	writes := installOpsWriteCounter(t, db.GetGorm())
	result, err := (&ops_tools.BusinessContextTool{}).Run(context.Background(), nil, agents.ToolEnv{
		BusinessID: biz.ID, DB: db,
	})
	require.NoError(t, err)
	assert.Equal(t, true, result.Data["ai_enabled"])
	assert.Equal(t, true, result.Data["operational"])
	assert.NotContains(t, result.Data, "subscription_plan")
	assert.Equal(t, 0, *writes)
}

func TestOpsReadOnlyStateSetupStatusIsBusinessScopedWithoutWrites(t *testing.T) {
	db := setupOpsNavigationDB(t)
	one := &database.Business{BusinessId: "setup-one", Name: "One", OwnerAddress: "0x", SettlementAddr: "0x1", TippingAddr: "0x2"}
	two := &database.Business{BusinessId: "setup-two", Name: "Two", OwnerAddress: "0x", SettlementAddr: "0x1", TippingAddr: "0x2"}
	require.NoError(t, db.GetGorm().Create(one).Error)
	require.NoError(t, db.GetGorm().Create(two).Error)
	require.NoError(t, db.GetGorm().Create(&database.Table{BusinessID: one.ID, TableCode: "one-1", Name: "One", IsActive: true}).Error)
	require.NoError(t, db.GetGorm().Create(&database.Table{BusinessID: two.ID, TableCode: "two-1", Name: "Two", IsActive: true}).Error)
	require.NoError(t, db.GetGorm().Create(&database.Table{BusinessID: two.ID, TableCode: "two-2", Name: "Two B", IsActive: true}).Error)

	writes := installOpsWriteCounter(t, db.GetGorm())
	result, err := (&ops_tools.SetupStatusTool{}).Run(context.Background(), nil, agents.ToolEnv{
		BusinessID: one.ID, DB: db,
	})
	require.NoError(t, err)
	steps := result.Data["steps"].(map[string]any)
	tables := steps["tables"].(map[string]any)
	assert.EqualValues(t, 1, tables["count"])
	assert.Equal(t, 0, *writes)
}

func TestOpsReadOnlyStateSetupStatusUsesInjectedDatabaseOnly(t *testing.T) {
	injected := setupOpsNavigationDB(t)
	biz := &database.Business{
		BusinessId: "setup-injected", Name: "Injected", OwnerAddress: "0x",
		SettlementAddr: "0x1", TippingAddr: "0x2",
	}
	require.NoError(t, injected.GetGorm().Create(biz).Error)
	require.NoError(t, injected.GetGorm().Create(&database.Table{
		BusinessID: biz.ID, TableCode: "injected-1", Name: "Injected", IsActive: true,
	}).Error)

	globalGorm, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:ops-global-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, globalGorm.AutoMigrate(
		&database.Business{}, &database.Table{}, &database.Menu{}, &database.Staff{},
		&database.Plugin{}, &database.BusinessPlugin{},
	))
	globalBiz := &database.Business{
		BusinessId: "setup-global", Name: "Global", OwnerAddress: "0x",
		SettlementAddr: "0x1", TippingAddr: "0x2",
	}
	require.NoError(t, globalGorm.Create(globalBiz).Error)
	require.Equal(t, biz.ID, globalBiz.ID)
	require.NoError(t, globalGorm.Create(&database.Table{
		BusinessID: biz.ID, TableCode: "global-1", Name: "Global one", IsActive: true,
	}).Error)
	require.NoError(t, globalGorm.Create(&database.Table{
		BusinessID: biz.ID, TableCode: "global-2", Name: "Global two", IsActive: true,
	}).Error)
	database.SetTestDB(globalGorm)
	t.Cleanup(func() { database.SetTestDB(injected.GetGorm()) })

	result, err := (&ops_tools.SetupStatusTool{}).Run(context.Background(), nil, agents.ToolEnv{
		BusinessID: biz.ID, DB: injected,
	})
	require.NoError(t, err)
	steps := result.Data["steps"].(map[string]any)
	tables := steps["tables"].(map[string]any)
	assert.EqualValues(t, 1, tables["count"], "global database contents must not affect injected reads")
}

func TestOpsReadOnlyStateSetupStatusRejectsMissingInjectedDatabase(t *testing.T) {
	db := setupOpsNavigationDB(t)
	biz := &database.Business{
		BusinessId: "setup-missing-db", Name: "Missing DB", OwnerAddress: "0x",
		SettlementAddr: "0x1", TippingAddr: "0x2",
	}
	require.NoError(t, db.GetGorm().Create(biz).Error)

	_, err := (&ops_tools.SetupStatusTool{}).Run(context.Background(), nil, agents.ToolEnv{BusinessID: biz.ID})
	require.ErrorContains(t, err, "missing business context")
}

func setupOpsNavigationDB(t *testing.T) *database.DB {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:ops-navigation-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{}, &database.Table{}, &database.Menu{}, &database.Staff{},
		&database.Plugin{}, &database.BusinessPlugin{},
	))
	return database.GetDBWrapper()
}

func installOpsWriteCounter(t *testing.T, db *gorm.DB) *int {
	t.Helper()
	writes := 0
	name := "ops_read_only_" + fmt.Sprint(time.Now().UnixNano())
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(name+"_create", func(*gorm.DB) { writes++ }))
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(name+"_update", func(*gorm.DB) { writes++ }))
	require.NoError(t, db.Callback().Delete().Before("gorm:delete").Register(name+"_delete", func(*gorm.DB) { writes++ }))
	return &writes
}
