package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/structs"
)

func validOptionalCustomerToken(t *testing.T, customer *database.Customer) string {
	t.Helper()

	tokenString := fmt.Sprintf("pending-customer-token-%d", customer.ID)
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &customer.ID,
		TokenHash: session.HashToken(tokenString),
		Provider:  "customer",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	token, err := GenerateCustomerToken(customer.ID, customer.Email, sess.ID)
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.UpdateTokenHash(sess.ID, session.HashToken(token)))
	return token
}

func setupOptionalCustomerTest(t *testing.T) *database.Customer {
	t.Helper()
	db := setupSessionTestDB(t)
	session.GlobalStore = session.NewStore(db)
	structs.SecretKey = []byte("test-secret-key-for-optional-customer-tests")
	t.Cleanup(func() {
		session.GlobalStore = nil
	})

	customer := &database.Customer{
		Email:         fmt.Sprintf("%s@example.com", t.Name()),
		Name:          "Optional Customer",
		Phone:         "+15555550100",
		WalletAddress: "0x123",
		IsActive:      true,
	}
	require.NoError(t, db.Create(customer).Error)
	return customer
}

func TestOptionalCustomerFromRequestIgnoresMissingCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	customer, ok := OptionalCustomerFromRequest(c)

	require.False(t, ok)
	require.Nil(t, customer)
}

func TestOptionalCustomerFromRequestRejectsInvalidCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.AddCookie(&http.Cookie{Name: "customer_token", Value: "invalid"})

	customer, ok := OptionalCustomerFromRequest(c)

	require.False(t, ok)
	require.Nil(t, customer)
}

func TestOptionalCustomerFromRequestReturnsActiveCustomerFromValidCookie(t *testing.T) {
	customer := setupOptionalCustomerTest(t)
	token := validOptionalCustomerToken(t, customer)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.AddCookie(&http.Cookie{Name: "customer_token", Value: token})

	resolved, ok := OptionalCustomerFromRequest(c)

	require.True(t, ok)
	require.NotNil(t, resolved)
	require.Equal(t, customer.ID, resolved.ID)
	require.Equal(t, customer.Email, resolved.Email)
	require.Equal(t, customer.Name, resolved.Name)
	require.Equal(t, customer.Phone, resolved.Phone)
	require.Equal(t, customer.WalletAddress, resolved.WalletAddress)
}

func TestOptionalCustomerFromRequestIgnoresBearerTokenWithoutCookie(t *testing.T) {
	customer := setupOptionalCustomerTest(t)
	token := validOptionalCustomerToken(t, customer)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Authorization", "Bearer "+token)

	resolved, ok := OptionalCustomerFromRequest(c)

	require.False(t, ok)
	require.Nil(t, resolved)
}

func TestOptionalCustomerFromRequestInvalidAuthorizationDoesNotMaskCookie(t *testing.T) {
	customer := setupOptionalCustomerTest(t)
	token := validOptionalCustomerToken(t, customer)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set("Authorization", "Bearer invalid")
	c.Request.AddCookie(&http.Cookie{Name: "customer_token", Value: token})

	resolved, ok := OptionalCustomerFromRequest(c)

	require.True(t, ok)
	require.NotNil(t, resolved)
	require.Equal(t, customer.ID, resolved.ID)
}

func TestOptionalCustomerFromRequestRejectsInactiveCustomer(t *testing.T) {
	customer := setupOptionalCustomerTest(t)
	token := validOptionalCustomerToken(t, customer)
	require.NoError(t, database.GetDB().Model(&database.Customer{}).Where("id = ?", customer.ID).Update("is_active", false).Error)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.AddCookie(&http.Cookie{Name: "customer_token", Value: token})

	resolved, ok := OptionalCustomerFromRequest(c)

	require.False(t, ok)
	require.Nil(t, resolved)
}

func TestOptionalCustomerFromRequestRejectsRevokedSession(t *testing.T) {
	customer := setupOptionalCustomerTest(t)
	token := validOptionalCustomerToken(t, customer)
	require.NoError(t, session.GlobalStore.RevokeByTokenHashWithReason(session.HashToken(token), session.RevocationReasonUnspecified))

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.AddCookie(&http.Cookie{Name: "customer_token", Value: token})

	resolved, ok := OptionalCustomerFromRequest(c)

	require.False(t, ok)
	require.Nil(t, resolved)
}

