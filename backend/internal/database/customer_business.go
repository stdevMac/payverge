package database

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// EnsureActiveCustomerBusinessConnection creates or reactivates a customer/business link.
// Loyalty history is preserved when an inactive connection is reactivated.
func EnsureActiveCustomerBusinessConnection(db *gorm.DB, customerID, businessID uint, optInMarketing bool) (*CustomerBusiness, error) {
	var existing CustomerBusiness
	err := db.Where("customer_id = ? AND business_id = ?", customerID, businessID).First(&existing).Error
	if err == nil {
		if existing.IsActive {
			return &existing, nil
		}

		updates := map[string]interface{}{
			"is_active":        true,
			"opt_in_marketing": optInMarketing,
			"opt_in_email":     true,
		}
		if existing.FirstVisitAt.IsZero() {
			updates["first_visit_at"] = time.Now()
		}

		if err := db.Model(&CustomerBusiness{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
			return nil, err
		}
		if err := db.First(&existing, existing.ID).Error; err != nil {
			return nil, err
		}
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	now := time.Now()
	connection := &CustomerBusiness{
		CustomerID:     customerID,
		BusinessID:     businessID,
		LoyaltyPoints:  0,
		TotalSpent:     0,
		VisitCount:     0,
		FirstVisitAt:   now,
		OptInMarketing: optInMarketing,
		OptInEmail:     true,
		IsActive:       true,
	}
	if err := db.Create(connection).Error; err != nil {
		return nil, err
	}

	return connection, nil
}
