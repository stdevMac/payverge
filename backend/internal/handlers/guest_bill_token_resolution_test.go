package handlers

// Guest payment/split resolvers accept public_token only. Guessable bill_number
// must 404 with no mutation / no provider call.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

const unknownGuestBillToken = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"

func setupGuestBillTokenDB(t *testing.T) database.Bill {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	dsn := fmt.Sprintf("file:%s-%d?mode=memory&cache=shared", dsnName, time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Table{},
		&database.Bill{},
		&database.Payment{},
		&database.BillSplitShare{},
		&database.Order{},
	))

	business := &database.Business{
		BusinessId:     fmt.Sprintf("token-res-%d", time.Now().UnixNano()),
		Name:           "Token Resolution Restaurant",
		OwnerAddress:   "0xTokenResOwner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		IsActive:       true,
	}
	require.NoError(t, gormDB.Create(business).Error)

	bill := &database.Bill{
		BusinessID:     business.ID,
		BillNumber:     "TOKEN-RES-BILL-1",
		Items:          "[]",
		Subtotal:       10000,
		TotalAmount:    10000,
		PaidAmount:     0,
		Status:         database.BillStatusOpen,
		SettlementAddr: business.SettlementAddr,
		TippingAddr:    business.TippingAddr,
	}
	require.NoError(t, gormDB.Create(bill).Error)

	var saved database.Bill
	require.NoError(t, gormDB.First(&saved, bill.ID).Error)
	require.NotEmpty(t, saved.PublicToken)
	return saved
}

func ginCtxWithBillToken(identifier string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Params = gin.Params{{Key: "bill_token", Value: identifier}}
	c.Request = httptest.NewRequest(http.MethodGet, "/guest/bill/"+identifier, nil)
	return c
}

func TestGuestPaymentResolvers_AcceptTokenRejectBillNumber(t *testing.T) {
	saved := setupGuestBillTokenDB(t)
	h := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, nil)

	// getGuestBillByToken — crypto-quote, crypto-payment, cross-chain-payment.
	byToken, err := h.getGuestBillByToken(ginCtxWithBillToken(saved.PublicToken))
	require.NoError(t, err)
	assert.Equal(t, saved.ID, byToken.ID)

	_, err = h.getGuestBillByToken(ginCtxWithBillToken(saved.BillNumber))
	require.Error(t, err, "bill_number must not resolve guest payment bill")

	_, err = h.getGuestBillByToken(ginCtxWithBillToken(unknownGuestBillToken))
	require.Error(t, err)

	// resolveBillPaymentSummary — request-alternative-payment,
	// alternative-payments, payment-breakdown. Route-param branch:
	sumByToken, err := h.resolveBillPaymentSummary(ginCtxWithBillToken(saved.PublicToken), "")
	require.NoError(t, err)
	assert.Equal(t, saved.ID, sumByToken.ID)

	_, err = h.resolveBillPaymentSummary(ginCtxWithBillToken(saved.BillNumber), "")
	require.Error(t, err, "bill_number must not resolve payment summary")

	_, err = h.resolveBillPaymentSummary(ginCtxWithBillToken(unknownGuestBillToken), "")
	require.Error(t, err)

	// Fallback-identifier branch (no :bill_token route param) — token only.
	noParamCtx := func() *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/guest/bill", nil)
		return c
	}
	sumFallbackToken, err := h.resolveBillPaymentSummary(noParamCtx(), saved.PublicToken)
	require.NoError(t, err)
	assert.Equal(t, saved.ID, sumFallbackToken.ID)

	_, err = h.resolveBillPaymentSummary(noParamCtx(), saved.BillNumber)
	require.Error(t, err)

	_, err = h.resolveBillPaymentSummary(noParamCtx(), unknownGuestBillToken)
	require.Error(t, err)
}

func TestConfirmedSplitPaymentForExecute_AcceptTokenRejectBillNumber(t *testing.T) {
	saved := setupGuestBillTokenDB(t)
	h := NewSplittingHandler(database.GetDBWrapper())

	share := &database.BillSplitShare{
		BillID:         saved.ID,
		GuestSessionID: "guest-exec-1",
		Mode:           database.BillSplitModeCustom,
		AmountCents:    2500,
		Status:         database.BillSplitShareStatusHeld,
	}
	require.NoError(t, database.GetDB().Create(share).Error)

	payment := &database.Payment{
		BillID:    saved.ID,
		PayerAddr: "0xPayer",
		Amount:    2500,
		TipAmount: 0,
		TxHash:    "0xexec-token-1",
		Status:    database.PaymentStatusConfirmed,
	}
	require.NoError(t, database.GetDB().Create(payment).Error)

	pay, gotShare, tender, err := h.confirmedSplitPaymentForExecute(
		saved.PublicToken, share.ID, "guest-exec-1", "crypto", "0xexec-token-1", 0)
	require.NoError(t, err)
	require.NotNil(t, pay)
	assert.Equal(t, share.ID, gotShare.ID)
	assert.Equal(t, "crypto", tender)

	// Guessable bill_number must not resolve the share's bill.
	_, _, _, err = h.confirmedSplitPaymentForExecute(
		saved.BillNumber, share.ID, "guest-exec-1", "crypto", "0xexec-token-1", 0)
	require.Error(t, err)

	// Unknown identifier must not match the share's bill.
	_, _, _, err = h.confirmedSplitPaymentForExecute(
		unknownGuestBillToken, share.ID, "guest-exec-1", "crypto", "0xexec-token-1", 0)
	require.Error(t, err)
}

func TestGuestOrdersByBillToken_AcceptTokenRejectBillNumber(t *testing.T) {
	gin.SetMode(gin.TestMode)
	saved := setupGuestBillTokenDB(t)

	// Token succeeds (empty orders list is fine).
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: saved.PublicToken}}
	c.Request = httptest.NewRequest(http.MethodGet, "/guest/bill/"+saved.PublicToken+"/orders", nil)
	GetGuestOrdersByBillNumber(c)
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// bill_number must 404 (guessable operator id is not a capability).
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Params = gin.Params{{Key: "bill_token", Value: saved.BillNumber}}
	c2.Request = httptest.NewRequest(http.MethodGet, "/guest/bill/"+saved.BillNumber+"/orders", nil)
	GetGuestOrdersByBillNumber(c2)
	assert.Equal(t, http.StatusNotFound, w2.Code, w2.Body.String())
}
