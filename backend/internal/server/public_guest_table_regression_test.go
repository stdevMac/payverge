package server

import (
	"context"
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

func setupPublicGuestTableHandlerTestDB(t *testing.T) {
	t.Helper()

	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Table{},
		&database.Bill{},
		&database.Payment{},
		&database.Menu{},
		&database.Offer{},
		&database.Bundle{},
		&database.BusinessLanguage{},
		&database.SupportedLanguage{},
		&database.BusinessOperatingHours{},
	))
	createSQLitePublicGuestBillItemsTable(t)
}

func setupPublicGuestTableHandlerTestDBWithLogger(t *testing.T, gormLogger logger.Interface) {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: gormLogger})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Table{},
		&database.Bill{},
		&database.Order{},
		&database.Payment{},
		&database.Menu{},
		&database.Offer{},
		&database.Bundle{},
		&database.Plugin{},
		&database.BusinessPlugin{},
		&database.PluginNotificationDelivery{},
		&database.PluginNotificationDeliveryAttempt{},
		&database.BusinessLanguage{},
		&database.SupportedLanguage{},
	))
	createSQLitePublicGuestBillItemsTable(t)
}

func createSQLitePublicGuestBillItemsTable(t testing.TB) {
	t.Helper()

	require.NoError(t, database.GetDB().Exec(`
CREATE TABLE IF NOT EXISTS bill_items (
	id text PRIMARY KEY,
	bill_id integer NOT NULL,
	menu_item_id text DEFAULT '',
	name text NOT NULL,
	price real NOT NULL,
	quantity integer NOT NULL,
	options text,
	item_type text DEFAULT 'menu_item',
	bundle_id integer,
	parent_bundle_id integer,
	source_offer_id integer,
	order_id INTEGER,
	subtotal real NOT NULL,
	created_at datetime
)`).Error)
}

type publicGuestSQLRecorder struct {
	logger.Interface
	statements []string
}

func (r *publicGuestSQLRecorder) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	r.statements = append(r.statements, sql)
}

func (r *publicGuestSQLRecorder) selectCount(table string) int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.TrimSpace(statement))
		if !strings.HasPrefix(normalized, "select") {
			continue
		}
		compact := strings.Join(strings.Fields(normalized), " ")
		if strings.Contains(normalized, "from `"+table+"`") ||
			strings.Contains(normalized, "from \""+table+"\"") ||
			strings.Contains(compact, "from "+table) {
			count++
		}
	}
	return count
}

func (r *publicGuestSQLRecorder) selectStarCount(table string) int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.TrimSpace(statement))
		if !strings.HasPrefix(normalized, "select *") {
			continue
		}
		if strings.Contains(normalized, "from `"+table+"`") || strings.Contains(normalized, "from \""+table+"\"") {
			count++
		}
	}
	return count
}

func createSensitiveGuestTableBusiness(t testing.TB, tableCode string) *database.Business {
	t.Helper()

	userID := uint(77)
	business := &database.Business{
		BusinessId:           fmt.Sprintf("guest-public-%s", tableCode),
		Name:                 "Guest Safe Business",
		OwnerAddress:         "0xOWNERSECRET",
		UserID:               &userID,
		Logo:                 "https://example.com/logo.png",
		Email:                "owner@example.com",
		SettlementAddr:       "0x1111111111111111111111111111111111111111",
		TippingAddr:          "0x2222222222222222222222222222222222222222",
		TaxRate:              8.5,
		ServiceFeeRate:       10.0,
		TaxInclusive:         true,
		ServiceInclusive:     false,
		Description:          "Public description",
		Phone:                "+54 11 5555 1234",
		Website:              "https://example.com",
		SocialMedia:          `{"instagram":"guest-safe"}`,
		DefaultCurrency:      "USD",
		DisplayCurrency:      "EUR",
		DefaultLanguage:      "es",
		GoogleReviewsEnabled: true,
		ShowReviews:          true,
		GooglePlaceID:        "place-123",
		GoogleBusinessName:   "Guest Safe Business",
		GoogleReviewLink:     "https://example.com/review",
		GoogleBusinessURL:    "https://example.com/maps",
		Timezone:             "UTC",
		KitchenEnabled:       true,
		OrdersEnabled:        true,
		CRMEnabled:           true,
		CounterEnabled:       true,
		AiSettings: database.BusinessAiSettings{
			AiEnabled:             true,
			AiName:                "Alfred",
			AiPriority:            "balanced",
			SpecialInstructions:   "internal-only",
			BusinessPageAiEnabled: true,
		},
	}
	require.NoError(t, database.GetDB().Create(business).Error)
	return business
}

