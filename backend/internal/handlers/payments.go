package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"math/big"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/blockchain"
	appconfig "github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/crm"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/fiscal"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services"
	"github.com/stdevmac/payverge/backend/internal/services/cashregister"
	operational_alerts "github.com/stdevmac/payverge/backend/internal/services/operational_alerts"
	"github.com/stdevmac/payverge/backend/internal/txhash"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const errCodeCashSessionRequired = "cash_session_required"
const errCodePaymentRequestExpired = "payment_request_expired"

// allowCashRecording rejects operator cash recording unless an applicable
// open cash-register session exists. Card/other methods are not gated.
// A session lookup error is an explicit 500, not a silent unassigned-cash write.
func (h *PaymentHandler) allowCashRecording(c *gin.Context, businessID uint, method database.AlternativePaymentMethod, requestID *uint) bool {
	recordingCash := method == database.PaymentMethodCash
	if requestID != nil && h.db != nil && h.db.GetGorm() != nil {
		var pending database.AlternativePayment
		if err := h.db.GetGorm().Select("payment_method").First(&pending, *requestID).Error; err == nil {
			recordingCash = pending.PaymentMethod == database.PaymentMethodCash
		}
	}
	if !recordingCash {
		return true
	}
	if h.db == nil || h.db.GetGorm() == nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to check cash register session")
		return false
	}
	session, err := cashregister.NewService(h.db.GetGorm()).EnsureDemoHouseCashSession(c.Request.Context(), businessID)
	if err != nil {
		logger.Logger.Errorf("Failed to check cash register session for business %d: %v", businessID, err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to check cash register session")
		return false
	}
	if session == nil {
		server.RespondWithError(c, http.StatusConflict, errCodeCashSessionRequired, "Open a cash register session before recording cash")
		return false
	}
	return true
}

func createPaymentReceivedOperationalAlert(ctx context.Context, bill *database.Bill, resourceID uint, metadata map[string]any) {
	createPaymentReceivedOperationalAlertForResource(ctx, bill, database.OperationalAlertResourceTypePayment, resourceID, metadata)
}

func createPaymentReceivedOperationalAlertForResource(ctx context.Context, bill *database.Bill, resourceType database.OperationalAlertResourceType, resourceID uint, metadata map[string]any) {
	if bill == nil {
		return
	}
	if err := operational_alerts.NewService(database.GetDB()).CreatePaymentReceivedAlertForResource(ctx, *bill, resourceType, resourceID, metadata); err != nil {
		log.Printf("failed to create payment operational alert: business_id=%d bill_id=%d resource_id=%d error=%v", bill.BusinessID, bill.ID, resourceID, err)
		if resourceType == database.OperationalAlertResourceTypeAltPayment {
			metrics.RecordAlternativePaymentRequestOutcome(metrics.AlternativePaymentRequestAlertFailed)
		}
	}
	resolvePaidBillOperationalAlert(ctx, bill, "payment_received")
}

func createPaymentRequestedOperationalAlert(ctx context.Context, bill *database.Bill, resourceID uint, metadata map[string]any) error {
	if bill == nil {
		return nil
	}
	return operational_alerts.NewService(database.GetDB()).CreatePaymentRequestedAlertForResource(ctx, *bill, database.OperationalAlertResourceTypeAltPayment, resourceID, metadata)
}

func createPaymentReceivedOperationalAlertForTxHash(ctx context.Context, bill *database.Bill, txHash string, metadata map[string]any) {
	if bill == nil {
		return
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["transaction_hash"] = strings.TrimSpace(txHash)
	payment, err := database.GetPaymentByTxHash(txHash)
	if err != nil {
		log.Printf("failed to load payment for operational alert: business_id=%d bill_id=%d tx_hash=%s error=%v", bill.BusinessID, bill.ID, txHash, err)
		createPaymentReceivedOperationalAlert(ctx, bill, bill.ID, metadata)
		return
	}
	metadata["payment_id"] = payment.ID
	createPaymentReceivedOperationalAlert(ctx, bill, payment.ID, metadata)
}

// createPaymentRefundReviewOperationalAlert surfaces an out-of-band partial
// provider refund for manual reconciliation, keyed off the original payment's
// txHash so the alert points at the local Payment row when it exists. We never
// auto-reverse partial refunds (see ReversePluginPayment full-reversal
// semantics), so the operator reconciles manually.
func createPaymentRefundReviewOperationalAlert(ctx context.Context, bill *database.Bill, txHash string, metadata map[string]any) {
	if bill == nil {
		return
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["transaction_hash"] = strings.TrimSpace(txHash)
	resourceID := bill.ID
	if payment, err := database.GetPaymentByTxHash(txHash); err != nil {
		log.Printf("failed to load payment for refund-review alert: business_id=%d bill_id=%d tx_hash=%s error=%v", bill.BusinessID, bill.ID, txHash, err)
	} else if payment != nil {
		metadata["payment_id"] = payment.ID
		resourceID = payment.ID
	}
	if err := operational_alerts.NewService(database.GetDB()).CreatePaymentRefundReviewAlert(ctx, *bill, resourceID, metadata); err != nil {
		log.Printf("failed to create refund-review operational alert: business_id=%d bill_id=%d tx_hash=%s error=%v", bill.BusinessID, bill.ID, txHash, err)
	}
}

func resolvePaidBillOperationalAlert(ctx context.Context, bill *database.Bill, reason string) {
	if bill == nil || bill.Status != database.BillStatusPaid {
		return
	}
	if err := operational_alerts.NewService(database.GetDB()).ResolveAlertForResource(
		ctx,
		bill.BusinessID,
		database.OperationalAlertResourceTypeBill,
		bill.ID,
		operational_alerts.Actor{Name: "payment"},
		reason,
	); err != nil {
		log.Printf("failed to resolve bill operational alert after payment: business_id=%d bill_id=%d error=%v", bill.BusinessID, bill.ID, err)
	}
}

type guestPaymentVerifier interface {
	VerifyUSDCTransfer(ctx context.Context, txHash string, recipient string, expectedAmountMicrounits int64) error
}

type guestMinimumPaymentVerifier interface {
	VerifyUSDCTransferAtLeast(ctx context.Context, txHash string, recipient string, expectedAmountMicrounits int64) error
}

// guestPaymentEvidenceVerifier optionally returns Transfer log evidence so
// payment settlement can persist a verified refund destination (Wave 4 Task 11).
type guestPaymentEvidenceVerifier interface {
	VerifyUSDCTransferWithEvidence(ctx context.Context, txHash string, recipient string, expectedAmountMicrounits int64, allowExcess bool) (blockchain.USDCTransferEvidence, error)
}

// guestSettlementContractProvider describes the exact settlement contract the
// verifier is able to prove. Quotes fail closed when this metadata is absent:
// otherwise the UI could transfer on one chain/token while the backend verifies
// another.
type guestSettlementContractProvider interface {
	ChainID() int64
	TokenSymbolForChain() string
}

// validatePaymentAmounts checks that payment amounts are positive and tips are non-negative.
func validatePaymentAmounts(amount, tip int64) error {
	if amount <= 0 {
		return fmt.Errorf("payment amount must be greater than zero")
	}
	if tip < 0 {
		return fmt.Errorf("tip amount cannot be negative")
	}
	return nil
}

// validatePaymentAmountMagnitude enforces the global sanity ceiling on a
// payment. amount and tip are integer cents (the unit
// contract). It rejects an amount or tip above maxCents — the dollars-as-int
// failure mode — returning an error whose message names the limit. It runs
// in-memory before any bill is loaded, so it adds no query.
func validatePaymentAmountMagnitude(amount, tip, maxCents int64) error {
	if amount > maxCents {
		return fmt.Errorf("payment amount %d cents exceeds the maximum allowed payment amount of %d cents", amount, maxCents)
	}
	if tip > maxCents {
		return fmt.Errorf("tip amount %d cents exceeds the maximum allowed payment amount of %d cents", tip, maxCents)
	}
	return nil
}

// dollarsToCents converts a float64 dollar amount to int64 cents.
// Used for crypto/cross-chain payments where amounts arrive as float64.
// math.Round handles IEEE 754 representation artifacts (e.g., 10.10 * 100 = 1009.9999... -> 1010).
func dollarsToCents(d float64) int64 {
	return int64(math.Round(d * 100))
}

// parseDollarAmountToCents converts a string dollar amount to int64 cents.
// Used for manual/alternative payments where amounts arrive as user-typed strings.
// Rejects inputs with more than 2 decimal places for precision safety.
func parseDollarAmountToCents(amount string) (int64, error) {
	raw := strings.TrimSpace(amount)
	if raw == "" {
		return 0, fmt.Errorf("amount is required")
	}

	sign := int64(1)
	if strings.HasPrefix(raw, "-") {
		sign = -1
		raw = strings.TrimPrefix(raw, "-")
	} else if strings.HasPrefix(raw, "+") {
		raw = strings.TrimPrefix(raw, "+")
	}

	parts := strings.Split(raw, ".")
	if len(parts) > 2 {
		return 0, fmt.Errorf("invalid amount format")
	}

	whole := parts[0]
	if whole == "" {
		whole = "0"
	}
	if !isDigitsOnly(whole) {
		return 0, fmt.Errorf("invalid amount format")
	}

	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
		if fraction == "" || !isDigitsOnly(fraction) {
			return 0, fmt.Errorf("invalid amount format")
		}
		if len(fraction) > 2 {
			return 0, fmt.Errorf("dollar amount must use cents precision")
		}
	}
	fraction = fraction + strings.Repeat("0", 2-len(fraction))

	cents, err := strconv.ParseInt(whole+fraction, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid amount format")
	}
	return sign * cents, nil
}

func isDigitsOnly(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// centsToDollars converts integer cents (4550) to a dollar amount (45.50).
// Mirror of the wire-contract helper in database/models_json.go so handler
// response payloads stay in dollars without pulling in the database package.
func centsToDollars(cents int64) float64 {
	return float64(cents) / 100.0
}

// PaymentHandler handles payment-related requests
type PaymentHandler struct {
	db                    *database.DB
	verifier              guestPaymentVerifier
	exchangeRates         *services.ExchangeRateService
	quoteSecret           []byte
	maxPaymentAmountCents int64
}

// NewPaymentHandler creates a new payment handler.
// The blockchain service is used only to derive a default USDC verifier when
// the caller does not supply one; the handler itself does not hold a reference.
func NewPaymentHandler(db *database.DB, blockchain *blockchain.BlockchainService, verifier interface{}, exchangeRates *services.ExchangeRateService) *PaymentHandler {
	var resolvedVerifier guestPaymentVerifier
	if typed, ok := verifier.(guestPaymentVerifier); ok {
		resolvedVerifier = typed
	} else if blockchain != nil {
		resolvedVerifier = blockchain
	}

	return &PaymentHandler{
		db:                    db,
		verifier:              resolvedVerifier,
		exchangeRates:         exchangeRates,
		quoteSecret:           resolveQuoteSecret(),
		maxPaymentAmountCents: resolveMaxPaymentAmountCents(),
	}
}

// resolveQuoteSecret picks the HMAC key for crypto quote tokens. It prefers an
// explicit CRYPTO_QUOTE_SECRET, else derives from the app's JWT_SECRET_KEY (so
// production gets a real secret), else a dev-only default. It never panics, so
// constructing a PaymentHandler in tests without JWT_SECRET_KEY is safe. It
// always returns a non-empty key.
func resolveQuoteSecret() []byte {
	if s := strings.TrimSpace(os.Getenv("CRYPTO_QUOTE_SECRET")); s != "" {
		return []byte(s)
	}
	if s := strings.TrimSpace(os.Getenv("JWT_SECRET_KEY")); s != "" {
		return []byte("payverge-crypto-quote:" + s)
	}
	return []byte("payverge-dev-crypto-quote-secret")
}

// defaultMaxPaymentAmountCents is the global sanity ceiling on a single payment:
// $1,000,000 expressed in integer cents. Settlement amounts are the
// integer-cents unit contract (DB stores int64 cents; see models_json.go).
// A caller that mistakenly sends dollars-as-int would 100x-overstate the
// amount; this ceiling turns that silent corruption into a hard rejection.
const defaultMaxPaymentAmountCents int64 = 100_000_000

// resolveMaxPaymentAmountCents reads MAX_PAYMENT_AMOUNT_CENTS once. A missing,
// blank, non-numeric, or non-positive value falls back to the default so a
// misconfigured env can never disable the guard.
func resolveMaxPaymentAmountCents() int64 {
	raw := strings.TrimSpace(os.Getenv("MAX_PAYMENT_AMOUNT_CENTS"))
	if raw == "" {
		return defaultMaxPaymentAmountCents
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || parsed <= 0 {
		return defaultMaxPaymentAmountCents
	}
	return parsed
}

// centsToMicrounits converts an integer-cents amount into USDC micro-units
// (6 decimals): 1 cent = 10_000 micro-units. Deriving the verification amount
// from the same integer cents that get credited to the bill guarantees the
// on-chain-verified amount and the ledgered amount are identical (no sub-cent
// rounding drift between a re-parsed float and the stored cents).
func centsToMicrounits(cents int64) int64 {
	return cents * 10_000
}

func cryptoPaymentUnavailableMessage() string {
	return "This business is not currently accepting this crypto payment method"
}

func (h *PaymentHandler) requireGuestCryptoPlugin(c *gin.Context, bill *database.Bill, pluginName string) bool {
	enabled, err := guestPaymentPluginConfigured(bill.BusinessID, pluginName)
	if err != nil {
		log.Printf("Failed to check crypto payment plugin %s for business %d: %v", pluginName, bill.BusinessID, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Internal server error")
		return false
	}
	if !enabled {
		server.RespondWithError(c, http.StatusUnprocessableEntity, "plugin_unavailable", cryptoPaymentUnavailableMessage())
		return false
	}
	if strings.TrimSpace(bill.SettlementAddr) == "" {
		server.RespondWithError(c, http.StatusUnprocessableEntity, "plugin_unavailable", "This business has not configured a settlement wallet")
		return false
	}
	return true
}

// verifyGuestSettlementAtLeast verifies the on-chain transfer credits at least
// the expected USD micro-units to the recipient. Shared by the direct-USDC and
// cross-chain guest payment paths. Prefers the at-least verifier (tolerates
// wallet over-send / bridge slippage) and falls back to exact match when the
// verifier does not implement the minimum interface.
func (h *PaymentHandler) verifyGuestSettlementAtLeast(ctx context.Context, txHash string, recipient string, expectedAmountMicrounits int64) error {
	if verifier, ok := h.verifier.(guestMinimumPaymentVerifier); ok {
		return verifier.VerifyUSDCTransferAtLeast(ctx, txHash, recipient, expectedAmountMicrounits)
	}
	return h.verifier.VerifyUSDCTransfer(ctx, txHash, recipient, expectedAmountMicrounits)
}

// verifyGuestSettlementWithEvidence verifies settlement and, when the verifier
// supports it, returns Transfer log evidence (including payer Topics[1]) for
// refund-destination capture. Falls back to verify-only when evidence is
// unavailable — settlement still succeeds, but UI must say manual support is
// required for refunds rather than inventing a destination.
func (h *PaymentHandler) verifyGuestSettlementWithEvidence(ctx context.Context, txHash string, recipient string, expectedAmountMicrounits int64, allowExcess bool) (blockchain.USDCTransferEvidence, error) {
	if verifier, ok := h.verifier.(guestPaymentEvidenceVerifier); ok {
		return verifier.VerifyUSDCTransferWithEvidence(ctx, txHash, recipient, expectedAmountMicrounits, allowExcess)
	}
	var empty blockchain.USDCTransferEvidence
	var err error
	if allowExcess {
		err = h.verifyGuestSettlementAtLeast(ctx, txHash, recipient, expectedAmountMicrounits)
	} else if h.verifier != nil {
		err = h.verifier.VerifyUSDCTransfer(ctx, txHash, recipient, expectedAmountMicrounits)
	} else {
		err = fmt.Errorf("payment verification service unavailable")
	}
	return empty, err
}

// canonicalizeGuestTxHash rewrites a guest-submitted settlement hash to its
// single canonical spelling (0x + 64 lowercase hex) or answers 400. The
// go-ethereum hash parser is lenient (case, missing 0x, zero padding,
// trailing garbage all resolve to the same receipt) while the payments ledger
// dedupes on the exact string, so every spelling must collapse to one key
// before verification and settlement.
func canonicalizeGuestTxHash(c *gin.Context, hash *string) bool {
	canonical, ok := txhash.Canonical(*hash)
	if !ok {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "transaction_hash must be 0x followed by 64 hex characters")
		return false
	}
	*hash = canonical
	return true
}

// requireTransferAfterQuote rejects a transfer mined before the quote it is
// claimed against: a payment cannot predate its own quote, so an older
// receipt (someone else's historical transfer to the same settlement
// address) must not settle this bill. Answers 400 and returns false on
// rejection. Fails closed (503) when the verifier pinned a block but could
// not report its time.
func requireTransferAfterQuote(c *gin.Context, claims cryptoQuoteClaims, ev blockchain.USDCTransferEvidence) bool {
	if claims.Iat <= 0 {
		server.RespondWithError(c, http.StatusBadRequest, "quote_invalid", "Payment quote invalid or expired")
		return false
	}
	if ev.BlockTimestamp == 0 {
		if strings.TrimSpace(ev.BlockHash) != "" {
			server.RespondWithError(c, http.StatusServiceUnavailable, "", "Payment verification temporarily unavailable")
			return false
		}
		return true // verifier without block evidence (verify-only fallback)
	}
	if transferPredatesQuote(claims, ev.BlockTimestamp) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "This transaction was sent before the payment quote was issued",
			"code":  "transfer_predates_quote",
		})
		return false
	}
	return true
}

