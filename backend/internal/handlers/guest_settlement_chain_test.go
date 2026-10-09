package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	appconfig "github.com/stdevmac/payverge/backend/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setSettlementProductionMode(t *testing.T, production bool) {
	t.Helper()
	appconfig.SetProductionModeOverride(production)
	t.Cleanup(func() { appconfig.SetProductionModeOverride(false) })
}

func TestGuestSettlementChainSupported(t *testing.T) {
	t.Setenv("ENV", "")
	t.Setenv("APP_ENV", "")

	tests := []struct {
		name       string
		production bool
		chainID    int64
		method     string
		want       bool
	}{
		{"production mainnet usdc", true, baseMainnetChainID, guestPaymentMethodUSDC, true},
		{"production mainnet cross-chain", true, baseMainnetChainID, guestPaymentMethodCrossChain, true},
		{"production sepolia usdc", true, baseSepoliaChainID, guestPaymentMethodUSDC, false},
		{"production sepolia cross-chain", true, baseSepoliaChainID, guestPaymentMethodCrossChain, false},
		{"development sepolia usdc", false, baseSepoliaChainID, guestPaymentMethodUSDC, true},
		{"development sepolia cross-chain", false, baseSepoliaChainID, guestPaymentMethodCrossChain, false},
		{"development ethereum mainnet", false, 1, guestPaymentMethodUSDC, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setSettlementProductionMode(t, tt.production)
			assert.Equal(t, tt.want, guestSettlementChainSupported(tt.chainID, tt.method))
		})
	}
}

func TestIssueCryptoQuoteSepoliaIsDevelopmentOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	t.Setenv("ENV", "")
	t.Setenv("APP_ENV", "")
	setupPaymentRegressionDB(t)

	t.Run("production refuses a Base Sepolia quote", func(t *testing.T) {
		setSettlementProductionMode(t, true)
		business := createPaymentRegressionBusiness(t, "quote-sepolia-prod", nil, "")
		bill := createPaymentRegressionBill(t, business.ID, 10)

		w := issueConfigurationQuote(t, newConfigurationQuoteHandler(&configuredMockVerifier{chainID: baseSepoliaChainID}), bill, guestPaymentMethodUSDC)

		require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
		var response struct {
			Code string `json:"code"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		assert.Equal(t, "plugin_unavailable", response.Code)
		assert.NotContains(t, w.Body.String(), "quote_token")
	})

	t.Run("development still quotes on Base Sepolia", func(t *testing.T) {
		setSettlementProductionMode(t, false)
		business := createPaymentRegressionBusiness(t, "quote-sepolia-dev", nil, "")
		bill := createPaymentRegressionBill(t, business.ID, 10)

		w := issueConfigurationQuote(t, newConfigurationQuoteHandler(&configuredMockVerifier{chainID: baseSepoliaChainID}), bill, guestPaymentMethodUSDC)

		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var quote map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &quote))
		assert.Equal(t, float64(baseSepoliaChainID), quote["chain_id"])
	})
}

// A quote minted on Sepolia (for example before a deploy flipped to
// production) must not verify a Sepolia transfer once production is on: the
// settlement gate runs again before any on-chain verification.
func TestProcessCryptoPaymentRejectsSepoliaTransferInProduction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	t.Setenv("ENV", "")
	t.Setenv("APP_ENV", "")
	setupPaymentRegressionDB(t)
	setSettlementProductionMode(t, true)

	business := createPaymentRegressionBusiness(t, "payment-sepolia-prod", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 10)
	verifier := &configuredMockVerifier{chainID: baseSepoliaChainID}
	handler := newConfigurationQuoteHandler(verifier)
	quote := signCryptoQuote(cryptoQuoteClaims{
		BusinessID:     business.ID,
		BillID:         bill.ID,
		LocalCents:     1000,
		USDMicrounits:  10_000_000,
		SettlementAddr: bill.SettlementAddr,
		ChainID:        baseSepoliaChainID,
		Token:          "USDC",
		PaymentMethod:  guestPaymentMethodUSDC,
		Exp:            time.Now().Add(10 * time.Minute).Unix(),
	}, []byte("quote-test-secret"))

	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-payment", handler.ProcessCryptoPayment)
	w := performPaymentRegressionRequest(t, router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-payment", bill.PublicToken),
		map[string]any{
			"transaction_hash": testEVMTxHash("tx-sepolia-prod"),
			"amount_paid":      10.0,
			"tip_amount":       0.0,
			"payment_method":   "USDC",
			"quote_token":      quote,
		})

	require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "plugin_unavailable")
	assert.Equal(t, int64(0), verifier.lastExpectedMicrounits,
		"a testnet transfer must be refused before on-chain verification in production")
}
