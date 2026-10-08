package database

import "gorm.io/gorm/clause"

// CreateBundle creates a new bundle
func CreateBundle(bundle *Bundle) error {
	return db.Create(bundle).Error
}

// GetBundlesByBusinessID retrieves all bundles for a business
func GetBundlesByBusinessID(businessID uint) ([]Bundle, error) {
	var bundles []Bundle
	err := db.Where("business_id = ?", businessID).Find(&bundles).Error
	return bundles, err
}

// GetActiveBundlesByBusinessID retrieves active bundles for a business.
func GetActiveBundlesByBusinessID(businessID uint) ([]Bundle, error) {
	var bundles []Bundle
	err := db.Where("business_id = ? AND is_active = ?", businessID, true).Find(&bundles).Error
	return bundles, err
}

// GetBundleByID retrieves a bundle by ID
func GetBundleByID(id uint) (*Bundle, error) {
	var bundle Bundle
	err := db.First(&bundle, id).Error
	if err != nil {
		return nil, err
	}
	return &bundle, nil
}

// UpdateBundle updates an existing bundle
func UpdateBundle(bundle *Bundle) error {
	return db.Omit(clause.Associations).Save(bundle).Error
}

// DeleteBundle deletes a bundle
func DeleteBundle(id uint) error {
	return db.Delete(&Bundle{}, id).Error
}