// refundDestinationFromTransferEvidence builds append-once transfer-log proof
// for the payment transaction. Empty verifier evidence keeps the explicit
// verify-only/manual-support fallback; non-empty evidence is committed
// atomically with the Payment by the database layer.
func refundDestinationFromTransferEvidence(ev blockchain.USDCTransferEvidence) *database.PaymentRefundDestination {
	if strings.TrimSpace(ev.From) == "" || ev.AmountBaseUnits <= 0 {
		return nil
	}
	logRef := fmt.Sprintf("%s:%d", strings.TrimSpace(ev.TxHash), ev.LogIndex)
	dest := &database.PaymentRefundDestination{
		ChainID:         int(ev.ChainID),
		Token:           ev.Token,
		AmountBaseUnits: ev.AmountBaseUnits,
		RefundAddress:   ev.From,
		EvidenceType:    database.RefundEvidenceTransferLog,
		LogRef:          &logRef,
		VerifiedAt:      time.Now().UTC(),
	}
	if dest.Token == "" {
		dest.Token = "USDC"
	}
	if dest.ChainID == 0 {
		// Direct Base settlement default when the verifier omitted chain ID.
		dest.ChainID = 8453
	}
	return dest
}

// blockEvidencePointers extracts persistable reorg-reconciliation evidence
// (block number + hash) from a verified transfer, as nullable pointers for the
// Payment row. Returns (nil, nil) when the verifier omitted a block hash (the
// verify-only fallback), which correctly leaves the settlement out of the reorg
// sweep rather than inventing a block.
func blockEvidencePointers(ev blockchain.USDCTransferEvidence) (*int64, *string) {
	h := strings.TrimSpace(ev.BlockHash)
	if h == "" {
		return nil, nil
	}
	n := int64(ev.BlockNumber)
	return &n, &h
}

// refundDestinationFromWalletSignature builds wallet-signature evidence for
// cross-chain settlements where the bridge sender is not a safe destination.
// The database settlement transaction assigns PaymentID and persists it.
func refundDestinationFromWalletSignature(chainID int, amountBaseUnits int64, refundAddress, signatureRef, logRef string) *database.PaymentRefundDestination {
	if amountBaseUnits <= 0 || strings.TrimSpace(refundAddress) == "" {
		return nil
	}
	sig := strings.TrimSpace(signatureRef)
	lr := strings.TrimSpace(logRef)
	dest := &database.PaymentRefundDestination{
		ChainID:         chainID,
		Token:           "USDC",
		AmountBaseUnits: amountBaseUnits,
		RefundAddress:   refundAddress,
		EvidenceType:    database.RefundEvidenceWalletSignature,
		VerifiedAt:      time.Now().UTC(),
	}
	if sig != "" {
		dest.SignatureRef = &sig
	}
	if lr != "" {
		dest.LogRef = &lr
	}
	return dest
}

// respondCryptoVerifyError maps an on-chain verification error to an HTTP
// response. A matched-but-not-yet-confirmed transfer (ErrAwaitingConfirmations)
// is a *retryable* 425 so the guest waits and retries the same idempotent tx
// hash; any other error is a hard verification failure (400).
func respondCryptoVerifyError(c *gin.Context, err error) {
	if errors.Is(err, blockchain.ErrAwaitingConfirmations) {
		c.JSON(http.StatusTooEarly, gin.H{
			"error": "Payment is awaiting blockchain confirmations. Please retry in a few seconds.",
			"code":  "awaiting_confirmations",
		})
		return
	}
	if errors.Is(err, blockchain.ErrVerificationUnavailable) {
		c.JSON(http.StatusTooEarly, gin.H{
			"error": "Blockchain verification is temporarily unavailable. Your transfer is safe — please retry in a moment.",
			"code":  "verification_unavailable",
		})
		return
	}
	server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Unable to verify payment transaction")
}

// parseAndCheckQuote verifies the quote token and binds it to this bill, amount,
// payment rail, and the verifier's current chain/token/destination contract.
// Returning the claims makes the signed destination authoritative for the
// transfer verification that follows.
func (h *PaymentHandler) parseAndCheckQuote(c *gin.Context, token string, bill *database.Bill, localCents int64, expectedMethod string) (cryptoQuoteClaims, bool) {
	var empty cryptoQuoteClaims
	claims, err := parseCryptoQuote(token, h.quoteSecret, time.Now())
	if err != nil {
		code := "quote_invalid"
		if errors.Is(err, errQuoteExpired) {
			// Guest FE maps crypto_quote_expired (not quote_expired).
			code = "crypto_quote_expired"
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "Payment quote invalid or expired", "code": code})
		return empty, false
	}
	if bill == nil || claims.BillID != bill.ID || claims.LocalCents != localCents {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Payment quote does not match this bill", "code": "quote_invalid"})
		return empty, false
	}
	// Bind the quote to its business. Server-minted quotes always carry a
	// business id, so a missing or different one is invalid.
	if claims.BusinessID != bill.BusinessID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Payment quote does not match this bill", "code": "quote_invalid"})
		return empty, false
	}

	method, methodOK := normalizeCryptoQuotePaymentMethod(expectedMethod)
	if !methodOK {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Payment quote method is invalid", "code": "quote_invalid"})
		return empty, false
	}
	current, ok := h.currentGuestSettlementContract(c, bill, method)
	if !ok {
		return empty, false
	}
	// Server-minted quotes always carry all four settlement-contract fields;
	// a token missing any of them fails the comparison below.
	if !strings.EqualFold(strings.TrimSpace(claims.SettlementAddr), current.Address) ||
		claims.ChainID != current.ChainID ||
		!strings.EqualFold(strings.TrimSpace(claims.Token), current.Token) ||
		!strings.EqualFold(strings.TrimSpace(claims.PaymentMethod), current.PaymentMethod) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Payment quote settlement configuration changed", "code": "quote_invalid"})
		return empty, false
	}
	return claims, true
}

func paymentWriteError(err error) (int, string) {
	switch {
	case errors.Is(err, database.ErrPaymentExceedsRemaining):
		return http.StatusConflict, "Payment exceeds remaining bill balance"
	case errors.Is(err, database.ErrPaymentTxHashConflict):
		return http.StatusConflict, "Payment transaction already recorded"
	case errors.Is(err, database.ErrCryptoQuoteExpired):
		return http.StatusConflict, "Payment quote expired before the payment could be recorded"
	case errors.Is(err, database.ErrCryptoQuoteUnavailable):
		return http.StatusConflict, "Payment quote already used for another transaction"
	case errors.Is(err, database.ErrBillNotPayable):
		return http.StatusConflict, "Bill is not open for payment"
	case errors.Is(err, database.ErrSplitAmountUnavailable):
		return http.StatusConflict, "Split share exceeds remaining bill balance"
	case errors.Is(err, database.ErrSplitHoldExpired):
		return http.StatusConflict, "Split share hold has expired"
	case errors.Is(err, database.ErrSplitShareAlreadyFinal):
		return http.StatusConflict, "Split share is already finalized"
	case errors.Is(err, database.ErrSplitShareNotFound):
		return http.StatusBadRequest, "Split share was not found"
	case errors.Is(err, database.ErrInvalidPaymentAmount):
		return http.StatusBadRequest, "Payment amount must be greater than zero"
	case errors.Is(err, database.ErrInvalidTipAmount):
		return http.StatusBadRequest, "Tip amount cannot be negative"
	default:
		return http.StatusInternalServerError, "Failed to update bill"
	}
}

// paymentWriteErrorCode maps settlement write failures to guest-stable snake_case
// codes consumed by frontend/src/lib/guestPaymentErrors.ts. English messages from
// paymentWriteError stay unchanged for logs.
func paymentWriteErrorCode(err error) string {
	switch {
	case errors.Is(err, database.ErrSplitAmountUnavailable),
		errors.Is(err, database.ErrSplitHoldExpired),
		errors.Is(err, database.ErrSplitShareAlreadyFinal):
		return "split_share_conflict"
	case errors.Is(err, database.ErrPaymentExceedsRemaining),
		errors.Is(err, database.ErrPaymentTxHashConflict),
		errors.Is(err, database.ErrBillNotPayable):
		return "payment_failed"
	case errors.Is(err, database.ErrCryptoQuoteExpired):
		// Checked first: expiry errors also wrap ErrCryptoQuoteUnavailable.
		return "crypto_quote_expired"
	case errors.Is(err, database.ErrCryptoQuoteUnavailable):
		return "crypto_quote_used"
	case errors.Is(err, database.ErrSplitShareNotFound),
		errors.Is(err, database.ErrInvalidPaymentAmount),
		errors.Is(err, database.ErrInvalidTipAmount):
		return server.ErrCodeInvalidInput
	default:
		return ""
	}
}

