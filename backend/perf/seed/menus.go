package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// menuItemID returns the deterministic ID for menu item m of business i.
// Order seeders re-derive these IDs to reference them as OrderItem.MenuItemID.
func menuItemID(bizIdx, itemIdx int) string {
	return fmt.Sprintf("perf-seed-%03d-item-%03d", bizIdx, itemIdx)
}

// menuPriceForItem returns a stable price in USD for item m of business i.
// Deterministic per (bizIdx, itemIdx) so re-runs don't drift; rounded to
// cents so the int64-cents conversion in orders.go is exact.
func menuPriceForItem(bizIdx, itemIdx int) float64 {
	// Hash-mix the indices into a per-item rng so prices vary but are stable.
	r := rand.New(rand.NewSource(int64(bizIdx*10_000 + itemIdx)))
	cents := 500 + r.Intn(2500) // 5.00 .. 30.00 USD in 1-cent increments
	return float64(cents) / 100.0
}

// seedMenus ensures every perf business has exactly one menu row whose
// Categories column holds a JSON array with itemsPerBiz MenuItems.
// FirstOrCreate dedupes by BusinessID — existing menus are NOT overwritten.
func seedMenus(ctx context.Context, db *gorm.DB, n, itemsPerBiz int) error {
	biz, err := loadSeededBusinesses(ctx, db)
	if err != nil {
		return err
	}

	for i, b := range biz {
		idx := i + 1 // 1-indexed

		items := make([]database.MenuItem, 0, itemsPerBiz)
		for j := 1; j <= itemsPerBiz; j++ {
			items = append(items, database.MenuItem{
				ID:          menuItemID(idx, j),
				Name:        fmt.Sprintf("Item %d", j),
				Description: fmt.Sprintf("Perf seed item %d for business %d", j, idx),
				Price:       menuPriceForItem(idx, j),
				Currency:    "USD",
				IsAvailable: true,
				SortOrder:   j,
			})
		}
		categories := []database.MenuCategory{{
			ID:          fmt.Sprintf("perf-seed-%03d-cat-001", idx),
			Name:        "Main",
			Description: "Perf seed category",
			Items:       items,
			SortOrder:   1,
		}}
		catsJSON, err := json.Marshal(categories)
		if err != nil {
			return fmt.Errorf("marshal categories biz=%d: %w", idx, err)
		}

		// FirstOrCreate keys on BusinessID — existing menu rows pass through
		// unchanged, so a re-run does not flip prices for already-seeded data.
		menu := database.Menu{}
		if err := db.WithContext(ctx).
			Where(database.Menu{BusinessID: b.ID}).
			Attrs(database.Menu{
				Categories: string(catsJSON),
				IsActive:   true,
				Version:    1,
			}).
			FirstOrCreate(&menu).Error; err != nil {
			return fmt.Errorf("upsert menu biz=%d: %w", idx, err)
		}
	}
	_ = n
	return nil
}
