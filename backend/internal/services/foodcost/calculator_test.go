package foodcost

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type stubItemStats struct {
	byPeriod map[string][]analytics.ItemStats
	byWindow map[string][]analytics.ItemStats // key: startUTC+"|"+endUTC
	gotStart time.Time
	gotEnd   time.Time
}

func (s *stubItemStats) GetPopularItems(_ uint, limit int, period string, _ *time.Location) ([]analytics.ItemStats, error) {
	rows := s.byPeriod[period]
	if limit > 0 && len(rows) > limit {
		return rows[:limit], nil
	}
	return rows, nil
}

func (s *stubItemStats) GetPopularItemsInWindow(_ uint, limit int, start, end time.Time) ([]analytics.ItemStats, error) {
	s.gotStart, s.gotEnd = start, end
	key := start.UTC().Format(time.RFC3339) + "|" + end.UTC().Format(time.RFC3339)
	rows := s.byWindow[key]
	if rows == nil {
		// Fall back to a single "custom" bucket when tests don't care about keying.
		rows = s.byWindow["custom"]
	}
	if limit > 0 && len(rows) > limit {
		return rows[:limit], nil
	}
	return rows, nil
}

func newCalcDB(t *testing.T) (*database.DB, *gorm.DB) {
	t.Helper()
	prev := database.GetDB()
	t.Cleanup(func() { database.SetTestDB(prev) })
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.InventoryItem{},
		&database.InventoryRecipe{},
		&database.Menu{},
	))
	database.SetTestDB(gormDB)
	return database.GetDBWrapper(), gormDB
}

// seedRecipe creates an inventory item and a recipe line linking it to menuID.
func seedRecipe(t *testing.T, g *gorm.DB, bizID uint, menuID, menuName string, qty, cost float64) {
	t.Helper()
	item := database.InventoryItem{BusinessID: bizID, Name: menuID + "-ing", Unit: "kg", CostPerUnit: cost}
	require.NoError(t, g.Create(&item).Error)
	require.NoError(t, g.Create(&database.InventoryRecipe{
		BusinessID: bizID, MenuItemID: menuID, MenuItemName: menuName,
		InventoryItemID: item.ID, QuantityRequired: qty,
	}).Error)
}

func TestAnalyze_PerItemAndSalesWeightedBlended(t *testing.T) {
	db, g := newCalcDB(t)
	biz := database.Business{Name: "Margin Co"}
	require.NoError(t, g.Create(&biz).Error)

	// Pizza: unit cost 3.00, sells 10.00 -> 30% food cost, 20 sold.
	seedRecipe(t, g, biz.ID, "pizza", "Pizza", 1, 3.0)
	// Steak: unit cost 13.00, sells 20.00 -> 65% food cost, 5 sold.
	seedRecipe(t, g, biz.ID, "steak", "Steak", 1, 13.0)

	items := &stubItemStats{byPeriod: map[string][]analytics.ItemStats{
		"week": {
			{ItemID: "pizza", ItemName: "Pizza", TotalSold: 20, RecognizedQuantity: 20, Revenue: 200, AveragePrice: 10.0},
			{ItemID: "steak", ItemName: "Steak", TotalSold: 5, RecognizedQuantity: 5, Revenue: 100, AveragePrice: 20.0},
		},
	}}

	calc := NewCalculator(db, items)
	report, err := calc.Analyze(biz.ID, "week", nil)
	require.NoError(t, err)

	// Sorted worst-first: Steak (65%) before Pizza (30%).
	require.Len(t, report.Items, 2)
	require.Equal(t, "Steak", report.Items[0].MenuItemName)
	require.InDelta(t, 0.65, report.Items[0].FoodCostPct, 0.0001)
	require.InDelta(t, 7.0, report.Items[0].MarginPerUnit, 0.0001)
	require.Equal(t, "Pizza", report.Items[1].MenuItemName)
	require.InDelta(t, 0.30, report.Items[1].FoodCostPct, 0.0001)

	// Sales-weighted blended: COGS = 3*20 + 13*5 = 125; Revenue = 10*20 + 20*5 = 300.
	require.InDelta(t, 125.0, report.EstimatedCOGS, 0.0001)
	require.InDelta(t, 300.0, report.TotalRevenue, 0.0001)
	require.InDelta(t, 125.0/300.0, report.BlendedFoodCostPct, 0.0001)
	require.Equal(t, 0, report.ItemsMissingCost)
	require.Equal(t, 0, report.ItemsWithoutRecipe)
}

