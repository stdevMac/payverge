package server

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// availDBSeq gives every test/benchmark invocation a unique in-memory DB name so
// `-count>1` reruns don't collide on the shared-cache sqlite instance.
var availDBSeq atomic.Int64

// setupInventoryAvailabilityTestDB migrates the tables the auto-hide path
// touches (Business + Menu + inventory recipe/item/settings) onto a shared
// in-memory sqlite DB and installs it as the package test DB.
func setupInventoryAvailabilityTestDB(t testing.TB) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), availDBSeq.Add(1))
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Menu{},
		&database.InventoryItem{},
		&database.InventoryRecipe{},
		&database.InventorySettings{},
	))
	database.SetTestDB(gormDB)
	return gormDB
}

// seedAvailabilityBusiness creates a business with inventory tracking enabled in
// the given AvailabilitySyncMode (warn | manual | hard_block).
func seedAvailabilityBusiness(t testing.TB, db *gorm.DB, syncMode string) *database.Business {
	t.Helper()
	biz := &database.Business{
		BusinessId:   fmt.Sprintf("biz-%s-%d", t.Name(), availDBSeq.Load()),
		Name:         "Avail Test",
		OwnerAddress: "0xowner",
	}
	require.NoError(t, db.Create(biz).Error)
	require.NoError(t, db.Create(&database.InventorySettings{
		BusinessID:           biz.ID,
		InventoryEnabled:     true,
		AvailabilitySyncMode: syncMode,
	}).Error)
	return biz
}

func seedInventoryItem(t testing.TB, db *gorm.DB, businessID uint, name string, qty float64, active bool) *database.InventoryItem {
	t.Helper()
	item := &database.InventoryItem{
		BusinessID:      businessID,
		Name:            name,
		Unit:            "unit",
		CurrentQuantity: qty,
		IsActive:        active,
	}
	require.NoError(t, db.Create(item).Error)
	if !active {
		// GORM omits false zero-values on Create when the column has a
		// default:true tag; force it with raw SQL.
		require.NoError(t, db.Exec(
			"UPDATE inventory_items SET is_active = 0 WHERE id = ?", item.ID,
		).Error)
	}
	return item
}

func seedRecipe(t testing.TB, db *gorm.DB, businessID uint, menuItemID string, inventoryItemID uint, required float64) {
	t.Helper()
	seedRecipeNamed(t, db, businessID, menuItemID, "", inventoryItemID, required)
}

func seedRecipeNamed(t testing.TB, db *gorm.DB, businessID uint, menuItemID, menuItemName string, inventoryItemID uint, required float64) {
	t.Helper()
	require.NoError(t, db.Create(&database.InventoryRecipe{
		BusinessID:       businessID,
		MenuItemID:       menuItemID,
		MenuItemName:     menuItemName,
		InventoryItemID:  inventoryItemID,
		QuantityRequired: required,
	}).Error)
}

// TestMenuFiltersDishesWithOutOfStockIngredients asserts the canonical
// out-of-stock semantics of database.OutOfStockMenuItemIDs: depleted stock and
// inactive/missing ingredients mark a dish out of stock; ample stock, exactly
// enough stock, and non-constraining (quantity_required <= 0) recipes do not.
func TestMenuFiltersDishesWithOutOfStockIngredients(t *testing.T) {
	db := setupInventoryAvailabilityTestDB(t)
	biz := seedAvailabilityBusiness(t, db, database.InventoryAvailabilityModeHardBlock)

	// Steak needs 1 beef; 0 in stock → out of stock.
	beef := seedInventoryItem(t, db, biz.ID, "Premium Beef", 0, true)
	seedRecipe(t, db, biz.ID, "steak", beef.ID, 1)

	// Salad needs 2 lettuce; 5 in stock → available.
	lettuce := seedInventoryItem(t, db, biz.ID, "Lettuce", 5, true)
	seedRecipe(t, db, biz.ID, "salad", lettuce.ID, 2)

	// Soup needs 3 stock units; exactly 3 in stock → available (strict <).
	stock := seedInventoryItem(t, db, biz.ID, "Stock", 3, true)
	seedRecipe(t, db, biz.ID, "soup", stock.ID, 3)

	// Ceviche's fish is INACTIVE → out of stock (canonical: missing/inactive
	// ingredient blocks, matching GetInventorySummary).
	fish := seedInventoryItem(t, db, biz.ID, "Fish", 10, false)
	seedRecipe(t, db, biz.ID, "ceviche", fish.ID, 1)

	// Burger's recipe row points at a DELETED inventory item → out of stock.
	seedRecipe(t, db, biz.ID, "burger", 999999, 1)

	// Garnish recipe has quantity_required = 0 → never constrains.
	empty := seedInventoryItem(t, db, biz.ID, "Herbs", 0, true)
	seedRecipe(t, db, biz.ID, "garnish", empty.ID, 0)

	unavailable, err := database.OutOfStockMenuItemIDs(biz.ID)
	require.NoError(t, err)
	require.Contains(t, unavailable, "steak")
	require.Contains(t, unavailable, "ceviche")
	require.Contains(t, unavailable, "burger")
	require.NotContains(t, unavailable, "salad")
	require.NotContains(t, unavailable, "soup", "exactly enough stock must count as available")
	require.NotContains(t, unavailable, "garnish", "quantity_required<=0 must not constrain")
}

