package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupGuestCheckoutTest(t testing.TB) (*GuestCheckoutService, database.Table) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&database.Business{}, &database.Table{}, &database.Bill{},
		&database.Order{}, &database.Menu{}, &database.Offer{}, &database.Bundle{},
		&database.InventorySettings{}, &database.InventoryItem{}, &database.InventoryRecipe{},
		&database.DeliveryOrder{}, &database.BillHistoryEvent{}, &database.BusinessOperatingHours{},
	))
	require.NoError(t, db.Exec(`CREATE TABLE bill_items (
		id TEXT PRIMARY KEY, bill_id INTEGER NOT NULL, menu_item_id TEXT DEFAULT '',
		name TEXT NOT NULL, price REAL NOT NULL, quantity INTEGER NOT NULL,
		options TEXT, item_type TEXT DEFAULT 'menu_item', bundle_id INTEGER,
		parent_bundle_id INTEGER, source_offer_id INTEGER, order_id INTEGER,
		subtotal REAL NOT NULL, created_at DATETIME
	)`).Error)
	database.SetTestDB(db)
	business := database.Business{
		BusinessId: "checkout-test", Name: "Checkout", OwnerAddress: "0xowner",
		KitchenEnabled: true, OrdersEnabled: true, TaxRate: 10,
		DefaultCurrency: "USD", Timezone: "UTC",
	}
	require.NoError(t, db.Create(&business).Error)
	table := database.Table{BusinessID: business.ID, Name: "1", TableCode: "TABLE-ONE", Capacity: 4, IsActive: true}
	require.NoError(t, db.Create(&table).Error)
	categories := []database.MenuCategory{{ID: "mains", Name: "Mains", Items: []database.MenuItem{{ID: "burger", Name: "Burger", Price: 9.99, IsAvailable: true}}}}
	raw, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, db.Create(&database.Menu{BusinessID: business.ID, Categories: string(raw), IsActive: true}).Error)
	require.NoError(t, db.Create(&database.InventorySettings{BusinessID: business.ID, InventoryEnabled: false}).Error)
	return NewGuestCheckoutService(db), table
}

func checkoutInput(tableCode, requestID string) GuestCheckoutInput {
	return GuestCheckoutInput{
		TableCode: tableCode, ClientRequestID: requestID,
		Items: []PromotionInputLine{{Name: "Burger", MenuItemID: "burger", Quantity: 1, ItemType: OrderItemTypeMenuItem}},
	}
}

func seedInventoryBlockedBundle(t *testing.T, service *GuestCheckoutService, table database.Table) GuestCheckoutInput {
	t.Helper()
	var settings database.InventorySettings
	require.NoError(t, service.db.Where("business_id = ?", table.BusinessID).First(&settings).Error)
	settings.InventoryEnabled = true
	settings.AvailabilitySyncMode = database.InventoryAvailabilityModeHardBlock
	require.NoError(t, service.db.Save(&settings).Error)

	stock := database.InventoryItem{
		BusinessID: table.BusinessID, Name: "Burger patties", Unit: "unit",
		CurrentQuantity: 0, ReorderThreshold: 2, IsActive: true,
	}
	require.NoError(t, service.db.Create(&stock).Error)
	require.NoError(t, service.db.Create(&database.InventoryRecipe{
		BusinessID: table.BusinessID, MenuItemID: "burger", MenuItemName: "Burger",
		InventoryItemID: stock.ID, QuantityRequired: 1,
	}).Error)

	refs, err := json.Marshal([]database.BundleItemRef{{MenuItemID: "burger", Name: "Burger", Quantity: 1}})
	require.NoError(t, err)
	bundle := database.Bundle{
		BusinessID: table.BusinessID, Name: "Burger Deal", Price: 8.99,
		Items: string(refs), IsActive: true,
	}
	require.NoError(t, service.db.Create(&bundle).Error)

	return GuestCheckoutInput{
		TableCode: table.TableCode, ClientRequestID: "blocked-bundle",
		Items: []PromotionInputLine{{
			Name: "Burger Deal", MenuItemID: fmt.Sprintf("bundle:%d", bundle.ID),
			Quantity: 1, ItemType: OrderItemTypeBundle, BundleID: &bundle.ID,
		}},
	}
}

func TestGuestCheckoutCreatesBillOrderAndItemsAtomically(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)
	result, err := service.Checkout(context.Background(), checkoutInput(table.TableCode, "request-1"))
	require.NoError(t, err)
	assert.False(t, result.Replay)
	assert.NotZero(t, result.Bill.ID)
	assert.NotZero(t, result.Order.ID)
	assert.EqualValues(t, 999, result.Quote.SubtotalCents)
	assert.EqualValues(t, 1099, result.Bill.TotalAmount)
	assertOrderSettlement(t, result.Quote, 999, 100, 0, 0, 1099)

	var billItems int64
	require.NoError(t, service.db.Model(&database.BillItem{}).Where("bill_id = ?", result.Bill.ID).Count(&billItems).Error)
	assert.EqualValues(t, 1, billItems)
}