func createGuestPublicTable(t testing.TB, businessID uint, tableCode string) *database.Table {
	t.Helper()

	table := &database.Table{
		BusinessID: businessID,
		Name:       "Table 7",
		TableCode:  tableCode,
		Capacity:   4,
		IsActive:   true,
		QRCode:     fmt.Sprintf("https://payverge.io/t/%s", tableCode),
	}
	require.NoError(t, database.GetDB().Create(table).Error)
	return table
}

func requireMapField(t *testing.T, payload map[string]any, key string) map[string]any {
	t.Helper()

	value, ok := payload[key]
	require.True(t, ok, "expected %s to be present", key)
	asMap, ok := value.(map[string]any)
	require.True(t, ok, "expected %s to be an object", key)
	return asMap
}

func decodeJSONResponse(body []byte, target any) error {
	return json.Unmarshal(body, target)
}

func assertSensitiveBusinessFieldsHidden(t *testing.T, businessResp map[string]any) {
	t.Helper()

	assert.NotContains(t, businessResp, "owner_address")
	assert.NotContains(t, businessResp, "user_id")
	assert.NotContains(t, businessResp, "email")
	assert.NotContains(t, businessResp, "settlement_address")
	assert.NotContains(t, businessResp, "tipping_address")
	assert.NotContains(t, businessResp, "stripe_customer_id")
	assert.NotContains(t, businessResp, "stripe_subscription_id")
	assert.NotContains(t, businessResp, "registration_tx_hash")
	assert.NotContains(t, businessResp, "last_payment_amount")
	assert.NotContains(t, businessResp, "total_paid")

	aiSettings, ok := businessResp["ai_settings"].(map[string]any)
	require.True(t, ok, "expected ai_settings to be present")
	assert.Equal(t, "Alfred", aiSettings["ai_name"])
	assert.NotContains(t, aiSettings, "special_instructions")
}

func assertSensitiveBillFieldsHidden(t *testing.T, billResp map[string]any) {
	t.Helper()

	assert.NotContains(t, billResp, "business_id")
	assert.NotContains(t, billResp, "table_id")
	assert.NotContains(t, billResp, "notes")
	assert.NotContains(t, billResp, "items")
	assert.NotContains(t, billResp, "payments")
	assert.NotContains(t, billResp, "business")
	assert.NotContains(t, billResp, "table")
}

func TestGetTableByCodePublic_DoesNotExposeSensitiveModels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicGuestTableHandlerTestDB(t)

	business := createSensitiveGuestTableBusiness(t, "SAFE1234")
	table := createGuestPublicTable(t, business.ID, "SAFE1234")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/table/%s", table.TableCode), nil)

	GetTableByCodePublic(c)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, decodeJSONResponse(w.Body.Bytes(), &resp))

	tableResp := requireMapField(t, resp, "table")
	assert.Equal(t, table.Name, tableResp["name"])
	assert.NotContains(t, tableResp, "id")
	assert.NotContains(t, tableResp, "business_id")
	assert.NotContains(t, tableResp, "qr_code")

	businessResp := requireMapField(t, resp, "business")
	assert.Equal(t, business.Name, businessResp["name"])
	assert.Equal(t, business.DefaultCurrency, businessResp["default_currency"])
	assertSensitiveBusinessFieldsHidden(t, businessResp)

	menuResp := requireMapField(t, resp, "menu")
	assert.NotContains(t, menuResp, "id")
	assert.NotContains(t, menuResp, "business_id")
}

func TestGetTableByCodePublicUsesSharedMenuCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupPublicGuestTableHandlerTestDBWithLogger(t, recorder)
	services.ResetPricingCache()

	business := createSensitiveGuestTableBusiness(t, "CACHE123")
	table := createGuestPublicTable(t, business.ID, "CACHE123")
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: business.ID,
		Categories: `[]`,
		IsActive:   true,
	}).Error)

	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
		c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/table/%s", table.TableCode), nil)

		GetTableByCodePublic(c)

		require.Equal(t, http.StatusOK, w.Code)
	}

	require.LessOrEqual(t, recorder.selectCount("menus"), 1, "menu data should be served from the shared pricing cache after the first request")
	require.LessOrEqual(t, recorder.selectCount("tables"), 1, "table context should be served from the guest table cache after the first request")
	require.LessOrEqual(t, recorder.selectCount("businesses"), 2, "business context should not be reloaded after table/menu caches are warm")
}

