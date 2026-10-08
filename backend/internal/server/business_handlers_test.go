package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func createBusinessHandlerTestBusiness(t *testing.T, ownerAddress, businessID string) *database.Business {
	t.Helper()

	business := &database.Business{
		BusinessId:     businessID,
		Name:           "Original Business",
		OwnerAddress:   ownerAddress,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		Address: database.BusinessAddress{
			Street:     "123 Main St",
			City:       "Buenos Aires",
			State:      "CABA",
			PostalCode: "1000",
			Country:    "AR",
		},
		WelcomeMessage:            "Welcome to our place",
		AboutStory:                "A long standing story",
		CustomURL:                 "original-url",
		DefaultQRLogoURL:          "https://cdn.example.com/original-logo.png",
		DefaultQRForegroundColor:  "#111111",
		DefaultQRBackgroundColor:  "#eeeeee",
		DefaultQRLogoSize:         18,
		DefaultQRShowBusinessName: true,
		DefaultQRShowTableName:    false,
		DefaultQRTextFont:         "Inter",
		IsActive:                  true,
	}
	require.NoError(t, database.GetDB().Create(business).Error)
	return business
}

func TestParseBillListDateBoundUsesBusinessLocationForDateOnlyBounds(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	from, err := parseBillListDateBound("2026-05-04", false, location)
	require.NoError(t, err)
	to, err := parseBillListDateBound("2026-05-04", true, location)
	require.NoError(t, err)

	assert.Equal(t, "2026-05-04T00:00:00-04:00", from.Format(time.RFC3339))
	assert.Equal(t, "2026-05-05T00:00:00-04:00", to.Format(time.RFC3339))
}