func TestAnalyze_CoverageCountsAndEdgeCases(t *testing.T) {
	db, g := newCalcDB(t)
	biz := database.Business{Name: "Coverage Co"}
	require.NoError(t, g.Create(&biz).Error)

	// Complete + sold.
	seedRecipe(t, g, biz.ID, "soup", "Soup", 1, 2.0)
	// Recipe present but ingredient cost 0 -> incomplete.
	seedRecipe(t, g, biz.ID, "bread", "Bread", 1, 0.0)
	// Recipe present with a real cost, but NOT sold this period.
	seedRecipe(t, g, biz.ID, "salad", "Salad", 1, 5.0)

	items := &stubItemStats{byPeriod: map[string][]analytics.ItemStats{
		"week": {
			{ItemID: "soup", ItemName: "Soup", TotalSold: 4, RecognizedQuantity: 4, Revenue: 40, AveragePrice: 10.0},
			{ItemID: "bread", ItemName: "Bread", TotalSold: 2, RecognizedQuantity: 2, Revenue: 6, AveragePrice: 3.0},
			{ItemID: "cola", ItemName: "Cola", TotalSold: 9, RecognizedQuantity: 9, Revenue: 18, AveragePrice: 2.0}, // sells, NO recipe
		},
	}}

	calc := NewCalculator(db, items)
	report, err := calc.Analyze(biz.ID, "week", nil)
	require.NoError(t, err)

	require.Equal(t, 2, report.ItemsMissingCost, "Bread (zero-cost ingredient) and Cola (sold, no recipe, unit_cost 0)")
	require.Equal(t, 1, report.ItemsWithoutRecipe, "Cola sells but has no recipe")

	// Soup COGS 2*4=8. Headline revenue includes Soup 40 plus Cola 18 (sold
	// with no recipe). Bread is incomplete so it stays out of both sides.
	// Salad has a recipe but no sales, so it must not move the blended metric.
	require.InDelta(t, 8.0/58.0, report.BlendedFoodCostPct, 0.0001)

	// Salad: recipe-mapped but unsold this period. It still appears as a row
	// with zeroed sale figures, and it is counted neither as a coverage gap nor
	// a missing-cost item. Its margin must be 0, NOT -UnitCost: a $0 average
	// price is "no sales", not "we sell it for free" (#797).
	var salad *ItemMargin
	for i := range report.Items {
		if report.Items[i].MenuItemID == "salad" {
			salad = &report.Items[i]
			break
		}
	}
	require.NotNil(t, salad, "unsold recipe-mapped item should still appear in report.Items")
	require.Equal(t, 0, salad.QtySold)
	require.InDelta(t, 0.0, salad.AvgPrice, 0.0001)
	require.InDelta(t, 0.0, salad.FoodCostPct, 0.0001)
	require.InDelta(t, 0.0, salad.MarginPerUnit, 0.0001,
		"unsold item must not get margin -UnitCost from a $0 average price")
	require.True(t, salad.HasCompleteCost, "Salad's ingredient cost is > 0")
	require.True(t, report.HasSales, "Soup/Bread/Cola sold this period")
}

