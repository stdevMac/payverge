package server

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"payverge/internal/database"
)

// Referral registration request
type RegisterReferrerRequest struct {
	WalletAddress       string `json:"wallet_address" binding:"required"`
	ReferralCode        string `json:"referral_code" binding:"required"`
	Tier                string `json:"tier" binding:"required"`
	RegistrationTxHash  string `json:"registration_tx_hash" binding:"required"`
}

// Referral code availability check request
type CheckReferralCodeRequest struct {
	ReferralCode string `json:"referral_code" binding:"required"`
}

// Referral commission claim request
type ClaimCommissionRequest struct {
	WalletAddress string `json:"wallet_address" binding:"required"`
	Amount        string `json:"amount" binding:"required"`
	TxHash        string `json:"tx_hash" binding:"required"`
}

// Process referral request (used when a business registers with a referral code)
type ProcessReferralRequest struct {
	BusinessID         uint   `json:"business_id" binding:"required"`
	ReferralCode       string `json:"referral_code" binding:"required"`
	RegistrationFee    string `json:"registration_fee" binding:"required"`
	Discount           string `json:"discount" binding:"required"`
	Commission         string `json:"commission" binding:"required"`
	ProcessedTxHash    string `json:"processed_tx_hash" binding:"required"`
}

// Referral statistics response
type ReferralStatsResponse struct {
	TotalReferrers      int64  `json:"total_referrers"`
	TotalReferrals      int64  `json:"total_referrals"`
	TotalCommissions    string `json:"total_commissions"`
	BasicReferrers      int64  `json:"basic_referrers"`
	PremiumReferrers    int64  `json:"premium_referrers"`
}

// RegisterReferrer handles referrer registration
func RegisterReferrer(c *gin.Context) {
	var req RegisterReferrerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate tier
	if req.Tier != "basic" && req.Tier != "premium" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid tier. Must be 'basic' or 'premium'"})
		return
	}

	// Validate referral code format (6-12 characters, alphanumeric)
	if len(req.ReferralCode) < 6 || len(req.ReferralCode) > 12 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Referral code must be 6-12 characters long"})
		return
	}

	// Check if referral code is alphanumeric
	for _, char := range req.ReferralCode {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9')) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Referral code must contain only letters and numbers"})
			return
		}
	}

	// Check if wallet address already exists
	var existingReferrer database.Referrer
	if err := database.GetDB().Where("wallet_address = ?", req.WalletAddress).First(&existingReferrer).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Wallet address already registered as referrer"})
		return
	}

	// Check if referral code already exists
	if err := database.GetDB().Where("referral_code = ?", req.ReferralCode).First(&existingReferrer).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Referral code already taken"})
		return
	}

	// Create new referrer
	referrer := database.Referrer{
		WalletAddress:       req.WalletAddress,
		ReferralCode:        req.ReferralCode,
		Tier:                database.ReferralTier(req.Tier),
		Status:              database.ReferralStatusActive,
		TotalReferrals:      0,
		TotalCommissions:    "0",
		ClaimableCommissions: "0",
		RegistrationTxHash:  req.RegistrationTxHash,
	}

	if err := database.GetDB().Create(&referrer).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register referrer"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Referrer registered successfully",
		"referrer": referrer,
	})
}

// CheckReferralCodeAvailability checks if a referral code is available
func CheckReferralCodeAvailability(c *gin.Context) {
	var req CheckReferralCodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate referral code format
	if len(req.ReferralCode) < 6 || len(req.ReferralCode) > 12 {
		c.JSON(http.StatusOK, gin.H{
			"available": false,
			"error": "Referral code must be 6-12 characters long",
		})
		return
	}

	// Check if referral code is alphanumeric
	for _, char := range req.ReferralCode {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9')) {
			c.JSON(http.StatusOK, gin.H{
				"available": false,
				"error": "Referral code must contain only letters and numbers",
			})
			return
		}
	}

	// Check if referral code exists
	var existingReferrer database.Referrer
	if err := database.GetDB().Where("referral_code = ?", req.ReferralCode).First(&existingReferrer).Error; err == nil {
		c.JSON(http.StatusOK, gin.H{
			"available": false,
			"error": "Referral code already taken",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"available": true,
		"message": "Referral code is available",
	})
}

// GetReferrer gets referrer information by wallet address
func GetReferrer(c *gin.Context) {
	walletAddress := c.Param("wallet_address")
	if walletAddress == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Wallet address is required"})
		return
	}

	var referrer database.Referrer
	if err := database.GetDB().Where("wallet_address = ?", walletAddress).First(&referrer).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Referrer not found"})
		return
	}

	c.JSON(http.StatusOK, referrer)
}

// GetReferrerByCode gets referrer information by referral code
func GetReferrerByCode(c *gin.Context) {
	referralCode := c.Param("referral_code")
	if referralCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Referral code is required"})
		return
	}

	var referrer database.Referrer
	if err := database.GetDB().Where("referral_code = ?", referralCode).First(&referrer).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Referrer not found"})
		return
	}

	c.JSON(http.StatusOK, referrer)
}

// GetReferrerReferrals gets all referrals made by a referrer
func GetReferrerReferrals(c *gin.Context) {
	walletAddress := c.Param("wallet_address")
	if walletAddress == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Wallet address is required"})
		return
	}

	// Get referrer
	var referrer database.Referrer
	if err := database.GetDB().Where("wallet_address = ?", walletAddress).First(&referrer).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Referrer not found"})
		return
	}

	// Get referral records with business information
	var referralRecords []database.ReferralRecord
	if err := database.GetDB().Preload("Business").Where("referrer_id = ?", referrer.ID).Find(&referralRecords).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch referral records"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"referrer": referrer,
		"referrals": referralRecords,
	})
}

