package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/blockchain"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hexHashResolvingVerifier mirrors the production BlockchainService lookup
// shape: the guest-supplied hash is resolved through common.HexToHash, so any
// spelling that decodes to the same 32 bytes ("0xabc…", "0xABC…", "abc…")
// lands on the SAME on-chain receipt. It holds exactly one real transfer of
// transferMicrounits USDC to the settlement address.
type hexHashResolvingVerifier struct {
	realTx             common.Hash
	transferMicrounits int64
	blockTime          uint64
	calls              int
}

func (v *hexHashResolvingVerifier) VerifyUSDCTransfer(ctx context.Context, txHash, recipient string, expected int64) error {
	_, err := v.VerifyUSDCTransferWithEvidence(ctx, txHash, recipient, expected, false)
	return err
}

func (v *hexHashResolvingVerifier) VerifyUSDCTransferAtLeast(ctx context.Context, txHash, recipient string, expected int64) error {
	_, err := v.VerifyUSDCTransferWithEvidence(ctx, txHash, recipient, expected, true)
	return err
}

func (v *hexHashResolvingVerifier) VerifyUSDCTransferWithEvidence(_ context.Context, txHash, recipient string, expected int64, allowExcess bool) (blockchain.USDCTransferEvidence, error) {
	v.calls++
	if common.HexToHash(strings.TrimSpace(txHash)) != v.realTx {
		return blockchain.USDCTransferEvidence{}, fmt.Errorf("no matching USDC transfer found for transaction")
	}
	if v.transferMicrounits < expected || (!allowExcess && v.transferMicrounits != expected) {
		return blockchain.USDCTransferEvidence{}, fmt.Errorf("no matching USDC transfer found for transaction")
	}
	blockTime := v.blockTime
	if blockTime == 0 {
		// A real receipt always has a block time; default to "just mined".
		blockTime = uint64(time.Now().Unix())
	}
	return blockchain.USDCTransferEvidence{
		From:            "0x000000000000000000000000000000000000dEaD",
		To:              common.HexToAddress(recipient).Hex(),
		AmountBaseUnits: v.transferMicrounits,
		TxHash:          txHash,
		LogIndex:        0,
		ChainID:         baseMainnetChainID,
		Token:           "USDC",
		BlockNumber:     100,
		BlockHash:       "0x" + strings.Repeat("ab", 32),
		BlockTimestamp:  blockTime,
	}, nil
}

func (v *hexHashResolvingVerifier) ChainID() int64              { return baseMainnetChainID }
func (v *hexHashResolvingVerifier) TokenSymbolForChain() string { return "USDC" }

// TestProcessCryptoPayment_TxHashSpellingVariantsCannotSettleSecondBill is the
// regression for the guest USDC replay: ONE real on-chain transfer must settle
// at most ONE bill. Re-submitting the same transfer with different hex casing
// or without the 0x prefix must not be treated as a new payment.
func TestProcessCryptoPayment_TxHashSpellingVariantsCannotSettleSecondBill(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	gormDB := setupPaymentRegressionDB(t)
	// The evidence verifier path persists a refund destination atomically with
	// the payment; the shared regression schema does not carry that table.
	require.NoError(t, gormDB.AutoMigrate(&database.PaymentRefundDestination{}))

	business := createPaymentRegressionBusiness(t, "crypto-replay", nil, "")
	billA := createPaymentRegressionBill(t, business.ID, 30)
	billB := createPaymentRegressionBill(t, business.ID, 30)
	billC := createPaymentRegressionBill(t, business.ID, 30)

	const realHexBody = "5c504ed432cb51138bcf09aa5e8a410dd4a1e204ef84bfed1be16dfba1b22060"
	verifier := &hexHashResolvingVerifier{
		realTx:             common.HexToHash("0x" + realHexBody),
		transferMicrounits: centsToMicrounits(3000), // one transfer of the quoted exact amount
	}
	handler := NewPaymentHandler(database.GetDBWrapper(), nil, verifier, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-payment", handler.ProcessCryptoPayment)

	pay := func(bill *database.Bill, hash string) int {
		w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/guest/bill/%s/crypto-payment", bill.PublicToken), map[string]any{
			"transaction_hash": hash,
			"amount_paid":      30.0,
			"tip_amount":       0.0,
			"payment_method":   "USDC",
			"quote_token":      signTestQuote(t, "quote-test-secret", bill.ID, 3000, centsToMicrounits(3000)),
		})
		return w.Code
	}

	require.Equal(t, http.StatusOK, pay(billA, "0x"+realHexBody), "the genuine first claim settles")

	codeB := pay(billB, "0x"+strings.ToUpper(realHexBody)) // uppercase hex body
	codeC := pay(billC, realHexBody)                       // no 0x prefix

	for name, bill := range map[string]*database.Bill{"B (uppercase hex)": billB, "C (no 0x prefix)": billC} {
		reloaded, _, err := database.GetBillByID(bill.ID)
		require.NoError(t, err)
		assert.Equalf(t, int64(0), reloaded.PaidAmount, "bill %s must not be settled by a replayed transfer", name)
		assert.Equalf(t, database.BillStatusOpen, reloaded.Status, "bill %s must stay open", name)
	}
	assert.NotEqual(t, http.StatusOK, codeB, "uppercase replay must be rejected")
	assert.NotEqual(t, http.StatusOK, codeC, "no-0x replay must be rejected")

	var count int64
	require.NoError(t, database.GetDB().Model(&database.Payment{}).
		Where("payment_method = ?", "crypto").Count(&count).Error)
	assert.Equal(t, int64(1), count, "one on-chain transfer must create exactly one payment row")
}