func TestGetBusinessByTableCode_DoesNotExposeSensitiveBusinessFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicGuestTableHandlerTestDB(t)

	business := createSensitiveGuestTableBusiness(t, "SAFE5678")
	table := createGuestPublicTable(t, business.ID, "SAFE5678")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/table/%s/business", table.TableCode), nil)

	GetBusinessByTableCode(c)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, decodeJSONResponse(w.Body.Bytes(), &resp))

	businessResp := requireMapField(t, resp, "business")
	assert.Equal(t, business.Name, businessResp["name"])
	assert.Equal(t, business.DisplayCurrency, businessResp["display_currency"])
	assertSensitiveBusinessFieldsHidden(t, businessResp)
}

func TestGetBusinessByTableCode_ExposesCRMEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicGuestTableHandlerTestDB(t)

	business := createSensitiveGuestTableBusiness(t, "CRMON1234")
	table := createGuestPublicTable(t, business.ID, "CRMON1234")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/table/%s/business", table.TableCode), nil)

	GetBusinessByTableCode(c)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, decodeJSONResponse(w.Body.Bytes(), &resp))

	businessResp := requireMapField(t, resp, "business")
	assert.Equal(t, true, businessResp["crm_enabled"])
}

func TestGetBusinessByTableCodeUsesSharedExtrasCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupPublicGuestTableHandlerTestDBWithLogger(t, recorder)
	services.ResetPricingCache()

	business := createSensitiveGuestTableBusiness(t, "BIZCACHE123")
	table := createGuestPublicTable(t, business.ID, "BIZCACHE123")
	require.NoError(t, database.GetDB().Create(&database.SupportedLanguage{
		Code:       "en",
		Name:       "English",
		NativeName: "English",
		IsActive:   true,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessLanguage{
		BusinessID:   business.ID,
		LanguageCode: "en",
		IsDefault:    true,
		DisplayOrder: 0,
	}).Error)

	recorder.statements = nil
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
		c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/table/%s/business", table.TableCode), nil)

		GetBusinessByTableCode(c)

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}

	require.LessOrEqual(t, recorder.selectCount("business_languages"), 1, "business language metadata should be cached for repeated guest business loads")
	require.LessOrEqual(t, recorder.selectCount("supported_languages"), 1, "supported language metadata should be cached for repeated guest business loads")
	require.LessOrEqual(t, recorder.selectCount("business_plugins"), 1, "Trustpilot plugin state should be cached for repeated guest business loads")
}

func BenchmarkGetBusinessByTableCodeSQLite(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	setupPublicGuestTableHandlerBenchmarkDB(b)
	services.ResetPricingCache()

	business := createSensitiveGuestTableBusiness(b, "BIZBENCH123")
	table := createGuestPublicTable(b, business.ID, "BIZBENCH123")
	require.NoError(b, database.GetDB().Create(&database.SupportedLanguage{
		Code:       "en",
		Name:       "English",
		NativeName: "English",
		IsActive:   true,
	}).Error)
	require.NoError(b, database.GetDB().Create(&database.BusinessLanguage{
		BusinessID:   business.ID,
		LanguageCode: "en",
		IsDefault:    true,
	}).Error)

	r := gin.New()
	r.GET("/guest/table/:code/business", GetBusinessByTableCode)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/table/%s/business", table.TableCode), nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("got %d, body=%s", w.Code, w.Body.String())
		}
	}
}

func BenchmarkGetTableByCodePublicSQLite(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	setupPublicGuestTableHandlerBenchmarkDB(b)
	services.ResetPricingCache()

	business := createSensitiveGuestTableBusiness(b, "SCANBENCH1")
	table := createGuestPublicTable(b, business.ID, "SCANBENCH1")
	require.NoError(b, database.GetDB().Create(&database.Menu{
		BusinessID: business.ID,
		Categories: `[]`,
		IsActive:   true,
	}).Error)

	r := gin.New()
	r.GET("/guest/table/:code", GetTableByCodePublic)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/table/%s", table.TableCode), nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("got %d, body=%s", w.Code, w.Body.String())
		}
	}
}

