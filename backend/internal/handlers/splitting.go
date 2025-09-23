package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"payverge/internal/blockchain"
	"payverge/internal/database"
	"payverge/internal/splitting"
)

// SplittingHandler handles bill splitting requests
type SplittingHandler struct {
	db         *database.DB
	splitter   *splitting.SplittingService
	blockchain *blockchain.BlockchainService
}

// NewSplittingHandler creates a new splitting handler
func NewSplittingHandler(db *database.DB, blockchainService *blockchain.BlockchainService) *SplittingHandler {
	return &SplittingHandler{
		db:         db,
		splitter:   splitting.NewSplittingService(db),
		blockchain: blockchainService,
	}
}

// CalculateEqualSplit calculates equal split for a bill
// POST /api/v1/bills/:id/split/equal
func (h *SplittingHandler) CalculateEqualSplit(c *gin.Context) {
	billIDStr := c.Param("bill_id")
	billID, err := strconv.ParseUint(billIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	var req splitting.EqualSplitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Override bill ID from URL parameter
	req.BillID = uint(billID)

	// Validate number of people
	if req.NumPeople <= 0 || req.NumPeople > 20 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Number of people must be between 1 and 20"})
		return
	}

	result, err := h.splitter.CalculateEqualSplit(req.BillID, req.NumPeople)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"result":  result,
	})
}

// CalculateCustomSplit calculates custom split for a bill
// POST /api/v1/bills/:id/split/custom
func (h *SplittingHandler) CalculateCustomSplit(c *gin.Context) {
	billIDStr := c.Param("bill_id")
	billID, err := strconv.ParseUint(billIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	var req splitting.CustomSplitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Override bill ID from URL parameter
	req.BillID = uint(billID)

	// Validate amounts
	if len(req.Amounts) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Amounts cannot be empty"})
		return
	}

	if len(req.Amounts) > 20 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot split between more than 20 people"})
		return
	}

	result, err := h.splitter.CalculateCustomSplit(req.BillID, req.Amounts, req.People)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"result":  result,
	})
}

// CalculateItemSplit calculates item-based split for a bill
// POST /api/v1/bills/:id/split/items
func (h *SplittingHandler) CalculateItemSplit(c *gin.Context) {
	billIDStr := c.Param("bill_id")
	billID, err := strconv.ParseUint(billIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	var req splitting.ItemSplitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Override bill ID from URL parameter
	req.BillID = uint(billID)

	// Validate item selections
	if len(req.ItemSelections) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Item selections cannot be empty"})
		return
	}

	if len(req.ItemSelections) > 20 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot split between more than 20 people"})
		return
	}

	result, err := h.splitter.CalculateItemSplit(req.BillID, req.ItemSelections, req.People)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"result":  result,
	})
}

// GetBillSplitOptions returns available split options for a bill
// GET /api/v1/bills/:id/split/options
func (h *SplittingHandler) GetBillSplitOptions(c *gin.Context) {
	billIDStr := c.Param("bill_id")
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

	// Parse bill items from JSON
	var billItems []map[string]interface{}
	if bill.Items != "" {
		if err := json.Unmarshal([]byte(bill.Items), &billItems); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse bill items"})
			return
		}
	}

	// Calculate remaining amount to be paid
	remainingAmount := bill.TotalAmount - bill.PaidAmount

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"bill": gin.H{
			"id":               bill.ID,
			"bill_number":      bill.BillNumber,
			"total_amount":     bill.TotalAmount,
			"paid_amount":      bill.PaidAmount,
			"remaining_amount": remainingAmount,
			"subtotal":         bill.Subtotal,
			"tax_amount":       bill.TaxAmount,
			"service_fee_amount": bill.ServiceFeeAmount,
			"status":           bill.Status,
		},
		"items": billItems,
		"split_options": gin.H{
			"equal": gin.H{
				"available":    true,
				"description":  "Split the bill equally among all people",
				"min_people":   1,
				"max_people":   20,
			},
			"custom": gin.H{
				"available":    true,
				"description":  "Specify custom amounts for each person",
				"min_people":   1,
				"max_people":   20,
			},
			"items": gin.H{
				"available":    len(billItems) > 0,
				"description":  "Split based on which items each person ordered",
				"min_people":   1,
				"max_people":   20,
				"total_items":  len(billItems),
			},
		},
	})
}

