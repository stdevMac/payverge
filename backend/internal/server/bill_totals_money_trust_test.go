package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/money"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateBill_DoesNotInflateBundleChildrenOrStripOptions is the dinner-trust
// gate for bill #755-style drift: list/tables/analytics must not disagree with
// detail because UpdateBill rewrote lines as bare Price×Qty (billing bundle
// children and dropping option surcharges).
func TestUpdateBill_DoesNotInflateBundleChildrenOrStripOptions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Table{}, &database.Bill{}, &database.Payment{}, &database.BillHistoryEvent{}, &database.Menu{}))
	createSQLiteBusinessRegressionBillItemsTable(t, gormDB)

	business := createOwnedBusiness(t, "0xOwnerMoneyTrust", "Money Trust Biz")
	business.TaxRate = 8.875
	business.ServiceFeeRate = 7.5
	require.NoError(t, database.GetDB().Save(business).Error)

	// GetBillByID hydrates leftover $0 bundle children from the live catalog.
	cats := []database.MenuCategory{{
		ID: "c1", Name: "Mains",
		Items: []database.MenuItem{
			{ID: "item-steak", Name: "Steak Plate", Price: 24, IsAvailable: true},
			{ID: "item-burger", Name: "Burger", Price: 12, IsAvailable: true},
		},
	}}
	menuJSON, err := json.Marshal(cats)
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: business.ID, Categories: string(menuJSON), IsActive: true, Version: 1,
	}).Error)

	bundleID := uint(10)
	parent := database.BillItem{
		ID:         "bundle-parent",
		MenuItemID: "bundle-10",
		Name:       "Date Night for Two",
		Price:      80,
		Quantity:   1,
		ItemType:   services.OrderItemTypeBundle,
		BundleID:   &bundleID,
		Subtotal:   80,
	}
	child := database.BillItem{
		ID:             "bundle-child",
		MenuItemID:     "item-steak",
		Name:           "Steak Plate",
		Price:          0, // catalog price must not bill when type is bundle_item
		Quantity:       1,
		ItemType:       services.OrderItemTypeBundleItem,
		BundleID:       &bundleID,
		ParentBundleID: &bundleID,
		Subtotal:       0,
	}
	withOptions := database.BillItem{
		ID:         "menu-opt",
		MenuItemID: "item-burger",
		Name:       "Burger",
		Price:      12,
		Quantity:   2,
		ItemType:   services.OrderItemTypeMenuItem,
		Options:    []database.MenuItemOption{{ID: "cheese", Name: "Extra Cheese", PriceChange: 1.5}},
		// Authoritative line = (12 + 1.50) × 2 = 27.00
		Subtotal: 27,
	}

	seedItems := []database.BillItem{parent, child, withOptions}
	itemsJSON, err := json.Marshal(seedItems)
	require.NoError(t, err)

	subtotalCents := int64(0)
	for _, item := range seedItems {
		if money.IsInformationalBillLine(item.ItemType) {
			continue
		}
		subtotalCents += money.CentsFromMajor(item.Subtotal)
	}
	tax, service, gross := money.BillTotalsCents(subtotalCents, business.TaxRate, business.ServiceFeeRate)

	bill := &database.Bill{
		BusinessID:       business.ID,
		BillNumber:       "MT-755",
		Status:           database.BillStatusOpen,
		Items:            string(itemsJSON),
		Subtotal:         subtotalCents,
		TaxAmount:        tax,
		ServiceFeeAmount: service,
		TotalAmount:      gross,
		SettlementAddr:   "0x1111111111111111111111111111111111111111",
		TippingAddr:      "0x2222222222222222222222222222222222222222",
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xOwnerMoneyTrust")
		c.Next()
	})
	router.PUT("/bills/:bill_id", UpdateBill)

	// Round-trip the same items. A buggy Price×Qty rewrite would:
	//   - bill the child Steak Plate catalog price (inflate), and/or
	//   - drop Extra Cheese (deflate).
	payload := map[string]any{
		"items": []map[string]any{
			{
				"id": "bundle-parent", "menu_item_id": "bundle-10", "name": "Date Night for Two",
				"price": 80, "quantity": 1, "item_type": "bundle", "bundle_id": bundleID, "subtotal": 80,
			},
			{
				"id": "bundle-child", "menu_item_id": "item-steak", "name": "Steak Plate",
				// Deliberately send a non-zero catalog price — must stay non-billable.
				"price": 24, "quantity": 1, "item_type": "bundle_item", "bundle_id": bundleID,
				"parent_bundle_id": bundleID, "subtotal": 0,
			},
			{
				"id": "menu-opt", "menu_item_id": "item-burger", "name": "Burger",
				"price": 12, "quantity": 2, "item_type": "menu_item",
				"options":  []map[string]any{{"id": "cheese", "name": "Extra Cheese", "price_change": 1.5}},
				"subtotal": 27,
			},
		},
	}

	w := performBusinessJSONRequest(t, router, http.MethodPut, fmt.Sprintf("/bills/%d", bill.ID), payload)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var updated database.Bill
	require.NoError(t, database.GetDB().First(&updated, bill.ID).Error)

	assert.Equal(t, int64(10700), updated.Subtotal, "80 + 27 = 107.00; child must not add")
	wantTax, wantService, wantTotal := money.BillTotalsCents(10700, 8.875, 7.5)
	assert.Equal(t, wantTax, updated.TaxAmount)
	assert.Equal(t, wantService, updated.ServiceFeeAmount)
	assert.Equal(t, wantTotal, updated.TotalAmount)
	assert.Equal(t,
		updated.Subtotal+updated.TaxAmount+updated.ServiceFeeAmount,
		updated.TotalAmount,
	)
}

