package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func alternativePaymentRequestRouter() *gin.Engine {
	router := gin.New()
	handler := NewPaymentHandler(database.GetDBWrapper(), nil, nil, nil)
	router.POST("/guest/bill/:bill_token/request-alternative-payment", handler.RequestAlternativePayment)
	return router
}

func setupAlternativePaymentRequestIntegrityDB(t *testing.T) {
	t.Helper()
	setupPaymentRegressionDB(t)
	require.NoError(t, database.GetDB().AutoMigrate(
		&database.OperationalAlert{},
		&database.OperationalAlertEvent{},
		&database.BusinessAlertSettings{},
	))
}

func performAlternativePaymentRequest(
	t *testing.T,
	router *gin.Engine,
	billToken string,
	idempotencyKey string,
	body map[string]any,
) *httptest.ResponseRecorder {
	t.Helper()

	payload, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(
		http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/request-alternative-payment", billToken),
		bytes.NewReader(payload),
	)
	req.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func decodeAlternativePaymentRequestID(t *testing.T, response *httptest.ResponseRecorder) uint {
	t.Helper()
	var body struct {
		RequestID uint `json:"request_id"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.NotZero(t, body.RequestID)
	return body.RequestID
}

func validAlternativePaymentRequestBody(amount string) map[string]any {
	return map[string]any{
		"amount":           amount,
		"payment_method":   "cash",
		"participant_name": "Alex",
	}
}

func TestRequestAlternativePayment_RequiresValidIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name string
		key  string
	}{
		{name: "missing", key: ""},
		{name: "whitespace_only", key: "   "},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setupAlternativePaymentRequestIntegrityDB(t)
			business := createPaymentRegressionBusiness(t, "request-idem-"+tc.name, nil, "")
			bill := createPaymentRegressionBill(t, business.ID, 20)

			response := performAlternativePaymentRequest(
				t,
				alternativePaymentRequestRouter(),
				bill.PublicToken,
				tc.key,
				validAlternativePaymentRequestBody("5.00"),
			)

			assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			var count int64
			require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).
				Where("bill_id = ?", bill.ID).Count(&count).Error)
			assert.Zero(t, count, "an invalid request identity must not create a pending hold")
		})
	}
}

func TestRequestAlternativePayment_RejectsAmountAboveGlobalCeilingBeforeMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAlternativePaymentRequestIntegrityDB(t)
	t.Setenv("MAX_PAYMENT_AMOUNT_CENTS", "1000")
	business := createPaymentRegressionBusiness(t, "request-global-ceiling", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 20)

	response := performAlternativePaymentRequest(
		t,
		alternativePaymentRequestRouter(),
		bill.PublicToken,
		"guest-alt-global-ceiling-0001",
		validAlternativePaymentRequestBody("10.01"),
	)

	assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "exceeds the maximum allowed payment amount")
	var count int64
	require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).
		Where("bill_id = ?", bill.ID).Count(&count).Error)
	assert.Zero(t, count, "an over-ceiling request must not create a pending hold")
}

func TestRequestAlternativePayment_SameKeySamePayloadReturnsOriginalRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAlternativePaymentRequestIntegrityDB(t)
	business := createPaymentRegressionBusiness(t, "request-idem-replay", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 20)
	router := alternativePaymentRequestRouter()
	body := validAlternativePaymentRequestBody("5.00")
	const key = "guest-alt-replay-0001"
	createdBefore := metrics.CurrentAlternativePaymentRequestOutcome(metrics.AlternativePaymentRequestCreated)
	replayedBefore := metrics.CurrentAlternativePaymentRequestOutcome(metrics.AlternativePaymentRequestReplayed)

	first := performAlternativePaymentRequest(t, router, bill.PublicToken, key, body)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	firstID := decodeAlternativePaymentRequestID(t, first)

	replay := performAlternativePaymentRequest(t, router, bill.PublicToken, key, body)
	require.Equal(t, http.StatusOK, replay.Code, replay.Body.String())
	assert.Equal(t, firstID, decodeAlternativePaymentRequestID(t, replay),
		"an exact replay must return the original durable request")

	var count int64
	require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).
		Where("bill_id = ?", bill.ID).Count(&count).Error)
	assert.Equal(t, int64(1), count, "an exact replay must not create another pending hold")
	var stored database.AlternativePayment
	require.NoError(t, database.GetDB().Where("bill_id = ?", bill.ID).First(&stored).Error)
	assert.Empty(t, stored.IdempotencyKey, "raw guest request keys must never be stored")
	assert.Len(t, stored.IdempotencyKeyHash, 64)
	assert.Len(t, stored.PayloadHash, 64)
	assert.Equal(t, float64(1), metrics.CurrentAlternativePaymentRequestOutcome(metrics.AlternativePaymentRequestCreated)-createdBefore)
	assert.Equal(t, float64(1), metrics.CurrentAlternativePaymentRequestOutcome(metrics.AlternativePaymentRequestReplayed)-replayedBefore)
}

func TestRequestAlternativePayment_SameKeyDifferentPayloadConflicts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAlternativePaymentRequestIntegrityDB(t)
	business := createPaymentRegressionBusiness(t, "request-idem-conflict", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 20)
	router := alternativePaymentRequestRouter()
	const key = "guest-alt-conflict-0001"
	conflictBefore := metrics.CurrentAlternativePaymentRequestOutcome(metrics.AlternativePaymentRequestConflict)

	first := performAlternativePaymentRequest(
		t, router, bill.PublicToken, key, validAlternativePaymentRequestBody("5.00"),
	)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	firstID := decodeAlternativePaymentRequestID(t, first)

	conflict := performAlternativePaymentRequest(
		t, router, bill.PublicToken, key, validAlternativePaymentRequestBody("6.00"),
	)
	assert.Equal(t, http.StatusConflict, conflict.Code, conflict.Body.String())

	var payments []database.AlternativePayment
	require.NoError(t, database.GetDB().Where("bill_id = ?", bill.ID).Find(&payments).Error)
	require.Len(t, payments, 1, "a payload conflict must not create another pending hold")
	assert.Equal(t, firstID, payments[0].ID)
	assert.Equal(t, int64(500), payments[0].Amount)
	assert.Equal(t, float64(1), metrics.CurrentAlternativePaymentRequestOutcome(metrics.AlternativePaymentRequestConflict)-conflictBefore)
}

func TestRequestAlternativePayment_ReservesOnlyOutstandingBillCapacity(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name               string
		paidCents          int64
		pendingCents       int64
		requestedAmount    string
		expectedSeededRows int64
	}{
		{name: "request_exceeds_bill_total", requestedAmount: "10.01"},
		{name: "confirmed_payment_reduces_remaining", paidCents: 600, requestedAmount: "4.01"},
		{name: "pending_request_holds_remaining", pendingCents: 700, requestedAmount: "3.01", expectedSeededRows: 1},
		{name: "zero_balance", paidCents: 1000, requestedAmount: "0.01"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setupAlternativePaymentRequestIntegrityDB(t)
			business := createPaymentRegressionBusiness(t, "request-capacity-"+tc.name, nil, "")
			bill := createPaymentRegressionBill(t, business.ID, 10)
			if tc.paidCents > 0 {
				require.NoError(t, database.GetDB().Model(bill).Update("paid_amount", tc.paidCents).Error)
			}
			if tc.pendingCents > 0 {
				require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
					BillID:          bill.ID,
					ParticipantAddr: "guest",
					ParticipantName: "First guest",
					Amount:          tc.pendingCents,
					BillAmountCents: tc.pendingCents,
					PaymentMethod:   database.PaymentMethodCash,
					Status:          database.AltPaymentStatusPending,
					CreatedAt:       time.Now(),
					UpdatedAt:       time.Now(),
				}).Error)
			}

			response := performAlternativePaymentRequest(
				t,
				alternativePaymentRequestRouter(),
				bill.PublicToken,
				"guest-alt-capacity-"+tc.name,
				validAlternativePaymentRequestBody(tc.requestedAmount),
			)

			assert.Equal(t, http.StatusConflict, response.Code, response.Body.String())
			var count int64
			require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).
				Where("bill_id = ?", bill.ID).Count(&count).Error)
			assert.Equal(t, tc.expectedSeededRows, count,
				"a request above outstanding capacity must not create a pending hold")

			var reloaded database.Bill
			require.NoError(t, database.GetDB().First(&reloaded, bill.ID).Error)
			assert.Equal(t, tc.paidCents, reloaded.PaidAmount,
				"a pending request must never change settled balance")
		})
	}
}

func TestRequestAlternativePayment_CreatesRequestedAlertNotReceivedAlert(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAlternativePaymentRequestIntegrityDB(t)
	business := createPaymentRegressionBusiness(t, "request-alert-honesty", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 20)

	response := performAlternativePaymentRequest(
		t,
		alternativePaymentRequestRouter(),
		bill.PublicToken,
		"guest-alt-alert-0001",
		validAlternativePaymentRequestBody("5.00"),
	)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	var alerts []database.OperationalAlert
	require.NoError(t, database.GetDB().Where(
		"business_id = ? AND resource_type = ?",
		business.ID,
		database.OperationalAlertResourceTypeAltPayment,
	).Find(&alerts).Error)
	require.Len(t, alerts, 1)
	assert.Equal(t, database.OperationalAlertType("payment_requested"), alerts[0].AlertType)
	assert.NotEqual(t, database.OperationalAlertTypePaymentReceived, alerts[0].AlertType,
		"a pending request must never tell operators that payment was received")
}

func TestRequestAlternativePayment_ExpiredPendingRequestReleasesCapacity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAlternativePaymentRequestIntegrityDB(t)
	business := createPaymentRegressionBusiness(t, "request-expired-capacity", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 10)
	expiredAt := time.Now().UTC().Add(-time.Minute)
	require.NoError(t, database.GetDB().Create(&database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "guest",
		ParticipantName: "Expired guest",
		Amount:          900,
		BillAmountCents: 900,
		PaymentMethod:   database.PaymentMethodCash,
		Status:          database.AltPaymentStatusPending,
		ExpiresAt:       &expiredAt,
	}).Error)

	response := performAlternativePaymentRequest(
		t,
		alternativePaymentRequestRouter(),
		bill.PublicToken,
		"guest-alt-after-expiry",
		validAlternativePaymentRequestBody("10.00"),
	)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	var expiredCount, pendingCount int64
	require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).
		Where("bill_id = ? AND status = ?", bill.ID, database.AltPaymentStatusExpired).Count(&expiredCount).Error)
	require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).
		Where("bill_id = ? AND status = ?", bill.ID, database.AltPaymentStatusPending).Count(&pendingCount).Error)
	assert.Equal(t, int64(1), expiredCount)
	assert.Equal(t, int64(1), pendingCount)
}

func TestRequestAlternativePayment_AlertSideEffectFailureDoesNotDiscardPersistedRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAlternativePaymentRequestIntegrityDB(t)
	business := createPaymentRegressionBusiness(t, "request-alert-atomicity", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 10)
	require.NoError(t, database.GetDB().Migrator().DropTable(&database.OperationalAlertEvent{}))

	response := performAlternativePaymentRequest(
		t,
		alternativePaymentRequestRouter(),
		bill.PublicToken,
		"guest-alt-alert-rollback",
		validAlternativePaymentRequestBody("5.00"),
	)
	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
	requestID := decodeAlternativePaymentRequestID(t, response)

	var paymentCount, alertCount int64
	require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).Where("bill_id = ?", bill.ID).Count(&paymentCount).Error)
	require.NoError(t, database.GetDB().Model(&database.OperationalAlert{}).Where("business_id = ?", business.ID).Count(&alertCount).Error)
	assert.Equal(t, int64(1), paymentCount, "the durable request must commit even when its alert side effect fails")
	assert.Zero(t, alertCount, "the failed alert side effect must not leave a partial alert")

	var payment database.AlternativePayment
	require.NoError(t, database.GetDB().First(&payment, requestID).Error)
	assert.Equal(t, database.AltPaymentStatusPending, payment.Status)
}

func TestRequestAlternativePayment_PaidBillConflictUsesBillNotPayableCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAlternativePaymentRequestIntegrityDB(t)
	business := createPaymentRegressionBusiness(t, "request-paid-conflict-code", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 10)
	require.NoError(t, database.GetDB().Model(bill).Updates(map[string]any{
		"paid_amount": 1000,
		"status":      database.BillStatusPaid,
	}).Error)

	response := performAlternativePaymentRequest(
		t,
		alternativePaymentRequestRouter(),
		bill.PublicToken,
		"guest-alt-paid-conflict",
		validAlternativePaymentRequestBody("5.00"),
	)
	assert.Equal(t, http.StatusConflict, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "bill_not_payable")

	var count int64
	require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).
		Where("bill_id = ?", bill.ID).Count(&count).Error)
	assert.Zero(t, count)
}

func TestRequestAlternativePayment_SuccessReturnsRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAlternativePaymentRequestIntegrityDB(t)
	business := createPaymentRegressionBusiness(t, "request-success-id", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 20)

	response := performAlternativePaymentRequest(
		t,
		alternativePaymentRequestRouter(),
		bill.PublicToken,
		"guest-alt-success-0001",
		validAlternativePaymentRequestBody("8.00"),
	)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var body struct {
		Success   bool   `json:"success"`
		RequestID uint   `json:"request_id"`
		Message   string `json:"message"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.NotZero(t, body.RequestID)
	assert.NotEmpty(t, body.Message)

	var stored database.AlternativePayment
	require.NoError(t, database.GetDB().First(&stored, body.RequestID).Error)
	assert.Equal(t, database.AltPaymentStatusPending, stored.Status)
	assert.Equal(t, int64(800), stored.Amount)
}

func TestRequestAlternativePayment_UnknownBillReturnsNotFoundWithoutPersistence(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAlternativePaymentRequestIntegrityDB(t)
	business := createPaymentRegressionBusiness(t, "request-unknown-bill", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 20)

	response := performAlternativePaymentRequest(
		t,
		alternativePaymentRequestRouter(),
		"missing-public-token",
		"guest-alt-unknown-bill",
		validAlternativePaymentRequestBody("5.00"),
	)

	assert.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
	var count int64
	require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).
		Where("bill_id = ?", bill.ID).Count(&count).Error)
	assert.Zero(t, count)
}