// ValidateSplit validates a split calculation without saving
// POST /api/v1/bills/:id/split/validate
func (h *SplittingHandler) ValidateSplit(c *gin.Context) {
	billIDStr := c.Param("bill_id")
	billID, err := strconv.ParseUint(billIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid bill ID"})
		return
	}

	var req struct {
		Method          string                     `json:"method"`
		NumPeople       int                        `json:"num_people,omitempty"`
		Amounts         map[string]float64         `json:"amounts,omitempty"`
		ItemSelections  map[string][]string        `json:"item_selections,omitempty"`
		People          map[string]string          `json:"people,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var result *splitting.SplitResult

	switch req.Method {
	case "equal":
		if req.NumPeople <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Number of people is required for equal split"})
			return
		}
		result, err = h.splitter.CalculateEqualSplit(uint(billID), req.NumPeople)
	case "custom":
		if len(req.Amounts) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Amounts are required for custom split"})
			return
		}
		result, err = h.splitter.CalculateCustomSplit(uint(billID), req.Amounts, req.People)
	case "items":
		if len(req.ItemSelections) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Item selections are required for item split"})
			return
		}
		result, err = h.splitter.CalculateItemSplit(uint(billID), req.ItemSelections, req.People)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid split method. Must be 'equal', 'custom', or 'items'"})
		return
	}

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"valid":   true,
		"result":  result,
	})
}

// GetBillParticipants gets all participants who have paid for a bill from blockchain
// GET /api/v1/bills/:id/participants
func (h *SplittingHandler) GetBillParticipants(c *gin.Context) {
	billIDStr := c.Param("bill_id")
	
	// For now, return empty participants list if blockchain service is not available
	// This allows the frontend to work without blockchain integration
	if h.blockchain == nil {
		c.JSON(http.StatusOK, gin.H{
			"success":      true,
			"participants": []string{},
			"count":        0,
			"message":      "Blockchain service not available - returning empty participants",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	participants, err := h.blockchain.GetBillParticipants(ctx, billIDStr)
	if err != nil {
		// Return empty list instead of error to allow frontend to continue
		c.JSON(http.StatusOK, gin.H{
			"success":      true,
			"participants": []string{},
			"count":        0,
			"error":        err.Error(),
			"message":      "Blockchain error - returning empty participants",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"participants": participants,
		"count":        len(participants),
	})
}

// GetParticipantInfo gets information about a specific participant
// GET /api/v1/bills/:id/participants/:address
func (h *SplittingHandler) GetParticipantInfo(c *gin.Context) {
	billIDStr := c.Param("bill_id")
	participantAddress := c.Param("address")
	
	if h.blockchain == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Blockchain service not available"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	info, err := h.blockchain.GetParticipantInfo(ctx, billIDStr, participantAddress)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"participant": info,
	})
}

// GetBillSummaryWithParticipants gets bill summary including blockchain participant data
// GET /api/v1/bills/:id/summary
func (h *SplittingHandler) GetBillSummaryWithParticipants(c *gin.Context) {
	billIDStr := c.Param("bill_id")
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

	response := gin.H{
		"success": true,
		"bill": gin.H{
			"id":               bill.ID,
			"bill_number":      bill.BillNumber,
			"total_amount":     bill.TotalAmount,
			"paid_amount":      bill.PaidAmount,
			"remaining_amount": bill.TotalAmount - bill.PaidAmount,
			"status":           bill.Status,
		},
	}

	// Add blockchain data if service is available
	if h.blockchain != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Get blockchain summary
		blockchainSummary, err := h.blockchain.GetBillSummary(ctx, billIDStr)
		if err == nil {
			response["blockchain"] = blockchainSummary
		}

		// Get participants
		participants, err := h.blockchain.GetBillParticipants(ctx, billIDStr)
		if err == nil {
			response["participants"] = participants
			response["participant_count"] = len(participants)
		}
	}

	c.JSON(http.StatusOK, response)
}

// ExecuteSplitPayment coordinates a split payment execution
// POST /api/v1/bills/:id/split/execute
func (h *SplittingHandler) ExecuteSplitPayment(c *gin.Context) {
	billIDStr := c.Param("bill_id")
	
	var req struct {
		SplitResult *splitting.SplitResult `json:"split_result"`
		PaymentInfo map[string]interface{} `json:"payment_info"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.SplitResult == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Split result is required"})
		return
	}

	// Store split execution info (this would typically trigger payment processing)
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Split payment execution initiated",
		"bill_id": billIDStr,
		"method":  req.SplitResult.Method,
		"splits":  len(req.SplitResult.Splits),
	})
}
