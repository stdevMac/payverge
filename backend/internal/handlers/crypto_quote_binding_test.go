package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/blockchain"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// persistTestCryptoQuote writes the crypto_payment_quotes row a signed test
// token points at, with EXACT amount exactMicrounits (offset 1 on top of
// exact-1) so fixture verifiers keep transferring the amount a test names.
// Any other live quote for the same wallet and amount is retired first: tests
// that sign one amount for several bills model quotes issued one after the
// other, and the partial unique index would (correctly) refuse two live ones.
func persistTestCryptoQuote(tb testing.TB, billID uint, method string, exactMicrounits int64, issuedAt time.Time) *database.CryptoPaymentQuote {
	tb.Helper()
	gdb := database.GetDB()
	if !gdb.Migrator().HasTable(&database.CryptoPaymentQuote{}) {
		require.NoError(tb, gdb.AutoMigrate(&database.CryptoPaymentQuote{}))
	}
	var bill database.Bill
	require.NoError(tb, gdb.Select("id", "business_id", "settlement_addr").Where("id = ?", billID).Take(&bill).Error)
	address := database.NormalizeCryptoQuoteAddress(bill.SettlementAddr)
	require.NoError(tb, gdb.Model(&database.CryptoPaymentQuote{}).
		Where("chain_id = ? AND settlement_address = ? AND exact_microunits = ? AND status = ?",
			baseMainnetChainID, address, exactMicrounits, database.CryptoPaymentQuoteStatusActive).
		Update("status", database.CryptoPaymentQuoteStatusExpired).Error)
	issued := issuedAt.UTC().Truncate(time.Second)
	quote := &database.CryptoPaymentQuote{
		BillID:            bill.ID,
		BusinessID:        bill.BusinessID,
		ChainID:           baseMainnetChainID,
		SettlementAddress: address,
		PaymentMethod:     method,
		BaseMicrounits:    exactMicrounits - 1,
		OffsetMicrounits:  1,
		ExactMicrounits:   exactMicrounits,
		Status:            database.CryptoPaymentQuoteStatusActive,
		IssuedAt:          issued,
		ExpiresAt:         issued.Add(cryptoQuoteTTL),
	}
	require.NoError(tb, gdb.Create(quote).Error)
	return quote
}

// assertPayerBoundQuoteAmount checks a quoted exact amount is the converted
// base plus a unique offset of 1..CryptoQuoteMaxOffsetMicrounits (under one
// cent).
func assertPayerBoundQuoteAmount(t testing.TB, baseMicrounits, exactMicrounits int64) {
	t.Helper()
	offset := exactMicrounits - baseMicrounits
	assert.GreaterOrEqualf(t, offset, int64(1), "exact %d must exceed base %d by the unique offset", exactMicrounits, baseMicrounits)
	assert.LessOrEqualf(t, offset, database.CryptoQuoteMaxOffsetMicrounits, "offset of exact %d over base %d must stay below one cent", exactMicrounits, baseMicrounits)
}

// enableGuestCrossChainSettlementForTest turns the disabled cross-chain rail
// back on for tests that keep its dormant code covered.
func enableGuestCrossChainSettlementForTest(t testing.TB) {
	t.Helper()
	prev := guestCrossChainSettlementEnabled
	guestCrossChainSettlementEnabled = true
	t.Cleanup(func() { guestCrossChainSettlementEnabled = prev })
}

// fakeUSDCChain is an in-memory Base chain holding real-looking USDC
// transfers. It verifies the way blockchain.Service does: the receipt is
// looked up by hash, the Transfer must reach the recipient, and the value
// must equal the expected amount (or reach it when allowExcess). A transfer to
// the recipient with another value is ErrTransferAmountMismatch.
type fakeUSDCChain struct {
	mu        sync.Mutex
	transfers map[common.Hash]fakeUSDCTransfer
	calls     int
	// onVerify, when set, runs during each verification: it models time
	// passing while the RPC lookup is in flight.
	onVerify func()
}

type fakeUSDCTransfer struct {
	from      string
	to        string
	amount    int64
	blockTime time.Time
}

func newFakeUSDCChain() *fakeUSDCChain {
	return &fakeUSDCChain{transfers: map[common.Hash]fakeUSDCTransfer{}}
}

// send mines a transfer now and returns its canonical hash.
func (f *fakeUSDCChain) send(label, from, to string, amount int64) string {
	return f.sendAt(label, from, to, amount, time.Now())
}

func (f *fakeUSDCChain) sendAt(label, from, to string, amount int64, at time.Time) string {
	hash := testEVMTxHash(label)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.transfers[common.HexToHash(hash)] = fakeUSDCTransfer{from: from, to: to, amount: amount, blockTime: at}
	return hash
}

func (f *fakeUSDCChain) VerifyUSDCTransfer(ctx context.Context, txHash, recipient string, expected int64) error {
	_, err := f.VerifyUSDCTransferWithEvidence(ctx, txHash, recipient, expected, false)
	return err
}

func (f *fakeUSDCChain) VerifyUSDCTransferAtLeast(ctx context.Context, txHash, recipient string, expected int64) error {
	_, err := f.VerifyUSDCTransferWithEvidence(ctx, txHash, recipient, expected, true)
	return err
}

