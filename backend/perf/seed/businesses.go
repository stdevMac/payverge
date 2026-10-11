package main

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// businessSeedID returns the deterministic BusinessId / CustomURL slug for
// the i-th seeded business (1-indexed for human-readability).
func businessSeedID(i int) string {
	return fmt.Sprintf("perf-seed-%03d", i)
}

// seedBusinesses upserts n businesses keyed by business_id. Existing rows are
// left untouched so re-running the CLI is safe.
func seedBusinesses(ctx context.Context, db *gorm.DB, n int) error {
	rows := make([]database.Business, 0, n)
	for i := 1; i <= n; i++ {
		seedID := businessSeedID(i)
		rows = append(rows, database.Business{
			BusinessId:          seedID,
			CustomURL:           seedID,
			OwnerAddress:        PlaceholderAddress,
			Name:                fmt.Sprintf("Perf Seed Restaurant %d", i),
			SettlementAddr:      PlaceholderAddress,
			TippingAddr:         PlaceholderAddress,
			Email:               fmt.Sprintf("perf-seed-%03d@example.test", i),
			IsActive:            true,
			DefaultCurrency:     "USD",
			DisplayCurrency:     "USD",
			DefaultLanguage:     "en",
			SourceLanguage:      "en",
			BusinessPageEnabled: true,
			Timezone:            "UTC",
			OnboardingState:     database.JSONRawMessage([]byte(`{}`)),
		})
	}
	// OnConflict on the unique `business_id` column keeps existing rows.
	if err := db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "business_id"}},
			DoNothing: true,
		}).
		CreateInBatches(rows, 100).Error; err != nil {
		return fmt.Errorf("create businesses: %w", err)
	}
	return nil
}

// loadSeededBusinesses returns the perf-seed businesses ordered by BusinessId
// so callers can index by (i-1) to recover the same i used at insert time.
func loadSeededBusinesses(ctx context.Context, db *gorm.DB) ([]database.Business, error) {
	var biz []database.Business
	if err := db.WithContext(ctx).
		Where("business_id LIKE ?", "perf-seed-%").
		Order("business_id").
		Find(&biz).Error; err != nil {
		return nil, fmt.Errorf("load businesses: %w", err)
	}
	return biz, nil
}
