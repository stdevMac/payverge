package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func enableGuestOrderingForBusiness(t *testing.T, businessID uint) {
	t.Helper()

	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", businessID).
		Updates(map[string]any{
			"kitchen_enabled": true,
			"orders_enabled":  true,
		}).Error)
}

func TestCreateGuestOrder_RejectsClosedBill(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Table{},
		&database.Bill{},
		&database.Order{},
		&database.Menu{},
		&database.Offer{},
		&database.Bundle{},
	))

	business := createOwnedBusiness(t, "0xOwnerA", "Guest Orders Biz")
	enableGuestOrderingForBusiness(t, business.ID)
	table := &database.Table{
		BusinessID: business.ID,
		Name:       "T1",
		TableCode:  "guest-table-1",
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)

	bill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     fmt.Sprintf("B-guest-%d", time.Now().UnixNano()),
		Status:         database.BillStatusClosed,
		SettlementAddr: business.SettlementAddr,
		TippingAddr:    business.TippingAddr,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	body, err := json.Marshal(map[string]any{
		"bill_id": bill.ID,
		"items": []map[string]any{
			{
				"menu_item_name": "Burger",
				"quantity":       1,
				"price":          10.0,
			},
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/guest/table/%s/order", table.TableCode), bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("X-Request-Id", "closed-bill-order-1")

	CreateGuestOrder(c)

	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "Bill is not open for new orders")
}

func TestCreateGuestOrder_ReusesExistingOrderForDuplicateRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Table{},
		&database.Bill{},
		&database.Order{},
		&database.Menu{},
		&database.Offer{},
		&database.Bundle{},
		&database.Plugin{},
		&database.BusinessPlugin{},
		&database.PluginNotificationDelivery{},
		&database.PluginNotificationDeliveryAttempt{},
	))

	business := createOwnedBusiness(t, "0xOwnerB", "Guest Idempotency Biz")
	enableGuestOrderingForBusiness(t, business.ID)
	telegramPlugin := &database.Plugin{
		Name:        "telegram",
		DisplayName: "Telegram Notifications",
		Category:    database.PluginCategoryIntegration,
		IsActive:    true,
	}
	require.NoError(t, database.GetDB().Create(telegramPlugin).Error)
	require.NoError(t, database.EnableBusinessPlugin(business.ID, telegramPlugin.ID, map[string]interface{}{
		"is_connected": true,
		"chat_id":      "123456789",
		"notification_settings": map[string]interface{}{
			"order_notifications": true,
		},
		"notifications": map[string]interface{}{
			"order_created": true,
		},
	}))
	table := &database.Table{
		BusinessID: business.ID,
		Name:       "T2",
		TableCode:  "guest-table-2",
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)

	menuCategories := []database.MenuCategory{
		{
			ID:   "mains",
			Name: "Mains",
			Items: []database.MenuItem{
				{ID: "burger", Name: "Burger", Price: 10, IsAvailable: true},
			},
		},
	}
	menuPayload, err := json.Marshal(menuCategories)
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: business.ID,
		Categories: string(menuPayload),
		IsActive:   true,
		Version:    1,
	}).Error)

	bill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     fmt.Sprintf("B-guest-%d", time.Now().UnixNano()),
		Status:         database.BillStatusOpen,
		SettlementAddr: business.SettlementAddr,
		TippingAddr:    business.TippingAddr,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	body, err := json.Marshal(map[string]any{
		"bill_id": bill.ID,
		"items": []map[string]any{
			{
				"menu_item_name": "Burger",
				"menu_item_id":   "burger",
				"quantity":       1,
				"price":          10.0,
			},
		},
		"notes": "Retry-safe order",
	})
	require.NoError(t, err)

	buildRequest := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/guest/table/%s/order", table.TableCode), bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-Id", "guest-order-retry-1")
		return req
	}

	firstW := httptest.NewRecorder()
	firstC, _ := gin.CreateTestContext(firstW)
	firstC.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	firstC.Request = buildRequest()

	CreateGuestOrder(firstC)

	require.Equal(t, http.StatusCreated, firstW.Code)

	secondW := httptest.NewRecorder()
	secondC, _ := gin.CreateTestContext(secondW)
	secondC.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	secondC.Request = buildRequest()

	CreateGuestOrder(secondC)

	require.Equal(t, http.StatusOK, secondW.Code)

	var firstResp struct {
		Order database.Order     `json:"order"`
		Quote orderQuoteResponse `json:"quote"`
	}
	require.NoError(t, json.Unmarshal(firstW.Body.Bytes(), &firstResp))
	require.Len(t, firstResp.Quote.Lines, 1)
	assert.Equal(t, 10.0, firstResp.Quote.Lines[0].UnitPrice)
	assert.Equal(t, 10.0, firstResp.Quote.Lines[0].Subtotal)
	assert.NotContains(t, firstW.Body.String(), "unit_price_cents")
	assert.NotContains(t, firstW.Body.String(), "subtotal_cents")

	var secondResp struct {
		Order     database.Order     `json:"order"`
		Quote     orderQuoteResponse `json:"quote"`
		Duplicate bool               `json:"duplicate"`
	}
	require.NoError(t, json.Unmarshal(secondW.Body.Bytes(), &secondResp))

	assert.Equal(t, firstResp.Order.ID, secondResp.Order.ID)
	require.Len(t, secondResp.Quote.Lines, 1)
	assert.Equal(t, 10.0, secondResp.Quote.Lines[0].UnitPrice)
	assert.NotContains(t, secondW.Body.String(), "unit_price_cents")
	assert.True(t, secondResp.Duplicate)

	var count int64
	require.NoError(t, database.GetDB().Model(&database.Order{}).Where("bill_id = ?", bill.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count)

	var deliveryCount int64
	require.NoError(t, database.GetDB().Model(&database.PluginNotificationDelivery{}).
		Where("business_id = ? AND plugin_name = ? AND event_type = ?", business.ID, "telegram", services.PluginEventOrderCreated).
		Count(&deliveryCount).Error)
	assert.EqualValues(t, 1, deliveryCount)

	otherTable := &database.Table{
		BusinessID: business.ID,
		Name:       "T3",
		TableCode:  "guest-table-3",
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(otherTable).Error)

	conflictW := httptest.NewRecorder()
	conflictC, _ := gin.CreateTestContext(conflictW)
	conflictC.Params = gin.Params{{Key: "code", Value: otherTable.TableCode}}
	conflictC.Request = buildRequest()

	CreateGuestOrder(conflictC)

	require.Equal(t, http.StatusConflict, conflictW.Code)
	var conflictResp map[string]any
	require.NoError(t, json.Unmarshal(conflictW.Body.Bytes(), &conflictResp))
	assert.Equal(t, services.GuestCheckoutReplayConflictCode, conflictResp["code"])
	assert.NotContains(t, conflictResp, "bill")
	assert.NotContains(t, conflictResp, "order")
	assert.NotContains(t, conflictW.Body.String(), "public_token")
}

