package handlers

// IMP-15 — bill void + refund HTTP handlers.
//
// These thin handlers sit alongside PaymentHandler and reuse its
// authorizeBillManagement helper, which restricts void/refund to the business
// owner or a platform admin — no staff role passes, and the route is gated
// on the owner-only bills:refund permission to match. The
// RequireManagerPIN middleware (IMP-14) writes the comp_void_audit row before
// the handler runs; we patch in the amount/reason/target via
// UpdateCompVoidAudit so the audit log captures the dollar value the action
// affected.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/fiscal"
	"github.com/stdevmac/payverge/backend/internal/plugins"
	"github.com/stdevmac/payverge/backend/internal/server"
)

// VoidBillRequest is the body of POST /inside/bills/:bill_id/void. The reason
// is required (and validated server-side to a minimum of 3 chars) so the
// audit log always carries a human-readable justification.
type VoidBillRequest struct {
	Reason string `json:"reason"`
}

// RefundBillPaymentRequest is the body of POST /inside/bills/:bill_id/refund.
type RefundBillPaymentRequest struct {
	PaymentID            uint   `json:"payment_id"`
	AlternativePaymentID uint   `json:"alternative_payment_id"`
	Reason               string `json:"reason"`
}

const minReversalReasonLength = 3

var (
	errPluginRefundUnavailable = errors.New("plugin refund unavailable")
	errPluginRefundFailed      = errors.New("plugin refund failed")
)

// VoidBill handles POST /inside/bills/:bill_id/void. The route already passed
// through RequireManagerPIN("bill", "void") — we focus on bill resolution +
// access + delegating to database.VoidBill.
func (h *PaymentHandler) VoidBill(c *gin.Context) {
	billID, err := strconv.ParseUint(strings.TrimSpace(c.Param("bill_id")), 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid bill ID")
		return
	}

	var req VoidBillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}

	reason := strings.TrimSpace(req.Reason)
	if len(reason) < minReversalReasonLength {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Reason must be at least 3 characters")
		return
	}

	bill, _, err := database.GetBillByID(uint(billID))
	if err != nil || bill == nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Bill not found")
		return
	}

	actor, authorized := h.authorizeBillManagement(c, bill)
	if !authorized {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Not authorized to modify this bill")
		return
	}

	updated, err := database.VoidBill(uint(billID), actor, reason)
	if err != nil {
		switch {
		case errors.Is(err, database.ErrBillVoidLiveKitchenTickets):
			// Distinct code so the operator UI can localize this instead of
			// leaking the sentinel's English at a Spanish-speaking manager.
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeBillVoidKitchenTicketsLive,
				"The kitchen is still working this check — bump or cancel the open tickets before voiding")
		case errors.Is(err, database.ErrBillCannotBeVoided):
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeInvalidInput, err.Error())
		case strings.Contains(err.Error(), "not found"):
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Bill not found")
		case strings.Contains(err.Error(), "required"):
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		default:
			server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to void bill")
		}
		return
	}

	// Enrich the audit row the middleware seeded with the bill total (in cents,
	// matching AmountCents) + reason. Bill.TotalAmount is int64 cents in-struct;
	// MarshalJSON converts to dollars only at the API boundary.
	server.UpdateCompVoidAudit(c, func(entry *database.CompVoidAudit) {
		amount := updated.TotalAmount
		entry.AmountCents = &amount
		entry.Reason = reason
	})

	// Announce the state change so operator dashboards drop the bill from
	// open/paid views immediately instead of waiting for the next poll.
	publishBillUpdatedSSE(updated)

	c.JSON(http.StatusOK, gin.H{
		"bill": updated,
	})
}

