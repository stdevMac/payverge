package crm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// TestCustomerConsentsToShare_FailClosed is the privacy default: missing
// customer or preferences must NOT grant sharing (fail-closed).
func TestCustomerConsentsToShare_FailClosed(t *testing.T) {
	assert.False(t, customerConsentsToShare(nil), "nil customer must not consent")
	assert.False(t, customerConsentsToShare(&database.Customer{}), "nil Preferences must not consent")
	assert.False(t, customerConsentsToShare(&database.Customer{Preferences: nil}), "explicit nil Preferences must not consent")
	assert.False(t, customerConsentsToShare(&database.Customer{
		Preferences: &database.CustomerPreferences{ShareDataWithBusinesses: false},
	}))
	assert.True(t, customerConsentsToShare(&database.Customer{
		Preferences: &database.CustomerPreferences{ShareDataWithBusinesses: true},
	}), "explicit opt-in still grants consent")
}

func TestSanitizeCustomerForOperator_RedactsWhenPreferencesNil(t *testing.T) {
	birthday := time.Date(1990, 3, 14, 0, 0, 0, 0, time.UTC)
	lastLogin := time.Now().UTC()
	cb := &database.CustomerBusiness{
		Customer: database.Customer{
			Email:           "no-prefs@example.com",
			Name:            "No Prefs",
			Phone:           "+54 11 5555 0001",
			WalletAddress:   "0xDeadBeefWallet",
			Birthday:        &birthday,
			ProfileImageURL: "https://cdn.example/avatar.png",
			LastLoginAt:     &lastLogin,
			Preferences:     nil,
		},
	}

	sanitizeCustomerForOperator(cb)

	assert.Equal(t, "no-prefs@example.com", cb.Customer.Email, "email stays (linkage key)")
	assert.Equal(t, "No Prefs", cb.Customer.Name, "name stays (linkage key)")
	assert.Empty(t, cb.Customer.Phone, "phone redacted when prefs missing")
	assert.Empty(t, cb.Customer.WalletAddress, "wallet redacted when prefs missing")
	assert.Nil(t, cb.Customer.Birthday, "birthday redacted when prefs missing")
	assert.Empty(t, cb.Customer.ProfileImageURL)
	assert.Nil(t, cb.Customer.LastLoginAt)
	assert.Nil(t, cb.Customer.Preferences, "Preferences always stripped for operators")
}

func TestSanitizeCustomerForOperator_RedactsWhenOptedOut(t *testing.T) {
	birthday := time.Date(1990, 3, 14, 0, 0, 0, 0, time.UTC)
	cb := &database.CustomerBusiness{
		Customer: database.Customer{
			Email:         "optout-unit@example.com",
			Name:          "Opt Out",
			Phone:         "+1 555 0100",
			WalletAddress: "0xWallet",
			Birthday:      &birthday,
			Preferences:   &database.CustomerPreferences{ShareDataWithBusinesses: false},
		},
	}
	sanitizeCustomerForOperator(cb)
	assert.Empty(t, cb.Customer.Phone)
	assert.Nil(t, cb.Customer.Birthday)
	assert.Empty(t, cb.Customer.WalletAddress)
	assert.Nil(t, cb.Customer.Preferences)
	assert.Equal(t, "optout-unit@example.com", cb.Customer.Email)
}