func TestCreateGuestOrder_ItemNotOrderableUsesArrayContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Table{},
		&database.Bill{},
		&database.Order{},
		&database.Menu{},
		&database.Offer{},
		&database.Bundle{},
		&database.InventorySettings{},
		&database.InventoryItem{},
		&database.InventoryRecipe{},
	))

	business := createOwnedBusiness(t, "0xOwnerBlocked", "Guest blocked item contract")
	enableGuestOrderingForBusiness(t, business.ID)
	table := &database.Table{
		BusinessID: business.ID,
		Name:       "Blocked",
		TableCode:  "guest-blocked-item",
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)
	categories := []database.MenuCategory{{
		ID:   "mains",
		Name: "Mains",
		Items: []database.MenuItem{{
			ID: "steak", Name: "Steak", Price: 25, IsAvailable: true,
		}},
	}}
	rawCategories, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: business.ID,
		Categories: string(rawCategories),
		IsActive:   true,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.InventorySettings{
		BusinessID:           business.ID,
		InventoryEnabled:     true,
		AvailabilitySyncMode: database.InventoryAvailabilityModeHardBlock,
	}).Error)
	stock := database.InventoryItem{
		BusinessID: business.ID, Name: "Steak stock", Unit: "unit",
		CurrentQuantity: 0, IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(&stock).Error)
	require.NoError(t, database.GetDB().Create(&database.InventoryRecipe{
		BusinessID: business.ID, MenuItemID: "steak", MenuItemName: "Steak",
		InventoryItemID: stock.ID, QuantityRequired: 1,
	}).Error)

	body, err := json.Marshal(map[string]any{
		"items": []map[string]any{{
			"menu_item_name": "Steak",
			"menu_item_id":   "steak",
			"quantity":       1,
			"price":          25,
		}},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodPost, "/guest/table/guest-blocked-item/order", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("X-Request-Id", "blocked-item-contract")

	CreateGuestOrder(c)

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var response struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
		Details   struct {
			Items []services.ItemNotOrderableDetail `json:"items"`
		} `json:"details"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "item_not_orderable", response.Code)
	assert.Equal(t, "One or more items are unavailable", response.Message)
	assert.Equal(t, "blocked-item-contract", response.RequestID)
	assert.Equal(t, []services.ItemNotOrderableDetail{{
		MenuItemID: "steak",
		Reason:     services.OrderabilityInventoryOut,
	}}, response.Details.Items)
}

func TestQuoteGuestOrder_InventoryOutIsConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Table{}, &database.Bill{}, &database.Order{}, &database.Menu{},
		&database.InventorySettings{}, &database.InventoryItem{}, &database.InventoryRecipe{},
	))

	business := createOwnedBusiness(t, "0xQuote86Owner", "Quote 86 Guest Biz")
	enableGuestOrderingForBusiness(t, business.ID)
	table := &database.Table{
		BusinessID: business.ID,
		Name:       "Blocked",
		TableCode:  "guest-quote-86",
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)
	categories := []database.MenuCategory{{
		ID:   "mains",
		Name: "Mains",
		Items: []database.MenuItem{{
			ID: "steak", Name: "Steak", Price: 25, IsAvailable: true,
		}},
	}}
	rawCategories, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: business.ID,
		Categories: string(rawCategories),
		IsActive:   true,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.InventorySettings{
		BusinessID:           business.ID,
		InventoryEnabled:     true,
		AvailabilitySyncMode: database.InventoryAvailabilityModeHardBlock,
	}).Error)
	stock := database.InventoryItem{
		BusinessID: business.ID, Name: "Steak stock", Unit: "unit",
		CurrentQuantity: 0, IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(&stock).Error)
	require.NoError(t, database.GetDB().Create(&database.InventoryRecipe{
		BusinessID: business.ID, MenuItemID: "steak", MenuItemName: "Steak",
		InventoryItemID: stock.ID, QuantityRequired: 1,
	}).Error)

	body, err := json.Marshal(map[string]any{
		"items": []map[string]any{{
			"menu_item_name": "Steak",
			"menu_item_id":   "steak",
			"quantity":       1,
			"price":          25,
		}},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodPost, "/guest/table/guest-quote-86/order/quote", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	QuoteGuestOrder(c)

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var response struct {
		Code    string `json:"code"`
		Details struct {
			Items []services.ItemNotOrderableDetail `json:"items"`
		} `json:"details"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "item_not_orderable", response.Code)
	require.NotEmpty(t, response.Details.Items)
	assert.Equal(t, "steak", response.Details.Items[0].MenuItemID)
	assert.Equal(t, services.OrderabilityInventoryOut, response.Details.Items[0].Reason)
}

