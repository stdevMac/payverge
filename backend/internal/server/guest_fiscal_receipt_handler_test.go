package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/runtimecontrol"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupGuestFiscalReceiptTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Table{},
		&database.Bill{},
		&database.Payment{},
		&database.FiscalReceipt{},
		&runtimecontrol.Control{},
	))
	return gormDB
}

func enableGuestFiscalRuntimeControl(t *testing.T) {
	t.Helper()
	require.NoError(t, database.GetDB().Save(&runtimecontrol.Control{
		Key:       runtimecontrol.ControlFiscal,
		Enabled:   true,
		Owner:     "guest-fiscal-test-owner",
		Reason:    "exercise the explicitly enabled guest fiscal path",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
		UpdatedBy: "guest-fiscal-test",
		UpdatedAt: time.Now().UTC(),
	}).Error)
}

func seedGuestFiscalBill(t *testing.T, token string) *database.Bill {
	t.Helper()
	return seedGuestFiscalBillFor(t, token, database.Business{
		Name: "Guest Fiscal Resto", OwnerAddress: "0xowner",
		Kind: database.BusinessKindReal,
	})
}

func seedGuestFiscalBillFor(t *testing.T, token string, business database.Business) *database.Bill {
	t.Helper()
	if business.Name == "" {
		business.Name = "Guest Fiscal Resto"
	}
	if business.OwnerAddress == "" {
		business.OwnerAddress = "0xowner"
	}
	if business.BusinessId == "" {
		business.BusinessId = "biz-" + token
	}
	require.NoError(t, database.GetDB().Create(&business).Error)
	bill := &database.Bill{
		BusinessID:     business.ID,
		BillNumber:     fmt.Sprintf("GF-%d", time.Now().UnixNano()),
		PublicToken:    token,
		TotalAmount:    121000,
		Status:         database.BillStatusPaid,
		SettlementAddr: "0xs",
		TippingAddr:    "0xt",
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	return bill
}

func seedAuthorizedGuestFiscalReceipt(t *testing.T, bill *database.Bill, number, cae, qr string, pdf string) {
	t.Helper()
	receipt := database.FiscalReceipt{
		BusinessID: bill.BusinessID, SettingsID: 1, BillID: bill.ID,
		Country: "AR", Provider: "arca", Action: "issue",
		ReceiptType: "factura_b", TotalAmountCents: 121000, Currency: "ARS",
		Status: database.FiscalStatusAuthorized,
	}
	if number != "" {
		receipt.ReceiptNumber = &number
	}
	if cae != "" {
		receipt.AuthCode = &cae
	}
	if qr != "" {
		receipt.QRPayload = &qr
	}
	if pdf != "" {
		receipt.PDFPath = &pdf
	}
	require.NoError(t, database.GetDB().Create(&receipt).Error)
}

func assertGuestFiscalReceiptNotAuthorized(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	body := strings.ToLower(w.Body.String())
	require.NotContains(t, body, `"status":"authorized"`)
	require.NotContains(t, body, "auth-")
	require.NotContains(t, body, "payverge.local")
	if w.Code == http.StatusOK {
		var parsed map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &parsed))
		assert.NotEqual(t, "authorized", parsed["status"])
		assert.NotContains(t, parsed, "auth_code")
		return
	}
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func guestFiscalReceiptRequest(t *testing.T, token string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/guest/bill/"+token+"/fiscal-receipt", nil)
	c.Params = gin.Params{{Key: "bill_token", Value: token}}
	return c, w
}

func TestGetGuestFiscalReceiptAuthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGuestFiscalReceiptTestDB(t)
	enableGuestFiscalRuntimeControl(t)
	bill := seedGuestFiscalBill(t, "tok-authorized-1")

	cae := "71234567890123"
	qr := "https://www.arca.gob.ar/fe/qr/?p=abc"
	seedAuthorizedGuestFiscalReceipt(t, bill, "00000042", cae, qr, "fiscal/1/receipt-1.pdf")

	c, w := guestFiscalReceiptRequest(t, "tok-authorized-1")
	GetGuestFiscalReceipt(c)
	require.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "authorized", body["status"])
	assert.Equal(t, "factura_b", body["receipt_type"])
	assert.Equal(t, "00000042", body["receipt_number"])
	assert.Equal(t, cae, body["auth_code"])
	assert.Equal(t, qr, body["qr_payload"])
	assert.Equal(t, true, body["pdf_available"])
}

func TestGetGuestFiscalReceiptPendingAndNone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGuestFiscalReceiptTestDB(t)
	enableGuestFiscalRuntimeControl(t)
	bill := seedGuestFiscalBill(t, "tok-pending-1")

	c, w := guestFiscalReceiptRequest(t, "tok-pending-1")
	GetGuestFiscalReceipt(c)
	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "none", body["status"])

	require.NoError(t, database.GetDB().Create(&database.FiscalReceipt{
		BusinessID: bill.BusinessID, SettingsID: 1, BillID: bill.ID,
		Country: "AR", Provider: "arca", Action: "issue",
		ReceiptType: "factura_b", TotalAmountCents: 121000, Currency: "ARS",
		Status: database.FiscalStatusPending,
	}).Error)

	c, w = guestFiscalReceiptRequest(t, "tok-pending-1")
	GetGuestFiscalReceipt(c)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "pending", body["status"])
}