// richOptedOutCustomer creates a customer with full PII collected at business A,
// consent (ShareDataWithBusinesses) set to false, and a second connection at
// business B. Returns (customer, businessA, businessB).
func richOptedOutCustomer(t *testing.T, svc *Service) (*database.Customer, *database.Business, *database.Business) {
	t.Helper()
	db := svc.db
	bizA := createCRMTestBusiness(t, db, "consent-biz-a", "0xConsentOwnerA", "Consent Biz A")
	bizB := createCRMTestBusiness(t, db, "consent-biz-b", "0xConsentOwnerB", "Consent Biz B")
	customer := createCRMTestCustomer(t, db, "optout@example.com")

	birthday := time.Date(1990, 3, 14, 0, 0, 0, 0, time.UTC)
	require.NoError(t, db.Model(&database.Customer{}).Where("id = ?", customer.ID).Updates(map[string]interface{}{
		"phone":          "+54 11 5555 0001",
		"wallet_address": "0xDeadBeefWallet",
		"birthday":       birthday,
	}).Error)
	// ShareDataWithBusinesses has gorm:"default:true", so a struct Create with a
	// false (zero-value) bool would be overwritten to true on insert. Production
	// opt-outs are written via a map update (crm/handlers.go), which does persist
	// false — mirror that here so the fixture actually represents a consent=false
	// customer.
	require.NoError(t, db.Create(&database.CustomerPreferences{
		CustomerID:        customer.ID,
		PreferredLanguage: "es",
	}).Error)
	require.NoError(t, db.Model(&database.CustomerPreferences{}).
		Where("customer_id = ?", customer.ID).
		Update("share_data_with_businesses", false).Error)

	for _, biz := range []*database.Business{bizA, bizB} {
		require.NoError(t, db.Create(&database.CustomerBusiness{
			CustomerID:   customer.ID,
			BusinessID:   biz.ID,
			FirstVisitAt: time.Now(),
			IsActive:     true,
		}).Error)
	}
	return customer, bizA, bizB
}

func TestGetBusinessCustomers_RedactsPIIWhenConsentFalse(t *testing.T) {
	db := setupCRMHandlerTestDB(t)
	svc := NewService(db)
	_, _, bizB := richOptedOutCustomer(t, svc)

	customers, total, err := svc.GetBusinessCustomers(bizB.ID, 1, 20, "", "")
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, customers, 1)

	got := customers[0].Customer
	assert.Equal(t, "optout@example.com", got.Email, "email stays (linkage key the operator already holds)")
	assert.Empty(t, got.Phone, "phone must be redacted when consent=false")
	assert.Nil(t, got.Birthday, "birthday must be redacted when consent=false")
	assert.Empty(t, got.WalletAddress, "wallet must be redacted when consent=false")
	assert.Empty(t, got.ProfileImageURL)
	assert.Nil(t, got.LastLoginAt)
	assert.Nil(t, got.Preferences, "operators never receive Customer.Preferences")
}

func TestGetBusinessCustomers_PreferencesNeverReturnedEvenWithConsent(t *testing.T) {
	db := setupCRMHandlerTestDB(t)
	svc := NewService(db)
	biz := createCRMTestBusiness(t, db, "consent-biz-c", "0xConsentOwnerC", "Consent Biz C")
	customer := createCRMTestCustomer(t, db, "consenting@example.com")
	require.NoError(t, db.Create(&database.CustomerPreferences{
		CustomerID:              customer.ID,
		ShareDataWithBusinesses: true,
	}).Error)
	require.NoError(t, db.Create(&database.CustomerBusiness{
		CustomerID: customer.ID, BusinessID: biz.ID, FirstVisitAt: time.Now(), IsActive: true,
	}).Error)

	customers, _, err := svc.GetBusinessCustomers(biz.ID, 1, 20, "", "")
	require.NoError(t, err)
	require.Len(t, customers, 1)
	assert.Nil(t, customers[0].Customer.Preferences)
	assert.Equal(t, "consenting@example.com", customers[0].Customer.Email)
}

func TestGetCustomerBusinessDetails_RedactsPIIWhenConsentFalse(t *testing.T) {
	db := setupCRMHandlerTestDB(t)
	svc := NewService(db)
	customer, _, bizB := richOptedOutCustomer(t, svc)

	var cbB database.CustomerBusiness
	require.NoError(t, db.Where("customer_id = ? AND business_id = ?", customer.ID, bizB.ID).First(&cbB).Error)

	details, err := svc.GetCustomerBusinessDetailsForBusiness(bizB.ID, cbB.ID)
	require.NoError(t, err)
	assert.Empty(t, details.Customer.Phone)
	assert.Nil(t, details.Customer.Birthday)
	assert.Empty(t, details.Customer.WalletAddress)
	assert.Nil(t, details.Customer.Preferences, "detail drawer must not receive guest-global preferences")
	assert.Equal(t, "optout@example.com", details.Customer.Email)
}

