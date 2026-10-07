package database

import (
	"errors"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/structs"

	"gorm.io/gorm"
)

func RegisterUser(user structs.User) error {
	dbUser := User{
		Address:                 user.Address,
		Role:                    string(user.Role),
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
		Email:                   dbUser.Email,
		Username:                dbUser.Username,
		Role:                    structs.Role(dbUser.Role),
		NotificationPreferences: dbUser.NotificationPreferences,
		LanguageSelected:        dbUser.LanguageSelected,
	}, nil
}

func GetUserByID(id uint) (structs.User, error) {
	var dbUser User
	result := db.First(&dbUser, id)
	if result.Error != nil {
		return structs.User{}, result.Error
	}

	return structs.User{
		Address:                 dbUser.Address,
		Email:                   dbUser.Email,
		Name:                    dbUser.Name,
		AuthMethod:              dbUser.AuthMethod,
		Role:                    structs.Role(dbUser.Role),
		NotificationPreferences: dbUser.NotificationPreferences,
		LanguageSelected:        dbUser.LanguageSelected,
	}, nil
}

func UpdateUser(user structs.User) error {
	dbUser := User{
		Address:                 user.Address,
		Role:                    string(user.Role),
		NotificationPreferences: user.NotificationPreferences,
	}

	result := db.Where("address = ?", user.Address).Updates(&dbUser)
	return result.Error
}

// UpdateUserProfileByAddress applies the self-service correction fields that
// do not require a separate identity-verification flow. Email changes remain
// outside this helper because changing a login identifier requires verification.
func UpdateUserProfileByAddress(address, username string) error {
	result := db.Model(&User{}).
		Where("LOWER(address) = ?", strings.ToLower(strings.TrimSpace(address))).
		Update("username", strings.TrimSpace(username))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpdateNotificationPreferences persists a user's full notification-preferences
// object. It writes every column explicitly via a map-based Updates so that
// disabling a notification (setting a bool to false) is honored. The shared
// struct-based UpdateUser would skip false values as Go zero-values, which made
// it impossible for a user to turn any notification OFF. The request's
// preferences object is treated as authoritative (the UI always sends the full
// object), matching the frontend contract.
//
// The row is identified by email (the unique primary identifier for modern
// OAuth/email users), falling back to the wallet address for SIWE-only users
// that have no email. It MUST NOT key on an empty value: Address is only an
// `index` (not unique) and is empty for every OAuth user, so a bare
// Where("address = ?", "") would match and rewrite EVERY address-less user's
// preferences at once. An identity-less call is rejected rather than silently
// fanning out, and a zero-row update is surfaced rather than reported as success.
func UpdateNotificationPreferences(user structs.User, preferences structs.NotificationPreferences) error {
	q := db.Model(&User{})
	switch {
	case user.Email != "":
		q = q.Where("email = ?", user.Email)
	case user.Address != "":
		q = q.Where("address = ?", user.Address)
	default:
		return errors.New("cannot update notification preferences: user has neither email nor address")
	}

	result := q.Updates(map[string]interface{}{
		"email_enabled":         preferences.EmailEnabled,
		"news_enabled":          preferences.NewsEnabled,
		"updates_enabled":       preferences.UpdatesEnabled,
		"transactional_enabled": preferences.TransactionalEnabled,
		"security_enabled":      preferences.SecurityEnabled,
		"reports_enabled":       preferences.ReportsEnabled,
		"statistics_enabled":    preferences.StatisticsEnabled,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("notification preferences update matched no user")
	}
	return nil
}

// GetUserByEmail retrieves a user by their email address
func GetUserByEmail(email string) (structs.User, error) {
	var dbUser User
	result := db.Where("email = ?", email).First(&dbUser)
	if result.Error != nil {
		return structs.User{}, result.Error
	}

	return structs.User{
		Address:                 dbUser.Address,
		Email:                   dbUser.Email,
		Name:                    dbUser.Name,
		AuthMethod:              dbUser.AuthMethod,
		Role:                    structs.Role(dbUser.Role),
		NotificationPreferences: dbUser.NotificationPreferences,
		LanguageSelected:        dbUser.LanguageSelected,
	}, nil
}

func UpdateUserLanguage(address, email, language string) error {
	if address != "" {
		result := db.Model(&User{}).Where("address = ?", address).Update("language_selected", language)
		return result.Error
	}
	if email != "" {
		result := db.Model(&User{}).Where("email = ?", email).Update("language_selected", language)
		return result.Error
	}
	return nil
}
