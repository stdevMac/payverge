package main

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// seedOffers installs a handful of running promotions so the Offers tab is
// populated. Idempotent on (business_id, name).
func seedOffers(ctx context.Context, db *gorm.DB, bizID uint) error {
	now := time.Now().UTC()
	weekFromNow := now.Add(7 * 24 * time.Hour)
	monthFromNow := now.Add(30 * 24 * time.Hour)
	monthAgo := now.Add(-30 * 24 * time.Hour)

	specs := []database.Offer{
		{
			BusinessID:    bizID,
			Name:          "Aperitivo Hour",
			Description:   "Aperol Spritz + Bruschetta for $20. Mon-Thu 4-6pm.",
			DiscountType:  "fixed",
			DiscountValue: 5.00,
			StartDate:     &monthAgo,
			EndDate:       &monthFromNow,
			IsActive:      true,
			ApplicableTo:  "category",
		},
		{
			BusinessID:    bizID,
			Name:          "Famiglia Sunday",
			Description:   "10% off Tuscan family-style dinner. Sundays from 5pm.",
			DiscountType:  "percentage",
			DiscountValue: 10.0,
			StartDate:     &monthAgo,
			EndDate:       &monthFromNow,
			IsActive:      true,
			ApplicableTo:  "all",
		},
		{
			BusinessID:    bizID,
			Name:          "Birthday Tiramisù",
			Description:   "Complimentary tiramisù on your birthday. Show ID at the table.",
			DiscountType:  "percentage",
			DiscountValue: 100.0,
			StartDate:     &monthAgo,
			EndDate:       &weekFromNow,
			IsActive:      true,
			ApplicableTo:  "item",
		},
	}

	for _, o := range specs {
		var existing database.Offer
		err := db.WithContext(ctx).
			Where("business_id = ? AND name = ?", bizID, o.Name).
			First(&existing).Error
		if err == nil {
			continue
		}
		if err != gorm.ErrRecordNotFound {
			return fmt.Errorf("query offer %q: %w", o.Name, err)
		}
		if err := db.WithContext(ctx).Create(&o).Error; err != nil {
			return fmt.Errorf("create offer %q: %w", o.Name, err)
		}
	}
	return nil
}
