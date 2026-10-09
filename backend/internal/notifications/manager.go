package notifications

import (
	"github.com/stdevmac/payverge/backend/internal/structs"
)

// NotificationManager delivers notifications by email.
type NotificationManager struct {
	emailDispatcher structs.NotificationDispatcher
}

// NewNotificationManager creates an email-only notification manager.
func NewNotificationManager(emailDispatcher structs.NotificationDispatcher) *NotificationManager {
	return &NotificationManager{
		emailDispatcher: emailDispatcher,
	}
}

// shouldSendNotification reports whether this template should be emailed
// given the user's preferences. EmailEnabled is the master switch; the
// template then selects the category flag.
func (m *NotificationManager) shouldSendNotification(templateName string, preferences structs.NotificationPreferences) bool {
	if !preferences.EmailEnabled {
		return false
	}

	switch templateName {
	case "generic_notification":
		return preferences.TransactionalEnabled
	case "welcome", "welcome_ai":
		return preferences.TransactionalEnabled
	case "password_reset", "email_verification":
		return preferences.TransactionalEnabled
	case "admin_notification", "daily_summary", "weekly_analytics":
		return preferences.ReportsEnabled
	default:
		return preferences.TransactionalEnabled // Default to transactional for unknown types
	}
}

// SendNotification stores the notification and emails it when the user's
// preferences allow. An admin who was not already emailed still receives
// the message; a send on the user path returns so the admin path cannot
// double-send.
func (m *NotificationManager) SendNotification(notification structs.Notification, user structs.User) {
	user.AddNotification(notification)

	if m.shouldSendNotification(notification.TemplateName, user.NotificationPreferences) {
		m.emailDispatcher.DispatchNotification(notification, user, "")
		return
	}

	if user.Role == structs.RoleAdmin {
		m.emailDispatcher.DispatchNotification(notification, user, "")
	}
}
