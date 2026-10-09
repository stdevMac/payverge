package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormlogger "gorm.io/gorm/logger"
)

func TestIssueCryptoQuote_USDBill_RateOne(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "quote-usd", nil, "")
	// USD-denominated business.
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		Update("default_currency", "USD").Error)
	bill := createPaymentRegressionBill(t, business.ID, 50) // $50.00

	exchangeRates := services.NewExchangeRateService(database.GetDBWrapper())
	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, exchangeRates)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-quote", handler.IssueCryptoQuote)

	w := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-quote", bill.PublicToken),
		map[string]any{"amount_paid": 50.0, "tip_amount": 5.0})

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		USDMicrounits int64   `json:"usd_microunits"`
		USDAmount     float64 `json:"usd_amount"`
		Rate          float64 `json:"rate"`
		ExpiresAt     int64   `json:"expires_at"`
		QuoteToken    string  `json:"quote_token"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// $55.00 -> 5500 cents -> 55_000_000 micro-units (rate 1), plus the
	// quote's unique sub-cent offset.
	assertPayerBoundQuoteAmount(t, 55_000_000, resp.USDMicrounits)
	assert.Equal(t, 1.0, resp.Rate)
	assert.NotEmpty(t, resp.QuoteToken)

	claims, err := parseCryptoQuote(resp.QuoteToken, []byte("quote-test-secret"), time.Now())
	require.NoError(t, err)
	assert.Equal(t, bill.ID, claims.BillID)
	assert.Equal(t, int64(5500), claims.LocalCents)
	assert.Equal(t, resp.USDMicrounits, claims.USDMicrounits, "the token carries the exact amount the guest is told to send")
}

func TestIssueCryptoQuoteUsesSplitShareAmountAndExtendsHold(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "quote-split-hold", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 100)
	now := time.Now().UTC()
	share, _, err := database.HoldBillSplitShare(database.HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-quote-split",
		Mode:           database.BillSplitModeCustom,
		AmountCents:    2500,
		IdempotencyKey: "quote-split-hold",
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)
	require.NotNil(t, share.HoldExpiresAt)
	originalExpiry := *share.HoldExpiresAt

	exchangeRates := services.NewExchangeRateService(database.GetDBWrapper())
	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, exchangeRates)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-quote", handler.IssueCryptoQuote)

	w := performPaymentRegressionRequestWithGuestSession(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-quote", bill.PublicToken),
		map[string]any{
			"amount_paid":    100.0,
			"tip_amount":     0.0,
			"split_share_id": share.ID,
		},
		"guest-quote-split",
	)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp cryptoQuoteResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assertPayerBoundQuoteAmount(t, 25_000_000, resp.USDMicrounits)

	claims, err := parseCryptoQuote(resp.QuoteToken, []byte("quote-test-secret"), time.Now())
	require.NoError(t, err)
	assert.Equal(t, int64(2500), claims.LocalCents)

	var reloadedShare database.BillSplitShare
	require.NoError(t, database.GetDB().Select("hold_expires_at").First(&reloadedShare, share.ID).Error)
	require.NotNil(t, reloadedShare.HoldExpiresAt)
	assert.True(t, reloadedShare.HoldExpiresAt.After(originalExpiry))
}

func TestIssueCryptoQuoteRejectsSplitShareFromAnotherGuestSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "quote-split-wrong-session", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 100)
	now := time.Now().UTC()
	share, _, err := database.HoldBillSplitShare(database.HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-owner",
		Mode:           database.BillSplitModeCustom,
		AmountCents:    2500,
		IdempotencyKey: "quote-split-wrong-session",
		HoldTTL:        5 * time.Minute,
		Now:            now,
	})
	require.NoError(t, err)
	require.NotNil(t, share.HoldExpiresAt)
	originalExpiry := *share.HoldExpiresAt

	exchangeRates := services.NewExchangeRateService(database.GetDBWrapper())
	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, exchangeRates)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-quote", handler.IssueCryptoQuote)

	w := performPaymentRegressionRequestWithGuestSession(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-quote", bill.PublicToken),
		map[string]any{
			"amount_paid":    100.0,
			"tip_amount":     0.0,
			"split_share_id": share.ID,
		},
		"guest-other",
	)

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())

	var reloadedShare database.BillSplitShare
	require.NoError(t, database.GetDB().Select("hold_expires_at").First(&reloadedShare, share.ID).Error)
	require.NotNil(t, reloadedShare.HoldExpiresAt)
	assert.True(t, reloadedShare.HoldExpiresAt.Equal(originalExpiry))
}

// createRegressionDeliveryOrder inserts a minimal delivery_orders row linked
// to a bill. Only the columns the payment gate projects (status,
// payment_expires_at) plus identity columns are populated.
func createRegressionDeliveryOrder(t *testing.T, businessID, billID uint, status database.DeliveryStatus, paymentExpiresAt *time.Time) {
	t.Helper()
	require.NoError(t, database.GetDB().Exec(
		`INSERT INTO delivery_orders (business_id, bill_id, delivery_number, status, payment_expires_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		businessID, billID, fmt.Sprintf("DEL-%d-%d", billID, time.Now().UnixNano()),
		string(status), paymentExpiresAt, time.Now(), time.Now(),
	).Error)
}