func TestCreateGuestOrder_UnknownMenuItemIDIsItemNotFound(t *testing.T) {
	table := seedGuestSteakMenuAndTable(t, "guest-unknown-item")

	body, err := json.Marshal(map[string]any{
		"items": []map[string]any{{
			"menu_item_id":   "does-not-exist",
			"menu_item_name": "Ghost Steak",
			"quantity":       1,
			"price":          42,
		}},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodPost, "/guest/table/guest-unknown-item/order", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("X-Request-Id", "unknown-item-contract")

	CreateGuestOrder(c)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	var response struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, services.OrderErrCodeItemNotFound, response.Code)
	assert.NotEqual(t, "item_not_orderable", response.Code)
	assert.NotContains(t, strings.ToLower(response.Error), "manual_disabled")
}

func TestCreateBillByTableCode_DisabledOrderingRejectsWhenKitchenDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Table{}, &database.Bill{}))

	business := createOwnedBusiness(t, "0xOwnerKitchenDisabled", "Kitchen Disabled Guest Bill Biz")
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		Updates(map[string]any{
			"kitchen_enabled": false,
			"orders_enabled":  true,
		}).Error)
	table := &database.Table{
		BusinessID: business.ID,
		Name:       "T-kitchen-disabled",
		TableCode:  "guest-kitchen-disabled",
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/guest/table/%s/bill", table.TableCode), nil)

	CreateBillByTableCode(c)

	require.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "ordering_disabled")

	var count int64
	require.NoError(t, database.GetDB().Model(&database.Bill{}).Where("table_id = ?", table.ID).Count(&count).Error)
	assert.Zero(t, count)
}

