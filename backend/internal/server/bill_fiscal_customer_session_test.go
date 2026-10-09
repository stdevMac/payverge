package server

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/guestsession"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// M-545: the guest fiscal identity is bound to the guest session that pays.
// A token holder who squats the identity first can be replaced by a guest with
// payment proof, and cannot replace a paying guest's identity.

const (
	squatterIdentity = `{
		"fiscal_customer_doc_type": "DNI",
		"fiscal_customer_doc_number": "87654321",
		"fiscal_customer_tax_condition": "consumidor_final",
		"fiscal_customer_name": "Squatter",
		"fiscal_customer_email": "squatter@example.com"
	}`
	payerIdentity = `{
		"fiscal_customer_doc_type": "CUIT",
		"fiscal_customer_doc_number": "20-11111111-2",
		"fiscal_customer_tax_condition": "responsable_inscripto",
		"fiscal_customer_name": "Payer SA"
	}`
)

func setupGuestFiscalSessionDB(t *testing.T) (*database.Business, *database.Bill) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gdb := setupFiscalCustomerTestDB(t)
	require.NoError(t, gdb.AutoMigrate(&database.AlternativePayment{}))
	return seedFiscalCustomerBill(t)
}

// guestFiscalPost runs the guest POST with the given guest session id (empty =
// no cookie) and returns the recorder.
func guestFiscalPost(t *testing.T, token, sessionID, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/guest/bill/"+token+"/fiscal-customer", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		req.AddCookie(&http.Cookie{Name: guestsession.CookieName, Value: sessionID + "." + guestsession.Sign(sessionID)})
	}
	c.Request = req
	c.Params = gin.Params{{Key: "bill_token", Value: token}}
	SetBillFiscalCustomerByNumber(c)
	return w
}

func seedGuestAltPayment(t *testing.T, billID uint, sessionID string, status database.AlternativePaymentStatus) {
	t.Helper()
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:            billID,
		ParticipantAddr:   "guest-" + sessionID,
		Amount:            1000,
		PaymentMethod:     database.PaymentMethodCash,
		Status:            status,
		PayerGuestSession: guestsession.FingerprintPtr(sessionID),
	}).Error)
}

func seedGuestConfirmedPayment(t *testing.T, billID uint, sessionID string) {
	t.Helper()
	require.NoError(t, database.GetDB().Create(&database.Payment{
		BillID:            billID,
		PayerAddr:         "crypto_guest",
		Amount:            1000,
		TxHash:            fmt.Sprintf("0xguest-fiscal-%s-%d", sessionID, time.Now().UnixNano()),
		Status:            database.PaymentStatusConfirmed,
		PayerGuestSession: guestsession.FingerprintPtr(sessionID),
	}).Error)
}

func TestGuestFiscalIdentity_FirstWriteRecordsSetterSession(t *testing.T) {
	_, bill := setupGuestFiscalSessionDB(t)

	w := guestFiscalPost(t, bill.PublicToken, "", payerIdentity)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var issued *http.Cookie
	for _, ck := range w.Result().Cookies() {
		if ck.Name == guestsession.CookieName {
			issued = ck
		}
	}
	require.NotNil(t, issued, "a cookieless guest gets a session so later payments can prove ownership")
	require.True(t, issued.HttpOnly)
	sessionID, ok := guestsession.Verify(issued.Value)
	require.True(t, ok)

	got := reloadFiscalBill(t, bill.ID)
	requirePtrEq(t, got.FiscalCustomerGuestSession, guestsession.Fingerprint(sessionID), "setter is the caller's session fingerprint")
	require.NotEqual(t, sessionID, *got.FiscalCustomerGuestSession, "raw session id is never stored")
}

func TestGuestFiscalIdentity_SameSessionCanCorrect(t *testing.T) {
	_, bill := setupGuestFiscalSessionDB(t)

	require.Equal(t, http.StatusOK, guestFiscalPost(t, bill.PublicToken, "diner", squatterIdentity).Code)
	w := guestFiscalPost(t, bill.PublicToken, "diner", `{"fiscal_customer_name": "Typo Fixed"}`)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	got := reloadFiscalBill(t, bill.ID)
	requirePtrEq(t, got.FiscalCustomerName, "Typo Fixed", "the setter session may correct its own identity")
	requirePtrEq(t, got.FiscalCustomerDocNumber, "87654321", "a partial correction leaves other fields")
}