func TestAddCustomer_DoesNotExposeExistingPIIWhenConsentFalse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	svc := NewService(db)
	handler := NewHandler(svc)

	// Business A collected the rich profile; consent is off; business B now
	// adds the same email manually.
	_, _, bizB := richOptedOutCustomer(t, svc)
	// Remove B's pre-existing connection so this is a genuine first link.
	require.NoError(t, db.Where("business_id = ?", bizB.ID).Delete(&database.CustomerBusiness{}).Error)

	body := bytes.NewBufferString(`{"email":"optout@example.com","name":"Operator Entered Name","phone":"+1 555 000 9999"}`)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", body)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: bizB.BusinessId}}

	handler.AddCustomer(c)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "+54 11 5555 0001", "existing global phone must not leak")
	assert.NotContains(t, w.Body.String(), "0xDeadBeefWallet", "existing wallet must not leak")
	assert.NotContains(t, w.Body.String(), "share_data_with_businesses", "preferences must not leak")

	// Positive shape assertions: the consent block in AddCustomer rebuilds the
	// embedded customer deterministically from the operator-supplied request.
	// These hard-fail if the sanitization block is ever removed — the raw
	// connection from EnsureActiveCustomerBusinessConnection carries a
	// zero-value Customer (empty email/name).
	var resp struct {
		Connection struct {
			Customer struct {
				Email string `json:"email"`
				Name  string `json:"name"`
			} `json:"customer"`
		} `json:"connection"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "optout@example.com", resp.Connection.Customer.Email, "response must echo the operator-supplied email")
	assert.Equal(t, "Operator Entered Name", resp.Connection.Customer.Name, "response must echo the operator-supplied name, not the global profile")

	// The link itself is created (the operator legitimately holds the email).
	var count int64
	require.NoError(t, db.Model(&database.CustomerBusiness{}).Where("business_id = ?", bizB.ID).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestExportBusinessCustomers_RedactsPhoneWhenConsentFalse(t *testing.T) {
	db := setupCRMHandlerTestDB(t)
	svc := NewService(db)
	_, _, bizB := richOptedOutCustomer(t, svc)

	var exports []CustomerExportRow
	err := svc.ExportBusinessCustomers(bizB.ID, func(row CustomerExportRow) error {
		exports = append(exports, row)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, exports, 1)
	assert.Equal(t, "optout@example.com", exports[0].Email)
	assert.Equal(t, "", exports[0].Phone, "opted-out phone must not reach the operator CSV")
}

func TestAddCustomer_NewCustomerDefaultsShareFalse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	svc := NewService(db)
	handler := NewHandler(svc)
	biz := createCRMTestBusiness(t, db, "add-optin-biz", "0xAddOptIn", "Add OptIn Biz")

	body := bytes.NewBufferString(`{"email":"new-guest@example.com","name":"New Guest","phone":"+1 555 111 2222"}`)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", body)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: biz.BusinessId}}

	handler.AddCustomer(c)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var customer database.Customer
	require.NoError(t, db.Where("email = ?", "new-guest@example.com").First(&customer).Error)
	var prefs database.CustomerPreferences
	require.NoError(t, db.Where("customer_id = ?", customer.ID).First(&prefs).Error)
	assert.False(t, prefs.ShareDataWithBusinesses, "operator-created customers default sharing OFF")

	// New customer has not opted in → response must not include phone as shared PII.
	assert.NotContains(t, w.Body.String(), "share_data_with_businesses")

	// L5-4: brand-new global customer → created:true discriminator.
	var newResp struct {
		Created bool `json:"created"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &newResp))
	assert.True(t, newResp.Created, "new customer path must set created=true")
}