func TestCreateGuestOrder_DisabledOrderingRejectsWhenOrdersDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Table{},
		&database.Bill{},
		&database.Order{},
		&database.Menu{},
		&database.Offer{},
		&database.Bundle{},
	))

	business := createOwnedBusiness(t, "0xOwnerOrdersDisabled", "Orders Disabled Guest Order Biz")
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		Updates(map[string]any{
			"kitchen_enabled": true,
			"orders_enabled":  false,
		}).Error)
	table := &database.Table{
		BusinessID: business.ID,
		Name:       "T-orders-disabled",
		TableCode:  "guest-orders-disabled",
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)

	bill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     fmt.Sprintf("B-disabled-%d", time.Now().UnixNano()),
		Status:         database.BillStatusOpen,
		SettlementAddr: business.SettlementAddr,
		TippingAddr:    business.TippingAddr,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	body, err := json.Marshal(map[string]any{
		"bill_id": bill.ID,
		"items": []map[string]any{
			{
				"menu_item_name": "Burger",
				"quantity":       1,
				"price":          10.0,
			},
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/guest/table/%s/order", table.TableCode), bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	CreateGuestOrder(c)

	require.Equal(t, http.StatusConflict, w.Code)
	assert.Contains(t, w.Body.String(), "ordering_disabled")

	var count int64
	require.NoError(t, database.GetDB().Model(&database.Order{}).Where("bill_id = ?", bill.ID).Count(&count).Error)
	assert.Zero(t, count)
}

func TestCreateBillByTableCode_AttachesOptionalCustomerFromCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Customer{},
		&database.Table{},
		&database.Bill{},
	))

	business := createOwnedBusiness(t, "0xOwnerGuestCRM", "Guest CRM Bill Biz")
	enableGuestOrderingForBusiness(t, business.ID)
	table := &database.Table{
		BusinessID: business.ID,
		Name:       "T-crm",
		TableCode:  "guest-crm-bill",
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)

	customer := &database.Customer{
		Email:    "guest-crm@example.com",
		Name:     "Guest CRM",
		IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(customer).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/guest/table/%s/bill", table.TableCode), nil)
	c.Request.AddCookie(&http.Cookie{Name: "customer_token", Value: validOptionalCustomerToken(t, customer)})

	CreateBillByTableCode(c)

	require.Equal(t, http.StatusCreated, w.Code)

	var resp struct {
		Bill struct {
			ID            uint  `json:"id"`
			CRMCustomerID *uint `json:"crm_customer_id"`
		} `json:"bill"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Bill.CRMCustomerID)
	assert.Equal(t, customer.ID, *resp.Bill.CRMCustomerID)

	var bill database.Bill
	require.NoError(t, database.GetDB().First(&bill, resp.Bill.ID).Error)
	require.NotNil(t, bill.CRMCustomerID)
	assert.Equal(t, customer.ID, *bill.CRMCustomerID)
}

func TestCreateBillByTableCode_WithoutCustomerCookieRemainsAnonymous(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Customer{},
		&database.Table{},
		&database.Bill{},
	))

	business := createOwnedBusiness(t, "0xOwnerGuestAnon", "Guest Anonymous Bill Biz")
	enableGuestOrderingForBusiness(t, business.ID)
	table := &database.Table{
		BusinessID: business.ID,
		Name:       "T-anon",
		TableCode:  "guest-anon-bill",
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/guest/table/%s/bill", table.TableCode), nil)

	CreateBillByTableCode(c)

	require.Equal(t, http.StatusCreated, w.Code)

	var resp struct {
		Bill struct {
			ID            uint  `json:"id"`
			CRMCustomerID *uint `json:"crm_customer_id"`
		} `json:"bill"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Nil(t, resp.Bill.CRMCustomerID)

	var bill database.Bill
	require.NoError(t, database.GetDB().First(&bill, resp.Bill.ID).Error)
	assert.Nil(t, bill.CRMCustomerID)
}

// TestCreateBillByTableCode_SnapshotsSettlementWallet is the regression guard
// for the guest-USDC-settlement bug. The unauthenticated QR table context is
// loaded through publicBusinessColumns, which DELIBERATELY drops the owner
// settlement/tipping wallet columns. A guest-created bill was therefore born
// with an empty settlement_addr, and POST /guest/bill/:token/crypto-payment
// then rejected the USDC payment with 503 "This business has not configured a
// settlement wallet" even though the business had one configured. The bill must
// snapshot the real owner wallets at creation time so guest USDC settlement can
// complete.
func TestCreateBillByTableCode_SnapshotsSettlementWallet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Table{},
		&database.Bill{},
	))

	business := createOwnedBusiness(t, "0xOwnerGuestUSDC", "Guest USDC Bill Biz")
	enableGuestOrderingForBusiness(t, business.ID)
	require.NotEmpty(t, business.SettlementAddr, "test business must have a settlement wallet configured")

	table := &database.Table{
		BusinessID: business.ID,
		Name:       "T-usdc",
		TableCode:  "guest-usdc-bill",
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/guest/table/%s/bill", table.TableCode), nil)

	CreateBillByTableCode(c)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp struct {
		Bill struct {
			ID uint `json:"id"`
		} `json:"bill"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotZero(t, resp.Bill.ID)

	var bill database.Bill
	require.NoError(t, database.GetDB().First(&bill, resp.Bill.ID).Error)
	assert.Equal(t, business.SettlementAddr, bill.SettlementAddr,
		"guest-created bill must snapshot the business settlement wallet (the USDC settlement destination)")
	assert.Equal(t, business.TippingAddr, bill.TippingAddr,
		"guest-created bill must snapshot the business tipping wallet")
	require.NotEmpty(t, bill.SettlementAddr,
		"empty settlement_addr makes guest crypto-payment fail 503 'not configured a settlement wallet'")
}

func TestCreateBillByTableCode_RejectsInactiveBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Table{}, &database.Bill{}))

	business := createOwnedBusiness(t, "0xOwnerInactive", "Inactive Guest Bill Biz")
	table := &database.Table{
		BusinessID: business.ID,
		Name:       "T-inactive",
		TableCode:  "guest-inactive-bill",
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		Update("is_active", false).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/guest/table/%s/bill", table.TableCode), nil)

	CreateBillByTableCode(c)

	require.Equal(t, http.StatusNotFound, w.Code)

	var count int64
	require.NoError(t, database.GetDB().Model(&database.Bill{}).Where("table_id = ?", table.ID).Count(&count).Error)
	assert.Zero(t, count)
}

func TestGetOpenBillByTableCodeReturnsPartialBill(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Table{}, &database.Bill{}))

	business := createOwnedBusiness(t, "0xGuestPartial", "Guest Partial")
	table := &database.Table{
		BusinessID: business.ID,
		Name:       "12",
		TableCode:  "partial-code",
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)
	require.NoError(t, database.GetDB().Create(&database.Bill{
		BusinessID:  business.ID,
		TableID:     table.ID,
		BillNumber:  "PARTIAL-GUEST",
		TotalAmount: 4000,
		PaidAmount:  1000,
		Status:      database.BillStatusPartial,
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/guest/table/partial-code/bill", nil)

	GetOpenBillByTableCode(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), "PARTIAL-GUEST")
}

func TestGetOpenBillByTableCodeUsesProjectedBillLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupGuestHandlerTestDBWithLogger(t, recorder)

	business := createOwnedBusiness(t, "0xGuestProjectedBill", "Guest Projected Bill")
	table := &database.Table{
		BusinessID: business.ID,
		Name:       "14",
		TableCode:  "projected-bill-code",
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)
	bill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     "PROJECTED-GUEST",
		Subtotal:       4000,
		TotalAmount:    4000,
		Status:         database.BillStatusOpen,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	require.NoError(t, database.GetDB().Create(&database.BillItem{
		ID:       "projected-item",
		BillID:   bill.ID,
		Name:     "Coffee",
		Price:    4000,
		Quantity: 1,
		Subtotal: 4000,
	}).Error)

	recorder.statements = nil
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/guest/table/projected-bill-code/bill", nil)

	GetOpenBillByTableCode(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "PROJECTED-GUEST")
	require.Zero(t, recorder.selectStarCount("bills"), "public open-bill lookup should project only response fields")
	require.False(t, recordedSelectMentionsColumn(recorder, "bills", "items"), "normalized public open-bill lookup should not read legacy items JSON")
	// PV-LIVE-20260720-001: open-by-table must project the guest capability.
	require.True(t, recordedSelectMentionsColumn(recorder, "bills", "public_token"),
		"public open-bill projection must include public_token")
	require.True(t, recordedSelectMentionsColumn(recorder, "bills", "loyalty_discount_cents"),
		"public open-bill projection must include loyalty_discount_cents")
	require.NotEmpty(t, bill.PublicToken, "create path must mint public_token")
	require.Contains(t, w.Body.String(), bill.PublicToken,
		"GET open bill by table must return non-empty public_token for guest capability routes")
}

// TestGetOpenBillByTableCodeReturnsPublicToken is the regression for
// PV-LIVE-20260720-001: GetPublicGuestOpenBillByTableID used to omit
// public_token from its Select, so guests received "" and the FE threw.
func TestGetOpenBillByTableCodeReturnsPublicToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Table{}, &database.Bill{}))

	business := createOwnedBusiness(t, "0xGuestPublicToken", "Guest Public Token")
	table := &database.Table{
		BusinessID: business.ID,
		Name:       "15",
		TableCode:  "public-token-bill-code",
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)
	bill := &database.Bill{
		BusinessID:  business.ID,
		TableID:     table.ID,
		BillNumber:  "TOKEN-GUEST-001",
		Subtotal:    1800,
		TotalAmount: 1800,
		Status:      database.BillStatusOpen,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	require.NotEmpty(t, bill.PublicToken)

	// Confirm DB has the token while the in-memory create path is not reused
	// for the open-bill GET (which re-reads via projection).
	var stored database.Bill
	require.NoError(t, database.GetDB().Select("id", "public_token", "bill_number").
		Where("id = ?", bill.ID).First(&stored).Error)
	require.Equal(t, bill.PublicToken, stored.PublicToken)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/guest/table/public-token-bill-code/bill", nil)

	GetOpenBillByTableCode(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body struct {
		Bill struct {
			BillNumber  string `json:"bill_number"`
			PublicToken string `json:"public_token"`
		} `json:"bill"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "TOKEN-GUEST-001", body.Bill.BillNumber)
	require.NotEmpty(t, body.Bill.PublicToken, "open-by-table must return non-empty public_token")
	require.Equal(t, bill.PublicToken, body.Bill.PublicToken)
	require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"),
		"guest bill responses must not be heuristically cacheable (#524)")
}