func (f *fakeUSDCChain) VerifyUSDCTransferWithEvidence(_ context.Context, txHash, recipient string, expected int64, allowExcess bool) (blockchain.USDCTransferEvidence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.onVerify != nil {
		f.onVerify()
	}
	tr, ok := f.transfers[common.HexToHash(txHash)]
	if !ok || !strings.EqualFold(common.HexToAddress(tr.to).Hex(), common.HexToAddress(recipient).Hex()) {
		return blockchain.USDCTransferEvidence{}, fmt.Errorf("no matching USDC transfer found for transaction")
	}
	if (allowExcess && tr.amount < expected) || (!allowExcess && tr.amount != expected) {
		return blockchain.USDCTransferEvidence{}, fmt.Errorf("%w: expected %d micro-units", blockchain.ErrTransferAmountMismatch, expected)
	}
	return blockchain.USDCTransferEvidence{
		From:            common.HexToAddress(tr.from).Hex(),
		To:              common.HexToAddress(tr.to).Hex(),
		AmountBaseUnits: tr.amount,
		TxHash:          txHash,
		ChainID:         baseMainnetChainID,
		Token:           "USDC",
		BlockNumber:     1000,
		BlockHash:       "0x" + strings.Repeat("cd", 32),
		BlockTimestamp:  uint64(tr.blockTime.Unix()),
	}, nil
}

func (f *fakeUSDCChain) ChainID() int64              { return baseMainnetChainID }
func (f *fakeUSDCChain) TokenSymbolForChain() string { return "USDC" }

const (
	bindingVenueWallet = "0x1111111111111111111111111111111111111111"
	bindingGuestA      = "0x000000000000000000000000000000000000a11e"
	bindingGuestB      = "0x000000000000000000000000000000000000b0b0"
)

type issuedBindingQuote struct {
	QuoteID       uint    `json:"quote_id"`
	USDMicrounits int64   `json:"usd_microunits"`
	USDAmount     float64 `json:"usd_amount"`
	QuoteToken    string  `json:"quote_token"`
	ExpiresAt     int64   `json:"expires_at"`
}

type payerBindingFixture struct {
	t        *testing.T
	chain    *fakeUSDCChain
	handler  *PaymentHandler
	router   *gin.Engine
	business *database.Business
}

func newPayerBindingFixture(t *testing.T, suffix string) *payerBindingFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	gormDB := setupPaymentRegressionDB(t)
	require.NoError(t, gormDB.AutoMigrate(&database.PaymentRefundDestination{}))
	business := createPaymentRegressionBusiness(t, suffix, nil, "")
	chain := newFakeUSDCChain()
	handler := NewPaymentHandler(database.GetDBWrapper(), nil, chain, services.NewExchangeRateService(database.GetDBWrapper()))
	router := gin.New()
	router.POST("/guest/bill/:bill_token/crypto-quote", handler.IssueCryptoQuote)
	router.POST("/guest/bill/:bill_token/crypto-payment", handler.ProcessCryptoPayment)
	router.POST("/guest/bill/:bill_token/cross-chain-payment", handler.ProcessCrossChainPayment)
	return &payerBindingFixture{t: t, chain: chain, handler: handler, router: router, business: business}
}

func (f *payerBindingFixture) quote(bill *database.Bill, amount float64) issuedBindingQuote {
	f.t.Helper()
	w := performPaymentRegressionRequest(f.t, f.router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-quote", bill.PublicToken),
		map[string]any{"amount_paid": amount, "tip_amount": 0.0, "payment_method": "usdc_payment"})
	require.Equal(f.t, http.StatusOK, w.Code, w.Body.String())
	var q issuedBindingQuote
	require.NoError(f.t, json.Unmarshal(w.Body.Bytes(), &q))
	return q
}

func (f *payerBindingFixture) pay(bill *database.Bill, amount float64, txHash, quoteToken string) *httptest.ResponseRecorder {
	f.t.Helper()
	return performPaymentRegressionRequest(f.t, f.router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-payment", bill.PublicToken),
		map[string]any{
			"transaction_hash": txHash,
			"amount_paid":      amount,
			"tip_amount":       0.0,
			"payment_method":   "USDC",
			"quote_token":      quoteToken,
		})
}

func (f *payerBindingFixture) quoteRow(id uint) database.CryptoPaymentQuote {
	f.t.Helper()
	var row database.CryptoPaymentQuote
	require.NoError(f.t, database.GetDB().Where("id = ?", id).Take(&row).Error)
	return row
}

func (f *payerBindingFixture) cryptoPaymentCount() int64 {
	f.t.Helper()
	var count int64
	require.NoError(f.t, database.GetDB().Model(&database.Payment{}).Where("payment_method = ?", "crypto").Count(&count).Error)
	return count
}

func requireBillUnpaid(t *testing.T, bill *database.Bill) {
	t.Helper()
	reloaded, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), reloaded.PaidAmount, "bill %d must not be settled", bill.ID)
	assert.Equal(t, database.BillStatusOpen, reloaded.Status)
}