func TestGuestFiscalIdentity_OtherSessionWithoutProofIsRejected(t *testing.T) {
	_, bill := setupGuestFiscalSessionDB(t)

	require.Equal(t, http.StatusOK, guestFiscalPost(t, bill.PublicToken, "first", squatterIdentity).Code)
	w := guestFiscalPost(t, bill.PublicToken, "second", payerIdentity)
	require.Equal(t, http.StatusConflict, w.Code, "body: %s", w.Body.String())

	got := reloadFiscalBill(t, bill.ID)
	requirePtrEq(t, got.FiscalCustomerName, "Squatter", "no payment proof, no replacement")
}

func TestGuestFiscalIdentity_PayerWithPendingProofReplacesSquatter(t *testing.T) {
	_, bill := setupGuestFiscalSessionDB(t)

	require.Equal(t, http.StatusOK, guestFiscalPost(t, bill.PublicToken, "squatter", squatterIdentity).Code)
	seedGuestAltPayment(t, bill.ID, "payer", database.AltPaymentStatusPending)

	w := guestFiscalPost(t, bill.PublicToken, "payer", payerIdentity)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	got := reloadFiscalBill(t, bill.ID)
	requirePtrEq(t, got.FiscalCustomerDocNumber, "20-11111111-2", "payer's document replaces the squat")
	requirePtrEq(t, got.FiscalCustomerName, "Payer SA", "payer's name replaces the squat")
	require.Nil(t, got.FiscalCustomerEmail, "nothing of the squatted identity survives (factura email redirect)")
	requirePtrEq(t, got.FiscalCustomerGuestSession, guestsession.Fingerprint("payer"), "payer becomes the setter")

	// The squatter cannot take it back without stronger proof than the payer.
	seedGuestAltPayment(t, bill.ID, "squatter", database.AltPaymentStatusPending)
	w = guestFiscalPost(t, bill.PublicToken, "squatter", squatterIdentity)
	require.Equal(t, http.StatusConflict, w.Code, "body: %s", w.Body.String())
	requirePtrEq(t, reloadFiscalBill(t, bill.ID).FiscalCustomerName, "Payer SA", "equal proof cannot replace")
}

func TestGuestFiscalIdentity_ConfirmedPaymentBeatsPendingSetter(t *testing.T) {
	_, bill := setupGuestFiscalSessionDB(t)

	seedGuestAltPayment(t, bill.ID, "squatter", database.AltPaymentStatusPending)
	require.Equal(t, http.StatusOK, guestFiscalPost(t, bill.PublicToken, "squatter", squatterIdentity).Code)
	seedGuestConfirmedPayment(t, bill.ID, "payer")

	w := guestFiscalPost(t, bill.PublicToken, "payer", payerIdentity)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	requirePtrEq(t, reloadFiscalBill(t, bill.ID).FiscalCustomerName, "Payer SA", "confirmed payment outranks a pending one")
}

func TestGuestFiscalIdentity_RejectedRequestIsNotProof(t *testing.T) {
	_, bill := setupGuestFiscalSessionDB(t)

	require.Equal(t, http.StatusOK, guestFiscalPost(t, bill.PublicToken, "first", squatterIdentity).Code)
	seedGuestAltPayment(t, bill.ID, "second", database.AltPaymentStatusRejected)

	w := guestFiscalPost(t, bill.PublicToken, "second", payerIdentity)
	require.Equal(t, http.StatusConflict, w.Code, "body: %s", w.Body.String())
}

func TestGuestFiscalIdentity_OperatorIdentityLockedEvenForPayer(t *testing.T) {
	business, bill := setupGuestFiscalSessionDB(t)

	// A guest sets it, then the operator corrects it: the operator now owns it.
	require.Equal(t, http.StatusOK, guestFiscalPost(t, bill.PublicToken, "diner", squatterIdentity).Code)
	c, w := newFiscalCustomerRequest(t, business.ID, bill.ID, `{"fiscal_customer_name": "Operator Fixed"}`)
	SetBillFiscalCustomer(c)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	require.Nil(t, reloadFiscalBill(t, bill.ID).FiscalCustomerGuestSession, "operator write clears the guest setter")

	seedGuestConfirmedPayment(t, bill.ID, "payer")
	for _, session := range []string{"diner", "payer"} {
		w := guestFiscalPost(t, bill.PublicToken, session, payerIdentity)
		require.Equal(t, http.StatusConflict, w.Code, "session %s: body: %s", session, w.Body.String())
	}
	requirePtrEq(t, reloadFiscalBill(t, bill.ID).FiscalCustomerName, "Operator Fixed", "guests cannot replace an operator identity")
}

