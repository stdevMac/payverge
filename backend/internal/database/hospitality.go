package database

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Gallery Images Functions

// GetBusinessGalleryImages retrieves active gallery images for a business.
// Optional limit (<=0 = legacy unbounded; otherwise clamped to [1,100]).
func GetBusinessGalleryImages(businessID uint, limit ...int) ([]BusinessGalleryImage, error) {
	db := GetDBWrapper()
	var images []BusinessGalleryImage

	q := db.GetGorm().Where("business_id = ? AND is_active = ?", businessID, true).
		Order("display_order ASC, created_at ASC")
	if len(limit) > 0 && limit[0] > 0 {
		lim := limit[0]
		if lim > 100 {
			lim = 100
		}
		q = q.Limit(lim)
	}

	err := q.Find(&images).Error
	return images, err
}

// GalleryUpdateResult reports what the gallery write path did so callers can
// skip the expensive caption translation fan-out when captions are unchanged.
type GalleryUpdateResult struct {
	// CaptionsChanged is true when any preserved row's caption text changed,
	// or when a new row was inserted with a non-empty caption.
	CaptionsChanged bool
	// PreservedIDs are existing row IDs that were updated in place (not recreated).
	PreservedIDs []uint
	// DeletedIDs are rows removed because they were absent from the incoming set.
	DeletedIDs []uint
	// CreatedIDs are newly inserted row IDs.
	CreatedIDs []uint
}