func TestCloseBill_RefusesUnpaidRemaining(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Table{}, &database.Bill{}, &database.Order{}, &database.Payment{}, &database.BillHistoryEvent{}))
	createSQLiteBusinessRegressionBillItemsTable(t, gormDB)

	business := createOwnedBusiness(t, "0xOwnerCloseUnpaid", "Close Unpaid Biz")
	bill := &database.Bill{
		BusinessID:     business.ID,
		BillNumber:     "UNPAID-762",
		Status:         database.BillStatusOpen,
		TotalAmount:    3782,
		PaidAmount:     0,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xOwnerCloseUnpaid")
		c.Next()
	})
	router.POST("/bills/:bill_id/close", CloseBill)

	w := performBusinessJSONRequest(t, router, http.MethodPost, fmt.Sprintf("/bills/%d/close", bill.ID), map[string]any{})
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "bill_unpaid_remaining")

	var reloaded database.Bill
	require.NoError(t, database.GetDB().First(&reloaded, bill.ID).Error)
	assert.Equal(t, database.BillStatusOpen, reloaded.Status)
}

func TestCloseBill_RefusesZeroWithLiveKitchen(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Table{}, &database.Bill{}, &database.Order{}, &database.Payment{}, &database.BillHistoryEvent{}))
	createSQLiteBusinessRegressionBillItemsTable(t, gormDB)

	business := createOwnedBusiness(t, "0xOwnerCloseKitchen", "Close Kitchen Biz")
	bill := &database.Bill{
		BusinessID:     business.ID,
		BillNumber:     "ZERO-1132",
		Status:         database.BillStatusOpen,
		TotalAmount:    0,
		PaidAmount:     0,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	require.NoError(t, database.GetDB().Create(&database.Order{
		BillID:      bill.ID,
		BusinessID:  business.ID,
		OrderNumber: "G86-36604192",
		Status:      database.OrderStatusInKitchen,
		Items:       `[{"name":"Iced Tea"}]`,
	}).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xOwnerCloseKitchen")
		c.Next()
	})
	router.POST("/bills/:bill_id/close", CloseBill)

	w := performBusinessJSONRequest(t, router, http.MethodPost, fmt.Sprintf("/bills/%d/close", bill.ID), map[string]any{})
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "bill_live_kitchen")

	var reloaded database.Bill
	require.NoError(t, database.GetDB().First(&reloaded, bill.ID).Error)
	assert.Equal(t, database.BillStatusOpen, reloaded.Status)
}
