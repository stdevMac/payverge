package migrations

import (
	"fmt"
	"web3-boilerplate/internal/database"
)

// SetDefaultNotificationPreferences sets all notification preference fields to true for existing users
func SetDefaultNotificationPreferences() error {
	err := database.SetDefaultNotificationPreferences()
	if err != nil {
		return fmt.Errorf("error setting default notification preferences: %v", err)
	}
	return nil
}