func respondPaymentWriteError(c *gin.Context, err error) bool {
	status, message := paymentWriteError(err)
	if status == http.StatusInternalServerError {
		return false
	}

	server.RespondWithError(c, status, paymentWriteErrorCode(err), message)
	return true
}

func publishGuestSplitStateForBill(bill *database.Bill) {
	if bill == nil || bill.ID == 0 {
		return
	}
	// Bill already resolved server-side — load split state by id (not guest token).
	state, err := database.GetBillSplitStateByBillID(bill.ID, time.Now().UTC())
	if err != nil {
		log.Printf("failed to publish guest split state for bill %d: %v", bill.ID, err)
		return
	}
	PublishGuestSplitState(state)
}

// publishRecordedPaymentSSE announces a RECORDED payment (a confirmed/completed
// payment or alternative-payment row — never a pending request) plus the
// resulting bill state to the business's SSE stream. payment.received is gated
// on financial:read and bill.updated on bills:read (sse_permissions.go).
// Amounts are int64 cents in-process; the event payload carries dollars —
// centsToDollars for the scalar fields, and the embedded bill row converts via
// its own MarshalJSON (money wire contract), so no double conversion happens.
// Call this AFTER the DB write has committed.
func publishRecordedPaymentSSE(bill *database.Bill, amountCents, tipCents int64, method string) {
	if bill == nil {
		return
	}
	hub := events.GetHub()
	hub.PublishJSON(bill.BusinessID, "payment.received", gin.H{
		"bill_id":    bill.ID,
		"amount":     centsToDollars(amountCents),
		"tip_amount": centsToDollars(tipCents),
		"method":     method,
	})
	hub.PublishJSON(bill.BusinessID, "bill.updated", gin.H{
		"bill_id": bill.ID,
		"status":  string(bill.Status),
		"bill":    bill,
	})
	events.NotifyTableBillChanged(bill)
}

// publishBillUpdatedSSE announces a bill mutation (void/refund/edit) so
// operator dashboards refetch instead of waiting for the next poll. The
// embedded bill row serializes money as dollars via MarshalJSON.
func publishBillUpdatedSSE(bill *database.Bill) {
	if bill == nil {
		return
	}
	events.GetHub().PublishJSON(bill.BusinessID, "bill.updated", gin.H{
		"bill_id": bill.ID,
		"status":  string(bill.Status),
		"bill":    bill,
	})
	events.NotifyTableBillChanged(bill)
}

// MarkAlternativePayment marks an alternative payment as received (owner or staff with bills:payment)
// POST /api/v1/inside/bills/:bill_id/alternative-payment
func (h *PaymentHandler) MarkAlternativePayment(c *gin.Context) {
	billIDStr := c.Param("bill_id")
	billID, err := strconv.ParseUint(billIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid bill ID")
		return
	}

	var req struct {
		RequestID            *uint  `json:"request_id"`
		ParticipantAddress   string `json:"participant_address"`
		ParticipantName      string `json:"participant_name"`
		Amount               string `json:"amount" binding:"required"`
		TipAmount            string `json:"tip_amount"`
		PaymentMethod        string `json:"payment_method" binding:"required"`
		BusinessConfirmation bool   `json:"business_confirmation"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}

	// Get bill from database
	bill, err := h.db.GetBill(uint(billID))
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Bill not found")
		return
	}

	confirmedBy, authorized := h.authorizeBillPayment(c, bill)
	if !authorized {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Access denied: not authorized to record payments")
		return
	}

	amountCents, err := parseDollarAmountToCents(req.Amount)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}

	tipCents := int64(0)
	if strings.TrimSpace(req.TipAmount) != "" {
		tipCents, err = parseDollarAmountToCents(req.TipAmount)
		if err != nil {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
			return
		}
	}

	if err := validatePaymentAmounts(amountCents, tipCents); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}

	participantAddr := strings.TrimSpace(req.ParticipantAddress)
	if participantAddr == "" {
		participantAddr = "counter"
	}
	participantName := strings.TrimSpace(req.ParticipantName)

	// Validate payment method
	var paymentMethod database.AlternativePaymentMethod
	switch req.PaymentMethod {
	case "cash":
		paymentMethod = database.PaymentMethodCash
	case "card":
		paymentMethod = database.PaymentMethodCard
	case "venmo":
		paymentMethod = database.PaymentMethodVenmo
	case "other":
		paymentMethod = database.PaymentMethodOther
	default:
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid payment method")
		return
	}

	if !h.allowCashRecording(c, bill.BusinessID, paymentMethod, req.RequestID) {
		return
	}

	// Attribute the closure to the staff member confirming the alternative
	// payment (cash/card/venmo/other) when this transitions the bill to paid.
	// Passed into the service so the stamp lands atomically with the
	// status transition.
	closingStaffID := server.ExtractStaffIDFromContext(c)

	if req.RequestID != nil {
		altPayment, updatedBill, err := database.ConfirmPendingAlternativePayment(uint(billID), *req.RequestID, confirmedBy, closingStaffID)
		if err != nil {
			metrics.RecordAlternativePaymentRequestOutcome(metrics.AlternativePaymentRequestRejected)
			switch {
			case errors.Is(err, database.ErrAlternativePaymentBillMismatch):
				server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Payment request does not belong to this bill")
				return
			case errors.Is(err, database.ErrAlternativePaymentAlreadyConfirmed):
				server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Payment request has already been confirmed")
				return
			case errors.Is(err, database.ErrAlternativePaymentRequestNotPending):
				server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Payment request is no longer pending")
				return
			case errors.Is(err, database.ErrAlternativePaymentRequestExpired):
				server.RespondWithError(c, http.StatusConflict, errCodePaymentRequestExpired, "Payment request has expired")
				return
			}

			if respondPaymentWriteError(c, err) {
				return
			}
			server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to confirm payment request")
			return
		}
		metrics.RecordAlternativePaymentRequestOutcome(metrics.AlternativePaymentRequestConfirmed)

		if updatedBill.Status == database.BillStatusPaid {
			enqueueFiscalJobForPaidBillWithAlternativePayment(updatedBill, nil, &altPayment.ID, "alternative_payment")
			enqueueReceiptForPaidBill(updatedBill)
		}
		h.flushPendingMilestones(updatedBill.BusinessID)
		publishGuestSplitStateForBill(updatedBill)
		// The payment row is now CONFIRMED — this is where operators learn
		// money actually arrived (the request only emitted payment.pending).
		publishRecordedPaymentSSE(updatedBill, altPayment.Amount, altPayment.TipAmountCents, string(altPayment.PaymentMethod))
		createPaymentReceivedOperationalAlertForResource(c.Request.Context(), updatedBill, database.OperationalAlertResourceTypeAltPayment, altPayment.ID, map[string]any{
			"alternative_payment_id": altPayment.ID,
			"amount_cents":           altPayment.Amount,
			"method":                 string(altPayment.PaymentMethod),
			"status":                 string(database.AltPaymentStatusConfirmed),
			"source":                 "alternative_payment_confirmation",
		})
		if sharedDeliveryService != nil {
			if err := sharedDeliveryService.HandleDeliveryBillPaid(updatedBill.ID); err != nil {
				log.Printf("delivery payment hook failed for bill %d: %v", updatedBill.ID, err)
			}
		}
	} else {
		// Create alternative payment record for manual confirmations.
		now := time.Now()
		altPayment := &database.AlternativePayment{
			BillID:          uint(billID),
			ParticipantAddr: participantAddr,
			ParticipantName: participantName,
			Amount:          amountCents,
			BillAmountCents: amountCents,
			TipAmountCents:  tipCents,
			PaymentMethod:   paymentMethod,
			Status:          database.AltPaymentStatusConfirmed,
			ConfirmedBy:     confirmedBy,
			ConfirmedAt:     &now,
			// Per-attempt idempotency key: a retried submit reuses it (deduped),
			// while two distinct cash tenders carry different keys (both credited).
			IdempotencyKey: strings.TrimSpace(c.GetHeader("Idempotency-Key")),
		}

		updatedBill, applied, err := database.CreateConfirmedAlternativePayment(altPayment, closingStaffID)
		if err != nil {
			if respondPaymentWriteError(c, err) {
				return
			}
			server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to save alternative payment")
			return
		}
		if applied && updatedBill.Status == database.BillStatusPaid {
			enqueueFiscalJobForPaidBillWithAlternativePayment(updatedBill, nil, &altPayment.ID, "alternative_payment")
			enqueueReceiptForPaidBill(updatedBill)
		}
		h.flushPendingMilestones(updatedBill.BusinessID)
		if applied {
			publishGuestSplitStateForBill(updatedBill)
			// Manual confirmations record money immediately — announce it.
			publishRecordedPaymentSSE(updatedBill, altPayment.Amount, altPayment.TipAmountCents, string(altPayment.PaymentMethod))
			createPaymentReceivedOperationalAlertForResource(c.Request.Context(), updatedBill, database.OperationalAlertResourceTypeAltPayment, altPayment.ID, map[string]any{
				"alternative_payment_id": altPayment.ID,
				"amount_cents":           altPayment.Amount,
				"method":                 string(altPayment.PaymentMethod),
				"status":                 string(database.AltPaymentStatusConfirmed),
				"source":                 "alternative_payment_manual",
			})
			if sharedDeliveryService != nil {
				if err := sharedDeliveryService.HandleDeliveryBillPaid(updatedBill.ID); err != nil {
					log.Printf("delivery payment hook failed for bill %d: %v", updatedBill.ID, err)
				}
			}
		}
	}

	// Get payment breakdown
	breakdown, err := h.getBillPaymentBreakdown(uint(billID))
	if err != nil {
		logger.Logger.Errorf("Failed to get payment breakdown for bill %d: %v", billID, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to get payment breakdown")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":           true,
		"message":           "Alternative payment marked successfully",
		"payment_breakdown": breakdown,
	})
}

// RequestAlternativePayment creates a pending alternative payment request (guest)
// POST /api/v1/bills/:bill_id/request-alternative-payment
func (h *PaymentHandler) RequestAlternativePayment(c *gin.Context) {
	var req struct {
		Amount          string `json:"amount" binding:"required"`
		TipAmount       string `json:"tip_amount"`
		PaymentMethod   string `json:"payment_method" binding:"required"`
		ParticipantName string `json:"participant_name"`
		SplitShareID    *uint  `json:"split_share_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}

	idempotencyKey := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if idempotencyKey == "" || len(idempotencyKey) > 256 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "A valid Idempotency-Key header is required")
		return
	}

	bill, err := h.resolveBillPaymentSummary(c, "")
	if err != nil {
		if errors.Is(err, errInvalidBillIdentifier) {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid bill ID")
			return
		}
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Bill not found")
		return
	}

	amountCents, err := parseDollarAmountToCents(req.Amount)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}
	tipCents := int64(0)
	if strings.TrimSpace(req.TipAmount) != "" {
		tipCents, err = parseDollarAmountToCents(req.TipAmount)
		if err != nil {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
			return
		}
	}

	if err := validatePaymentAmounts(amountCents, tipCents); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}
	if req.SplitShareID == nil && tipCents > 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Tip amount is only supported for split share requests")
		return
	}

	participantAddr := "guest"
	participantName := strings.TrimSpace(req.ParticipantName)
	splitGuestSessionID := ""
	if req.SplitShareID != nil {
		guestSessionID, ok := getExistingGuestSession(c)
		if !ok {
			server.RespondWithError(c, http.StatusConflict, "split_share_conflict", "Split share is no longer available")
			return
		}
		share, err := database.GetActiveBillSplitShareForGuest(*req.SplitShareID, bill.ID, guestSessionID, time.Now().UTC())
		if err != nil {
			switch {
			case errors.Is(err, database.ErrSplitShareNotFound):
				server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Split share does not belong to this bill")
			case errors.Is(err, database.ErrSplitHoldExpired), errors.Is(err, database.ErrSplitShareAlreadyFinal), errors.Is(err, database.ErrSplitGuestMismatch):
				server.RespondWithError(c, http.StatusConflict, "split_share_conflict", "Split share is no longer available")
			default:
				server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to validate split share")
			}
			return
		}
		amountCents = share.AmountCents
		splitGuestSessionID = guestSessionID
		participantAddr = database.BillSplitAlternativePaymentParticipantAddr(share.ID)
		if participantName == "" {
			participantName = strings.TrimSpace(share.DisplayName)
		}
	}
	if err := validatePaymentAmountMagnitude(amountCents, tipCents, h.maxPaymentAmountCents); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}

	// Validate payment method
	var paymentMethod database.AlternativePaymentMethod
	switch req.PaymentMethod {
	case "cash":
		paymentMethod = database.PaymentMethodCash
	case "card":
		paymentMethod = database.PaymentMethodCard
	case "venmo":
		paymentMethod = database.PaymentMethodVenmo
	case "other":
		paymentMethod = database.PaymentMethodOther
	default:
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid payment method")
		return
	}

	// Create pending alternative payment request
	altPayment := &database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: participantAddr,
		ParticipantName: participantName,
		Amount:          amountCents,
		BillAmountCents: amountCents,
		TipAmountCents:  tipCents,
		PaymentMethod:   paymentMethod,
		Status:          database.AltPaymentStatusPending,
		// Payment proof for the guest fiscal identity binding (M-545).
		PayerGuestSession: guestPayerSession(c),
	}
	keyDigest := sha256.Sum256([]byte(idempotencyKey))
	payloadBytes, err := json.Marshal(struct {
		BillID          uint                              `json:"bill_id"`
		AmountCents     int64                             `json:"amount_cents"`
		TipAmountCents  int64                             `json:"tip_amount_cents"`
		PaymentMethod   database.AlternativePaymentMethod `json:"payment_method"`
		ParticipantAddr string                            `json:"participant_address"`
		ParticipantName string                            `json:"participant_name"`
		SplitShareID    uint                              `json:"split_share_id"`
	}{
		BillID:          bill.ID,
		AmountCents:     amountCents,
		TipAmountCents:  tipCents,
		PaymentMethod:   paymentMethod,
		ParticipantAddr: participantAddr,
		ParticipantName: participantName,
		SplitShareID: func() uint {
			if req.SplitShareID == nil {
				return 0
			}
			return *req.SplitShareID
		}(),
	})
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to prepare payment request")
		return
	}
	payloadDigest := sha256.Sum256(payloadBytes)
	altPayment.IdempotencyKeyHash = hex.EncodeToString(keyDigest[:])
	altPayment.PayloadHash = hex.EncodeToString(payloadDigest[:])

	// The pending request is the durable guest-facing result. Commit it before
	// attempting the operator alert so an alert/event schema or delivery issue
	// cannot turn a saved cashier request into a generic guest failure.
	storedPayment, replayed, err := h.db.AlternativePaymentService.CreatePendingRequest(altPayment, time.Now().UTC(), splitGuestSessionID, splitGuestPaymentHoldTTL, nil)
	if err != nil {
		switch {
		case errors.Is(err, database.ErrAlternativePaymentRequestConflict):
			metrics.RecordAlternativePaymentRequestOutcome(metrics.AlternativePaymentRequestConflict)
			server.RespondWithError(c, http.StatusConflict, "idempotency_conflict", "Idempotency key was already used for a different payment request")
		case errors.Is(err, database.ErrPaymentRequestAlreadyPending):
			metrics.RecordAlternativePaymentRequestOutcome(metrics.AlternativePaymentRequestRejected)
			// A repeat "pay at the counter" tap: staff already have a request
			// covering this balance, so tell the guest to wait, not that the
			// bill changed.
			server.RespondWithError(c, http.StatusConflict, "payment_request_pending", "A payment request for this bill is already waiting for staff")
		case errors.Is(err, database.ErrPaymentExceedsRemaining), errors.Is(err, database.ErrBillNotPayable):
			metrics.RecordAlternativePaymentRequestOutcome(metrics.AlternativePaymentRequestRejected)
			// Distinct from generic CONFLICT so guests on partial/paid bills get
			// actionable copy ("balance changed / already paid") rather than a
			// vague retry message.
			server.RespondWithError(c, http.StatusConflict, "bill_not_payable", "Payment request exceeds the bill's outstanding balance")
		case errors.Is(err, database.ErrSplitHoldExpired), errors.Is(err, database.ErrSplitShareAlreadyFinal), errors.Is(err, database.ErrSplitAmountUnavailable):
			metrics.RecordAlternativePaymentRequestOutcome(metrics.AlternativePaymentRequestRejected)
			server.RespondWithError(c, http.StatusConflict, "split_share_conflict", "Split share is no longer available")
		default:
			server.RespondWithError(c, http.StatusInternalServerError, "payment_request_failed", "Failed to save payment request")
		}
		return
	}
	altPayment = storedPayment
	if replayed {
		metrics.RecordAlternativePaymentRequestOutcome(metrics.AlternativePaymentRequestReplayed)
	} else {
		metrics.RecordAlternativePaymentRequestOutcome(metrics.AlternativePaymentRequestCreated)
	}
	if !replayed {
		if alertErr := createPaymentRequestedOperationalAlert(c.Request.Context(), bill, altPayment.ID, map[string]any{
			"alternative_payment_id": altPayment.ID,
			"amount_cents":           altPayment.Amount,
			"method":                 string(altPayment.PaymentMethod),
			"status":                 string(database.AltPaymentStatusPending),
			"source":                 "alternative_payment_request",
		}); alertErr != nil {
			metrics.RecordAlternativePaymentRequestOutcome(metrics.AlternativePaymentRequestAlertFailed)
			log.Printf("failed to create payment-request operational alert: business_id=%d bill_id=%d resource_id=%d error=%v", bill.BusinessID, bill.ID, altPayment.ID, alertErr)
		}
	}

	// This is only a REQUEST (status pending) — no money has been recorded
	// yet, so it must NOT announce payment.received or raise a payment_received
	// operational alert (operators would celebrate a payment that may never be
	// confirmed). payment.received / payment_received alerts are emitted at
	// confirmation time (MarkAlternativePayment).
	if !replayed {
		events.GetHub().PublishJSON(bill.BusinessID, "payment.pending", gin.H{
			"bill_id": bill.ID,
			"amount":  centsToDollars(amountCents),
			"method":  req.PaymentMethod,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message":    "Alternative payment request sent to business owner",
		"request_id": altPayment.ID,
	})
}

