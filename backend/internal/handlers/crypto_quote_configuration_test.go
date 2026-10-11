package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type configuredMockVerifier struct {
	mockGuestPaymentVerifier
	chainID int64
	token   string
}

func (m *configuredMockVerifier) ChainID() int64 {
	if m.chainID == 0 {
		return 8453
	}
	return m.chainID
}

func (m *configuredMockVerifier) TokenSymbolForChain() string {
	if m.token == "" {
		return "USDC"
	}
	return m.token
}

func (m *configuredMockVerifier) VerifyUSDCTransfer(ctx context.Context, txHash string, recipient string, expectedAmountMicrounits int64) error {
	return m.mockGuestPaymentVerifier.VerifyUSDCTransfer(ctx, txHash, recipient, expectedAmountMicrounits)
}

func (m *configuredMockVerifier) VerifyUSDCTransferAtLeast(ctx context.Context, txHash string, recipient string, expectedAmountMicrounits int64) error {
	return m.mockGuestPaymentVerifier.VerifyUSDCTransferAtLeast(ctx, txHash, recipient, expectedAmountMicrounits)
}

func issueConfigurationQuote(t *testing.T, handler *PaymentHandler, bill *database.Bill, method string) *httptest.ResponseRecorder {
	t.Helper()
	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-quote", handler.IssueCryptoQuote)
	return performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-quote", bill.PublicToken),
		map[string]any{
			"amount_paid":    10.0,
			"tip_amount":     0.0,
			"payment_method": method,
		})
}

func TestIsUnusableGuestSettlementAddress(t *testing.T) {
	assert.True(t, isUnusableGuestSettlementAddress(common.HexToAddress("0x0000000000000000000000000000000000000000")))
	assert.True(t, isUnusableGuestSettlementAddress(common.HexToAddress("0x0000000000000000000000000000000000000001")))
	assert.True(t, isUnusableGuestSettlementAddress(common.HexToAddress("0x000000000000000000000000000000000000dE01")))
	assert.False(t, isUnusableGuestSettlementAddress(common.HexToAddress("0x000000000000000000000000000000000000dE02")))
	assert.False(t, isUnusableGuestSettlementAddress(common.HexToAddress("0x1111111111111111111111111111111111111111")))
}

func newConfigurationQuoteHandler(verifier guestPaymentVerifier) *PaymentHandler {
	return NewPaymentHandler(
		database.GetDBWrapper(),
		nil,
		verifier,
		services.NewExchangeRateService(database.GetDBWrapper()),
	)
}

func TestIssueCryptoQuoteRejectsInvalidSettlementConfiguration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	for _, testCase := range []struct {
		name       string
		address    string
		verifier   guestPaymentVerifier
		wantStatus int
		wantCode   string
	}{
		{
			name:       "empty settlement wallet",
			address:    "",
			verifier:   &mockGuestPaymentVerifier{},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "plugin_unavailable",
		},
		{
			name:       "malformed settlement wallet",
			address:    "not-an-evm-address",
			verifier:   &mockGuestPaymentVerifier{},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "plugin_unavailable",
		},
		{
			name:       "unsupported settlement chain",
			address:    "0x1111111111111111111111111111111111111111",
			verifier:   &configuredMockVerifier{chainID: 1},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "plugin_unavailable",
		},
		{
			name:       "settlement verifier unavailable",
			address:    "0x1111111111111111111111111111111111111111",
			verifier:   nil,
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "plugin_unavailable",
		},
		{
			name:       "zero settlement wallet",
			address:    "0x0000000000000000000000000000000000000000",
			verifier:   &configuredMockVerifier{},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "plugin_unavailable",
		},
		{
			name:       "low placeholder settlement wallet",
			address:    "0x0000000000000000000000000000000000000001",
			verifier:   &configuredMockVerifier{},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "plugin_unavailable",
		},
		{
			name:       "demo seed placeholder settlement wallet",
			address:    "0x000000000000000000000000000000000000dE01",
			verifier:   &configuredMockVerifier{},
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "plugin_unavailable",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			business := createPaymentRegressionBusiness(t, "quote-config-"+fmt.Sprint(time.Now().UnixNano()), nil, "")
			bill := createPaymentRegressionBill(t, business.ID, 10)
			require.NoError(t, database.GetDB().Model(&database.Bill{}).
				Where("id = ?", bill.ID).
				Update("settlement_addr", testCase.address).Error)
			bill.SettlementAddr = testCase.address

			w := issueConfigurationQuote(t, newConfigurationQuoteHandler(testCase.verifier), bill, "usdc_payment")

			require.Equal(t, testCase.wantStatus, w.Code, w.Body.String())
			var response struct {
				Code string `json:"code"`
			}
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
			assert.Equal(t, testCase.wantCode, response.Code)
			assert.NotContains(t, w.Body.String(), "quote_token")
		})
	}
}