func TestGuestCheckoutDateNightChildrenCarryCatalogPriceAndTimestamps(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)
	categories := []database.MenuCategory{
		{
			ID: "mains", Name: "Mains",
			Items: []database.MenuItem{
				{ID: "demo-steak", Name: "Steak Plate", Price: 42, IsAvailable: true},
				{ID: "demo-dessert", Name: "Chocolate Tart", Price: 9, IsAvailable: true},
			},
		},
		{
			ID: "drinks", Name: "Drinks",
			Items: []database.MenuItem{
				{ID: "demo-cocktail", Name: "Demo Spritz", Price: 12, IsAvailable: true},
			},
		},
	}
	raw, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, service.db.Model(&database.Menu{}).Where("business_id = ?", table.BusinessID).Update("categories", string(raw)).Error)

	refs, err := json.Marshal([]database.BundleItemRef{
		{MenuItemID: "demo-steak", Quantity: 1},
		{MenuItemID: "demo-cocktail", Quantity: 2},
		{MenuItemID: "demo-dessert", Quantity: 1},
	})
	require.NoError(t, err)
	bundle := database.Bundle{
		BusinessID: table.BusinessID, Name: "Date Night for Two", Price: 68,
		Items: string(refs), IsActive: true,
	}
	require.NoError(t, service.db.Create(&bundle).Error)

	result, err := service.Checkout(context.Background(), GuestCheckoutInput{
		TableCode: table.TableCode, ClientRequestID: "date-night-core",
		Items: []PromotionInputLine{{
			Name: "Date Night for Two", Quantity: 1, UnitPrice: 68,
			ItemType: OrderItemTypeBundle, BundleID: &bundle.ID,
		}},
	})
	require.NoError(t, err)

	_, guestItems, err := database.GetPublicGuestOpenBillByTableID(table.ID)
	require.NoError(t, err)
	prices := map[string]float64{}
	for _, item := range guestItems {
		if item.ItemType != OrderItemTypeBundleItem {
			continue
		}
		prices[item.Name] = item.Price
		assert.Equal(t, 0.0, item.Subtotal)
		assert.False(t, item.CreatedAt.IsZero(), "guest payload must not emit year-1 timestamps")
		assert.GreaterOrEqual(t, item.CreatedAt.Year(), 2020)
	}
	assert.Equal(t, 42.0, prices["Steak Plate"])
	assert.Equal(t, 12.0, prices["Demo Spritz"])
	assert.Equal(t, 9.0, prices["Chocolate Tart"])

	var operatorItems []database.BillItem
	require.NoError(t, service.db.Where("bill_id = ?", result.Bill.ID).Find(&operatorItems).Error)
	for _, item := range operatorItems {
		if item.ItemType != OrderItemTypeBundleItem {
			continue
		}
		assert.Greater(t, item.Price, 0.0, "operator child %s catalog price", item.Name)
		assert.False(t, item.CreatedAt.IsZero(), "operator created_at must not be year-1")
		assert.GreaterOrEqual(t, item.CreatedAt.Year(), 2020)
	}
}

func TestGuestCheckoutNotificationUsesFinalPayableTotal(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)
	require.NoError(t, service.db.AutoMigrate(
		&database.Plugin{},
		&database.BusinessPlugin{},
		&database.PluginNotificationDelivery{},
	))
	plugin := database.Plugin{
		Name: "telegram", DisplayName: "Telegram",
		IsActive: true, Category: "integration",
	}
	require.NoError(t, service.db.Create(&plugin).Error)
	require.NoError(t, service.db.Create(&database.BusinessPlugin{
		BusinessID: table.BusinessID,
		PluginID:   plugin.ID,
		IsEnabled:  true,
		Config:     `{"is_connected":true,"chat_id":"12345"}`,
	}).Error)
	ResetTelegramNotificationEligibilityCache()
	t.Cleanup(ResetTelegramNotificationEligibilityCache)

	result, err := service.Checkout(context.Background(), checkoutInput(table.TableCode, "request-notification-total"))
	require.NoError(t, err)

	var delivery database.PluginNotificationDelivery
	require.NoError(t, service.db.Where(
		"business_id = ? AND plugin_name = ? AND event_type = ?",
		table.BusinessID, "telegram", PluginEventOrderCreated,
	).First(&delivery).Error)
	assert.Equal(t, float64(result.Quote.FinalTotalCents), delivery.Payload["total_cents"])
}

func TestGuestCheckoutReplaysByBusinessRequestIdentity(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)
	first, err := service.Checkout(context.Background(), checkoutInput(table.TableCode, "request-replay"))
	require.NoError(t, err)
	second, err := service.Checkout(context.Background(), checkoutInput(table.TableCode, "request-replay"))
	require.NoError(t, err)
	assert.True(t, second.Replay)
	assert.Equal(t, first.Bill.ID, second.Bill.ID)
	assert.Equal(t, first.Order.ID, second.Order.ID)
	firstSettlement := quoteSettlementContract(t, first.Quote)
	secondSettlement := quoteSettlementContract(t, second.Quote)
	assert.Equal(t, firstSettlement, secondSettlement)
}

func TestGuestCheckoutPersistsQuoteSnapshotAtomically(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)

	result, err := service.Checkout(context.Background(), checkoutInput(table.TableCode, "request-snapshot"))
	require.NoError(t, err)

	var raw sql.NullString
	require.NoError(t, service.db.Raw("SELECT quote_snapshot FROM orders WHERE id = ?", result.Order.ID).Scan(&raw).Error)
	require.True(t, raw.Valid)
	require.JSONEq(t, fmt.Sprintf(`{"version":1,"quote":%s}`, mustJSON(t, result.Quote)), raw.String)
}

func TestGuestCheckoutReplayUsesOriginalQuoteAfterBusinessRatesChange(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)
	first, err := service.Checkout(context.Background(), checkoutInput(table.TableCode, "request-rate-snapshot"))
	require.NoError(t, err)

	require.NoError(t, service.db.Model(&database.Business{}).Where("id = ?", table.BusinessID).Updates(map[string]interface{}{
		"tax_rate": 21, "service_fee_rate": 7,
	}).Error)
	replayed, err := service.Checkout(context.Background(), checkoutInput(table.TableCode, "request-rate-snapshot"))
	require.NoError(t, err)

	assert.True(t, replayed.Replay)
	assert.Equal(t, first.Quote, replayed.Quote)
}