// GetPendingAlternativePayments gets pending alternative payment requests (business owner only)
// GET /api/v1/inside/bills/:bill_id/pending-alternative-payments
func (h *PaymentHandler) GetPendingAlternativePayments(c *gin.Context) {
	billIDStr := c.Param("bill_id")
	billID, err := strconv.ParseUint(billIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid bill ID")
		return
	}

	bill, err := h.getBillPaymentSummaryByID(uint(billID))
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Bill not found")
		return
	}

	if _, authorized := h.authorizeBillManagement(c, bill); !authorized {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Access denied: not bill owner")
		return
	}

	// Get pending payments from database
	pendingPayments, err := h.db.AlternativePaymentService.GetPendingByBillID(uint(billID))
	if err != nil {
		logger.Logger.Errorf("Failed to get pending payments for bill %d: %v", billID, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to get pending payments")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"pending_payments": pendingPayments,
	})
}

// CancelPendingAlternativePayment releases a pending request without recording
// money. This is an operator payment action and is distinct from rejecting a
// request so support and metrics can explain how each reservation ended.
func (h *PaymentHandler) CancelPendingAlternativePayment(c *gin.Context) {
	h.resolvePendingAlternativePayment(c, database.AltPaymentStatusCancelled)
}

// RejectPendingAlternativePayment records that the operator did not accept the
// guest's claimed tender. It never updates the bill's settled balance.
func (h *PaymentHandler) RejectPendingAlternativePayment(c *gin.Context) {
	h.resolvePendingAlternativePayment(c, database.AltPaymentStatusRejected)
}

func (h *PaymentHandler) resolvePendingAlternativePayment(c *gin.Context, targetStatus database.AlternativePaymentStatus) {
	billID, err := strconv.ParseUint(c.Param("bill_id"), 10, 32)
	if err != nil || billID == 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid bill ID")
		return
	}
	requestID, err := strconv.ParseUint(c.Param("request_id"), 10, 32)
	if err != nil || requestID == 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid payment request ID")
		return
	}

	bill, err := h.getBillPaymentSummaryByID(uint(billID))
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Bill not found")
		return
	}
	resolvedBy, authorized := h.authorizeBillPayment(c, bill)
	if !authorized {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Access denied: not authorized to resolve payment requests")
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
		server.RespondBindError(c, err)
		return
	}

	now := time.Now().UTC()
	var resolved *database.AlternativePayment
	if targetStatus == database.AltPaymentStatusRejected {
		resolved, err = database.RejectPendingAlternativePayment(uint(billID), uint(requestID), resolvedBy, body.Reason, now)
	} else {
		resolved, err = database.CancelPendingAlternativePayment(uint(billID), uint(requestID), resolvedBy, body.Reason, now)
	}
	if err != nil {
		switch {
		case errors.Is(err, database.ErrAlternativePaymentBillMismatch):
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Payment request does not belong to this bill")
		case errors.Is(err, database.ErrAlternativePaymentRequestExpired):
			server.RespondWithError(c, http.StatusConflict, errCodePaymentRequestExpired, "Payment request has expired")
		case errors.Is(err, database.ErrAlternativePaymentAlreadyConfirmed), errors.Is(err, database.ErrAlternativePaymentRequestNotPending):
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Payment request is no longer pending")
		case errors.Is(err, gorm.ErrRecordNotFound):
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Payment request not found")
		default:
			logger.Logger.Errorf("Failed to resolve alternative payment request %d for bill %d: %v", requestID, billID, err)
			server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to resolve payment request")
		}
		return
	}

	if targetStatus == database.AltPaymentStatusRejected {
		metrics.RecordAlternativePaymentRequestOutcome(metrics.AlternativePaymentRequestRejected)
	} else {
		metrics.RecordAlternativePaymentRequestOutcome(metrics.AlternativePaymentRequestCancelled)
	}
	publishGuestSplitStateForBill(bill)
	if hub := events.GetHub(); hub != nil {
		hub.PublishJSON(bill.BusinessID, "payment.request.resolved", gin.H{
			"bill_id":    bill.ID,
			"request_id": resolved.ID,
			"status":     resolved.Status,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"status":  resolved.Status,
		"payment": resolved,
	})
}

// guestAlternativePaymentView is the anonymous guest projection of a confirmed
// alternative payment. ConfirmedAt is the row UpdatedAt (this route returns
// confirmed rows only). Staff identity and payer addresses stay off the payload.
type guestAlternativePaymentView struct {
	ID            uint      `json:"id"`
	PaymentMethod string    `json:"payment_method"`
	Status        string    `json:"status"`
	Amount        float64   `json:"amount"`
	ConfirmedAt   time.Time `json:"confirmed_at"`
}

func guestAlternativePaymentViews(rows []database.AlternativePayment) []guestAlternativePaymentView {
	views := make([]guestAlternativePaymentView, 0, len(rows))
	for _, row := range rows {
		views = append(views, guestAlternativePaymentView{
			ID:            row.ID,
			PaymentMethod: string(row.PaymentMethod),
			Status:        string(row.Status),
			Amount:        centsToDollars(row.Amount),
			ConfirmedAt:   row.UpdatedAt,
		})
	}
	return views
}

// GetBillAlternativePayments gets all confirmed alternative payments for a bill (public)
// GET /api/v1/guest/bill/:bill_token/alternative-payments
func (h *PaymentHandler) GetBillAlternativePayments(c *gin.Context) {
	bill, err := h.resolveBillPaymentSummary(c, "")
	if err != nil {
		if errors.Is(err, errInvalidBillIdentifier) {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid bill ID")
			return
		}
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Bill not found")
		return
	}

	// Get confirmed alternative payments from database
	altPayments, err := h.db.AlternativePaymentService.GetConfirmedByBillID(bill.ID)
	if err != nil {
		logger.Logger.Errorf("Failed to get alternative payments for bill %d: %v", bill.ID, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to get alternative payments")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"alternative_payments": guestAlternativePaymentViews(altPayments),
	})
}

// GetBillPaymentBreakdown gets the payment breakdown for a bill (public)
// GET /api/v1/bills/:bill_id/payment-breakdown
func (h *PaymentHandler) GetBillPaymentBreakdown(c *gin.Context) {
	bill, err := h.resolveBillPaymentSummary(c, "")
	if err != nil {
		if errors.Is(err, errInvalidBillIdentifier) {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid bill ID")
			return
		}
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Bill not found")
		return
	}

	if strings.TrimSpace(c.Param("bill_id")) != "" {
		if _, authorized := h.authorizeBillManagement(c, bill); !authorized {
			server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Access denied: not bill owner")
			return
		}
	}

	breakdown, err := h.buildBillPaymentBreakdown(bill)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to get payment breakdown")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"breakdown": breakdown,
	})
}

// Helper function to calculate payment breakdown
func (h *PaymentHandler) getBillPaymentBreakdown(billID uint) (*database.PaymentBreakdown, error) {
	bill, err := h.getBillPaymentSummaryByID(billID)
	if err != nil {
		return nil, err
	}

	return h.buildBillPaymentBreakdown(bill)
}

func (h *PaymentHandler) getBillPaymentSummaryByID(billID uint) (*database.Bill, error) {
	var bill database.Bill
	if err := h.db.GetGorm().
		Select("id", "business_id", "bill_number", "total_amount", "paid_amount", "status").
		First(&bill, billID).Error; err != nil {
		return nil, err
	}
	return &bill, nil
}

func (h *PaymentHandler) buildBillPaymentBreakdown(bill *database.Bill) (*database.PaymentBreakdown, error) {
	alternativePaid, err := h.db.AlternativePaymentService.GetConfirmedTotalByBillID(bill.ID)
	if err != nil {
		return nil, err
	}

	// Calculate crypto paid (total paid - alternative paid)
	cryptoPaid := bill.PaidAmount - alternativePaid
	if cryptoPaid < 0 {
		log.Printf(
			"payment breakdown inconsistency: bill_id=%d paid_amount_cents=%d confirmed_alternative_cents=%d",
			bill.ID,
			bill.PaidAmount,
			alternativePaid,
		)
		cryptoPaid = 0
	}

	remaining := bill.TotalAmount - bill.PaidAmount
	if remaining < 0 {
		remaining = 0
	}

	breakdown := &database.PaymentBreakdown{
		TotalAmount:     bill.TotalAmount,
		CryptoPaid:      cryptoPaid,
		AlternativePaid: alternativePaid,
		Remaining:       remaining,
		IsComplete:      bill.Status == database.BillStatusPaid,
	}

	return breakdown, nil
}