// UpdateBusinessGalleryImages upserts gallery images for a business.
// Unchanged rows keep their primary keys (so ID-keyed caption translations
// stay valid). Rows missing from the incoming set are deleted. Display order
// is taken from the slice position.
func UpdateBusinessGalleryImages(businessID uint, images []BusinessGalleryImage) (GalleryUpdateResult, error) {
	var result GalleryUpdateResult
	db := GetDBWrapper()

	tx := db.GetGorm().Begin()
	if tx.Error != nil {
		return result, tx.Error
	}

	var existing []BusinessGalleryImage
	if err := tx.Where("business_id = ?", businessID).Find(&existing).Error; err != nil {
		tx.Rollback()
		return result, err
	}

	byID := make(map[uint]BusinessGalleryImage, len(existing))
	// URL index for clients that omit IDs (legacy full-replace payloads).
	byURL := make(map[string]BusinessGalleryImage, len(existing))
	for _, row := range existing {
		byID[row.ID] = row
		// First wins if duplicate URLs exist; rare and acceptable for matching.
		if _, ok := byURL[row.ImageURL]; !ok {
			byURL[row.ImageURL] = row
		}
	}

	matched := make(map[uint]struct{}, len(images))
	now := time.Now()

	for i, incoming := range images {
		incoming.BusinessID = businessID
		incoming.DisplayOrder = i
		incoming.UpdatedAt = now

		var prior BusinessGalleryImage
		var found bool
		if incoming.ID != 0 {
			prior, found = byID[incoming.ID]
			if found && prior.BusinessID != businessID {
				found = false
			}
		}
		if !found {
			if prior, found = byURL[incoming.ImageURL]; found {
				if _, already := matched[prior.ID]; already {
					found = false
				}
			}
		}

		if found {
			matched[prior.ID] = struct{}{}
			if prior.Caption != incoming.Caption {
				result.CaptionsChanged = true
			}
			// Update in place — preserve CreatedAt and primary key.
			updates := map[string]interface{}{
				"image_url":     incoming.ImageURL,
				"caption":       incoming.Caption,
				"display_order": i,
				"is_active":     incoming.IsActive,
				"updated_at":    now,
			}
			if err := tx.Model(&BusinessGalleryImage{}).Where("id = ? AND business_id = ?", prior.ID, businessID).Updates(updates).Error; err != nil {
				tx.Rollback()
				return result, err
			}
			result.PreservedIDs = append(result.PreservedIDs, prior.ID)
			continue
		}

		// New row.
		row := BusinessGalleryImage{
			BusinessID:   businessID,
			ImageURL:     incoming.ImageURL,
			Caption:      incoming.Caption,
			DisplayOrder: i,
			IsActive:     incoming.IsActive,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := tx.Create(&row).Error; err != nil {
			tx.Rollback()
			return result, err
		}
		result.CreatedIDs = append(result.CreatedIDs, row.ID)
		if row.Caption != "" {
			result.CaptionsChanged = true
		}
	}

	// Delete rows not present in the incoming set.
	for _, row := range existing {
		if _, ok := matched[row.ID]; ok {
			continue
		}
		if err := tx.Where("id = ? AND business_id = ?", row.ID, businessID).Delete(&BusinessGalleryImage{}).Error; err != nil {
			tx.Rollback()
			return result, err
		}
		result.DeletedIDs = append(result.DeletedIDs, row.ID)
	}

	if err := tx.Commit().Error; err != nil {
		return result, err
	}
	return result, nil
}

// Operating Hours Functions

// GetBusinessOperatingHours retrieves all operating hours for a business
func GetBusinessOperatingHours(businessID uint) ([]BusinessOperatingHours, error) {
	db := GetDBWrapper()
	var hours []BusinessOperatingHours

	err := db.GetGorm().Where("business_id = ?", businessID).
		Order("day_of_week ASC, open_time ASC").Find(&hours).Error

	return hours, err
}

// ValidateBusinessOperatingHours accepts 0–2 open periods per day (split
// shifts such as lunch 11–15 + dinner 19–23) or a single closed row. A day
// cannot mix is_closed with open periods.
func ValidateBusinessOperatingHours(hours []BusinessOperatingHours) error {
	type dayBucket struct {
		closed int
		open   int
	}
	byDay := make(map[int]*dayBucket, 7)
	for _, hour := range hours {
		if hour.DayOfWeek < 0 || hour.DayOfWeek > 6 {
			return fmt.Errorf("day_of_week must be between 0 and 6")
		}
		b := byDay[hour.DayOfWeek]
		if b == nil {
			b = &dayBucket{}
			byDay[hour.DayOfWeek] = b
		}
		if hour.IsClosed {
			b.closed++
		} else {
			b.open++
		}
	}
	for day, b := range byDay {
		if b.closed > 0 && b.open > 0 {
			return fmt.Errorf("closed day cannot also have open periods for day_of_week %d", day)
		}
		if b.closed > 1 {
			return fmt.Errorf("duplicate closed row for day_of_week %d", day)
		}
		if b.open > 2 {
			return fmt.Errorf("at most 2 open periods per day (day_of_week %d)", day)
		}
	}
	return nil
}

// UpdateBusinessOperatingHours replaces all operating hours for a business
func UpdateBusinessOperatingHours(businessID uint, hours []BusinessOperatingHours) error {
	if err := ValidateBusinessOperatingHours(hours); err != nil {
		return err
	}

	db := GetDBWrapper()

	// Start transaction
	tx := db.GetGorm().Begin()
	if tx.Error != nil {
		return tx.Error
	}

	// Delete existing hours
	if err := tx.Where("business_id = ?", businessID).Delete(&BusinessOperatingHours{}).Error; err != nil {
		tx.Rollback()
		return err
	}

	// Insert new hours
	for _, hour := range hours {
		hour.BusinessID = businessID
		hour.CreatedAt = time.Now()
		hour.UpdatedAt = time.Now()
		if err := tx.Create(&hour).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit().Error
}

// GetBusinessOperatingHoursByDay retrieves all operating-hour rows for a day
// (0–2 open periods for split shifts, or a single closed row). Returns
// gorm.ErrRecordNotFound when the day has no rows.
func GetBusinessOperatingHoursByDay(businessID uint, dayOfWeek int) ([]BusinessOperatingHours, error) {
	db := GetDBWrapper()
	var hours []BusinessOperatingHours

	err := db.GetGorm().Where("business_id = ? AND day_of_week = ?", businessID, dayOfWeek).
		Order("open_time ASC").Find(&hours).Error
	if err != nil {
		return nil, err
	}
	if len(hours) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return hours, nil
}

// GetBusinessOperatingExceptions returns all dated overrides for a business,
// ordered by calendar date ascending.
func GetBusinessOperatingExceptions(businessID uint) ([]BusinessOperatingException, error) {
	db := GetDBWrapper()
	var rows []BusinessOperatingException
	err := db.GetGorm().Where("business_id = ?", businessID).
		Order("exception_date ASC").Find(&rows).Error
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "no such table") ||
			strings.Contains(msg, "does not exist") ||
			strings.Contains(msg, "undefined_table") {
			return []BusinessOperatingException{}, nil
		}
	}
	return rows, err
}