// items_missing_cost must count every SOLD SKU whose unit cost is 0 or unknown:
// incomplete recipe cost (Bread), no recipe at all (Cola/Ojo), or a recipe that
// computes unitCost==0 while HasCost is true (qty_required 0). Unsold complete
// dishes (Salad) stay out of the counter. Regression for #859, where venue 142
// reported items_missing_cost=0 while Fernet/Flan/Milanesa/Ojo sold at unit_cost 0.
func TestAnalyze_SoldZeroOrUnknownUnitCostIncrementsMissingCost(t *testing.T) {
	db, g := newCalcDB(t)
	biz := database.Business{Name: "Missing Cost Co"}
	require.NoError(t, g.Create(&biz).Error)

	// a) Sold SKU with unit_cost 0 because its recipe ingredient CostPerUnit is 0.
	seedRecipe(t, g, biz.ID, "bread", "Bread", 1, 0.0)
	// c) Sold recipe-mapped item: CostPerUnit>0 but QuantityRequired==0 → unitCost 0,
	// complete==true (HasCost is CostPerUnit>0). Must still count as missing cost.
	seedRecipe(t, g, biz.ID, "zero-qty", "ZeroQty", 0, 4.0)
	// Unsold complete-cost item must NOT increment.
	seedRecipe(t, g, biz.ID, "salad", "Salad", 1, 5.0)
	// Sold complete-cost item must NOT increment.
	seedRecipe(t, g, biz.ID, "soup", "Soup", 1, 2.0)

	items := &stubItemStats{byPeriod: map[string][]analytics.ItemStats{
		"week": {
			{ItemID: "bread", ItemName: "Bread", TotalSold: 2, RecognizedQuantity: 2, Revenue: 6, AveragePrice: 3.0},
			{ItemID: "zero-qty", ItemName: "ZeroQty", TotalSold: 3, RecognizedQuantity: 3, Revenue: 15, AveragePrice: 5.0},
			{ItemID: "cola", ItemName: "Cola", TotalSold: 9, RecognizedQuantity: 9, Revenue: 18, AveragePrice: 2.0}, // b) sold, no recipe
			{ItemID: "soup", ItemName: "Soup", TotalSold: 4, RecognizedQuantity: 4, Revenue: 40, AveragePrice: 10.0},
		},
	}}

	calc := NewCalculator(db, items)
	report, err := calc.Analyze(biz.ID, "week", nil)
	require.NoError(t, err)

	byID := make(map[string]ItemMargin, len(report.Items))
	for _, it := range report.Items {
		byID[it.MenuItemID] = it
	}

	bread, ok := byID["bread"]
	require.True(t, ok, "Bread must appear in report.Items")
	require.InDelta(t, 0.0, bread.UnitCost, 0.0001)
	require.False(t, bread.HasCompleteCost)
	require.Equal(t, 2, bread.QtySold)

	cola, ok := byID["cola"]
	require.True(t, ok, "sold SKU with no recipe must appear in report.Items")
	require.InDelta(t, 0.0, cola.UnitCost, 0.0001)
	require.False(t, cola.HasCompleteCost)
	require.Equal(t, 9, cola.QtySold)

	zeroQty, ok := byID["zero-qty"]
	require.True(t, ok, "qty_required 0 recipe item must appear in report.Items")
	require.InDelta(t, 0.0, zeroQty.UnitCost, 0.0001)
	require.True(t, zeroQty.HasCompleteCost, "HasCost is true when CostPerUnit>0 even if qty_required is 0")
	require.Equal(t, 3, zeroQty.QtySold)

	salad, ok := byID["salad"]
	require.True(t, ok, "unsold complete-cost item still appears")
	require.Equal(t, 0, salad.QtySold)
	require.True(t, salad.HasCompleteCost)
	require.Greater(t, salad.UnitCost, 0.0)

	soup, ok := byID["soup"]
	require.True(t, ok)
	require.Equal(t, 4, soup.QtySold)
	require.True(t, soup.HasCompleteCost)
	require.Greater(t, soup.UnitCost, 0.0)

	require.Equal(t, 3, report.ItemsMissingCost,
		"Bread (zero ingredient cost) + Cola (no recipe) + ZeroQty (computed unitCost 0)")
	require.Equal(t, 1, report.ItemsWithoutRecipe, "Cola is still the coverage-gap counter")
}

