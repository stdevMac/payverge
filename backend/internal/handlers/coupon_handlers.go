package handlers

import (
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"payverge/internal/database"
	"payverge/internal/services"
	"payverge/internal/structs"
)

// CouponHandlers handles coupon-related API endpoints
type CouponHandlers struct {
	couponService *services.CouponService
}

// NewCouponHandlers creates a new coupon handlers instance
func NewCouponHandlers(couponService *services.CouponService) *CouponHandlers {
	return &CouponHandlers{
		couponService: couponService,
	}
}

// CouponRequest represents a coupon creation/update request
type CouponRequest struct {
	Code   string `json:"code" binding:"required"`
	Amount string `json:"amount" binding:"required"`
	Expiry string `json:"expiry,omitempty"`
}

// CouponResponse represents a coupon response
type CouponResponse struct {
	Success bool                    `json:"success"`
	Coupon  *structs.CouponDetails  `json:"coupon,omitempty"`
	Message string                  `json:"message,omitempty"`
	Error   string                  `json:"error,omitempty"`
}

// CouponsResponse represents multiple coupons response
type CouponsResponse struct {
	Success bool                     `json:"success"`
	Coupons []structs.CouponDetails  `json:"coupons"`
	Message string                   `json:"message,omitempty"`
	Error   string                   `json:"error,omitempty"`
}

