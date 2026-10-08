package auth

import (
	"log"

	"gorm.io/gorm"
)

// MigrateExistingUsers migrates existing wallet-based users to the new auth system
// This creates UserAuth records for users who signed up with wallets
func MigrateExistingUsers(db *gorm.DB) error {
	log.Println("[Auth] Migrating existing wallet users to new auth system...")

	// Find all users with wallet addresses but no corresponding UserAuth record
	type UserWithAddress struct {
		ID      uint
		Address string
		Email   string
	}

	var users []UserWithAddress
	if err := db.Table("users").
		Select("id, address, email").
		Where("address != '' AND address IS NOT NULL").
		Find(&users).Error; err != nil {
		log.Printf("[Auth] Failed to fetch users for migration: %v", err)
		return err
	}

	migrated := 0
	for _, user := range users {
		// Check if auth record already exists
		var existingAuth UserAuth
		err := db.Where("user_id = ? AND provider = ?", user.ID, "wallet").First(&existingAuth).Error
		if err == nil {
			// Already migrated
			continue
		}

		// Create wallet auth record
		auth := UserAuth{
			UserID:        user.ID,
			Provider:      string(AuthProviderWallet),
			WalletAddress: user.Address,
			EmailVerified: user.Email != "", // If they have email, consider it verified
		}

		if err := db.Create(&auth).Error; err != nil {
			log.Printf("[Auth] Failed to create auth record for user %d: %v", user.ID, err)
			continue
		}

		migrated++
	}

	log.Printf("[Auth] Migrated %d existing wallet users to new auth system", migrated)
	return nil
}