// TestIssueCryptoQuote_PersistsExactAmountBoundToQuoteRow pins the issuance
// contract: the response amount is the converted amount plus a sub-cent
// offset, it equals the persisted row's exact amount, and the token is bound
// to that row.
func TestIssueCryptoQuote_PersistsExactAmountBoundToQuoteRow(t *testing.T) {
	f := newPayerBindingFixture(t, "binding-issue")
	bill := createPaymentRegressionBill(t, f.business.ID, 30)

	q := f.quote(bill, 30)
	require.NotZero(t, q.QuoteID)
	base := centsToMicrounits(3000)
	assert.Greater(t, q.USDMicrounits, base, "the exact amount carries a unique offset")
	assert.LessOrEqual(t, q.USDMicrounits, base+database.CryptoQuoteMaxOffsetMicrounits, "the offset stays below one cent")
	assert.InDelta(t, float64(q.USDMicrounits)/1_000_000, q.USDAmount, 1e-9)

	row := f.quoteRow(q.QuoteID)
	assert.Equal(t, bill.ID, row.BillID)
	assert.Equal(t, f.business.ID, row.BusinessID)
	assert.Equal(t, base, row.BaseMicrounits)
	assert.Equal(t, q.USDMicrounits, row.ExactMicrounits)
	assert.Equal(t, strings.ToLower(bindingVenueWallet), row.SettlementAddress)
	assert.Equal(t, database.CryptoPaymentQuoteStatusActive, row.Status)
	assert.Equal(t, q.ExpiresAt, row.ExpiresAt.Unix())

	claims, err := parseCryptoQuote(q.QuoteToken, []byte("quote-test-secret"), time.Now())
	require.NoError(t, err)
	assert.Equal(t, q.QuoteID, claims.QuoteID)
	assert.Equal(t, q.USDMicrounits, claims.USDMicrounits)
	assert.Equal(t, row.IssuedAt.Unix(), claims.Iat)
	assert.Equal(t, int64(3000), claims.LocalCents, "the ledger amount stays in cents; the offset is never booked")
}

// (a) A transfer somebody else made cannot settle a quote unless it carries
// that quote's exact amount; round amounts, overpayments and another guest's
// exact amount are all refused without settling.
func TestProcessCryptoPayment_ThirdPartyTransferWithOtherAmountCannotSettle(t *testing.T) {
	f := newPayerBindingFixture(t, "binding-third-party")
	billA := createPaymentRegressionBill(t, f.business.ID, 30)
	billB := createPaymentRegressionBill(t, f.business.ID, 30)
	quoteA := f.quote(billA, 30)
	quoteB := f.quote(billB, 30)
	require.NotEqual(t, quoteA.USDMicrounits, quoteB.USDMicrounits, "two live quotes to one wallet never share an amount")

	// Guest A pays bill A exactly. Guest B then presents A's transfer for B.
	txA := f.chain.send("guest-a-pays-a", bindingGuestA, bindingVenueWallet, quoteA.USDMicrounits)
	w := f.pay(billB, 30, txA, quoteB.QuoteToken)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "amount_mismatch")
	requireBillUnpaid(t, billB)

	// A round $30.00 transfer (no offset) and an overpayment do not settle B.
	round := f.chain.send("third-party-round", bindingGuestB, bindingVenueWallet, centsToMicrounits(3000))
	over := f.chain.send("third-party-over", bindingGuestB, bindingVenueWallet, quoteB.USDMicrounits+1)
	for _, hash := range []string{round, over} {
		w = f.pay(billB, 30, hash, quoteB.QuoteToken)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), "amount_mismatch")
	}
	requireBillUnpaid(t, billB)

	// Review c1 L1: the guest is told to ask staff, so staff get a record of
	// every refused transfer on the bill (first as headline, rest as trail).
	var alerts []database.OperationalAlert
	require.NoError(t, database.GetDB().Where("business_id = ? AND resource_id = ?", f.business.ID, billB.ID).Find(&alerts).Error)
	require.Len(t, alerts, 1, "one bill-scoped review alert for the refused transfers")
	assert.Equal(t, database.OperationalAlertTypePaymentRefundReview, alerts[0].AlertType)
	assert.Equal(t, database.OperationalAlertResourceTypeBill, alerts[0].ResourceType)
	assert.Equal(t, database.OperationalAlertPriorityHigh, alerts[0].Priority)
	var alertMeta map[string]any
	require.NoError(t, json.Unmarshal(alerts[0].Metadata, &alertMeta))
	assert.Equal(t, "amount_mismatch", alertMeta["reason"])
	assert.Equal(t, txA, alertMeta["transaction_hash"])
	assert.EqualValues(t, quoteB.QuoteID, alertMeta["quote_id"])
	for _, hash := range []string{round, over} {
		assert.Contains(t, string(alerts[0].Metadata), hash, "every refused transfer stays on the alert")
	}
	assert.NotContains(t, string(alerts[0].Metadata), "microunits", "amounts stay out of alert metadata")
	var alertsOnA int64
	require.NoError(t, database.GetDB().Model(&database.OperationalAlert{}).Where("resource_id = ? AND alert_type = ?", billA.ID, database.OperationalAlertTypePaymentRefundReview).Count(&alertsOnA).Error)
	assert.Zero(t, alertsOnA)

	assert.Equal(t, database.CryptoPaymentQuoteStatusActive, f.quoteRow(quoteB.QuoteID).Status, "a refused transfer must not burn the quote")
	assert.Zero(t, f.cryptoPaymentCount())

	// The real payer still settles with the transfer that matches their quote.
	w = f.pay(billA, 30, txA, quoteA.QuoteToken)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	rowA := f.quoteRow(quoteA.QuoteID)
	assert.Equal(t, database.CryptoPaymentQuoteStatusConsumed, rowA.Status)
	require.NotNil(t, rowA.ConsumedTxHash)
	assert.Equal(t, txA, *rowA.ConsumedTxHash)
	require.NotNil(t, rowA.PaymentID)
	assert.Equal(t, int64(1), f.cryptoPaymentCount())

	// Guest B's own exact transfer settles B.
	txB := f.chain.send("guest-b-pays-b", bindingGuestB, bindingVenueWallet, quoteB.USDMicrounits)
	w = f.pay(billB, 30, txB, quoteB.QuoteToken)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

