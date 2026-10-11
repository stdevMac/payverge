package database

import (
	"time"

	"gorm.io/gorm"
)

// CustomerAddress represents a saved delivery address for a customer
type CustomerAddress struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	CustomerID uint      `gorm:"not null;index" json:"customer_id"`
	Customer   *Customer `gorm:"foreignKey:CustomerID" json:"customer,omitempty"`
	BusinessID *uint     `gorm:"index" json:"business_id,omitempty"`
	Business   *Business `gorm:"foreignKey:BusinessID" json:"business,omitempty"`

	// Address Label
	Label string `gorm:"size:100" json:"label"` // e.g., "Home", "Work", "Mom's House"

	// Address Details
	Street           string `gorm:"size:255;not null" json:"street"`
	Apartment        string `gorm:"size:100" json:"apartment"`
	City             string `gorm:"size:100;not null" json:"city"`
	State            string `gorm:"size:100" json:"state"`
	PostalCode       string `gorm:"size:20" json:"postal_code"`
	Country          string `gorm:"size:100;not null" json:"country"`
	FormattedAddress string `gorm:"type:text" json:"formatted_address"`

	// Location Coordinates
	Latitude  *float64 `gorm:"type:decimal(10,8)" json:"latitude"`
	Longitude *float64 `gorm:"type:decimal(11,8)" json:"longitude"`

	// Delivery Instructions
	DeliveryInstructions string `gorm:"type:text" json:"delivery_instructions"`
	ContactlessDelivery  bool   `gorm:"default:false" json:"contactless_delivery"`
	LeaveAtDoor          bool   `gorm:"default:false" json:"leave_at_door"`

	// Contact Information (optional override)
	ContactName  string `gorm:"size:255" json:"contact_name"`
	ContactPhone string `gorm:"size:50" json:"contact_phone"`

	// Status
	IsDefault bool `gorm:"default:false;index" json:"is_default"`
	IsActive  bool `gorm:"default:true" json:"is_active"`

	// Metadata
	LastUsedAt *time.Time `json:"last_used_at"`
	UsageCount int        `gorm:"default:0" json:"usage_count"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName specifies the table name for CustomerAddress
func (CustomerAddress) TableName() string {
	return "customer_addresses"
}

// BeforeSave ensures only one default address per customer within the same scope.
func (ca *CustomerAddress) BeforeSave(tx *gorm.DB) error {
	if ca.IsDefault {
		query := tx.Model(&CustomerAddress{}).
			Where("customer_id = ? AND id != ?", ca.CustomerID, ca.ID)
		if ca.BusinessID == nil {
			query = query.Where("business_id IS NULL")
		} else {
			query = query.Where("business_id = ?", *ca.BusinessID)
		}

		if err := query.Update("is_default", false).Error; err != nil {
			return err
		}
	}
	return nil
}