func TestGuestCheckoutReplayFallsBackForLegacyOrInvalidQuoteSnapshots(t *testing.T) {
	for _, tc := range []struct {
		name     string
		snapshot interface{}
	}{
		{name: "legacy null", snapshot: nil},
		{name: "malformed json", snapshot: "not-json"},
		{name: "unknown version", snapshot: `{"version":2,"quote":{}}`},
		{name: "incomplete quote", snapshot: `{"version":1,"quote":{"subtotal_cents":999}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, table := setupGuestCheckoutTest(t)
			first, err := service.Checkout(context.Background(), checkoutInput(table.TableCode, "request-fallback"))
			require.NoError(t, err)
			require.NoError(t, service.db.Exec("UPDATE orders SET quote_snapshot = ? WHERE id = ?", tc.snapshot, first.Order.ID).Error)
			require.NoError(t, service.db.Model(&database.Business{}).Where("id = ?", table.BusinessID).Updates(map[string]interface{}{
				"tax_rate": 21, "service_fee_rate": 7,
			}).Error)

			replayed, err := service.Checkout(context.Background(), checkoutInput(table.TableCode, "request-fallback"))
			require.NoError(t, err)
			assertOrderSettlement(t, replayed.Quote, 999, 210, 70, 0, 1279)
		})
	}
}

func TestGuestCheckoutKeepsEachOrderQuoteScopedToThatOrder(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)
	first, err := service.Checkout(context.Background(), checkoutInput(table.TableCode, "request-first"))
	require.NoError(t, err)

	secondInput := checkoutInput(table.TableCode, "request-second")
	secondInput.BillID = &first.Bill.ID
	second, err := service.Checkout(context.Background(), secondInput)
	require.NoError(t, err)

	assert.Equal(t, first.Bill.ID, second.Bill.ID)
	assert.EqualValues(t, 1998, second.Bill.Subtotal)
	assert.EqualValues(t, 200, second.Bill.TaxAmount)
	assert.EqualValues(t, 2198, second.Bill.TotalAmount)
	assertOrderSettlement(t, second.Quote, 999, 100, 0, 0, 1099)

	replayedFirst, err := service.Checkout(context.Background(), checkoutInput(table.TableCode, "request-first"))
	require.NoError(t, err)
	assert.True(t, replayedFirst.Replay)
	assert.Equal(t, first.Order.ID, replayedFirst.Order.ID)
	assert.EqualValues(t, 2198, replayedFirst.Bill.TotalAmount)
	assertOrderSettlement(t, replayedFirst.Quote, 999, 100, 0, 0, 1099)
}

func TestBusinessQuoteIncludesTaxServiceAndFinalTotal(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)
	require.NoError(t, service.db.Model(&database.Business{}).
		Where("id = ?", table.BusinessID).
		Update("service_fee_rate", 5).Error)

	projection, err := service.QuoteForBusiness(
		context.Background(),
		table.BusinessID,
		checkoutInput(table.TableCode, "quote-settlement").Items,
		"",
	)
	require.NoError(t, err)
	contract := quoteSettlementContract(t, projection.Quote)
	assert.Equal(t, float64(999), contract["net_subtotal_cents"])
	assert.Equal(t, float64(100), contract["tax_cents"])
	assert.Equal(t, float64(50), contract["service_fee_cents"])
	assert.Equal(t, float64(0), contract["tip_cents"])
	assert.Equal(t, float64(1149), contract["final_total_cents"])
}

func TestBusinessQuoteAndCheckoutSettlement_NYCTaxOn2100(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)
	require.NoError(t, service.db.Model(&database.Business{}).
		Where("id = ?", table.BusinessID).
		Updates(map[string]any{"tax_rate": 8.875, "service_fee_rate": 4.0}).Error)
	categories := []database.MenuCategory{{
		ID: "mains", Name: "Mains",
		Items: []database.MenuItem{{ID: "burger", Name: "Burger", Price: 2100, IsAvailable: true}},
	}}
	raw, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, service.db.Model(&database.Menu{}).
		Where("business_id = ?", table.BusinessID).
		Update("categories", string(raw)).Error)

	projection, err := service.QuoteForBusiness(
		context.Background(),
		table.BusinessID,
		checkoutInput(table.TableCode, "quote-nyc-tax").Items,
		"",
	)
	require.NoError(t, err)
	assertOrderSettlement(t, projection.Quote, 210000, 18638, 8400, 0, 237038)

	result, err := service.Checkout(context.Background(), checkoutInput(table.TableCode, "checkout-nyc-tax"))
	require.NoError(t, err)
	assert.EqualValues(t, 210000, result.Bill.Subtotal)
	assert.EqualValues(t, 18638, result.Bill.TaxAmount)
	assert.EqualValues(t, 8400, result.Bill.ServiceFeeAmount)
	assert.EqualValues(t, 237038, result.Bill.TotalAmount)
	assertOrderSettlement(t, result.Quote, 210000, 18638, 8400, 0, 237038)
}

func quoteSettlementContract(t *testing.T, quote Quote) map[string]interface{} {
	t.Helper()
	raw, err := json.Marshal(quote)
	require.NoError(t, err)
	var contract map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &contract))
	return contract
}

func mustJSON(t *testing.T, value interface{}) string {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return string(raw)
}

func assertOrderSettlement(t *testing.T, quote Quote, netSubtotal, tax, serviceFee, tip, finalTotal int64) {
	t.Helper()
	contract := quoteSettlementContract(t, quote)
	assert.Equal(t, float64(netSubtotal), contract["net_subtotal_cents"])
	assert.Equal(t, float64(tax), contract["tax_cents"])
	assert.Equal(t, float64(serviceFee), contract["service_fee_cents"])
	assert.Equal(t, float64(tip), contract["tip_cents"])
	assert.Equal(t, float64(finalTotal), contract["final_total_cents"])
}

func TestGuestCheckoutRejectsReplayWhenRequestBelongsToAnotherTable(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)
	first, err := service.Checkout(context.Background(), checkoutInput(table.TableCode, "request-cross-table"))
	require.NoError(t, err)

	otherTable := database.Table{
		BusinessID: table.BusinessID,
		Name:       "2",
		TableCode:  "TABLE-TWO",
		Capacity:   4,
		IsActive:   true,
	}
	require.NoError(t, service.db.Create(&otherTable).Error)

	result, err := service.Checkout(context.Background(), checkoutInput(otherTable.TableCode, "request-cross-table"))
	var conflict *GuestCheckoutReplayConflictError
	require.ErrorAs(t, err, &conflict)
	assert.Zero(t, result.Bill.ID, "a replay conflict must not expose the other table's bill")
	assert.Zero(t, result.Order.ID, "a replay conflict must not expose the other table's order")
	assert.Equal(t, first.Bill.ID, conflict.ExistingBillID)
}

func TestGuestCheckoutApprovalDoesNotBillItemsTwice(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)
	result, err := service.Checkout(context.Background(), checkoutInput(table.TableCode, "request-approve"))
	require.NoError(t, err)
	before := result.Bill.TotalAmount

	require.NoError(t, database.UpdateOrderStatus(result.Order.ID, database.OrderStatusApproved, "staff", ""))
	var bill database.Bill
	require.NoError(t, service.db.First(&bill, result.Bill.ID).Error)
	assert.Equal(t, before, bill.TotalAmount)
	var itemCount int64
	require.NoError(t, service.db.Model(&database.BillItem{}).Where("bill_id = ?", bill.ID).Count(&itemCount).Error)
	assert.EqualValues(t, 1, itemCount)
}

func TestGuestCheckoutRollsBackBillWhenOrderInsertFails(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)
	require.NoError(t, service.db.Exec(`CREATE TRIGGER reject_guest_order BEFORE INSERT ON orders
		WHEN NEW.created_by = 'guest' BEGIN SELECT RAISE(FAIL, 'forced order insert failure'); END;`).Error)

	_, err := service.Checkout(context.Background(), checkoutInput(table.TableCode, "request-fail"))
	require.Error(t, err)
	var bills int64
	require.NoError(t, service.db.Model(&database.Bill{}).Where("table_id = ?", table.ID).Count(&bills).Error)
	assert.Zero(t, bills)
}

func TestGuestCheckoutRejectsBundleWhenAChildIsInventoryBlocked(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)
	input := seedInventoryBlockedBundle(t, service, table)

	_, err := service.Checkout(context.Background(), input)
	var blocked *ItemNotOrderableError
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, []ItemNotOrderableDetail{{
		MenuItemID: "burger",
		Reason:     OrderabilityInventoryOut,
	}}, blocked.Items)

	var orders int64
	require.NoError(t, service.db.Model(&database.Order{}).Count(&orders).Error)
	assert.Zero(t, orders)
}

func TestGuestCheckoutReturnsAuthoritativeReasonForManualDisabledItem(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)
	var menu database.Menu
	require.NoError(t, service.db.Where("business_id = ?", table.BusinessID).First(&menu).Error)
	categories := []database.MenuCategory{{
		ID: "mains", Name: "Mains",
		Items: []database.MenuItem{{ID: "burger", Name: "Burger", Price: 9.99, IsAvailable: false}},
	}}
	raw, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, service.db.Model(&menu).Update("categories", string(raw)).Error)

	_, err = service.Checkout(context.Background(), checkoutInput(table.TableCode, "manual-disabled"))
	var blocked *ItemNotOrderableError
	require.ErrorAs(t, err, &blocked)
	assert.Equal(t, []ItemNotOrderableDetail{{
		MenuItemID: "burger", Reason: OrderabilityManualDisabled,
	}}, blocked.Items)
}

func seedTeaSteakMenu(t *testing.T, service *GuestCheckoutService, table database.Table) {
	t.Helper()
	categories := []database.MenuCategory{
		{ID: "mains", Name: "Mains", Items: []database.MenuItem{
			{ID: "demo-steak", Name: "Steak Plate", Price: 42, IsAvailable: true},
		}},
		{ID: "drinks", Name: "Drinks", Items: []database.MenuItem{
			{ID: "demo-tea", Name: "Iced Tea", Price: 5, IsAvailable: true},
		}},
	}
	raw, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, service.db.Model(&database.Menu{}).Where("business_id = ?", table.BusinessID).Update("categories", string(raw)).Error)
}

func TestGuestQuoteAndCheckoutUseCatalogNameNotClientName(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)
	seedTeaSteakMenu(t, service, table)
	forged := []PromotionInputLine{{
		Name: "Steak Plate", MenuItemID: "demo-tea", Quantity: 1, ItemType: OrderItemTypeMenuItem,
	}}

	projection, err := service.QuoteForTable(context.Background(), table.TableCode, forged, "")
	require.NoError(t, err)
	assert.EqualValues(t, 500, projection.Quote.SubtotalCents)
	assert.Equal(t, "demo-tea", projection.Quote.Lines[0].Key)

	result, err := service.Checkout(context.Background(), GuestCheckoutInput{
		TableCode: table.TableCode, ClientRequestID: "forged-name", Items: forged,
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.Order.Items)
	var persisted []database.OrderItem
	require.NoError(t, json.Unmarshal([]byte(result.Order.Items), &persisted))
	require.NotEmpty(t, persisted)
	assert.Equal(t, "Iced Tea", persisted[0].MenuItemName)
	assert.Equal(t, "demo-tea", persisted[0].MenuItemID)
	assert.EqualValues(t, 5, persisted[0].Price)
	assert.EqualValues(t, 500, result.Quote.SubtotalCents)

	var billItem database.BillItem
	require.NoError(t, service.db.Where("bill_id = ?", result.Bill.ID).First(&billItem).Error)
	assert.Equal(t, "Iced Tea", billItem.Name)
	assert.Equal(t, "demo-tea", billItem.MenuItemID)
	assert.EqualValues(t, 5, billItem.Price)
}

func discountOnlySteakLine() []PromotionInputLine {
	return []PromotionInputLine{{
		Name: "Steak Plate", MenuItemID: "demo-steak", Quantity: 1,
		ItemType: OrderItemTypeDiscount, UnitPrice: -1000,
	}}
}

func TestGuestQuoteRejectsDiscountOnlyLines(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)
	seedTeaSteakMenu(t, service, table)
	discountOnly := discountOnlySteakLine()

	_, err := service.QuoteForTable(context.Background(), table.TableCode, discountOnly, "")
	require.ErrorIs(t, err, errNoValidPricedLines)

	_, err = service.QuoteForBusiness(context.Background(), table.BusinessID, discountOnly, "")
	require.ErrorIs(t, err, errNoValidPricedLines)
}

func TestGuestCheckoutRejectsDiscountOnlyLines(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)
	seedTeaSteakMenu(t, service, table)

	_, err := service.Checkout(context.Background(), GuestCheckoutInput{
		TableCode: table.TableCode, ClientRequestID: "discount-only", Items: discountOnlySteakLine(),
	})
	require.ErrorIs(t, err, errNoValidPricedLines)

	var orders, bills int64
	require.NoError(t, service.db.Model(&database.Order{}).Count(&orders).Error)
	require.NoError(t, service.db.Model(&database.Bill{}).Count(&bills).Error)
	assert.Zero(t, orders)
	assert.Zero(t, bills)
}

func TestGuestQuoteProjectsBundleChildInventoryOrderability(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)
	input := seedInventoryBlockedBundle(t, service, table)

	_, err := service.QuoteForTable(context.Background(), table.TableCode, input.Items, "")
	var unavailable *ItemNotOrderableError
	require.ErrorAs(t, err, &unavailable)
	require.NotEmpty(t, unavailable.Items)
	assert.Equal(t, "burger", unavailable.Items[0].MenuItemID)
	assert.Equal(t, OrderabilityInventoryOut, unavailable.Items[0].Reason)
}

func TestGuestQuoteRejectsInventoryOutMenuItem(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)
	var settings database.InventorySettings
	require.NoError(t, service.db.Where("business_id = ?", table.BusinessID).First(&settings).Error)
	settings.InventoryEnabled = true
	settings.AvailabilitySyncMode = database.InventoryAvailabilityModeHardBlock
	require.NoError(t, service.db.Save(&settings).Error)
	stock := database.InventoryItem{
		BusinessID: table.BusinessID, Name: "Patties", Unit: "unit",
		CurrentQuantity: 0, IsActive: true,
	}
	require.NoError(t, service.db.Create(&stock).Error)
	require.NoError(t, service.db.Create(&database.InventoryRecipe{
		BusinessID: table.BusinessID, MenuItemID: "burger", MenuItemName: "Burger",
		InventoryItemID: stock.ID, QuantityRequired: 1,
	}).Error)

	items := []PromotionInputLine{{
		Name: "Burger", MenuItemID: "burger", Quantity: 1, ItemType: OrderItemTypeMenuItem,
	}}
	_, err := service.QuoteForTable(context.Background(), table.TableCode, items, "")
	var unavailable *ItemNotOrderableError
	require.ErrorAs(t, err, &unavailable)
	assert.Equal(t, "burger", unavailable.Items[0].MenuItemID)
	assert.Equal(t, OrderabilityInventoryOut, unavailable.Items[0].Reason)
}

func TestGuestCheckoutUnknownMenuItemIDIsItemNotFound(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)
	_, err := service.Checkout(context.Background(), GuestCheckoutInput{
		TableCode: table.TableCode, ClientRequestID: "unknown-id",
		Items: []PromotionInputLine{{
			Name: "Ghost Burger", MenuItemID: "does-not-exist", Quantity: 1,
			ItemType: OrderItemTypeMenuItem,
		}},
	})
	assertOrderValidationCode(t, err, OrderErrCodeItemNotFound)
}

func TestBusinessOpenAtReportsConfiguredClosure(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)
	var business database.Business
	require.NoError(t, service.db.First(&business, table.BusinessID).Error)
	now := time.Now().UTC()
	require.NoError(t, service.db.Create(&database.BusinessOperatingHours{
		BusinessID: business.ID, DayOfWeek: int(now.Weekday()), IsClosed: true,
	}).Error)

	assert.False(t, BusinessOpenAt(service.db, &business, now))
}

// FIND-035: guest checkout must not create orders outside operating hours,
// including when the client sends a stale/forged menu_item_id that is absent
// from the orderability projection (previously skipped the gate).
func TestCheckout_RejectsWhenBusinessClosed(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)
	var business database.Business
	require.NoError(t, service.db.First(&business, table.BusinessID).Error)
	now := time.Now().UTC()
	require.NoError(t, service.db.Create(&database.BusinessOperatingHours{
		BusinessID: business.ID, DayOfWeek: int(now.Weekday()), IsClosed: true,
	}).Error)

	// Correct catalog id
	_, err := service.Checkout(context.Background(), checkoutInput(table.TableCode, "closed-1"))
	require.Error(t, err)
	var ve *OrderValidationError
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, OrderErrCodeBusinessClosed, ve.Code)

	// Stale/forged id + real name — must still fail closed, not 201.
	_, err = service.Checkout(context.Background(), GuestCheckoutInput{
		TableCode: table.TableCode, ClientRequestID: "closed-2",
		Items: []PromotionInputLine{{
			Name: "Burger", MenuItemID: "demo-harvest-stale", Quantity: 1, ItemType: OrderItemTypeMenuItem,
		}},
	})
	require.Error(t, err)
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, OrderErrCodeBusinessClosed, ve.Code)

	var orderCount int64
	require.NoError(t, service.db.Model(&database.Order{}).Count(&orderCount).Error)
	assert.Zero(t, orderCount)
}

func TestBlockedOrderItems_UnknownIDFailsClosed(t *testing.T) {
	projection := map[string]Orderability{
		"burger": {Orderable: true, State: OrderabilityAvailable},
	}
	blocked := blockedOrderItems([]database.OrderItem{
		{MenuItemID: "not-in-menu", ItemType: OrderItemTypeMenuItem},
	}, projection)
	require.Len(t, blocked, 1)
	assert.Equal(t, "not-in-menu", blocked[0].MenuItemID)
	assert.Equal(t, OrderabilityManualDisabled, blocked[0].Reason)
}

func TestGuestCheckoutPreservesLoyaltyDiscountOnRepricing(t *testing.T) {
	svc, table := setupGuestCheckoutTest(t)

	first, err := svc.Checkout(context.Background(), checkoutInput(table.TableCode, "req-loyalty-1"))
	require.NoError(t, err)

	// A guest redeems loyalty points between the two checkouts: bake a 300¢
	// discount into the bill exactly like loyalty.RedeemPoints does.
	var bill database.Bill
	require.NoError(t, database.GetDB().First(&bill, first.Bill.ID).Error)
	gross := bill.Subtotal + bill.TaxAmount + bill.ServiceFeeAmount
	require.NoError(t, database.GetDB().Model(&database.Bill{}).
		Where("id = ?", bill.ID).
		Updates(map[string]interface{}{
			"loyalty_discount_cents": int64(300),
			"total_amount":           database.NetBillTotalCents(gross, 300),
		}).Error)

	// Second checkout on the same table reuses the same open bill.
	_, err = svc.Checkout(context.Background(), checkoutInput(table.TableCode, "req-loyalty-2"))
	require.NoError(t, err)

	var updated database.Bill
	require.NoError(t, database.GetDB().First(&updated, first.Bill.ID).Error)
	expectedGross := updated.Subtotal + updated.TaxAmount + updated.ServiceFeeAmount
	require.Equal(t, database.NetBillTotalCents(expectedGross, 300), updated.TotalAmount,
		"second checkout must keep the 300¢ loyalty discount in total_amount")
	require.EqualValues(t, 300, updated.LoyaltyDiscountCents)
}

func seedDriftedBeefCheckoutMenu(t *testing.T, service *GuestCheckoutService, table database.Table, beefQty float64) []database.MenuCategory {
	t.Helper()
	categories := []database.MenuCategory{{
		ID: "mains", Name: "Mains",
		Items: []database.MenuItem{
			{ID: "demo-bowl", Name: "Harvest Bowl", Price: 18.50, IsAvailable: true, DietaryTags: []string{"vegetarian"}},
			{ID: "demo-steak", Name: "Steak Plate", Price: 24, IsAvailable: true},
		},
	}}
	raw, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, service.db.Model(&database.Menu{}).Where("business_id = ?", table.BusinessID).
		Update("categories", string(raw)).Error)

	var settings database.InventorySettings
	require.NoError(t, service.db.Where("business_id = ?", table.BusinessID).First(&settings).Error)
	settings.InventoryEnabled = true
	settings.AvailabilitySyncMode = database.InventoryAvailabilityModeWarn
	require.NoError(t, service.db.Save(&settings).Error)

	greens := database.InventoryItem{
		BusinessID: table.BusinessID, Name: "Mixed Greens", Unit: "kg",
		CurrentQuantity: 48, IsActive: true,
	}
	beef := database.InventoryItem{
		BusinessID: table.BusinessID, Name: "Premium Beef", Unit: "kg",
		CurrentQuantity: beefQty, IsActive: true,
	}
	require.NoError(t, service.db.Create(&greens).Error)
	require.NoError(t, service.db.Create(&beef).Error)
	require.NoError(t, service.db.Create([]database.InventoryRecipe{
		{
			BusinessID: table.BusinessID, MenuItemID: "demo-bowl", MenuItemName: "Harvest Bowl",
			InventoryItemID: greens.ID, QuantityRequired: 0.25,
		},
		// #727 shape B: correct steak id, name drifted onto the bowl (id-wins).
		{
			BusinessID: table.BusinessID, MenuItemID: "demo-steak", MenuItemName: "Harvest Bowl",
			InventoryItemID: beef.ID, QuantityRequired: 0.35,
		},
	}).Error)
	return categories
}

func TestProjectOrderabilityTx_DriftedBeefBlocksSteakNotHarvestBowl(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)
	categories := seedDriftedBeefCheckoutMenu(t, service, table, 0)
	var business database.Business
	require.NoError(t, service.db.First(&business, table.BusinessID).Error)

	require.NoError(t, service.db.Transaction(func(tx *gorm.DB) error {
		projection, err := projectOrderabilityTx(tx, &business, categories, OrderabilityContextGuest, true)
		require.NoError(t, err)
		require.Equal(t, OrderabilityInventoryOut, projection["demo-steak"].State)
		assert.False(t, projection["demo-steak"].Orderable)
		assert.True(t, projection["demo-bowl"].Orderable, "vegetarian Harvest Bowl must stay sellable")
		assert.NotEqual(t, OrderabilityInventoryOut, projection["demo-bowl"].State)
		return nil
	}))

	var beef database.InventoryItem
	require.NoError(t, service.db.Where("business_id = ? AND name = ?", table.BusinessID, "Premium Beef").
		First(&beef).Error)
	var beefRecipe database.InventoryRecipe
	require.NoError(t, service.db.Where("business_id = ? AND inventory_item_id = ?", table.BusinessID, beef.ID).
		First(&beefRecipe).Error)
	assert.Equal(t, "demo-steak", beefRecipe.MenuItemID)
	assert.Equal(t, "Harvest Bowl", beefRecipe.MenuItemName,
		"#727 B2: the conflicted row resolves id-wins at read time and must not be healed")
}

func TestGuestCheckout_DriftedBeefAllowsBowlBlocksSteak(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)
	seedDriftedBeefCheckoutMenu(t, service, table, 0)

	bowl, err := service.Checkout(context.Background(), GuestCheckoutInput{
		TableCode: table.TableCode, ClientRequestID: "bowl-ok",
		Items: []PromotionInputLine{{
			Name: "Harvest Bowl", MenuItemID: "demo-bowl", Quantity: 1, ItemType: OrderItemTypeMenuItem,
		}},
	})
	require.NoError(t, err, "zero beef must not 86 the vegetarian bowl at checkout")
	assert.NotZero(t, bowl.Order.ID)

	_, err = service.Checkout(context.Background(), GuestCheckoutInput{
		TableCode: table.TableCode, ClientRequestID: "steak-blocked",
		Items: []PromotionInputLine{{
			Name: "Steak Plate", MenuItemID: "demo-steak", Quantity: 1, ItemType: OrderItemTypeMenuItem,
		}},
	})
	var blocked *ItemNotOrderableError
	require.ErrorAs(t, err, &blocked)
	require.Equal(t, []ItemNotOrderableDetail{{
		MenuItemID: "demo-steak",
		Reason:     OrderabilityInventoryOut,
	}}, blocked.Items)
}

func TestValidateOperatorOrderableLinesTx_DriftedBeefBlocksSteakNotBowl(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)
	categories := seedDriftedBeefCheckoutMenu(t, service, table, 0)
	var business database.Business
	require.NoError(t, service.db.First(&business, table.BusinessID).Error)

	require.NoError(t, service.db.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, ValidateOperatorOrderableLinesTx(tx, &business, categories, []database.OrderItem{{
			MenuItemID: "demo-bowl", MenuItemName: "Harvest Bowl", Quantity: 1, ItemType: OrderItemTypeMenuItem,
		}}))
		err := ValidateOperatorOrderableLinesTx(tx, &business, categories, []database.OrderItem{{
			MenuItemID: "demo-steak", MenuItemName: "Steak Plate", Quantity: 1, ItemType: OrderItemTypeMenuItem,
		}})
		var blocked *ItemNotOrderableError
		require.ErrorAs(t, err, &blocked)
		require.Equal(t, []ItemNotOrderableDetail{{
			MenuItemID: "demo-steak",
			Reason:     OrderabilityInventoryOut,
		}}, blocked.Items)
		return nil
	}))
}

func seedZeroBeefParrillaCheckout(t *testing.T, service *GuestCheckoutService, table database.Table) database.Bundle {
	t.Helper()
	categories := []database.MenuCategory{
		{
			ID: "parrilla", Name: "Parrilla",
			Items: []database.MenuItem{
				{ID: "demo-bife", Name: "Bife de chorizo", Price: 34000, IsAvailable: true},
				{ID: "demo-ojo-de-bife", Name: "Ojo de bife", Price: 39500, IsAvailable: true},
				{ID: "demo-asado-tira", Name: "Asado de tira", Price: 29800, IsAvailable: true},
				{ID: "demo-parrillada", Name: "Parrillada para dos", Price: 68000, IsAvailable: true},
			},
		},
		{
			ID: "otros", Name: "Otros",
			Items: []database.MenuItem{
				{ID: "demo-ensalada", Name: "Ensalada mixta", Price: 12500, IsAvailable: true},
				{ID: "demo-sorrentinos", Name: "Sorrentinos caseros", Price: 19800, IsAvailable: true},
				{ID: "demo-malbec-botella", Name: "Botella de Malbec", Price: 28500, IsAvailable: true},
				{ID: "demo-flan", Name: "Flan casero", Price: 8900, IsAvailable: true},
			},
		},
	}
	raw, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, service.db.Model(&database.Menu{}).Where("business_id = ?", table.BusinessID).
		Update("categories", string(raw)).Error)

	var settings database.InventorySettings
	require.NoError(t, service.db.Where("business_id = ?", table.BusinessID).First(&settings).Error)
	settings.InventoryEnabled = true
	settings.AvailabilitySyncMode = database.InventoryAvailabilityModeWarn
	require.NoError(t, service.db.Save(&settings).Error)

	greens := database.InventoryItem{
		BusinessID: table.BusinessID, Name: "Verdura de estación", Unit: "kg",
		CurrentQuantity: 48, IsActive: true,
	}
	pasta := database.InventoryItem{
		BusinessID: table.BusinessID, Name: "Masa para sorrentinos", Unit: "kg",
		CurrentQuantity: 12, IsActive: true,
	}
	wine := database.InventoryItem{
		BusinessID: table.BusinessID, Name: "Malbec de bodega", Unit: "l",
		CurrentQuantity: 35, IsActive: true,
	}
	custard := database.InventoryItem{
		BusinessID: table.BusinessID, Name: "Crema para flan", Unit: "kg",
		CurrentQuantity: 8, IsActive: true,
	}
	beef := database.InventoryItem{
		BusinessID: table.BusinessID, Name: "Bife de chorizo (media res)", Unit: "kg",
		CurrentQuantity: 0, IsActive: true,
	}
	require.NoError(t, service.db.Create(&greens).Error)
	require.NoError(t, service.db.Create(&pasta).Error)
	require.NoError(t, service.db.Create(&wine).Error)
	require.NoError(t, service.db.Create(&custard).Error)
	require.NoError(t, service.db.Create(&beef).Error)
	require.NoError(t, service.db.Create([]database.InventoryRecipe{
		{BusinessID: table.BusinessID, MenuItemID: "demo-bife", MenuItemName: "Bife de chorizo", InventoryItemID: beef.ID, QuantityRequired: 0.40},
		{BusinessID: table.BusinessID, MenuItemID: "demo-ojo-de-bife", MenuItemName: "Ojo de bife", InventoryItemID: beef.ID, QuantityRequired: 0.45},
		{BusinessID: table.BusinessID, MenuItemID: "demo-asado-tira", MenuItemName: "Asado de tira", InventoryItemID: beef.ID, QuantityRequired: 0.35},
		{BusinessID: table.BusinessID, MenuItemID: "demo-parrillada", MenuItemName: "Parrillada para dos", InventoryItemID: beef.ID, QuantityRequired: 0.80},
		{BusinessID: table.BusinessID, MenuItemID: "demo-ensalada", MenuItemName: "Ensalada mixta", InventoryItemID: greens.ID, QuantityRequired: 0.20},
		{BusinessID: table.BusinessID, MenuItemID: "demo-sorrentinos", MenuItemName: "Sorrentinos caseros", InventoryItemID: pasta.ID, QuantityRequired: 0.25},
		{BusinessID: table.BusinessID, MenuItemID: "demo-malbec-botella", MenuItemName: "Botella de Malbec", InventoryItemID: wine.ID, QuantityRequired: 0.75},
		{BusinessID: table.BusinessID, MenuItemID: "demo-flan", MenuItemName: "Flan casero", InventoryItemID: custard.ID, QuantityRequired: 0.15},
	}).Error)

	refs, err := json.Marshal([]database.BundleItemRef{
		{MenuItemID: "demo-parrillada", Name: "Parrillada para dos", Quantity: 1},
		{MenuItemID: "demo-malbec-botella", Name: "Botella de Malbec", Quantity: 1},
		{MenuItemID: "demo-flan", Name: "Flan casero", Quantity: 2},
	})
	require.NoError(t, err)
	bundle := database.Bundle{
		BusinessID: table.BusinessID, Name: "Noche de parrilla para dos", Price: 105000,
		Items: string(refs), IsActive: true,
	}
	require.NoError(t, service.db.Create(&bundle).Error)
	return bundle
}

func assertQuoteInventoryOut(t *testing.T, service *GuestCheckoutService, tableCode string, line PromotionInputLine, wantID string) {
	t.Helper()
	_, err := service.QuoteForTable(context.Background(), tableCode, []PromotionInputLine{line}, "")
	var blocked *ItemNotOrderableError
	require.ErrorAs(t, err, &blocked, wantID)
	require.Equal(t, []ItemNotOrderableDetail{{
		MenuItemID: wantID,
		Reason:     OrderabilityInventoryOut,
	}}, blocked.Items, wantID)
}

func TestQuoteForTable_ZeroBeefBlocksAllBeefDishesAndNocheDeParrilla(t *testing.T) {
	service, table := setupGuestCheckoutTest(t)
	bundle := seedZeroBeefParrillaCheckout(t, service, table)

	for _, item := range []struct {
		id   string
		name string
	}{
		{"demo-bife", "Bife de chorizo"},
		{"demo-ojo-de-bife", "Ojo de bife"},
		{"demo-asado-tira", "Asado de tira"},
		{"demo-parrillada", "Parrillada para dos"},
	} {
		assertQuoteInventoryOut(t, service, table.TableCode, PromotionInputLine{
			Name: item.name, MenuItemID: item.id, Quantity: 1, ItemType: OrderItemTypeMenuItem,
		}, item.id)
	}

	assertQuoteInventoryOut(t, service, table.TableCode, PromotionInputLine{
		Name: bundle.Name, MenuItemID: fmt.Sprintf("bundle:%d", bundle.ID),
		Quantity: 1, ItemType: OrderItemTypeBundle, BundleID: &bundle.ID,
	}, "demo-parrillada")

	for _, item := range []struct {
		id   string
		name string
	}{
		{"demo-ensalada", "Ensalada mixta"},
		{"demo-sorrentinos", "Sorrentinos caseros"},
	} {
		quote, err := service.QuoteForTable(context.Background(), table.TableCode, []PromotionInputLine{{
			Name: item.name, MenuItemID: item.id, Quantity: 1, ItemType: OrderItemTypeMenuItem,
		}}, "")
		require.NoError(t, err, item.id)
		assert.True(t, quote.Orderability[item.id].Orderable, item.id)
		assert.Equal(t, OrderabilityAvailable, quote.Orderability[item.id].State, item.id)
	}
}