// A no-sales window must say so (HasSales=false) and must not fabricate
// negative per-unit margins from $0 average prices. Regression for #797, where
// a quiet week painted Harvest Bowl -1.05 / Steak -7.52 margins and the UI
// presented cost math for sales that never happened.
func TestAnalyze_NoSalesWindowHasNoNegativeMargins(t *testing.T) {
	db, g := newCalcDB(t)
	biz := database.Business{Name: "Quiet Week Co"}
	require.NoError(t, g.Create(&biz).Error)

	seedRecipe(t, g, biz.ID, "bowl", "Harvest Bowl", 1, 1.05)
	seedRecipe(t, g, biz.ID, "steak", "Steak", 1, 7.52)
	seedRecipe(t, g, biz.ID, "tea", "Tea", 1, 0.378)

	// No stats at all for the window: a genuinely sale-free week.
	items := &stubItemStats{byPeriod: map[string][]analytics.ItemStats{"week": {}}}

	calc := NewCalculator(db, items)
	report, err := calc.Analyze(biz.ID, "week", nil)
	require.NoError(t, err)

	require.False(t, report.HasSales, "no stats -> window has no recognized sales")
	require.InDelta(t, 0.0, report.TotalRevenue, 0.0001)
	require.InDelta(t, 0.0, report.EstimatedCOGS, 0.0001)
	require.Len(t, report.Items, 3)
	for _, it := range report.Items {
		require.Equal(t, 0, it.QtySold)
		require.InDelta(t, 0.0, it.AvgPrice, 0.0001)
		require.GreaterOrEqual(t, it.MarginPerUnit, 0.0,
			"item %s must not carry a negative margin fabricated from $0 avg price", it.MenuItemID)
		require.InDelta(t, 0.0, it.MarginPerUnit, 0.0001)
	}

	// Wire contract: the report says has_sales so surfaces can render an
	// honest "no sales this period" instead of cost math over nothing.
	blob, err := json.Marshal(report)
	require.NoError(t, err)
	require.Contains(t, string(blob), `"has_sales":false`)
}

// Manual plate COGS (MenuItem.Cogs) covers dishes without a recipe so food-cost
// coverage is not recipe-or-nothing (Task 35).
func TestAnalyze_UsesMenuItemCogsWhenNoRecipe(t *testing.T) {
	db, g := newCalcDB(t)
	biz := database.Business{Name: "Cogs Co"}
	require.NoError(t, g.Create(&biz).Error)

	// Recipe for pizza wins over any plate COGS.
	seedRecipe(t, g, biz.ID, "pizza", "Pizza", 1, 3.0)
	// Cola has only plate COGS (no recipe).
	cats := []database.MenuCategory{{
		Name: "Drinks",
		Items: []database.MenuItem{
			{ID: "cola", Name: "Cola", Price: 2.0, Cogs: 0.40, IsAvailable: true},
			// Pizza also has a plate cogs, but recipe must win (unit cost stays 3).
			{ID: "pizza", Name: "Pizza", Price: 10.0, Cogs: 99.0, IsAvailable: true},
		},
	}}
	raw, err := json.Marshal(cats)
	require.NoError(t, err)
	require.NoError(t, g.Create(&database.Menu{
		BusinessID: biz.ID, Categories: string(raw), IsActive: true, Version: 1,
	}).Error)

	items := &stubItemStats{byPeriod: map[string][]analytics.ItemStats{
		"week": {
			{ItemID: "pizza", ItemName: "Pizza", TotalSold: 10, RecognizedQuantity: 10, Revenue: 100, AveragePrice: 10.0},
			{ItemID: "cola", ItemName: "Cola", TotalSold: 20, RecognizedQuantity: 20, Revenue: 40, AveragePrice: 2.0},
		},
	}}

	calc := NewCalculator(db, items)
	report, err := calc.Analyze(biz.ID, "week", nil)
	require.NoError(t, err)

	var cola, pizza *ItemMargin
	for i := range report.Items {
		switch report.Items[i].MenuItemID {
		case "cola":
			cola = &report.Items[i]
		case "pizza":
			pizza = &report.Items[i]
		}
	}
	require.NotNil(t, cola, "plate-cogs item should appear without a recipe")
	require.InDelta(t, 0.40, cola.UnitCost, 0.0001)
	require.True(t, cola.HasCompleteCost)
	require.InDelta(t, 0.40/2.0, cola.FoodCostPct, 0.0001)

	require.NotNil(t, pizza)
	require.InDelta(t, 3.0, pizza.UnitCost, 0.0001, "recipe cost must win over plate COGS")

	// Cola no longer counts as without-recipe coverage gap.
	require.Equal(t, 0, report.ItemsWithoutRecipe)
	// COGS = pizza 3*10 + cola 0.4*20 = 30 + 8 = 38; rev = 100 + 40 = 140
	require.InDelta(t, 38.0, report.EstimatedCOGS, 0.0001)
}