// TestMenuAvailability_TenantScoped asserts a recipe never matches an inventory
// row from another business (the JOIN is scoped by business_id on both sides).
func TestMenuAvailability_TenantScoped(t *testing.T) {
	db := setupInventoryAvailabilityTestDB(t)
	bizA := seedAvailabilityBusiness(t, db, database.InventoryAvailabilityModeHardBlock)

	bizB := &database.Business{BusinessId: "biz-b", Name: "B", OwnerAddress: "0xb"}
	require.NoError(t, db.Create(bizB).Error)

	// Business A has an empty-stock ingredient; its recipe must never surface
	// in business B's out-of-stock set.
	out := seedInventoryItem(t, db, bizA.ID, "Out", 0, true)
	seedRecipe(t, db, bizA.ID, "dishA", out.ID, 1)

	unavailableB, err := database.OutOfStockMenuItemIDs(bizB.ID)
	require.NoError(t, err)
	require.NotContains(t, unavailableB, "dishA", "business B must not see business A's out-of-stock dishes")
}

// TestMenuAvailability_SingleQuery asserts availability resolution stays
// constant (not N+1 per recipe). #766 reconcile writes leftover name/id
// drift before the JOIN; guest hide uses the same name-wins resolver.
func TestMenuAvailability_SingleQuery(t *testing.T) {
	db := setupInventoryAvailabilityTestDB(t)
	biz := seedAvailabilityBusiness(t, db, database.InventoryAvailabilityModeHardBlock)
	for i := 0; i < 50; i++ {
		item := seedInventoryItem(t, db, biz.ID, fmt.Sprintf("ing-%d", i), 0, true)
		seedRecipe(t, db, biz.ID, fmt.Sprintf("dish-%d", i), item.ID, 1)
	}

	var count int64
	const cb = "count_menu_availability_queries"
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(cb, func(_ *gorm.DB) {
		atomic.AddInt64(&count, 1)
	}))
	defer func() { _ = db.Callback().Query().Remove(cb) }()

	_, err := database.OutOfStockMenuItemIDs(biz.ID)
	require.NoError(t, err)
	require.LessOrEqual(t, atomic.LoadInt64(&count), int64(5), "availability must stay constant (not N+1 per recipe)")
}