// testEVMTxHash derives a well-formed, deterministic 0x+64-hex transaction
// hash from a readable label so fixtures stay legible while satisfying the
// canonical-hash contract enforced at request binding.
func testEVMTxHash(label string) string {
	sum := sha256.Sum256([]byte(label))
	return "0x" + hex.EncodeToString(sum[:])
}

func signTestQuoteIssuedAt(t testing.TB, secret string, billID uint, localCents, usdMicrounits int64, iat time.Time) string {
	t.Helper()
	return signTestQuoteForMethod(t, secret, billID, guestPaymentMethodUSDC, localCents, usdMicrounits, iat)
}

// signTestQuoteForMethod persists a quote row for the rail and signs a token
// carrying every field a server-minted quote carries: business, quote row,
// settlement contract (address, Base mainnet, USDC, method) and iat.
func signTestQuoteForMethod(t testing.TB, secret string, billID uint, method string, localCents, usdMicrounits int64, iat time.Time) string {
	t.Helper()
	quote := persistTestCryptoQuote(t, billID, method, usdMicrounits, iat)
	var bill database.Bill
	require.NoError(t, database.GetDB().Select("id", "business_id", "settlement_addr").Where("id = ?", billID).Take(&bill).Error)
	return signCryptoQuote(cryptoQuoteClaims{
		BusinessID:     bill.BusinessID,
		QuoteID:        quote.ID,
		BillID:         billID,
		LocalCents:     localCents,
		USDMicrounits:  quote.ExactMicrounits,
		SettlementAddr: bill.SettlementAddr,
		ChainID:        baseMainnetChainID,
		Token:          "USDC",
		PaymentMethod:  method,
		Iat:            quote.IssuedAt.Unix(),
		Exp:            quote.ExpiresAt.Unix(),
	}, []byte(secret))
}

// testQuoteMethodForPath maps a guest payment route to the rail its quote binds.
func testQuoteMethodForPath(path string) string {
	if strings.Contains(path, "cross-chain-payment") {
		return guestPaymentMethodCrossChain
	}
	return guestPaymentMethodUSDC
}