func setupPublicGuestTableHandlerBenchmarkDB(b *testing.B) {
	b.Helper()

	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", b.Name(), time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(b, err)

	sqlDB, err := gormDB.DB()
	require.NoError(b, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	require.NoError(b, gormDB.AutoMigrate(
		&database.Business{},
		&database.Table{},
		&database.Bill{},
		&database.Payment{},
		&database.Menu{},
		&database.Offer{},
		&database.Bundle{},
		&database.Plugin{},
		&database.BusinessPlugin{},
		&database.BusinessLanguage{},
		&database.SupportedLanguage{},
	))
	createSQLitePublicGuestBillItemsTable(b)
}

func TestGetMenuByTableCode_DoesNotExposeRawMenuIdentifiers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicGuestTableHandlerTestDB(t)

	business := createSensitiveGuestTableBusiness(t, "MENU1234")
	table := createGuestPublicTable(t, business.ID, "MENU1234")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/table/%s/menu", table.TableCode), nil)

	GetMenuByTableCode(c)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, decodeJSONResponse(w.Body.Bytes(), &resp))

	menuResp := requireMapField(t, resp, "menu")
	assert.NotContains(t, menuResp, "id")
	assert.NotContains(t, menuResp, "business_id")
	assert.NotContains(t, menuResp, "is_active")
}

func TestGetMenuByTableCodeUsesSharedMenuCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupPublicGuestTableHandlerTestDBWithLogger(t, recorder)
	services.ResetPricingCache()

	business := createSensitiveGuestTableBusiness(t, "MENUCACHE123")
	table := createGuestPublicTable(t, business.ID, "MENUCACHE123")
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: business.ID,
		Categories: `[]`,
		IsActive:   true,
	}).Error)

	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
		c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/table/%s/menu", table.TableCode), nil)

		GetMenuByTableCode(c)

		require.Equal(t, http.StatusOK, w.Code)
	}

	require.LessOrEqual(t, recorder.selectCount("menus"), 1, "menu-by-table should use the shared pricing cache after the first request")
}

func TestCreateGuestOrderUsesSharedMenuCache(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupPublicGuestTableHandlerTestDBWithLogger(t, recorder)
	services.ResetPricingCache()
	services.ResetTelegramNotificationEligibilityCache()

	business := createSensitiveGuestTableBusiness(t, "ORDCACHE123")
	table := createGuestPublicTable(t, business.ID, "ORDCACHE123")
	categories := []database.MenuCategory{
		{
			ID:   "mains",
			Name: "Mains",
			Items: []database.MenuItem{
				{ID: "burger", Name: "Burger", Price: 10, IsAvailable: true},
			},
		},
	}
	categoriesJSON, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: business.ID,
		Categories: string(categoriesJSON),
		IsActive:   true,
	}).Error)

	bills := []database.Bill{
		{
			BusinessID:     business.ID,
			TableID:        table.ID,
			BillNumber:     fmt.Sprintf("B-order-cache-%d-1", time.Now().UnixNano()),
			Status:         database.BillStatusOpen,
			SettlementAddr: business.SettlementAddr,
			TippingAddr:    business.TippingAddr,
		},
		{
			BusinessID:     business.ID,
			TableID:        table.ID,
			BillNumber:     fmt.Sprintf("B-order-cache-%d-2", time.Now().UnixNano()),
			Status:         database.BillStatusOpen,
			SettlementAddr: business.SettlementAddr,
			TippingAddr:    business.TippingAddr,
		},
	}
	require.NoError(t, database.GetDB().Create(&bills).Error)

	recorder.statements = nil
	for i := range bills {
		body := fmt.Sprintf(`{
			"bill_id": %d,
			"items": [
				{"menu_item_name": "Burger", "menu_item_id": "burger", "quantity": 1, "price": 10}
			]
		}`, bills[i].ID)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
		c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/guest/table/%s/order", table.TableCode), strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Request.Header.Set("X-Request-Id", fmt.Sprintf("order-cache-%d", i))

		CreateGuestOrder(c)

		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	}

	require.LessOrEqual(t, recorder.selectCount("menus"), 1, "guest order pricing should reuse the shared menu snapshot after the first request")
	require.LessOrEqual(t, recorder.selectCount("offers"), 1, "guest order pricing should reuse the shared promotion snapshot after the first request")
	require.LessOrEqual(t, recorder.selectCount("bundles"), 1, "guest order pricing should reuse the shared promotion snapshot after the first request")
	require.LessOrEqual(t, recorder.selectCount("orders"), len(bills), "guest order creation should perform only the request-identity replay lookup, not reload an order it just inserted")
}

