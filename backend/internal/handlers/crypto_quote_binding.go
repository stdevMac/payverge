package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/blockchain"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services"
	"github.com/stdevmac/payverge/backend/internal/services/operational_alerts"

	"github.com/gin-gonic/gin"
)

// Guest crypto payer binding (plan §3.1 / C1).
//
// A USDC Transfer log proves that some wallet sent the venue wallet N
// micro-USDC. It says nothing about which bill the money was meant for, so
// before payer binding any guest could claim another guest's transfer (or an
// unrelated one) for their own bill. Every guest USDC quote now reserves a
// server-chosen exact amount in crypto_payment_quotes that no
// other live quote for the same wallet shares. Settlement requires:
//
//   - the signed token names a persisted quote row (QuoteID) that matches the
//     bill, business, chain, wallet, rail and exact amount it was signed for;
//   - the quote is still active and unexpired;
//   - the on-chain transfer carries exactly that amount (no "at least");
//   - the transfer was mined after the quote was issued; and
//   - the quote is consumed in the same transaction as the payment insert.
//
// Anything else never auto-settles.

// guestCrossChainSettlementEnabled gates the LI.FI cross-chain guest rail.
//
// The rail is DISABLED because it cannot be payer-bound: a bridged delivery
// lands as whatever the route produced after fees and slippage (so no exact
// amount can be required), its on-chain sender is the bridge contract rather
// than the guest, and proving which source-chain transfer funded it would
// need the LI.FI status API or a source-chain RPC per supported chain. With
// only "at least the quoted floor reached the venue wallet" to go on, one
// guest's bridged payment could settle another guest's bill. Until the
// settlement is bound to a verified source-chain transaction this stays off.
//
// It is deliberately a package variable and not configuration: production
// has no way to turn it on. Tests flip it to keep the dormant code covered.
// Its value comes from services.CrossChainGuestSettlementAvailable, which also
// drives the operator catalog (the plugin is seeded coming-soon while off), so
// the guest gate and what operators are offered cannot drift apart.
var guestCrossChainSettlementEnabled = services.CrossChainGuestSettlementAvailable

const guestCrossChainDisabledMessage = "Cross-chain payments are disabled: a bridged transfer cannot be bound to a single payment quote. Pay with USDC on Base instead."

// respondGuestCrossChainDisabled answers every cross-chain guest request while
// the rail is off. 422 plugin_unavailable is the code the guest UI already
// renders as "this payment method is not available".
func respondGuestCrossChainDisabled(c *gin.Context) {
	server.RespondWithError(c, http.StatusUnprocessableEntity, "plugin_unavailable", guestCrossChainDisabledMessage)
}

// guestCryptoQuoteClientKey is the per-client identity behind
// database.CryptoQuoteMaxActivePerClient: the client IP (the same identity the
// guest write rate limiter uses) under an HMAC keyed with the quote secret,
// truncated to 32 hex chars. The quote table never stores the IP itself, and
// the key is not reversible without the secret. Empty when there is no IP,
// which skips the per-client cap.
func guestCryptoQuoteClientKey(secret []byte, clientIP string) string {
	clientIP = strings.TrimSpace(clientIP)
	if clientIP == "" {
		return ""
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte("payverge-crypto-quote-client\x00"))
	mac.Write([]byte(clientIP))
	return hex.EncodeToString(mac.Sum(nil)[:16])
}

// issueGuestCryptoQuote persists the quote row that reserves this quote's
// exact amount. It answers the request and returns false on failure.
func issueGuestCryptoQuote(c *gin.Context, bill *database.Bill, settlement guestSettlementContract, baseMicrounits int64, issuedAt time.Time, clientKey string) (*database.CryptoPaymentQuote, bool) {
	quote, err := database.IssueCryptoPaymentQuote(database.IssueCryptoPaymentQuoteInput{
		BillID:            bill.ID,
		BusinessID:        bill.BusinessID,
		SettlementAddress: settlement.Address,
		ChainID:           settlement.ChainID,
		PaymentMethod:     settlement.PaymentMethod,
		BaseMicrounits:    baseMicrounits,
		IssuedAt:          issuedAt,
		ExpiresAt:         issuedAt.Add(cryptoQuoteTTL),
		ClientKey:         clientKey,
	})
	switch {
	case err == nil:
		return quote, true
	case errors.Is(err, database.ErrCryptoQuoteClientLimitReached):
		server.RespondWithError(c, http.StatusTooManyRequests, "crypto_quote_limit", "You already have several open payment quotes for this bill. Finish an open payment or wait a few minutes for one to expire.")
	case errors.Is(err, database.ErrCryptoQuoteLimitReached):
		server.RespondWithError(c, http.StatusTooManyRequests, "crypto_quote_limit", "Too many open payment quotes for this bill. Wait for one to expire or finish an open payment.")
	case errors.Is(err, database.ErrCryptoQuoteAmountsExhausted):
		log.Printf("crypto quote: no unique amount for bill %d wallet %s base %d", bill.ID, settlement.Address, baseMicrounits)
		server.RespondWithError(c, http.StatusServiceUnavailable, "crypto_quote_limit", "Crypto payments are busy right now. Please try again in a few minutes.")
	default:
		log.Printf("crypto quote: persist quote for bill %d: %v", bill.ID, err)
		server.RespondWithError(c, http.StatusServiceUnavailable, "", "Payment quote unavailable")
	}
	return nil, false
}