// (b) A quote settles at most once: an idempotent retry of the same transfer
// is fine, but a second, different transfer of the same exact amount cannot
// reuse it.
func TestProcessCryptoPayment_QuoteCannotBeUsedTwice(t *testing.T) {
	f := newPayerBindingFixture(t, "binding-single-use")
	bill := createPaymentRegressionBill(t, f.business.ID, 90) // partial payments keep the bill open
	q := f.quote(bill, 30)

	tx1 := f.chain.send("single-use-1", bindingGuestA, bindingVenueWallet, q.USDMicrounits)
	w := f.pay(bill, 30, tx1, q.QuoteToken)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	w = f.pay(bill, 30, tx1, q.QuoteToken)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Payment already processed")

	tx2 := f.chain.send("single-use-2", bindingGuestB, bindingVenueWallet, q.USDMicrounits)
	callsBefore := f.chain.calls
	w = f.pay(bill, 30, tx2, q.QuoteToken)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "crypto_quote_used")
	assert.Equal(t, callsBefore, f.chain.calls, "a used quote is refused before any chain lookup")

	reloaded, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3000), reloaded.PaidAmount)
	assert.Equal(t, int64(1), f.cryptoPaymentCount())
}

// (c) An expired quote cannot settle, even while its signed token is still
// inside the HMAC expiry: the persisted row is authoritative.
func TestProcessCryptoPayment_ExpiredQuoteCannotSettle(t *testing.T) {
	f := newPayerBindingFixture(t, "binding-expired")

	for name, expire := range map[string]func(id uint){
		"expires_at_passed": func(id uint) {
			require.NoError(t, database.GetDB().Model(&database.CryptoPaymentQuote{}).Where("id = ?", id).
				Update("expires_at", time.Now().UTC().Add(-time.Second)).Error)
		},
		"status_expired": func(id uint) {
			require.NoError(t, database.GetDB().Model(&database.CryptoPaymentQuote{}).Where("id = ?", id).
				Update("status", database.CryptoPaymentQuoteStatusExpired).Error)
		},
	} {
		t.Run(name, func(t *testing.T) {
			bill := createPaymentRegressionBill(t, f.business.ID, 30)
			q := f.quote(bill, 30)
			tx := f.chain.send("expired-"+name, bindingGuestA, bindingVenueWallet, q.USDMicrounits)
			expire(q.QuoteID)
			callsBefore := f.chain.calls

			w := f.pay(bill, 30, tx, q.QuoteToken)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), "crypto_quote_expired")
			assert.Equal(t, callsBefore, f.chain.calls)
			requireBillUnpaid(t, bill)
		})
	}

	t.Run("token_without_quote_row", func(t *testing.T) {
		bill := createPaymentRegressionBill(t, f.business.ID, 30)
		rowless := signCryptoQuote(cryptoQuoteClaims{
			BusinessID:     f.business.ID,
			BillID:         bill.ID,
			LocalCents:     3000,
			USDMicrounits:  centsToMicrounits(3000),
			SettlementAddr: bill.SettlementAddr,
			ChainID:        baseMainnetChainID,
			Token:          "USDC",
			PaymentMethod:  guestPaymentMethodUSDC,
			Iat:            time.Now().Unix(),
			Exp:            time.Now().Add(cryptoQuoteTTL).Unix(),
		}, []byte("quote-test-secret"))
		tx := f.chain.send("rowless-token", bindingGuestA, bindingVenueWallet, centsToMicrounits(3000))
		w := f.pay(bill, 30, tx, rowless)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), "crypto_quote_expired", "a token without a quote row must re-quote")
		requireBillUnpaid(t, bill)
	})

	t.Run("quote_row_of_another_bill", func(t *testing.T) {
		billA := createPaymentRegressionBill(t, f.business.ID, 30)
		billB := createPaymentRegressionBill(t, f.business.ID, 30)
		qa := f.quote(billA, 30)
		forged := signCryptoQuote(cryptoQuoteClaims{
			BusinessID:     f.business.ID,
			QuoteID:        qa.QuoteID,
			BillID:         billB.ID,
			LocalCents:     3000,
			USDMicrounits:  qa.USDMicrounits,
			SettlementAddr: billB.SettlementAddr,
			ChainID:        baseMainnetChainID,
			Token:          "USDC",
			PaymentMethod:  guestPaymentMethodUSDC,
			Iat:            time.Now().Unix(),
			Exp:            time.Now().Add(cryptoQuoteTTL).Unix(),
		}, []byte("quote-test-secret"))
		tx := f.chain.send("cross-bill-row", bindingGuestA, bindingVenueWallet, qa.USDMicrounits)
		w := f.pay(billB, 30, tx, forged)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), "quote_invalid")
		requireBillUnpaid(t, billB)
	})

	t.Run("transfer_mined_before_quote", func(t *testing.T) {
		bill := createPaymentRegressionBill(t, f.business.ID, 30)
		q := f.quote(bill, 30)
		tx := f.chain.sendAt("mined-before-quote", bindingGuestA, bindingVenueWallet, q.USDMicrounits, time.Now().Add(-10*time.Minute))
		w := f.pay(bill, 30, tx, q.QuoteToken)
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), "transfer_predates_quote")
		requireBillUnpaid(t, bill)
	})
}