func newCryptoQuoteRegressionRouter(t *testing.T) *gin.Engine {
	t.Helper()
	exchangeRates := services.NewExchangeRateService(database.GetDBWrapper())
	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, exchangeRates)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-quote", handler.IssueCryptoQuote)
	return router
}

func TestIssueCryptoQuote_RejectsCancelledDeliveryOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "quote-del-cancel", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 50)
	createRegressionDeliveryOrder(t, business.ID, bill.ID, database.DeliveryStatusCancelled, nil)

	router := newCryptoQuoteRegressionRouter(t)
	w := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-quote", bill.PublicToken),
		map[string]any{"amount_paid": 50.0, "tip_amount": 0.0})

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var resp struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "payment_failed", resp.Code)
	assert.NotEmpty(t, resp.Error)
	assert.NotContains(t, w.Body.String(), "quote_token", "no signed quote may leak for a cancelled delivery")
}

func TestIssueCryptoQuote_RejectsFailedDeliveryOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "quote-del-failed", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 50)
	createRegressionDeliveryOrder(t, business.ID, bill.ID, database.DeliveryStatusFailed, nil)

	router := newCryptoQuoteRegressionRouter(t)
	w := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-quote", bill.PublicToken),
		map[string]any{"amount_paid": 50.0, "tip_amount": 0.0})

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
}

func TestIssueCryptoQuote_RejectsExpiredDeliveryPaymentWindow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "quote-del-expired", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 50)
	// Payment window lapsed but the expiry sweeper has not cancelled the
	// delivery yet: still 'confirmed' with a past payment_expires_at. The
	// quote gate must not depend on sweep timing.
	expired := time.Now().Add(-time.Minute)
	createRegressionDeliveryOrder(t, business.ID, bill.ID, database.DeliveryStatusConfirmed, &expired)

	router := newCryptoQuoteRegressionRouter(t)
	w := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-quote", bill.PublicToken),
		map[string]any{"amount_paid": 50.0, "tip_amount": 0.0})

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var resp struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "payment_failed", resp.Code)
}

func TestIssueCryptoQuote_AllowsActiveDeliveryOrder_NarrowProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	gormDB := setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "quote-del-active", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 50)
	future := time.Now().Add(10 * time.Minute)
	createRegressionDeliveryOrder(t, business.ID, bill.ID, database.DeliveryStatusConfirmed, &future)

	router := newCryptoQuoteRegressionRouter(t)

	recorder := &paymentDetailsSQLRecorder{Interface: gormlogger.Default.LogMode(gormlogger.Silent)}
	gormDB.Logger = recorder

	w := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-quote", bill.PublicToken),
		map[string]any{"amount_paid": 50.0, "tip_amount": 0.0})

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp cryptoQuoteResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.QuoteToken)

	// Access-shape guard (payment path): exactly one indexed lookup on
	// delivery_orders, projected columns only — never a hydrated aggregate.
	assert.Equal(t, 1, recorder.selectCount("delivery_orders"),
		"delivery gate must be a single delivery_orders lookup")
	assert.Equal(t, 0, recorder.selectStarCount("delivery_orders"),
		"delivery gate must project status/payment_expires_at, not SELECT *")
	assert.Equal(t, 1, recorder.selectWhereMentionsColumn("delivery_orders", "bill_id"),
		"delivery gate must look up by indexed bill_id")
}