// CreateCoupon creates a new coupon (admin only)
// This endpoint tracks coupon creation but the actual smart contract call
// should be made from the frontend for proper wallet integration
func (ch *CouponHandlers) CreateCoupon(c *gin.Context) {
	var req CouponRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, CouponResponse{
			Success: false,
			Error:   "Invalid request format: " + err.Error(),
		})
		return
	}

	// Validate coupon code format
	if err := ch.couponService.ValidateCouponCode(req.Code); err != nil {
		c.JSON(http.StatusBadRequest, CouponResponse{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Parse amount
	amount, err := ch.couponService.ParseUSDC(req.Amount)
	if err != nil {
		c.JSON(http.StatusBadRequest, CouponResponse{
			Success: false,
			Error:   "Invalid amount format: " + err.Error(),
		})
		return
	}

	// Generate coupon hash
	couponHash := ch.couponService.GenerateCouponHash(req.Code)

	// Parse expiry if provided
	var expiryTime *time.Time
	if req.Expiry != "" {
		parsed, err := time.Parse(time.RFC3339, req.Expiry)
		if err != nil {
			// Try alternative format
			parsed, err = time.Parse("2006-01-02T15:04", req.Expiry)
			if err != nil {
				c.JSON(http.StatusBadRequest, CouponResponse{
					Success: false,
					Error:   "Invalid expiry format. Use RFC3339 or YYYY-MM-DDTHH:MM",
				})
				return
			}
		}
		expiryTime = &parsed
	}

	// Create database record
	coupon := &database.Coupon{
		Code:           req.Code,
		Hash:           couponHash,
		DiscountAmount: amount.String(),
		ExpiryTime:     expiryTime,
		IsActive:       true,
		IsUsed:         false,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := database.SaveCoupon(coupon); err != nil {
		log.Printf("Error saving coupon to database: %v", err)
		c.JSON(http.StatusInternalServerError, CouponResponse{
			Success: false,
			Error:   "Failed to save coupon",
		})
		return
	}

	// Convert to response format
	couponDetails := structs.CouponDetails{
		Code:           coupon.Code,
		Hash:           coupon.Hash,
		DiscountAmount: ch.couponService.FormatUSDC(amount),
		ExpiryTime:     expiryTime,
		IsActive:       coupon.IsActive,
		IsUsed:         coupon.IsUsed,
		CreatedAt:      coupon.CreatedAt,
	}

	c.JSON(http.StatusOK, CouponResponse{
		Success: true,
		Coupon:  &couponDetails,
		Message: "Coupon created successfully. Please complete the transaction on the blockchain.",
	})
}

// GetCoupon retrieves a specific coupon by code or hash
func (ch *CouponHandlers) GetCoupon(c *gin.Context) {
	codeOrHash := c.Param("codeOrHash")
	if codeOrHash == "" {
		c.JSON(http.StatusBadRequest, CouponResponse{
			Success: false,
			Error:   "Code or hash is required",
		})
		return
	}

	var coupon *database.Coupon
	var err error

	// Try to get by code first, then by hash
	if strings.HasPrefix(codeOrHash, "0x") {
		coupon, err = database.GetCouponByHash(codeOrHash)
	} else {
		coupon, err = database.GetCouponByCode(codeOrHash)
	}

	if err != nil {
		c.JSON(http.StatusNotFound, CouponResponse{
			Success: false,
			Error:   "Coupon not found",
		})
		return
	}

	// Try to sync with smart contract if available
	if ch.couponService != nil {
		contractInfo, err := ch.couponService.GetCouponInfo(coupon.Hash)
		if err == nil && contractInfo != nil {
			// Update database with smart contract state
			coupon.IsActive = contractInfo.IsActive
			coupon.IsUsed = contractInfo.IsUsed
			if contractInfo.IsUsed && coupon.UsedBy == "" {
				// We don't have the business address from the contract call
				// This would need to be updated via event watching
			}
			database.UpdateCoupon(coupon)
		}
	}

	// Parse amount for response
	amount, _ := ch.couponService.ParseUSDC(coupon.DiscountAmount)
	
	couponDetails := structs.CouponDetails{
		Code:           coupon.Code,
		Hash:           coupon.Hash,
		DiscountAmount: ch.couponService.FormatUSDC(amount),
		ExpiryTime:     coupon.ExpiryTime,
		IsActive:       coupon.IsActive,
		IsUsed:         coupon.IsUsed,
		UsedBy:         coupon.UsedBy,
		UsedAt:         coupon.UsedAt,
		CreatedAt:      coupon.CreatedAt,
	}

	c.JSON(http.StatusOK, CouponResponse{
		Success: true,
		Coupon:  &couponDetails,
	})
}

// GetAllCoupons retrieves all coupons (admin only)
func (ch *CouponHandlers) GetAllCoupons(c *gin.Context) {
	coupons, err := database.GetAllCoupons()
	if err != nil {
		log.Printf("Error getting coupons from database: %v", err)
		c.JSON(http.StatusInternalServerError, CouponsResponse{
			Success: false,
			Error:   "Failed to retrieve coupons",
		})
		return
	}

	log.Printf("Retrieved %d coupons from database", len(coupons))

	// Convert to response format - ensure we always have a slice, even if empty
	couponDetails := make([]structs.CouponDetails, 0, len(coupons))
	for _, coupon := range coupons {
		log.Printf("Processing coupon %s with discount_amount: %s", coupon.Code, coupon.DiscountAmount)
		
		// The database stores raw USDC units (e.g., 25000000 for $25.00)
		// So we need to create a big.Int directly from the stored string
		amount := new(big.Int)
		_, ok := amount.SetString(coupon.DiscountAmount, 10)
		if !ok {
			log.Printf("Error parsing discount amount as big.Int: %s", coupon.DiscountAmount)
			amount = big.NewInt(0)
		}
		
		formattedAmount := ch.couponService.FormatUSDC(amount)
		log.Printf("Formatted amount for coupon %s: %s", coupon.Code, formattedAmount)
		
		couponDetails = append(couponDetails, structs.CouponDetails{
			Code:           coupon.Code,
			Hash:           coupon.Hash,
			DiscountAmount: formattedAmount,
			ExpiryTime:     coupon.ExpiryTime,
			IsActive:       coupon.IsActive,
			IsUsed:         coupon.IsUsed,
			UsedBy:         coupon.UsedBy,
			UsedAt:         coupon.UsedAt,
			CreatedAt:      coupon.CreatedAt,
		})
	}

	log.Printf("Sending response with %d coupon details", len(couponDetails))

	c.JSON(http.StatusOK, CouponsResponse{
		Success: true,
		Coupons: couponDetails,
	})
}

// ValidateCoupon validates a coupon for use (public endpoint)
func (ch *CouponHandlers) ValidateCoupon(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"valid":   false,
			"message": "Coupon code is required",
		})
		return
	}

	coupon, err := database.GetCouponByCode(code)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"valid":   false,
			"message": "Coupon not found",
		})
		return
	}

	// Check if coupon is active
	if !coupon.IsActive {
		c.JSON(http.StatusOK, gin.H{
			"valid":   false,
			"message": "Coupon is not active",
		})
		return
	}

	// Check if coupon is already used
	if coupon.IsUsed {
		c.JSON(http.StatusOK, gin.H{
			"valid":   false,
			"message": "Coupon has already been used",
		})
		return
	}

	// Check expiry
	if coupon.ExpiryTime != nil && time.Now().After(*coupon.ExpiryTime) {
		c.JSON(http.StatusOK, gin.H{
			"valid":   false,
			"message": "Coupon has expired",
		})
		return
	}

	// Try to sync with smart contract
	if ch.couponService != nil {
		contractInfo, err := ch.couponService.GetCouponInfo(coupon.Hash)
		if err == nil && contractInfo != nil {
			if !contractInfo.IsActive || contractInfo.IsUsed {
				c.JSON(http.StatusOK, gin.H{
					"valid":   false,
					"message": "Coupon is not valid on blockchain",
				})
				return
			}
		}
	}

	amount, _ := ch.couponService.ParseUSDC(coupon.DiscountAmount)

	c.JSON(http.StatusOK, gin.H{
		"valid":          true,
		"code":           coupon.Code,
		"discountAmount": ch.couponService.FormatUSDC(amount),
		"hash":           coupon.Hash,
		"message":        "Coupon is valid",
	})
}