// (d) Quotes issued concurrently for one receiving wallet each get their own
// exact amount, whether they are for one bill or many.
func TestIssueCryptoQuote_ConcurrentQuotesForOneWalletGetDistinctExactAmounts(t *testing.T) {
	f := newPayerBindingFixture(t, "binding-concurrent")
	const perBill = 4
	bills := []*database.Bill{
		createPaymentRegressionBill(t, f.business.ID, 30),
		createPaymentRegressionBill(t, f.business.ID, 30),
		createPaymentRegressionBill(t, f.business.ID, 30),
	}

	type result struct {
		code int
		body []byte
	}
	results := make([]result, len(bills)*perBill)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			bill := bills[i%len(bills)]
			payload, _ := json.Marshal(map[string]any{"amount_paid": 30.0, "tip_amount": 0.0, "payment_method": "usdc_payment"})
			req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/guest/bill/%s/crypto-quote", bill.PublicToken), bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			<-start
			f.router.ServeHTTP(w, req)
			results[i] = result{code: w.Code, body: w.Body.Bytes()}
		}(i)
	}
	close(start)
	wg.Wait()

	amounts := map[int64]uint{}
	ids := map[uint]bool{}
	for _, r := range results {
		require.Equal(t, http.StatusOK, r.code, string(r.body))
		var q issuedBindingQuote
		require.NoError(t, json.Unmarshal(r.body, &q))
		require.Falsef(t, ids[q.QuoteID], "quote id %d issued twice", q.QuoteID)
		ids[q.QuoteID] = true
		prev, dup := amounts[q.USDMicrounits]
		require.Falsef(t, dup, "quotes %d and %d share exact amount %d", prev, q.QuoteID, q.USDMicrounits)
		amounts[q.USDMicrounits] = q.QuoteID
	}

	var active int64
	require.NoError(t, database.GetDB().Model(&database.CryptoPaymentQuote{}).
		Where("settlement_address = ? AND status = ?", strings.ToLower(bindingVenueWallet), database.CryptoPaymentQuoteStatusActive).
		Count(&active).Error)
	assert.Equal(t, int64(len(results)), active)
}

// quoteFrom requests a quote as a client at remoteAddr (the identity the
// per-client cap keys on, as the guest write rate limiter does).
func (f *payerBindingFixture) quoteFrom(bill *database.Bill, amount float64, remoteAddr string) *httptest.ResponseRecorder {
	f.t.Helper()
	payload, _ := json.Marshal(map[string]any{"amount_paid": amount, "tip_amount": 0.0, "payment_method": "usdc_payment"})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/guest/bill/%s/crypto-quote", bill.PublicToken), bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	return w
}

// Review c1 L3: anyone holding the bill link could mint the bill's whole
// quote allowance in under a minute and lock every other payer out for a
// quote TTL. One address now stops at the per-client cap and the other payers
// on the bill still get quotes; the stored key is a keyed hash, not the IP.
func TestIssueCryptoQuote_OneClientCannotTakeEveryQuoteSlotOfABill(t *testing.T) {
	f := newPayerBindingFixture(t, "binding-client-cap")
	bill := createPaymentRegressionBill(t, f.business.ID, 30)
	const griefer, payer = "198.51.100.7:40000", "203.0.113.9:40000"

	for i := int64(0); i < database.CryptoQuoteMaxActivePerClient; i++ {
		w := f.quoteFrom(bill, 1, griefer)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	w := f.quoteFrom(bill, 1, griefer)
	require.Equal(t, http.StatusTooManyRequests, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "crypto_quote_limit")

	w = f.quoteFrom(bill, 30, payer)
	require.Equal(t, http.StatusOK, w.Code, "another payer on the bill must still get a quote: %s", w.Body.String())
	var q issuedBindingQuote
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &q))

	row := f.quoteRow(q.QuoteID)
	require.NotNil(t, row.ClientKey)
	assert.Equal(t, guestCryptoQuoteClientKey([]byte("quote-test-secret"), "203.0.113.9"), *row.ClientKey)
	assert.NotContains(t, *row.ClientKey, "203.0.113.9", "the quote row never stores the client IP")
}

func TestGuestCryptoQuoteClientKey(t *testing.T) {
	secret := []byte("quote-test-secret")
	key := guestCryptoQuoteClientKey(secret, "203.0.113.9")
	assert.Regexp(t, `^[0-9a-f]{32}$`, key, "matches the crypto_payment_quotes_client_key_chk shape")
	assert.Equal(t, key, guestCryptoQuoteClientKey(secret, " 203.0.113.9 "))
	assert.NotEqual(t, key, guestCryptoQuoteClientKey(secret, "203.0.113.10"))
	assert.NotEqual(t, key, guestCryptoQuoteClientKey([]byte("another-secret"), "203.0.113.9"),
		"the key depends on the quote secret, so it cannot be recomputed from the IP alone")
	assert.Empty(t, guestCryptoQuoteClientKey(secret, ""), "no address means no per-client cap")
}

