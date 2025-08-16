package server

import (
	"web3-boilerplate/internal/notifications"
	"web3-boilerplate/internal/structs"
)

var notificationManager *notifications.NotificationManager

// SetNotificationManager sets the notification manager for the server package
func SetNotificationManager(manager *notifications.NotificationManager) {
	notificationManager = manager
}

// SendNotification sends a notification to a user using their preferred notification method
func SendNotification(notification structs.Notification, user structs.User) {
	if notificationManager != nil {
		notificationManager.SendNotification(notification, user)
	}
}