func TestCreateBillByTableCodeRejectsPartialActiveBill(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Table{}, &database.Bill{}))

	business := createOwnedBusiness(t, "0xGuestPartialDup", "Guest Partial Dup")
	enableGuestOrderingForBusiness(t, business.ID)
	table := &database.Table{
		BusinessID: business.ID,
		Name:       "12",
		TableCode:  "partial-dup",
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)
	require.NoError(t, database.GetDB().Create(&database.Bill{
		BusinessID:  business.ID,
		TableID:     table.ID,
		BillNumber:  "PARTIAL-DUP",
		TotalAmount: 4000,
		PaidAmount:  1000,
		Status:      database.BillStatusPartial,
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/guest/table/partial-dup/bill", nil)

	CreateBillByTableCode(c)

	require.Equal(t, http.StatusConflict, w.Code)
}

func TestCreateGuestBillByTableCode_ReturnsConflictWhenMalformedActiveBillExists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Table{}, &database.Bill{}))
	createSQLiteBillItemsTable(t)

	business := createOwnedBusiness(t, "0xOwnerGuestLookup", "Guest Lookup Error Biz")
	enableGuestOrderingForBusiness(t, business.ID)
	table := &database.Table{
		BusinessID: business.ID,
		Name:       "T-guest-lookup",
		TableCode:  "guest-lookup-error",
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)
	createMalformedActiveBillForTable(t, business.ID, table.ID)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/guest/table/%s/bill", table.TableCode), nil)

	CreateBillByTableCode(c)

	require.Equal(t, http.StatusConflict, w.Code)

	var count int64
	require.NoError(t, database.GetDB().Model(&database.Bill{}).Where("table_id = ?", table.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestGetOpenBillByTableCode_ReturnsInternalErrorWhenActiveBillLookupFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Table{}, &database.Bill{}))
	createSQLiteBillItemsTable(t)

	business := createOwnedBusiness(t, "0xOwnerGuestOpenLookup", "Guest Open Lookup Error Biz")
	table := &database.Table{
		BusinessID: business.ID,
		Name:       "T-guest-open-lookup",
		TableCode:  "guest-open-lookup-error",
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)
	createMalformedActiveBillForTable(t, business.ID, table.ID)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/table/%s/bill/open", table.TableCode), nil)

	GetOpenBillByTableCode(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}