// loadBoundGuestCryptoQuote resolves the quote row a verified token names and
// checks it against the token and the bill. The row is authoritative for the
// quote's issue time, so the transfer-after-quote check below it uses the
// persisted value.
//
// A quote that was already consumed by this same transaction is returned with
// ok=true so an idempotent retry reaches the settlement layer, which answers
// "already processed" without consuming anything.
func loadBoundGuestCryptoQuote(c *gin.Context, claims *cryptoQuoteClaims, bill *database.Bill, txHash string, now time.Time) (*database.CryptoPaymentQuote, bool) {
	if claims.QuoteID == 0 {
		// Signed before payer binding shipped: its amount was never reserved,
		// so it cannot prove which transfer is this guest's. The guest
		// re-quotes.
		c.JSON(http.StatusBadRequest, gin.H{"error": "Payment quote expired. Please start the payment again.", "code": "crypto_quote_expired"})
		return nil, false
	}
	quote, err := database.GetCryptoPaymentQuote(claims.QuoteID)
	if errors.Is(err, database.ErrCryptoQuoteNotFound) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Payment quote does not match this bill", "code": "quote_invalid"})
		return nil, false
	}
	if err != nil {
		log.Printf("crypto payment: load quote %d for bill %d: %v", claims.QuoteID, bill.ID, err)
		server.RespondWithError(c, http.StatusServiceUnavailable, "", "Payment verification temporarily unavailable")
		return nil, false
	}
	if quote.BillID != bill.ID ||
		quote.BusinessID != bill.BusinessID ||
		quote.ChainID != claims.ChainID ||
		quote.SettlementAddress != database.NormalizeCryptoQuoteAddress(claims.SettlementAddr) ||
		!strings.EqualFold(quote.PaymentMethod, claims.PaymentMethod) ||
		quote.ExactMicrounits != claims.USDMicrounits {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Payment quote does not match this bill", "code": "quote_invalid"})
		return nil, false
	}
	claims.Iat = quote.IssuedAt.Unix()

	switch quote.Status {
	case database.CryptoPaymentQuoteStatusConsumed:
		if quote.ConsumedTxHash != nil && *quote.ConsumedTxHash == txHash {
			return quote, true
		}
		respondGuestCryptoQuoteUsed(c)
		return nil, false
	case database.CryptoPaymentQuoteStatusActive:
		if !quote.ExpiresAt.After(now) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Payment quote expired. Please start the payment again.", "code": "crypto_quote_expired"})
			return nil, false
		}
		return quote, true
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Payment quote expired. Please start the payment again.", "code": "crypto_quote_expired"})
		return nil, false
	}
}

func respondGuestCryptoQuoteUsed(c *gin.Context) {
	server.RespondWithError(c, http.StatusConflict, "crypto_quote_used", "This payment quote was already used for another transaction")
}

// respondGuestTransferAmountMismatch answers a transfer that reached the venue
// wallet with a value other than the quote's exact amount. It never settles:
// the amount is what ties a transfer to this guest's quote. Because the guest
// is told to ask staff, it also raises the operator alert that gives staff a
// record of the refused transfer.
func respondGuestTransferAmountMismatch(c *gin.Context, bill *database.Bill, quote *database.CryptoPaymentQuote, txHash string, err error) {
	slog.Warn("crypto_payment_amount_mismatch",
		"bill_id", bill.ID,
		"business_id", bill.BusinessID,
		"quote_id", quote.ID,
		"expected_microunits", quote.ExactMicrounits,
		"tx_hash", txHash,
		"error", err.Error(),
	)
	alertSvc := operational_alerts.NewService(database.GetDB())
	if alertErr := alertSvc.CreateCryptoAmountMismatchAlert(c.Request.Context(), *bill, txHash, map[string]any{
		"quote_id": quote.ID,
		"chain_id": quote.ChainID,
	}); alertErr != nil {
		log.Printf("failed to create crypto amount mismatch alert for bill %d: %v", bill.ID, alertErr)
	}
	c.JSON(http.StatusBadRequest, gin.H{
		"error": fmt.Sprintf("This transaction does not carry the exact quoted amount (%s USDC), so it cannot be matched to this bill. Ask staff for help if you already sent it.", formatMicroUSDC(quote.ExactMicrounits)),
		"code":  "amount_mismatch",
	})
}