// authorizeBillManagement authorizes destructive bill management (void/refund)
// and recovery reads (pending alternative payments, audit). It returns
// (actor, true) for the business owner — matched by the web3 owner address or
// the linked user_id — and for platform admins (ResolveContextPermissions
// allAccess). No staff role is authorized here; the mutation routes stay gated
// on the owner-only bills:refund permission to match.
func (h *PaymentHandler) authorizeBillManagement(c *gin.Context, bill *database.Bill) (string, bool) {
	if _, allAccess := server.ResolveContextPermissions(c); allAccess {
		return paymentActorFromContext(c), true
	}

	business, err := h.getPaymentManagementBusiness(bill.BusinessID)
	if err != nil {
		return "", false
	}

	switch c.GetString("token_type") {
	case "web3":
		address := getStringFromContext(c, "address", "wallet_address")
		if address != "" && strings.EqualFold(address, business.OwnerAddress) {
			return address, true
		}
	case "user":
		if userIDVal, exists := c.Get("user_id"); exists && business.UserID != nil {
			if userID, ok := normalizeContextUint(userIDVal); ok && userID == *business.UserID {
				confirmedBy := getStringFromContext(c, "email", "address", "wallet_address")
				if confirmedBy == "" {
					confirmedBy = fmt.Sprintf("user:%d", userID)
				}
				return confirmedBy, true
			}
		}

	}

	return "", false
}

// authorizeBillPayment authorizes recording in-person tender (cash/card/venmo/other).
// Owners/platform admins pass via authorizeBillManagement or allAccess; floor
// staff pass when they hold bills:payment on the active business.
func (h *PaymentHandler) authorizeBillPayment(c *gin.Context, bill *database.Bill) (string, bool) {
	if confirmedBy, ok := h.authorizeBillManagement(c, bill); ok {
		return confirmedBy, true
	}

	perms, allAccess := server.ResolveContextPermissions(c)
	if allAccess {
		return paymentActorFromContext(c), true
	}
	for _, p := range perms {
		if p == string(server.PermBillsPayment) {
			return paymentActorFromContext(c), true
		}
	}
	return "", false
}

func paymentActorFromContext(c *gin.Context) string {
	if name, ok := c.Get("staff_name"); ok {
		if s, ok := name.(string); ok && strings.TrimSpace(s) != "" {
			return "staff:" + strings.TrimSpace(s)
		}
	}
	if staffID := server.ExtractStaffIDFromContext(c); staffID != nil {
		return fmt.Sprintf("staff:%d", *staffID)
	}
	if addr := getStringFromContext(c, "address", "wallet_address", "email"); addr != "" {
		return addr
	}
	if userIDVal, exists := c.Get("user_id"); exists {
		if userID, ok := normalizeContextUint(userIDVal); ok && userID != 0 {
			return fmt.Sprintf("user:%d", userID)
		}
	}
	return "operator"
}

func (h *PaymentHandler) getPaymentManagementBusiness(businessID uint) (*database.Business, error) {
	var business database.Business
	if err := h.db.GetGorm().
		Select("id", "owner_address", "user_id").
		First(&business, businessID).Error; err != nil {
		return nil, err
	}
	return &business, nil
}

func (h *PaymentHandler) getPaymentLockBusiness(businessID uint) (*database.Business, error) {
	var business database.Business
	if err := h.db.GetGorm().
		// IsBusinessOperational inputs: the demo short-circuit and the admin
		// close/suspend lock. Without them a closed venue keeps taking guest
		// payments.
		Select("id", "is_demo", "is_active", "closed_at").
		First(&business, businessID).Error; err != nil {
		return nil, err
	}
	return &business, nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

var errInvalidBillIdentifier = errors.New("invalid bill identifier")

func (h *PaymentHandler) resolveBillPaymentSummary(c *gin.Context, fallback string) (*database.Bill, error) {
	query := h.db.GetGorm().Select("id", "business_id", "bill_number", "total_amount", "paid_amount", "status")
	var bill database.Bill
	// Public guest routes bind :bill_token (public_token capability only).
	if token := strings.TrimSpace(c.Param("bill_token")); token != "" {
		if err := query.Where(database.PublicBillTokenWhere, token).First(&bill).Error; err != nil {
			return nil, err
		}
		return &bill, nil
	}

	raw := firstNonEmptyString(fallback, c.Param("id"), c.Param("bill_id"))
	if raw == "" {
		return nil, errInvalidBillIdentifier
	}

	if parsedID, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 32); err == nil {
		if err := query.First(&bill, uint(parsedID)).Error; err != nil {
			return nil, err
		}
		return &bill, nil
	}

	// Non-numeric fallback is a public_token (guest capability), never bill_number.
	if err := query.Where(database.PublicBillTokenWhere, strings.TrimSpace(raw)).First(&bill).Error; err != nil {
		return nil, errInvalidBillIdentifier
	}
	return &bill, nil
}

func getStringFromContext(c *gin.Context, keys ...string) string {
	for _, key := range keys {
		if value, exists := c.Get(key); exists {
			str := strings.TrimSpace(fmt.Sprint(value))
			if str != "" && str != "<nil>" {
				return str
			}
		}
	}
	return ""
}

func normalizeContextUint(value interface{}) (uint, bool) {
	switch v := value.(type) {
	case uint:
		return v, true
	case uint64:
		return uint(v), true
	case int:
		if v >= 0 {
			return uint(v), true
		}
	case int64:
		if v >= 0 {
			return uint(v), true
		}
	case float64:
		if v >= 0 {
			return uint(v), true
		}
	case string:
		parsed, err := strconv.ParseUint(strings.TrimSpace(v), 10, 32)
		if err == nil {
			return uint(parsed), true
		}
	}
	return 0, false
}

// CryptoPaymentRequest represents a crypto payment request
type CryptoPaymentRequest struct {
	TransactionHash   string  `json:"transaction_hash" binding:"required"`
	AmountPaid        float64 `json:"amount_paid" binding:"required,gt=0,lte=1000000"`
	TipAmount         float64 `json:"tip_amount" binding:"gte=0,lte=100000"`
	PaymentMethod     string  `json:"payment_method" binding:"required"`
	BlockchainNetwork string  `json:"blockchain_network"`
	QuoteToken        string  `json:"quote_token" binding:"required"`
	SplitShareID      *uint   `json:"split_share_id"`
}

func (h *PaymentHandler) getGuestBillByToken(c *gin.Context) (*database.Bill, error) {
	token := strings.TrimSpace(c.Param("bill_token"))
	if token == "" {
		return nil, fmt.Errorf("bill not found")
	}

	var bill database.Bill
	if err := h.db.GetGorm().
		Select(
			"id",
			"business_id",
			"bill_number",
			"total_amount",
			"paid_amount",
			"status",
			"settlement_addr",
			"tipping_addr",
			"crm_customer_id",
		).
		Where(database.PublicBillTokenWhere, token).
		First(&bill).Error; err != nil {
		return nil, err
	}
	return &bill, nil
}

// cryptoQuoteRequest is the input to the quote endpoint: the local-currency
// amount and tip the guest intends to pay (dollars).
type cryptoQuoteRequest struct {
	AmountPaid    float64 `json:"amount_paid" binding:"required,gt=0,lte=1000000"`
	TipAmount     float64 `json:"tip_amount" binding:"gte=0,lte=100000"`
	SplitShareID  *uint   `json:"split_share_id"`
	PaymentMethod string  `json:"payment_method"`
}

// cryptoQuoteResponse returns the EXACT USDC amount the guest must transfer
// (micro-units + display dollars: the converted amount plus the quote's unique
// sub-cent offset), the rate used, the token expiry, the persisted quote id,
// and the signed token.
type cryptoQuoteResponse struct {
	QuoteID        uint    `json:"quote_id"`
	USDMicrounits  int64   `json:"usd_microunits"`
	USDAmount      float64 `json:"usd_amount"`
	Rate           float64 `json:"rate"`
	SettlementAddr string  `json:"settlement_address"`
	ChainID        int64   `json:"chain_id"`
	Token          string  `json:"token"`
	ExpiresAt      int64   `json:"expires_at"`
	QuoteToken     string  `json:"quote_token"`
}

const cryptoQuoteTTL = 30 * time.Minute
const splitGuestPaymentHoldTTL = cryptoQuoteTTL

const (
	guestPaymentMethodUSDC       = "usdc_payment"
	guestPaymentMethodCrossChain = "cross_chain_payment"
	baseMainnetChainID           = int64(8453)
	baseSepoliaChainID           = int64(84532)
	// maxPlaceholderSettlement is the inclusive ceiling of the reserved
	// low-address range used by seeds and fixtures. 0x…dE01 is the demo
	// venue wallet; anything at or below it is not a funded payout target.
	maxPlaceholderSettlement = int64(0xDe01)
)

type guestSettlementContract struct {
	Address       string
	ChainID       int64
	Token         string
	PaymentMethod string
}

func normalizeCryptoQuotePaymentMethod(value string) (string, bool) {
	method := strings.ToLower(strings.TrimSpace(value))
	// Compatibility for clients deployed before the method discriminator: the
	// only quote consumer at that point was the direct-USDC flow.
	if method == "" {
		method = guestPaymentMethodUSDC
	}
	switch method {
	case guestPaymentMethodUSDC, guestPaymentMethodCrossChain:
		return method, true
	default:
		return "", false
	}
}

func (h *PaymentHandler) resolveGuestSettlementContract(c *gin.Context, bill *database.Bill, requestedMethod string) (guestSettlementContract, bool) {
	var contract guestSettlementContract
	method, ok := normalizeCryptoQuotePaymentMethod(requestedMethod)
	if !ok {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Unsupported crypto payment method")
		return contract, false
	}
	if !h.requireGuestCryptoPlugin(c, bill, method) {
		return contract, false
	}
	return h.currentGuestSettlementContract(c, bill, method)
}

func (h *PaymentHandler) currentGuestSettlementContract(c *gin.Context, bill *database.Bill, method string) (guestSettlementContract, bool) {
	var contract guestSettlementContract
	address := strings.TrimSpace(bill.SettlementAddr)
	if !common.IsHexAddress(address) {
		server.RespondWithError(c, http.StatusUnprocessableEntity, "plugin_unavailable", "This business has not configured a valid settlement wallet")
		return contract, false
	}
	parsed := common.HexToAddress(address)
	if isUnusableGuestSettlementAddress(parsed) {
		server.RespondWithError(c, http.StatusUnprocessableEntity, "plugin_unavailable", "This business has not configured a valid settlement wallet")
		return contract, false
	}
	// One lookup covers the demo refusal and the payout-wallet freshness
	// check. The demo refusal stays Base-mainnet-only; a rotated wallet must
	// be rejected on every chain before a guest signs a stale recipient.
	isDemo, businessSettlement, loaded := h.guestBusinessSettlementState(bill.BusinessID)
	if loaded && !strings.EqualFold(strings.TrimSpace(businessSettlement), address) {
		server.RespondWithError(c, http.StatusConflict, "settlement_wallet_changed", "This business changed its payout wallet; refresh the bill and try again")
		return contract, false
	}
	provider, ok := h.verifier.(guestSettlementContractProvider)
	if !ok || provider == nil {
		server.RespondWithError(c, http.StatusUnprocessableEntity, "plugin_unavailable", "Crypto settlement verification is unavailable")
		return contract, false
	}
	chainID := provider.ChainID()
	token := strings.ToUpper(strings.TrimSpace(provider.TokenSymbolForChain()))
	if !guestSettlementChainSupported(chainID, method) || token != "USDC" {
		server.RespondWithError(c, http.StatusUnprocessableEntity, "plugin_unavailable", "Crypto settlement configuration is unsupported")
		return contract, false
	}
	if chainID == baseMainnetChainID && (!loaded || isDemo) {
		server.RespondWithError(c, http.StatusUnprocessableEntity, "plugin_unavailable", "This business is not currently accepting this crypto payment method")
		return contract, false
	}

	return guestSettlementContract{
		Address:       parsed.Hex(),
		ChainID:       chainID,
		Token:         token,
		PaymentMethod: method,
	}, true
}

// guestSettlementChainSupported reports whether guest crypto settlement may
// be quoted and verified on chainID for method. Base mainnet is always
// supported. Base Sepolia is development/test only (direct USDC): testnet
// USDC is free to mint, so a production instance verifying Sepolia transfers
// would settle real bills for nothing. The production lookup runs only on the
// Sepolia branch, keeping the mainnet hot path a constant compare.
func guestSettlementChainSupported(chainID int64, method string) bool {
	switch chainID {
	case baseMainnetChainID:
		return true
	case baseSepoliaChainID:
		return method == guestPaymentMethodUSDC && !appconfig.IsProductionMode(false)
	default:
		return false
	}
}

