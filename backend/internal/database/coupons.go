package database

import (
	"time"
	"gorm.io/gorm"
)

// Coupon represents a discount coupon in the database
type Coupon struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	Code           string     `gorm:"uniqueIndex;not null" json:"code"`
	Hash           string     `gorm:"uniqueIndex;not null" json:"hash"` // keccak256 hash for smart contract
	DiscountAmount string     `gorm:"not null" json:"discountAmount"`   // Stored as string to preserve precision
	ExpiryTime     *time.Time `json:"expiryTime,omitempty"`
	IsActive       bool       `gorm:"default:true" json:"isActive"`
	IsUsed         bool       `gorm:"default:false" json:"isUsed"`
	UsedBy         string     `json:"usedBy,omitempty"`         // Ethereum address of user who used it
	UsedAt         *time.Time `json:"usedAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// SaveCoupon creates a new coupon in the database
func SaveCoupon(coupon *Coupon) error {
	result := db.Create(coupon)
	return result.Error
}

// GetCouponByCode retrieves a coupon by its code
func GetCouponByCode(code string) (*Coupon, error) {
	var coupon Coupon
	result := db.Where("code = ?", code).First(&coupon)
	if result.Error != nil {
		return nil, result.Error
	}
	return &coupon, nil
}

// GetCouponByHash retrieves a coupon by its hash
func GetCouponByHash(hash string) (*Coupon, error) {
	var coupon Coupon
	result := db.Where("hash = ?", hash).First(&coupon)
	if result.Error != nil {
		return nil, result.Error
	}
	return &coupon, nil
}

// GetAllCoupons retrieves all coupons from the database
func GetAllCoupons() ([]Coupon, error) {
	var coupons []Coupon
	result := db.Order("created_at DESC").Find(&coupons)
	if result.Error != nil {
		return nil, result.Error
	}
	return coupons, nil
}

// GetActiveCoupons retrieves all active (not used, not expired, active) coupons
func GetActiveCoupons() ([]Coupon, error) {
	var coupons []Coupon
	now := time.Now()
	
	result := db.Where("is_active = ? AND is_used = ? AND (expiry_time IS NULL OR expiry_time > ?)", 
		true, false, now).Order("created_at DESC").Find(&coupons)
	
	if result.Error != nil {
		return nil, result.Error
	}
	return coupons, nil
}

// GetCouponsByStatus retrieves coupons by their status
func GetCouponsByStatus(isActive, isUsed bool) ([]Coupon, error) {
	var coupons []Coupon
	result := db.Where("is_active = ? AND is_used = ?", isActive, isUsed).
		Order("created_at DESC").Find(&coupons)
	
	if result.Error != nil {
		return nil, result.Error
	}
	return coupons, nil
}

// GetExpiredCoupons retrieves all expired coupons
func GetExpiredCoupons() ([]Coupon, error) {
	var coupons []Coupon
	now := time.Now()
	
	result := db.Where("expiry_time IS NOT NULL AND expiry_time <= ?", now).
		Order("created_at DESC").Find(&coupons)
	
	if result.Error != nil {
		return nil, result.Error
	}
	return coupons, nil
}

// UpdateCoupon updates an existing coupon
func UpdateCoupon(coupon *Coupon) error {
	coupon.UpdatedAt = time.Now()
	result := db.Save(coupon)
	return result.Error
}

// DeleteCoupon soft deletes a coupon
func DeleteCoupon(id uint) error {
	result := db.Delete(&Coupon{}, id)
	return result.Error
}

// DeleteCouponByCode soft deletes a coupon by code
func DeleteCouponByCode(code string) error {
	result := db.Where("code = ?", code).Delete(&Coupon{})
	return result.Error
}

// MarkCouponAsUsed marks a coupon as used by a specific address
func MarkCouponAsUsed(hash, usedBy string) error {
	now := time.Now()
	result := db.Model(&Coupon{}).Where("hash = ?", hash).Updates(map[string]interface{}{
		"is_used":    true,
		"used_by":    usedBy,
		"used_at":    &now,
		"updated_at": now,
	})
	return result.Error
}

// DeactivateCoupon marks a coupon as inactive
func DeactivateCoupon(code string) error {
	now := time.Now()
	result := db.Model(&Coupon{}).Where("code = ?", code).Updates(map[string]interface{}{
		"is_active":  false,
		"updated_at": now,
	})
	return result.Error
}

// GetCouponUsageStats returns usage statistics for coupons
func GetCouponUsageStats() (map[string]interface{}, error) {
	var stats struct {
		Total      int64 `json:"total"`
		Active     int64 `json:"active"`
		Used       int64 `json:"used"`
		Expired    int64 `json:"expired"`
		Deactivated int64 `json:"deactivated"`
	}

	// Total coupons
	if err := db.Model(&Coupon{}).Count(&stats.Total).Error; err != nil {
		return nil, err
	}

	// Active coupons (active, not used, not expired)
	now := time.Now()
	if err := db.Model(&Coupon{}).Where("is_active = ? AND is_used = ? AND (expiry_time IS NULL OR expiry_time > ?)", 
		true, false, now).Count(&stats.Active).Error; err != nil {
		return nil, err
	}

	// Used coupons
	if err := db.Model(&Coupon{}).Where("is_used = ?", true).Count(&stats.Used).Error; err != nil {
		return nil, err
	}

	// Expired coupons
	if err := db.Model(&Coupon{}).Where("expiry_time IS NOT NULL AND expiry_time <= ?", now).Count(&stats.Expired).Error; err != nil {
		return nil, err
	}

	// Deactivated coupons
	if err := db.Model(&Coupon{}).Where("is_active = ?", false).Count(&stats.Deactivated).Error; err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"total":       stats.Total,
		"active":      stats.Active,
		"used":        stats.Used,
		"expired":     stats.Expired,
		"deactivated": stats.Deactivated,
	}, nil
}

// CleanupExpiredCoupons can be used to clean up old expired coupons (optional)
func CleanupExpiredCoupons(olderThanDays int) error {
	cutoffDate := time.Now().AddDate(0, 0, -olderThanDays)
	
	result := db.Where("expiry_time IS NOT NULL AND expiry_time <= ?", cutoffDate).Delete(&Coupon{})
	return result.Error
}