func setupServerTestDB(t testing.TB) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.AutoMigrate(
		&session.UserSession{},
		&database.Customer{},
		&database.Business{},
		&database.CustomerBusiness{},
		&database.Table{},
		&database.Bill{},
	))

	database.SetTestDB(db)
	session.GlobalStore = session.NewStore(db)
	structs.SecretKey = []byte("test-secret-key-for-customer-table-check-in")
	gin.SetMode(gin.TestMode)

	t.Cleanup(func() {
		session.GlobalStore = nil
		require.NoError(t, sqlDB.Close())
	})

	return db
}

func setupServerTestDBWithLogger(t testing.TB, gormLogger logger.Interface) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: gormLogger})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	require.NoError(t, db.AutoMigrate(
		&session.UserSession{},
		&database.Customer{},
		&database.Business{},
		&database.CustomerBusiness{},
		&database.Table{},
		&database.Bill{},
	))

	database.SetTestDB(db)
	session.GlobalStore = session.NewStore(db)
	structs.SecretKey = []byte("test-secret-key-for-customer-table-check-in")
	gin.SetMode(gin.TestMode)

	t.Cleanup(func() {
		session.GlobalStore = nil
		require.NoError(t, sqlDB.Close())
	})

	return db
}

func createTestCustomer(t testing.TB, db *gorm.DB, email string) *database.Customer {
	t.Helper()

	customer := &database.Customer{
		Email:    email,
		Name:     "Test Customer",
		IsActive: true,
	}
	require.NoError(t, db.Create(customer).Error)
	return customer
}

func createTestBusiness(t testing.TB, db *gorm.DB) *database.Business {
	t.Helper()

	business := &database.Business{
		BusinessId:      fmt.Sprintf("checkin-business-%d", time.Now().UnixNano()),
		OwnerAddress:    "0x1111111111111111111111111111111111111111",
		Name:            "Check-in Business",
		SettlementAddr:  "0x2222222222222222222222222222222222222222",
		TippingAddr:     "0x3333333333333333333333333333333333333333",
		IsActive:        true,
		DefaultCurrency: "USD",
		DefaultLanguage: "en",
		SourceLanguage:  "en",
	}
	require.NoError(t, db.Create(business).Error)
	return business
}

func createTestTable(t testing.TB, db *gorm.DB, businessID uint, code string) *database.Table {
	t.Helper()

	table := &database.Table{
		BusinessID: businessID,
		TableCode:  code,
		Name:       code,
		Capacity:   4,
		IsActive:   true,
	}
	require.NoError(t, db.Create(table).Error)
	return table
}

func createOpenBillForTable(t testing.TB, db *gorm.DB, businessID uint, tableID uint) *database.Bill {
	t.Helper()

	bill := &database.Bill{
		BusinessID:     businessID,
		TableID:        tableID,
		BillNumber:     fmt.Sprintf("CHECKIN-%d", time.Now().UnixNano()),
		Items:          "[]",
		Status:         database.BillStatusOpen,
		SettlementAddr: "0x2222222222222222222222222222222222222222",
		TippingAddr:    "0x3333333333333333333333333333333333333333",
	}
	require.NoError(t, db.Create(bill).Error)
	return bill
}