// RefundBillPayment handles POST /inside/bills/:bill_id/refund. Refunds a
// single payment row (one call = one payment row reversed).
//
// Plugin-backed payments call the provider refund hook before the local ledger
// is reversed. Providers without a live refund implementation return a
// conflict/bad-gateway response and leave the payment confirmed, so Payverge
// never creates a false local-only card refund. Cashier/alternative payments
// remain operator-attested local reversals.
func (h *PaymentHandler) RefundBillPayment(c *gin.Context) {
	billID, err := strconv.ParseUint(strings.TrimSpace(c.Param("bill_id")), 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid bill ID")
		return
	}

	var req RefundBillPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}

	if (req.PaymentID == 0 && req.AlternativePaymentID == 0) || (req.PaymentID != 0 && req.AlternativePaymentID != 0) {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "exactly one payment_id or alternative_payment_id is required")
		return
	}

	reason := strings.TrimSpace(req.Reason)
	if len(reason) < minReversalReasonLength {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Reason must be at least 3 characters")
		return
	}

	bill, _, err := database.GetBillByID(uint(billID))
	if err != nil || bill == nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Bill not found")
		return
	}

	actor, authorized := h.authorizeBillManagement(c, bill)
	if !authorized {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Not authorized to modify this bill")
		return
	}

	if req.AlternativePaymentID != 0 {
		updatedBill, refundedPayment, err := database.RefundBillAlternativePayment(uint(billID), req.AlternativePaymentID, actor, reason)
		if err != nil {
			switch {
			case errors.Is(err, database.ErrPaymentNotRefundable):
				server.RespondWithError(c, http.StatusConflict, server.ErrCodeInvalidInput, "Payment cannot be refunded")
			case strings.Contains(err.Error(), "not found"):
				server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, err.Error())
			case strings.Contains(err.Error(), "does not belong"):
				server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
			case strings.Contains(err.Error(), "required"):
				server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
			default:
				server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to refund payment")
			}
			return
		}

		server.UpdateCompVoidAudit(c, func(entry *database.CompVoidAudit) {
			amount := refundedPayment.Amount
			entry.AmountCents = &amount
			entry.Reason = reason
			entry.TargetID = strconv.FormatUint(uint64(refundedPayment.ID), 10)
			entry.TargetType = "alternative_payment"
		})

		// Best-effort: emit an AFIP credit note for the refunded FISCAL amount (the
		// bill portion, net of tip). Per-payment discriminator so each partial
		// refund gets its own nota de crédito. Never fails the refund.
		altFiscalCents := refundedPayment.BillAmountCents
		if altFiscalCents <= 0 {
			altFiscalCents = refundedPayment.Amount - refundedPayment.TipAmountCents
		}
		enqueueFiscalCreditNoteForRefund(updatedBill, altFiscalCents, fmt.Sprintf("altpay:%d", refundedPayment.ID), actor)

		// Announce the ledger reversal so dashboards refresh the bill.
		publishBillUpdatedSSE(updatedBill)

		c.JSON(http.StatusOK, gin.H{
			"bill":                updatedBill,
			"alternative_payment": refundedPayment,
		})
		return
	}

	updatedBill, refundedPayment, err := h.refundBillPaymentWithExternalTender(bill, req.PaymentID, actor, reason)
	if err != nil {
		switch {
		case errors.Is(err, errPluginRefundUnavailable):
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeInvalidInput, "Payment provider refund is unavailable; local ledger was not changed")
		case errors.Is(err, errPluginRefundFailed):
			server.RespondWithError(c, http.StatusBadGateway, "", "Payment provider refund failed; local ledger was not changed")
		case errors.Is(err, database.ErrPaymentNotRefundable):
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeInvalidInput, "Payment cannot be refunded")
		case strings.Contains(err.Error(), "not found"):
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, err.Error())
		case strings.Contains(err.Error(), "does not belong"):
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		case strings.Contains(err.Error(), "required"):
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		default:
			server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to refund payment")
		}
		return
	}

	// Decorate the comp_void_audit row with the payment ID + refunded amount (in
	// cents, matching AmountCents) so the audit feed can show "Refunded $32.50
	// (payment #7)". Payment.Amount/TipAmount are int64 cents in-struct; the FE
	// divides AmountCents by 100 for display.
	server.UpdateCompVoidAudit(c, func(entry *database.CompVoidAudit) {
		amount := refundedPayment.Amount + refundedPayment.TipAmount
		entry.AmountCents = &amount
		entry.Reason = reason
		entry.TargetID = strconv.FormatUint(uint64(refundedPayment.ID), 10)
		entry.TargetType = "payment"
	})

	// Best-effort: emit an AFIP credit note for the refunded FISCAL amount. The
	// bill portion is Payment.Amount (TipAmount is separate and never entered the
	// factura). Per-payment discriminator so each partial refund gets its own nota
	// de crédito. Never fails the refund.
	enqueueFiscalCreditNoteForRefund(updatedBill, refundedPayment.Amount, fmt.Sprintf("payment:%d", refundedPayment.ID), actor)

	// Announce the ledger reversal so dashboards refresh the bill.
	publishBillUpdatedSSE(updatedBill)

	c.JSON(http.StatusOK, gin.H{
		"bill":    updatedBill,
		"payment": refundedPayment,
	})
}