func TestCreateBillByTableCodeChecksActiveBillWithoutHydratingBill(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupPublicGuestTableHandlerTestDBWithLogger(t, recorder)
	services.ResetPricingCache()

	business := createSensitiveGuestTableBusiness(t, "BILLEXISTS")
	table := createGuestPublicTable(t, business.ID, "BILLEXISTS")

	recorder.statements = nil
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodPost, fmt.Sprintf("/guest/table/%s/bill", table.TableCode), nil)

	CreateBillByTableCode(c)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.Zero(t, recorder.selectCount("businesses"), "table-code context should load table and business with one joined lookup on cache miss")
	require.Zero(t, recorder.selectStarCount("bills"), "new-bill path only needs an active-bill existence probe, not a full bill hydrate")
}

func TestGetTableByCodePublic_ReturnsNotFoundForInactiveBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicGuestTableHandlerTestDB(t)

	business := createSensitiveGuestTableBusiness(t, "INACTIVE1")
	table := createGuestPublicTable(t, business.ID, "INACTIVE1")
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		Update("is_active", false).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/table/%s", table.TableCode), nil)

	GetTableByCodePublic(c)

	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetBillByNumberPublic_DoesNotExposeSensitiveBillFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicGuestTableHandlerTestDB(t)

	business := createSensitiveGuestTableBusiness(t, "BILLSAFE1")
	table := createGuestPublicTable(t, business.ID, "BILLSAFE1")
	bill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     fmt.Sprintf("B-safe-%d", time.Now().UnixNano()),
		Notes:          "Internal kitchen note",
		Items:          `[{"name":"Secret"}]`,
		Subtotal:       30,
		TaxAmount:      3,
		TotalAmount:    33,
		PaidAmount:     5,
		Status:         database.BillStatusOpen,
		SettlementAddr: business.SettlementAddr,
		TippingAddr:    business.TippingAddr,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	require.NotEmpty(t, bill.PublicToken)
	c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/bill/%s", bill.PublicToken), nil)

	GetBillByNumberPublic(c)

	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, decodeJSONResponse(w.Body.Bytes(), &resp))

	billResp := requireMapField(t, resp, "bill")
	assert.EqualValues(t, float64(bill.ID), billResp["id"])
	assert.Equal(t, bill.BillNumber, billResp["bill_number"])
	assert.Equal(t, bill.SettlementAddr, billResp["settlement_address"])
	assert.Equal(t, bill.TippingAddr, billResp["tipping_address"])
	assertSensitiveBillFieldsHidden(t, billResp)
}

func TestGetBillByNumberPublicUsesProjectedBillLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	setupPublicGuestTableHandlerTestDBWithLogger(t, recorder)

	business := createSensitiveGuestTableBusiness(t, "BILLPROJ1")
	table := createGuestPublicTable(t, business.ID, "BILLPROJ1")
	bill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     fmt.Sprintf("B-projected-%d", time.Now().UnixNano()),
		Items:          `[{"name":"Projected","quantity":1,"price":12.5,"subtotal":12.5}]`,
		Subtotal:       1250,
		TotalAmount:    1250,
		Status:         database.BillStatusOpen,
		SettlementAddr: business.SettlementAddr,
		TippingAddr:    business.TippingAddr,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	for i := 0; i < 250; i++ {
		require.NoError(t, database.GetDB().Create(&database.Payment{
			BillID:    bill.ID,
			PayerAddr: fmt.Sprintf("0xpayer%03d", i),
			Amount:    1250,
			TxHash:    fmt.Sprintf("public_bill_payment_%03d", i),
			Status:    database.PaymentStatusConfirmed,
		}).Error)
	}

	recorder.statements = nil
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	require.NotEmpty(t, bill.PublicToken)
	c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/bill/%s", bill.PublicToken), nil)

	GetBillByNumberPublic(c)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp map[string]any
	require.NoError(t, decodeJSONResponse(w.Body.Bytes(), &resp))
	items, ok := resp["items"].([]any)
	require.True(t, ok)
	require.Len(t, items, 1)
	assert.Zero(t, recorder.selectStarCount("bills"), "public guest bill lookup should project bill response fields instead of SELECT *")
	assert.Zero(t, recorder.selectCount("businesses"), "public guest bill lookup should not preload full business rows")
	assert.Zero(t, recorder.selectCount("tables"), "public guest bill lookup should not preload bill table")
	assert.Zero(t, recorder.selectCount("payments"), "public guest bill lookup should not preload bill payments")
}