// Review c1 L4: a quote that lapses while the transfer is being verified is
// reported as expired, not as "already used for another transaction". The
// transfer verified, so the settlement-review alert still fires.
func TestProcessCryptoPayment_QuoteExpiringDuringVerifyIsReportedExpired(t *testing.T) {
	f := newPayerBindingFixture(t, "binding-expire-mid-verify")
	bill := createPaymentRegressionBill(t, f.business.ID, 30)
	q := f.quote(bill, 30)
	tx := f.chain.send("expire-mid-verify", bindingGuestA, bindingVenueWallet, q.USDMicrounits)

	f.chain.onVerify = func() {
		require.NoError(t, database.GetDB().Model(&database.CryptoPaymentQuote{}).
			Where("id = ?", q.QuoteID).
			Update("expires_at", time.Now().Add(-time.Second)).Error)
	}
	w := f.pay(bill, 30, tx, q.QuoteToken)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "crypto_quote_expired")
	assert.NotContains(t, w.Body.String(), "crypto_quote_used")
	requireBillUnpaid(t, bill)
	assert.Zero(t, f.cryptoPaymentCount())
	assert.Equal(t, database.CryptoPaymentQuoteStatusActive, f.quoteRow(q.QuoteID).Status, "rolled back with the payment")

	var alert database.OperationalAlert
	require.NoError(t, database.GetDB().Where("business_id = ? AND resource_id = ?", f.business.ID, bill.ID).Take(&alert).Error)
	assert.Equal(t, database.OperationalAlertPriorityUrgent, alert.Priority, "a verified transfer that did not settle still needs review")
	assert.Contains(t, string(alert.Metadata), tx)
}

// (e) A verified transfer already recorded on another bill is the replay
// signature: it must not settle, it must page the operator through the
// existing alert pipeline, and it must leave a structured warning. This case
// reaches the ledger's tx-hash conflict (B's quote carries A's exact amount
// and postdates the transfer); the realistic replays, refused before the
// ledger, are covered by TestProcessCryptoPayment_ReplayOfRecordedTransfer*.
func TestProcessCryptoPayment_TxHashConflictRaisesOperatorAlert(t *testing.T) {
	f := newPayerBindingFixture(t, "binding-conflict")
	billA := createPaymentRegressionBill(t, f.business.ID, 30)
	billB := createPaymentRegressionBill(t, f.business.ID, 30)

	quoteA := f.quote(billA, 30)
	tx := f.chain.send("conflict-a", bindingGuestA, bindingVenueWallet, quoteA.USDMicrounits)
	require.Equal(t, http.StatusOK, f.pay(billA, 30, tx, quoteA.QuoteToken).Code)

	// Once A's quote is consumed its amount may (after the cooldown) be quoted
	// again; model the worst case where B holds that same exact amount.
	tokenB := signTestQuote(t, "quote-test-secret", billB.ID, 3000, quoteA.USDMicrounits)
	claimsB, err := parseCryptoQuote(tokenB, []byte("quote-test-secret"), time.Now())
	require.NoError(t, err)

	logs := captureWarnLogs(t)

	w := f.pay(billB, 30, tx, tokenB)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Equal(t, "crypto_tx_already_recorded", responseErrorCode(t, w))
	requireBillUnpaid(t, billB)
	assert.Equal(t, database.CryptoPaymentQuoteStatusActive, f.quoteRow(claimsB.QuoteID).Status, "the conflicting settlement rolled back, quote untouched")
	assert.Equal(t, int64(1), f.cryptoPaymentCount())

	var alerts []database.OperationalAlert
	require.NoError(t, database.GetDB().Where("business_id = ? AND resource_id = ?", f.business.ID, billB.ID).Find(&alerts).Error)
	require.Len(t, alerts, 1, "exactly one operator alert for the replayed transfer")
	assert.Equal(t, database.OperationalAlertPriorityHigh, alerts[0].Priority)
	assert.Contains(t, string(alerts[0].Metadata), "tx_hash_conflict")
	assert.Contains(t, string(alerts[0].Metadata), tx)
	assert.Contains(t, alerts[0].Body, billA.BillNumber, "the alert names the bill the transfer already paid")

	var entry map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &entry), logs.String())
	assert.Equal(t, "crypto_payment_tx_hash_conflict", entry["msg"])
	assert.Equal(t, "WARN", entry["level"])
	assert.Equal(t, tx, entry["tx_hash"])
	assert.EqualValues(t, billB.ID, entry["bill_id"])
	assert.EqualValues(t, claimsB.QuoteID, entry["quote_id"])
}

func responseErrorCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
	return body.Code
}

func captureWarnLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	prevLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prevLogger) })
	return &logs
}

func billReviewAlerts(t *testing.T, businessID, billID uint) []database.OperationalAlert {
	t.Helper()
	var alerts []database.OperationalAlert
	require.NoError(t, database.GetDB().
		Where("business_id = ? AND resource_id = ? AND alert_type = ?", businessID, billID, database.OperationalAlertTypePaymentRefundReview).
		Find(&alerts).Error)
	return alerts
}