// A cold-start business with no recipe-mapped items must still return a
// non-nil Items slice so the report serializes as "items":[] and not
// "items":null. The accounting tab FE reads foodCost.items.length, and a null
// there throws "Cannot read properties of null (reading 'length')", blanking
// the whole tab. Regression for that production crash.
func TestAnalyze_EmptyReportSerializesItemsAsArray(t *testing.T) {
	db, g := newCalcDB(t)
	biz := database.Business{Name: "Cold Start Co"}
	require.NoError(t, g.Create(&biz).Error)

	// Items sold this period, but none has a recipe. They still appear as
	// coverage-gap rows so total_revenue is the full sales slice.
	items := &stubItemStats{byPeriod: map[string][]analytics.ItemStats{
		"week": {
			{ItemID: "cola", ItemName: "Cola", TotalSold: 9, RecognizedQuantity: 9, Revenue: 18, AveragePrice: 2.0},
		},
	}}

	calc := NewCalculator(db, items)
	report, err := calc.Analyze(biz.ID, "week", nil)
	require.NoError(t, err)

	require.NotNil(t, report.Items, "Items must be non-nil so it serializes as [] not null")
	require.Len(t, report.Items, 1)
	require.Equal(t, 1, report.ItemsWithoutRecipe, "Cola sells but has no recipe")
	require.InDelta(t, 18.0, report.TotalRevenue, 0.0001)
	require.InDelta(t, 0, report.EstimatedCOGS, 0.0001)
	require.False(t, report.Items[0].HasCompleteCost)

	// Assert the actual wire contract the FE depends on.
	blob, err := json.Marshal(report)
	require.NoError(t, err)
	require.NotContains(t, string(blob), `"items":null`)
}

func TestAnalyze_SoldItemsWithoutRecipeCountTowardTotalRevenue(t *testing.T) {
	db, g := newCalcDB(t)
	biz := database.Business{Name: "Partial Recipes Co"}
	require.NoError(t, g.Create(&biz).Error)
	seedRecipe(t, g, biz.ID, "pizza", "Pizza", 1, 3.0)

	items := &stubItemStats{byPeriod: map[string][]analytics.ItemStats{
		"week": {
			{ItemID: "pizza", ItemName: "Pizza", TotalSold: 10, RecognizedQuantity: 10, Revenue: 100, AveragePrice: 10.0},
			{ItemID: "cola", ItemName: "Cola", TotalSold: 20, RecognizedQuantity: 20, Revenue: 40, AveragePrice: 2.0},
		},
	}}

	calc := NewCalculator(db, items)
	report, err := calc.Analyze(biz.ID, "week", nil)
	require.NoError(t, err)

	require.Equal(t, 1, report.ItemsWithoutRecipe)
	require.InDelta(t, 30.0, report.EstimatedCOGS, 0.0001)
	require.InDelta(t, 140.0, report.TotalRevenue, 0.0001, "cola sales must stay in the headline denominator")
	require.InDelta(t, 30.0/140.0, report.BlendedFoodCostPct, 0.0001)
}

