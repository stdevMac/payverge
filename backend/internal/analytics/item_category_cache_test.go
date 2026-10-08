package analytics

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedPulseMenu creates an active menu with translations and one paid bill
// whose item ("Taco") resolves through the menu to a translated category.
func seedPulseMenu(t *testing.T, db *database.DB, fixedNow time.Time) database.Business {
	t.Helper()
	require.NoError(t, db.GetGorm().AutoMigrate(&database.Menu{}, &database.Translation{}))
	createAnalyticsBillItemsTable(t, db)
	business := database.Business{Name: "Pulse Cache Restaurant", DefaultLanguage: "en"}
	require.NoError(t, db.GetGorm().Create(&business).Error)
	require.NoError(t, db.MenuService.Create(&database.Menu{BusinessID: business.ID, IsActive: true}, []database.MenuCategory{
		{Name: "Comida", Items: []database.MenuItem{{Name: "Taco"}}},
	}))
	require.NoError(t, db.GetGorm().Create(&database.Translation{
		BusinessID: business.ID, EntityType: "category", EntityID: 0, FieldName: "name", LanguageCode: "en", TranslatedText: "Food",
	}).Error)

	confirmedAt := fixedNow.Add(-2 * time.Hour)
	bill := createAnalyticsBill(t, db, business.ID, "PULSE-CACHE", 5000, 5000, 0, database.BillStatusPaid, confirmedAt.Add(-time.Hour), confirmedAt)
	require.NoError(t, db.GetGorm().Create(&database.BillItem{
		ID: "pulse-cache-taco", BillID: bill.ID, Name: "Taco", Price: 50, Quantity: 1, Subtotal: 50, CreatedAt: confirmedAt,
	}).Error)
	require.NoError(t, db.GetGorm().Create(&database.Payment{
		BillID: bill.ID, PayerAddr: "0xpulsecache", Amount: 5000, TxHash: "analytics_pulse_cache",
		Status: database.PaymentStatusConfirmed, PaymentMethod: "crypto",
		ConfirmedAt: &confirmedAt, CreatedAt: confirmedAt, UpdatedAt: confirmedAt,
	}).Error)
	return business
}

func menuBlobReads(sqls []string) int {
	n := 0
	for _, sql := range sqls {
		normalized := strings.ToLower(sql)
		if strings.Contains(normalized, "menus") && strings.Contains(normalized, "select *") {
			n++
		}
	}
	return n
}

func translationReads(sqls []string) int {
	n := 0
	for _, sql := range sqls {
		normalized := strings.ToLower(sql)
		if strings.Contains(normalized, "from") && strings.Contains(normalized, "translations") {
			n++
		}
	}
	return n
}

// perf-dashboard-pulse: the item→category map is rebuilt from the whole menu
// JSON blob plus two translation reads on every dashboard pulse. The second
// call for an unchanged menu version must not read the blob or translations.
func TestGetPopularItemsCachesCategoryMapByMenuVersion(t *testing.T) {
	recorder := &recordingAnalyticsLogger{}
	db := setupAnalyticsTestDBWithLogger(t, recorder)
	fixedNow := time.Date(2026, 5, 11, 20, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(db).WithClock(func() time.Time { return fixedNow })
	business := seedPulseMenu(t, db, fixedNow)

	recorder.Reset()
	items, err := service.GetPopularItems(business.ID, 5, "today", time.UTC)
	require.NoError(t, err)
	require.Equal(t, "Food", requireItemStats(t, items, "Taco").Category)
	assert.Equal(t, 1, menuBlobReads(recorder.SQLs()), "first call loads the menu")

	recorder.Reset()
	items, err = service.GetPopularItems(business.ID, 5, "today", time.UTC)
	require.NoError(t, err)
	require.Equal(t, "Food", requireItemStats(t, items, "Taco").Category)
	assert.Equal(t, 0, menuBlobReads(recorder.SQLs()), "second call must not reload the menu blob: %v", recorder.SQLs())
	assert.Equal(t, 0, translationReads(recorder.SQLs()), "second call must not reload translations")

	// A menu write bumps the version: the next call rebuilds the map.
	require.NoError(t, db.GetGorm().Model(&database.Menu{}).Where("business_id = ?", business.ID).Updates(map[string]any{
		"categories": `[{"id":"c1","name":"Street","items":[{"id":"i1","name":"Taco"}]}]`,
		"version":    2,
	}).Error)
	require.NoError(t, db.GetGorm().Where("business_id = ?", business.ID).Delete(&database.Translation{}).Error)

	recorder.Reset()
	items, err = service.GetPopularItems(business.ID, 5, "today", time.UTC)
	require.NoError(t, err)
	assert.Equal(t, "Street", requireItemStats(t, items, "Taco").Category)
	assert.Equal(t, 1, menuBlobReads(recorder.SQLs()))
}

// BenchmarkGetPopularItemsMenuMapping measures a dashboard pulse against a
// 20x30 menu with translations (perf-dashboard-pulse).
func BenchmarkGetPopularItemsMenuMapping(b *testing.B) {
	db, businessID := setupPopularItemsBenchmarkDB(b)
	if err := db.GetGorm().AutoMigrate(&database.Menu{}, &database.Translation{}); err != nil {
		b.Fatalf("migrate menu: %v", err)
	}
	if err := db.GetGorm().Model(&database.Business{}).Where("id = ?", businessID).Update("default_language", "en").Error; err != nil {
		b.Fatalf("set language: %v", err)
	}
	categories := make([]database.MenuCategory, 0, 20)
	var translations []database.Translation
	for c := 0; c < 20; c++ {
		cat := database.MenuCategory{Name: fmt.Sprintf("Cat%d", c), Description: strings.Repeat("d", 200)}
		for i := 0; i < 30; i++ {
			cat.Items = append(cat.Items, database.MenuItem{Name: fmt.Sprintf("Item%d", c*30+i), Description: strings.Repeat("x", 300)})
			translations = append(translations, database.Translation{BusinessID: businessID, EntityType: "menu_item", EntityID: uint(c*1000 + i), FieldName: "name", LanguageCode: "en", TranslatedText: fmt.Sprintf("Item %d EN", c*30+i)})
		}
		translations = append(translations, database.Translation{BusinessID: businessID, EntityType: "category", EntityID: uint(c), FieldName: "name", LanguageCode: "en", TranslatedText: fmt.Sprintf("Category %d", c)})
		categories = append(categories, cat)
	}
	if err := db.MenuService.Create(&database.Menu{BusinessID: businessID, IsActive: true}, categories); err != nil {
		b.Fatalf("create menu: %v", err)
	}
	if err := db.GetGorm().CreateInBatches(&translations, 200).Error; err != nil {
		b.Fatalf("create translations: %v", err)
	}
	service := NewAnalyticsService(db).WithClock(func() time.Time {
		return time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := service.GetPopularItems(businessID, 5, "week", time.UTC); err != nil {
			b.Fatalf("GetPopularItems: %v", err)
		}
	}
}
