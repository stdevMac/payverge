package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/database/dbtest"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func performBusinessJSONRequest(t *testing.T, router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	payload, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(method, path, bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func createSQLiteBusinessRegressionBillItemsTable(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec(`
CREATE TABLE bill_items (
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

func TestDetermineBusinessOwnerLanguage_PrefersOAuthUserLanguage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.User{}))

	user := &database.User{
		ID:               42,
		Email:            "owner@example.com",
		LanguageSelected: "pt",
		Role:             "user",
	}
	require.NoError(t, database.GetDB().Create(user).Error)

	businessUserID := user.ID
	business := &database.Business{
		BusinessId:      "lang-biz",
		Name:            "Language Biz",
		UserID:          &businessUserID,
		Email:           user.Email,
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultLanguage: "en",
		OwnerAddress:    "",
	}

	assert.Equal(t, "pt", determineBusinessOwnerLanguage(business))
}

func TestGetUserList_SearchesBusinessName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.User{}))

	coreUser := &database.User{
		Email: "core-owner@example.com",
		Name:  "Core Owner",
		Role:  "user",
	}
	aiProUser := &database.User{
		Email: "ai-owner@example.com",
		Name:  "AI Owner",
		Role:  "user",
	}
	require.NoError(t, database.GetDB().Create(coreUser).Error)
	require.NoError(t, database.GetDB().Create(aiProUser).Error)

	coreUserID := coreUser.ID
	aiProUserID := aiProUser.ID
	require.NoError(t, database.GetDB().Create(&database.Business{
		BusinessId:     "core-cafe",
		Name:           "Core Cafe",
		UserID:         &coreUserID,
		OwnerAddress:   "0xCoreOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}).Error)
	require.NoError(t, database.GetDB().Create(&database.Business{
		BusinessId:     "needle-bistro",
		Name:           "Needle Bistro",
		UserID:         &aiProUserID,
		OwnerAddress:   "0xAIProOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/admin/users?search=Needle", nil)

	GetUserList(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Users []AdminUserListItem `json:"users"`
		Total int64               `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Users, 1)
	assert.Equal(t, aiProUser.ID, response.Users[0].ID)
	assert.Equal(t, "Needle Bistro", response.Users[0].BusinessName)
	assert.Equal(t, int64(1), response.Total)
}

func TestGetPaymentHistory_RejectsCrossBusinessStaffAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Payment{}))

	business := createOwnedBusiness(t, "0xOwnerA", "Target Biz")
	otherBusiness := createOwnedBusiness(t, "0xOwnerB", "Other Biz")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/inside/businesses/%d/payments/history", business.ID), nil)
	c.Set("token_type", "staff")
	c.Set("staff_business_id", float64(otherBusiness.ID))

	GetPaymentHistory(c)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "Access denied")
}

func TestGetBill_DeniesCrossTenantAccessWithoutMiddlewareContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupBillAccessMiddlewareDB(t)
	_, bill := createBillAccessFixture(t, "0xOwnerA", false)

	router := gin.New()
	router.GET("/bills/:bill_id",
		func(c *gin.Context) {
			c.Set("token_type", "web3")
			c.Set("address", "0xOwnerB")
		},
		GetBill,
	)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/bills/%d", bill.ID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "Not authorized to access this bill")
}

func TestPaymentHistoryRoute_ServerForbiddenByRBAC(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Payment{}))

	business := createOwnedBusiness(t, "0xOwnerA", "Payment Biz")
	staff := createStaffMember(t, business.ID, "server@example.com", "Server")
	require.NoError(t, database.GetDB().Model(&database.Staff{}).Where("id = ?", staff.ID).Update("role", database.StaffRoleServer).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_role", string(database.StaffRoleServer))
		c.Set("staff_id", staff.ID)
		c.Set("staff_business_id", float64(business.ID))
		c.Next()
	})
	router.GET("/inside/businesses/:id/payments/history", RoleBasedAccessMiddleware("financial:read"), GetPaymentHistory)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/inside/businesses/%d/payments/history", business.ID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestGetPaymentHistory_UsesExplicitDateRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Table{}, &database.Bill{}, &database.Payment{}, &database.AlternativePayment{}))
	require.NoError(t, dbtest.EnsurePaymentEventsView(gormDB))

	business := createOwnedBusiness(t, "0xOwnerA", "Payment Range Biz")
	require.NoError(t, database.GetDB().Model(business).Updates(map[string]any{
		"default_currency": "JPY",
		"display_currency": "JPY",
	}).Error)
	table := &database.Table{
		BusinessID: business.ID,
		TableCode:  "T-1",
		Name:       "Patio 1",
	}
	require.NoError(t, database.GetDB().Create(table).Error)

	now := time.Now()
	inRangeTime := now.AddDate(0, 0, -2)
	outOfRangeTime := now.AddDate(0, -2, 0)

	inRangeBill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     "IN-RANGE-001",
		TotalAmount:    40,
		TipAmount:      5,
		Status:         database.BillStatusPaid,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		CreatedAt:      inRangeTime,
		UpdatedAt:      inRangeTime,
	}
	require.NoError(t, database.GetDB().Create(inRangeBill).Error)

	outOfRangeBill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     "OUT-OF-RANGE-001",
		TotalAmount:    25,
		TipAmount:      2,
		Status:         database.BillStatusPaid,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		CreatedAt:      outOfRangeTime,
		UpdatedAt:      outOfRangeTime,
	}
	require.NoError(t, database.GetDB().Create(outOfRangeBill).Error)

	require.NoError(t, database.GetDB().Create(&database.Payment{
		BillID:    inRangeBill.ID,
		PayerAddr: "0xabc",
		Amount:    40,
		TipAmount: 5,
		TxHash:    "0xinrange",
		Status:    database.PaymentStatusConfirmed,
		CreatedAt: inRangeTime,
		UpdatedAt: inRangeTime,
	}).Error)

	require.NoError(t, database.GetDB().Create(&database.Payment{
		BillID:    outOfRangeBill.ID,
		PayerAddr: "0xdef",
		Amount:    25,
		TipAmount: 2,
		TxHash:    "0xoutofrange",
		Status:    database.PaymentStatusConfirmed,
		CreatedAt: outOfRangeTime,
		UpdatedAt: outOfRangeTime,
	}).Error)

	startDate := now.AddDate(0, 0, -7).Format("2006-01-02")
	endDate := now.Format("2006-01-02")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/payments/history?start_date=%s&end_date=%s", business.ID, startDate, endDate),
		nil,
	)
	c.Set("token_type", "web3")
	c.Set("address", "0xOwnerA")

	GetPaymentHistory(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var items []PaymentHistoryItem
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &items))
	require.Len(t, items, 1)
	assert.Equal(t, "IN-RANGE-001", items[0].BillNumber)
	assert.Equal(t, "Patio 1", items[0].TableName)
	assert.Equal(t, "JPY", items[0].Currency)
}

func TestLoadPaymentHistoryItemsBatchLoadsBillsAndTables(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Table{}, &database.Bill{}, &database.Payment{}, &database.AlternativePayment{}))
	require.NoError(t, dbtest.EnsurePaymentEventsView(gormDB))

	business := createOwnedBusiness(t, "0xOwnerA", "Payment Batch Biz")
	paymentTime := time.Now().Add(-time.Hour)

	for i := 0; i < 6; i++ {
		table := &database.Table{
			BusinessID: business.ID,
			TableCode:  fmt.Sprintf("T-%d", i),
			Name:       fmt.Sprintf("Table %d", i),
		}
		require.NoError(t, database.GetDB().Create(table).Error)

		bill := &database.Bill{
			BusinessID:     business.ID,
			TableID:        table.ID,
			BillNumber:     fmt.Sprintf("BATCH-%03d", i),
			TotalAmount:    1000,
			Status:         database.BillStatusPaid,
			SettlementAddr: "0x1111111111111111111111111111111111111111",
			TippingAddr:    "0x2222222222222222222222222222222222222222",
			CreatedAt:      paymentTime,
			UpdatedAt:      paymentTime,
		}
		require.NoError(t, database.GetDB().Create(bill).Error)

		require.NoError(t, database.GetDB().Create(&database.Payment{
			BillID:    bill.ID,
			PayerAddr: fmt.Sprintf("0xpayer%d", i),
			Amount:    1000,
			TipAmount: 100,
			TxHash:    fmt.Sprintf("0xbatch%d", i),
			Status:    database.PaymentStatusConfirmed,
			CreatedAt: paymentTime,
			UpdatedAt: paymentTime,
		}).Error)
	}

	queryCount := 0
	require.NoError(t, gormDB.Callback().Query().Before("gorm:query").Register("count_payment_history_queries", func(_ *gorm.DB) {
		queryCount++
	}))

	items, _, err := loadPaymentHistoryItemsFiltered(business.ID, paymentTime.Add(-time.Hour), paymentTime.Add(time.Hour), PaymentHistoryFilter{})

	require.NoError(t, err)
	require.Len(t, items, 6)
	assert.LessOrEqual(t, queryCount, 5, "payment history should batch-load business, payments, bills, and tables")
}

// TestAddBillItem_TotalEqualsSumOfComponents guards the bill-totals invariant:
// the stored subtotal + tax + service-fee must always sum to the stored total.
// The inline computation rounded (subtotal + UNROUNDED tax + UNROUNDED fee) for
// the total while rounding tax/fee separately for storage, so with both a tax
// and a service fee that land on a rounding boundary (8.5% + 8.5% on $1.00) the
// total drifted a cent away from its own line items — a bill that doesn't add
// up, and the guest is charged the mismatched total. CreateBill already used the
// canonical services.ComputeBillTotals; UpdateBill/AddBillItem must too.
func TestAddBillItem_TotalEqualsSumOfComponents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Table{}, &database.Bill{}, &database.Payment{}, &database.BillHistoryEvent{}))
	createSQLiteBusinessRegressionBillItemsTable(t, gormDB)

	business := createOwnedBusiness(t, "0xOwnerA", "Totals Biz")
	business.TaxRate = 8.5
	business.ServiceFeeRate = 8.5
	require.NoError(t, database.GetDB().Save(business).Error)

	bill := &database.Bill{
		BusinessID:     business.ID,
		BillNumber:     "TOTALS-001",
		Status:         database.BillStatusOpen,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xOwnerA")
		c.Next()
	})
	router.POST("/bills/:bill_id/items", AddBillItem)

	w := performBusinessJSONRequest(t, router, http.MethodPost, fmt.Sprintf("/bills/%d/items", bill.ID), map[string]any{
		"menu_item_id": "item-1",
		"name":         "Item",
		"price":        1.00,
		"quantity":     1,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var updated database.Bill
	require.NoError(t, database.GetDB().First(&updated, bill.ID).Error)
	assert.Equal(t,
		updated.Subtotal+updated.TaxAmount+updated.ServiceFeeAmount,
		updated.TotalAmount,
		"subtotal(%d)+tax(%d)+fee(%d) must equal total(%d)",
		updated.Subtotal, updated.TaxAmount, updated.ServiceFeeAmount, updated.TotalAmount,
	)
}

// TestAddBillItem_PreservesLoyaltyDiscount guards the loyalty redemption money
// path against the operator-edit recompute. A guest redeems points on an open
// bill (TotalAmount baked net of LoyaltyDiscountCents); if the operator then adds
// an item, the handler recomputes the total — and must keep it net of the still-
// applied discount, not silently restore the gross total (which would erase the
// redemption the guest already spent points on).
func TestAddBillItem_PreservesLoyaltyDiscount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Table{}, &database.Bill{}, &database.Payment{}, &database.BillHistoryEvent{}))
	createSQLiteBusinessRegressionBillItemsTable(t, gormDB)

	business := createOwnedBusiness(t, "0xOwnerA", "Loyalty Edit Biz")
	business.TaxRate = 0
	business.ServiceFeeRate = 0
	require.NoError(t, database.GetDB().Save(business).Error)

	// Open bill already carrying a $3.00 redemption.
	bill := &database.Bill{
		BusinessID:           business.ID,
		BillNumber:           "LOYALTY-EDIT-001",
		Status:               database.BillStatusOpen,
		LoyaltyDiscountCents: 300,
		SettlementAddr:       "0x1111111111111111111111111111111111111111",
		TippingAddr:          "0x2222222222222222222222222222222222222222",
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xOwnerA")
		c.Next()
	})
	router.POST("/bills/:bill_id/items", AddBillItem)

	// Add a $10.00 item -> gross 1000¢, net of the $3 discount = 700¢.
	w := performBusinessJSONRequest(t, router, http.MethodPost, fmt.Sprintf("/bills/%d/items", bill.ID), map[string]any{
		"menu_item_id": "item-1",
		"name":         "Entree",
		"price":        10.00,
		"quantity":     1,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var updated database.Bill
	require.NoError(t, database.GetDB().First(&updated, bill.ID).Error)
	assert.Equal(t, int64(300), updated.LoyaltyDiscountCents, "discount must remain applied")
	assert.Equal(t, int64(1000), updated.Subtotal, "gross subtotal unchanged")
	assert.Equal(t, int64(700), updated.TotalAmount, "total must stay net of the loyalty discount (1000 - 300)")
}

func TestAddBillItem_AllowsZeroPriceItems(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Table{}, &database.Bill{}, &database.Payment{}, &database.BillHistoryEvent{}))
	createSQLiteBusinessRegressionBillItemsTable(t, gormDB)

	business := createOwnedBusiness(t, "0xOwnerA", "Zero Price Biz")
	bill := &database.Bill{
		BusinessID:     business.ID,
		BillNumber:     "ZERO-PRICE-001",
		Status:         database.BillStatusOpen,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xOwnerA")
		c.Next()
	})
	router.POST("/bills/:bill_id/items", AddBillItem)

	w := performBusinessJSONRequest(t, router, http.MethodPost, fmt.Sprintf("/bills/%d/items", bill.ID), map[string]any{
		"menu_item_id": "comp-dessert",
		"name":         "Comp Dessert",
		"price":        0,
		"quantity":     1,
	})

	require.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Items []database.BillItem `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Items, 1)
	assert.Equal(t, 0.0, response.Items[0].Price)
	assert.Equal(t, 0.0, response.Items[0].Subtotal)
}

func TestExportPaymentHistory_ReturnsPaymentCSV(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupStaffHandlerTestDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.Table{}, &database.Bill{}, &database.Payment{}, &database.AlternativePayment{}))
	require.NoError(t, dbtest.EnsurePaymentEventsView(gormDB))

	business := createOwnedBusiness(t, "0xOwnerA", "Payment Export Biz")
	table := &database.Table{
		BusinessID: business.ID,
		TableCode:  "T-2",
		Name:       "Window 2",
	}
	require.NoError(t, database.GetDB().Create(table).Error)

	paymentTime := time.Now().AddDate(0, 0, -1)
	bill := &database.Bill{
		BusinessID:     business.ID,
		TableID:        table.ID,
		BillNumber:     "EXPORT-001",
		TotalAmount:    55,
		TipAmount:      6,
		Status:         database.BillStatusPaid,
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		CreatedAt:      paymentTime,
		UpdatedAt:      paymentTime,
	}
	require.NoError(t, database.GetDB().Create(bill).Error)

	require.NoError(t, database.GetDB().Create(&database.Payment{
		BillID:    bill.ID,
		PayerAddr: "0xaaa",
		Amount:    55,
		TipAmount: 6,
		TxHash:    "0xexport",
		Status:    database.PaymentStatusConfirmed,
		CreatedAt: paymentTime,
		UpdatedAt: paymentTime,
	}).Error)

	startDate := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	endDate := time.Now().Format("2006-01-02")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", business.ID)}}
	c.Request = httptest.NewRequest(
		http.MethodGet,
		fmt.Sprintf("/inside/businesses/%d/payments/export?start_date=%s&end_date=%s&format=csv", business.ID, startDate, endDate),
		nil,
	)
	c.Set("token_type", "web3")
	c.Set("address", "0xOwnerA")

	ExportPaymentHistory(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/csv", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Header().Get("Content-Disposition"), "payment_history_custom.csv")
	assert.Contains(t, w.Body.String(), "Date,Bill Number,Table,Amount,Tip Amount,Currency,Status,Payer Address,Transaction Hash")
	assert.Contains(t, w.Body.String(), "EXPORT-001")
	assert.NotContains(t, w.Body.String(), "Total Amount,Tip Amount,Status,Items")
}
