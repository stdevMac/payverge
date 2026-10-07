package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// BenchmarkGetMenuByBusinessCustomUrl_Storefront measures the guest-hot
// storefront menu read (`GET /business/:customUrl/menu`) with the pricing cache
// warm — the shape production serves on every /b/{slug} page load.
//
// Backend Performance Gate evidence for #862: that change clones the cached
// category slice before stamping serve-time 86 flags and folds an orderability
// digest into the ETag, both on this exact path. The bench keeps the cache warm
// (no ResetPricingCache inside the loop) so the clone + digest cost is what
// moves between base and branch.
//
// Deliberately written against helpers that exist on both the base sha and the
// branch so the same file can be run on either.
func BenchmarkGetMenuByBusinessCustomUrl_Storefront(b *testing.B) {
	gin.SetMode(gin.TestMode)
	db := setupBenchmarkStaffHandlerDB(b)
	// Keep GORM query logging out of the measurement.
	db.Logger = logger.Default.LogMode(logger.Silent)
	require.NoError(b, db.AutoMigrate(
		&database.Business{},
		&database.Menu{},
		&database.Table{},
		&database.BusinessOperatingHours{},
		&database.Offer{},
		&database.Bundle{},
		&database.InventoryItem{},
		&database.InventoryRecipe{},
		&database.InventorySettings{},
		&database.Translation{},
	))

	business := createPublicBusinessRouteTestBusiness(b, "bench-storefront", true, true)
	require.NoError(b, db.Model(business).Updates(map[string]interface{}{
		"default_language": "es",
		"default_currency": "ARS",
		"display_currency": "ARS",
		"kitchen_enabled":  true,
		"orders_enabled":   true,
	}).Error)
	require.NoError(b, db.Create(&database.InventorySettings{
		BusinessID:           business.ID,
		InventoryEnabled:     true,
		AvailabilitySyncMode: database.InventoryAvailabilityModeHardBlock,
	}).Error)

	// Open every day so the hours lookup is a hit, not a logged miss.
	for day := 0; day < 7; day++ {
		require.NoError(b, db.Create(&database.BusinessOperatingHours{
			BusinessID: business.ID,
			DayOfWeek:  day,
			OpenTime:   "00:00",
			CloseTime:  "23:59",
			IsClosed:   false,
		}).Error)
	}

	// 6 categories x 20 items = 120-item catalog, the realistic upper end for a
	// full-service venue. A third of them are inventory-backed; one is 86'd.
	const categoryCount, itemsPerCategory = 6, 20
	categories := make([]database.MenuCategory, 0, categoryCount)
	for c := 0; c < categoryCount; c++ {
		items := make([]database.MenuItem, 0, itemsPerCategory)
		for i := 0; i < itemsPerCategory; i++ {
			id := fmt.Sprintf("bench-item-%d-%02d", c, i)
			items = append(items, database.MenuItem{
				ID:          id,
				Name:        fmt.Sprintf("Plato %d-%02d", c, i),
				Description: "Descripcion de plato realista para el benchmark de storefront.",
				Price:       3500 + float64(i)*250,
				Currency:    "ARS",
				IsAvailable: true,
				SortOrder:   i,
			})
		}
		categories = append(categories, database.MenuCategory{
			ID:        fmt.Sprintf("bench-cat-%d", c),
			Name:      fmt.Sprintf("Categoria %d", c),
			SortOrder: c,
			Items:     items,
		})
	}
	raw, err := json.Marshal(categories)
	require.NoError(b, err)
	require.NoError(b, db.Create(&database.Menu{
		BusinessID: business.ID, Categories: string(raw), IsActive: true, Version: 1,
	}).Error)

	for c := 0; c < categoryCount; c++ {
		// One inventory-backed dish per category; category 0's is out of stock
		// so the 86 path is exercised, the rest stay sellable.
		qty := 40.0
		if c == 0 {
			qty = 0
		}
		stock := seedInventoryItem(b, db, business.ID, fmt.Sprintf("Insumo %d", c), qty, true)
		seedRecipe(b, db, business.ID, fmt.Sprintf("bench-item-%d-00", c), stock.ID, 0.5)
	}

	run := func() {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "customUrl", Value: business.CustomURL}}
		c.Request = httptest.NewRequest(http.MethodGet, "/business/"+business.CustomURL+"/menu", nil)
		GetMenuByBusinessCustomUrl(c)
		if w.Code != http.StatusOK {
			b.Fatalf("storefront menu returned %d: %s", w.Code, w.Body.String())
		}
	}

	// Warm the pricing cache so the loop measures the steady-state guest read.
	run()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		run()
	}
}