// Review c1 R2-M1: the realistic replay. Guest A pays bill A with the exact
// amount of A's real quote; someone then presents A's transfer for bill B
// under B's own real quote. The unique offsets differ, so the transfer does
// not carry B's amount, but it must be reported as a transfer already
// recorded on bill A (tx_hash_conflict), never as a wrong-amount transfer
// that invites staff to settle or refund it by hand.
func TestProcessCryptoPayment_ReplayOfRecordedTransferWithRealQuoteIsAConflict(t *testing.T) {
	f := newPayerBindingFixture(t, "binding-replay-real-quote")
	billA := createPaymentRegressionBill(t, f.business.ID, 30)
	billB := createPaymentRegressionBill(t, f.business.ID, 30)
	quoteA := f.quote(billA, 30)
	quoteB := f.quote(billB, 30)
	require.NotEqual(t, quoteA.USDMicrounits, quoteB.USDMicrounits)

	txA := f.chain.send("replay-real-a", bindingGuestA, bindingVenueWallet, quoteA.USDMicrounits)
	require.Equal(t, http.StatusOK, f.pay(billA, 30, txA, quoteA.QuoteToken).Code)

	logs := captureWarnLogs(t)
	// Any spelling of the recorded hash is the same transfer.
	for _, spelling := range []string{txA, "0X" + strings.ToUpper(txA[2:])} {
		w := f.pay(billB, 30, spelling, quoteB.QuoteToken)
		require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
		assert.Equal(t, "crypto_tx_already_recorded", responseErrorCode(t, w))
		assert.NotContains(t, w.Body.String(), "amount_mismatch")
	}
	requireBillUnpaid(t, billB)
	assert.Equal(t, database.CryptoPaymentQuoteStatusActive, f.quoteRow(quoteB.QuoteID).Status, "a refused replay must not burn the quote")
	assert.Equal(t, int64(1), f.cryptoPaymentCount())

	alerts := billReviewAlerts(t, f.business.ID, billB.ID)
	require.Len(t, alerts, 1, "one bill-scoped review alert for the replay")
	assert.Equal(t, database.OperationalAlertPriorityHigh, alerts[0].Priority)
	assert.Equal(t, "Crypto payment reused on another bill", alerts[0].Title)
	assert.Contains(t, alerts[0].Body, "already paid bill "+billA.BillNumber)
	assert.NotContains(t, alerts[0].Body, "wrong amount")
	var meta map[string]any
	require.NoError(t, json.Unmarshal(alerts[0].Metadata, &meta))
	assert.Equal(t, "tx_hash_conflict", meta["reason"], "a replay is not a wrong-amount transfer")
	assert.Equal(t, txA, meta["transaction_hash"])
	assert.EqualValues(t, billA.ID, meta["recorded_bill_id"])
	assert.Equal(t, billA.BillNumber, meta["recorded_bill_number"])
	assert.EqualValues(t, quoteB.QuoteID, meta["quote_id"])
	assert.NotContains(t, meta, "amount_cents", "amounts stay out of alert metadata")
	assert.Empty(t, billReviewAlerts(t, f.business.ID, billA.ID), "the bill the transfer paid needs no review")

	var entry map[string]any
	firstLine, _, _ := bytes.Cut(bytes.TrimSpace(logs.Bytes()), []byte("\n"))
	require.NoError(t, json.Unmarshal(firstLine, &entry), logs.String())
	assert.Equal(t, "crypto_payment_tx_hash_conflict", entry["msg"])
	assert.Equal(t, txA, entry["tx_hash"])
	assert.EqualValues(t, billB.ID, entry["bill_id"])
	assert.EqualValues(t, billA.ID, entry["recorded_bill_id"])
	assert.NotContains(t, logs.String(), "crypto_payment_amount_mismatch")

	// B's real payer still settles B with their own exact transfer.
	txB := f.chain.send("replay-real-b", bindingGuestB, bindingVenueWallet, quoteB.USDMicrounits)
	require.Equal(t, http.StatusOK, f.pay(billB, 30, txB, quoteB.QuoteToken).Code)
}

// A transfer replayed on the bill it already paid, under a second quote of
// that bill, is a duplicate presentation: refused, not counted twice, and
// filed as "presented twice" rather than as a wrong-amount transfer.
func TestProcessCryptoPayment_ReplayOfRecordedTransferOnItsOwnBillIsAConflict(t *testing.T) {
	f := newPayerBindingFixture(t, "binding-replay-same-bill")
	bill := createPaymentRegressionBill(t, f.business.ID, 90) // partial payments keep the bill open
	first := f.quote(bill, 30)
	tx := f.chain.send("replay-same-bill", bindingGuestA, bindingVenueWallet, first.USDMicrounits)
	require.Equal(t, http.StatusOK, f.pay(bill, 30, tx, first.QuoteToken).Code)

	second := f.quote(bill, 30)
	w := f.pay(bill, 30, tx, second.QuoteToken)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Equal(t, "crypto_tx_already_recorded", responseErrorCode(t, w))

	reloaded, _, err := database.GetBillByID(bill.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3000), reloaded.PaidAmount, "the transfer is counted once")
	assert.Equal(t, int64(1), f.cryptoPaymentCount())
	assert.Equal(t, database.CryptoPaymentQuoteStatusActive, f.quoteRow(second.QuoteID).Status)

	alerts := billReviewAlerts(t, f.business.ID, bill.ID)
	require.Len(t, alerts, 1)
	assert.Equal(t, "Crypto payment presented twice", alerts[0].Title)
	assert.Contains(t, string(alerts[0].Metadata), "tx_hash_conflict")

	// The original quote's idempotent retry is still a plain success.
	w = f.pay(bill, 30, tx, first.QuoteToken)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Payment already processed")
}