// TestGuestCryptoSettlement_RejectsTransferMinedBeforeQuote pins quote-window
// binding: a transfer that was already on-chain before the quote was minted
// (for example another guest's earlier payment to the same settlement
// address) cannot settle the bill, on either guest crypto rail.
func TestGuestCryptoSettlement_RejectsTransferMinedBeforeQuote(t *testing.T) {
	enableGuestCrossChainSettlementForTest(t) // keep the dormant cross-chain rail covered
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	gormDB := setupPaymentRegressionDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.PaymentRefundDestination{}))

	business := createPaymentRegressionBusiness(t, "crypto-predates", nil, "")
	quoteIssued := time.Now()
	minedBefore := uint64(quoteIssued.Add(-10 * time.Minute).Unix())

	for _, rail := range []struct {
		name, path string
		extra      map[string]any
	}{
		{"usdc", "crypto-payment", map[string]any{"payment_method": "USDC"}},
		{"cross-chain", "cross-chain-payment", map[string]any{"source_chain": "ethereum", "source_token": "USDC", "lifi_route_id": "route-1"}},
	} {
		t.Run(rail.name, func(t *testing.T) {
			bill := createPaymentRegressionBill(t, business.ID, 30)
			hash := testEVMTxHash("historical-" + rail.name)
			verifier := &hexHashResolvingVerifier{
				realTx:             common.HexToHash(hash),
				transferMicrounits: centsToMicrounits(3000),
				blockTime:          minedBefore,
			}
			handler := NewPaymentHandler(database.GetDBWrapper(), nil, verifier, nil)
			router := gin.New()
			router.POST("/guest/bill/:bill_token/crypto-payment", handler.ProcessCryptoPayment)
			router.POST("/guest/bill/:bill_token/cross-chain-payment", handler.ProcessCrossChainPayment)

			body := map[string]any{
				"transaction_hash": hash,
				"amount_paid":      30.0,
				"tip_amount":       0.0,
				"quote_token":      signTestQuoteForMethod(t, "quote-test-secret", bill.ID, testQuoteMethodForPath(rail.path), 3000, centsToMicrounits(3000), quoteIssued),
			}
			for k, v := range rail.extra {
				body[k] = v
			}
			w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/guest/bill/%s/%s", bill.PublicToken, rail.path), body)
			assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), "transfer_predates_quote")

			reloaded, _, err := database.GetBillByID(bill.ID)
			require.NoError(t, err)
			assert.Equal(t, int64(0), reloaded.PaidAmount)

			// The same transfer mined inside the quote window settles.
			verifier.blockTime = uint64(quoteIssued.Add(time.Minute).Unix())
			w = performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/guest/bill/%s/%s", bill.PublicToken, rail.path), body)
			assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
		})
	}
}

// TestGuestCryptoSettlement_RejectsNonCanonicalHashBeforeVerifying pins that
// malformed hash spellings are refused at binding, before any RPC lookup.
func TestGuestCryptoSettlement_RejectsNonCanonicalHashBeforeVerifying(t *testing.T) {
	enableGuestCrossChainSettlementForTest(t) // keep the dormant cross-chain rail covered
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	setupPaymentRegressionDB(t)
	business := createPaymentRegressionBusiness(t, "crypto-noncanonical", nil, "")
	bill := createPaymentRegressionBill(t, business.ID, 30)
	body := "5c504ed432cb51138bcf09aa5e8a410dd4a1e204ef84bfed1be16dfba1b22060"
	verifier := &hexHashResolvingVerifier{realTx: common.HexToHash("0x" + body), transferMicrounits: centsToMicrounits(3000)}
	handler := NewPaymentHandler(database.GetDBWrapper(), nil, verifier, nil)
	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-payment", handler.ProcessCryptoPayment)
	router.POST("/guest/bill/:bill_token/cross-chain-payment", handler.ProcessCrossChainPayment)

	for _, path := range []string{"crypto-payment", "cross-chain-payment"} {
		for _, hash := range []string{body, "0x00" + body, "0x" + body + "zz", "0x" + body[:62]} {
			w := performPaymentRegressionRequest(t, router, http.MethodPost, fmt.Sprintf("/guest/bill/%s/%s", bill.PublicToken, path), map[string]any{
				"transaction_hash": hash,
				"amount_paid":      30.0,
				"tip_amount":       0.0,
				"payment_method":   "USDC",
				"source_chain":     "ethereum",
				"source_token":     "USDC",
				"quote_token":      signTestQuoteForMethod(t, "quote-test-secret", bill.ID, testQuoteMethodForPath(path), 3000, centsToMicrounits(3000), time.Now()),
			})
			assert.Equalf(t, http.StatusBadRequest, w.Code, "%s %q", path, hash)
		}
	}
	assert.Zero(t, verifier.calls, "malformed hashes must never reach the chain verifier")
}
