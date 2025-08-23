package database

import (
	"log"

	"web3-boilerplate/internal/structs"
)

func RegisterUser(user structs.User) error {
	dbUser := User{
		Address:                 user.Address,
		ReferralCode:           user.ReferralCode,
		NotificationPreferences: user.NotificationPreferences,
	}

	result := db.Create(&dbUser)
	return result.Error
}

func GetUserByAddress(address string) (structs.User, error) {
	var dbUser User
	result := db.Where("address = ?", address).First(&dbUser)
	if result.Error != nil {
		return structs.User{}, result.Error
	}

	return structs.User{
		Address:                 dbUser.Address,
		ReferralCode:           dbUser.ReferralCode,
		NotificationPreferences: dbUser.NotificationPreferences,
	}, nil
}

func GetUserByReferralCode(referralCode string) (structs.User, error) {
	var dbUser User
	result := db.Where("referral_code = ?", referralCode).First(&dbUser)
	if result.Error != nil {
		return structs.User{}, result.Error
	}

	return structs.User{
		Address:                 dbUser.Address,
		ReferralCode:           dbUser.ReferralCode,
		NotificationPreferences: dbUser.NotificationPreferences,
	}, nil
}

func GetUserByTokenId(tokenId string) (structs.User, error) {
	var dbUser User
	result := db.Where("token_id = ?", tokenId).First(&dbUser)
	if result.Error != nil {
		return structs.User{}, result.Error
	}

	return structs.User{
		Address:                 dbUser.Address,
		ReferralCode:           dbUser.ReferralCode,
		NotificationPreferences: dbUser.NotificationPreferences,
	}, nil
}

func UpdateUser(user structs.User) error {
	dbUser := User{
		Address:                 user.Address,
		ReferralCode:           user.ReferralCode,
		NotificationPreferences: user.NotificationPreferences,
	}

	result := db.Where("address = ?", user.Address).Updates(&dbUser)
	return result.Error
}

func GetAllUsers() ([]structs.User, error) {
	var dbUsers []User
	result := db.Find(&dbUsers)
	if result.Error != nil {
		return nil, result.Error
	}

	users := make([]structs.User, len(dbUsers))
	for i, dbUser := range dbUsers {
		users[i] = structs.User{
			Address:                 dbUser.Address,
			ReferralCode:           dbUser.ReferralCode,
				NotificationPreferences: dbUser.NotificationPreferences,
		}
	}

	return users, nil
}

func UpdateNotificationPreferences(user structs.User, preferences structs.NotificationPreferences) error {
	user.NotificationPreferences = preferences
	return UpdateUser(user)
}

// SetDefaultNotificationPreferences sets default notification preferences for all users that don't have them set
func SetDefaultNotificationPreferences() error {
	// Default notification preferences with all fields set to true
	defaultPreferences := structs.NotificationPreferences{
		EmailEnabled:         true,
		NewsEnabled:          true,
		UpdatesEnabled:       true,
		TransactionalEnabled: true,
		SecurityEnabled:      true,
		ReportsEnabled:       true,
		StatisticsEnabled:    true,
	}

	// Update all users that have empty notification preferences
	result := db.Model(&User{}).
		Where("notification_preferences_email_enabled = ? OR notification_preferences_email_enabled IS NULL", false).
		Updates(map[string]interface{}{
			"notification_preferences_email_enabled":         defaultPreferences.EmailEnabled,
			"notification_preferences_news_enabled":          defaultPreferences.NewsEnabled,
			"notification_preferences_updates_enabled":       defaultPreferences.UpdatesEnabled,
			"notification_preferences_transactional_enabled": defaultPreferences.TransactionalEnabled,
			"notification_preferences_security_enabled":      defaultPreferences.SecurityEnabled,
			"notification_preferences_reports_enabled":       defaultPreferences.ReportsEnabled,
			"notification_preferences_statistics_enabled":    defaultPreferences.StatisticsEnabled,
		})

	if result.Error != nil {
		return result.Error
	}

	log.Printf("Updated notification preferences for %d users", result.RowsAffected)
	return nil
}
