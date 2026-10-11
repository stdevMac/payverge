package database

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// ErrSpaceRegionNotFound is returned when a region is missing or not owned by the business.
var ErrSpaceRegionNotFound = errors.New("space region not found")

// ListSpaceRegionsBySpace returns regions for a space scoped to businessID.
func ListSpaceRegionsBySpace(businessID, spaceID uint) ([]SpaceRegion, error) {
	var regions []SpaceRegion
	if err := db.Where("business_id = ? AND space_id = ?", businessID, spaceID).
		Order("sort_order ASC, id ASC").Find(&regions).Error; err != nil {
		return nil, fmt.Errorf("list space regions: %w", err)
	}
	return regions, nil
}

// ReplaceSpaceRegionsForSpace deletes existing regions for the space and inserts next.
// Used when publishing a layout that rewrites the region set from JSON.
func ReplaceSpaceRegionsForSpace(businessID, spaceID uint, regions []SpaceRegion) error {
	if _, err := GetRestaurantSpaceByID(businessID, spaceID); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		// Clear table region_id pointers first (FK would SET NULL on delete anyway).
		if err := tx.Model(&Table{}).
			Where("business_id = ? AND space_id = ?", businessID, spaceID).
			Update("region_id", nil).Error; err != nil {
			return err
		}
		if err := tx.Where("business_id = ? AND space_id = ?", businessID, spaceID).
			Delete(&SpaceRegion{}).Error; err != nil {
			return err
		}
		for i := range regions {
			regions[i].ID = 0
			regions[i].BusinessID = businessID
			regions[i].SpaceID = spaceID
			if len(regions[i].PolygonJSON) == 0 {
				regions[i].PolygonJSON = JSONRawMessage(`[]`)
			}
			if err := tx.Create(&regions[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
