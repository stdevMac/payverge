package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/logger"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupEmailReceiptTest(t *testing.T) (*gorm.DB, *PaymentHandler) {
	t.Helper()
	// Other handler tests in this package may leave a global mailer. These
	// tests drive EmailBillReceipt through the sendGuestReceiptEmail seam
	// only — clear the process-wide instance so a missed seam cannot send.
	originalMailer := emails.EmailServerInstance
	emails.EmailServerInstance = nil
	t.Cleanup(func() { emails.EmailServerInstance = originalMailer })
	// Unique DSN per test to avoid cross-test shared-memory collisions under -race.
	dsn := "file:email_receipt_test_" + t.Name() + "?mode=memory&cache=shared"
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	// BillItem uses gen_random_uuid() which SQLite rejects; the handler treats
	// a missing items table as an empty list via GetBillItems error path.
	require.NoError(t, gormDB.Migrator().DropTable(
		&database.Bill{}, &database.Business{}, &database.GuestReceiptSend{},
	))
	require.NoError(t, gormDB.AutoMigrate(
		&database.Bill{}, &database.Business{}, &database.GuestReceiptSend{},
	))
	database.SetTestDB(gormDB)
	return gormDB, NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
}

func emailReceiptRouter(handler *PaymentHandler) *gin.Engine {
	router := gin.New()
	router.POST("/guest/bill/:bill_token/email-receipt", handler.EmailBillReceipt)
	return router
}

func postEmailReceipt(router *gin.Engine, token string, body map[string]any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/guest/bill/"+token+"/email-receipt", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	return w
}

func emailPtr(s string) *string { return &s }

func stubGuestReceiptMailer(t *testing.T) *int {
	t.Helper()
	var sendCount int
	original := sendGuestReceiptEmail
	sendGuestReceiptEmail = func([]string, string, string, string, string, []map[string]interface{}, string, string, uint, uint) error {
		sendCount++
		return nil
	}
	t.Cleanup(func() { sendGuestReceiptEmail = original })
	return &sendCount
}

func TestEmailBillReceiptSendsForSettledBill(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupEmailReceiptTest(t)

	require.NoError(t, db.Create(&database.Business{
		ID: 9, BusinessId: "biz-9", Name: "Test Bistro", OwnerAddress: "0xown",
		SettlementAddr: "0x1", TippingAddr: "0x2", DefaultLanguage: "en",
	}).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID: 5, BusinessID: 9, BillNumber: "B-100", PublicToken: "tok-100",
		Status: database.BillStatusPaid, TotalAmount: 2550,
		SettlementAddr: "0x1", TippingAddr: "0x2",
		FiscalCustomerEmail: emailPtr("guest@example.com"),
	}).Error)

	var sentTo []string
	var sentTotal string
	original := sendGuestReceiptEmail
	sendGuestReceiptEmail = func(to []string, businessName, paymentDate, paymentMethod, transactionID string, items []map[string]interface{}, totalAmount, language string, _, _ uint) error {
		sentTo = to
		sentTotal = totalAmount
		return nil
	}
	t.Cleanup(func() { sendGuestReceiptEmail = original })

	w := postEmailReceipt(emailReceiptRouter(handler), "tok-100", map[string]any{
		"email": "guest@example.com",
	})

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, []string{"guest@example.com"}, sentTo)
	assert.NotEmpty(t, sentTotal, "total must be formatted from DB cents")

	var recorded database.GuestReceiptSend
	require.NoError(t, db.First(&recorded).Error)
	assert.Equal(t, uint(5), recorded.BillID)
	assert.Equal(t, logger.RedactEmail("guest@example.com"), recorded.RecipientRedacted)
	assert.NotContains(t, recorded.RecipientRedacted, "guest@example.com")
}

func TestEmailBillReceiptRejectsUnsettledAndBadInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupEmailReceiptTest(t)
	require.NoError(t, db.Create(&database.Business{
		ID: 9, BusinessId: "biz-9", Name: "Test Bistro", OwnerAddress: "0xown",
		SettlementAddr: "0x1", TippingAddr: "0x2",
	}).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID: 6, BusinessID: 9, BillNumber: "B-101", PublicToken: "tok-101",
		Status: database.BillStatusOpen, TotalAmount: 1000,
		SettlementAddr: "0x1", TippingAddr: "0x2",
	}).Error)

	original := sendGuestReceiptEmail
	sendGuestReceiptEmail = func([]string, string, string, string, string, []map[string]interface{}, string, string, uint, uint) error {
		return errors.New("must not be called")
	}
	t.Cleanup(func() { sendGuestReceiptEmail = original })

	router := emailReceiptRouter(handler)

	openBill := postEmailReceipt(router, "tok-101", map[string]any{"email": "guest@example.com"})
	require.Equal(t, http.StatusConflict, openBill.Code)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(openBill.Body.Bytes(), &payload))
	assert.Equal(t, "bill_not_settled", payload["code"])

	badEmail := postEmailReceipt(router, "tok-101", map[string]any{"email": "not-an-email"})
	require.Equal(t, http.StatusBadRequest, badEmail.Code)

	missingBill := postEmailReceipt(router, "tok-nope", map[string]any{"email": "guest@example.com"})
	require.Equal(t, http.StatusNotFound, missingBill.Code)
}

