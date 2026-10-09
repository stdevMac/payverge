package database

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// ErrSpaceLayoutElementNotFound is returned for missing/foreign layout elements.
var ErrSpaceLayoutElementNotFound = errors.New("space layout element not found")

// ListSpaceLayoutElements lists elements for a space; when draftOnly is non-nil,
// filters by is_draft.
func ListSpaceLayoutElements(businessID, spaceID uint, draftOnly *bool) ([]SpaceLayoutElement, error) {
	var els []SpaceLayoutElement
	q := db.Where("business_id = ? AND space_id = ?", businessID, spaceID)
	if draftOnly != nil {
		q = q.Where("is_draft = ?", *draftOnly)
	}
	if err := q.Order("z_index ASC, id ASC").Find(&els).Error; err != nil {
		return nil, fmt.Errorf("list space layout elements: %w", err)
	}
	return els, nil
}

// ReplaceDraftLayoutElements replaces all draft elements for a space.
func ReplaceDraftLayoutElements(businessID, spaceID uint, elements []SpaceLayoutElement) error {
	if _, err := GetRestaurantSpaceByID(businessID, spaceID); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("business_id = ? AND space_id = ? AND is_draft = ?", businessID, spaceID, true).
			Delete(&SpaceLayoutElement{}).Error; err != nil {
			return err
		}
		for i := range elements {
			elements[i].ID = 0
			elements[i].BusinessID = businessID
			elements[i].SpaceID = spaceID
			elements[i].IsDraft = true
			if len(elements[i].GeometryJSON) == 0 {
				elements[i].GeometryJSON = JSONRawMessage(`{}`)
			}
			if len(elements[i].MetaJSON) == 0 {
				elements[i].MetaJSON = JSONRawMessage(`{}`)
			}
			if err := tx.Create(&elements[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