// ProcessReferral processes a referral when a business registers
func ProcessReferral(c *gin.Context) {
	var req ProcessReferralRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get referrer by code
	var referrer database.Referrer
	if err := database.GetDB().Where("referral_code = ? AND status = ?", req.ReferralCode, database.ReferralStatusActive).First(&referrer).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Active referrer not found with this code"})
		return
	}

	// Check if business exists
	var business database.Business
	if err := database.GetDB().Where("id = ?", req.BusinessID).First(&business).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return
	}

	// Create referral record
	referralRecord := database.ReferralRecord{
		ReferrerID:       referrer.ID,
		BusinessID:       req.BusinessID,
		RegistrationFee:  req.RegistrationFee,
		Discount:         req.Discount,
		Commission:       req.Commission,
		CommissionPaid:   false,
		ProcessedTxHash:  req.ProcessedTxHash,
	}

	if err := database.GetDB().Create(&referralRecord).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create referral record"})
		return
	}

	// Update referrer statistics
	referrer.TotalReferrals++
	// Add commission to claimable commissions (we'll need to parse and add the amounts)
	if err := database.GetDB().Save(&referrer).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update referrer statistics"})
		return
	}

	// Update business with referral code
	business.ReferredByCode = req.ReferralCode
	if err := database.GetDB().Save(&business).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update business referral info"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Referral processed successfully",
		"referral_record": referralRecord,
	})
}

// ClaimCommission processes a commission claim
func ClaimCommission(c *gin.Context) {
	var req ClaimCommissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get referrer
	var referrer database.Referrer
	if err := database.GetDB().Where("wallet_address = ?", req.WalletAddress).First(&referrer).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Referrer not found"})
		return
	}

	// Create commission claim record
	claim := database.ReferralCommissionClaim{
		ReferrerID: referrer.ID,
		Amount:     req.Amount,
		TxHash:     req.TxHash,
		Status:     "completed",
	}

	if err := database.GetDB().Create(&claim).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create commission claim"})
		return
	}

	// Update referrer's last claimed date
	now := time.Now()
	referrer.LastClaimedAt = &now
	// Reset claimable commissions (we'll need to implement proper amount calculation)
	referrer.ClaimableCommissions = "0"

	if err := database.GetDB().Save(&referrer).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update referrer"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Commission claimed successfully",
		"claim": claim,
	})
}

// GetReferralStats gets overall referral system statistics
func GetReferralStats(c *gin.Context) {
	var stats ReferralStatsResponse

	// Count total referrers
	database.GetDB().Model(&database.Referrer{}).Count(&stats.TotalReferrers)

	// Count basic and premium referrers
	database.GetDB().Model(&database.Referrer{}).Where("tier = ?", database.ReferralTierBasic).Count(&stats.BasicReferrers)
	database.GetDB().Model(&database.Referrer{}).Where("tier = ?", database.ReferralTierPremium).Count(&stats.PremiumReferrers)

	// Count total referrals
	database.GetDB().Model(&database.ReferralRecord{}).Count(&stats.TotalReferrals)

	// Calculate total commissions (this would need proper decimal handling in production)
	stats.TotalCommissions = "0" // Placeholder

	c.JSON(http.StatusOK, stats)
}

// UpdateReferralCode allows a referrer to update their referral code
func UpdateReferralCode(c *gin.Context) {
	walletAddress := c.Param("wallet_address")
	if walletAddress == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Wallet address is required"})
		return
	}

	var req struct {
		NewReferralCode string `json:"new_referral_code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate new referral code format
	if len(req.NewReferralCode) < 6 || len(req.NewReferralCode) > 12 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Referral code must be 6-12 characters long"})
		return
	}

	// Check if new referral code is alphanumeric
	for _, char := range req.NewReferralCode {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9')) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Referral code must contain only letters and numbers"})
			return
		}
	}

	// Get referrer
	var referrer database.Referrer
	if err := database.GetDB().Where("wallet_address = ?", walletAddress).First(&referrer).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Referrer not found"})
		return
	}

	// Check if new referral code already exists
	var existingReferrer database.Referrer
	if err := database.GetDB().Where("referral_code = ? AND id != ?", req.NewReferralCode, referrer.ID).First(&existingReferrer).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Referral code already taken"})
		return
	}

	// Update referral code
	referrer.ReferralCode = req.NewReferralCode
	if err := database.GetDB().Save(&referrer).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update referral code"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Referral code updated successfully",
		"referrer": referrer,
	})
}

// DeactivateReferrer deactivates a referrer (admin function)
func DeactivateReferrer(c *gin.Context) {
	walletAddress := c.Param("wallet_address")
	if walletAddress == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Wallet address is required"})
		return
	}

	// Get referrer
	var referrer database.Referrer
	if err := database.GetDB().Where("wallet_address = ?", walletAddress).First(&referrer).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Referrer not found"})
		return
	}

	// Deactivate referrer
	referrer.Status = database.ReferralStatusInactive
	if err := database.GetDB().Save(&referrer).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to deactivate referrer"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Referrer deactivated successfully",
		"referrer": referrer,
	})
}