func TestIssueCryptoQuote_RejectsTerminalBillStatuses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "quote-bill-terminal", nil, "")
	router := newCryptoQuoteRegressionRouter(t)

	for _, status := range []database.BillStatus{
		database.BillStatusPaid,
		database.BillStatusClosed,
		database.BillStatusVoided,
	} {
		bill := createPaymentRegressionBill(t, business.ID, 50)
		require.NoError(t, database.GetDB().Model(&database.Bill{}).
			Where("id = ?", bill.ID).
			Update("status", status).Error)

		w := performPaymentRegressionRequest(t, router, http.MethodPost,
			fmt.Sprintf("/guest/bill/%s/crypto-quote", bill.PublicToken),
			map[string]any{"amount_paid": 50.0, "tip_amount": 0.0})

		require.Equalf(t, http.StatusConflict, w.Code,
			"bill status %q must not be quotable: %s", status, w.Body.String())
	}
}

func TestIssueCryptoQuote_RejectsZeroAmount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "quote-zero", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 50)

	exchangeRates := services.NewExchangeRateService(database.GetDBWrapper())
	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, exchangeRates)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-quote", handler.IssueCryptoQuote)

	w := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-quote", bill.PublicToken),
		map[string]any{"amount_paid": 0.0, "tip_amount": 0.0})

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// Audit: crypto-quote must not mint signed amounts above remaining balance.
// Without this gate, a guest can lock usd_amount=9999 on a small open bill,
// transfer that USDC on-chain, then fail at ApplyConfirmedPayment with
// ErrPaymentExceedsRemaining — irreversible overpay with no bill credit.
func TestIssueCryptoQuote_RejectsAmountAboveRemaining(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "quote-over-total", nil, "")
	// $21.11 open bill; adversarial client requests $9999.
	bill := createPaymentRegressionBill(t, business.ID, 21.11)

	router := newCryptoQuoteRegressionRouter(t)
	w := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-quote", bill.PublicToken),
		map[string]any{"amount_paid": 9999.0, "tip_amount": 0.0})

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	var resp struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "payment_failed", resp.Code)
	assert.Contains(t, resp.Error, "exceeds remaining")
	assert.NotContains(t, w.Body.String(), "quote_token",
		"no signed quote may leak for an over-remaining amount")
}

func TestIssueCryptoQuote_RejectsPartialOverquote(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "quote-partial-over", nil, "")
	// Total $44.66, already paid $34.66 → remaining $10.00.
	// Client still requests amount_paid=34.66 (or full total).
	bill := createPaymentRegressionBill(t, business.ID, 44.66)
	require.NoError(t, database.GetDB().Model(&database.Bill{}).
		Where("id = ?", bill.ID).
		Updates(map[string]any{
			"paid_amount": int64(3466),
			"status":      database.BillStatusPartial,
		}).Error)

	router := newCryptoQuoteRegressionRouter(t)
	w := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-quote", bill.PublicToken),
		map[string]any{"amount_paid": 34.66, "tip_amount": 0.0})

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "quote_token")
}

func TestIssueCryptoQuote_RejectsAmountReservedByPendingCashRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "quote-pending-reserve", nil, "")
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		Update("default_currency", "USD").Error)
	bill := createPaymentRegressionBill(t, business.ID, 17.50)
	now := time.Now().UTC()
	expires := now.Add(5 * time.Minute)
	pending := database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "guest",
		ParticipantName: "QA",
		Amount:          1,
		BillAmountCents: 1,
		PaymentMethod:   database.PaymentMethodCash,
		Status:          database.AltPaymentStatusPending,
		ExpiresAt:       &expires,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	require.NoError(t, database.GetDB().Create(&pending).Error)

	router := newCryptoQuoteRegressionRouter(t)
	w := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-quote", bill.PublicToken),
		map[string]any{"amount_paid": 17.50, "tip_amount": 0.0, "payment_method": "usdc_payment"})

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "quote_token")

	ok := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-quote", bill.PublicToken),
		map[string]any{"amount_paid": 17.49, "tip_amount": 0.0, "payment_method": "usdc_payment"})
	require.Equal(t, http.StatusOK, ok.Code, ok.Body.String())
}

