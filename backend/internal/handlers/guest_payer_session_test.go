package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/guestsession"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// M-545: guest-initiated payment rows carry the payer's guest session
// fingerprint so the guest fiscal identity can be bound to the paying session.

func TestGuestPayerSession_StaffDeviceIsNotAGuest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.Nil(t, guestPayerSession(nil))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Set("token_type", "staff")
	c.Set("staff_id", uint(9))
	require.Nil(t, guestPayerSession(c), "a POS device is not a diner's browser")
	require.Empty(t, w.Result().Cookies(), "no guest cookie is issued to staff")

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c2.Request.AddCookie(&http.Cookie{Name: guestsession.CookieName, Value: "diner." + guestsession.Sign("diner")})
	fp := guestPayerSession(c2)
	require.NotNil(t, fp)
	require.Equal(t, guestsession.Fingerprint("diner"), *fp)
}

func TestStorePluginPaymentRecordForPayer_StampsGuestSession(t *testing.T) {
	setupHandlerTestDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(&database.AlternativePayment{}))
	business := createTestBusiness(t)
	bill := createTestBill(t, business.ID)
	ph := NewPluginHandlers(nil, nil)

	tracker, err := ph.storePluginPaymentRecordForPayer(
		bill.ID, business.ID, "stripe", "pi_guest_payer_1", 1500, "USD", 1500, 0, nil, guestsession.FingerprintPtr("diner"),
	)
	require.NoError(t, err)
	require.NotNil(t, tracker.PayerGuestSession)
	require.Equal(t, guestsession.Fingerprint("diner"), *tracker.PayerGuestSession)

	// A replayed create from another session reuses the tracker and keeps the
	// original attribution.
	again, err := ph.storePluginPaymentRecordForPayer(
		bill.ID, business.ID, "stripe", "pi_guest_payer_1", 1500, "USD", 1500, 0, nil, guestsession.FingerprintPtr("other"),
	)
	require.NoError(t, err)
	require.Equal(t, tracker.ID, again.ID)
	var stored database.AlternativePayment
	require.NoError(t, database.GetDB().First(&stored, tracker.ID).Error)
	require.Equal(t, guestsession.Fingerprint("diner"), *stored.PayerGuestSession)

	// Operator-initiated trackers (MP QR / Point) carry no guest attribution.
	op, err := ph.storePluginPaymentRecord(bill.ID, business.ID, "stripe", "pi_operator_1", 1500, "USD", 1500, 0, nil)
	require.NoError(t, err)
	require.Nil(t, op.PayerGuestSession)
}

func TestRequestAlternativePayment_StampsPayerGuestSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAlternativePaymentRequestIntegrityDB(t)
	business := createPaymentRegressionBusiness(t, "request-guest-session", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 20)

	payload, err := json.Marshal(validAlternativePaymentRequestBody("5.00"))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/request-alternative-payment", bill.PublicToken), bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "guest-alt-session-stamp-0001")
	req.AddCookie(&http.Cookie{Name: guestsession.CookieName, Value: "diner." + guestsession.Sign("diner")})
	w := httptest.NewRecorder()
	alternativePaymentRequestRouter().ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var alt database.AlternativePayment
	require.NoError(t, database.GetDB().Where("bill_id = ?", bill.ID).First(&alt).Error)
	require.NotNil(t, alt.PayerGuestSession)
	require.Equal(t, guestsession.Fingerprint("diner"), *alt.PayerGuestSession)

	proof, err := database.GuestPaymentProofForSession(database.GetDB(), bill.ID, guestsession.Fingerprint("diner"), alt.CreatedAt)
	require.NoError(t, err)
	require.Equal(t, database.GuestPaymentProofPending, proof, "a pending tender request is pending payment proof")
}
