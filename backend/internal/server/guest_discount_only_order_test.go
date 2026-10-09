package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedGuestSteakMenuAndTable(t *testing.T, tableCode string) *database.Table {
	t.Helper()
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

	business := createOwnedBusiness(t, "0xDiscountOnlyOwner", "Discount Only Guest Biz")
	enableGuestOrderingForBusiness(t, business.ID)
	table := &database.Table{
		BusinessID: business.ID,
		Name:       "D1",
		TableCode:  tableCode,
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)

	menuPayload, err := json.Marshal([]database.MenuCategory{{
		ID:   "mains",
		Name: "Mains",
		Items: []database.MenuItem{
			{ID: "demo-steak", Name: "Steak Plate", Price: 42, IsAvailable: true},
		},
	}})
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: business.ID,
		Categories: string(menuPayload),
		IsActive:   true,
		Version:    1,
	}).Error)
	return table
}

func discountOnlyQuoteBody(t *testing.T) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"items": []map[string]any{{
			"menu_item_id":   "demo-steak",
			"menu_item_name": "Steak Plate",
			"quantity":       1,
			"item_type":      "discount",
			"price":          -1000,
		}},
	})
	require.NoError(t, err)
	return body
}

func TestQuoteGuestOrder_RejectsDiscountOnlyLines(t *testing.T) {
	table := seedGuestSteakMenuAndTable(t, "guest-discount-quote")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/guest/table/%s/order/quote", table.TableCode),
		bytes.NewReader(discountOnlyQuoteBody(t)),
	)
	c.Request.Header.Set("Content-Type", "application/json")

	QuoteGuestOrder(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "at least one valid item")
	assert.NotContains(t, w.Body.String(), `"total":0`)
	assert.NotContains(t, w.Body.String(), `"total": 0`)
}

func TestCreateGuestOrder_RejectsDiscountOnlyLines(t *testing.T) {
	table := seedGuestSteakMenuAndTable(t, "guest-discount-checkout")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/guest/table/%s/order", table.TableCode),
		bytes.NewReader(discountOnlyQuoteBody(t)),
	)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("X-Request-Id", "discount-only-checkout")

	CreateGuestOrder(c)

	require.Equal(t, http.StatusBadRequest, w.Code)

	var orders, bills int64
	require.NoError(t, database.GetDB().Model(&database.Order{}).Count(&orders).Error)
	require.NoError(t, database.GetDB().Model(&database.Bill{}).Count(&bills).Error)
	assert.Zero(t, orders, "discount-only checkout must not create a kitchen order")
	assert.Zero(t, bills, "discount-only checkout must not open a $0 bill")
}
