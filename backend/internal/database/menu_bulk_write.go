package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// ApplyMenuCategoriesTx writes the whole categories tree for a business's active
// menu under optimistic-concurrency control, inside the caller's transaction.
// It only writes when the stored version still equals expectedVersion and bumps
// the version by one atomically. Returns ErrMenuVersionConflict on mismatch.
// Mirrors updateMenuWithVersionCheck but is tx-aware so apply can bundle the
// menu write, audit insert, and proposal status update atomically.
func ApplyMenuCategoriesTx(tx *gorm.DB, businessID uint, categories []MenuCategory, expectedVersion uint) (newVersion uint, err error) {
	var menu Menu
	if err := tx.Where("business_id = ? AND is_active = ?", businessID, true).First(&menu).Error; err != nil {
		return 0, fmt.Errorf("failed to load menu: %w", err)
	}
	ensureMenuEntityIDs(categories)
	categoriesJSON, err := json.Marshal(categories)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal categories: %w", err)
	}
	result := tx.Model(&Menu{}).
		Where("id = ? AND version = ?", menu.ID, expectedVersion).
		Updates(map[string]interface{}{
			"categories": string(categoriesJSON),
			"version":    expectedVersion + 1,
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return 0, fmt.Errorf("failed to update menu: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return 0, ErrMenuVersionConflict
	}
	return expectedVersion + 1, nil
}

// ApplyMenuCategories is the non-transactional convenience wrapper.
func ApplyMenuCategories(businessID uint, categories []MenuCategory, expectedVersion uint) (uint, error) {
	return ApplyMenuCategoriesTx(db, businessID, categories, expectedVersion)
}

// AppendMenuCategories merges newCategories into a business's active menu under
// optimistic concurrency, retrying on a concurrent version bump, and creates the
// menu when none exists. Used by the AI import + wizard rails, which hold no
// client version. Never surfaces ErrMenuVersionConflict to the caller.
func AppendMenuCategories(businessID uint, newCategories []MenuCategory) (uint, error) {
	const maxAttempts = 5
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		menu, existing, err := GetMenuByBusinessID(businessID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				newMenu := &Menu{BusinessID: businessID, IsActive: true, Version: 1}
				if cErr := CreateMenu(newMenu, newCategories); cErr != nil {
					lastErr = cErr
					continue // a racing create -> next loop finds the menu and CAS-merges
				}
				return newMenu.Version, nil
			}
			return 0, err
		}
		merged := append(append([]MenuCategory{}, existing...), newCategories...)
		v, aErr := ApplyMenuCategories(businessID, merged, menu.Version)
		if aErr == nil {
			return v, nil
		}
		if errors.Is(aErr, ErrMenuVersionConflict) {
			lastErr = aErr
			continue
		}
		return 0, aErr
	}
	return 0, lastErr
}
