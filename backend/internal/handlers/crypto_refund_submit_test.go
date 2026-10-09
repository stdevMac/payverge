package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestSubmitCryptoRefundTx_TxHashParityWithGuestPayments drives the owner
// refund-submit endpoint with the spellings the guest payment path already
// refuses or folds (internal/txhash): the stored hash is the
// canonical 0x + 64 lowercase hex, an idempotent re-submit in another case is
// accepted, a second refund cannot claim the same transfer by changing case
// (409), and lenient spellings go-ethereum would still resolve are a 400
// before the database is touched.
func TestSubmitCryptoRefundTx_TxHashParityWithGuestPayments(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gormDB := setupPaymentRegressionDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.PaymentRefundDestination{}, &database.PaymentRefund{}))

	business := &database.Business{Name: "Refund Parity", BusinessId: "refund-parity", SettlementAddr: "0x1111111111111111111111111111111111111111"}
	require.NoError(t, gormDB.Create(business).Error)

	approvedRefund := func(label string) *database.PaymentRefund {
		t.Helper()
		bill := &database.Bill{
			BusinessID:     business.ID,
			BillNumber:     "REFUND-PARITY-" + label,
			TotalAmount:    2500,
			PaidAmount:     2500,
			Status:         database.BillStatusPaid,
			SettlementAddr: business.SettlementAddr,
		}
		require.NoError(t, gormDB.Create(bill).Error)
		payment := &database.Payment{
			BillID:          bill.ID,
			PayerAddr:       "crypto_guest",
			Amount:          2500,
			TxHash:          testEVMTxHash("refund-parity-payment-" + label),
			Status:          database.PaymentStatusConfirmed,
			PaymentMethod:   "crypto",
			SettlementChain: "base",
		}
		require.NoError(t, gormDB.Create(payment).Error)
		require.NoError(t, database.GetDB().Create(&database.PaymentRefundDestination{
			PaymentID:       payment.ID,
			ChainID:         84532,
			Token:           "USDC",
			AmountBaseUnits: 25_000_000,
			RefundAddress:   "0xdead00000000000000000000000000000000beef",
			EvidenceType:    database.RefundEvidenceTransferLog,
			VerifiedAt:      time.Now().UTC(),
		}).Error)
		refund, err := database.RequestCryptoRefund(database.CreateCryptoRefundInput{
			BusinessID: business.ID, BillID: bill.ID, PaymentID: payment.ID,
			AmountBaseUnits: 25_000_000, Reason: "on-chain refund",
			RequestedBy: "owner", IdempotencyKey: "refund-parity-" + label,
		})
		require.NoError(t, err)
		approved, err := database.ApproveCryptoRefund(refund.ID, business.ID, "owner")
		require.NoError(t, err)
		return approved
	}
	first := approvedRefund("a")
	second := approvedRefund("b")

	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, nil)
	submit := func(refundID uint, txHash string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"tx_hash": txHash})
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{
			{Key: "id", Value: fmt.Sprint(business.ID)},
			{Key: "refund_id", Value: fmt.Sprint(refundID)},
		}
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user_id", uint(1))
		handler.SubmitCryptoRefundTx(c)
		return w
	}

	const hexBody = "e15dd03ab99f5506fe7df79d6f90a30ed0d383ccdcf5f280f4dd9d1abff6c669"
	canonical := "0x" + hexBody

	w := submit(first.ID, "0X"+strings.ToUpper(hexBody))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Refund struct {
			Status          string `json:"status"`
			SubmittedTxHash string `json:"submitted_tx_hash"`
		} `json:"refund"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, canonical, resp.Refund.SubmittedTxHash)
	require.Equal(t, string(database.PaymentRefundStatusSubmitted), resp.Refund.Status)

	w = submit(first.ID, "0x"+strings.ToUpper(hexBody[:32])+hexBody[32:])
	require.Equal(t, http.StatusOK, w.Code, "idempotent re-submit in another case: %s", w.Body.String())

	for _, spelling := range []string{canonical, "0X" + strings.ToUpper(hexBody)} {
		w = submit(second.ID, spelling)
		require.Equalf(t, http.StatusConflict, w.Code, "spelling %q of a claimed transfer: %s", spelling, w.Body.String())
	}
	for _, lenient := range []string{hexBody, "0x00" + hexBody, "0x" + hexBody + "00", "0x" + hexBody[:63]} {
		w = submit(second.ID, lenient)
		require.Equalf(t, http.StatusBadRequest, w.Code, "lenient spelling %q: %s", lenient, w.Body.String())
	}

	reloaded, err := database.GetCryptoRefund(second.ID, business.ID)
	require.NoError(t, err)
	require.Equal(t, database.PaymentRefundStatusAwaitingSignature, reloaded.Status)
	require.Nil(t, reloaded.SubmittedTxHash)
}