func BenchmarkGetBillByNumberPublicSQLite(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)
	setupPublicGuestTableHandlerBenchmarkDB(b)

	business := createSensitiveGuestTableBusiness(b, "BILLBENCH1")
	table := createGuestPublicTable(b, business.ID, "BILLBENCH1")
	billItems := "[" + strings.TrimSuffix(strings.Repeat(`{"name":"Bench","quantity":1,"price":12.5,"subtotal":12.5},`, 256), ",") + "]"
	bill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     fmt.Sprintf("B-bench-%d", time.Now().UnixNano()),
		Items:          billItems,
		Subtotal:       1250,
		TotalAmount:    1250,
		Status:         database.BillStatusOpen,
		SettlementAddr: business.SettlementAddr,
		TippingAddr:    business.TippingAddr,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(b, database.GetDB().Create(bill).Error)
	for i := 0; i < 250; i++ {
		require.NoError(b, database.GetDB().Create(&database.Payment{
			BillID:    bill.ID,
			PayerAddr: fmt.Sprintf("0xpayer%03d", i),
			Amount:    1250,
			TxHash:    fmt.Sprintf("public_bill_bench_payment_%03d", i),
			Status:    database.PaymentStatusConfirmed,
		}).Error)
	}

	r := gin.New()
	r.GET("/guest/bill/:bill_token", GetBillByNumberPublic)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/bill/%s", bill.PublicToken), nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("got %d, body=%s", w.Code, w.Body.String())
		}
	}
}

func TestGetTableByCodePublic_ExposesAIAvailable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// ai toggle off => ai_available must be false
	t.Run("ai toggle off hides AI", func(t *testing.T) {
		setupPublicGuestTableHandlerTestDB(t)
		business := createSensitiveGuestTableBusiness(t, "AIOFF001")
		business.AiSettings.AiEnabled = false
		require.NoError(t, database.GetDB().Save(business).Error)
		table := createGuestPublicTable(t, business.ID, "AIOFF001")

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
		c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/table/%s", table.TableCode), nil)
		GetTableByCodePublic(c)

		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]any
		require.NoError(t, decodeJSONResponse(w.Body.Bytes(), &resp))
		businessResp := requireMapField(t, resp, "business")
		assert.Equal(t, false, businessResp["ai_available"])
	})

	// active + toggle on => ai_available true
	t.Run("active venue with AI toggle shows AI", func(t *testing.T) {
		setupPublicGuestTableHandlerTestDB(t)
		business := createSensitiveGuestTableBusiness(t, "AION0001")
		require.NoError(t, database.GetDB().Save(business).Error)
		table := createGuestPublicTable(t, business.ID, "AION0001")

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
		c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/table/%s", table.TableCode), nil)
		GetTableByCodePublic(c)

		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string]any
		require.NoError(t, decodeJSONResponse(w.Body.Bytes(), &resp))
		businessResp := requireMapField(t, resp, "business")
		assert.Equal(t, true, businessResp["ai_available"])
	})
}

func TestGetTableByCodePublic_ExposesOperatingHours(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupPublicGuestTableHandlerTestDB(t)

	business := createSensitiveGuestTableBusiness(t, "HRS00001")
	require.NoError(t, database.GetDB().Create(&database.BusinessOperatingHours{
		BusinessID: business.ID,
		DayOfWeek:  1,
		OpenTime:   "11:00",
		CloseTime:  "22:00",
		IsClosed:   false,
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.BusinessOperatingHours{
		BusinessID: business.ID,
		DayOfWeek:  2,
		OpenTime:   "11:00",
		CloseTime:  "22:00",
		IsClosed:   false,
	}).Error)
	table := createGuestPublicTable(t, business.ID, "HRS00001")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "code", Value: table.TableCode}}
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/guest/table/%s", table.TableCode), nil)
	GetTableByCodePublic(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, decodeJSONResponse(w.Body.Bytes(), &resp))
	businessResp := requireMapField(t, resp, "business")
	hoursRaw, ok := businessResp["hours"].([]any)
	require.True(t, ok, "business.hours should be an array")
	require.Len(t, hoursRaw, 2)
	first := hoursRaw[0].(map[string]any)
	assert.Contains(t, first, "day_of_week")
	assert.Contains(t, first, "open_time")
	assert.Contains(t, first, "close_time")
	assert.Contains(t, first, "is_closed")
	// Sensitive embedded Business relation must not leak via hours rows.
	assert.NotContains(t, first, "business")
	assert.NotContains(t, first, "settlement_addr")
}