// GetBusinessOperatingExceptionForDate returns the override for a calendar
// date in the business timezone day (date-only). Returns gorm.ErrRecordNotFound
// when no exception exists (including when the exceptions table has not been
// migrated yet — callers should fall through to the weekly hours grid).
func GetBusinessOperatingExceptionForDate(businessID uint, date time.Time) (*BusinessOperatingException, error) {
	db := GetDBWrapper()
	day := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	var row BusinessOperatingException
	err := db.GetGorm().Where("business_id = ? AND exception_date = ?", businessID, day).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		// Pre-migration / lean test DBs: treat missing relation as "no exception".
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "no such table") ||
			strings.Contains(msg, "does not exist") ||
			strings.Contains(msg, "undefined_table") {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &row, nil
}

// ValidateBusinessOperatingExceptions checks date uniqueness and closed/open shape.
func ValidateBusinessOperatingExceptions(rows []BusinessOperatingException) error {
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		key := row.ExceptionDate.UTC().Format("2006-01-02")
		if key == "0001-01-01" {
			return fmt.Errorf("exception_date is required")
		}
		if _, ok := seen[key]; ok {
			return fmt.Errorf("duplicate exception_date %s", key)
		}
		seen[key] = struct{}{}
		if !row.IsClosed {
			if row.OpenTime == nil || strings.TrimSpace(*row.OpenTime) == "" {
				return fmt.Errorf("open_time is required for open exception on %s", key)
			}
			if row.CloseTime == nil || strings.TrimSpace(*row.CloseTime) == "" {
				return fmt.Errorf("close_time is required for open exception on %s", key)
			}
		}
	}
	return nil
}

// UpdateBusinessOperatingExceptions replaces all dated overrides for a business.
func UpdateBusinessOperatingExceptions(businessID uint, rows []BusinessOperatingException) error {
	if err := ValidateBusinessOperatingExceptions(rows); err != nil {
		return err
	}
	db := GetDBWrapper()
	tx := db.GetGorm().Begin()
	if tx.Error != nil {
		return tx.Error
	}
	if err := tx.Where("business_id = ?", businessID).Delete(&BusinessOperatingException{}).Error; err != nil {
		tx.Rollback()
		return err
	}
	now := time.Now()
	for _, row := range rows {
		row.ID = 0
		row.BusinessID = businessID
		row.ExceptionDate = time.Date(
			row.ExceptionDate.Year(), row.ExceptionDate.Month(), row.ExceptionDate.Day(),
			0, 0, 0, 0, time.UTC,
		)
		row.CreatedAt = now
		row.UpdatedAt = now
		if err := tx.Create(&row).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit().Error
}

// Special Features Functions

// GetBusinessSpecialFeatures retrieves all active special features for a business
func GetBusinessSpecialFeatures(businessID uint) ([]BusinessSpecialFeature, error) {
	db := GetDBWrapper()
	var features []BusinessSpecialFeature

	err := db.GetGorm().Where("business_id = ? AND is_active = ?", businessID, true).
		Order("display_order ASC, created_at ASC").Find(&features).Error

	return features, err
}

// UpdateBusinessSpecialFeatures replaces all special features for a business
func UpdateBusinessSpecialFeatures(businessID uint, features []BusinessSpecialFeature) error {
	db := GetDBWrapper()

	// Start transaction
	tx := db.GetGorm().Begin()
	if tx.Error != nil {
		return tx.Error
	}

	// Delete existing features
	if err := tx.Where("business_id = ?", businessID).Delete(&BusinessSpecialFeature{}).Error; err != nil {
		tx.Rollback()
		return err
	}

	// Insert new features
	for i, feature := range features {
		feature.BusinessID = businessID
		feature.DisplayOrder = i
		feature.CreatedAt = time.Now()
		feature.UpdatedAt = time.Now()
		if err := tx.Create(&feature).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit().Error
}

// Design Settings Functions

// Hospitality Settings Functions

// Utility Functions