func TestGuestFiscalIdentity_ForgedCookieGetsFreshSession(t *testing.T) {
	_, bill := setupGuestFiscalSessionDB(t)

	require.Equal(t, http.StatusOK, guestFiscalPost(t, bill.PublicToken, "victim", squatterIdentity).Code)

	// An attacker who knows (or guesses) the victim's session id cannot present
	// it without the server HMAC: the forged cookie is ignored.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/guest/bill/"+bill.PublicToken+"/fiscal-customer", bytes.NewBufferString(payerIdentity))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: guestsession.CookieName, Value: "victim.0000"})
	c.Request = req
	c.Params = gin.Params{{Key: "bill_token", Value: bill.PublicToken}}
	SetBillFiscalCustomerByNumber(c)
	require.Equal(t, http.StatusConflict, w.Code, "body: %s", w.Body.String())
}

// BenchmarkGuestFiscalCustomerPost covers the guest POST paths the binding
// touches: the common first write and a proof-checked override attempt.
func BenchmarkGuestFiscalCustomerPost(b *testing.B) {
	gin.SetMode(gin.TestMode)
	gdb, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", b.Name())), &gorm.Config{})
	if err != nil {
		b.Fatal(err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		b.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(gdb)
	if err := gdb.AutoMigrate(&database.Business{}, &database.Table{}, &database.Bill{}, &database.Payment{}, &database.AlternativePayment{}); err != nil {
		b.Fatal(err)
	}
	business := &database.Business{Name: "Bench Resto", OwnerAddress: "0xowner", Address: database.BusinessAddress{Country: "AR"}}
	if err := gdb.Create(business).Error; err != nil {
		b.Fatal(err)
	}
	seedBill := func(b *testing.B) *database.Bill {
		bill := &database.Bill{
			BusinessID: business.ID, BillNumber: fmt.Sprintf("FCB-%d", time.Now().UnixNano()),
			TotalAmount: 50000, Status: database.BillStatusOpen, SettlementAddr: "0xsettle", TippingAddr: "0xtip",
		}
		if err := gdb.Create(bill).Error; err != nil {
			b.Fatal(err)
		}
		return bill
	}
	run := func(b *testing.B, token, session, body string, want int) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest(http.MethodPost, "/guest/bill/"+token+"/fiscal-customer", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: guestsession.CookieName, Value: session + "." + guestsession.Sign(session)})
		c.Request = req
		c.Params = gin.Params{{Key: "bill_token", Value: token}}
		SetBillFiscalCustomerByNumber(c)
		if w.Code != want {
			b.Fatalf("status %d want %d: %s", w.Code, want, w.Body.String())
		}
	}

	b.Run("first_write", func(b *testing.B) {
		bill := seedBill(b)
		reset := map[string]any{
			"fiscal_customer_doc_type": nil, "fiscal_customer_doc_number": nil,
			"fiscal_customer_tax_condition": nil, "fiscal_customer_name": nil,
			"fiscal_customer_email": nil, "fiscal_customer_guest_session": nil,
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			if err := gdb.Model(&database.Bill{}).Where("id = ?", bill.ID).Updates(reset).Error; err != nil {
				b.Fatal(err)
			}
			b.StartTimer()
			run(b, bill.PublicToken, "bench-diner", payerIdentity, http.StatusOK)
		}
	})

	b.Run("override_rejected_equal_proof", func(b *testing.B) {
		bill := seedBill(b)
		run(b, bill.PublicToken, "bench-setter", squatterIdentity, http.StatusOK)
		for _, s := range []string{"bench-setter", "bench-other"} {
			if err := gdb.Create(&database.AlternativePayment{
				BillID: bill.ID, ParticipantAddr: "g-" + s, Amount: 1000, PaymentMethod: database.PaymentMethodCash,
				Status: database.AltPaymentStatusPending, PayerGuestSession: guestsession.FingerprintPtr(s),
			}).Error; err != nil {
				b.Fatal(err)
			}
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			run(b, bill.PublicToken, "bench-other", payerIdentity, http.StatusConflict)
		}
	})
}