// isUnusableGuestSettlementAddress reports burn, precompile, and seed
// placeholder destinations. common.IsHexAddress accepts these, but a signed
// mainnet quote to any of them would destroy guest funds.
func isUnusableGuestSettlementAddress(addr common.Address) bool {
	if addr == (common.Address{}) {
		return true
	}
	return new(big.Int).SetBytes(addr.Bytes()).Cmp(big.NewInt(maxPlaceholderSettlement)) <= 0
}

// guestBusinessSettlementState loads is_demo and settlement_addr in one
// query. ok is false when the row cannot be read; callers treat that as
// demo (fail closed) and skip the wallet comparison, matching the previous
// guestBusinessIsDemo miss path.
func (h *PaymentHandler) guestBusinessSettlementState(businessID uint) (isDemo bool, settlementAddr string, ok bool) {
	if h.db == nil || h.db.GetGorm() == nil || businessID == 0 {
		return false, "", false
	}
	var row struct {
		IsDemo         bool   `gorm:"column:is_demo"`
		SettlementAddr string `gorm:"column:settlement_addr"`
	}
	err := h.db.GetGorm().
		Table("businesses").
		Select("is_demo, settlement_addr").
		Where("id = ?", businessID).
		Take(&row).Error
	if err != nil {
		return false, "", false
	}
	return row.IsDemo, row.SettlementAddr, true
}

func reserveGuestPaymentSplitShare(c *gin.Context, bill *database.Bill, splitShareID *uint) (*database.BillSplitShare, bool) {
	if splitShareID == nil {
		return nil, true
	}
	if bill == nil || *splitShareID == 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid split share")
		return nil, false
	}
	guestSessionID, ok := getExistingGuestSession(c)
	if !ok {
		server.RespondWithError(c, http.StatusConflict, "split_share_conflict", "Split share is no longer available")
		return nil, false
	}

	share, err := database.ExtendBillSplitShareHoldForGuest(*splitShareID, bill.ID, guestSessionID, time.Now().UTC(), splitGuestPaymentHoldTTL)
	if err != nil {
		switch {
		case errors.Is(err, database.ErrSplitShareNotFound):
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Split share does not belong to this bill")
		case errors.Is(err, database.ErrSplitHoldExpired), errors.Is(err, database.ErrSplitShareAlreadyFinal), errors.Is(err, database.ErrSplitGuestMismatch):
			server.RespondWithError(c, http.StatusConflict, "split_share_conflict", "Split share is no longer available")
		default:
			server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to reserve split share")
		}
		return nil, false
	}
	return share, true
}

// guardCryptoPaymentAmountWithinRemaining rejects an amount that exceeds the
// cashier remaining: total − paid − pending alternative requests − held
// split shares (same definition as CreatePendingRequest). reservedByThisQuote
// is the guest's already-held split share and is not reserved twice. Tips are
// not charged against remaining (same rule as applyBillPaymentAmounts).
// Settlement paths re-check inside ApplyConfirmedPayment (after tx-hash
// idempotency) so guest retries of a partial payment are not 409'd incorrectly.
// Returns false after writing the error response when the amount is too high.
func guardCryptoPaymentAmountWithinRemaining(c *gin.Context, bill *database.Bill, amountCents, reservedByThisQuote int64) bool {
	if bill == nil {
		server.RespondWithError(c, http.StatusConflict, "payment_failed",
			"Payment exceeds remaining bill balance")
		return false
	}
	exceeds, err := database.CryptoQuoteExceedsRemaining(*bill, amountCents, reservedByThisQuote, time.Now().UTC())
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to check remaining bill balance")
		return false
	}
	if exceeds {
		server.RespondWithError(c, http.StatusConflict, "payment_failed",
			"Payment exceeds remaining bill balance")
		return false
	}
	return true
}

// guardCryptoQuoteBillPayable rejects crypto-quote issuance for bills that can
// no longer accept payment. USDC transfers are irreversible, so no signed
// quote may be minted for a dead bill: (1) the bill itself is terminal
// (paid/closed/voided), or (2) the bill belongs to a delivery order that is
// cancelled/failed, or whose prepay payment window has already lapsed even if
// the expiry sweeper has not cancelled it yet (delivery_lifecycle.go sweep).
// The delivery check is a single indexed lookup on delivery_orders.bill_id
// projecting only status + payment_expires_at — never the full aggregate.
// Returns false after writing the error response when the quote must not be
// issued.
func (h *PaymentHandler) guardCryptoQuoteBillPayable(c *gin.Context, bill *database.Bill) bool {
	switch bill.Status {
	case database.BillStatusPaid, database.BillStatusClosed, database.BillStatusVoided:
		server.RespondWithError(c, http.StatusConflict, "payment_failed",
			"This bill is no longer accepting payments")
		return false
	}

	var links []struct {
		Status           database.DeliveryStatus
		PaymentExpiresAt *time.Time
	}
	if err := h.db.GetGorm().
		Model(&database.DeliveryOrder{}).
		Select("status", "payment_expires_at").
		Where("bill_id = ?", bill.ID).
		Order("id DESC").
		Limit(1).
		Find(&links).Error; err != nil {
		// Fail closed: this gate protects an irreversible money path.
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to validate order status")
		return false
	}
	if len(links) == 0 {
		return true // not a delivery bill
	}

	link := links[0]
	switch {
	case link.Status == database.DeliveryStatusCancelled || link.Status == database.DeliveryStatusFailed:
		server.RespondWithError(c, http.StatusConflict, "payment_failed",
			"This delivery order has been cancelled and can no longer be paid")
		return false
	case link.Status == database.DeliveryStatusConfirmed &&
		link.PaymentExpiresAt != nil && time.Now().After(*link.PaymentExpiresAt):
		server.RespondWithError(c, http.StatusConflict, "payment_failed",
			"The payment window for this delivery order has expired")
		return false
	}
	return true
}

// billCurrency returns the currency a bill is denominated in (the business's
// DefaultCurrency), defaulting to USD when unset.
func (h *PaymentHandler) billCurrency(businessID uint) string {
	var business database.Business
	if err := h.db.GetGorm().
		Select("id", "default_currency").
		First(&business, businessID).Error; err != nil {
		return "USD"
	}
	currency := strings.ToUpper(strings.TrimSpace(business.DefaultCurrency))
	if currency == "" {
		return "USD"
	}
	return currency
}

// IssueCryptoQuote locks a USD settlement amount for a crypto payment.
// POST /api/v1/guest/bill/:bill_number/crypto-quote
func (h *PaymentHandler) IssueCryptoQuote(c *gin.Context) {
	var req cryptoQuoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if method, ok := normalizeCryptoQuotePaymentMethod(req.PaymentMethod); ok && method == guestPaymentMethodCrossChain && !guestCrossChainSettlementEnabled {
		respondGuestCrossChainDisabled(c)
		return
	}

	amountCents := dollarsToCents(req.AmountPaid)
	tipCents := dollarsToCents(req.TipAmount)
	if err := validatePaymentAmounts(amountCents, tipCents); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}

	bill, err := h.getGuestBillByToken(c)
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Bill not found")
		return
	}

	// Terminal-bill / dead-delivery gate: a stale pay page (or direct API
	// call) must not obtain a signed quote for a bill whose linked delivery
	// order was cancelled or whose payment window expired — the resulting
	// on-chain USDC transfer would be irreversible and unrecordable.
	if !h.guardCryptoQuoteBillPayable(c, bill) {
		return
	}

	// Same admin-lifecycle guard as the payment path.
	if billBusiness, err := h.getPaymentLockBusiness(bill.BusinessID); err != nil || billBusiness == nil || !database.IsBusinessOperational(billBusiness) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "This business is not currently accepting payments",
			"code":  services.OrderErrCodeBusinessUnavailable,
		})
		return
	}

	settlement, ok := h.resolveGuestSettlementContract(c, bill, req.PaymentMethod)
	if !ok {
		return
	}

	if h.exchangeRates == nil {
		server.RespondWithError(c, http.StatusServiceUnavailable, "", "Currency conversion unavailable")
		return
	}

	var reservedByThisQuote int64
	if share, ok := reserveGuestPaymentSplitShare(c, bill, req.SplitShareID); !ok {
		return
	} else if share != nil {
		amountCents = share.AmountCents
		reservedByThisQuote = share.AmountCents
	}

	// Cap principal (not tip) to remaining balance before minting a signed
	// quote. Without this, a guest can lock usd_amount far above what
	// ApplyConfirmedPayment will accept, send irreversible USDC against that
	// quote, then fail at ledger with ErrPaymentExceedsRemaining.
	if !guardCryptoPaymentAmountWithinRemaining(c, bill, amountCents, reservedByThisQuote) {
		return
	}

	localCents := amountCents + tipCents
	currency := h.billCurrency(bill.BusinessID)
	rate := 1.0
	var usdMicrounits int64
	if currency == "USD" {
		// USD bills settle 1:1 (1 cent = 10_000 USDC micro-units); compute the
		// floor exactly from integer cents to avoid float rounding drift.
		usdMicrounits = centsToMicrounits(localCents)
	} else {
		// Use the fail-closed quote converter: a stale/missing rate must abort
		// the quote rather than lock the guest into a possibly-wrong amount.
		converted, err := h.exchangeRates.ConvertAmountForQuote(float64(localCents)/100.0, currency, "USD")
		if err != nil {
			server.RespondWithError(c, http.StatusServiceUnavailable, "", "Currency conversion unavailable")
			return
		}
		if localCents > 0 {
			rate = converted / (float64(localCents) / 100.0)
		}
		usdMicrounits = int64(math.Round(converted * 1_000_000))
	}
	if usdMicrounits <= 0 {
		server.RespondWithError(c, http.StatusServiceUnavailable, "", "Currency conversion unavailable")
		return
	}

	// Reserve this quote's exact amount: the converted amount plus a unique
	// sub-cent offset no other live quote for this wallet holds. The persisted
	// row (not the token) is what settlement checks and consumes.
	quote, ok := issueGuestCryptoQuote(c, bill, settlement, usdMicrounits, time.Now(), guestCryptoQuoteClientKey(h.quoteSecret, c.ClientIP()))
	if !ok {
		return
	}
	exactMicrounits := quote.ExactMicrounits
	exp := quote.ExpiresAt.Unix()
	token := signCryptoQuote(cryptoQuoteClaims{
		BusinessID:     bill.BusinessID,
		QuoteID:        quote.ID,
		BillID:         bill.ID,
		LocalCents:     localCents,
		USDMicrounits:  exactMicrounits,
		SettlementAddr: settlement.Address,
		ChainID:        settlement.ChainID,
		Token:          settlement.Token,
		PaymentMethod:  settlement.PaymentMethod,
		Iat:            quote.IssuedAt.Unix(),
		Exp:            exp,
	}, h.quoteSecret)

	c.JSON(http.StatusOK, cryptoQuoteResponse{
		QuoteID:        quote.ID,
		USDMicrounits:  exactMicrounits,
		USDAmount:      float64(exactMicrounits) / 1_000_000.0,
		Rate:           rate,
		SettlementAddr: settlement.Address,
		ChainID:        settlement.ChainID,
		Token:          settlement.Token,
		ExpiresAt:      exp,
		QuoteToken:     token,
	})
}