// EnqueueFiscalCreditNoteForRefund is the exported seam for noncustodial crypto
// refund confirmation (Wave 4 Task 13). Callers outside this package (the
// cryptorefund worker) must use this entry point so credit notes stay
// single-sourced with IMP-15 refunds. Does not modify fiscal internals.
func EnqueueFiscalCreditNoteForRefund(bill *database.Bill, amountCents int64, discriminator, actor string) {
	enqueueFiscalCreditNoteForRefund(bill, amountCents, discriminator, actor)
}

// enqueueFiscalCreditNoteForRefund best-effort enqueues an AFIP nota de crédito
// for the fiscal amount just refunded on a bill. It finds the bill's authorized
// issue receipt (newest) and, if one exists, enqueues an idempotent credit_note
// job via the fiscal service. It mirrors enqueueFiscalJobForPaidBill's posture:
// construct the service, log on error, and NEVER fail the caller — a refund must
// never be rejected because the credit-note enqueue failed. If no authorized
// issue receipt exists (fiscal off, never issued, or not yet authorized), it
// silently does nothing.
//
// amountCents is the FISCAL amount refunded (net of tip — tips never entered the
// factura), in int64 cents. discriminator scopes the idempotency key to the
// specific refund (e.g. "payment:7") so partial refunds each get their own nota
// de crédito and a retried refund of the same payment is a no-op. Both full and
// partial refunds now emit a credit note for exactly the refunded amount; the
// partial credit-note amount is honored by the worker + AR mapper.
func enqueueFiscalCreditNoteForRefund(bill *database.Bill, amountCents int64, discriminator, actor string) {
	if bill == nil {
		return
	}
	if amountCents <= 0 {
		log.Printf("fiscal credit note for bill %d skipped: non-positive refunded fiscal amount (%d)", bill.ID, amountCents)
		return
	}
	db := database.GetDB()
	if db == nil {
		log.Printf("enqueue fiscal credit note for refunded bill %d skipped: DB unavailable", bill.ID)
		return
	}

	var receipt database.FiscalReceipt
	err := db.
		Select("id").
		Where("bill_id = ? AND business_id = ? AND action = ? AND status = ?",
			bill.ID, bill.BusinessID, fiscal.ActionIssueReceipt, database.FiscalStatusAuthorized).
		Order("created_at DESC, id DESC").
		First(&receipt).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("enqueue fiscal credit note for refunded bill %d failed to load issue receipt: %v", bill.ID, err)
			return
		}
		// Wave 4: no AUTHORIZED issue receipt yet (refund raced issuance, or
		// the issue job is still retrying). Instead of silently dropping the
		// nota de crédito, enqueue a DEFERRED credit-note job that the fiscal
		// worker completes once the issue receipt authorizes. Skip only when
		// fiscal was never enabled — IssueCreditNoteDeferred no-ops on nil
		// active settings.
		svc := fiscal.NewService(db, fiscal.NewProviderRegistry())
		if deferErr := svc.IssueCreditNoteDeferred(context.Background(), bill.BusinessID, bill.ID, amountCents, discriminator, actor); deferErr != nil {
			log.Printf("enqueue DEFERRED fiscal credit note for refunded bill %d failed: %v", bill.ID, deferErr)
		}
		return
	}

	svc := fiscal.NewService(db, fiscal.NewProviderRegistry())
	if err := svc.IssueCreditNote(context.Background(), bill.BusinessID, receipt.ID, amountCents, discriminator, "refund", actor); err != nil {
		if errors.Is(err, fiscal.ErrCreditNoteExceedsReceipt) {
			log.Printf("fiscal credit note for refunded bill %d (receipt %d) REJECTED: would over-credit the original receipt total — needs manual reconciliation", bill.ID, receipt.ID)
			return
		}
		log.Printf("enqueue fiscal credit note for refunded bill %d (receipt %d) failed: %v", bill.ID, receipt.ID, err)
	}
}