func TestRequestAlternativePayment_PersistenceFailureReturnsStableErrorCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAlternativePaymentRequestIntegrityDB(t)
	business := createPaymentRegressionBusiness(t, "request-persistence-failure", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 20)
	require.NoError(t, database.GetDB().Migrator().DropTable(&database.AlternativePayment{}))

	response := performAlternativePaymentRequest(
		t,
		alternativePaymentRequestRouter(),
		bill.PublicToken,
		"guest-alt-persistence-failure",
		validAlternativePaymentRequestBody("5.00"),
	)

	assert.Equal(t, http.StatusInternalServerError, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "payment_request_failed")
}

func TestRequestAlternativePayment_SplitReplayAndConflictDoNotMutateShare(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAlternativePaymentRequestIntegrityDB(t)
	business := createPaymentRegressionBusiness(t, "request-split-idempotency", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 10)
	const guestSessionID = "guest-split-idempotency"
	share, _, err := database.HoldBillSplitShare(database.HoldBillSplitShareInput{
		BillID:         bill.ID,
		GuestSessionID: guestSessionID,
		DisplayName:    "Alex",
		Mode:           database.BillSplitModeCustom,
		AmountCents:    500,
		IdempotencyKey: "split-idempotency-hold",
		HoldTTL:        time.Minute,
		Now:            time.Now().UTC(),
	})
	require.NoError(t, err)

	router := alternativePaymentRequestRouter()
	body := map[string]any{
		"amount":           "5.00",
		"tip_amount":       "1.00",
		"payment_method":   "cash",
		"participant_name": "Alex",
		"split_share_id":   share.ID,
	}
	first := performPaymentRegressionRequestWithGuestSession(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/request-alternative-payment", bill.PublicToken), body, guestSessionID)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())

	var afterFirst database.BillSplitShare
	require.NoError(t, database.GetDB().First(&afterFirst, share.ID).Error)
	require.Equal(t, int64(100), afterFirst.TipCents)
	require.NotNil(t, afterFirst.HoldExpiresAt)

	replay := performPaymentRegressionRequestWithGuestSession(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/request-alternative-payment", bill.PublicToken), body, guestSessionID)
	require.Equal(t, http.StatusOK, replay.Code, replay.Body.String())
	require.Equal(t, decodeAlternativePaymentRequestID(t, first), decodeAlternativePaymentRequestID(t, replay))

	conflictingBody := map[string]any{
		"amount":           "5.00",
		"tip_amount":       "2.00",
		"payment_method":   "cash",
		"participant_name": "Alex",
		"split_share_id":   share.ID,
	}
	conflict := performPaymentRegressionRequestWithGuestSession(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/request-alternative-payment", bill.PublicToken), conflictingBody, guestSessionID)
	require.Equal(t, http.StatusConflict, conflict.Code, conflict.Body.String())

	var afterRetries database.BillSplitShare
	require.NoError(t, database.GetDB().First(&afterRetries, share.ID).Error)
	assert.Equal(t, afterFirst.TipCents, afterRetries.TipCents, "conflicting retry must not change the split tip")
	assert.Equal(t, afterFirst.HoldExpiresAt, afterRetries.HoldExpiresAt, "replay/conflict must not extend the split hold")
	var paymentCount int64
	require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).Where("bill_id = ?", bill.ID).Count(&paymentCount).Error)
	assert.Equal(t, int64(1), paymentCount)
}

func TestConfirmPendingAlternativePayment_RejectsExpiredRequestWithoutSettlingBill(t *testing.T) {
	setupAlternativePaymentRequestIntegrityDB(t)
	business := createPaymentRegressionBusiness(t, "confirm-expired-request", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 10)
	expiredAt := time.Now().UTC().Add(-time.Minute)
	payment := database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "guest",
		ParticipantName: "Alex",
		Amount:          500,
		BillAmountCents: 500,
		PaymentMethod:   database.PaymentMethodCash,
		Status:          database.AltPaymentStatusPending,
		ExpiresAt:       &expiredAt,
	}
	require.NoError(t, database.GetDB().Create(&payment).Error)

	_, _, err := database.ConfirmPendingAlternativePayment(bill.ID, payment.ID, "owner", nil)
	require.ErrorIs(t, err, database.ErrAlternativePaymentRequestExpired)

	var reloadedBill database.Bill
	require.NoError(t, database.GetDB().First(&reloadedBill, bill.ID).Error)
	assert.Zero(t, reloadedBill.PaidAmount)
	assert.Equal(t, database.BillStatusOpen, reloadedBill.Status)
	var reloadedPayment database.AlternativePayment
	require.NoError(t, database.GetDB().First(&reloadedPayment, payment.ID).Error)
	assert.Equal(t, database.AltPaymentStatusPending, reloadedPayment.Status)
}

