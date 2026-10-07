package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPluginListTestDB(t *testing.T) {
	t.Helper()
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&Business{}, &Plugin{}, &BusinessPlugin{}))
	SetTestDB(gormDB)
}

func TestListBusinessesWithPluginEnabled_OnlyEnabledActive(t *testing.T) {
	setupPluginListTestDB(t)

	mp, err := CreatePlugin(Plugin{
		Name: "mercadopago", DisplayName: "MP", Category: PluginCategoryPayment, IsActive: true,
	})
	require.NoError(t, err)
	other, err := CreatePlugin(Plugin{
		Name: "stripe", DisplayName: "Stripe", Category: PluginCategoryPayment, IsActive: true,
	})
	require.NoError(t, err)
	inactive, err := CreatePlugin(Plugin{
		Name: "paypal", DisplayName: "PayPal", Category: PluginCategoryPayment, IsActive: true,
	})
	require.NoError(t, err)
	// GORM default:true on IsActive ignores false on Create; force inactive after insert.
	require.NoError(t, GetDB().Model(inactive).Update("is_active", false).Error)

	// business 1: mercadopago enabled
	require.NoError(t, EnableBusinessPlugin(1, mp.ID, map[string]interface{}{"connection_mode": "oauth"}))
	// business 2: mercadopago enabled
	require.NoError(t, EnableBusinessPlugin(2, mp.ID, map[string]interface{}{"connection_mode": "manual"}))
	// business 3: mercadopago disabled
	require.NoError(t, EnableBusinessPlugin(3, mp.ID, map[string]interface{}{}))
	require.NoError(t, DisableBusinessPlugin(3, mp.ID))
	// business 4: only stripe
	require.NoError(t, EnableBusinessPlugin(4, other.ID, map[string]interface{}{}))
	// business 5: inactive catalog plugin
	require.NoError(t, EnableBusinessPlugin(5, inactive.ID, map[string]interface{}{}))

	ids, err := ListBusinessesWithPluginEnabled("mercadopago")
	require.NoError(t, err)
	assert.ElementsMatch(t, []uint{1, 2}, ids)

	ids, err = ListBusinessesWithPluginEnabled("stripe")
	require.NoError(t, err)
	assert.Equal(t, []uint{4}, ids)

	ids, err = ListBusinessesWithPluginEnabled("paypal")
	require.NoError(t, err)
	assert.Empty(t, ids)

	ids, err = ListBusinessesWithPluginEnabled("does-not-exist")
	require.NoError(t, err)
	assert.Empty(t, ids)
}
