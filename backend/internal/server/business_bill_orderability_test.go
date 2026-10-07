package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func failBusinessBillInventoryRecipeQuery(t *testing.T) {
	t.Helper()
	db := database.GetDB()
	callbackName := "payverge:test:business_bill_inventory_projection_failure"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement != nil && tx.Statement.Schema != nil && tx.Statement.Schema.Table == "inventory_recipes" {
			_ = tx.AddError(errors.New("injected inventory projection failure"))
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callbackName) })
}

func seedBusinessBillOrderability(t *testing.T, businessID uint, mode string) *database.Bundle {
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

func performBusinessBillCreateWithBundle(t *testing.T, business *database.Business, counterID uint, bundle *database.Bundle) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"counter_id": counterID,
		"items": []map[string]any{{
			"name": bundle.Name, "menu_item_id": fmt.Sprintf("bundle:%d", bundle.ID),
			"quantity": 1, "item_type": services.OrderItemTypeBundle, "bundle_id": bundle.ID,
		}},
	})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatUint(uint64(business.ID), 10)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", business.OwnerAddress)
	CreateBill(c)
	return w
}

func setupBusinessBillOrderabilityTest(t *testing.T, mode string) (*database.Business, *database.Counter, *database.Bundle) {
	t.Helper()
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Counter{}, &database.Table{}, &database.Bill{}, &database.BillHistoryEvent{},
		&database.Menu{}, &database.Offer{}, &database.Bundle{},
		&database.InventorySettings{}, &database.InventoryItem{}, &database.InventoryRecipe{}, &database.InventoryMovement{},
		&database.OperationalAlert{}, &database.OperationalAlertEvent{}, &database.BusinessAlertSettings{},
	))
	createSQLiteBillItemsTable(t)
	business := createBusinessHandlerTestBusiness(t, "0xOwnerOrderability", "biz-bill-orderability-"+mode)
	createBillCreationMenu(t, business.ID)
	counter := &database.Counter{BusinessID: business.ID, Name: "Front", IsActive: true}
	require.NoError(t, database.GetDB().Create(counter).Error)
	bundle := seedBusinessBillOrderability(t, business.ID, mode)
	return business, counter, bundle
}

func TestCreateBillRejectsHardBlockedExpandedBundleChild(t *testing.T) {
	business, counter, bundle := setupBusinessBillOrderabilityTest(t, database.InventoryAvailabilityModeHardBlock)

	w := performBusinessBillCreateWithBundle(t, business, counter.ID, bundle)

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var response struct {
		Code    string `json:"code"`
		Details struct {
			Items []services.ItemNotOrderableDetail `json:"items"`
		} `json:"details"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, ErrCodeInventoryInsufficientStock, response.Code)
	require.Equal(t, []services.ItemNotOrderableDetail{{
		MenuItemID: "burger", Reason: services.OrderabilityInventoryOut,
	}}, response.Details.Items)

	var count int64
	require.NoError(t, database.GetDB().Model(&database.Bill{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestCreateBillRejectsWarnModeOutOfStockExpandedBundleChild(t *testing.T) {
	business, counter, bundle := setupBusinessBillOrderabilityTest(t, database.InventoryAvailabilityModeWarn)

	w := performBusinessBillCreateWithBundle(t, business, counter.ID, bundle)

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var response struct {
		Code    string `json:"code"`
		Details struct {
			Items []services.ItemNotOrderableDetail `json:"items"`
		} `json:"details"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, ErrCodeInventoryInsufficientStock, response.Code)
	require.Equal(t, []services.ItemNotOrderableDetail{{
		MenuItemID: "burger", Reason: services.OrderabilityInventoryOut,
	}}, response.Details.Items)

	var count int64
	require.NoError(t, database.GetDB().Model(&database.Bill{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestCreateBillFailsClosedWhenInventoryProjectionFails(t *testing.T) {
	business, counter, bundle := setupBusinessBillOrderabilityTest(t, database.InventoryAvailabilityModeHardBlock)
	failBusinessBillInventoryRecipeQuery(t)

	w := performBusinessBillCreateWithBundle(t, business, counter.ID, bundle)

	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	var response struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Equal(t, ErrCodeInternal, response.Code)

	var count int64
	require.NoError(t, database.GetDB().Model(&database.Bill{}).Count(&count).Error)
	require.Zero(t, count)
}