func createCustomerTokenForTest(t testing.TB, customerID uint) string {
	t.Helper()

	tokenString := fmt.Sprintf("customer-checkin-token-%d", customerID)
	sess, err := session.GlobalStore.Create(session.CreateInput{
		UserID:    &customerID,
		TokenHash: session.HashToken(tokenString),
		Provider:  "customer",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	token, err := GenerateCustomerToken(customerID, fmt.Sprintf("customer-%d@example.com", customerID), sess.ID)
	require.NoError(t, err)
	require.NoError(t, session.GlobalStore.UpdateTokenHash(sess.ID, session.HashToken(token)))
	return token
}

func TestCustomerTableCheckInConnectsCustomerAndAttachesOpenBill(t *testing.T) {
	db := setupServerTestDB(t)
	customer := createTestCustomer(t, db, "diner@example.com")
	business := createTestBusiness(t, db)
	table := createTestTable(t, db, business.ID, "T-CHECKIN-1")
	bill := createOpenBillForTable(t, db, business.ID, table.ID)

	router := gin.New()
	router.POST("/customer/table/:code/check-in", CustomerAuthenticationMiddleware(), CheckInCustomerToTable)

	req := httptest.NewRequest(http.MethodPost, "/customer/table/T-CHECKIN-1/check-in", nil)
	req.AddCookie(&http.Cookie{Name: "customer_token", Value: createCustomerTokenForTest(t, customer.ID)})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var updated database.Bill
	require.NoError(t, db.First(&updated, bill.ID).Error)
	require.NotNil(t, updated.CRMCustomerID)
	require.Equal(t, customer.ID, *updated.CRMCustomerID)

	var connection database.CustomerBusiness
	require.NoError(t, db.Where("customer_id = ? AND business_id = ?", customer.ID, business.ID).First(&connection).Error)
	require.True(t, connection.IsActive)
}

func TestCustomerTableCheckInUsesNarrowLookupShape(t *testing.T) {
	recorder := &publicGuestSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	db := setupServerTestDBWithLogger(t, recorder)
	customer := createTestCustomer(t, db, "shape-diner@example.com")
	business := createTestBusiness(t, db)
	table := createTestTable(t, db, business.ID, "T-CHECKIN-SHAPE")
	createOpenBillForTable(t, db, business.ID, table.ID)

	router := gin.New()
	router.POST("/customer/table/:code/check-in", CustomerAuthenticationMiddleware(), CheckInCustomerToTable)

	recorder.statements = nil
	req := httptest.NewRequest(http.MethodPost, "/customer/table/T-CHECKIN-SHAPE/check-in", nil)
	req.AddCookie(&http.Cookie{Name: "customer_token", Value: createCustomerTokenForTest(t, customer.ID)})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Zero(t, recorder.selectCount("businesses"), "check-in should reuse the active business loaded with table context")
	require.Zero(t, recorder.selectStarCount("bills"), "check-in should attach CRM ownership without hydrating full bill rows")
}

func TestCustomerTableCheckInReactivatesInactiveConnection(t *testing.T) {
	db := setupServerTestDB(t)
	customer := createTestCustomer(t, db, "reactivated-diner@example.com")
	business := createTestBusiness(t, db)
	table := createTestTable(t, db, business.ID, "T-CHECKIN-REACTIVATE")
	bill := createOpenBillForTable(t, db, business.ID, table.ID)
	connection := &database.CustomerBusiness{
		CustomerID:     customer.ID,
		BusinessID:     business.ID,
		FirstVisitAt:   time.Now().Add(-14 * 24 * time.Hour),
		LoyaltyPoints:  75,
		TotalSpent:     82.25,
		VisitCount:     3,
		OptInMarketing: false,
		OptInEmail:     false,
		IsActive:       false,
	}
	require.NoError(t, db.Create(connection).Error)
	require.NoError(t, db.Model(&database.CustomerBusiness{}).Where("id = ?", connection.ID).Updates(map[string]interface{}{
		"is_active":        false,
		"opt_in_marketing": false,
		"opt_in_email":     false,
	}).Error)

	router := gin.New()
	router.POST("/customer/table/:code/check-in", CustomerAuthenticationMiddleware(), CheckInCustomerToTable)

	req := httptest.NewRequest(http.MethodPost, "/customer/table/T-CHECKIN-REACTIVATE/check-in", nil)
	req.AddCookie(&http.Cookie{Name: "customer_token", Value: createCustomerTokenForTest(t, customer.ID)})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var updatedBill database.Bill
	require.NoError(t, db.First(&updatedBill, bill.ID).Error)
	require.NotNil(t, updatedBill.CRMCustomerID)
	require.Equal(t, customer.ID, *updatedBill.CRMCustomerID)

	var reactivated database.CustomerBusiness
	require.NoError(t, db.First(&reactivated, connection.ID).Error)
	require.True(t, reactivated.IsActive)
	require.True(t, reactivated.OptInMarketing)
	require.True(t, reactivated.OptInEmail)
	require.Equal(t, 75, reactivated.LoyaltyPoints)
	require.InDelta(t, 82.25, reactivated.TotalSpent, 0.001)
	require.Equal(t, 3, reactivated.VisitCount)
}

func TestCustomerTableCheckInRejectsDifferentClaimedCustomer(t *testing.T) {
	db := setupServerTestDB(t)
	customerA := createTestCustomer(t, db, "first@example.com")
	customerB := createTestCustomer(t, db, "second@example.com")
	business := createTestBusiness(t, db)
	table := createTestTable(t, db, business.ID, "T-CHECKIN-CONFLICT")
	bill := createOpenBillForTable(t, db, business.ID, table.ID)
	require.NoError(t, db.Model(&database.Bill{}).Where("id = ?", bill.ID).Update("crm_customer_id", customerA.ID).Error)

	router := gin.New()
	router.POST("/customer/table/:code/check-in", CustomerAuthenticationMiddleware(), CheckInCustomerToTable)

	req := httptest.NewRequest(http.MethodPost, "/customer/table/T-CHECKIN-CONFLICT/check-in", nil)
	req.AddCookie(&http.Cookie{Name: "customer_token", Value: createCustomerTokenForTest(t, customerB.ID)})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusConflict, w.Code)
	require.Contains(t, w.Body.String(), "bill_customer_conflict")

	var updated database.Bill
	require.NoError(t, db.First(&updated, bill.ID).Error)
	require.NotNil(t, updated.CRMCustomerID)
	require.Equal(t, customerA.ID, *updated.CRMCustomerID)
}

func TestCustomerTableCheckInConflictDoesNotConnectLosingCustomer(t *testing.T) {
	db := setupServerTestDB(t)
	customerA := createTestCustomer(t, db, "winner@example.com")
	customerB := createTestCustomer(t, db, "loser@example.com")
	business := createTestBusiness(t, db)
	table := createTestTable(t, db, business.ID, "T-CHECKIN-NO-CONNECTION")
	bill := createOpenBillForTable(t, db, business.ID, table.ID)
	require.NoError(t, db.Model(&database.Bill{}).Where("id = ?", bill.ID).Update("crm_customer_id", customerA.ID).Error)

	router := gin.New()
	router.POST("/customer/table/:code/check-in", CustomerAuthenticationMiddleware(), CheckInCustomerToTable)

	req := httptest.NewRequest(http.MethodPost, "/customer/table/T-CHECKIN-NO-CONNECTION/check-in", nil)
	req.AddCookie(&http.Cookie{Name: "customer_token", Value: createCustomerTokenForTest(t, customerB.ID)})
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusConflict, w.Code)
	require.Contains(t, w.Body.String(), "bill_customer_conflict")

	var count int64
	require.NoError(t, db.Model(&database.CustomerBusiness{}).
		Where("customer_id = ? AND business_id = ?", customerB.ID, business.ID).
		Count(&count).Error)
	require.Zero(t, count)
}

