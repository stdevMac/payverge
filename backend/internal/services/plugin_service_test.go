package services

import (
	"github.com/stdevmac/payverge/backend/internal/database"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPluginServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := gormDB.AutoMigrate(&database.Business{}, &database.Plugin{}, &database.BusinessPlugin{}); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}
	database.SetTestDB(gormDB)

	biz := database.Business{Name: "Test Restaurant", Email: "restaurant@example.com"}
	biz.ID = 100
	if err := gormDB.Create(&biz).Error; err != nil {
		t.Fatalf("failed to create test business: %v", err)
	}

	plugins := []database.Plugin{
		{Name: PluginNameUSDCPayment, DisplayName: "USDC Payment", IsActive: true},
		{Name: PluginNameCrossChainPayment, DisplayName: "Any Token Payment", IsActive: true},
		{Name: PluginNameStripe, DisplayName: "Stripe", IsActive: true},
	}
	for _, p := range plugins {
		if err := gormDB.Create(&p).Error; err != nil {
			t.Fatalf("failed to create plugin %s: %v", p.Name, err)
		}
	}

	return gormDB
}

func TestEnableDefaultPaymentPluginsIsNoOp(t *testing.T) {
	gormDB := setupPluginServiceTestDB(t)
	ps := NewPluginService(nil)

	if err := ps.EnableDefaultPaymentPlugins(100); err != nil {
		t.Fatalf("EnableDefaultPaymentPlugins returned error: %v", err)
	}
	if err := ps.EnableDefaultPaymentPlugins(100); err != nil {
		t.Fatalf("second EnableDefaultPaymentPlugins call returned error: %v", err)
	}

	var bpCount int64
	gormDB.Model(&database.BusinessPlugin{}).Where("business_id = ?", 100).Count(&bpCount)
	if bpCount != 0 {
		t.Fatalf("expected crypto auto-enable no-op, found %d business_plugin rows", bpCount)
	}
}