// ProcessCryptoPayment handles crypto payment completion
// POST /api/v1/guest/bill/:bill_number/crypto-payment
func (h *PaymentHandler) ProcessCryptoPayment(c *gin.Context) {
	var req CryptoPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if !canonicalizeGuestTxHash(c, &req.TransactionHash) {
		return
	}
	if req.TipAmount < 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "tip amount cannot be negative")
		return
	}

	amountCents := dollarsToCents(req.AmountPaid)
	tipCents := dollarsToCents(req.TipAmount)
	if err := validatePaymentAmounts(amountCents, tipCents); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}

	bill, err := h.getGuestBillByToken(c)
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Bill not found")
		return
	}

	// Reject crypto settlement against suspended/closed businesses. Prevents
	// replaying a tx hash against a disabled business to appear paid.
	if billBusiness, err := h.getPaymentLockBusiness(bill.BusinessID); err != nil || billBusiness == nil || !database.IsBusinessOperational(billBusiness) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "This business is not currently accepting payments",
			"code":  services.OrderErrCodeBusinessUnavailable,
		})
		return
	}

	if !h.requireGuestCryptoPlugin(c, bill, "usdc_payment") {
		return
	}

	if h.verifier == nil {
		server.RespondWithError(c, http.StatusServiceUnavailable, "", "Payment verification service unavailable")
		return
	}

	var splitShare *database.BillSplitShare
	if share, ok := reserveGuestPaymentSplitShare(c, bill, req.SplitShareID); !ok {
		return
	} else if share != nil {
		splitShare = share
		amountCents = share.AmountCents
	}

	// Remaining is enforced at quote issuance (IssueCryptoQuote) and again
	// inside ApplyConfirmedPayment. Do not re-check here before the
	// tx-hash idempotency path: a successful first apply reduces remaining,
	// and a guest retry of the same hash would 409 incorrectly.
	// The signed quote locks the USD amount (immune to FX drift) and binds it to
	// this bill + the amounts being ledgered. Payer binding: the token names a
	// persisted quote whose EXACT amount no other live quote for this wallet
	// shares, so the on-chain transfer must carry exactly that amount (never
	// "at least"), and the quote is consumed with the payment insert.
	quoteClaims, ok := h.parseAndCheckQuote(c, req.QuoteToken, bill, amountCents+tipCents, guestPaymentMethodUSDC)
	if !ok {
		return
	}
	boundQuote, ok := loadBoundGuestCryptoQuote(c, &quoteClaims, bill, req.TransactionHash, time.Now())
	if !ok {
		return
	}
	lockedUSDMicrounits := boundQuote.ExactMicrounits
	transferEvidence, err := h.verifyGuestSettlementWithEvidence(
		c.Request.Context(),
		req.TransactionHash,
		quoteClaims.SettlementAddr,
		lockedUSDMicrounits,
		false, // exact: the unique amount is what ties the transfer to this quote
	)
	if err != nil {
		if isGuestTransferAmountMismatch(err) {
			// A transfer that already paid another bill (or this bill under
			// another quote) carries that quote's amount, not this one's: it
			// is a replay, not a wrong-amount payment, and must be reported
			// as one before staff are invited to settle or refund it.
			if respondIfGuestTransferAlreadyRecorded(c, bill, boundQuote.ID, req.TransactionHash, guestPaymentMethodUSDC) {
				return
			}
			respondGuestTransferAmountMismatch(c, bill, boundQuote, req.TransactionHash, err)
			return
		}
		respondCryptoVerifyError(c, err)
		return
	}
	// Same for a recorded transfer replayed under a later quote that reused
	// its exact amount: it predates the quote, but it is a replay first.
	if transferEvidence.BlockTimestamp != 0 &&
		transferPredatesQuote(quoteClaims, transferEvidence.BlockTimestamp) &&
		respondIfGuestTransferAlreadyRecorded(c, bill, boundQuote.ID, req.TransactionHash, guestPaymentMethodUSDC) {
		return
	}
	if !requireTransferAfterQuote(c, quoteClaims, transferEvidence) {
		return
	}
	// Normalize the verified proof before settlement and pass it into the DB
	// transaction. Empty evidence is an explicit verify-only fallback; otherwise
	// payment + refund destination commit or roll back together.
	if transferEvidence.AmountBaseUnits <= 0 {
		transferEvidence.AmountBaseUnits = lockedUSDMicrounits
	}
	if strings.TrimSpace(transferEvidence.TxHash) == "" {
		transferEvidence.TxHash = req.TransactionHash
	}
	refundDestination := refundDestinationFromTransferEvidence(transferEvidence)

	resolveBillCRMCustomer(c, bill)

	// Guest-initiated crypto payments usually carry no staff context, in
	// which case this evaluates to nil and leaves ClosedByStaffID unset.
	// When a staff member drives the flow from a POS/register device it
	// correctly attributes the closure atomically.
	closingStaffID := server.ExtractStaffIDFromContext(c)

	// Capture pre-settle bill: ApplyConfirmedPayment / SettleBillSplitShare
	// return a nil bill pointer on error, and we still need the scalars for
	// the settlement-review alert below.
	preSettleBill := bill
	applied := false
	if splitShare != nil {
		splitBlockNum, splitBlockHash := blockEvidencePointers(transferEvidence)
		_, settledBill, wasApplied, settleErr := database.SettleBillSplitShare(database.SettleBillSplitShareInput{
			ShareID:           splitShare.ID,
			GuestSessionID:    splitShare.GuestSessionID,
			IdempotencyKey:    req.TransactionHash,
			Tender:            "crypto",
			TxHash:            req.TransactionHash,
			PayerAddr:         "crypto_guest",
			TipCents:          tipCents,
			RefundDestination: refundDestination,
			BlockNumber:       splitBlockNum,
			BlockHash:         splitBlockHash,
			Now:               time.Now().UTC(),

			CryptoQuoteID:              boundQuote.ID,
			CryptoQuoteExactMicrounits: boundQuote.ExactMicrounits,
		})
		bill = settledBill
		applied = wasApplied
		err = settleErr
	} else {
		blockNum, blockHash := blockEvidencePointers(transferEvidence)
		bill, applied, err = database.ApplyConfirmedPayment(database.ConfirmedPaymentInput{
			BillID:            bill.ID,
			PayerAddr:         "crypto_guest",
			Amount:            amountCents,
			TipAmount:         tipCents,
			TxHash:            req.TransactionHash,
			Status:            database.PaymentStatusConfirmed,
			PaymentMethod:     "crypto",
			RefundDestination: refundDestination,
			BlockNumber:       blockNum,
			BlockHash:         blockHash,

			CryptoQuoteID:              boundQuote.ID,
			CryptoQuoteExactMicrounits: boundQuote.ExactMicrounits,
			PayerGuestSession:          guestPayerSession(c),
		}, closingStaffID)
	}
	if err != nil {
		// A verified transfer that is already recorded elsewhere is the
		// replay signature: page the operator, never settle.
		if errors.Is(err, database.ErrPaymentTxHashConflict) {
			holder, lookupErr := database.FindPaymentTxHashHolder(req.TransactionHash)
			if lookupErr != nil {
				log.Printf("failed to find the payment holding crypto transfer %s (bill %d): %v", req.TransactionHash, boundQuote.BillID, lookupErr)
			}
			respondGuestCryptoTxAlreadyRecorded(c, preSettleBill, boundQuote.ID, req.TransactionHash, guestPaymentMethodUSDC, holder)
			return
		}
		// Verified on-chain but not recorded: this is real money with no DB
		// row. Duplicate-hash conflicts are NOT that case — the payment is
		// already recorded under the same tx (handled above).
		if preSettleBill != nil {
			alertSvc := operational_alerts.NewService(database.GetDB())
			if alertErr := alertSvc.CreateCryptoSettlementReviewAlert(c.Request.Context(), *preSettleBill, req.TransactionHash, map[string]any{
				"amount_cents": amountCents,
				"tip_cents":    tipCents,
			}); alertErr != nil {
				log.Printf("failed to create crypto settlement review alert for bill %d: %v", preSettleBill.ID, alertErr)
			}
		}
		status, message := paymentWriteError(err)
		server.RespondWithError(c, status, paymentWriteErrorCode(err), message)
		return
	}

	// Only fire downstream side effects for newly applied payments.
	if applied && bill.Status == database.BillStatusPaid {
		// The in-memory bill is the lean payment-summary projection and is stale
		// after the settlement write. Reload the fresh SCALAR columns only — every
		// downstream consumer here reads scalar bill fields (and re-fetches its own
		// relations), so the heavy GetBillByID aggregate hydration is over-fetch.
		if reloadedBill, _, reloadErr := database.GetBillByIDLean(bill.ID); reloadErr == nil {
			bill = reloadedBill
		} else {
			log.Printf("Failed to reload bill %d after crypto payment for CRM attribution: %v", bill.ID, reloadErr)
		}
		recordCRMSettlementVisitForPaidBill(bill, "crypto")
		paymentMethodStr := req.PaymentMethod
		if paymentMethodStr == "" {
			paymentMethodStr = "USDC"
		}
		h.sendPaymentCompletionEmails(bill, bill.CRMCustomerID, req.TransactionHash, paymentMethodStr)
		enqueueFiscalJobForPaidBill(bill, nil, "crypto")
		enqueueReceiptForPaidBill(bill)
	}

	h.flushPendingMilestones(bill.BusinessID)

	if applied {
		// Match alt/plugin/webhook: tip_amount + bill.updated via the shared helper.
		publishRecordedPaymentSSE(bill, amountCents, tipCents, "crypto")
		publishGuestSplitStateForBill(bill)
		if sharedDeliveryService != nil {
			if err := sharedDeliveryService.HandleDeliveryBillPaid(bill.ID); err != nil {
				log.Printf("delivery payment hook failed for bill %d: %v", bill.ID, err)
			}
		}
		createPaymentReceivedOperationalAlertForTxHash(c.Request.Context(), bill, req.TransactionHash, map[string]any{
			"amount_cents":      amountCents,
			"tip_cents":         tipCents,
			"method":            "crypto",
			"payment_status":    string(database.PaymentStatusConfirmed),
			"settlement_source": "guest_crypto",
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"message":          map[bool]string{true: "Payment processed successfully", false: "Payment already processed"}[applied],
		"bill_number":      bill.BillNumber,
		"transaction_hash": req.TransactionHash,
		"amount_paid":      centsToDollars(amountCents),
		"tip_amount":       req.TipAmount,
		"bill_status":      bill.Status,
		"remaining_amount": centsToDollars(bill.TotalAmount - bill.PaidAmount),
	})
}

// CrossChainPaymentRequest represents a cross-chain payment via LI.FI
type CrossChainPaymentRequest struct {
	TransactionHash string  `json:"transaction_hash" binding:"required"`
	AmountPaid      float64 `json:"amount_paid" binding:"required,gt=0,lte=1000000"`
	TipAmount       float64 `json:"tip_amount" binding:"gte=0,lte=100000"`
	SourceChain     string  `json:"source_chain" binding:"required"`
	SourceToken     string  `json:"source_token" binding:"required"`
	LifiRouteId     string  `json:"lifi_route_id"`
	QuoteToken      string  `json:"quote_token" binding:"required"`
	SplitShareID    *uint   `json:"split_share_id"`
	// Wave 4 Task 11: guest wallet signature binding refund destination.
	// Required for new clients; when missing, settlement may still succeed but
	// no refund destination is stored (operator UI: manual support required).
	RefundAddress          string `json:"refund_address"`
	RefundBindingSignature string `json:"refund_binding_signature"`
	RefundBindingChainID   int64  `json:"refund_binding_chain_id"`
	RefundBindingExp       int64  `json:"refund_binding_exp"`
}