func TestIssueCryptoQuoteRejectsDemoBusinessOnMainnet(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "quote-demo-mainnet", nil, "")
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		Update("is_demo", true).Error)

	bill := createPaymentRegressionBill(t, business.ID, 10)
	w := issueConfigurationQuote(t, newConfigurationQuoteHandler(&configuredMockVerifier{}), bill, "usdc_payment")

	require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	var response struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	assert.Equal(t, "plugin_unavailable", response.Code)
	assert.NotContains(t, w.Body.String(), "quote_token")
}

func TestIssueCryptoQuoteRejectsDisabledRequestedPlugin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "quote-disabled-plugin", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 10)
	var plugin database.Plugin
	require.NoError(t, database.GetDB().Where("name = ?", "usdc_payment").First(&plugin).Error)
	require.NoError(t, database.GetDB().Model(&database.BusinessPlugin{}).
		Where("business_id = ? AND plugin_id = ?", business.ID, plugin.ID).
		Update("is_enabled", false).Error)

	w := issueConfigurationQuote(t, newConfigurationQuoteHandler(&configuredMockVerifier{}), bill, "usdc_payment")

	require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "plugin_unavailable")
	assert.NotContains(t, w.Body.String(), "quote_token")
}

func TestCryptoQuoteBindsSettlementContractAndRejectsWalletChange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "quote-wallet-version", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 10)
	verifier := &configuredMockVerifier{}
	handler := newConfigurationQuoteHandler(verifier)

	quoteRecorder := issueConfigurationQuote(t, handler, bill, "usdc_payment")
	require.Equal(t, http.StatusOK, quoteRecorder.Code, quoteRecorder.Body.String())
	var quote map[string]any
	require.NoError(t, json.Unmarshal(quoteRecorder.Body.Bytes(), &quote))
	assert.Equal(t, bill.SettlementAddr, quote["settlement_address"])
	assert.Equal(t, float64(8453), quote["chain_id"])
	assert.Equal(t, "USDC", quote["token"])
	quoteToken, ok := quote["quote_token"].(string)
	require.True(t, ok)

	claims, err := parseCryptoQuote(quoteToken, []byte("quote-test-secret"), time.Now())
	require.NoError(t, err)
	encodedClaims, err := json.Marshal(claims)
	require.NoError(t, err)
	var claimMap map[string]any
	require.NoError(t, json.Unmarshal(encodedClaims, &claimMap))
	assert.Equal(t, bill.SettlementAddr, claimMap["settlement_address"])
	assert.Equal(t, float64(8453), claimMap["chain_id"])
	assert.Equal(t, "USDC", claimMap["token"])
	assert.Equal(t, "usdc_payment", claimMap["payment_method"])

	// Rotate the way UpdateBusiness does: business and open bill move together.
	newWallet := "0x3333333333333333333333333333333333333333"
	require.NoError(t, database.GetDB().Model(&database.Business{}).
		Where("id = ?", business.ID).
		Update("settlement_addr", newWallet).Error)
	require.NoError(t, database.GetDB().Model(&database.Bill{}).
		Where("id = ?", bill.ID).
		Update("settlement_addr", newWallet).Error)

	settlementRouter := gin.New()
	settlementRouter.POST("/guest/bill/:bill_token/crypto-payment", handler.ProcessCryptoPayment)
	w := performPaymentRegressionRequest(t, settlementRouter, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-payment", bill.PublicToken),
		map[string]any{
			"transaction_hash": testEVMTxHash("tx-wallet-version"),
			"amount_paid":      10.0,
			"tip_amount":       0.0,
			"payment_method":   "USDC",
			"quote_token":      quoteToken,
		})

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "quote_invalid")
	assert.Equal(t, int64(0), verifier.lastExpectedMicrounits,
		"a wallet-version mismatch must fail before on-chain verification")
}

func TestProcessCryptoPaymentRejectsExpiredSettlementQuote(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)

	business := createPaymentRegressionBusiness(t, "quote-expired-settlement", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 10)
	verifier := &configuredMockVerifier{}
	handler := newConfigurationQuoteHandler(verifier)
	expired := signCryptoQuote(cryptoQuoteClaims{
		BusinessID:     business.ID,
		BillID:         bill.ID,
		LocalCents:     1000,
		USDMicrounits:  10_000_000,
		SettlementAddr: bill.SettlementAddr,
		ChainID:        baseMainnetChainID,
		Token:          "USDC",
		PaymentMethod:  guestPaymentMethodUSDC,
		Exp:            time.Now().Add(-time.Second).Unix(),
	}, []byte("quote-test-secret"))

	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-payment", handler.ProcessCryptoPayment)
	w := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-payment", bill.PublicToken),
		map[string]any{
			"transaction_hash": testEVMTxHash("tx-expired-quote"),
			"amount_paid":      10.0,
			"tip_amount":       0.0,
			"payment_method":   "USDC",
			"quote_token":      expired,
		})

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "crypto_quote_expired")
	assert.Equal(t, int64(0), verifier.lastExpectedMicrounits)
}