func TestAnalyze_RejectsBadPeriod(t *testing.T) {
	db, g := newCalcDB(t)
	biz := database.Business{Name: "Bad Period Co"}
	require.NoError(t, g.Create(&biz).Error)
	calc := NewCalculator(db, &stubItemStats{})
	_, err := calc.Analyze(biz.ID, "fortnight", nil)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrUnsupportedPeriod)
}

// TestAnalyzeWindow_UsesExplicitRange proves custom start/end does not fall
// back to a period preset — the window provider receives the exact bounds.
func TestAnalyzeWindow_UsesExplicitRange(t *testing.T) {
	db, g := newCalcDB(t)
	biz := database.Business{Name: "Window Co"}
	require.NoError(t, g.Create(&biz).Error)
	seedRecipe(t, g, biz.ID, "burger", "Burger", 0.3, 10.0)

	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	items := &stubItemStats{
		byWindow: map[string][]analytics.ItemStats{
			"custom": {
				{ItemID: "burger", ItemName: "Burger", TotalSold: 10, RecognizedQuantity: 10, Revenue: 150, AveragePrice: 15.0},
			},
		},
	}
	calc := NewCalculator(db, items)
	report, err := calc.AnalyzeWindow(biz.ID, start, end)
	require.NoError(t, err)
	require.True(t, items.gotStart.Equal(start))
	require.True(t, items.gotEnd.Equal(end))
	require.InDelta(t, 0.2, report.BlendedFoodCostPct, 0.001) // unit 3 / price 15
	require.InDelta(t, 150.0, report.TotalRevenue, 0.01)
	require.Contains(t, report.Period, "2026-07-01")
}

// Production analytics keys ItemStats by bill_items.name ("Use name as ID
// since we aggregated by name" — analytics/service.go), NOT by menu-item ID.
// The calculator must also match recipes by MenuItemName, or every real
// business reads 0% food cost / $0 COGS / $0 revenue while labor shows sales.
func TestAnalyze_MatchesStatsKeyedByItemName(t *testing.T) {
	db, g := newCalcDB(t)
	biz := database.Business{Name: "Name Key Co"}
	require.NoError(t, g.Create(&biz).Error)

	seedRecipe(t, g, biz.ID, "demo-pizza", "Pizza", 1, 3.0)
	seedRecipe(t, g, biz.ID, "demo-steak", "Steak", 1, 13.0)

	// Stats keyed by display name, exactly as analytics.GetPopularItems emits.
	items := &stubItemStats{byPeriod: map[string][]analytics.ItemStats{
		"week": {
			{ItemID: "Pizza", ItemName: "Pizza", TotalSold: 20, RecognizedQuantity: 20, Revenue: 200, AveragePrice: 10.0},
			{ItemID: "Steak", ItemName: "Steak", TotalSold: 5, RecognizedQuantity: 5, Revenue: 100, AveragePrice: 20.0},
		},
	}}

	calc := NewCalculator(db, items)
	report, err := calc.Analyze(biz.ID, "week", nil)
	require.NoError(t, err)

	require.InDelta(t, 125.0, report.EstimatedCOGS, 0.0001)
	require.InDelta(t, 300.0, report.TotalRevenue, 0.0001)
	require.InDelta(t, 125.0/300.0, report.BlendedFoodCostPct, 0.0001)
	// Both stats matched recipes via the name fallback — nothing is "without
	// a recipe".
	require.Equal(t, 0, report.ItemsWithoutRecipe)
}