// TestMenuResponseKeepsOutOfStockDishVisibleButUnorderable is the end-to-end
// behavior test through the public response builder: hard-block inventory must
// preserve menu discovery while preventing the depleted item from being ordered.
func TestMenuResponseKeepsOutOfStockDishVisibleButUnorderable(t *testing.T) {
	db := setupInventoryAvailabilityTestDB(t)
	biz := seedAvailabilityBusiness(t, db, database.InventoryAvailabilityModeHardBlock)

	beef := seedInventoryItem(t, db, biz.ID, "Beef", 0, true)
	seedRecipe(t, db, biz.ID, "steak", beef.ID, 1)

	cats := []database.MenuCategory{{
		ID: "c1", Name: "Mains",
		Items: []database.MenuItem{
			{ID: "steak", Name: "Steak Plate", Price: 30, IsAvailable: true},
			{ID: "pasta", Name: "Pasta", Price: 18, IsAvailable: true},
		},
	}}
	raw, _ := json.Marshal(cats)
	menu := &database.Menu{BusinessID: biz.ID, Categories: string(raw), IsActive: true, Version: 1}
	require.NoError(t, db.Create(menu).Error)

	resp := buildPublicGuestMenuResponse(menu)
	out, _ := json.Marshal(resp["categories"])
	var got []database.MenuCategory
	require.NoError(t, json.Unmarshal(out, &got))
	require.Len(t, got, 1)
	require.Len(t, got[0].Items, 2, "out-of-stock dishes must remain visible")
	require.Equal(t, "steak", got[0].Items[0].ID)
	require.Equal(t, "pasta", got[0].Items[1].ID)

	projection, ok := resp["item_orderability"].(map[string]services.Orderability)
	require.True(t, ok, "item_orderability must be a typed projection")
	require.Equal(t, services.Orderability{State: services.OrderabilityInventoryOut}, projection["steak"])
	require.Equal(t, services.Orderability{Orderable: true, State: services.OrderabilityAvailable}, projection["pasta"])
	require.Equal(t, "out_of_stock", got[0].Items[0].InventoryStatus)
	require.False(t, got[0].Items[0].IsAvailable, "inventory-out dishes must not stay sellable on the guest catalog")
	require.Empty(t, got[0].Items[1].InventoryStatus)
	require.True(t, got[0].Items[1].IsAvailable)

	steakJSON, err := json.Marshal(got[0].Items[0])
	require.NoError(t, err)
	var steakWire map[string]any
	require.NoError(t, json.Unmarshal(steakJSON, &steakWire))
	require.Equal(t, "out_of_stock", steakWire["inventory_status"])
	require.Equal(t, false, steakWire["is_available"])
	_, hasOrderabilityState := steakWire["orderability_state"]
	require.False(t, hasOrderabilityState, "live catalog rows do not carry orderability_state")
}

// TestMenuResponse_InventoryUnavailableHarvestBowlIsNotSellable is the #728
// guest serializer contract: when operator Menu Builder would show
// "Unavailable · inventory" for Harvest Bowl, the guest catalog must not
// keep is_available:true (cards that only read that flag + allergen tags
// otherwise look like a normal $18.50 vegetarian dish). Steak stays
// sellable when its recipe is stocked.
func TestMenuResponse_InventoryUnavailableHarvestBowlIsNotSellable(t *testing.T) {
	db := setupInventoryAvailabilityTestDB(t)
	biz := seedAvailabilityBusiness(t, db, database.InventoryAvailabilityModeWarn)

	greens := seedInventoryItem(t, db, biz.ID, "Mixed Greens", 0, true)
	seedRecipe(t, db, biz.ID, "demo-bowl", greens.ID, 0.25)
	beef := seedInventoryItem(t, db, biz.ID, "Premium Beef", 10, true)
	seedRecipe(t, db, biz.ID, "demo-steak", beef.ID, 0.35)

	cats := []database.MenuCategory{{
		ID: "mains", Name: "Mains",
		Items: []database.MenuItem{
			{ID: "demo-bowl", Name: "Harvest Bowl", Price: 18.5, IsAvailable: true, DietaryTags: []string{"vegetarian"}, Allergens: []string{"gluten", "sesame"}},
			{ID: "demo-steak", Name: "Steak Plate", Price: 42, IsAvailable: true},
		},
	}}
	raw, err := json.Marshal(cats)
	require.NoError(t, err)
	menu := &database.Menu{BusinessID: biz.ID, Categories: string(raw), IsActive: true, Version: 1}
	require.NoError(t, db.Create(menu).Error)

	resp := buildPublicGuestMenuResponse(menu, biz)
	out, err := json.Marshal(resp["categories"])
	require.NoError(t, err)
	var got []database.MenuCategory
	require.NoError(t, json.Unmarshal(out, &got))
	require.Len(t, got, 1)
	require.Len(t, got[0].Items, 2)

	bowl, steak := got[0].Items[0], got[0].Items[1]
	require.Equal(t, "demo-bowl", bowl.ID)
	require.Equal(t, "Harvest Bowl", bowl.Name)
	require.False(t, bowl.IsAvailable)
	require.Equal(t, "out_of_stock", bowl.InventoryStatus)
	require.Equal(t, []string{"vegetarian"}, bowl.DietaryTags)
	require.Equal(t, []string{"gluten", "sesame"}, bowl.Allergens)

	require.Equal(t, "demo-steak", steak.ID)
	require.True(t, steak.IsAvailable)
	require.Empty(t, steak.InventoryStatus)

	projection, ok := resp["item_orderability"].(map[string]services.Orderability)
	require.True(t, ok)
	require.Equal(t, services.Orderability{State: services.OrderabilityInventoryOut}, projection["demo-bowl"])
	require.Equal(t, services.Orderability{Orderable: true, State: services.OrderabilityAvailable}, projection["demo-steak"])

	bowlJSON, err := json.Marshal(bowl)
	require.NoError(t, err)
	var bowlWire map[string]any
	require.NoError(t, json.Unmarshal(bowlJSON, &bowlWire))
	require.Equal(t, false, bowlWire["is_available"])
	require.Equal(t, "out_of_stock", bowlWire["inventory_status"])
}

