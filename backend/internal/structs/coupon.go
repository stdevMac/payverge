package structs

import "time"

// CouponDetails represents coupon information for API responses
type CouponDetails struct {
	Code           string     `json:"code"`
	Hash           string     `json:"hash"`
	DiscountAmount string     `json:"discountAmount"` // Formatted USDC amount (e.g., "10.50")
	ExpiryTime     *time.Time `json:"expiryTime,omitempty"`
	IsActive       bool       `json:"isActive"`
	IsUsed         bool       `json:"isUsed"`
	UsedBy         string     `json:"usedBy,omitempty"`
	UsedAt         *time.Time `json:"usedAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
}

// CouponCreateRequest represents a request to create a new coupon
type CouponCreateRequest struct {
	Code   string `json:"code" binding:"required" validate:"min=3,max=32"`
	Amount string `json:"amount" binding:"required" validate:"numeric"`
	Expiry string `json:"expiry,omitempty"` // RFC3339 format or empty for no expiry
}

// CouponValidationResponse represents the response for coupon validation
type CouponValidationResponse struct {
	Valid          bool   `json:"valid"`
	Code           string `json:"code,omitempty"`
	DiscountAmount string `json:"discountAmount,omitempty"`
	Hash           string `json:"hash,omitempty"`
	Message        string `json:"message"`
	Status         string `json:"status,omitempty"` // "available", "used", "expired", "inactive", "invalid"
}

// CouponUsageRequest represents a request to mark a coupon as used
type CouponUsageRequest struct {
	Hash   string `json:"hash" binding:"required"`
	UsedBy string `json:"usedBy" binding:"required"`
	TxHash string `json:"txHash,omitempty"`
}

// CouponStats represents coupon usage statistics
type CouponStats struct {
	Total       int64 `json:"total"`
	Active      int64 `json:"active"`
	Used        int64 `json:"used"`
	Expired     int64 `json:"expired"`
	Deactivated int64 `json:"deactivated"`
}

// CouponEvent represents a blockchain event related to coupons
type CouponEvent struct {
	Type           string    `json:"type"` // "created", "used", "deactivated"
	CouponHash     string    `json:"couponHash"`
	Code           string    `json:"code,omitempty"`
	DiscountAmount string    `json:"discountAmount,omitempty"`
	ExpiryTime     *int64    `json:"expiryTime,omitempty"`
	Business       string    `json:"business,omitempty"`
	Timestamp      time.Time `json:"timestamp"`
	BlockNumber    uint64    `json:"blockNumber"`
	TxHash         string    `json:"txHash"`
}