// ProcessCrossChainPayment handles cross-chain payment completion via LI.FI
// POST /api/v1/guest/bill/:bill_number/cross-chain-payment
func (h *PaymentHandler) ProcessCrossChainPayment(c *gin.Context) {
	if !guestCrossChainSettlementEnabled {
		respondGuestCrossChainDisabled(c)
		return
	}
	var req CrossChainPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if !canonicalizeGuestTxHash(c, &req.TransactionHash) {
		return
	}
	if req.TipAmount < 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "tip amount cannot be negative")
		return
	}

	amountCents := dollarsToCents(req.AmountPaid)
	tipCents := dollarsToCents(req.TipAmount)
	if err := validatePaymentAmounts(amountCents, tipCents); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}

	bill, err := h.getGuestBillByToken(c)
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Bill not found")
		return
	}

	// Reject cross-chain settlement against suspended/closed businesses.
	if billBusiness, err := h.getPaymentLockBusiness(bill.BusinessID); err != nil || billBusiness == nil || !database.IsBusinessOperational(billBusiness) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "This business is not currently accepting payments",
			"code":  services.OrderErrCodeBusinessUnavailable,
		})
		return
	}

	if !h.requireGuestCryptoPlugin(c, bill, "cross_chain_payment") {
		return
	}

	if h.verifier == nil {
		server.RespondWithError(c, http.StatusServiceUnavailable, "", "Payment verification service unavailable")
		return
	}

	var splitShare *database.BillSplitShare
	if share, ok := reserveGuestPaymentSplitShare(c, bill, req.SplitShareID); !ok {
		return
	} else if share != nil {
		splitShare = share
		amountCents = share.AmountCents
	}

	// Remaining is enforced at quote issuance and inside ApplyConfirmedPayment.
	// Skipping an early check here preserves tx-hash idempotent retries after
	// the first apply reduced remaining (same rationale as ProcessCryptoPayment).
	// Threshold the cross-chain settlement against the locked USD quote amount.
	// At-least tolerates bridge fees/slippage above the locked floor.
	// Bridge contract sender is NOT a safe refund destination — require a
	// guest wallet signature binding the refund address when provided.
	quoteClaims, ok := h.parseAndCheckQuote(c, req.QuoteToken, bill, amountCents+tipCents, guestPaymentMethodCrossChain)
	if !ok {
		return
	}
	lockedUSDMicrounits := quoteClaims.USDMicrounits

	// Verify refund-destination binding BEFORE settlement when the guest
	// supplied a signature. The binding is optional (the guest web app does not
	// send one); without it there is no destination and refunds are manual.
	var verifiedRefundAddr string
	var refundSigRef string
	if strings.TrimSpace(req.RefundBindingSignature) != "" || strings.TrimSpace(req.RefundAddress) != "" {
		chainID := req.RefundBindingChainID
		if chainID == 0 {
			chainID = quoteClaims.ChainID
		}
		binding := RefundDestinationBinding{
			Domain:          refundBindingDomain(),
			BillID:          bill.ID,
			BusinessID:      bill.BusinessID,
			SourceTx:        req.TransactionHash,
			AmountBaseUnits: lockedUSDMicrounits,
			ChainID:         chainID,
			Destination:     quoteClaims.SettlementAddr,
			RefundAddress:   req.RefundAddress,
			Exp:             req.RefundBindingExp,
		}
		if err := VerifyRefundDestinationBinding(
			binding,
			req.RefundBindingSignature,
			bill.ID,
			bill.BusinessID,
			req.TransactionHash,
			lockedUSDMicrounits,
			chainID,
			quoteClaims.SettlementAddr,
			time.Now().UTC(),
		); err != nil {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid refund destination binding: "+err.Error())
			return
		}
		verifiedRefundAddr = strings.TrimSpace(req.RefundAddress)
		refundSigRef = strings.TrimSpace(req.RefundBindingSignature)
	}

	// Evidence is requested only for the mined block time (quote-window
	// binding). Its Transfer-log sender is the bridge contract and is never
	// used as a refund destination on this path.
	crossChainEvidence, err := h.verifyGuestSettlementWithEvidence(
		c.Request.Context(),
		req.TransactionHash,
		quoteClaims.SettlementAddr,
		lockedUSDMicrounits,
		true,
	)
	if err != nil {
		respondCryptoVerifyError(c, err)
		return
	}
	if !requireTransferAfterQuote(c, quoteClaims, crossChainEvidence) {
		return
	}

	resolveBillCRMCustomer(c, bill)
	var refundDestination *database.PaymentRefundDestination
	if verifiedRefundAddr != "" {
		refundChainID := int(req.RefundBindingChainID)
		if refundChainID == 0 {
			refundChainID = int(quoteClaims.ChainID)
		}
		refundDestination = refundDestinationFromWalletSignature(
			refundChainID,
			lockedUSDMicrounits,
			verifiedRefundAddr,
			refundSigRef,
			req.TransactionHash,
		)
	}

	// Guest-initiated cross-chain payments usually carry no staff context.
	closingStaffID := server.ExtractStaffIDFromContext(c)

	// Capture pre-settle bill: settle helpers nil the bill pointer on error.
	preSettleBill := bill
	applied := false
	if splitShare != nil {
		_, settledBill, wasApplied, settleErr := database.SettleBillSplitShare(database.SettleBillSplitShareInput{
			ShareID:           splitShare.ID,
			GuestSessionID:    splitShare.GuestSessionID,
			IdempotencyKey:    req.TransactionHash,
			Tender:            "cross-chain",
			TxHash:            req.TransactionHash,
			PayerAddr:         "cross_chain_guest",
			TipCents:          tipCents,
			SourceChain:       req.SourceChain,
			SourceToken:       req.SourceToken,
			LifiRouteID:       req.LifiRouteId,
			RefundDestination: refundDestination,
			Now:               time.Now().UTC(),
		})
		bill = settledBill
		applied = wasApplied
		err = settleErr
	} else {
		bill, applied, err = database.ApplyConfirmedPayment(database.ConfirmedPaymentInput{
			BillID:            bill.ID,
			PayerAddr:         "cross_chain_guest",
			Amount:            amountCents,
			TipAmount:         tipCents,
			TxHash:            req.TransactionHash,
			Status:            database.PaymentStatusConfirmed,
			PaymentMethod:     "cross-chain",
			SourceChain:       req.SourceChain,
			SourceToken:       req.SourceToken,
			SettlementChain:   "base",
			LifiRouteID:       req.LifiRouteId,
			RefundDestination: refundDestination,
			PayerGuestSession: guestPayerSession(c),
		}, closingStaffID)
	}
	if err != nil {
		// Verified on-chain but not recorded: this is real money with no DB
		// row. Duplicate-hash conflicts are NOT that case — the payment is
		// already recorded under the same tx.
		if !errors.Is(err, database.ErrPaymentTxHashConflict) && preSettleBill != nil {
			alertSvc := operational_alerts.NewService(database.GetDB())
			if alertErr := alertSvc.CreateCryptoSettlementReviewAlert(c.Request.Context(), *preSettleBill, req.TransactionHash, map[string]any{
				"amount_cents": amountCents,
				"tip_cents":    tipCents,
			}); alertErr != nil {
				log.Printf("failed to create crypto settlement review alert for bill %d: %v", preSettleBill.ID, alertErr)
			}
		}
		status, message := paymentWriteError(err)
		server.RespondWithError(c, status, paymentWriteErrorCode(err), message)
		return
	}

	// Only fire downstream side effects for newly applied payments.
	if applied && bill.Status == database.BillStatusPaid {
		// The in-memory bill is the lean payment-summary projection and is stale
		// after the settlement write. Reload the fresh SCALAR columns only — every
		// downstream consumer here reads scalar bill fields (and re-fetches its own
		// relations), so the heavy GetBillByID aggregate hydration is over-fetch.
		if reloadedBill, _, reloadErr := database.GetBillByIDLean(bill.ID); reloadErr == nil {
			bill = reloadedBill
		} else {
			log.Printf("Failed to reload bill %d after cross-chain payment for CRM attribution: %v", bill.ID, reloadErr)
		}
		recordCRMSettlementVisitForPaidBill(bill, "cross-chain")
		paymentMethod := fmt.Sprintf("%s (%s)", req.SourceToken, req.SourceChain)
		h.sendPaymentCompletionEmails(bill, bill.CRMCustomerID, req.TransactionHash, paymentMethod)
		enqueueFiscalJobForPaidBill(bill, nil, "cross_chain")
		enqueueReceiptForPaidBill(bill)
	}

	h.flushPendingMilestones(bill.BusinessID)

	if applied {
		// Match alt/plugin/webhook: tip_amount + bill.updated via the shared helper.
		publishRecordedPaymentSSE(bill, amountCents, tipCents, "cross-chain")
		publishGuestSplitStateForBill(bill)
		if sharedDeliveryService != nil {
			if err := sharedDeliveryService.HandleDeliveryBillPaid(bill.ID); err != nil {
				log.Printf("delivery payment hook failed for bill %d: %v", bill.ID, err)
			}
		}
		createPaymentReceivedOperationalAlertForTxHash(c.Request.Context(), bill, req.TransactionHash, map[string]any{
			"amount_cents":      amountCents,
			"tip_cents":         tipCents,
			"method":            "cross-chain",
			"source_chain":      req.SourceChain,
			"source_token":      req.SourceToken,
			"payment_status":    string(database.PaymentStatusConfirmed),
			"settlement_source": "guest_cross_chain",
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"message":          map[bool]string{true: "Cross-chain payment processed successfully", false: "Cross-chain payment already processed"}[applied],
		"bill_number":      bill.BillNumber,
		"transaction_hash": req.TransactionHash,
		"amount_paid":      centsToDollars(amountCents),
		"tip_amount":       req.TipAmount,
		"source_chain":     req.SourceChain,
		"source_token":     req.SourceToken,
		"settlement_chain": "base",
		"bill_status":      bill.Status,
		"remaining_amount": centsToDollars(bill.TotalAmount - bill.PaidAmount),
	})
}

func resolveBillCRMCustomer(c *gin.Context, bill *database.Bill) *uint {
	if bill == nil {
		return nil
	}
	if bill.CRMCustomerID != nil {
		return bill.CRMCustomerID
	}
	customer, ok := server.OptionalCustomerFromRequest(c)
	if !ok {
		return nil
	}
	attachedBill, err := database.AttachCRMCustomerIDToBillIfEmpty(bill.ID, customer.ID)
	if err != nil {
		log.Printf("Failed to attach CRM customer %d to bill %d for payment attribution: %v", customer.ID, bill.ID, err)
		return nil
	}
	bill.CRMCustomerID = attachedBill.CRMCustomerID
	return bill.CRMCustomerID
}

func recordCRMSettlementVisitForPaidBill(bill *database.Bill, source string) {
	if bill == nil || bill.Status != database.BillStatusPaid {
		return
	}
	service := crm.NewService(database.GetDB())
	if err := service.RecordBillSettlementVisit(bill.ID); err != nil {
		log.Printf("CRM settlement visit failed for %s bill %d: %v", source, bill.ID, err)
	}
}

func enqueueFiscalJobForPaidBill(bill *database.Bill, paymentID *uint, source string) {
	enqueueFiscalJobForPaidBillWithAlternativePayment(bill, paymentID, nil, source)
}

func enqueueFiscalJobForPaidBillWithAlternativePayment(bill *database.Bill, paymentID *uint, alternativePaymentID *uint, source string) {
	if bill == nil || bill.Status != database.BillStatusPaid {
		return
	}
	db := database.GetDB()
	if db == nil {
		log.Printf("enqueue fiscal job for %s bill %d skipped: DB unavailable", source, bill.ID)
		return
	}
	svc := fiscal.NewService(db, fiscal.NewProviderRegistry())
	if err := svc.HandleBillPaid(context.Background(), fiscal.BillPaidInput{
		BillID:               bill.ID,
		PaymentID:            paymentID,
		AlternativePaymentID: alternativePaymentID,
		Source:               source,
		Actor:                "system",
	}); err != nil {
		log.Printf("enqueue fiscal job for %s bill %d failed: %v", source, bill.ID, err)
		alertSvc := operational_alerts.NewService(database.GetDB())
		if _, alertErr := alertSvc.UpsertAlert(context.Background(), operational_alerts.UpsertAlertInput{
			BusinessID:   bill.BusinessID,
			AlertType:    database.OperationalAlertTypeFiscalIssueFailed,
			ResourceType: database.OperationalAlertResourceTypeBill,
			ResourceID:   bill.ID,
			Priority:     database.OperationalAlertPriorityHigh,
			Title:        "Fiscal receipt was not queued",
			Body: fmt.Sprintf("Bill %s is paid but its fiscal receipt could not be queued: %v. Issue it manually from the bill.",
				bill.BillNumber, err),
			Metadata: map[string]any{"error": err.Error()},
		}); alertErr != nil {
			log.Printf("failed to raise fiscal-enqueue alert for bill %d: %v", bill.ID, alertErr)
		}
	}
}

// sendPaymentCompletionEmails sends payment receipt and thank you emails after successful payment
// Looks up customer email from the server-derived bill CRM customer when CRM is enabled.
func (h *PaymentHandler) sendPaymentCompletionEmails(bill *database.Bill, customerID *uint, transactionHash string, paymentMethod string) {
	// Skip if no customer ID provided
	if customerID == nil {
		return
	}
	if emails.EmailServerInstance == nil {
		return
	}

	// Get business information
	business, err := h.db.GetBusinessByID(bill.BusinessID)
	if err != nil {
		logger.Logger.Warnf("Failed to get business for payment emails: %v", err)
		return
	}

	// Check if CRM is enabled for this business
	if !business.CRMEnabled {
		return
	}

	var connection database.CustomerBusiness
	if err := database.GetDB().
		Where("customer_id = ? AND business_id = ? AND is_active = ?", *customerID, bill.BusinessID, true).
		First(&connection).Error; err != nil {
		logger.Logger.Infof("Skipping payment emails for customer %d on business %d: no active CRM connection", *customerID, bill.BusinessID)
		return
	}

	// Get customer from CRM database
	var customer database.Customer
	if err := database.GetDB().First(&customer, customerID).Error; err != nil {
		logger.Logger.Warnf("Failed to get customer for payment emails: %v", err)
		return
	}

	// Skip if customer has no email
	if customer.Email == "" || !customer.IsActive {
		return
	}

	// Get business language preference (default to English)
	language := "en"
	if business.DefaultLanguage != "" {
		language = business.DefaultLanguage
	}

	// Get bill items for receipt
	items, err := h.db.GetBillItems(bill.ID)
	if err != nil {
		logger.Logger.Warnf("Failed to get bill items for receipt: %v", err)
		items = []database.BillItem{} // Continue with empty items
	}

	// Resolve the business display currency for guest-facing amounts —
	// hardcoded "$" previously mislabeled ARS/AED/JPY receipts.
	currency := strings.TrimSpace(business.DisplayCurrency)
	if currency == "" {
		currency = strings.TrimSpace(business.DefaultCurrency)
	}
	if currency == "" {
		currency = "USD"
	}

	// Build structured items for the receipt template (engine escapes each field).
	receiptItems := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		lineCents := int64(math.Round(item.Price * float64(item.Quantity) * 100))
		receiptItems = append(receiptItems, map[string]interface{}{
			"name":       item.Name,
			"quantity":   item.Quantity,
			"line_total": receiptEmailMoney(lineCents, currency),
		})
	}

	// Send payment receipt email
	if err := emails.EmailServerInstance.ForReceipt(bill.BusinessID, bill.ID).SendPaymentReceiptEmail(
		[]string{customer.Email},
		business.Name,
		receiptEmailDate(time.Now(), language),
		paymentMethod,
		transactionHash,
		receiptItems,
		receiptEmailMoney(bill.TotalAmount, currency),
		language,
	); err != nil {
		// Log error but don't fail the request
		logger.Logger.Warnf("Failed to send payment receipt email: %v", err)
	}

	// Send thank you email
	if err := emails.EmailServerInstance.ForBill(bill.BusinessID, bill.ID).SendThankYouGuestEmail(
		[]string{customer.Email},
		business.Name,
		language,
	); err != nil {
		// Log error but don't fail the request
		logger.Logger.Warnf("Failed to send thank you email: %v", err)
	}

	logger.Logger.Infof("Payment completion emails sent to: %s (Customer ID: %d) for bill #%s", logger.RedactEmail(customer.Email), *customerID, bill.BillNumber)
}

func (h *PaymentHandler) newMilestoneTracker() *services.MilestoneTracker {
	return services.NewMilestoneTracker(h.db)
}

func (h *PaymentHandler) flushPendingMilestones(businessID uint) {
	milestoneTracker := h.newMilestoneTracker()
	if err := milestoneTracker.ProcessPendingMilestones(businessID); err != nil {
		logger.Logger.Warnf("Failed to flush pending milestones: %v", err)
	}
}