// TestMenuResponse_DriftedBeefRecipeEightySixesSteakNotHarvestBowl is the
// QA-verified prod drift (#727 shape B): the beef recipe keeps
// menu_item_id=demo-steak while its cached name drifted to "Harvest Bowl",
// with Premium Beef at 0. Under the #727 B2 id-wins resolver the stored id is
// the strong key, so guest QR hide must 86 Steak Plate and keep Harvest Bowl
// sellable — matching #766 checkout.
func TestMenuResponse_DriftedBeefRecipeEightySixesSteakNotHarvestBowl(t *testing.T) {
	db := setupInventoryAvailabilityTestDB(t)
	biz := seedAvailabilityBusiness(t, db, database.InventoryAvailabilityModeWarn)

	greens := seedInventoryItem(t, db, biz.ID, "Mixed Greens", 48, true)
	seedRecipeNamed(t, db, biz.ID, "demo-bowl", "Harvest Bowl", greens.ID, 0.25)
	beef := seedInventoryItem(t, db, biz.ID, "Premium Beef", 0, true)
	seedRecipeNamed(t, db, biz.ID, "demo-steak", "Harvest Bowl", beef.ID, 0.35) // #727 shape B

	cats := []database.MenuCategory{{
		ID: "mains", Name: "Mains",
		Items: []database.MenuItem{
			{ID: "demo-bowl", Name: "Harvest Bowl", Price: 18.5, IsAvailable: true, DietaryTags: []string{"vegetarian"}, Allergens: []string{"gluten", "sesame"}},
			{ID: "demo-steak", Name: "Steak Plate", Price: 42, IsAvailable: true},
		},
	}}
	raw, err := json.Marshal(cats)
	require.NoError(t, err)
	menu := &database.Menu{BusinessID: biz.ID, Categories: string(raw), IsActive: true, Version: 1}
	require.NoError(t, db.Create(menu).Error)

	oos, err := database.OutOfStockMenuItemIDs(biz.ID)
	require.NoError(t, err)
	require.Contains(t, oos, "demo-steak")
	require.NotContains(t, oos, "demo-bowl", "drifted beef recipe must not hide Harvest Bowl")

	resp := buildPublicGuestMenuResponse(menu, biz)
	out, err := json.Marshal(resp["categories"])
	require.NoError(t, err)
	var got []database.MenuCategory
	require.NoError(t, json.Unmarshal(out, &got))
	require.Len(t, got, 1)
	require.Len(t, got[0].Items, 2)

	bowl, steak := got[0].Items[0], got[0].Items[1]
	require.Equal(t, "demo-bowl", bowl.ID)
	require.Equal(t, "Harvest Bowl", bowl.Name)
	require.True(t, bowl.IsAvailable, "Harvest Bowl must stay sellable when the beef recipe's name drifted onto it")
	require.Empty(t, bowl.InventoryStatus)
	require.Equal(t, []string{"vegetarian"}, bowl.DietaryTags)

	require.Equal(t, "demo-steak", steak.ID)
	require.Equal(t, "Steak Plate", steak.Name)
	require.False(t, steak.IsAvailable)
	require.Equal(t, "out_of_stock", steak.InventoryStatus)

	projection, ok := resp["item_orderability"].(map[string]services.Orderability)
	require.True(t, ok)
	require.Equal(t, services.Orderability{Orderable: true, State: services.OrderabilityAvailable}, projection["demo-bowl"])
	require.Equal(t, services.Orderability{State: services.OrderabilityInventoryOut}, projection["demo-steak"])

	steakJSON, err := json.Marshal(steak)
	require.NoError(t, err)
	var steakWire map[string]any
	require.NoError(t, json.Unmarshal(steakJSON, &steakWire))
	require.Equal(t, false, steakWire["is_available"])
	require.Equal(t, "out_of_stock", steakWire["inventory_status"])
}

