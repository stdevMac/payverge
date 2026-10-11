package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func failOperatorOrderInventoryRecipeQuery(t *testing.T) {
	t.Helper()
	db := database.GetDB()
	callbackName := "payverge:test:operator_order_inventory_projection_failure"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "inventory_recipes" {
			_ = tx.AddError(errors.New("injected inventory projection failure"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callbackName) })
}

func seedOperatorOrderInventory(t *testing.T, businessID uint, mode string) *database.Bundle {
	t.Helper()

	require.NoError(t, database.GetDB().Create(&database.InventorySettings{
		BusinessID: businessID, InventoryEnabled: true, AvailabilitySyncMode: mode,
	}).Error)
	stock := &database.InventoryItem{
		BusinessID: businessID, Name: "Burger patties", Unit: "unit",
		CurrentQuantity: 0, ReorderThreshold: 1, IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(stock).Error)
	require.NoError(t, database.GetDB().Create(&database.InventoryRecipe{
		BusinessID: businessID, MenuItemID: "burger", MenuItemName: "Burger",
		InventoryItemID: stock.ID, QuantityRequired: 1,
	}).Error)

	refs, err := json.Marshal([]database.BundleItemRef{{MenuItemID: "burger", Name: "Burger", Quantity: 1}})
	require.NoError(t, err)
	bundle := &database.Bundle{
		BusinessID: businessID, Name: "Burger Deal", Price: 8, Items: string(refs), IsActive: true,
	}
	require.NoError(t, database.GetDB().Create(bundle).Error)
	return bundle
}

func performOperatorOrderCreate(t *testing.T, businessID, billID uint, bundle *database.Bundle) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(CreateOrderRequest{
		BillID: billID,
		Items: []CreateOrderItemRequest{{
			MenuItemName: bundle.Name, MenuItemID: fmt.Sprintf("bundle:%d", bundle.ID),
			Quantity: 1, ItemType: services.OrderItemTypeBundle, BundleID: &bundle.ID,
		}},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", businessID)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("X-Request-Id", "operator-orderability-request")
	c.Set("address", "0xoperator")
	NewOrderHandler(nil).CreateOrder(c)
	return w
}

func responseOrderID(t *testing.T, w *httptest.ResponseRecorder) uint {
	t.Helper()
	var response struct {
		Order database.Order `json:"order"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.NotZero(t, response.Order.ID)
	return response.Order.ID
}

func TestCreateOrderRejectsHardBlockedExpandedBundleChild(t *testing.T) {
	setupHandlerTestDB(t)
	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)
	createTestMenu(t, business.ID)
	bundle := seedOperatorOrderInventory(t, business.ID, database.InventoryAvailabilityModeHardBlock)

	w := performOperatorOrderCreate(t, business.ID, bill.ID, bundle)

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var response struct {
		Code    string `json:"code"`
		Details struct {
			Items []services.ItemNotOrderableDetail `json:"items"`
		} `json:"details"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, server.ErrCodeInventoryInsufficientStock, response.Code)
	require.Equal(t, []services.ItemNotOrderableDetail{{
		MenuItemID: "burger", Reason: services.OrderabilityInventoryOut,
	}}, response.Details.Items)

	var count int64
	require.NoError(t, database.GetDB().Model(&database.Order{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestCreateOrderRejectsWarnModeOutOfStockExpandedBundleChild(t *testing.T) {
	setupHandlerTestDB(t)
	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)
	createTestMenu(t, business.ID)
	bundle := seedOperatorOrderInventory(t, business.ID, database.InventoryAvailabilityModeWarn)

	w := performOperatorOrderCreate(t, business.ID, bill.ID, bundle)

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var response struct {
		Code    string `json:"code"`
		Details struct {
			Items []services.ItemNotOrderableDetail `json:"items"`
		} `json:"details"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, server.ErrCodeInventoryInsufficientStock, response.Code)
	require.Equal(t, []services.ItemNotOrderableDetail{{
		MenuItemID: "burger", Reason: services.OrderabilityInventoryOut,
	}}, response.Details.Items)

	var count int64
	require.NoError(t, database.GetDB().Model(&database.Order{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestCreateOrderFailsClosedWhenInventoryProjectionFails(t *testing.T) {
	setupHandlerTestDB(t)
	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)
	createTestMenu(t, business.ID)
	bundle := seedOperatorOrderInventory(t, business.ID, database.InventoryAvailabilityModeHardBlock)
	failOperatorOrderInventoryRecipeQuery(t)

	w := performOperatorOrderCreate(t, business.ID, bill.ID, bundle)

	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	var response struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, server.ErrCodeInternal, response.Code)

	var count int64
	require.NoError(t, database.GetDB().Model(&database.Order{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestCreateOrderReplaysBeforeInventoryBecomesUnavailable(t *testing.T) {
	setupHandlerTestDB(t)
	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)
	createTestMenu(t, business.ID)
	// Seed with stock so the first create succeeds under warn mode; then
	// deplete + switch to hard_block before the idempotent replay.
	bundle := seedOperatorOrderInventory(t, business.ID, database.InventoryAvailabilityModeWarn)
	require.NoError(t, database.GetDB().Model(&database.InventoryItem{}).
		Where("business_id = ?", business.ID).
		Update("current_quantity", 10).Error)

	first := performOperatorOrderCreate(t, business.ID, bill.ID, bundle)
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())
	originalID := responseOrderID(t, first)
	require.NoError(t, database.GetDB().Model(&database.InventoryItem{}).
		Where("business_id = ?", business.ID).
		Update("current_quantity", 0).Error)
	require.NoError(t, database.GetDB().Model(&database.InventorySettings{}).
		Where("business_id = ?", business.ID).
		Update("availability_sync_mode", database.InventoryAvailabilityModeHardBlock).Error)

	replay := performOperatorOrderCreate(t, business.ID, bill.ID, bundle)
	require.Equal(t, http.StatusOK, replay.Code, replay.Body.String())
	require.Contains(t, replay.Body.String(), `"duplicate":true`)
	require.Equal(t, originalID, responseOrderID(t, replay))

	var count int64
	require.NoError(t, database.GetDB().Model(&database.Order{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestCreateOrderReplaysBeforeInventoryProjectionFailure(t *testing.T) {
	setupHandlerTestDB(t)
	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)
	createTestMenu(t, business.ID)
	bundle := seedOperatorOrderInventory(t, business.ID, database.InventoryAvailabilityModeWarn)
	require.NoError(t, database.GetDB().Model(&database.InventoryItem{}).
		Where("business_id = ?", business.ID).
		Update("current_quantity", 10).Error)

	first := performOperatorOrderCreate(t, business.ID, bill.ID, bundle)
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())
	originalID := responseOrderID(t, first)
	failOperatorOrderInventoryRecipeQuery(t)

	replay := performOperatorOrderCreate(t, business.ID, bill.ID, bundle)
	require.Equal(t, http.StatusOK, replay.Code, replay.Body.String())
	require.Contains(t, replay.Body.String(), `"duplicate":true`)
	require.Equal(t, originalID, responseOrderID(t, replay))

	var count int64
	require.NoError(t, database.GetDB().Model(&database.Order{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
