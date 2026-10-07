package main

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// seedLoyalty installs a 4-tier program: Welcome → Cucina → Famiglia →
// Maestro. Stored as int64 cents; the tier model marshals them as
// float64 dollars on the wire.
func seedLoyalty(ctx context.Context, db *gorm.DB, bizID uint) error {
	var existing database.LoyaltyProgram
	err := db.WithContext(ctx).
		Where("business_id = ?", bizID).
		First(&existing).Error
	if err == nil {
		return nil
	}
	if err != gorm.ErrRecordNotFound {
		return fmt.Errorf("query loyalty program: %w", err)
	}

	program := database.LoyaltyProgram{
		BusinessID:                bizID,
		Enabled:                   true,
		PointsPerDollar:           1,   // earn 1 pt per $1
		RedemptionPointsPerDollar: 100, // 100 pts = $1 discount (~1% cashback)
		Tiers: []database.LoyaltyTier{
			{Name: "Welcome", MinLifetimeSpentCents: 0, SortOrder: 0, Color: "#94a3b8"},
			{Name: "Cucina", MinLifetimeSpentCents: 20000, SortOrder: 1, Color: "#1a6b6a"},
			{Name: "Famiglia", MinLifetimeSpentCents: 75000, SortOrder: 2, Color: "#c0392b"},
			{Name: "Maestro", MinLifetimeSpentCents: 200000, SortOrder: 3, Color: "#b7791f"},
		},
	}
	if err := db.WithContext(ctx).Create(&program).Error; err != nil {
		return fmt.Errorf("create loyalty program: %w", err)
	}
	return nil
}