// TestMenuResponse_ClosedHoursKeepsInventoryStatusOnCatalog is the live #723
// shape: guest ResolveOrderability remaps inventory_out → business_closed, and
// the catalog row still has no orderability_state. The 86 must survive as
// inventory_status so diner cards can omit the steak.
func TestMenuResponse_ClosedHoursKeepsInventoryStatusOnCatalog(t *testing.T) {
	db := setupInventoryAvailabilityTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.BusinessOperatingHours{}))
	biz := seedAvailabilityBusiness(t, db, database.InventoryAvailabilityModeHardBlock)
	biz.KitchenEnabled = true
	biz.OrdersEnabled = true
	// Empty timezone makes BusinessOpenAt evaluate in UTC, not the operator host's local weekday.
	biz.Timezone = "UTC"
	require.NoError(t, db.Create(&database.BusinessOperatingHours{
		BusinessID: biz.ID,
		DayOfWeek:  int(time.Now().UTC().Weekday()),
		IsClosed:   true,
	}).Error)

	beef := seedInventoryItem(t, db, biz.ID, "Beef", 0, true)
	seedRecipe(t, db, biz.ID, "steak", beef.ID, 1)

	cats := []database.MenuCategory{{
		ID: "c1", Name: "Mains",
		Items: []database.MenuItem{
			{ID: "steak", Name: "Steak Plate", Price: 30, IsAvailable: true},
			{ID: "pasta", Name: "Pasta", Price: 18, IsAvailable: true},
		},
	}}
	raw, _ := json.Marshal(cats)
	menu := &database.Menu{BusinessID: biz.ID, Categories: string(raw), IsActive: true, Version: 1}
	require.NoError(t, db.Create(menu).Error)

	liveCats := append([]database.MenuCategory(nil), cats...)
	payload, liveCats, _, _ := applyGuestLivePromotions(biz, menu, liveCats, nil, nil)
	projection, ok := payload["item_orderability"].(map[string]services.Orderability)
	require.True(t, ok)
	require.Equal(t, services.Orderability{State: services.OrderabilityBusinessClosed}, projection["steak"])
	require.Equal(t, services.Orderability{State: services.OrderabilityBusinessClosed}, projection["pasta"])
	require.Equal(t, "out_of_stock", liveCats[0].Items[0].InventoryStatus, "86 flag must survive hours overwrite")
	require.False(t, liveCats[0].Items[0].IsAvailable, "guest catalog must un-sell the 86 after hours remap")
	require.Empty(t, liveCats[0].Items[1].InventoryStatus)
	require.True(t, liveCats[0].Items[1].IsAvailable)

	resp := buildPublicGuestMenuResponse(menu, biz)
	out, _ := json.Marshal(resp["categories"])
	var got []database.MenuCategory
	require.NoError(t, json.Unmarshal(out, &got))
	require.Equal(t, "out_of_stock", got[0].Items[0].InventoryStatus)
	require.False(t, got[0].Items[0].IsAvailable)
	require.Empty(t, got[0].Items[1].InventoryStatus)
	require.True(t, got[0].Items[1].IsAvailable)

	steakJSON, err := json.Marshal(got[0].Items[0])
	require.NoError(t, err)
	var steakWire map[string]any
	require.NoError(t, json.Unmarshal(steakJSON, &steakWire))
	require.Equal(t, "out_of_stock", steakWire["inventory_status"])
	require.Equal(t, false, steakWire["is_available"])
	_, hasOrderabilityState := steakWire["orderability_state"]
	require.False(t, hasOrderabilityState)
}

