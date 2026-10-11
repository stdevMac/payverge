package main

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// tableCode returns the deterministic TableCode for table m of business i.
func tableCode(bizIdx, tableIdx int) string {
	return fmt.Sprintf("perf-seed-%03d-t%02d", bizIdx, tableIdx)
}

// seedTables ensures every perf business has at least one table so historical
// bills have a valid TableID to attach to. We seed exactly one table per
// business (sufficient for the bench/k6 traffic shapes).
func seedTables(ctx context.Context, db *gorm.DB, n int) error {
	biz, err := loadSeededBusinesses(ctx, db)
	if err != nil {
		return err
	}
	if len(biz) == 0 {
		return fmt.Errorf("no businesses to attach tables to (seedBusinesses must run first)")
	}

	rows := make([]database.Table, 0, len(biz))
	for i, b := range biz {
		idx := i + 1 // 1-indexed for human-readability
		rows = append(rows, database.Table{
			BusinessID:        b.ID,
			TableCode:         tableCode(idx, 1),
			Name:              "T01",
			Capacity:          4,
			IsActive:          true,
			QRForegroundColor: "#000000",
			QRBackgroundColor: "#FFFFFF",
			QRLogoSize:        20,
			QRTextFont:        "Verdana",
		})
	}

	if err := db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "table_code"}},
			DoNothing: true,
		}).
		CreateInBatches(rows, 100).Error; err != nil {
		return fmt.Errorf("create tables: %w", err)
	}
	_ = n
	return nil
}

// loadSeededTables returns one Table per perf business, ordered by business_id.
// Re-reads after seedTables so callers see the DB-assigned IDs.
func loadSeededTables(ctx context.Context, db *gorm.DB) (map[uint]database.Table, error) {
	biz, err := loadSeededBusinesses(ctx, db)
	if err != nil {
		return nil, err
	}
	ids := make([]uint, 0, len(biz))
	for _, b := range biz {
		ids = append(ids, b.ID)
	}
	var tables []database.Table
	if err := db.WithContext(ctx).
		Where("business_id IN ?", ids).
		Where("table_code LIKE ?", "perf-seed-%").
		Order("business_id, table_code").
		Find(&tables).Error; err != nil {
		return nil, fmt.Errorf("load tables: %w", err)
	}
	out := make(map[uint]database.Table, len(tables))
	for _, t := range tables {
		// First-write-wins (the one matching `-t01`).
		if _, ok := out[t.BusinessID]; !ok {
			out[t.BusinessID] = t
		}
	}
	return out, nil
}