func TestEmailBillReceiptCapsFourthSendIn24h(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupEmailReceiptTest(t)
	require.NoError(t, db.Create(&database.Business{
		ID: 9, BusinessId: "biz-9", Name: "Test Bistro", OwnerAddress: "0xown",
		SettlementAddr: "0x1", TippingAddr: "0x2", DefaultLanguage: "en",
	}).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID: 7, BusinessID: 9, BillNumber: "B-102", PublicToken: "tok-102",
		Status: database.BillStatusPaid, TotalAmount: 1800,
		SettlementAddr: "0x1", TippingAddr: "0x2",
		FiscalCustomerEmail: emailPtr("guest@example.com"),
	}).Error)

	sendCount := stubGuestReceiptMailer(t)

	router := emailReceiptRouter(handler)
	for i := 0; i < 3; i++ {
		w := postEmailReceipt(router, "tok-102", map[string]any{
			"email": "guest@example.com",
		})
		require.Equal(t, http.StatusOK, w.Code, "send %d: %s", i+1, w.Body.String())
	}
	require.Equal(t, 3, *sendCount)

	fourth := postEmailReceipt(router, "tok-102", map[string]any{"email": "guest@example.com"})
	require.Equal(t, http.StatusTooManyRequests, fourth.Code, fourth.Body.String())
	var payload map[string]any
	require.NoError(t, json.Unmarshal(fourth.Body.Bytes(), &payload))
	assert.Equal(t, "receipt_send_limit", payload["code"])
	assert.Equal(t, 3, *sendCount, "fourth send must not call the mailer")

	var persisted int64
	require.NoError(t, db.Model(&database.GuestReceiptSend{}).Count(&persisted).Error)
	assert.Equal(t, int64(3), persisted)
}

func TestEmailBillReceiptFailedSendDoesNotConsumeQuota(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupEmailReceiptTest(t)
	require.NoError(t, db.Create(&database.Business{
		ID: 9, BusinessId: "biz-9", Name: "Test Bistro", OwnerAddress: "0xown",
		SettlementAddr: "0x1", TippingAddr: "0x2", DefaultLanguage: "en",
	}).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID: 8, BusinessID: 9, BillNumber: "B-103", PublicToken: "tok-103",
		Status: database.BillStatusClosed, TotalAmount: 900,
		SettlementAddr: "0x1", TippingAddr: "0x2",
		FiscalCustomerEmail: emailPtr("guest@example.com"),
	}).Error)

	var sendCount int
	original := sendGuestReceiptEmail
	sendGuestReceiptEmail = func([]string, string, string, string, string, []map[string]interface{}, string, string, uint, uint) error {
		sendCount++
		if sendCount == 1 {
			return errors.New("transport down")
		}
		return nil
	}
	t.Cleanup(func() { sendGuestReceiptEmail = original })

	router := emailReceiptRouter(handler)
	fail := postEmailReceipt(router, "tok-103", map[string]any{"email": "guest@example.com"})
	require.Equal(t, http.StatusServiceUnavailable, fail.Code)
	var persisted int64
	require.NoError(t, db.Model(&database.GuestReceiptSend{}).Count(&persisted).Error)
	assert.Equal(t, int64(0), persisted)

	ok := postEmailReceipt(router, "tok-103", map[string]any{"email": "guest@example.com"})
	require.Equal(t, http.StatusOK, ok.Code, ok.Body.String())
	require.Equal(t, 2, sendCount)
	require.NoError(t, db.Model(&database.GuestReceiptSend{}).Count(&persisted).Error)
	assert.Equal(t, int64(1), persisted)
}