// A guest who taps "pay at the counter" again while their first request still
// covers the whole balance gets payment_request_pending ("staff are on the
// way"), not the generic bill_not_payable ("the bill changed"). A request that
// only overshoots the unreserved part keeps bill_not_payable.
func TestRequestAlternativePayment_RepeatCashierRequestReportsPending(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAlternativePaymentRequestIntegrityDB(t)
	business := createPaymentRegressionBusiness(t, "request-repeat-pending", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 10)
	router := alternativePaymentRequestRouter()

	first := performAlternativePaymentRequest(t, router, bill.PublicToken, "guest-alt-repeat-first", validAlternativePaymentRequestBody("10.00"))
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())

	repeat := performAlternativePaymentRequest(t, router, bill.PublicToken, "guest-alt-repeat-second", validAlternativePaymentRequestBody("10.00"))
	assert.Equal(t, http.StatusConflict, repeat.Code, repeat.Body.String())
	assert.Contains(t, repeat.Body.String(), "payment_request_pending")
	assert.NotContains(t, repeat.Body.String(), "bill_not_payable")

	var count int64
	require.NoError(t, database.GetDB().Model(&database.AlternativePayment{}).
		Where("bill_id = ? AND status = ?", bill.ID, database.AltPaymentStatusPending).Count(&count).Error)
	assert.Equal(t, int64(1), count, "the repeat must not reserve the balance twice")

	partialBill := createPaymentRegressionBill(t, business.ID, 10)
	partial := performAlternativePaymentRequest(t, router, partialBill.PublicToken, "guest-alt-partial-first", validAlternativePaymentRequestBody("4.00"))
	require.Equal(t, http.StatusOK, partial.Code, partial.Body.String())
	over := performAlternativePaymentRequest(t, router, partialBill.PublicToken, "guest-alt-partial-over", validAlternativePaymentRequestBody("8.00"))
	assert.Equal(t, http.StatusConflict, over.Code, over.Body.String())
	assert.Contains(t, over.Body.String(), "bill_not_payable")
}