// Issue #529 AC: held split shares are the same reservation class as pending
// cashier requests. A $0.01 hold on $17.50 remaining must refuse a $17.50
// USDC quote so the held guest cannot be raced by a second rail.
func TestIssueCryptoQuote_RejectsAmountReservedByHeldSplitShare(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "quote-held-split-reserve", nil, "")
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		Update("default_currency", "USD").Error)
	bill := createPaymentRegressionBill(t, business.ID, 17.50)
	_, _, err := database.HoldBillSplitShare(database.HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-held-split",
		Mode:           database.BillSplitModeCustom,
		AmountCents:    1,
		IdempotencyKey: "quote-held-split-reserve",
		HoldTTL:        5 * time.Minute,
		Now:            time.Now().UTC(),
	})
	require.NoError(t, err)

	router := newCryptoQuoteRegressionRouter(t)
	w := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-quote", bill.PublicToken),
		map[string]any{"amount_paid": 17.50, "tip_amount": 0.0, "payment_method": "usdc_payment"})

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "quote_token")

	ok := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-quote", bill.PublicToken),
		map[string]any{"amount_paid": 17.49, "tip_amount": 0.0, "payment_method": "usdc_payment"})
	require.Equal(t, http.StatusOK, ok.Code, ok.Body.String())
}

// CreatePendingRequest does not reserve a held share twice. A guest paying
// their own $10 hold on a $17.50 bill (leftover $7.50) must still receive a
// quote for the share — the hold already reserved that $10.
func TestIssueCryptoQuote_AllowsOwnHeldSplitShareAboveLeftover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "quote-own-split-share", nil, "")
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		Update("default_currency", "USD").Error)
	bill := createPaymentRegressionBill(t, business.ID, 17.50)
	share, _, err := database.HoldBillSplitShare(database.HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: "guest-own-split",
		Mode:           database.BillSplitModeCustom,
		AmountCents:    1000,
		IdempotencyKey: "quote-own-split-share",
		HoldTTL:        5 * time.Minute,
		Now:            time.Now().UTC(),
	})
	require.NoError(t, err)

	router := newCryptoQuoteRegressionRouter(t)
	w := performPaymentRegressionRequestWithGuestSession(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-quote", bill.PublicToken),
		map[string]any{
			"amount_paid":    17.50,
			"tip_amount":     0.0,
			"payment_method": "usdc_payment",
			"split_share_id": share.ID,
		},
		"guest-own-split",
	)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp cryptoQuoteResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assertPayerBoundQuoteAmount(t, 10_000_000, resp.USDMicrounits)
	assert.NotEmpty(t, resp.QuoteToken)
}

func TestIssueCryptoQuote_AllowsExactRemainingPlusTip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "quote-remaining-ok", nil, "")
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		Update("default_currency", "USD").Error)
	bill := createPaymentRegressionBill(t, business.ID, 50.0)
	require.NoError(t, database.GetDB().Model(&database.Bill{}).
		Where("id = ?", bill.ID).
		Updates(map[string]any{
			"paid_amount": int64(4000), // $40 paid → $10 remaining
			"status":      database.BillStatusPartial,
		}).Error)

	router := newCryptoQuoteRegressionRouter(t)
	w := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-quote", bill.PublicToken),
		map[string]any{"amount_paid": 10.0, "tip_amount": 2.5})

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp cryptoQuoteResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	// $10 principal + $2.50 tip = $12.50 → 12_500_000 micro-units.
	assertPayerBoundQuoteAmount(t, 12_500_000, resp.USDMicrounits)
	assert.NotEmpty(t, resp.QuoteToken)

	claims, err := parseCryptoQuote(resp.QuoteToken, []byte("quote-test-secret"), time.Now())
	require.NoError(t, err)
	assert.Equal(t, int64(1250), claims.LocalCents)
}