// L5-4: linking an existing email must not claim created=true (toast honesty).
func TestAddCustomer_ExistingEmailReturnsCreatedFalse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	svc := NewService(db)
	handler := NewHandler(svc)

	_, _, bizB := richOptedOutCustomer(t, svc)
	require.NoError(t, db.Where("business_id = ?", bizB.ID).Delete(&database.CustomerBusiness{}).Error)

	body := bytes.NewBufferString(`{"email":"optout@example.com","name":"Operator Entered Name","phone":"+1 555 000 9999"}`)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", body)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: bizB.BusinessId}}

	handler.AddCustomer(c)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp struct {
		Created bool `json:"created"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.False(t, resp.Created, "existing email link must set created=false")
}

func TestConcurrent_AddCustomer_SameEmailTenantSafe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCRMHandlerTestDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// SQLite shared-cache in-memory databases can fail both concurrent writers
	// with table locks while upgrading read transactions to writes. Keep this
	// handler-level race test on one connection; Postgres concurrency is covered
	// by the real database-backed gates.
	sqlDB.SetMaxOpenConns(1)

	svc := NewService(db)
	handler := NewHandler(svc)
	bizA := createCRMTestBusiness(t, db, "race-biz-a", "0xRaceOwnerA", "Race Biz A")
	bizB := createCRMTestBusiness(t, db, "race-biz-b", "0xRaceOwnerB", "Race Biz B")

	const email = "concurrent-add@example.com"
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	wg.Add(2)
	for _, biz := range []*database.Business{bizA, bizB} {
		biz := biz
		go func() {
			defer wg.Done()
			body := bytes.NewBufferString(fmt.Sprintf(
				`{"email":%q,"name":"Concurrent Guest","phone":"+1 555 000 1111"}`, email,
			))
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/", body)
			c.Request.Header.Set("Content-Type", "application/json")
			c.Params = gin.Params{{Key: "id", Value: biz.BusinessId}}
			handler.AddCustomer(c)
			codes <- w.Code
		}()
	}
	wg.Wait()
	close(codes)

	for code := range codes {
		require.Equal(t, http.StatusCreated, code, "both concurrent AddCustomer calls must succeed (race re-loads winner)")
	}

	var customers int64
	require.NoError(t, db.Model(&database.Customer{}).Where("email = ?", email).Count(&customers).Error)
	assert.EqualValues(t, 1, customers, "exactly one customer row")

	var prefs int64
	require.NoError(t, db.Model(&database.CustomerPreferences{}).
		Joins("JOIN customers ON customers.id = customer_preferences.customer_id").
		Where("customers.email = ?", email).
		Count(&prefs).Error)
	assert.EqualValues(t, 1, prefs, "exactly one preferences row")

	var customer database.Customer
	require.NoError(t, db.Where("email = ?", email).First(&customer).Error)

	// Each business gets exactly one tenant-scoped connection; no duplicates/leaks.
	var connA, connB int64
	require.NoError(t, db.Model(&database.CustomerBusiness{}).
		Where("customer_id = ? AND business_id = ?", customer.ID, bizA.ID).Count(&connA).Error)
	require.NoError(t, db.Model(&database.CustomerBusiness{}).
		Where("customer_id = ? AND business_id = ?", customer.ID, bizB.ID).Count(&connB).Error)
	assert.EqualValues(t, 1, connA, "biz A must have exactly one connection")
	assert.EqualValues(t, 1, connB, "biz B must have exactly one connection")

	var foreign int64
	require.NoError(t, db.Model(&database.CustomerBusiness{}).
		Where("customer_id = ? AND business_id NOT IN ?", customer.ID, []uint{bizA.ID, bizB.ID}).
		Count(&foreign).Error)
	assert.EqualValues(t, 0, foreign, "no leaked connections outside the two tenants")
}

func TestGetBusinessCustomers_ConsentLoadIsNotNPlusOne(t *testing.T) {
	db := setupCRMHandlerTestDB(t)
	recorder := &lcrmSQLRecorder{}
	db.Logger = recorder
	svc := NewService(db)
	biz := createCRMTestBusiness(t, db, "consent-biz-shape", "0xConsentShape", "Consent Shape Biz")
	for i := 0; i < 5; i++ {
		customer := createCRMTestCustomer(t, db, fmt.Sprintf("shape-%d@example.com", i))
		require.NoError(t, db.Create(&database.CustomerBusiness{
			CustomerID: customer.ID, BusinessID: biz.ID, FirstVisitAt: time.Now(), IsActive: true,
		}).Error)
	}

	recorder.reset()
	_, _, err := svc.GetBusinessCustomers(biz.ID, 1, 20, "", "")
	require.NoError(t, err)

	prefSelects := recorder.countMatching(func(s string) bool {
		return strings.HasPrefix(s, "select") && strings.Contains(s, "customer_preferences")
	})
	assert.LessOrEqual(t, prefSelects, 1, "consent must load via a single batched preload, not per-row queries")
}