// MarkCouponUsed marks a coupon as used (called by blockchain event handler)
func (ch *CouponHandlers) MarkCouponUsed(c *gin.Context) {
	var req struct {
		Hash     string `json:"hash" binding:"required"`
		UsedBy   string `json:"usedBy" binding:"required"`
		TxHash   string `json:"txHash"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request format",
		})
		return
	}

	coupon, err := database.GetCouponByHash(req.Hash)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   "Coupon not found",
		})
		return
	}

	// Update coupon status
	now := time.Now()
	coupon.IsUsed = true
	coupon.UsedBy = req.UsedBy
	coupon.UsedAt = &now
	coupon.UpdatedAt = now

	if err := database.UpdateCoupon(coupon); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to update coupon",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Coupon marked as used",
	})
}

// DeactivateCoupon marks a coupon as inactive (admin only)
func (ch *CouponHandlers) DeactivateCoupon(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, CouponResponse{
			Success: false,
			Error:   "Coupon code is required",
		})
		return
	}

	coupon, err := database.GetCouponByCode(code)
	if err != nil {
		c.JSON(http.StatusNotFound, CouponResponse{
			Success: false,
			Error:   "Coupon not found",
		})
		return
	}

	// Update coupon status
	coupon.IsActive = false
	coupon.UpdatedAt = time.Now()

	if err := database.UpdateCoupon(coupon); err != nil {
		c.JSON(http.StatusInternalServerError, CouponResponse{
			Success: false,
			Error:   "Failed to deactivate coupon",
		})
		return
	}

	c.JSON(http.StatusOK, CouponResponse{
		Success: true,
		Message: "Coupon deactivated successfully. Please complete the transaction on the blockchain.",
	})
}

// SyncWithBlockchain syncs all coupons with blockchain state (admin only)
func (ch *CouponHandlers) SyncWithBlockchain(c *gin.Context) {
	if ch.couponService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "Blockchain service not available",
		})
		return
	}

	coupons, err := database.GetAllCoupons()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to retrieve coupons",
		})
		return
	}

	syncedCount := 0
	for _, coupon := range coupons {
		contractInfo, err := ch.couponService.GetCouponInfo(coupon.Hash)
		if err != nil {
			log.Printf("Failed to get contract info for coupon %s: %v", coupon.Code, err)
			continue
		}

		// Update database with contract state
		updated := false
		if coupon.IsActive != contractInfo.IsActive {
			coupon.IsActive = contractInfo.IsActive
			updated = true
		}
		if coupon.IsUsed != contractInfo.IsUsed {
			coupon.IsUsed = contractInfo.IsUsed
			updated = true
		}

		if updated {
			coupon.UpdatedAt = time.Now()
			if err := database.UpdateCoupon(&coupon); err != nil {
				log.Printf("Failed to update coupon %s: %v", coupon.Code, err)
			} else {
				syncedCount++
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"message":     fmt.Sprintf("Synced %d coupons with blockchain", syncedCount),
		"syncedCount": syncedCount,
		"totalCount":  len(coupons),
	})
}
