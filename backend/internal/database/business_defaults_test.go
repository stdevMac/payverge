package database

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestNewBusinessDefaultsKitchenAndOrdersEnabled(t *testing.T) {
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(&Business{}))

	biz := Business{Name: "Default Flags Cafe"}
	require.NoError(t, gormDB.Create(&biz).Error)

	var loaded Business
	require.NoError(t, gormDB.First(&loaded, biz.ID).Error)
	require.True(t, loaded.KitchenEnabled, "kitchen_enabled must default ON for new businesses")
	require.True(t, loaded.OrdersEnabled, "orders_enabled must default ON for new businesses")
	require.False(t, loaded.CRMEnabled, "crm_enabled default must stay OFF")
}
