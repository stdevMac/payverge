package handlers

// Wave 4 Tasks 12–14: operator crypto refund HTTP surface.
//
// Noncustodial: the backend builds a canonical UNSIGNED USDC transfer request
// and verifies submitted tx hashes. It NEVER holds a treasury private key.
// Mainnet submission is gated by CRYPTO_REFUND_MAINNET_ENABLED (default OFF).

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/txhash"

	"github.com/gin-gonic/gin"
)

// CryptoRefundMainnetEnabled reports whether mainnet (Base 8453 / ETH 1)
// unsigned-request handoff is enabled. Default false — operators use manual
// tx-hash submission / testnet. Env: CRYPTO_REFUND_MAINNET_ENABLED=true.
func CryptoRefundMainnetEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("CRYPTO_REFUND_MAINNET_ENABLED")))
	return v == "1" || v == "true" || v == "yes"
}

// CryptoRefundOperatorView is the operator-safe JSON projection. Status is
// always the raw state machine value — never rewritten as "refunded" unless
// status == confirmed. Destination is MASKED.
type CryptoRefundOperatorView struct {
	ID                uint   `json:"id"`
	BusinessID        uint   `json:"business_id"`
	BillID            uint   `json:"bill_id"`
	PaymentID         uint   `json:"payment_id"`
	ChainID           int    `json:"chain_id"`
	Token             string `json:"token"`
	AmountBaseUnits   int64  `json:"amount_base_units"`
	MaskedRecipient   string `json:"masked_recipient"`
	RecipientOverride bool   `json:"recipient_override"`
	Reason            string `json:"reason"`
	Status            string `json:"status"`
	// HonestLifecycleLabel is operator copy; only "confirmed" is "Refunded".
	HonestLifecycleLabel string  `json:"honest_lifecycle_label"`
	IdempotencyKey       string  `json:"idempotency_key"`
	SubmittedTxHash      *string `json:"submitted_tx_hash,omitempty"`
	Confirmations        int     `json:"confirmations"`
	LastError            *string `json:"last_error,omitempty"`
	MainnetSubmissionOff bool    `json:"mainnet_submission_off"`
	ManualTxHashRequired bool    `json:"manual_tx_hash_required"`
	ExplorerURL          string  `json:"explorer_url,omitempty"`
	RequestedBy          string  `json:"requested_by"`
	ApprovedBy           *string `json:"approved_by,omitempty"`
	RequestedAt          string  `json:"requested_at"`
	ConfirmedAt          *string `json:"confirmed_at,omitempty"`
	CreatedAt            string  `json:"created_at"`
	UpdatedAt            string  `json:"updated_at"`
}

// RefundDestinationOperatorView is a masked projection for operator UI.
// has_evidence=false means refund requires manual support.
type RefundDestinationOperatorView struct {
	PaymentID             uint   `json:"payment_id"`
	HasEvidence           bool   `json:"has_evidence"`
	ManualSupportRequired bool   `json:"manual_support_required"`
	ChainID               int    `json:"chain_id,omitempty"`
	Token                 string `json:"token,omitempty"`
	AmountBaseUnits       int64  `json:"amount_base_units,omitempty"`
	MaskedAddress         string `json:"masked_address,omitempty"`
	EvidenceType          string `json:"evidence_type,omitempty"`
	RefundableBaseUnits   int64  `json:"refundable_base_units,omitempty"`
}

func honestCryptoRefundLabel(status database.PaymentRefundStatus) string {
	switch status {
	case database.PaymentRefundStatusRequested:
		return "Refund requested — awaiting approval"
	case database.PaymentRefundStatusApproved:
		return "Approved — prepare signature"
	case database.PaymentRefundStatusAwaitingSignature:
		return "Awaiting owner signature / tx submission"
	case database.PaymentRefundStatusSubmitted:
		return "Transaction submitted — not yet refunded"
	case database.PaymentRefundStatusConfirming:
		return "Confirming on-chain — not yet refunded"
	case database.PaymentRefundStatusConfirmed:
		return "Refunded (on-chain confirmed)"
	case database.PaymentRefundStatusFailed:
		return "On-chain verification failed — ledger unchanged"
	case database.PaymentRefundStatusRejected:
		return "Rejected — ledger unchanged"
	case database.PaymentRefundStatusCancelled:
		return "Cancelled — ledger unchanged"
	default:
		return string(status)
	}
}