func TestEmailBillReceiptAllowsSendAfter24hWindow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupEmailReceiptTest(t)
	require.NoError(t, db.Create(&database.Business{
		ID: 9, BusinessId: "biz-9", Name: "Test Bistro", OwnerAddress: "0xown",
		SettlementAddr: "0x1", TippingAddr: "0x2", DefaultLanguage: "en",
	}).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID: 10, BusinessID: 9, BillNumber: "B-104", PublicToken: "tok-104",
		Status: database.BillStatusPaid, TotalAmount: 500,
		SettlementAddr: "0x1", TippingAddr: "0x2",
		FiscalCustomerEmail: emailPtr("guest@example.com"),
	}).Error)
	old := time.Now().UTC().Add(-25 * time.Hour)
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&database.GuestReceiptSend{
			BillID:            10,
			SentAt:            old,
			RecipientRedacted: "g***@example.com",
		}).Error)
	}

	var sendCount int
	original := sendGuestReceiptEmail
	sendGuestReceiptEmail = func([]string, string, string, string, string, []map[string]interface{}, string, string, uint, uint) error {
		sendCount++
		return nil
	}
	t.Cleanup(func() { sendGuestReceiptEmail = original })

	w := postEmailReceipt(emailReceiptRouter(handler), "tok-104", map[string]any{
		"email": "guest@example.com",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 1, sendCount)
}

func assertUnboundReceiptRejected(t *testing.T, db *gorm.DB, w *httptest.ResponseRecorder, sendCount *int) {
	t.Helper()
	require.GreaterOrEqual(t, w.Code, 400, w.Body.String())
	require.Less(t, w.Code, 500, w.Body.String())
	require.NotEqual(t, http.StatusOK, w.Code)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.NotEqual(t, true, payload["success"])
	assert.Equal(t, "receipt_recipient_unbound", payload["code"])
	if sendCount != nil {
		assert.Equal(t, 0, *sendCount, "unbound recipient must not call the mailer")
	}
	var persisted int64
	require.NoError(t, db.Model(&database.GuestReceiptSend{}).Count(&persisted).Error)
	assert.Equal(t, int64(0), persisted, "unbound recipient must not consume a send slot")
}

func TestEmailBillReceiptRejectsUnboundRecipient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupEmailReceiptTest(t)
	require.NoError(t, db.Create(&database.Business{
		ID: 9, BusinessId: "biz-9", Name: "Test Bistro", OwnerAddress: "0xown",
		SettlementAddr: "0x1", TippingAddr: "0x2", DefaultLanguage: "en",
	}).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID: 11, BusinessID: 9, BillNumber: "B-105", PublicToken: "tok-105",
		Status: database.BillStatusPaid, TotalAmount: 1200,
		SettlementAddr: "0x1", TippingAddr: "0x2",
		FiscalCustomerEmail: emailPtr("guest@example.com"),
	}).Error)

	sendCount := stubGuestReceiptMailer(t)
	w := postEmailReceipt(emailReceiptRouter(handler), "tok-105", map[string]any{
		"email": "attacker@example.invalid",
	})
	assertUnboundReceiptRejected(t, db, w, sendCount)
}

func TestEmailBillReceiptRejectsWhenBillHasNoAssociatedEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupEmailReceiptTest(t)
	require.NoError(t, db.Create(&database.Business{
		ID: 9, BusinessId: "biz-9", Name: "Test Bistro", OwnerAddress: "0xown",
		SettlementAddr: "0x1", TippingAddr: "0x2", DefaultLanguage: "en",
	}).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID: 12, BusinessID: 9, BillNumber: "B-106", PublicToken: "tok-106",
		Status: database.BillStatusPaid, TotalAmount: 800,
		SettlementAddr: "0x1", TippingAddr: "0x2",
	}).Error)

	sendCount := stubGuestReceiptMailer(t)
	w := postEmailReceipt(emailReceiptRouter(handler), "tok-106", map[string]any{
		"email": "guest@example.com",
	})
	assertUnboundReceiptRejected(t, db, w, sendCount)
}

func TestEmailBillReceiptAllowsCRMCustomerEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupEmailReceiptTest(t)
	require.NoError(t, db.AutoMigrate(&database.Customer{}))
	require.NoError(t, db.Create(&database.Business{
		ID: 9, BusinessId: "biz-9", Name: "Test Bistro", OwnerAddress: "0xown",
		SettlementAddr: "0x1", TippingAddr: "0x2", DefaultLanguage: "en",
	}).Error)
	customer := database.Customer{Email: "crm-guest@example.com", PasswordHash: "x", Name: "CRM Guest", IsActive: true}
	require.NoError(t, db.Create(&customer).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID: 13, BusinessID: 9, BillNumber: "B-107", PublicToken: "tok-107",
		Status: database.BillStatusPaid, TotalAmount: 1500,
		SettlementAddr: "0x1", TippingAddr: "0x2",
		CRMCustomerID: &customer.ID,
	}).Error)

	var sentTo []string
	original := sendGuestReceiptEmail
	sendGuestReceiptEmail = func(to []string, _, _, _, _ string, _ []map[string]interface{}, _, _ string, _, _ uint) error {
		sentTo = to
		return nil
	}
	t.Cleanup(func() { sendGuestReceiptEmail = original })

	w := postEmailReceipt(emailReceiptRouter(handler), "tok-107", map[string]any{
		"email": "crm-guest@example.com",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, []string{"crm-guest@example.com"}, sentTo)

	unbound := postEmailReceipt(emailReceiptRouter(handler), "tok-107", map[string]any{
		"email": "attacker@example.invalid",
	})
	require.Equal(t, http.StatusForbidden, unbound.Code, unbound.Body.String())
	var persisted int64
	require.NoError(t, db.Model(&database.GuestReceiptSend{}).Count(&persisted).Error)
	assert.Equal(t, int64(1), persisted)
}