// Once a consumed quote's amount leaves the reuse cooldown it can be quoted
// again. A recorded transfer replayed under that later quote carries the
// right amount but predates the quote; it is still reported as already
// recorded on the bill it paid, not as a stale transfer.
func TestProcessCryptoPayment_ReplayOfRecordedTransferUnderReusedAmountIsAConflict(t *testing.T) {
	f := newPayerBindingFixture(t, "binding-replay-reused-amount")
	billA := createPaymentRegressionBill(t, f.business.ID, 30)
	billB := createPaymentRegressionBill(t, f.business.ID, 30)
	exact := centsToMicrounits(3000) + 77
	issuedA := time.Now().Add(-25 * time.Minute)
	tokenA := signTestQuoteIssuedAt(t, "quote-test-secret", billA.ID, 3000, exact, issuedA)
	txA := f.chain.sendAt("replay-reused-a", bindingGuestA, bindingVenueWallet, exact, issuedA.Add(time.Minute))
	require.Equal(t, http.StatusOK, f.pay(billA, 30, txA, tokenA).Code)

	tokenB := signTestQuote(t, "quote-test-secret", billB.ID, 3000, exact)
	w := f.pay(billB, 30, txA, tokenB)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Equal(t, "crypto_tx_already_recorded", responseErrorCode(t, w))
	assert.NotContains(t, w.Body.String(), "transfer_predates_quote")
	requireBillUnpaid(t, billB)

	alerts := billReviewAlerts(t, f.business.ID, billB.ID)
	require.Len(t, alerts, 1)
	assert.Contains(t, string(alerts[0].Metadata), "tx_hash_conflict")
	assert.Contains(t, alerts[0].Body, billA.BillNumber)

	// A stale transfer nobody recorded keeps the predates answer and no alert.
	billC := createPaymentRegressionBill(t, f.business.ID, 30)
	stale := f.chain.sendAt("stale-unrecorded", bindingGuestB, bindingVenueWallet, exact+1, time.Now().Add(-20*time.Minute))
	tokenC := signTestQuote(t, "quote-test-secret", billC.ID, 3000, exact+1)
	w = f.pay(billC, 30, stale, tokenC)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.Equal(t, "transfer_predates_quote", responseErrorCode(t, w))
	assert.Empty(t, billReviewAlerts(t, f.business.ID, billC.ID))
}

// A wrong-amount transfer is only filed as one once the ledger confirms it is
// not a recorded payment; when that lookup fails the guest gets a retryable
// answer and no alert is filed under the wrong reason.
func TestProcessCryptoPayment_AmountMismatchFailsClosedWhenRecordedLookupFails(t *testing.T) {
	f := newPayerBindingFixture(t, "binding-replay-lookup-down")
	bill := createPaymentRegressionBill(t, f.business.ID, 30)
	q := f.quote(bill, 30)
	wrong := f.chain.send("lookup-down", bindingGuestA, bindingVenueWallet, q.USDMicrounits+5)
	// Break only the holder lookup: the payments table disappears once the
	// verifier has run.
	f.chain.onVerify = func() {
		require.NoError(t, database.GetDB().Exec("ALTER TABLE payments RENAME TO payments_offline").Error)
	}
	t.Cleanup(func() {
		_ = database.GetDB().Exec("ALTER TABLE payments_offline RENAME TO payments").Error
	})
	w := f.pay(bill, 30, wrong, q.QuoteToken)
	require.Equal(t, http.StatusServiceUnavailable, w.Code, w.Body.String())
	assert.Equal(t, "verification_unavailable", responseErrorCode(t, w))
	assert.Empty(t, billReviewAlerts(t, f.business.ID, bill.ID))
	assert.Equal(t, database.CryptoPaymentQuoteStatusActive, f.quoteRow(q.QuoteID).Status)
}

// Cross-chain settlement cannot be payer-bound (bridged amount is variable,
// the on-chain sender is the bridge), so the rail is explicitly disabled:
// no quote, no settlement, not offered to guests.
func TestGuestCrossChainRail_IsDisabledWithClearError(t *testing.T) {
	f := newPayerBindingFixture(t, "binding-cross-chain-off")
	bill := createPaymentRegressionBill(t, f.business.ID, 30)

	w := performPaymentRegressionRequest(t, f.router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/crypto-quote", bill.PublicToken),
		map[string]any{"amount_paid": 30.0, "tip_amount": 0.0, "payment_method": "cross_chain_payment"})
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "plugin_unavailable")
	assert.Contains(t, w.Body.String(), "Cross-chain payments are disabled")
	var quotes int64
	require.NoError(t, database.GetDB().Model(&database.CryptoPaymentQuote{}).Count(&quotes).Error)
	assert.Zero(t, quotes)

	tx := f.chain.send("bridged", bindingGuestA, bindingVenueWallet, centsToMicrounits(3000))
	w = performPaymentRegressionRequest(t, f.router, http.MethodPost,
		fmt.Sprintf("/guest/bill/%s/cross-chain-payment", bill.PublicToken),
		map[string]any{
			"transaction_hash": tx,
			"amount_paid":      30.0,
			"tip_amount":       0.0,
			"source_chain":     "ethereum",
			"source_token":     "USDC",
			"quote_token":      signTestCrossChainQuote(t, "quote-test-secret", bill.ID, 3000, centsToMicrounits(3000)),
		})
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "plugin_unavailable")
	assert.Zero(t, f.chain.calls, "a disabled rail never reaches the chain verifier")
	requireBillUnpaid(t, bill)

	assert.False(t, guestPaymentOptionVisible("cross_chain_payment"))
	assert.True(t, guestPaymentOptionVisible("usdc_payment"))

	// The guest gate and the operator catalog (seeded coming-soon while the
	// rail is off) read one source, so they cannot drift apart.
	assert.Equal(t, services.CrossChainGuestSettlementAvailable, guestCrossChainSettlementEnabled)
}
