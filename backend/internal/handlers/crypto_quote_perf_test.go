package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/blockchain"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/logger"
)

// BenchmarkIssueCryptoQuoteSQLite measures guest quote issuance end to end
// (bill lookup, payable/subscription/plugin gates, settlement contract, and —
// once quotes are persisted — the unique-amount reservation). Quote rows are
// cleared between iterations outside the timer so the benchmark measures a
// steady state of one active quote per wallet instead of exhausting the
// 9999-slot offset space.
func BenchmarkIssueCryptoQuoteSQLite(b *testing.B) {
	gin.SetMode(gin.TestMode)
	b.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	_, billToken, _ := setupCryptoPaymentPerfDB(b, logger.Default.LogMode(logger.Silent))
	gormDB := database.GetDB()
	if err := gormDB.AutoMigrate(&database.DeliveryOrder{}); err != nil {
		b.Fatalf("migrate delivery orders: %v", err)
	}
	hasQuoteTable := gormDB.Migrator().HasTable("crypto_payment_quotes")
	handler := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, services.NewExchangeRateService(database.GetDBWrapper()))
	payload, _ := json.Marshal(map[string]any{"amount_paid": 1.0, "tip_amount": 0.0, "payment_method": "usdc_payment"})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if hasQuoteTable {
			b.StopTimer()
			if err := gormDB.Exec("DELETE FROM crypto_payment_quotes").Error; err != nil {
				b.Fatalf("reset quotes: %v", err)
			}
			b.StartTimer()
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "bill_token", Value: billToken}}
		c.Request = httptest.NewRequest(http.MethodPost, "/guest/bill/"+billToken+"/crypto-quote", bytes.NewReader(payload))
		c.Request.Header.Set("Content-Type", "application/json")
		handler.IssueCryptoQuote(c)
		if w.Code != http.StatusOK {
			b.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
		}
	}
}

// quoteStatements returns the recorded statements that touch the quote table,
// whitespace-normalized and lower-cased, grouped by leading verb.
func quoteStatements(r *paymentDetailsSQLRecorder) map[string][]string {
	byVerb := map[string][]string{}
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		if !strings.Contains(normalized, "crypto_payment_quotes") {
			continue
		}
		verb, _, _ := strings.Cut(normalized, " ")
		byVerb[verb] = append(byVerb[verb], normalized)
	}
	return byVerb
}

// TestCryptoQuoteBindingAccessShape pins the database cost payer binding adds
// to the two guest crypto routes, so a later change cannot quietly turn the
// quote table into a per-request history scan:
//   - issuance: one stale-slot UPDATE, one per-bill COUNT, one taken-amount
//     read bounded to the 9999-wide offset window (exact_microunits column
//     only), and one INSERT;
//   - settlement: one primary-key read of the quote and one conditional
//     consume UPDATE inside the settlement transaction.
func TestCryptoQuoteBindingAccessShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("CRYPTO_QUOTE_SECRET", "quote-test-secret")
	recorder := &paymentDetailsSQLRecorder{Interface: logger.Default.LogMode(logger.Silent)}
	handler, billToken, billID := setupCryptoPaymentPerfDB(t, recorder)
	require.NoError(t, database.GetDB().AutoMigrate(&database.DeliveryOrder{}))
	issuer := NewPaymentHandler(database.GetDBWrapper(), nil, &mockGuestPaymentVerifier{}, services.NewExchangeRateService(database.GetDBWrapper()))

	payload, _ := json.Marshal(map[string]any{"amount_paid": 1.0, "tip_amount": 0.0, "payment_method": "usdc_payment"})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "bill_token", Value: billToken}}
	c.Request = httptest.NewRequest(http.MethodPost, "/guest/bill/"+billToken+"/crypto-quote", bytes.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	recorder.statements = nil
	issuer.IssueCryptoQuote(c)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	issued := quoteStatements(recorder)
	assert.Len(t, issued["update"], 1, "issuance expires the wallet's lapsed slots once: %v", issued["update"])
	assert.Len(t, issued["insert"], 1, "issuance inserts one quote row: %v", issued["insert"])
	require.Len(t, issued["select"], 2, "issuance reads the per-bill count and the taken amounts once each: %v", issued["select"])
	for _, statement := range issued["select"] {
		assert.NotContains(t, statement, "select *", "issuance must not hydrate quote rows")
	}
	var takenRead string
	for _, statement := range issued["select"] {
		if strings.Contains(statement, "exact_microunits") && !strings.Contains(statement, "count(") {
			takenRead = statement
		}
	}
	require.NotEmpty(t, takenRead, "taken-amount read not found in %v", issued["select"])
	assert.Contains(t, takenRead, "between", "taken-amount read must stay inside the offset window")

	token := signTestQuote(t, "quote-test-secret", billID, 100, centsToMicrounits(100))
	recorder.statements = nil
	w = performCryptoPaymentPerfRequest(handler, billToken, testEVMTxHash("tx-quote-access-shape"), token)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	settled := quoteStatements(recorder)
	require.Len(t, settled["select"], 1, "settlement loads the bound quote once: %v", settled["select"])
	assert.Contains(t, settled["select"][0], "where id =", "the bound quote is loaded by primary key")
	require.Len(t, settled["update"], 1, "settlement consumes the quote with one conditional UPDATE: %v", settled["update"])
	assert.Contains(t, settled["update"][0], "and status = ", "the consume UPDATE is conditional on the quote still being active")
	assert.Contains(t, settled["update"][0], "and expires_at > ", "the consume UPDATE is conditional on the quote being unexpired")
	assert.Empty(t, settled["insert"], "settlement never inserts quote rows")
	// Review c1 R2-M1: the "already recorded on another bill?" lookup runs
	// only when a transfer is refused, never on the settlement happy path.
	assert.Zero(t, holderLookups(recorder), "settlement must not run the replay holder lookup")

	// A refused transfer (here the recorded one, replayed under another quote
	// whose amount it does not carry) runs the lookup exactly once.
	replayToken := signTestQuote(t, "quote-test-secret", billID, 100, centsToMicrounits(100)+5)
	handler.verifier = &mockGuestPaymentVerifier{err: fmt.Errorf("%w: expected other amount", blockchain.ErrTransferAmountMismatch)}
	recorder.statements = nil
	w = performCryptoPaymentPerfRequest(handler, billToken, testEVMTxHash("tx-quote-access-shape"), replayToken)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "crypto_tx_already_recorded")
	assert.Equal(t, 1, holderLookups(recorder), "a refused transfer checks the ledger once: %v", recorder.statements)
}

// holderLookups counts FindPaymentTxHashHolder statements (payments joined to
// bills by tx hash).
func holderLookups(r *paymentDetailsSQLRecorder) int {
	count := 0
	for _, statement := range r.statements {
		normalized := strings.ToLower(strings.Join(strings.Fields(statement), " "))
		if strings.Contains(normalized, "join bills on bills.id = payments.bill_id") &&
			strings.Contains(normalized, "payments.tx_hash") {
			count++
		}
	}
	return count
}