func TestEmailBillReceiptAllowsDeliveryOrderEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupEmailReceiptTest(t)
	require.NoError(t, db.Exec(`CREATE TABLE delivery_orders (
		id INTEGER PRIMARY KEY,
		bill_id INTEGER NOT NULL,
		customer_email TEXT
	)`).Error)
	require.NoError(t, db.Create(&database.Business{
		ID: 9, BusinessId: "biz-9", Name: "Test Bistro", OwnerAddress: "0xown",
		SettlementAddr: "0x1", TippingAddr: "0x2", DefaultLanguage: "en",
	}).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID: 14, BusinessID: 9, BillNumber: "B-108", PublicToken: "tok-108",
		Status: database.BillStatusPaid, TotalAmount: 2200,
		SettlementAddr: "0x1", TippingAddr: "0x2",
	}).Error)
	require.NoError(t, db.Exec(
		`INSERT INTO delivery_orders (bill_id, customer_email) VALUES (?, ?)`,
		14, "delivery-guest@example.com",
	).Error)

	var sentTo []string
	original := sendGuestReceiptEmail
	sendGuestReceiptEmail = func(to []string, _, _, _, _ string, _ []map[string]interface{}, _, _ string, _, _ uint) error {
		sentTo = to
		return nil
	}
	t.Cleanup(func() { sendGuestReceiptEmail = original })

	w := postEmailReceipt(emailReceiptRouter(handler), "tok-108", map[string]any{
		"email": "delivery-guest@example.com",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, []string{"delivery-guest@example.com"}, sentTo)
}

func TestEmailBillReceiptRecipientMatchIsCaseInsensitive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupEmailReceiptTest(t)
	require.NoError(t, db.Create(&database.Business{
		ID: 9, BusinessId: "biz-9", Name: "Test Bistro", OwnerAddress: "0xown",
		SettlementAddr: "0x1", TippingAddr: "0x2", DefaultLanguage: "en",
	}).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID: 15, BusinessID: 9, BillNumber: "B-109", PublicToken: "tok-109",
		Status: database.BillStatusPaid, TotalAmount: 600,
		SettlementAddr: "0x1", TippingAddr: "0x2",
		FiscalCustomerEmail: emailPtr("Guest@Example.com"),
	}).Error)

	sendCount := stubGuestReceiptMailer(t)
	w := postEmailReceipt(emailReceiptRouter(handler), "tok-109", map[string]any{
		"email": "guest@example.com",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 1, *sendCount)
}

func TestEmailBillReceiptUnboundDoesNotUseCap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupEmailReceiptTest(t)
	require.NoError(t, db.Create(&database.Business{
		ID: 9, BusinessId: "biz-9", Name: "Test Bistro", OwnerAddress: "0xown",
		SettlementAddr: "0x1", TippingAddr: "0x2", DefaultLanguage: "en",
	}).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID: 16, BusinessID: 9, BillNumber: "B-110", PublicToken: "tok-110",
		Status: database.BillStatusPaid, TotalAmount: 700,
		SettlementAddr: "0x1", TippingAddr: "0x2",
		FiscalCustomerEmail: emailPtr("guest@example.com"),
	}).Error)
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&database.GuestReceiptSend{
			BillID:            16,
			SentAt:            now,
			RecipientRedacted: "g***@example.com",
		}).Error)
	}

	sendCount := stubGuestReceiptMailer(t)
	w := postEmailReceipt(emailReceiptRouter(handler), "tok-110", map[string]any{
		"email": "attacker@example.invalid",
	})
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	var payload map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &payload))
	assert.Equal(t, "receipt_recipient_unbound", payload["code"])
	assert.NotEqual(t, "receipt_send_limit", payload["code"])
	assert.Equal(t, 0, *sendCount)
	var persisted int64
	require.NoError(t, db.Model(&database.GuestReceiptSend{}).Count(&persisted).Error)
	assert.Equal(t, int64(3), persisted)
}
