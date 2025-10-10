package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"payverge/internal/blockchain"
	"payverge/internal/database"

	"github.com/gin-gonic/gin"
)

// PaymentHandler handles payment-related requests
type PaymentHandler struct {
	db         *database.DB
	blockchain *blockchain.BlockchainService
}

// NewPaymentHandler creates a new payment handler
func NewPaymentHandler(db *database.DB, blockchain *blockchain.BlockchainService) *PaymentHandler {
	return &PaymentHandler{
		db:         db,
		blockchain: blockchain,
	}
}

// CreateBillPayment processes a payment for a bill
// POST /api/v1/bills/:id/payments
func (h *PaymentHandler) CreateBillPayment(c *gin.Context) {
	billIDStr := c.Param("id")
	billID, err := strconv.ParseUint(billIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	var req blockchain.PaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get bill from database
	bill, err := h.db.GetBill(uint(billID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Set bill ID in request
	req.BillID = strconv.FormatUint(uint64(bill.ID), 10)

	// Process payment through blockchain service
	result, err := h.blockchain.ProcessPayment(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Update bill payment status in database
	bill.PaidAmount += float64(req.Amount)
	bill.TipAmount += float64(req.TipAmount)

	// Update bill status based on payment
	if bill.PaidAmount >= bill.TotalAmount {
		bill.Status = database.BillStatusPaid
	} else if bill.PaidAmount > 0 {
		bill.Status = "partial"
	}

	err = h.db.UpdateBill(bill)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update bill"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"payment_result": result,
		"bill":           bill,
	})
}

// GetBillPayments retrieves payment history for a bill
// GET /api/v1/bills/:id/payments
func (h *PaymentHandler) GetBillPayments(c *gin.Context) {
	billIDStr := c.Param("id")
	billID, err := strconv.ParseUint(billIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	// Get bill from database
	bill, err := h.db.GetBill(uint(billID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Get payments from blockchain
	payments, err := h.blockchain.GetBillPayments(c.Request.Context(), strconv.FormatUint(uint64(bill.ID), 10))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bill":     bill,
		"payments": payments,
	})
}

// GetPaymentDetails retrieves details for a specific payment transaction
// GET /api/v1/payments/:txHash
func (h *PaymentHandler) GetPaymentDetails(c *gin.Context) {
	txHash := c.Param("txHash")
	if txHash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Transaction hash required"})
		return
	}

	// In a real implementation, you would query the blockchain for transaction details
	// For now, return a placeholder response
	c.JSON(http.StatusOK, gin.H{
		"transaction_hash": txHash,
		"status":           "confirmed",
		"block_number":     12345,
		"gas_used":         21000,
		"timestamp":        "2024-01-01T00:00:00Z",
	})
}

// GetBillTotalPaid gets the total amount paid for a bill from blockchain
// GET /api/v1/bills/:id/total-paid
func (h *PaymentHandler) GetBillTotalPaid(c *gin.Context) {
	billIDStr := c.Param("id")
	billID, err := strconv.ParseUint(billIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	// Get bill from database
	bill, err := h.db.GetBill(uint(billID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Get total paid from blockchain
	totalPaid, err := h.blockchain.GetBillTotalPaid(strconv.FormatUint(uint64(bill.ID), 10))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"bill_id":    bill.ID,
		"total_paid": totalPaid,
		"bill_total": bill.TotalAmount,
		"remaining":  bill.TotalAmount - float64(totalPaid),
	})
}

// WebhookPaymentConfirmation handles payment confirmation webhooks
// POST /api/v1/payments/webhook
func (h *PaymentHandler) WebhookPaymentConfirmation(c *gin.Context) {
	var webhook struct {
		TransactionHash string `json:"transaction_hash"`
		BillID          string `json:"bill_id"`
		Amount          int64  `json:"amount"`
		TipAmount       int64  `json:"tip_amount"`
		Status          string `json:"status"`
	}

	if err := c.ShouldBindJSON(&webhook); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Parse bill ID
	billID, err := strconv.ParseUint(webhook.BillID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	// Get bill from database
	bill, err := h.db.GetBill(uint(billID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Update bill with confirmed payment
	if webhook.Status == "confirmed" {
		bill.PaidAmount += float64(webhook.Amount)
		bill.TipAmount += float64(webhook.TipAmount)

		// Update bill status
		if bill.PaidAmount >= bill.TotalAmount {
			bill.Status = database.BillStatusPaid
		} else if bill.PaidAmount > 0 {
			bill.Status = "partial"
		}

		err = h.db.UpdateBill(bill)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update bill"})
			return
		}

		// TODO: Send WebSocket notification to business dashboard
		// TODO: Send WebSocket notification to guest bill view
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "processed",
		"bill":   bill,
	})
}

// MarkAlternativePayment marks an alternative payment as received (business owner only)
// POST /api/v1/inside/bills/:bill_id/alternative-payment
func (h *PaymentHandler) MarkAlternativePayment(c *gin.Context) {
	billIDStr := c.Param("bill_id")
	billID, err := strconv.ParseUint(billIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	var req struct {
		ParticipantAddress   string `json:"participant_address" binding:"required"`
		Amount               string `json:"amount" binding:"required"`
		PaymentMethod        string `json:"payment_method" binding:"required"`
		BusinessConfirmation bool   `json:"business_confirmation"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get bill from database
	bill, err := h.db.GetBill(uint(billID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Convert amount to float64 (assuming it's in USDC with 6 decimals)
	amountFloat, err := strconv.ParseFloat(req.Amount, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid amount format"})
		return
	}
	// Convert from micro USDC to USDC
	amountFloat = amountFloat / 1_000_000

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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payment method"})
		return
	}

	// Get user address from context (set by auth middleware)
	userAddress, exists := c.Get("user_address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Create alternative payment record
	now := time.Now()
	altPayment := &database.AlternativePayment{
		BillID:          uint(billID),
		ParticipantAddr: req.ParticipantAddress,
		Amount:          amountFloat,
		PaymentMethod:   paymentMethod,
		Status:          database.AltPaymentStatusConfirmed,
		ConfirmedBy:     userAddress.(string),
		ConfirmedAt:     &now,
	}

	// Save to database
	if err := h.db.AlternativePaymentService.Create(altPayment); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save alternative payment"})
		return
	}

	// Update bill paid amount
	bill.PaidAmount += amountFloat
	if bill.PaidAmount >= bill.TotalAmount {
		bill.Status = database.BillStatusPaid
	}

	if err := h.db.UpdateBill(bill); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update bill"})
		return
	}

	// Get payment breakdown
	breakdown, err := h.getBillPaymentBreakdown(uint(billID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get payment breakdown"})
		return
	}

	// TODO: Send WebSocket notification

	c.JSON(http.StatusOK, gin.H{
		"success":           true,
		"message":           "Alternative payment marked successfully",
		"payment_breakdown": breakdown,
	})
}

// RequestAlternativePayment creates a pending alternative payment request (guest)
// POST /api/v1/bills/:bill_id/request-alternative-payment
func (h *PaymentHandler) RequestAlternativePayment(c *gin.Context) {
	billIDStr := c.Param("bill_id")
	billID, err := strconv.ParseUint(billIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	var req struct {
		Amount          string `json:"amount" binding:"required"`
		PaymentMethod   string `json:"payment_method" binding:"required"`
		ParticipantName string `json:"participant_name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get bill from database to validate it exists
	_, err = h.db.GetBill(uint(billID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Convert amount to float64
	amountFloat, err := strconv.ParseFloat(req.Amount, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid amount format"})
		return
	}
	amountFloat = amountFloat / 1_000_000

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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payment method"})
		return
	}

	// Create pending alternative payment request
	altPayment := &database.AlternativePayment{
		BillID:          uint(billID),
		ParticipantAddr: "guest", // Placeholder for guest requests
		ParticipantName: req.ParticipantName,
		Amount:          amountFloat,
		PaymentMethod:   paymentMethod,
		Status:          database.AltPaymentStatusPending,
	}

	// Save to database
	if err := h.db.AlternativePaymentService.Create(altPayment); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save payment request"})
		return
	}

	// TODO: Send WebSocket notification to business

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
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	// Get pending payments from database
	pendingPayments, err := h.db.AlternativePaymentService.GetPendingByBillID(uint(billID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get pending payments"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"pending_payments": pendingPayments,
	})
}

// GetBillAlternativePayments gets all confirmed alternative payments for a bill (public)
// GET /api/v1/bills/:bill_id/alternative-payments
func (h *PaymentHandler) GetBillAlternativePayments(c *gin.Context) {
	billIDStr := c.Param("bill_id")
	billID, err := strconv.ParseUint(billIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	// Get confirmed alternative payments from database
	altPayments, err := h.db.AlternativePaymentService.GetConfirmedByBillID(uint(billID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get alternative payments"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"alternative_payments": altPayments,
	})
}

// GetBillPaymentBreakdown gets the payment breakdown for a bill (public)
// GET /api/v1/bills/:bill_id/payment-breakdown
func (h *PaymentHandler) GetBillPaymentBreakdown(c *gin.Context) {
	billIDStr := c.Param("bill_id")
	billID, err := strconv.ParseUint(billIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	breakdown, err := h.getBillPaymentBreakdown(uint(billID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get payment breakdown"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"breakdown": breakdown,
	})
}

// Helper function to calculate payment breakdown
func (h *PaymentHandler) getBillPaymentBreakdown(billID uint) (*database.PaymentBreakdown, error) {
	// Get bill
	bill, err := h.db.GetBill(billID)
	if err != nil {
		return nil, err
	}

	// Get alternative payments total
	altPayments, err := h.db.AlternativePaymentService.GetConfirmedByBillID(billID)
	if err != nil {
		return nil, err
	}

	var alternativePaid float64
	for _, payment := range altPayments {
		alternativePaid += payment.Amount
	}

	// Calculate crypto paid (total paid - alternative paid)
	cryptoPaid := bill.PaidAmount - alternativePaid
	if cryptoPaid < 0 {
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

// CryptoPaymentRequest represents a crypto payment request
type CryptoPaymentRequest struct {
	TransactionHash   string  `json:"transaction_hash" binding:"required"`
	AmountPaid        float64 `json:"amount_paid" binding:"required"`
	TipAmount         float64 `json:"tip_amount"`
	PaymentMethod     string  `json:"payment_method" binding:"required"`
	BlockchainNetwork string  `json:"blockchain_network"`
}

// ProcessCryptoPayment handles crypto payment completion
// POST /api/v1/inside/bills/:bill_id/crypto-payment
func (h *PaymentHandler) ProcessCryptoPayment(c *gin.Context) {
	billIDStr := c.Param("bill_id")
	billID, err := strconv.ParseUint(billIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	var req CryptoPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get bill from database
	bill, err := h.db.GetBill(uint(billID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Create bill on-chain if it doesn't exist yet
	// This is where the bill creator address creates the on-chain bill
	if h.blockchain != nil {
		metadata := fmt.Sprintf(`{"bill_id":%d,"business_name":"%s","bill_number":"%s"}`,
			billID, "Business", bill.BillNumber) // TODO: Get actual business name
		nonce := fmt.Sprintf("bill_%d_%d", billID, time.Now().Unix())

		_, err = h.blockchain.CreateBill(
			fmt.Sprintf("%d", billID),   // Use database bill ID as blockchain bill ID
			bill.SettlementAddr,         // Business address
			int64(bill.TotalAmount*1e6), // Convert to USDC wei (6 decimals)
			metadata,
			nonce,
		)
		if err != nil {
			// Log error but continue - bill might already exist on-chain
			fmt.Printf("Note: Could not create bill on-chain (may already exist): %v\n", err)
		}
	}

	// Update bill with payment information
	totalPaid := req.AmountPaid + req.TipAmount
	bill.PaidAmount += totalPaid

	// Check if bill is fully paid
	if bill.PaidAmount >= bill.TotalAmount {
		bill.Status = database.BillStatusPaid
	}
	// Note: We keep the bill as "open" if not fully paid

	// Update the bill in database
	err = h.db.UpdateBill(bill)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update bill"})
		return
	}

	// Create payment record
	payment := &database.Payment{
		BillID:    uint(billID),
		PayerAddr: "crypto_guest", // Placeholder for crypto payments
		Amount:    req.AmountPaid,
		TipAmount: req.TipAmount,
		TxHash:    req.TransactionHash,
		Status:    database.PaymentStatusConfirmed,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err = database.CreatePayment(payment)
	if err != nil {
		// Log error but don't fail the request since bill was already updated
		// TODO: Add proper logging
	}

	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"message":          "Payment processed successfully",
		"bill_id":          billID,
		"transaction_hash": req.TransactionHash,
		"amount_paid":      req.AmountPaid,
		"tip_amount":       req.TipAmount,
		"bill_status":      bill.Status,
		"remaining_amount": bill.TotalAmount - bill.PaidAmount,
	})
}

// CreateOnChainBillRequest represents a request to create a bill on-chain
type CreateOnChainBillRequest struct {
	BusinessAddress string  `json:"business_address" binding:"required"`
	TotalAmount     float64 `json:"total_amount" binding:"required"`
}

// CreateOnChainBill creates a bill on the blockchain
// POST /api/v1/inside/bills/:bill_id/create-onchain
func (h *PaymentHandler) CreateOnChainBill(c *gin.Context) {
	billIDStr := c.Param("bill_id")
	billID, err := strconv.ParseUint(billIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	var req CreateOnChainBillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get bill from database
	bill, err := h.db.GetBill(uint(billID))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	// Create bill on-chain using the blockchain service
	if h.blockchain != nil {
		metadata := fmt.Sprintf(`{"bill_id":%d,"business_address":"%s","bill_number":"%s","created_at":"%s"}`,
			billID, req.BusinessAddress, bill.BillNumber, bill.CreatedAt.Format("2006-01-02T15:04:05Z"))
		nonce := fmt.Sprintf("bill_%d_%d", billID, time.Now().Unix())

		result, err := h.blockchain.CreateBill(
			fmt.Sprintf("%d", billID),  // Use database bill ID as blockchain bill ID
			req.BusinessAddress,        // Business address from request
			int64(req.TotalAmount*1e6), // Convert to USDC wei (6 decimals)
			metadata,
			nonce,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to create bill on-chain",
				"details": err.Error(),
				"hint":    "Business might not be registered. Please register the business first.",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success":            true,
			"message":            "Bill created on-chain successfully",
			"bill_id":            billID,
			"transaction_hash":   result.TransactionHash,
			"blockchain_bill_id": fmt.Sprintf("%d", billID),
		})
	} else {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Blockchain service not available",
		})
	}
}
