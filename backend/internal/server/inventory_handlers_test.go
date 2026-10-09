package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupInventoryHandlerTestDB(t *testing.T) {
	t.Helper()

	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.InventorySettings{},
		&database.InventoryItem{},
		&database.InventoryMovement{},
	))
}

func createInventoryHandlerTestBusiness(t *testing.T) *database.Business {
	t.Helper()
	business := &database.Business{
		BusinessId:      "inventory-handler-" + strconv.FormatInt(time.Now().UnixNano(), 10),
		Name:            "Inventory Handler",
		OwnerAddress:    "0xowner",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
		DefaultLanguage: "en",
		IsActive:        true,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	require.NoError(t, database.GetDB().Create(business).Error)
	return business
}

func TestGetInventorySettings_AcceptsBusinessSlug(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupInventoryHandlerTestDB(t)

	business := &database.Business{
		BusinessId:      "inventory-slug",
		Name:            "Inventory Slug",
		OwnerAddress:    "0xowner",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
		DefaultLanguage: "en",
		IsActive:        true,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	require.NoError(t, database.GetDB().Create(business).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: business.BusinessId}}
	c.Request = httptest.NewRequest(http.MethodGet, "/businesses/inventory-slug/inventory/settings", nil)
	c.Set("address", "0xowner")

	GetInventorySettings(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"business_id":`+strconv.Itoa(int(business.ID)))
}

func TestCreateInventoryAdjustment_TargetQuantityRecordsAbsoluteCount(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		body           string
		wantStatus     int
		wantBody       string
		wantDelta      float64
		wantFinalStock float64
	}{
		{
			// A physical count sets the absolute on-hand value; the recorded
			// movement delta is (target - live), here 12 - 5 = +7.
			name:           "absolute count above current",
			body:           `{"inventory_item_id":1,"movement_type":"correction","target_quantity":12}`,
			wantStatus:     http.StatusCreated,
			wantDelta:      7,
			wantFinalStock: 12,
		},
		{
			// target_quantity:0 is a VALID count (shelf is empty) — unlike
			// quantity_change:0, which is rejected.
			name:           "absolute count to zero",
			body:           `{"inventory_item_id":1,"movement_type":"correction","target_quantity":0}`,
			wantStatus:     http.StatusCreated,
			wantDelta:      -5,
			wantFinalStock: 0,
		},
		{
			// When both are present, the absolute target wins and quantity_change
			// is ignored (target 9 -> final 9, not 5+3).
			name:           "target wins over quantity_change",
			body:           `{"inventory_item_id":1,"movement_type":"correction","target_quantity":9,"quantity_change":3}`,
			wantStatus:     http.StatusCreated,
			wantDelta:      4,
			wantFinalStock: 9,
		},
		{
			name:       "negative target rejected",
			body:       `{"inventory_item_id":1,"movement_type":"correction","target_quantity":-3}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   "negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupInventoryHandlerTestDB(t)
			business := createInventoryHandlerTestBusiness(t)
			item := &database.InventoryItem{
				BusinessID:       business.ID,
				Name:             "Tomatoes",
				Unit:             "kg",
				CurrentQuantity:  5,
				ReorderThreshold: 1,
				CostPerUnit:      1,
				IsActive:         true,
			}
			require.NoError(t, database.GetDB().Create(item).Error)

			body := strings.ReplaceAll(tt.body, `"inventory_item_id":1`, `"inventory_item_id":`+strconv.Itoa(int(item.ID)))
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(int(business.ID))}}
			c.Request = httptest.NewRequest(http.MethodPost, "/inventory/adjustments", bytes.NewBufferString(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("address", "0xowner")

			CreateInventoryAdjustment(c)

			require.Equal(t, tt.wantStatus, w.Code)
			if tt.wantBody != "" {
				assert.Contains(t, w.Body.String(), tt.wantBody)
			}
			if tt.wantStatus == http.StatusCreated {
				var refreshed database.InventoryItem
				require.NoError(t, database.GetDB().First(&refreshed, item.ID).Error)
				assert.Equal(t, tt.wantFinalStock, refreshed.CurrentQuantity)

				var movement database.InventoryMovement
				require.NoError(t, database.GetDB().Where("inventory_item_id = ?", item.ID).Order("id DESC").First(&movement).Error)
				assert.Equal(t, tt.wantDelta, movement.QuantityDelta)
			}
		})
	}
}

func TestCreateInventoryAdjustment_QuantityChangePresenceAndZeroValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		body           string
		wantStatus     int
		wantBody       string
		wantQuantity   float64
		wantFinalStock float64
	}{
		{
			name:       "omitted quantity",
			body:       `{"inventory_item_id":1,"movement_type":"manual_adjustment"}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   "quantity_change is required",
		},
		{
			name:       "zero quantity",
			body:       `{"inventory_item_id":1,"movement_type":"manual_adjustment","quantity_change":0}`,
			wantStatus: http.StatusBadRequest,
			wantBody:   "quantity change must be non-zero",
		},
		{
			name:           "positive quantity",
			body:           `{"inventory_item_id":1,"movement_type":"manual_adjustment","quantity_change":3}`,
			wantStatus:     http.StatusCreated,
			wantQuantity:   3,
			wantFinalStock: 8,
		},
		{
			name:           "negative quantity",
			body:           `{"inventory_item_id":1,"movement_type":"manual_adjustment","quantity_change":-2}`,
			wantStatus:     http.StatusCreated,
			wantQuantity:   -2,
			wantFinalStock: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupInventoryHandlerTestDB(t)
			business := createInventoryHandlerTestBusiness(t)
			item := &database.InventoryItem{
				BusinessID:       business.ID,
				Name:             "Tomatoes",
				Unit:             "kg",
				CurrentQuantity:  5,
				ReorderThreshold: 1,
				CostPerUnit:      1,
				IsActive:         true,
			}
			require.NoError(t, database.GetDB().Create(item).Error)

			body := strings.ReplaceAll(tt.body, `"inventory_item_id":1`, `"inventory_item_id":`+strconv.Itoa(int(item.ID)))
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(int(business.ID))}}
			c.Request = httptest.NewRequest(http.MethodPost, "/inventory/adjustments", bytes.NewBufferString(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("address", "0xowner")

			CreateInventoryAdjustment(c)

			require.Equal(t, tt.wantStatus, w.Code)
			if tt.wantBody != "" {
				assert.Contains(t, w.Body.String(), tt.wantBody)
			}
			if tt.wantStatus == http.StatusCreated {
				var refreshed database.InventoryItem
				require.NoError(t, database.GetDB().First(&refreshed, item.ID).Error)
				assert.Equal(t, tt.wantFinalStock, refreshed.CurrentQuantity)

				var movement database.InventoryMovement
				require.NoError(t, database.GetDB().Where("inventory_item_id = ?", item.ID).First(&movement).Error)
				assert.Equal(t, tt.wantQuantity, movement.QuantityDelta)
			}
		})
	}
}