func TestUpdateBusiness_PreservesExistingFieldsOnPartialPatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "biz-preserve")

	body, err := json.Marshal(map[string]string{
		"name": "Updated Business",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", "0xOwnerA")

	UpdateBusiness(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var persisted database.Business
	require.NoError(t, database.GetDB().First(&persisted, business.ID).Error)
	assert.Equal(t, "Updated Business", persisted.Name)
	assert.Equal(t, business.Address, persisted.Address)
	assert.Equal(t, "Welcome to our place", persisted.WelcomeMessage)
	assert.Equal(t, "A long standing story", persisted.AboutStory)
}

// TestUpdateBusiness_RejectsUnknownTimezone closes the write side of the
// timezone contract. An unrecognised zone used to be stored verbatim and then
// silently resolved to UTC on every read, so an operator in Buenos Aires who
// mistyped their zone saw UTC day boundaries in analytics, payroll, and
// reports with nothing anywhere saying so. It is also the only path by which
// unbounded free text reaches the process-lifetime location cache.
func TestUpdateBusiness_RejectsUnknownTimezone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "biz-bad-tz")
	original := business.Timezone

	body, err := json.Marshal(map[string]interface{}{"timezone": "Not/ARealZone"})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", "0xOwnerA")

	UpdateBusiness(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var persisted database.Business
	require.NoError(t, database.GetDB().First(&persisted, business.ID).Error)
	assert.Equal(t, original, persisted.Timezone, "a rejected update must not persist")
}

func TestUpdateBusiness_AcceptsKnownTimezone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "biz-good-tz")

	body, err := json.Marshal(map[string]interface{}{
		"timezone": "America/Argentina/Buenos_Aires",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", "0xOwnerA")

	UpdateBusiness(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var persisted database.Business
	require.NoError(t, database.GetDB().First(&persisted, business.ID).Error)
	assert.Equal(t, "America/Argentina/Buenos_Aires", persisted.Timezone)
}

func TestUpdateBusiness_UpdatesDefaultQRSettingsAndAllowsClearingCustomURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "biz-default-qr")

	body, err := json.Marshal(map[string]interface{}{
		"custom_url":                    "",
		"default_qr_logo_url":           "https://cdn.example.com/new-logo.png",
		"default_qr_foreground_color":   "#222222",
		"default_qr_background_color":   "#ffffff",
		"default_qr_logo_size":          24,
		"default_qr_show_business_name": false,
		"default_qr_show_table_name":    true,
		"default_qr_text_font":          "Verdana",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", "0xOwnerA")

	UpdateBusiness(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var persisted database.Business
	require.NoError(t, database.GetDB().First(&persisted, business.ID).Error)
	assert.Equal(t, "", persisted.CustomURL)
	assert.Equal(t, "https://cdn.example.com/new-logo.png", persisted.DefaultQRLogoURL)
	assert.Equal(t, "#222222", persisted.DefaultQRForegroundColor)
	assert.Equal(t, "#ffffff", persisted.DefaultQRBackgroundColor)
	assert.Equal(t, 24, persisted.DefaultQRLogoSize)
	assert.False(t, persisted.DefaultQRShowBusinessName)
	assert.True(t, persisted.DefaultQRShowTableName)
	assert.Equal(t, "Verdana", persisted.DefaultQRTextFont)
}

func TestUpdateBusiness_RejectsAIWaiterEnableForSuspendedBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "biz-suspended-ai")
	business.AiSettings.AiEnabled = false
	require.NoError(t, database.GetDB().Save(business).Error)
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		UpdateColumn("is_active", false).Error)

	body, err := json.Marshal(map[string]bool{
		"ai_enabled": true,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", "0xOwnerA")

	UpdateBusiness(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "business_suspended")

	var persisted database.Business
	require.NoError(t, database.GetDB().First(&persisted, business.ID).Error)
	assert.False(t, persisted.AiSettings.AiEnabled)
}

func TestUpdateBusiness_WalletChangeDoesNotPanicWhenEmailServerUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	migrateWalletRotationTables(t, setupStaffHandlerTestDB(t))

	originalEmailServer := emails.EmailServerInstance
	emails.EmailServerInstance = nil
	defer func() { emails.EmailServerInstance = originalEmailServer }()

	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "biz-wallet-email-guard")
	business.Email = "owner@example.com"
	require.NoError(t, database.GetDB().Save(business).Error)

	body, err := json.Marshal(map[string]string{
		"settlement_address": "0x3333333333333333333333333333333333333333",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", "0xOwnerA")

	UpdateBusiness(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var persisted database.Business
	require.NoError(t, database.GetDB().First(&persisted, business.ID).Error)
	assert.Equal(t, "0x3333333333333333333333333333333333333333", persisted.SettlementAddr)
}

func TestCreateBusiness_GeneratesBusinessIDWhenMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	body, err := json.Marshal(map[string]interface{}{
		"name":               "Fallback Business ID",
		"settlement_address": "0x1111111111111111111111111111111111111111",
		"tipping_address":    "0x2222222222222222222222222222222222222222",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", "0xOwnerA")

	CreateBusiness(c)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp database.Business
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.BusinessId)

	var persisted database.Business
	require.NoError(t, database.GetDB().First(&persisted, resp.ID).Error)
	assert.NotEmpty(t, persisted.BusinessId)
}

// setupBillListTestTables migrates the tables the bill-list handlers touch and
// manually creates bill_items. The bill-list query projects
// `(SELECT COUNT(*) FROM bill_items ...) AS item_count` (a15b0d1b), but
// bill_items is a SQL-only table deliberately excluded from GORM autoMigrate
// (AutoMigrate would destructively rewrite its jsonb columns to text on
// Postgres), and RunEnsureBillItemsSchema's missing-table branch is
// intentionally a no-op on non-Postgres — sqlite tests own their own table
// creation. DDL mirrors internal/database/bill_list_itemcount_test.go.
func setupBillListTestTables(t *testing.T, gormDB *gorm.DB) {
	t.Helper()
	require.NoError(t, gormDB.AutoMigrate(&database.Table{}, &database.Bill{}))
	gormDB.Exec("DROP TABLE IF EXISTS bill_items")
	require.NoError(t, gormDB.Exec(`
		CREATE TABLE bill_items (
			id TEXT PRIMARY KEY,
			bill_id INTEGER NOT NULL,
			menu_item_id TEXT DEFAULT '',
			name TEXT NOT NULL,
			price REAL NOT NULL,
			quantity INTEGER NOT NULL,
			options TEXT,
			item_type TEXT DEFAULT 'menu_item',
			bundle_id INTEGER,
			parent_bundle_id INTEGER,
			source_offer_id INTEGER,
			order_id INTEGER,
			subtotal REAL NOT NULL,
			created_at DATETIME
		)`).Error)
	require.NoError(t, gormDB.Exec("CREATE INDEX idx_bill_items_bill_id ON bill_items(bill_id)").Error)
}

func TestGetBusinessBills_DefaultsToPaginatedResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	setupBillListTestTables(t, gormDB)

	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "biz-bill-pages")
	for i := 0; i < 25; i++ {
		require.NoError(t, database.GetDB().Create(&database.Bill{
			BusinessID: business.ID,
			BillNumber: fmt.Sprintf("B-page-handler-%02d", i),
			Status:     database.BillStatusOpen,
		}).Error)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set("address", "0xOwnerA")

	GetBusinessBills(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Bills      []database.Bill `json:"bills"`
		Total      int64           `json:"total"`
		Page       int             `json:"page"`
		PageSize   int             `json:"page_size"`
		TotalPages int             `json:"total_pages"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Bills, 20)
	assert.Equal(t, int64(25), resp.Total)
	assert.Equal(t, 1, resp.Page)
	assert.Equal(t, 20, resp.PageSize)
	assert.Equal(t, 2, resp.TotalPages)
}

func TestGetBusinessBills_ResolvesBusinessSlug(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	setupBillListTestTables(t, gormDB)

	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "biz-bill-slug")
	require.NoError(t, database.GetDB().Create(&database.Bill{
		BusinessID:  business.ID,
		BillNumber:  "B-slug-handler-01",
		Status:      database.BillStatusOpen,
		TotalAmount: 4200,
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: business.BusinessId}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set("address", "0xOwnerA")

	GetBusinessBills(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Bills []database.Bill `json:"bills"`
		Total int64           `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Bills, 1)
	assert.Equal(t, "B-slug-handler-01", resp.Bills[0].BillNumber)
	assert.Equal(t, int64(1), resp.Total)
}

func TestGetBusinessBills_ReturnsNotFoundForUnknownBusinessIdentifier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	setupBillListTestTables(t, gormDB)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "missing-business"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set("address", "0xOwnerA")

	GetBusinessBills(c)

	require.Equal(t, http.StatusNotFound, w.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Business not found", resp["error"])
}

func TestGetBusinessBills_OmitsEmptyNestedBusinessAndTableObjects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	setupBillListTestTables(t, gormDB)

	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "biz-bill-clean-list")
	require.NoError(t, database.GetDB().Create(&database.Bill{
		BusinessID:  business.ID,
		BillNumber:  "B-clean-handler-01",
		Status:      database.BillStatusOpen,
		TotalAmount: 4200,
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set("address", "0xOwnerA")

	GetBusinessBills(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	bills, ok := resp["bills"].([]any)
	require.True(t, ok)
	require.Len(t, bills, 1)
	bill, ok := bills[0].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, bill, "business")
	assert.NotContains(t, bill, "table")
}

func TestGetOpenBusinessBills_ClampsRequestedPageSize(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	setupBillListTestTables(t, gormDB)

	business := createBusinessHandlerTestBusiness(t, "0xOwnerA", "biz-open-bill-pages")
	for i := 0; i < 130; i++ {
		status := database.BillStatusOpen
		if i%10 == 0 {
			status = database.BillStatusClosed
		}
		require.NoError(t, database.GetDB().Create(&database.Bill{
			BusinessID: business.ID,
			BillNumber: fmt.Sprintf("B-open-page-handler-%03d", i),
			Status:     status,
		}).Error)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/?page=1&page_size=500", nil)
	c.Set("address", "0xOwnerA")

	GetOpenBusinessBills(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Bills    []database.Bill `json:"bills"`
		Total    int64           `json:"total"`
		PageSize int             `json:"page_size"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Len(t, resp.Bills, 100)
	assert.Equal(t, int64(117), resp.Total)
	assert.Equal(t, 100, resp.PageSize)
	for _, bill := range resp.Bills {
		assert.Equal(t, database.BillStatusOpen, bill.Status)
	}
}

func TestGetOpenBusinessBillsIncludesPartialBills(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	setupBillListTestTables(t, gormDB)
	business := createBusinessHandlerTestBusiness(t, "0xOwnerPartial", "biz-partial-active")

	require.NoError(t, gormDB.Create(&database.Bill{
		BusinessID:  business.ID,
		BillNumber:  "OPEN-1",
		Status:      database.BillStatusOpen,
		CreatedAt:   time.Now().Add(-2 * time.Hour),
		TotalAmount: 1500,
	}).Error)
	require.NoError(t, gormDB.Create(&database.Bill{
		BusinessID:  business.ID,
		BillNumber:  "PARTIAL-1",
		Status:      database.BillStatusPartial,
		PaidAmount:  500,
		TotalAmount: 1500,
		CreatedAt:   time.Now().Add(-1 * time.Hour),
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatUint(uint64(business.ID), 10)}}
	c.Request = httptest.NewRequest(http.MethodGet, "/inside/businesses/1/bills/open?page=1&page_size=20", nil)

	GetOpenBusinessBills(c)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Bills []database.Bill `json:"bills"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Bills, 2)
	require.Contains(t, []database.BillStatus{body.Bills[0].Status, body.Bills[1].Status}, database.BillStatusPartial)
}

func createBillCreationMenu(t *testing.T, businessID uint) {
	t.Helper()

	categories := []database.MenuCategory{
		{
			ID:   "mains",
			Name: "Mains",
			Items: []database.MenuItem{
				{ID: "burger", Name: "Burger", Price: 10, IsAvailable: true},
			},
		},
	}
	payload, err := json.Marshal(categories)
	require.NoError(t, err)

	require.NoError(t, database.GetDB().Create(&database.Menu{
		BusinessID: businessID,
		Categories: string(payload),
		IsActive:   true,
		Version:    1,
	}).Error)
}

func createMalformedActiveBillForTable(t *testing.T, businessID uint, tableID uint) *database.Bill {
	t.Helper()

	bill := &database.Bill{
		BusinessID:     businessID,
		TableID:        tableID,
		BillNumber:     fmt.Sprintf("B-malformed-%d", time.Now().UnixNano()),
		Status:         database.BillStatusOpen,
		Items:          "{",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	return bill
}

func createSQLiteBillItemsTable(t testing.TB) {
	t.Helper()

	require.NoError(t, database.GetDB().Exec(`
		CREATE TABLE IF NOT EXISTS bill_items (
			id text PRIMARY KEY,
			bill_id integer NOT NULL,
			menu_item_id text,
			name text NOT NULL,
			price real NOT NULL,
			quantity integer NOT NULL,
			options text,
			item_type text,
			bundle_id integer,
			parent_bundle_id integer,
			source_offer_id integer,
			order_id INTEGER,
			subtotal real NOT NULL,
			created_at datetime
		)
	`).Error)
}

func TestPromotionLinesToBillItemsPreservesBundleOccurrence(t *testing.T) {
	bundleID := uint(10)
	lines := []database.OrderItem{
		{
			ID:                 "bundle-parent",
			ItemType:           services.OrderItemTypeBundle,
			BundleID:           &bundleID,
			BundleOccurrenceID: "bundle-parent",
			MenuItemName:       "Combo",
			Quantity:           1,
			Price:              15,
			Subtotal:           15,
		},
		{
			ID:                 "bundle-child",
			ItemType:           services.OrderItemTypeBundleItem,
			ParentBundleID:     &bundleID,
			BundleOccurrenceID: "bundle-parent",
			MenuItemName:       "Burger",
			Quantity:           1,
		},
	}

	items, subtotal := promotionLinesToBillItems(lines)
	require.Len(t, items, 2)
	require.Equal(t, "bundle-parent", items[0].BundleOccurrenceID)
	require.Equal(t, "bundle-parent", items[1].BundleOccurrenceID)
	require.Equal(t, 15.0, subtotal)
}

func TestPreserveBundleOccurrencesRestoresMetadataOmittedByClient(t *testing.T) {
	existing := []database.BillItem{
		{ID: "parent", BundleOccurrenceID: "occurrence-a"},
		{ID: "child", BundleOccurrenceID: "occurrence-a"},
	}
	incoming := []database.BillItem{{ID: "parent"}, {ID: "child"}, {ID: "new-item"}}

	got := preserveBundleOccurrences(incoming, existing)
	require.Equal(t, "occurrence-a", got[0].BundleOccurrenceID)
	require.Equal(t, "occurrence-a", got[1].BundleOccurrenceID)
	require.Empty(t, got[2].BundleOccurrenceID)
}

func TestCreateBusinessBill_ReturnsInternalErrorWhenActiveBillLookupFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Table{},
		&database.Bill{},
		&database.BillHistoryEvent{},
		&database.Menu{},
		&database.Offer{},
		&database.Bundle{},
	))
	createSQLiteBillItemsTable(t)
	business := createBusinessHandlerTestBusiness(t, "0xOwnerBillCreateLookup", "biz-bill-create-lookup")
	table := &database.Table{
		BusinessID: business.ID,
		Name:       "Lookup Error Table",
		TableCode:  "lookup-error-table",
		IsActive:   true,
	}
	require.NoError(t, database.GetDB().Create(table).Error)
	createBillCreationMenu(t, business.ID)
	createMalformedActiveBillForTable(t, business.ID, table.ID)

	body, err := json.Marshal(map[string]any{
		"table_id": table.ID,
		"items": []map[string]any{
			{
				"menu_item_id": "burger",
				"name":         "Burger",
				"quantity":     1,
			},
		},
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatUint(uint64(business.ID), 10)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", "0xOwnerBillCreateLookup")

	CreateBill(c)

	require.Equal(t, http.StatusInternalServerError, w.Code)

	var count int64
	require.NoError(t, database.GetDB().Model(&database.Bill{}).Where("table_id = ?", table.ID).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

// Manual mark-paid / approve-cash Gin handlers were removed (unmounted). Live
// settlement coverage lives on MarkAlternativePayment in internal/handlers
// (payments_regression_test.go + sse_emit_correctness_test.go).

func TestCreateBusiness_SendsSingleOnboardingEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	originalSender := sendCreateBusinessOnboardingEmail
	defer func() { sendCreateBusinessOnboardingEmail = originalSender }()

	var sendCount int
	var sentBusiness *database.Business
	sendCreateBusinessOnboardingEmail = func(business *database.Business) error {
		sendCount++
		snapshot := *business
		sentBusiness = &snapshot
		return nil
	}

	body, err := json.Marshal(map[string]interface{}{
		"name":               "Welcome Restaurant",
		"email":              "owner@example.com",
		"owner_name":         "Owner Name",
		"default_language":   "es",
		"settlement_address": "0x1111111111111111111111111111111111111111",
		"tipping_address":    "0x2222222222222222222222222222222222222222",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", "0xOwnerA")

	CreateBusiness(c)

	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, 1, sendCount, "registration should trigger exactly one onboarding email")
	if assert.NotNil(t, sentBusiness) {
		assert.Equal(t, "owner@example.com", sentBusiness.Email)
		assert.Equal(t, "es", sentBusiness.DefaultLanguage)
	}
}

func TestCreateBusiness_DoesNotAutoEnableCryptoPaymentPlugins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gdb := setupStaffHandlerTestDB(t)

	require.NoError(t, gdb.Create(&database.Plugin{Name: services.PluginNameUSDCPayment, DisplayName: "USDC Payment", IsActive: true}).Error)
	require.NoError(t, gdb.Create(&database.Plugin{Name: services.PluginNameCrossChainPayment, DisplayName: "Any Token Payment", IsActive: true}).Error)

	body, err := json.Marshal(map[string]interface{}{
		"name":               "Payment Ready Restaurant",
		"email":              "payment-ready@example.com",
		"owner_name":         "Owner Name",
		"settlement_address": "0x1111111111111111111111111111111111111111",
		"tipping_address":    "0x2222222222222222222222222222222222222222",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", "0xOwnerPaymentReady")

	CreateBusiness(c)

	require.Equal(t, http.StatusCreated, w.Code)

	var business database.Business
	require.NoError(t, gdb.Where("email = ?", "payment-ready@example.com").First(&business).Error)

	var bpCount int64
	require.NoError(t, gdb.Model(&database.BusinessPlugin{}).Where("business_id = ?", business.ID).Count(&bpCount).Error)
	assert.Zero(t, bpCount, "crypto rails must stay opt-in — never auto-enable on create")
}

func TestCreateBusiness_RejectsIdenticalSettlementAndTipWallets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_ = setupStaffHandlerTestDB(t)

	same := "0x1111111111111111111111111111111111111111"
	body, err := json.Marshal(map[string]interface{}{
		"name":               "Same Wallet Restaurant",
		"email":              "same-wallet@example.com",
		"owner_name":         "Owner Name",
		"settlement_address": same,
		"tipping_address":    same,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", "0xOwnerSameWallet")

	CreateBusiness(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Tip wallet must be different")
}

// Crypto rails must never light up before the business has somewhere to receive
// funds: creation without a settlement address enables NO payment plugins.
func TestCreateBusiness_NoPluginsWithoutSettlementAddress(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gdb := setupStaffHandlerTestDB(t)

	require.NoError(t, gdb.Create(&database.Plugin{Name: services.PluginNameUSDCPayment, DisplayName: "USDC Payment", IsActive: true}).Error)
	require.NoError(t, gdb.Create(&database.Plugin{Name: services.PluginNameCrossChainPayment, DisplayName: "Any Token Payment", IsActive: true}).Error)

	body, err := json.Marshal(map[string]interface{}{
		"name":       "No Wallet Restaurant",
		"email":      "no-wallet-biz@example.com",
		"owner_name": "Owner Name",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", "0xOwnerNoWallet")

	CreateBusiness(c)

	require.Equal(t, http.StatusCreated, w.Code)

	var business database.Business
	require.NoError(t, gdb.Where("email = ?", "no-wallet-biz@example.com").First(&business).Error)

	var bpCount int64
	require.NoError(t, gdb.Model(&database.BusinessPlugin{}).Where("business_id = ?", business.ID).Count(&bpCount).Error)
	assert.Zero(t, bpCount, "no payment plugins may be enabled before a settlement address exists")
}

// Adding a settlement address later must NOT auto-enable crypto rails —
// dinner venues opt into USDC under Plugins; Mercado Pago leads checkout.
func TestUpdateBusiness_DoesNotAutoEnableCryptoPluginsWhenSettlementAddressAdded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gdb := setupStaffHandlerTestDB(t)
	migrateWalletRotationTables(t, gdb)

	require.NoError(t, gdb.Create(&database.Plugin{Name: services.PluginNameUSDCPayment, DisplayName: "USDC Payment", IsActive: true}).Error)
	require.NoError(t, gdb.Create(&database.Plugin{Name: services.PluginNameCrossChainPayment, DisplayName: "Any Token Payment", IsActive: true}).Error)

	business := &database.Business{
		BusinessId:   "biz-late-wallet",
		Name:         "Late Wallet Restaurant",
		OwnerName:    "Owner Name",
		Email:        "late-wallet@example.com",
		OwnerAddress: "0xOwnerLateWallet",
		IsActive:     true,
	}
	require.NoError(t, gdb.Create(business).Error)

	body, err := json.Marshal(map[string]string{
		"settlement_address": "0x3333333333333333333333333333333333333333",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", "0xOwnerLateWallet")

	UpdateBusiness(c)

	require.Equal(t, http.StatusOK, w.Code)

	var bpCount int64
	require.NoError(t, gdb.Model(&database.BusinessPlugin{}).Where("business_id = ?", business.ID).Count(&bpCount).Error)
	assert.Zero(t, bpCount, "crypto rails must stay opt-in after adding a settlement wallet")
}

func TestUpdateBusiness_RejectsIdenticalSettlementAndTipWallets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gdb := setupStaffHandlerTestDB(t)

	same := "0x4444444444444444444444444444444444444444"
	business := &database.Business{
		BusinessId:     "biz-same-tip",
		Name:           "Same Tip Restaurant",
		OwnerName:      "Owner Name",
		Email:          "same-tip@example.com",
		OwnerAddress:   "0xOwnerSameTip",
		SettlementAddr: "0x3333333333333333333333333333333333333333",
		IsActive:       true,
	}
	require.NoError(t, gdb.Create(business).Error)

	body, err := json.Marshal(map[string]string{
		"tipping_address":    same,
		"settlement_address": same,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", "0xOwnerSameTip")

	UpdateBusiness(c)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Tip wallet must be different")
}

// TestUpdateBusiness_LegacySameWalletsAllowProfileOnlySave pins Chopper's
// SECURITY REQUEST_CHANGES on PR #315: legacy USDC rows may already share one
// settlement+tip address. UpdateBusiness must not 400 those venues on
// profile-only saves (name/tax) that never mutate wallets. Wallet mutations
// that produce the same collision still reject.
func TestUpdateBusiness_LegacySameWalletsAllowProfileOnlySave(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gdb := setupStaffHandlerTestDB(t)

	same := "0x5555555555555555555555555555555555555555"
	business := &database.Business{
		BusinessId:     "biz-legacy-same-wallets",
		Name:           "Legacy Same Wallets",
		OwnerName:      "Owner Name",
		Email:          "legacy-same-wallets@example.com",
		OwnerAddress:   "0xOwnerLegacySame",
		SettlementAddr: same,
		TippingAddr:    same,
		TaxRate:        0.1,
		IsActive:       true,
	}
	require.NoError(t, gdb.Create(business).Error)

	nameBody, err := json.Marshal(map[string]string{
		"name": "Renamed Legacy Venue",
	})
	require.NoError(t, err)

	nameW := httptest.NewRecorder()
	nameC, _ := gin.CreateTestContext(nameW)
	nameC.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	nameC.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(nameBody))
	nameC.Request.Header.Set("Content-Type", "application/json")
	nameC.Set("address", "0xOwnerLegacySame")

	UpdateBusiness(nameC)

	require.Equal(t, http.StatusOK, nameW.Code, "name-only save must succeed for legacy same-wallet venues")

	var afterName database.Business
	require.NoError(t, gdb.First(&afterName, business.ID).Error)
	assert.Equal(t, "Renamed Legacy Venue", afterName.Name)
	assert.Equal(t, same, afterName.SettlementAddr)
	assert.Equal(t, same, afterName.TippingAddr)

	tax := 0.21
	taxBody, err := json.Marshal(map[string]interface{}{
		"tax_rate": tax,
	})
	require.NoError(t, err)

	taxW := httptest.NewRecorder()
	taxC, _ := gin.CreateTestContext(taxW)
	taxC.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	taxC.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(taxBody))
	taxC.Request.Header.Set("Content-Type", "application/json")
	taxC.Set("address", "0xOwnerLegacySame")

	UpdateBusiness(taxC)

	require.Equal(t, http.StatusOK, taxW.Code, "tax-only save must succeed for legacy same-wallet venues")

	var afterTax database.Business
	require.NoError(t, gdb.First(&afterTax, business.ID).Error)
	assert.InDelta(t, tax, afterTax.TaxRate, 0.0001)
	assert.Equal(t, same, afterTax.SettlementAddr)
	assert.Equal(t, same, afterTax.TippingAddr)

	newSame := "0x6666666666666666666666666666666666666666"
	walletBody, err := json.Marshal(map[string]string{
		"settlement_address": newSame,
		"tipping_address":    newSame,
	})
	require.NoError(t, err)

	walletW := httptest.NewRecorder()
	walletC, _ := gin.CreateTestContext(walletW)
	walletC.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	walletC.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(walletBody))
	walletC.Request.Header.Set("Content-Type", "application/json")
	walletC.Set("address", "0xOwnerLegacySame")

	UpdateBusiness(walletC)

	require.Equal(t, http.StatusBadRequest, walletW.Code)
	assert.Contains(t, walletW.Body.String(), "Tip wallet must be different")

	var afterWallet database.Business
	require.NoError(t, gdb.First(&afterWallet, business.ID).Error)
	assert.Equal(t, same, afterWallet.SettlementAddr, "rejected wallet mutation must not persist")
	assert.Equal(t, same, afterWallet.TippingAddr, "rejected wallet mutation must not persist")
}

func TestCreateBusiness_SucceedsWhenPluginEnableFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gdb := setupStaffHandlerTestDB(t)
	// Deliberately do NOT seed the payment plugins.

	body, err := json.Marshal(map[string]interface{}{
		"name":               "Plugin Failure Restaurant",
		"email":              "plugin-failure-biz@example.com",
		"owner_name":         "Owner Name",
		"settlement_address": "0x1111111111111111111111111111111111111111",
		"tipping_address":    "0x2222222222222222222222222222222222222222",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", "0xOwnerPluginFailure")

	CreateBusiness(c)

	require.Equal(t, http.StatusCreated, w.Code, "business creation must succeed even if plugin enable fails")

	var business database.Business
	require.NoError(t, gdb.Where("email = ?", "plugin-failure-biz@example.com").First(&business).Error)
}

func TestCreateBusiness_SendsAdminNewSignupAlert(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)

	originalSender := sendAdminNewSignupEmailHook
	defer func() { sendAdminNewSignupEmailHook = originalSender }()

	var sendCount int
	sendAdminNewSignupEmailHook = func(business *database.Business) error {
		sendCount++
		return nil
	}

	body, err := json.Marshal(map[string]interface{}{
		"name":               "Admin Alert Restaurant",
		"email":              "admin-alert-biz@example.com",
		"owner_name":         "Owner Name",
		"settlement_address": "0x1111111111111111111111111111111111111111",
		"tipping_address":    "0x2222222222222222222222222222222222222222",
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", "0xOwnerAdminAlert")

	CreateBusiness(c)

	require.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, 1, sendCount, "business creation should trigger exactly one admin signup alert")
}

// TestUpdateBusiness_StaffResponseDoesNotLeakOwnerSecrets verifies that a
// staff member updating a business receives a scoped projection, not the
// full Business struct. (RBAC-BIZRECORD-LEAK-01 sibling)
func TestUpdateBusiness_StaffResponseDoesNotLeakOwnerSecrets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gdb := setupOwnershipTestDB(t)
	require.NoError(t, gdb.AutoMigrate(&database.BusinessDesignSettings{}, &database.BusinessAiSettings{}))

	owner := "0x0000000000000000000000000000000000000001"
	onboardTime := time.Now().Add(-24 * time.Hour)
	biz := &database.Business{
		BusinessId:            "update-leak-biz",
		OwnerAddress:          owner,
		Name:                  "Update Leak Biz",
		IsActive:              true,
		SettlementAddr:        "0xSETTLE_SECRET",
		TippingAddr:           "0xTIP_SECRET",
		Email:                 "owner@secret.test",
		DefaultCurrency:       "USD",
		OnboardingCompletedAt: &onboardTime,
	}
	require.NoError(t, gdb.Create(biz).Error)

	body, _ := json.Marshal(map[string]string{"name": "Updated Name"})
	c, w := makeTestContext("PUT", "/businesses/"+biz.BusinessId,
		gin.Params{{Key: "id", Value: biz.BusinessId}})
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("token_type", "staff")
	c.Set("staff_business_id", biz.ID)

	UpdateBusiness(c)

	require.Equal(t, http.StatusOK, w.Code)
	resp := w.Body.String()
	assert.NotContains(t, resp, "0xSETTLE_SECRET")
	assert.NotContains(t, resp, "0xTIP_SECRET")
	assert.NotContains(t, resp, "owner@secret.test")
	assert.NotContains(t, resp, `"settlement_address"`)
	assert.NotContains(t, resp, `"tipping_address"`)
	assert.NotContains(t, resp, `"owner_address"`)

	// Operational fields must be present.
	assert.Contains(t, resp, `"name":"Updated Name"`)
	assert.Contains(t, resp, `"onboarding_completed_at"`)
}

// TestUpdateBusiness_OwnerResponseHasFullRecord verifies the positive case.
func TestUpdateBusiness_OwnerResponseHasFullRecord(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gdb := setupOwnershipTestDB(t)
	require.NoError(t, gdb.AutoMigrate(&database.BusinessDesignSettings{}, &database.BusinessAiSettings{}))

	owner := "0x0000000000000000000000000000000000000001"
	biz := &database.Business{
		BusinessId:      "update-owner-biz",
		OwnerAddress:    owner,
		Name:            "Owner Biz",
		IsActive:        true,
		SettlementAddr:  "0xSETTLE_OWNER",
		TippingAddr:     "0xTIP_OWNER",
		Email:           "owner@owner.test",
		DefaultCurrency: "USD",
	}
	require.NoError(t, gdb.Create(biz).Error)

	body, _ := json.Marshal(map[string]string{"name": "Owner Updated"})
	c, w := makeTestContext("PUT", "/businesses/"+biz.BusinessId,
		gin.Params{{Key: "id", Value: biz.BusinessId}})
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", owner)

	UpdateBusiness(c)

	require.Equal(t, http.StatusOK, w.Code)
	resp := w.Body.String()
	assert.Contains(t, resp, "0xSETTLE_OWNER")
	assert.Contains(t, resp, "0xTIP_OWNER")
	assert.Contains(t, resp, "owner@owner.test")
}

func TestUpdateBusiness_StaffWalletWriteReturnsSkippedFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gdb := setupOwnershipTestDB(t)
	require.NoError(t, gdb.AutoMigrate(&database.BusinessDesignSettings{}, &database.BusinessAiSettings{}))

	owner := "0x0000000000000000000000000000000000000001"
	biz := &database.Business{
		BusinessId:      "skipped-fields-biz",
		OwnerAddress:    owner,
		Name:            "Skipped Fields Biz",
		IsActive:        true,
		SettlementAddr:  "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		DefaultCurrency: "USD",
	}
	require.NoError(t, gdb.Create(biz).Error)

	body, _ := json.Marshal(map[string]string{
		"name":               "Renamed By Manager",
		"settlement_address": "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	})
	c, w := makeTestContext("PUT", "/businesses/"+biz.BusinessId,
		gin.Params{{Key: "id", Value: biz.BusinessId}})
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("token_type", "staff")
	c.Set("staff_business_id", biz.ID)

	UpdateBusiness(c)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Contains(t, resp, "skipped_fields",
		"a staff request carrying owner-only fields must disclose the drop")
	assert.ElementsMatch(t, []interface{}{"settlement_address"}, resp["skipped_fields"])

	// The wallet did NOT move; the rename DID land.
	var row database.Business
	require.NoError(t, gdb.First(&row, biz.ID).Error)
	assert.Equal(t, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", row.SettlementAddr)
	assert.Equal(t, "Renamed By Manager", row.Name)
}

func TestUpdateBusiness_StaffCleanSaveHasNoSkippedFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gdb := setupOwnershipTestDB(t)
	require.NoError(t, gdb.AutoMigrate(&database.BusinessDesignSettings{}, &database.BusinessAiSettings{}))

	owner := "0x0000000000000000000000000000000000000002"
	biz := &database.Business{
		BusinessId:      "clean-staff-save-biz",
		OwnerAddress:    owner,
		Name:            "Clean Staff Save Biz",
		IsActive:        true,
		DefaultCurrency: "USD",
	}
	require.NoError(t, gdb.Create(biz).Error)

	body, _ := json.Marshal(map[string]string{"name": "Just A Rename"})
	c, w := makeTestContext("PUT", "/businesses/"+biz.BusinessId,
		gin.Params{{Key: "id", Value: biz.BusinessId}})
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("token_type", "staff")
	c.Set("staff_business_id", biz.ID)

	UpdateBusiness(c)

	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "skipped_fields")
}

// TestUpdateBusiness_NameOnlySaveDoesNotUnpublishBusinessPage pins the
// pointer-patch contract P2-23's frontend fix relies on: fields absent from
// the PUT body survive untouched.
func TestUpdateBusiness_NameOnlySaveDoesNotUnpublishBusinessPage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	business := createBusinessHandlerTestBusiness(t, "0xOwnerPage", "biz-page-preserve")
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		Updates(map[string]interface{}{
			"business_page_enabled": true,
			"banner_images":         `["https://cdn.example.com/banner-1.jpg"]`,
			"show_gallery":          true,
		}).Error)

	body, _ := json.Marshal(map[string]string{"name": "Renamed Without Page Fields"})
	c, w := makeTestContext("PUT", "/businesses/"+business.BusinessId,
		gin.Params{{Key: "id", Value: business.BusinessId}})
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", "0xOwnerPage")

	UpdateBusiness(c)
	require.Equal(t, http.StatusOK, w.Code)

	var row database.Business
	require.NoError(t, database.GetDB().First(&row, business.ID).Error)
	assert.True(t, row.BusinessPageEnabled, "unsent business_page_enabled must survive")
	assert.Equal(t, `["https://cdn.example.com/banner-1.jpg"]`, row.BannerImages)
	assert.True(t, row.ShowGallery)
	assert.Equal(t, "Welcome to our place", row.WelcomeMessage)
	assert.Equal(t, "Renamed Without Page Fields", row.Name)
}

func putBusinessUpdate(t *testing.T, business *database.Business, owner string, payload map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	c, w := makeTestContext(http.MethodPut, "/businesses/"+business.BusinessId,
		gin.Params{{Key: "id", Value: business.BusinessId}})
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("address", owner)
	UpdateBusiness(c)
	return w
}

func TestUpdateBusiness_RejectsMetadataServiceLogo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	const owner = "0xOwnerLogoSSRF"
	business := createBusinessHandlerTestBusiness(t, owner, "biz-ssrf-logo")
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		Update("logo", "/media/public/existing-logo.png").Error)

	w := putBusinessUpdate(t, business, owner, map[string]string{
		"logo": "http://169.254.169.254/x",
	})
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Logo must be an uploaded image")

	var row database.Business
	require.NoError(t, database.GetDB().First(&row, business.ID).Error)
	assert.Equal(t, "/media/public/existing-logo.png", row.Logo)
}

func TestUpdateBusiness_RejectsMetadataServiceBannerImages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	const owner = "0xOwnerBannerSSRF"
	business := createBusinessHandlerTestBusiness(t, owner, "biz-ssrf-banner")
	original := `["/media/public/existing-banner.png"]`
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		Update("banner_images", original).Error)

	w := putBusinessUpdate(t, business, owner, map[string]string{
		"banner_images": `["http://169.254.169.254/x"]`,
	})
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Banner images must be uploaded images")

	var row database.Business
	require.NoError(t, database.GetDB().First(&row, business.ID).Error)
	assert.Equal(t, original, row.BannerImages)
}

func TestUpdateBusiness_PersistsOwnMediaLogoAndBanners(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	const owner = "0xOwnerOwnMedia"
	business := createBusinessHandlerTestBusiness(t, owner, "biz-own-media")

	const logo = "/media/public/logo.png"
	const banners = `["/media/public/banner.png"]`
	w := putBusinessUpdate(t, business, owner, map[string]string{
		"logo":          logo,
		"banner_images": banners,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var row database.Business
	require.NoError(t, database.GetDB().First(&row, business.ID).Error)
	assert.Equal(t, logo, row.Logo)
	assert.Equal(t, banners, row.BannerImages)
}

func TestValidateStorefrontImageURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "empty", raw: "", wantErr: false},
		{name: "public media", raw: "/media/public/logo.png", wantErr: false},
		{name: "link local", raw: "http://169.254.169.254/x", wantErr: true},
		{name: "foreign https", raw: "https://evil.example/x.png", wantErr: true},
		{name: "protected media", raw: "/media/protected/secret.png", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateStorefrontImageURL(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCheckCustomURLAvailability_LookupFailureReturns500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	require.NoError(t, database.GetDB().Exec("DROP TABLE businesses").Error)

	c, w := makeTestContext(http.MethodGet, "/?url=free-slug", nil)
	CheckCustomURLAvailability(c)

	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), `"available":true`)
	assert.Contains(t, w.Body.String(), "Failed to check custom URL availability")
	assert.NotContains(t, w.Body.String(), "no such table")

	err := validateCustomURL("free-slug", 0)
	require.ErrorIs(t, err, errCustomURLLookupFailed)
}

// migrateWalletRotationTables adds the tables UpdateBusiness touches when a
// payout wallet rotates (open bills and active crypto quotes).
func migrateWalletRotationTables(t *testing.T, gdb *gorm.DB) {
	t.Helper()
	require.NoError(t, gdb.AutoMigrate(&database.Bill{}, &database.CryptoPaymentQuote{}))
}