// isGuestTransferAmountMismatch reports the verifier's "a transfer to the
// recipient exists but with another value" outcome.
func isGuestTransferAmountMismatch(err error) bool {
	return errors.Is(err, blockchain.ErrTransferAmountMismatch)
}

// formatMicroUSDC renders micro-USDC with all six decimals (1.000042).
func formatMicroUSDC(microunits int64) string {
	sign := ""
	if microunits < 0 {
		sign = "-"
		microunits = -microunits
	}
	return fmt.Sprintf("%s%d.%06d", sign, microunits/1_000_000, microunits%1_000_000)
}

// reportGuestCryptoTxHashConflict raises the operator alert for a verified
// transfer that is already recorded as a payment (on another bill, or on this
// bill under another quote), and logs it as a structured warning. This is the
// replay signature: someone presented a transfer that already settled
// something. holder, when known, is the payment that records it, so the alert
// can tell staff which bill the transfer already paid. Amounts stay out of the
// metadata (alerts:read is broader than financial:read).
func reportGuestCryptoTxHashConflict(c *gin.Context, bill *database.Bill, quoteID uint, txHash, rail string, holder *database.PaymentTxHashHolder) {
	if bill == nil {
		return
	}
	attrs := []any{
		"bill_id", bill.ID,
		"business_id", bill.BusinessID,
		"quote_id", quoteID,
		"tx_hash", txHash,
		"rail", rail,
	}
	if holder != nil {
		attrs = append(attrs, "recorded_bill_id", holder.BillID, "recorded_payment_id", holder.PaymentID)
	}
	slog.Warn("crypto_payment_tx_hash_conflict", attrs...)
	alertSvc := operational_alerts.NewService(database.GetDB())
	if err := alertSvc.CreateCryptoTxHashConflictAlert(c.Request.Context(), *bill, txHash, holder, map[string]any{
		"quote_id": quoteID,
		"rail":     rail,
	}); err != nil {
		log.Printf("failed to create crypto tx hash conflict alert for bill %d: %v", bill.ID, err)
	}
}

// respondGuestCryptoTxAlreadyRecorded answers a verified transfer that the
// ledger already holds. The guest copy says the transfer cannot be applied to
// this bill; staff get the tx_hash_conflict alert naming the bill it paid.
func respondGuestCryptoTxAlreadyRecorded(c *gin.Context, bill *database.Bill, quoteID uint, txHash, rail string, holder *database.PaymentTxHashHolder) {
	reportGuestCryptoTxHashConflict(c, bill, quoteID, txHash, rail, holder)
	server.RespondWithError(c, http.StatusConflict, "crypto_tx_already_recorded", "Payment transaction already recorded")
}

// respondIfGuestTransferAlreadyRecorded runs on the refusal paths only (a
// transfer to the venue wallet that does not carry this quote's exact amount,
// or that predates this quote). Before the refusal is filed as a wrong-amount
// or stale transfer, it checks whether the transfer is already recorded as a
// payment: a replay of a transfer that settled another bill must surface as a
// tx-hash conflict naming that bill, never as a wrong-amount transfer staff
// might settle or refund by hand. Returns true when it answered the request
// (409 for a recorded transfer, 503 when the lookup failed, so a replay is
// never misfiled because the ledger was briefly unreachable). The settlement
// happy path never pays for this lookup.
func respondIfGuestTransferAlreadyRecorded(c *gin.Context, bill *database.Bill, quoteID uint, txHash, rail string) bool {
	holder, err := database.FindPaymentTxHashHolder(txHash)
	if err != nil {
		log.Printf("failed to check whether crypto transfer %s is already recorded (bill %d): %v", txHash, bill.ID, err)
		server.RespondWithError(c, http.StatusServiceUnavailable, "verification_unavailable", "Payment verification temporarily unavailable")
		return true
	}
	if holder == nil {
		return false
	}
	respondGuestCryptoTxAlreadyRecorded(c, bill, quoteID, txHash, rail, holder)
	return true
}