func BenchmarkCustomerTableCheckInSQLite(b *testing.B) {
	db := setupServerTestDB(b)
	seed := time.Now().UnixNano()
	customer := createTestCustomer(b, db, fmt.Sprintf("bench-checkin-%d@example.com", seed))
	business := createTestBusiness(b, db)
	tableCode := fmt.Sprintf("T-CHECKIN-BENCH-%d", seed)
	table := createTestTable(b, db, business.ID, tableCode)
	bill := createOpenBillForTable(b, db, business.ID, table.ID)
	require.NoError(b, db.Model(&database.Bill{}).Where("id = ?", bill.ID).Update("crm_customer_id", customer.ID).Error)

	connection := &database.CustomerBusiness{
		CustomerID:     customer.ID,
		BusinessID:     business.ID,
		FirstVisitAt:   time.Now().Add(-24 * time.Hour),
		OptInMarketing: true,
		OptInEmail:     true,
		IsActive:       true,
	}
	require.NoError(b, db.Create(connection).Error)

	router := gin.New()
	router.POST("/customer/table/:code/check-in", CustomerAuthenticationMiddleware(), CheckInCustomerToTable)
	token := createCustomerTokenForTest(b, customer.ID)
	path := fmt.Sprintf("/customer/table/%s/check-in", tableCode)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.AddCookie(&http.Cookie{Name: "customer_token", Value: token})
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}
	}
}