func explorerURLFor(chainID int, txHash string) string {
	txHash = strings.TrimSpace(txHash)
	if txHash == "" {
		return ""
	}
	switch chainID {
	case 8453:
		return "https://basescan.org/tx/" + txHash
	case 84532:
		return "https://sepolia.basescan.org/tx/" + txHash
	case 1:
		return "https://etherscan.io/tx/" + txHash
	case 11155111:
		return "https://sepolia.etherscan.io/tx/" + txHash
	default:
		return ""
	}
}

func toCryptoRefundView(row *database.PaymentRefund) CryptoRefundOperatorView {
	if row == nil {
		return CryptoRefundOperatorView{}
	}
	mainnetOff := database.IsMainnetCryptoRefundChain(row.ChainID) && !CryptoRefundMainnetEnabled()
	v := CryptoRefundOperatorView{
		ID:                   row.ID,
		BusinessID:           row.BusinessID,
		BillID:               row.BillID,
		PaymentID:            row.PaymentID,
		ChainID:              row.ChainID,
		Token:                row.Token,
		AmountBaseUnits:      row.AmountBaseUnits,
		MaskedRecipient:      database.MaskRefundAddress(row.VerifiedRecipient),
		RecipientOverride:    row.RecipientOverride,
		Reason:               row.Reason,
		Status:               string(row.Status),
		HonestLifecycleLabel: honestCryptoRefundLabel(row.Status),
		IdempotencyKey:       row.IdempotencyKey,
		SubmittedTxHash:      row.SubmittedTxHash,
		Confirmations:        row.Confirmations,
		LastError:            row.LastError,
		MainnetSubmissionOff: mainnetOff,
		// Manual path whenever mainnet is off OR status awaits external submit.
		ManualTxHashRequired: mainnetOff || row.Status == database.PaymentRefundStatusAwaitingSignature || row.Status == database.PaymentRefundStatusApproved,
		RequestedBy:          row.RequestedBy,
		ApprovedBy:           row.ApprovedBy,
		RequestedAt:          row.RequestedAt.UTC().Format(time.RFC3339),
		CreatedAt:            row.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:            row.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if row.SubmittedTxHash != nil {
		v.ExplorerURL = explorerURLFor(row.ChainID, *row.SubmittedTxHash)
	}
	if row.ConfirmedAt != nil {
		s := row.ConfirmedAt.UTC().Format(time.RFC3339)
		v.ConfirmedAt = &s
	}
	return v
}

// UnsignedRefundTransferRequest is the canonical unsigned Base-USDC transfer
// the owner's connected settlement wallet / Safe must sign+submit. Backend
// never signs this.
type UnsignedRefundTransferRequest struct {
	ChainID         int    `json:"chain_id"`
	Token           string `json:"token"`
	TokenAddress    string `json:"token_address"`
	From            string `json:"from"` // Business.SettlementAddr
	To              string `json:"to"`   // verified recipient
	AmountBaseUnits int64  `json:"amount_base_units"`
	RefundID        uint   `json:"refund_id"`
	// Warning when mainnet signing handoff is disabled.
	MainnetDisabled bool   `json:"mainnet_disabled"`
	Message         string `json:"message"`
}

func usdcTokenAddressForChain(chainID int) string {
	switch chainID {
	case 8453:
		return "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"
	case 84532:
		return "0x82d491aB292C06Aa7148234b910cdea5FE788223"
	case 1:
		return "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"
	case 11155111:
		return "0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238"
	default:
		return ""
	}
}

// RequestCryptoRefund handles POST /businesses/:id/crypto-refunds
func (h *PaymentHandler) RequestCryptoRefund(c *gin.Context) {
	businessID, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	var req struct {
		BillID            uint    `json:"bill_id" binding:"required"`
		PaymentID         uint    `json:"payment_id" binding:"required"`
		AmountBaseUnits   int64   `json:"amount_base_units" binding:"required"`
		Reason            string  `json:"reason" binding:"required"`
		IdempotencyKey    string  `json:"idempotency_key" binding:"required"`
		RecipientOverride *string `json:"recipient_override"`
		OverrideReason    string  `json:"override_reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}

	actor, ok := h.cryptoRefundActor(c)
	if !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Not authorized")
		return
	}

	row, err := database.RequestCryptoRefund(database.CreateCryptoRefundInput{
		BusinessID:        uint(businessID),
		BillID:            req.BillID,
		PaymentID:         req.PaymentID,
		AmountBaseUnits:   req.AmountBaseUnits,
		Reason:            req.Reason,
		RequestedBy:       actor,
		IdempotencyKey:    req.IdempotencyKey,
		RecipientOverride: req.RecipientOverride,
		OverrideReason:    req.OverrideReason,
	})
	if err != nil {
		respondCryptoRefundError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"refund": toCryptoRefundView(row)})
}

// ApproveCryptoRefund handles POST /businesses/:id/crypto-refunds/:refund_id/approve
func (h *PaymentHandler) ApproveCryptoRefund(c *gin.Context) {
	businessID, refundID, ok := parseBusinessAndRefundID(c)
	if !ok {
		return
	}
	actor, ok := h.cryptoRefundActor(c)
	if !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Not authorized")
		return
	}
	row, err := database.ApproveCryptoRefund(refundID, businessID, actor)
	if err != nil {
		respondCryptoRefundError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"refund": toCryptoRefundView(row)})
}

// RejectCryptoRefund handles POST /businesses/:id/crypto-refunds/:refund_id/reject
func (h *PaymentHandler) RejectCryptoRefund(c *gin.Context) {
	businessID, refundID, ok := parseBusinessAndRefundID(c)
	if !ok {
		return
	}
	actor, ok := h.cryptoRefundActor(c)
	if !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Not authorized")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)
	row, err := database.RejectCryptoRefund(refundID, businessID, actor, req.Reason)
	if err != nil {
		respondCryptoRefundError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"refund": toCryptoRefundView(row)})
}

// ListCryptoRefunds handles GET /businesses/:id/crypto-refunds
func (h *PaymentHandler) ListCryptoRefunds(c *gin.Context) {
	businessID, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	// Optional payment_id filter: when present, return EVERY refund for that one
	// payment (tenant-scoped) instead of the business-wide newest-100 window —
	// the operator refund panel only cares about one payment, and the 100-cap
	// could hide older refunds for it (audit LOW).
	var rows []database.PaymentRefund
	if raw := strings.TrimSpace(c.Query("payment_id")); raw != "" {
		paymentID, perr := strconv.ParseUint(raw, 10, 32)
		if perr != nil {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid payment ID")
			return
		}
		rows, err = database.ListCryptoRefundsForBusinessPayment(uint(businessID), uint(paymentID))
	} else {
		rows, err = database.ListCryptoRefundsForBusiness(uint(businessID), 100)
	}
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to list crypto refunds")
		return
	}
	out := make([]CryptoRefundOperatorView, 0, len(rows))
	for i := range rows {
		out = append(out, toCryptoRefundView(&rows[i]))
	}
	c.JSON(http.StatusOK, gin.H{"refunds": out})
}

// GetPaymentRefundDestination handles GET /businesses/:id/payments/:payment_id/refund-destination
// Returns a MASKED operator projection. Never the full address on public routes.
func (h *PaymentHandler) GetPaymentRefundDestination(c *gin.Context) {
	businessID, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}
	paymentID, err := strconv.ParseUint(strings.TrimSpace(c.Param("payment_id")), 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid payment ID")
		return
	}

	// Tenant check: payment must belong to a bill of this business.
	var payment database.Payment
	if err := database.GetDB().First(&payment, uint(paymentID)).Error; err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Payment not found")
		return
	}
	bill, _, err := database.GetBillByIDLean(payment.BillID)
	if err != nil || bill == nil || bill.BusinessID != uint(businessID) { //nolint:staticcheck // three-return API
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Payment not found")
		return
	}

	dest, err := database.GetPaymentRefundDestination(uint(paymentID))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"destination": RefundDestinationOperatorView{
				PaymentID:             uint(paymentID),
				HasEvidence:           false,
				ManualSupportRequired: true,
			},
		})
		return
	}
	remaining, _ := database.RefundableCryptoBalanceBaseUnits(uint(paymentID))
	c.JSON(http.StatusOK, gin.H{
		"destination": RefundDestinationOperatorView{
			PaymentID:             uint(paymentID),
			HasEvidence:           true,
			ManualSupportRequired: false,
			ChainID:               dest.ChainID,
			Token:                 dest.Token,
			AmountBaseUnits:       dest.AmountBaseUnits,
			MaskedAddress:         database.MaskRefundAddress(dest.RefundAddress),
			EvidenceType:          string(dest.EvidenceType),
			RefundableBaseUnits:   remaining,
		},
	})
}

// GetCryptoRefundUnsignedRequest handles GET .../crypto-refunds/:refund_id/unsigned-request
func (h *PaymentHandler) GetCryptoRefundUnsignedRequest(c *gin.Context) {
	businessID, refundID, ok := parseBusinessAndRefundID(c)
	if !ok {
		return
	}
	row, err := database.GetCryptoRefund(refundID, businessID)
	if err != nil {
		respondCryptoRefundError(c, err)
		return
	}
	if row.Status != database.PaymentRefundStatusAwaitingSignature &&
		row.Status != database.PaymentRefundStatusApproved &&
		row.Status != database.PaymentRefundStatusFailed {
		server.RespondWithError(c, http.StatusConflict, server.ErrCodeInvalidInput, "Refund is not awaiting signature")
		return
	}

	// Sign from the wallet that received this payment (stored at payment
	// time), not the live business wallet: after a rotation the refund must
	// still leave the old wallet, which is what the worker verifies.
	settlement, err := database.CryptoRefundSourceWallet(nil, row.PaymentID)
	if err != nil {
		if errors.Is(err, database.ErrCryptoRefundSourceWalletUnknown) {
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeInvalidInput, "The wallet that received this payment is unknown")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to load the payment wallet")
		return
	}

	mainnetOff := database.IsMainnetCryptoRefundChain(row.ChainID) && !CryptoRefundMainnetEnabled()
	msg := "Sign and submit this ERC-20 transfer from the business settlement wallet (or Safe). Backend does not custody keys."
	if mainnetOff {
		msg = "Mainnet crypto refund signing handoff is OFF (CRYPTO_REFUND_MAINNET_ENABLED=false). Submit a tx hash from external treasury tooling after sending USDC manually on the configured network. Instant refunds are not promised."
	}

	c.JSON(http.StatusOK, gin.H{
		"unsigned_request": UnsignedRefundTransferRequest{
			ChainID:         row.ChainID,
			Token:           row.Token,
			TokenAddress:    usdcTokenAddressForChain(row.ChainID),
			From:            settlement,
			To:              row.VerifiedRecipient,
			AmountBaseUnits: row.AmountBaseUnits,
			RefundID:        row.ID,
			MainnetDisabled: mainnetOff,
			Message:         msg,
		},
	})
}

// SubmitCryptoRefundTx handles POST .../crypto-refunds/:refund_id/submit-tx
func (h *PaymentHandler) SubmitCryptoRefundTx(c *gin.Context) {
	businessID, refundID, ok := parseBusinessAndRefundID(c)
	if !ok {
		return
	}
	var req struct {
		TxHash string `json:"tx_hash" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	if _, ok := h.cryptoRefundActor(c); !ok {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Not authorized")
		return
	}
	canonicalTxHash, valid := txhash.Canonical(req.TxHash)
	if !valid {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "tx_hash must be 0x followed by 64 hex characters")
		return
	}
	row, err := database.SubmitCryptoRefundTxHash(refundID, businessID, canonicalTxHash)
	if err != nil {
		respondCryptoRefundError(c, err)
		return
	}
	// Notify operator dashboards — status is submitted, NOT refunded.
	events.GetHub().PublishJSON(businessID, "crypto_refund.submitted", gin.H{
		"refund_id": row.ID,
		"bill_id":   row.BillID,
		"status":    string(row.Status),
		"tx_hash":   canonicalTxHash,
	})
	c.JSON(http.StatusOK, gin.H{"refund": toCryptoRefundView(row)})
}

func parseBusinessAndRefundID(c *gin.Context) (businessID, refundID uint, ok bool) {
	bid, err := strconv.ParseUint(strings.TrimSpace(c.Param("id")), 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return 0, 0, false
	}
	rid, err := strconv.ParseUint(strings.TrimSpace(c.Param("refund_id")), 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid refund ID")
		return 0, 0, false
	}
	return uint(bid), uint(rid), true
}

func (h *PaymentHandler) cryptoRefundActor(c *gin.Context) (string, bool) {
	// Prefer staff identity; fall back to wallet address / user id.
	if staffID := server.ExtractStaffIDFromContext(c); staffID != nil {
		return fmt.Sprintf("staff:%d", *staffID), true
	}
	if addr, exists := c.Get("address"); exists {
		if s, ok := addr.(string); ok && strings.TrimSpace(s) != "" {
			return "wallet:" + strings.TrimSpace(s), true
		}
	}
	if uid, exists := c.Get("user_id"); exists {
		return fmt.Sprintf("user:%v", uid), true
	}
	return "", false
}

func respondCryptoRefundError(c *gin.Context, err error) {
	switch {
	case err == nil:
		return
	case errors.Is(err, database.ErrCryptoRefundNotFound):
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, err.Error())
	case errors.Is(err, database.ErrCryptoRefundDestinationMissing):
		server.RespondWithError(c, http.StatusConflict, server.ErrCodeInvalidInput, "Verified refund destination is unavailable; refund requires manual support")
	case errors.Is(err, database.ErrCryptoRefundActiveExists):
		server.RespondWithError(c, http.StatusConflict, server.ErrCodeInvalidInput, err.Error())
	case errors.Is(err, database.ErrCryptoRefundAmountExceedsBalance):
		server.RespondWithError(c, http.StatusConflict, server.ErrCodeInvalidInput, err.Error())
	case errors.Is(err, database.ErrCryptoRefundNotRefundable):
		server.RespondWithError(c, http.StatusConflict, server.ErrCodeInvalidInput, err.Error())
	case errors.Is(err, database.ErrCryptoRefundInvalidTransition):
		server.RespondWithError(c, http.StatusConflict, server.ErrCodeInvalidInput, err.Error())
	case errors.Is(err, database.ErrCryptoRefundTxHashReused):
		server.RespondWithError(c, http.StatusConflict, server.ErrCodeInvalidInput, err.Error())
	case errors.Is(err, database.ErrCryptoRefundOverrideRequiresReason):
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
	case errors.Is(err, database.ErrCryptoRefundChainNotAllowlisted):
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
	case errors.Is(err, database.ErrCryptoRefundIdempotencyConflict):
		server.RespondWithError(c, http.StatusConflict, server.ErrCodeInvalidInput, err.Error())
	case strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "malformed") || strings.Contains(err.Error(), "min 3"):
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
	case strings.Contains(err.Error(), "second distinct approver"):
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, err.Error())
	default:
		server.RespondWithError(c, http.StatusInternalServerError, "", "Crypto refund operation failed")
	}
}