func TestGetGuestFiscalReceiptUnknownToken404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGuestFiscalReceiptTestDB(t)

	c, w := guestFiscalReceiptRequest(t, "tok-does-not-exist")
	GetGuestFiscalReceipt(c)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetGuestFiscalReceiptPDFNotAvailable404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGuestFiscalReceiptTestDB(t)
	enableGuestFiscalRuntimeControl(t)
	bill := seedGuestFiscalBill(t, "tok-nopdf-1")
	require.NoError(t, database.GetDB().Create(&database.FiscalReceipt{
		BusinessID: bill.BusinessID, SettingsID: 1, BillID: bill.ID,
		Country: "AR", Provider: "arca", Action: "issue",
		ReceiptType: "factura_b", TotalAmountCents: 121000, Currency: "ARS",
		Status: database.FiscalStatusAuthorized, // authorized but no PDFPath yet
	}).Error)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/guest/bill/tok-nopdf-1/fiscal-receipt/pdf", nil)
	c.Params = gin.Params{{Key: "bill_token", Value: "tok-nopdf-1"}}
	GetGuestFiscalReceiptPDF(c)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetGuestFiscalReceiptHidesDemoSeedAuthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGuestFiscalReceiptTestDB(t)
	// Production currently ships fiscal_enabled=false; demo seed still writes
	// authorized rows with AUTH-<id> / DEMO-<id> / payverge.local placeholders.
	bill := seedGuestFiscalBillFor(t, "tok-demo-seed-1", database.Business{
		Name: "Payverge Core Demo Kitchen", OwnerAddress: "0xdemo",
		Kind: database.BusinessKindDemo, IsDemo: true,
	})
	seedAuthorizedGuestFiscalReceipt(t, bill,
		fmt.Sprintf("DEMO-%d", bill.ID),
		fmt.Sprintf("AUTH-%d", bill.ID),
		fmt.Sprintf("https://payverge.local/fiscal/%d", bill.ID),
		"fiscal/demo/receipt.pdf",
	)

	c, w := guestFiscalReceiptRequest(t, "tok-demo-seed-1")
	GetGuestFiscalReceipt(c)
	assertGuestFiscalReceiptNotAuthorized(t, w)
}

func TestGetGuestFiscalReceiptHidesWhenFiscalDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGuestFiscalReceiptTestDB(t)
	// No runtime_controls row: fiscal_enabled defaults false (fail closed).
	bill := seedGuestFiscalBill(t, "tok-fiscal-off-1")
	seedAuthorizedGuestFiscalReceipt(t, bill, "00000099", "71234567890123",
		"https://www.arca.gob.ar/fe/qr/?p=real", "fiscal/1/real.pdf")

	c, w := guestFiscalReceiptRequest(t, "tok-fiscal-off-1")
	GetGuestFiscalReceipt(c)
	assertGuestFiscalReceiptNotAuthorized(t, w)
}

func TestGetGuestFiscalReceiptHidesNonProductionBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGuestFiscalReceiptTestDB(t)
	enableGuestFiscalRuntimeControl(t)

	for _, tc := range []struct {
		token  string
		kind   database.BusinessKind
		isDemo bool
		name   string
	}{
		{"tok-kind-demo-1", database.BusinessKindDemo, true, "Demo Cafe"},
		{"tok-kind-test-1", database.BusinessKindTest, false, "CI Fixture"},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			bill := seedGuestFiscalBillFor(t, tc.token, database.Business{
				Name: tc.name, OwnerAddress: "0xnp", Kind: tc.kind, IsDemo: tc.isDemo,
			})
			seedAuthorizedGuestFiscalReceipt(t, bill, "00000100", "71234567890199",
				"https://www.arca.gob.ar/fe/qr/?p=np", "")

			c, w := guestFiscalReceiptRequest(t, tc.token)
			GetGuestFiscalReceipt(c)
			assertGuestFiscalReceiptNotAuthorized(t, w)
		})
	}
}

func TestGetGuestFiscalReceiptHidesPlaceholderMarkers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGuestFiscalReceiptTestDB(t)
	enableGuestFiscalRuntimeControl(t)

	cases := []struct {
		name, token, number, cae, qr string
	}{
		{"auth-prefix", "tok-ph-auth", "00000001", "AUTH-99", "https://www.arca.gob.ar/fe/qr/?p=x"},
		{"demo-number", "tok-ph-num", "DEMO-99", "71234567890123", "https://www.arca.gob.ar/fe/qr/?p=x"},
		{"local-qr", "tok-ph-qr", "00000001", "71234567890123", "https://payverge.local/fiscal/99"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bill := seedGuestFiscalBill(t, tc.token)
			seedAuthorizedGuestFiscalReceipt(t, bill, tc.number, tc.cae, tc.qr, "")

			c, w := guestFiscalReceiptRequest(t, tc.token)
			GetGuestFiscalReceipt(c)
			assertGuestFiscalReceiptNotAuthorized(t, w)
		})
	}
}

func TestGetGuestFiscalReceiptPDFHidesDemoSeed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupGuestFiscalReceiptTestDB(t)
	bill := seedGuestFiscalBillFor(t, "tok-demo-pdf-1", database.Business{
		Name: "Demo Kitchen", OwnerAddress: "0xdemo",
		Kind: database.BusinessKindDemo, IsDemo: true,
	})
	seedAuthorizedGuestFiscalReceipt(t, bill,
		fmt.Sprintf("DEMO-%d", bill.ID),
		fmt.Sprintf("AUTH-%d", bill.ID),
		fmt.Sprintf("https://payverge.local/fiscal/%d", bill.ID),
		"fiscal/demo/receipt.pdf",
	)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/guest/bill/tok-demo-pdf-1/fiscal-receipt/pdf", nil)
	c.Params = gin.Params{{Key: "bill_token", Value: "tok-demo-pdf-1"}}
	GetGuestFiscalReceiptPDF(c)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.NotContains(t, w.Body.String(), "AUTH-")
	assert.NotContains(t, w.Body.String(), "payverge.local")
}
