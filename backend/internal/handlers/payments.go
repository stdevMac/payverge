package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"payverge/internal/blockchain"
	"payverge/internal/database"
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
		"status":          "confirmed",
		"block_number":    12345,
		"gas_used":        21000,
		"timestamp":       "2024-01-01T00:00:00Z",
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