// H3: guests hit this route on every page load and each ~10s poll. When no
// bill is active the response must be a benign 200 {"bill": null, "items": []}
// instead of a 404, which otherwise logs a browser console error per pageview.
func TestGetOpenBillByTableCode_NoBillReturns200Null(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Table{}, &database.Bill{}))

	business := createOwnedBusiness(t, "0xOwnerGuestOpenNoBill", "Guest Open No Bill Biz")
	table := &database.Table{
		BusinessID: business.ID,
		Name:       "T-guest-open-no-bill",
		TableCode:  "guest-open-no-bill",
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/table/%s/bill/open", table.TableCode), nil)

	GetOpenBillByTableCode(c)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Bill  *json.RawMessage `json:"bill"`
		Items []any            `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Nil(t, body.Bill)
	require.NotNil(t, body.Items)
}

func setupGuestHandlerTestDBWithLogger(t testing.TB, gormLogger logger.Interface) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: gormLogger})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
	})

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Table{},
		&database.Bill{},
	))
	createSQLiteBillItemsTable(t)
	services.ResetPricingCache()

	return gormDB
}

func BenchmarkGuestOpenBillByTableCodeSQLite(b *testing.B) {
	gin.SetMode(gin.TestMode)
	db := setupGuestHandlerTestDBWithLogger(b, logger.Default.LogMode(logger.Silent))

	seed := time.Now().UnixNano()
	business := &database.Business{
		BusinessId:     fmt.Sprintf("bench-open-bill-%d", seed),
		Name:           "Bench Open Bill",
		OwnerAddress:   "0x1111111111111111111111111111111111111111",
		SettlementAddr: "0x2222222222222222222222222222222222222222",
		TippingAddr:    "0x3333333333333333333333333333333333333333",
		IsActive:       true,
	}
	require.NoError(b, db.Create(business).Error)
	tableCode := fmt.Sprintf("bench-open-bill-%d", seed)
	table := &database.Table{
		BusinessID: business.ID,
		Name:       "Bench",
		TableCode:  tableCode,
		IsActive:   true,
	}
	require.NoError(b, db.Create(table).Error)
	bill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     fmt.Sprintf("BENCH-%d", seed),
		Subtotal:       12500,
		TaxAmount:      500,
		TotalAmount:    13000,
		PaidAmount:     3000,
		Status:         database.BillStatusPartial,
		SettlementAddr: business.SettlementAddr,
		TippingAddr:    business.TippingAddr,
	}
	require.NoError(b, db.Create(bill).Error)
	items := make([]database.BillItem, 0, 8)
	for i := 0; i < 8; i++ {
		items = append(items, database.BillItem{
			ID:       fmt.Sprintf("bench-item-%d", i),
			BillID:   bill.ID,
			Name:     fmt.Sprintf("Item %d", i),
			Price:    1500,
			Quantity: 1,
			Subtotal: 1500,
		})
	}
	require.NoError(b, db.Create(&items).Error)

	router := gin.New()
	router.GET("/guest/table/:code/bill", GetOpenBillByTableCode)
	path := fmt.Sprintf("/guest/table/%s/bill", tableCode)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}
	}
}

func recordedSelectMentionsColumn(recorder *publicGuestSQLRecorder, table string, column string) bool {
	quotedTable := "`" + table + "`"
	quotedColumn := "`" + column + "`"
	for _, statement := range recorder.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		if !strings.HasPrefix(normalized, "select ") {
			continue
		}
		if !strings.Contains(normalized, "from "+quotedTable) && !strings.Contains(normalized, "from \""+table+"\"") {
			continue
		}
		if strings.Contains(normalized, quotedColumn) || strings.Contains(normalized, "."+column) {
			return true
		}
	}
	return false
}

func TestCreateGuestOrder_RejectsInactiveBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Table{},
		&database.Bill{},
		&database.Order{},
		&database.Menu{},
		&database.Offer{},
		&database.Bundle{},
	))

	business := createOwnedBusiness(t, "0xOwnerInactive2", "Inactive Guest Order Biz")
	table := &database.Table{
		BusinessID: business.ID,
		Name:       "T-order-inactive",
		TableCode:  "guest-inactive-order",
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)

	bill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     fmt.Sprintf("B-inactive-%d", time.Now().UnixNano()),
		Status:         database.BillStatusOpen,
		SettlementAddr: business.SettlementAddr,
		TippingAddr:    business.TippingAddr,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		Update("is_active", false).Error)

	body, err := json.Marshal(map[string]any{
		"bill_id": bill.ID,
		"items": []map[string]any{
			{
				"menu_item_name": "Burger",
				"quantity":       1,
				"price":          10.0,
			},
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/guest/table/%s/order", table.TableCode), bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	CreateGuestOrder(c)

	require.Equal(t, http.StatusNotFound, w.Code)

	var count int64
	require.NoError(t, database.GetDB().Model(&database.Order{}).Where("bill_id = ?", bill.ID).Count(&count).Error)
	assert.Zero(t, count)
}
