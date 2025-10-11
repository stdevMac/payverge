package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"payverge/internal/database"
)

type WithdrawalHandler struct {
	db *database.DB
}

func NewWithdrawalHandler(db *database.DB) *WithdrawalHandler {
	return &WithdrawalHandler{
		db: db,
	}
}

// CreateWithdrawalRequest represents the request body for creating a withdrawal record
type CreateWithdrawalRequest struct {
	TransactionHash   string  `json:"transaction_hash" binding:"required"`
	PaymentAmount     float64 `json:"payment_amount"`
	TipAmount         float64 `json:"tip_amount"`
	TotalAmount       float64 `json:"total_amount" binding:"required"`
	WithdrawalAddress string  `json:"withdrawal_address" binding:"required"`
	BlockchainNetwork string  `json:"blockchain_network" binding:"required"`
}

// UpdateWithdrawalStatusRequest represents the request body for updating withdrawal status
type UpdateWithdrawalStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=pending confirmed failed"`
}

// CreateWithdrawal creates a new withdrawal history record
func (h *WithdrawalHandler) CreateWithdrawal(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	var req CreateWithdrawalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify business exists and user has access
	var business database.Business
	if err := h.db.GetGorm().First(&business, businessID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	// Get user address from context (set by auth middleware)
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User address not found"})
		return
	}

	// Verify user owns the business
	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Create withdrawal record
	withdrawal := database.WithdrawalHistory{
		BusinessID:        uint(businessID),
		TransactionHash:   req.TransactionHash,
		PaymentAmount:     req.PaymentAmount,
		TipAmount:         req.TipAmount,
		TotalAmount:       req.TotalAmount,
		WithdrawalAddress: req.WithdrawalAddress,
		BlockchainNetwork: req.BlockchainNetwork,
		Status:            "pending",
	}

	if err := h.db.GetGorm().Create(&withdrawal).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create withdrawal record"})
		return
	}

	c.JSON(http.StatusCreated, withdrawal)
}

// GetWithdrawalHistory retrieves withdrawal history for a business
func (h *WithdrawalHandler) GetWithdrawalHistory(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	// Verify business exists and user has access
	var business database.Business
	if err := h.db.GetGorm().First(&business, businessID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	// Get user address from context
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User address not found"})
		return
	}

	// Verify user owns the business
	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Get pagination parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	offset := (page - 1) * limit

	// Get withdrawals with pagination
	var withdrawals []database.WithdrawalHistory
	var total int64

	// Count total records
	h.db.GetGorm().Model(&database.WithdrawalHistory{}).Where("business_id = ?", businessID).Count(&total)

	// Get paginated records
	if err := h.db.GetGorm().Where("business_id = ?", businessID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&withdrawals).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve withdrawal history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"withdrawals": withdrawals,
		"pagination": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
			"pages": (total + int64(limit) - 1) / int64(limit),
		},
	})
}

// GetWithdrawal retrieves a specific withdrawal record
func (h *WithdrawalHandler) GetWithdrawal(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	withdrawalIDStr := c.Param("withdrawalId")
	withdrawalID, err := strconv.ParseUint(withdrawalIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid withdrawal ID"})
		return
	}

	// Verify business exists and user has access
	var business database.Business
	if err := h.db.GetGorm().First(&business, businessID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	// Get user address from context
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User address not found"})
		return
	}

	// Verify user owns the business
	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Get withdrawal record
	var withdrawal database.WithdrawalHistory
	if err := h.db.GetGorm().Where("id = ? AND business_id = ?", withdrawalID, businessID).First(&withdrawal).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Withdrawal not found"})
		return
	}

	c.JSON(http.StatusOK, withdrawal)
}

// UpdateWithdrawalStatus updates the status of a withdrawal (for blockchain confirmation)
func (h *WithdrawalHandler) UpdateWithdrawalStatus(c *gin.Context) {
	businessIDStr := c.Param("id")
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
		return
	}

	withdrawalIDStr := c.Param("withdrawalId")
	withdrawalID, err := strconv.ParseUint(withdrawalIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid withdrawal ID"})
		return
	}

	var req UpdateWithdrawalStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Verify business exists and user has access
	var business database.Business
	if err := h.db.GetGorm().First(&business, businessID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	// Get user address from context
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User address not found"})
		return
	}

	// Verify user owns the business
	if business.OwnerAddress != userAddress.(string) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return
	}

	// Update withdrawal status
	updates := map[string]interface{}{
		"status":     req.Status,
		"updated_at": time.Now(),
	}

	// Set confirmed_at timestamp if status is confirmed
	if req.Status == "confirmed" {
		now := time.Now()
		updates["confirmed_at"] = &now
	}

	if err := h.db.GetGorm().Model(&database.WithdrawalHistory{}).
		Where("id = ? AND business_id = ?", withdrawalID, businessID).
		Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update withdrawal status"})
		return
	}

	// Get updated withdrawal record
	var withdrawal database.WithdrawalHistory
	h.db.GetGorm().Where("id = ? AND business_id = ?", withdrawalID, businessID).First(&withdrawal)

	c.JSON(http.StatusOK, withdrawal)
}
