package handlers

import (
	"net/http"
	"strconv"
	"time"

	"invoice-generator/database"
	"invoice-generator/models"
	"invoice-generator/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type InvoiceHandler struct {
	db        *gorm.DB
	qrLink    *services.QRLinkService
	email     *services.EmailService
}

type CreateInvoiceRequest struct {
	Creator     string     `json:"creator" binding:"required"`
	CreatorName string     `json:"creator_name"`
	Title       string     `json:"title" binding:"required"`
	Description string     `json:"description"`
	Amount      uint64     `json:"amount" binding:"required,min=1"`
	PayerEmail  string     `json:"payer_email"`
	PayerName   string     `json:"payer_name"`
	DueDate     *time.Time `json:"due_date"`
}

type InvoiceResponse struct {
	*models.Invoice
	AmountFormatted string `json:"amount_formatted"`
	PaymentStatus   string `json:"payment_status"`
}

func NewInvoiceHandler() *InvoiceHandler {
	return &InvoiceHandler{
		db:     database.GetDB(),
		qrLink: services.NewQRLinkService(),
		email:  services.NewEmailService(),
	}
}

func (h *InvoiceHandler) CreateInvoice(c *gin.Context) {
	var req CreateInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// Input validation and sanitization
	if len(req.Title) > 200 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Title too long (max 200 characters)"})
		return
	}
	if len(req.Description) > 1000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Description too long (max 1000 characters)"})
		return
	}
	if req.Amount > 1000000*1000000 { // Max 1M USDC
		c.JSON(http.StatusBadRequest, gin.H{"error": "Amount too large"})
		return
	}
	if req.PayerEmail != "" && len(req.PayerEmail) > 254 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email too long"})
		return
	}

	// Create invoice record (without invoice_id initially)
	invoice := models.Invoice{
		Creator:     req.Creator,
		CreatorName: req.CreatorName,
		Title:       req.Title,
		Description: req.Description,
		Amount:      req.Amount,
		PayerEmail:  req.PayerEmail,
		PayerName:   req.PayerName,
		DueDate:     req.DueDate,
		Status:      "pending",
	}

	// Save to database first to get an ID
	if err := h.db.Create(&invoice).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create invoice"})
		return
	}

	// Use the database ID as the invoice ID for now
	// In production, this would be set after the smart contract transaction
	invoice.InvoiceID = uint64(invoice.ID)

	// Generate payment link and QR code
	paymentLink := h.qrLink.GeneratePaymentLink(invoice.InvoiceID)
	qrCodeURL, err := h.qrLink.GenerateQRCode(paymentLink, invoice.InvoiceID)
	if err != nil {
		// Log error but don't fail the request
		qrCodeURL = ""
	}

	// Update invoice with generated links
	invoice.PaymentLink = paymentLink
	invoice.QRCodeURL = qrCodeURL

	// Create metadata URI (in production, this could be IPFS)
	metadataURI := paymentLink + "/metadata"
	invoice.MetadataURI = metadataURI

	// Save updated invoice
	if err := h.db.Save(&invoice).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update invoice"})
		return
	}

	response := &InvoiceResponse{
		Invoice:         &invoice,
		AmountFormatted: formatUSDC(invoice.Amount),
		PaymentStatus:   getPaymentStatus(&invoice),
	}

	c.JSON(http.StatusCreated, response)
}

func (h *InvoiceHandler) GetInvoice(c *gin.Context) {
	invoiceIDStr := c.Param("id")
	invoiceID, err := strconv.ParseUint(invoiceIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invoice ID"})
		return
	}

	var invoice models.Invoice
	result := h.db.Where("invoice_id = ?", invoiceID).First(&invoice)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Invoice not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	response := &InvoiceResponse{
		Invoice:         &invoice,
		AmountFormatted: formatUSDC(invoice.Amount),
		PaymentStatus:   getPaymentStatus(&invoice),
	}

	c.JSON(http.StatusOK, response)
}

func (h *InvoiceHandler) GetInvoicesByCreator(c *gin.Context) {
	creator := c.Query("creator")
	if creator == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Creator address required"})
		return
	}

	var invoices []models.Invoice
	result := h.db.Where("creator = ?", creator).Order("created_at DESC").Find(&invoices)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	var responses []InvoiceResponse
	for _, invoice := range invoices {
		responses = append(responses, InvoiceResponse{
			Invoice:         &invoice,
			AmountFormatted: formatUSDC(invoice.Amount),
			PaymentStatus:   getPaymentStatus(&invoice),
		})
	}

	c.JSON(http.StatusOK, responses)
}

func (h *InvoiceHandler) GetInvoicePayments(c *gin.Context) {
	invoiceIDStr := c.Param("id")
	invoiceID, err := strconv.ParseUint(invoiceIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invoice ID"})
		return
	}

	var payments []models.Payment
	result := h.db.Where("invoice_id = ?", invoiceID).Order("created_at DESC").Find(&payments)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	c.JSON(http.StatusOK, payments)
}

func (h *InvoiceHandler) CancelInvoice(c *gin.Context) {
	invoiceIDStr := c.Param("id")
	invoiceID, err := strconv.ParseUint(invoiceIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invoice ID"})
		return
	}

	creator := c.Query("creator")
	if creator == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Creator address required"})
		return
	}

	var invoice models.Invoice
	result := h.db.Where("invoice_id = ? AND creator = ?", invoiceID, creator).First(&invoice)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Invoice not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	if invoice.AmountPaid > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot cancel invoice with payments"})
		return
	}

	invoice.Status = "cancelled"
	invoice.IsActive = false

	if err := h.db.Save(&invoice).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel invoice"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Invoice cancelled successfully"})
}

func (h *InvoiceHandler) GetInvoiceMetadata(c *gin.Context) {
	invoiceIDStr := c.Param("id")
	invoiceID, err := strconv.ParseUint(invoiceIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid invoice ID"})
		return
	}

	var invoice models.Invoice
	result := h.db.Where("invoice_id = ?", invoiceID).First(&invoice)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Invoice not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}

	// Return metadata in a format suitable for IPFS or smart contract
	metadata := gin.H{
		"invoice_id":   invoice.InvoiceID,
		"title":        invoice.Title,
		"description":  invoice.Description,
		"amount":       invoice.Amount,
		"creator":      invoice.Creator,
		"creator_name": invoice.CreatorName,
		"due_date":     invoice.DueDate,
		"created_at":   invoice.CreatedAt,
	}

	c.JSON(http.StatusOK, metadata)
}

// Helper functions
func formatUSDC(amount uint64) string {
	return strconv.FormatFloat(float64(amount)/1000000, 'f', 2, 64)
}

func getPaymentStatus(invoice *models.Invoice) string {
	if invoice.AmountPaid == 0 {
		return "pending"
	} else if invoice.AmountPaid < invoice.Amount {
		return "partially_paid"
	} else {
		return "paid"
	}
}
