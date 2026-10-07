package database

import (
	"strings"
	"time"
)

// Public storefront hospitality bounds. These are guest-landing caps, not
// operator-editor caps: a published page does not need unbounded history.
const (
	PublicStorefrontGalleryLimit   = 24
	PublicStorefrontHoursLimit     = 14 // 7 days × 2 split-shift periods
	PublicStorefrontExceptionLimit = 366
	PublicStorefrontFeatureLimit   = 24
	PublicStorefrontLanguageLimit  = 32
	PublicSupportedLanguageLimit   = 32
)

var (
	publicStorefrontGalleryColumns = []string{
		"id", "business_id", "image_url", "caption", "display_order", "is_active", "created_at", "updated_at",
	}
	publicStorefrontHoursColumns = []string{
		"id", "business_id", "day_of_week", "open_time", "close_time", "kitchen_close_time", "is_closed", "created_at", "updated_at",
	}
	publicStorefrontExceptionColumns = []string{
		"id", "business_id", "exception_date", "open_time", "close_time", "kitchen_close_time", "is_closed", "label", "created_at", "updated_at",
	}
	publicStorefrontFeatureColumns = []string{
		"id", "business_id", "title", "description", "icon", "display_order", "is_active", "created_at", "updated_at",
	}
	publicStorefrontBusinessLanguageColumns = []string{
		"id", "business_id", "language_code", "is_default", "display_order", "created_at", "updated_at",
	}
	publicStorefrontSupportedLanguageColumns = []string{
		"code", "name", "native_name",
	}
)

func ignoreMissingRelation(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "no such table") ||
		strings.Contains(msg, "does not exist") ||
		strings.Contains(msg, "undefined_table") {
		return nil
	}
	return err
}

func publicOperatingExceptionFrom() time.Time {
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	// Yesterday covers "still today" in timezones west of UTC at late evening.
	return today.AddDate(0, 0, -1)
}

// GetPublicBusinessGalleryImages is the bounded, projected gallery read for
// GET /business/:customUrl. Active rows only.
func GetPublicBusinessGalleryImages(businessID uint) ([]BusinessGalleryImage, error) {
	var images []BusinessGalleryImage
	err := GetDBWrapper().GetGorm().
		Select(publicStorefrontGalleryColumns).
		Where("business_id = ? AND is_active = ?", businessID, true).
		Order("display_order ASC, created_at ASC").
		Limit(PublicStorefrontGalleryLimit).
		Find(&images).Error
	if ignoreMissingRelation(err) != nil {
		return nil, err
	}
	return images, nil
}

// GetPublicBusinessOperatingHours is the bounded, projected weekly-hours read
// for the public storefront.
func GetPublicBusinessOperatingHours(businessID uint) ([]BusinessOperatingHours, error) {
	var hours []BusinessOperatingHours
	err := GetDBWrapper().GetGorm().
		Select(publicStorefrontHoursColumns).
		Where("business_id = ?", businessID).
		Order("day_of_week ASC, open_time ASC").
		Limit(PublicStorefrontHoursLimit).
		Find(&hours).Error
	if ignoreMissingRelation(err) != nil {
		return nil, err
	}
	return hours, nil
}

// GetBusinessOperatingExceptionsFrom returns dated overrides on/after from,
// projected and bounded. Used by the public storefront and reservation
// availability so venue history cannot grow those guest reads forever.
// Operator GetBusinessOperatingExceptions stays unbounded (replace-all editor).
func GetBusinessOperatingExceptionsFrom(businessID uint, from time.Time, limit int) ([]BusinessOperatingException, error) {
	if limit <= 0 {
		limit = PublicStorefrontExceptionLimit
	}
	if limit > PublicStorefrontExceptionLimit {
		limit = PublicStorefrontExceptionLimit
	}
	day := time.Date(from.UTC().Year(), from.UTC().Month(), from.UTC().Day(), 0, 0, 0, 0, time.UTC)
	var rows []BusinessOperatingException
	err := GetDBWrapper().GetGorm().
		Select(publicStorefrontExceptionColumns).
		Where("business_id = ? AND exception_date >= ?", businessID, day).
		Order("exception_date ASC").
		Limit(limit).
		Find(&rows).Error
	if ignoreMissingRelation(err) != nil {
		return nil, err
	}
	if rows == nil {
		rows = []BusinessOperatingException{}
	}
	return rows, nil
}

// GetPublicBusinessOperatingExceptions is the storefront/reservation window:
// yesterday UTC through the next year of dated overrides.
func GetPublicBusinessOperatingExceptions(businessID uint) ([]BusinessOperatingException, error) {
	return GetBusinessOperatingExceptionsFrom(businessID, publicOperatingExceptionFrom(), PublicStorefrontExceptionLimit)
}

// GetPublicBusinessSpecialFeatures is the bounded, projected features read
// for the public storefront. Active rows only.
func GetPublicBusinessSpecialFeatures(businessID uint) ([]BusinessSpecialFeature, error) {
	var features []BusinessSpecialFeature
	err := GetDBWrapper().GetGorm().
		Select(publicStorefrontFeatureColumns).
		Where("business_id = ? AND is_active = ?", businessID, true).
		Order("display_order ASC, created_at ASC").
		Limit(PublicStorefrontFeatureLimit).
		Find(&features).Error
	if ignoreMissingRelation(err) != nil {
		return nil, err
	}
	return features, nil
}
