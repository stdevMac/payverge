package main

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// tableSpec defines one named table. Capacities reflect a believable
// 19-table North Beach trattoria with a mix of patio, window, bar, booth
// and private dining seating.
type tableSpec struct {
	Code     string
	Name     string
	Capacity int
}

func showcaseTables() []tableSpec {
	return []tableSpec{
		// Outdoor patio (6)
		{"showcase-bellavista-p01", "Patio 1", 2},
		{"showcase-bellavista-p02", "Patio 2", 2},
		{"showcase-bellavista-p03", "Patio 3", 4},
		{"showcase-bellavista-p04", "Patio 4", 4},
		{"showcase-bellavista-p05", "Patio 5", 6},
		{"showcase-bellavista-p06", "Patio 6", 6},
		// Window seats (4)
		{"showcase-bellavista-w01", "Window 1", 2},
		{"showcase-bellavista-w02", "Window 2", 2},
		{"showcase-bellavista-w03", "Window 3", 4},
		{"showcase-bellavista-w04", "Window 4", 4},
		// Bar (4)
		{"showcase-bellavista-b01", "Bar 1", 1},
		{"showcase-bellavista-b02", "Bar 2", 1},
		{"showcase-bellavista-b03", "Bar 3", 1},
		{"showcase-bellavista-b04", "Bar 4", 1},
		// Booths (4)
		{"showcase-bellavista-bt01", "Booth 1", 4},
		{"showcase-bellavista-bt02", "Booth 2", 4},
		{"showcase-bellavista-bt03", "Booth 3", 6},
		{"showcase-bellavista-bt04", "Booth 4", 6},
		// Private dining (1)
		{"showcase-bellavista-pdr", "Private Dining Room", 18},
	}
}

// seedTables upserts the 19 named tables. Dedupe key is the unique
// `table_code` column.
func seedTables(ctx context.Context, db *gorm.DB, bizID uint) error {
	specs := showcaseTables()
	rows := make([]database.Table, 0, len(specs))
	for _, s := range specs {
		rows = append(rows, database.Table{
			BusinessID:         bizID,
			TableCode:          s.Code,
			Name:               s.Name,
			Capacity:           s.Capacity,
			IsActive:           true,
			QRForegroundColor:  "#1a6b6a",
			QRBackgroundColor:  "#faf9f6",
			QRLogoSize:         22,
			QRShowBusinessName: true,
			QRShowTableName:    true,
			QRTextFont:         "Verdana",
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
	return nil
}

// loadTables returns all showcase tables ordered by code so callers can
// reference them by index when constructing reservations/bills.
func loadTables(ctx context.Context, db *gorm.DB, bizID uint) ([]database.Table, error) {
	var tables []database.Table
	if err := db.WithContext(ctx).
		Where("business_id = ? AND table_code LIKE ?", bizID, "showcase-bellavista-%").
		Order("table_code").
		Find(&tables).Error; err != nil {
		return nil, fmt.Errorf("load tables: %w", err)
	}
	return tables, nil
}
