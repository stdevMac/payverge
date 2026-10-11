package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/handlers"
)

func setupOperatorOrderIdemDB(t *testing.T) (uint, uint) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{}, &database.Menu{}, &database.Offer{}, &database.Bundle{},
		&database.Table{}, &database.Bill{}, &database.Order{},
		&database.Plugin{}, &database.BusinessPlugin{},
		&database.OperationalAlert{}, &database.OperationalAlertEvent{}, &database.BusinessAlertSettings{},
	))

	business := &database.Business{
		BusinessId:     fmt.Sprintf("idem-%s", t.Name()),
		Name:           "Idem Biz",
		OwnerAddress:   "0x0000000000000000000000000000000000000001",
		SettlementAddr: "0x0000000000000000000000000000000000000002",
		TippingAddr:    "0x0000000000000000000000000000000000000003",
	}
	require.NoError(t, database.GetDB().Create(business).Error)

	categories, err := json.Marshal([]database.MenuCategory{{
		ID: "cat-1", Name: "Mains",
		Items: []database.MenuItem{{ID: "item-1", Name: "Burger", Price: 10, IsAvailable: true}},
	}})
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: business.ID, Categories: string(categories), IsActive: true,
	}).Error)

	bill := &database.Bill{
		BusinessID: business.ID, BillNumber: "B-IDEM-1", Status: database.BillStatusOpen,
		Items: "[]", SettlementAddr: "0xS", TippingAddr: "0xT",
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	return business.ID, bill.ID
}

func TestCreateOrder_DeduplicatesOnRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	bizID, billID := setupOperatorOrderIdemDB(t)

	h := handlers.NewOrderHandler(nil)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("staff_email", "staff@example.test"); c.Next() })
	r.POST("/api/v1/businesses/:id/orders", h.CreateOrder)

	body := fmt.Sprintf(`{"bill_id": %d, "items":[{"menu_item_id":"item-1","menu_item_name":"Burger","quantity":1,"price":10}]}`, billID)
	url := fmt.Sprintf("/api/v1/businesses/%d/orders", bizID)

	post := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Request-Id", "staff-dedupe-123")
		r.ServeHTTP(w, req)
		return w
	}

	first := post()
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())

	// Double-click: same X-Request-Id replays the existing order.
	second := post()
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	assert.Contains(t, second.Body.String(), `"duplicate":true`)

	var count int64
	require.NoError(t, database.GetDB().Model(&database.Order{}).Where("bill_id = ?", billID).Count(&count).Error)
	assert.Equal(t, int64(1), count)
}