// TestAnalyze_DuplicateNameStatCountedOnce guards against double-counting when
// two distinct menu items share a display name. analytics.GetPopularItems
// aggregates bill_items by name, so both recipe items resolve to the SAME
// name-keyed stat. The stat's sales must be claimed by exactly one item, not
// added to the blended COGS/revenue twice — otherwise food-cost total_revenue
// exceeds the analytics revenue for the same window.
func TestAnalyze_DuplicateNameStatCountedOnce(t *testing.T) {
	db, g := newCalcDB(t)
	biz := database.Business{Name: "Dup Name Co"}
	require.NoError(t, g.Create(&biz).Error)

	// Two distinct MenuItemIDs, same display name "Burger", each with its own recipe.
	seedRecipe(t, g, biz.ID, "burger-lunch", "Burger", 1, 4.0)
	seedRecipe(t, g, biz.ID, "burger-dinner", "Burger", 1, 4.0)

	// analytics emits ONE "Burger" stat (aggregated by name): 100 sold, $1500.
	items := &stubItemStats{byPeriod: map[string][]analytics.ItemStats{
		"week": {
			{ItemID: "Burger", ItemName: "Burger", TotalSold: 100, RecognizedQuantity: 100, Revenue: 1500, AveragePrice: 15.0},
		},
	}}

	calc := NewCalculator(db, items)
	report, err := calc.Analyze(biz.ID, "week", nil)
	require.NoError(t, err)

	// The 100 burgers must be counted once: COGS 100*4=400, revenue 100*15=1500.
	// Before the fix both items claimed QtySold=100 → COGS 800, revenue 3000.
	require.InDelta(t, 400.0, report.EstimatedCOGS, 0.0001, "COGS must not double-count the shared stat")
	require.InDelta(t, 1500.0, report.TotalRevenue, 0.0001, "revenue must match the analytics stat, not 2x it")
	require.InDelta(t, 400.0/1500.0, report.BlendedFoodCostPct, 0.0001)
}

// A combo component is priced at 0 (revenue lives on the parent line) but its
// recipe is still consumed. COGS must include that cost; the parent's revenue
// still lands in the headline total.
func TestAnalyze_BundleComponentCostWithoutOwnPrice(t *testing.T) {
	db, g := newCalcDB(t)
	biz := database.Business{Name: "Combo Co"}
	require.NoError(t, g.Create(&biz).Error)
	seedRecipe(t, g, biz.ID, "combo-fries", "Combo Fries", 1, 1.0)

	items := &stubItemStats{byPeriod: map[string][]analytics.ItemStats{
		"week": {
			{ItemID: "combo-fries", ItemName: "Combo Fries", TotalSold: 3, RecognizedQuantity: 3, AveragePrice: 0},
			{ItemID: "combo-lunch", ItemName: "Lunch Combo", TotalSold: 1, RecognizedQuantity: 1, Revenue: 10, AveragePrice: 10},
		},
	}}

	calc := NewCalculator(db, items)
	report, err := calc.Analyze(biz.ID, "week", nil)
	require.NoError(t, err)

	require.True(t, report.HasSales)
	require.InDelta(t, 3.0, report.EstimatedCOGS, 0.0001, "3 recognized component units at $1 ingredient cost")
	require.InDelta(t, 10.0, report.TotalRevenue, 0.0001, "combo revenue stays on the parent line")

	var component *ItemMargin
	for i := range report.Items {
		if report.Items[i].MenuItemID == "combo-fries" {
			component = &report.Items[i]
			break
		}
	}
	require.NotNil(t, component)
	require.Equal(t, 3, component.QtySold)
	require.InDelta(t, 0, component.AvgPrice, 0.0001)
	require.True(t, component.HasCompleteCost)
}
