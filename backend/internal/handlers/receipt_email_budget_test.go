package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A guest-typed receipt send is tenant-triggered mail: when the venue's
// outbound budget refuses it, the guest gets a quota answer (429), not an
// outage (503), and the per-bill slot is handed back.
func TestEmailBillReceiptTenantBudgetExceededIs429AndReleasesSlot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupEmailReceiptTest(t)
	require.NoError(t, db.Create(&database.Business{
		ID: 9, BusinessId: "biz-9", Name: "Test Bistro", OwnerAddress: "0xown",
		SettlementAddr: "0x1", TippingAddr: "0x2", DefaultLanguage: "en",
	}).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID: 11, BusinessID: 9, BillNumber: "B-111", PublicToken: "tok-111",
		Status: database.BillStatusPaid, TotalAmount: 1200,
		SettlementAddr: "0x1", TippingAddr: "0x2",
		FiscalCustomerEmail: emailPtr("guest@example.com"),
	}).Error)

	var gotBusiness, gotBill uint
	original := sendGuestReceiptEmail
	sendGuestReceiptEmail = func(_ []string, _, _, _, _ string, _ []map[string]interface{}, _, _ string, businessID, billID uint) error {
		gotBusiness, gotBill = businessID, billID
		return fmt.Errorf("%w: business_daily", emails.ErrTenantMailBudgetExceeded)
	}
	t.Cleanup(func() { sendGuestReceiptEmail = original })

	w := postEmailReceipt(emailReceiptRouter(handler), "tok-111", map[string]any{"email": "guest@example.com"})

	require.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "email_budget_exceeded")
	assert.Equal(t, uint(9), gotBusiness, "send must be attributed to the bill's business")
	assert.Equal(t, uint(11), gotBill)
	var persisted int64
	require.NoError(t, db.Model(&database.GuestReceiptSend{}).Count(&persisted).Error)
	assert.Equal(t, int64(0), persisted, "a refused send must not consume the per-bill slot")
}

// A repeat request inside the tenant dedupe window mails nothing. The guest is
// told so (already_sent) instead of a plain success, and the per-bill slot is
// handed back so the skipped send does not count toward the bill's cap.
func TestEmailBillReceiptDuplicateReportsAlreadySentAndReleasesSlot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupEmailReceiptTest(t)
	require.NoError(t, db.Create(&database.Business{
		ID: 9, BusinessId: "biz-9", Name: "Test Bistro", OwnerAddress: "0xown",
		SettlementAddr: "0x1", TippingAddr: "0x2", DefaultLanguage: "en",
	}).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID: 13, BusinessID: 9, BillNumber: "B-113", PublicToken: "tok-113",
		Status: database.BillStatusPaid, TotalAmount: 1200,
		SettlementAddr: "0x1", TippingAddr: "0x2",
		FiscalCustomerEmail: emailPtr("guest@example.com"),
	}).Error)

	calls := 0
	original := sendGuestReceiptEmail
	sendGuestReceiptEmail = func([]string, string, string, string, string, []map[string]interface{}, string, string, uint, uint) error {
		calls++
		if calls == 1 {
			return nil
		}
		return emails.ErrTenantMailDuplicate
	}
	t.Cleanup(func() { sendGuestReceiptEmail = original })

	router := emailReceiptRouter(handler)
	first := postEmailReceipt(router, "tok-113", map[string]any{"email": "guest@example.com"})
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	assert.Contains(t, first.Body.String(), `"already_sent":false`)

	second := postEmailReceipt(router, "tok-113", map[string]any{"email": "guest@example.com"})
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	assert.Contains(t, second.Body.String(), `"already_sent":true`)

	var persisted int64
	require.NoError(t, db.Model(&database.GuestReceiptSend{}).Where("bill_id = ?", 13).Count(&persisted).Error)
	assert.Equal(t, int64(1), persisted, "only the delivered send keeps its per-bill slot")
}

func TestEmailBillReceiptSanitizesGuestDisplayStrings(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, handler := setupEmailReceiptTest(t)
	require.NoError(t, db.Create(&database.Business{
		ID: 9, BusinessId: "biz-9", Name: "Test Bistro", OwnerAddress: "0xown",
		SettlementAddr: "0x1", TippingAddr: "0x2", DefaultLanguage: "en",
	}).Error)
	require.NoError(t, db.Create(&database.Bill{
		ID: 12, BusinessID: 9, BillNumber: "B-112", PublicToken: "tok-112",
		Status: database.BillStatusPaid, TotalAmount: 1200,
		SettlementAddr: "0x1", TippingAddr: "0x2",
		FiscalCustomerEmail: emailPtr("guest@example.com"),
	}).Error)

	var gotMethod, gotTx string
	original := sendGuestReceiptEmail
	sendGuestReceiptEmail = func(_ []string, _, _, paymentMethod, transactionID string, _ []map[string]interface{}, _, _ string, _, _ uint) error {
		gotMethod, gotTx = paymentMethod, transactionID
		return nil
	}
	t.Cleanup(func() { sendGuestReceiptEmail = original })

	w := postEmailReceipt(emailReceiptRouter(handler), "tok-112", map[string]any{
		"email":          "guest@example.com",
		"payment_method": "Your account is locked, visit evil.example now",
		"transaction_id": "https://evil.example/reset",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Empty(t, gotMethod, "free text must not be relayed as a payment method")
	assert.Empty(t, gotTx, "a URL must not be relayed as a transaction id")
}

func TestSanitizeReceiptPaymentMethod(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain label", "Cash", "Cash"},
		{"label with parens", "Crypto (USDC)", "Crypto (USDC)"},
		{"raw method value", "usdc_payment", "usdc_payment"},
		{"accented", "Tarjeta de crédito", "Tarjeta de crédito"},
		{"devanagari with marks", "क्रिप्टो (USDC)", "क्रिप्टो (USDC)"},
		{"thai with marks", "บัตรเครดิต", "บัตรเครดิต"},
		{"collapses whitespace", "  Card \t payment ", "Card payment"},
		{"empty", "   ", ""},
		{"domain", "visit evil.example", ""},
		{"url", "http://x", ""},
		{"email address", "me@evil.example", ""},
		{"control char", "Cash\u0007", ""},
		{"angle brackets", "<b>Cash</b>", ""},
		{"exactly 40 runes", strings.Repeat("é", 40), strings.Repeat("é", 40)},
		{"41 runes", strings.Repeat("é", 41), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, sanitizeReceiptPaymentMethod(tc.in))
		})
	}
}

func TestSanitizeReceiptTransactionID(t *testing.T) {
	txHash := "0x" + strings.Repeat("ab", 32)
	cases := []struct {
		name, in, want string
	}{
		{"stripe intent", "pi_3Nabc-DEF", "pi_3Nabc-DEF"},
		{"tx hash", txHash, txHash},
		{"numeric", " 123456789 ", "123456789"},
		{"exactly 80", strings.Repeat("a", 80), strings.Repeat("a", 80)},
		{"81", strings.Repeat("a", 81), ""},
		{"space", "abc def", ""},
		{"url", "https://evil.example", ""},
		{"non ascii", "ñ123", ""},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, sanitizeReceiptTransactionID(tc.in))
		})
	}
}