func TestProjectOrderability_GuestClosedStampsInventoryStatusOnCatalog(t *testing.T) {
	db := setupInventoryAvailabilityTestDB(t)
	biz := seedAvailabilityBusiness(t, db, database.InventoryAvailabilityModeHardBlock)
	biz.KitchenEnabled = true
	biz.OrdersEnabled = true

	beef := seedInventoryItem(t, db, biz.ID, "Beef", 0, true)
	seedRecipe(t, db, biz.ID, "steak", beef.ID, 1)

	cats := []database.MenuCategory{{
		Items: []database.MenuItem{
			{ID: "steak", Name: "Steak Plate", IsAvailable: true},
			{ID: "pasta", Name: "Pasta", IsAvailable: true},
		},
	}}
	raw, err := json.Marshal(cats)
	require.NoError(t, err)
	require.NoError(t, db.Create(&database.Menu{
		BusinessID: biz.ID, Categories: string(raw), IsActive: true, Version: 1,
	}).Error)
	projection := services.ProjectOrderability(biz, cats, services.OrderabilityContextGuest, false)
	require.Equal(t, services.Orderability{State: services.OrderabilityBusinessClosed}, projection["steak"])
	require.Equal(t, services.Orderability{State: services.OrderabilityBusinessClosed}, projection["pasta"])
	require.Equal(t, "out_of_stock", cats[0].Items[0].InventoryStatus)
	require.Empty(t, cats[0].Items[1].InventoryStatus)
}

// BenchmarkPublicGuestMenuResponse measures the full guest menu response build
// (parse → availability resolve → project → re-serialize) on a realistic
// 30-item menu, 10 items backed by recipes, inventory enabled in hard_block.
func BenchmarkPublicGuestMenuResponse(b *testing.B) {
	menu := seedMenuAvailabilityBenchFixture(b)
	b.ResetTimer()
	b.ReportAllocs()
	for range b.N {
		_ = buildPublicGuestMenuResponse(menu)
	}
}

// BenchmarkPublicGuestMenuResponse_InventoryDisabled is the BASELINE proxy: the
// identical fixture with inventory tracking OFF, so the availability query is
// skipped and only the parse/project/serialize cost remains. The delta against
// BenchmarkPublicGuestMenuResponse is the added cost of the one batched query.
func BenchmarkPublicGuestMenuResponse_InventoryDisabled(b *testing.B) {
	menu := seedMenuAvailabilityBenchFixture(b)
	require.NoError(b, database.GetDB().Exec(
		"UPDATE inventory_settings SET inventory_enabled = 0 WHERE business_id = ?", menu.BusinessID,
	).Error)
	b.ResetTimer()
	b.ReportAllocs()
	for range b.N {
		_ = buildPublicGuestMenuResponse(menu)
	}
}

// seedMenuAvailabilityBenchFixture builds a fresh DB + business (hard_block
// mode) + 30-item menu (3 categories × 10 items), the first 10 items backed by
// recipes with alternating stock, and returns the menu to render.
func seedMenuAvailabilityBenchFixture(b *testing.B) *database.Menu {
	b.Helper()
	db := setupInventoryAvailabilityTestDB(b)
	biz := seedAvailabilityBusiness(b, db, database.InventoryAvailabilityModeHardBlock)

	cats := make([]database.MenuCategory, 3)
	itemID := 0
	for c := range cats {
		items := make([]database.MenuItem, 10)
		for i := range items {
			id := fmt.Sprintf("item-%d", itemID)
			items[i] = database.MenuItem{ID: id, Name: "Item", Price: 1000, IsAvailable: true}
			// First 10 items get a recipe; alternate stock so half are hidden.
			if itemID < 10 {
				qty := float64(0)
				if itemID%2 == 0 {
					qty = 5
				}
				item := seedInventoryItem(b, db, biz.ID, fmt.Sprintf("ing-%d", itemID), qty, true)
				seedRecipe(b, db, biz.ID, id, item.ID, 1)
			}
			itemID++
		}
		cats[c] = database.MenuCategory{ID: fmt.Sprintf("cat-%d", c), Name: "Category", Items: items}
	}
	raw, _ := json.Marshal(cats)
	return &database.Menu{BusinessID: biz.ID, Categories: string(raw)}
}