func (h *PaymentHandler) refundBillPaymentWithExternalTender(bill *database.Bill, paymentID uint, actor, reason string) (*database.Bill, *database.Payment, error) {
	if bill == nil {
		return nil, nil, fmt.Errorf("bill not found")
	}

	var payment database.Payment
	if err := database.GetDB().
		Select("id", "bill_id", "amount", "tip_amount", "tx_hash", "status", "payment_method").
		First(&payment, paymentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, fmt.Errorf("payment not found")
		}
		return nil, nil, fmt.Errorf("failed to load payment: %w", err)
	}

	if payment.BillID != bill.ID {
		return nil, nil, fmt.Errorf("payment does not belong to this bill")
	}

	if paymentRequiresOriginalTenderRefund(payment) && !strings.HasPrefix(strings.TrimSpace(payment.TxHash), "plugin_") {
		return nil, nil, errPluginRefundUnavailable
	}

	if strings.HasPrefix(strings.TrimSpace(payment.TxHash), "plugin_") {
		pendingPayment, err := database.MarkPaymentRefundPending(bill.ID, payment.ID)
		if err != nil {
			return nil, nil, err
		}
		if err := refundPluginTenderIfNeeded(bill.BusinessID, pendingPayment); err != nil {
			if restoreErr := database.RestorePaymentRefundPending(payment.ID); restoreErr != nil {
				return nil, nil, fmt.Errorf("%w: %v; additionally failed to restore payment refund state: %v", errPluginRefundFailed, err, restoreErr)
			}
			return nil, nil, err
		}
	}

	return database.RefundBillPayment(bill.ID, payment.ID, actor, reason)
}

func paymentRequiresOriginalTenderRefund(payment database.Payment) bool {
	switch strings.ToLower(strings.TrimSpace(payment.PaymentMethod)) {
	case "crypto", "cross-chain", "cross_chain":
		return true
	default:
		return false
	}
}

func refundPluginTenderIfNeeded(businessID uint, payment *database.Payment) error {
	if payment == nil || (payment.Status != database.PaymentStatusConfirmed && payment.Status != database.PaymentStatusRefundPending) {
		return nil
	}

	providerPaymentID, found := strings.CutPrefix(strings.TrimSpace(payment.TxHash), "plugin_")
	if !found || strings.TrimSpace(providerPaymentID) == "" {
		return nil
	}
	providerPaymentID = strings.TrimSpace(providerPaymentID)

	var tracker database.AlternativePayment
	if err := database.GetDB().
		Select("payment_method").
		Where("bill_id = ? AND participant_addr = ?", payment.BillID, providerPaymentID).
		First(&tracker).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("%w: missing provider tracker for payment %d", errPluginRefundUnavailable, payment.ID)
		}
		return fmt.Errorf("failed to load provider tracker for refund: %w", err)
	}

	pluginName := strings.TrimSpace(string(tracker.PaymentMethod))
	if pluginName == "" {
		return fmt.Errorf("%w: missing provider name for payment %d", errPluginRefundUnavailable, payment.ID)
	}

	plugin, exists := plugins.GetPluginByName(pluginName)
	if !exists {
		return fmt.Errorf("%w: provider %s is not registered", errPluginRefundUnavailable, pluginName)
	}

	paymentPlugin, ok := plugin.(plugins.PaymentPlugin)
	if !ok {
		return fmt.Errorf("%w: provider %s is not a payment plugin", errPluginRefundUnavailable, pluginName)
	}

	refundAmount := payment.Amount + payment.TipAmount
	if err := paymentPlugin.RefundPayment(businessID, providerPaymentID, refundAmount); err != nil {
		lower := strings.ToLower(err.Error())
		if strings.Contains(lower, "unsupported") {
			return fmt.Errorf("%w: %v", errPluginRefundUnavailable, err)
		}
		return fmt.Errorf("%w: %v", errPluginRefundFailed, err)
	}

	return nil
}

// GetBillAuditLog handles GET /inside/bills/:bill_id/audit. Returns recent
// comp_void_audit rows that target the bill, its items, or its payments —
// powering the "Voids & refunds" section of the bill-details modal.
func (h *PaymentHandler) GetBillAuditLog(c *gin.Context) {
	billID, err := strconv.ParseUint(strings.TrimSpace(c.Param("bill_id")), 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid bill ID")
		return
	}

	bill, _, err := database.GetBillByID(uint(billID))
	if err != nil || bill == nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Bill not found")
		return
	}

	if _, authorized := h.authorizeBillManagement(c, bill); !authorized {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Not authorized to view this bill")
		return
	}

	rows, err := database.ListCompVoidAuditForBill(uint(billID), 50)
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "", "Failed to load audit log")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"entries": rows,
	})
}
