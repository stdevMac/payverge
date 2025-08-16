package notifications

import (
	"web3-boilerplate/internal/emails"
	"web3-boilerplate/internal/structs"
)

// NotificationManager handles routing notifications to the appropriate dispatcher
type NotificationManager struct {
	emailDispatcher    structs.NotificationDispatcher
	telegramDispatcher structs.NotificationDispatcher
}

// NewNotificationManager creates a new notification manager
func NewNotificationManager(emailDispatcher, telegramDispatcher structs.NotificationDispatcher) *NotificationManager {
	return &NotificationManager{
		emailDispatcher:    emailDispatcher,
		telegramDispatcher: telegramDispatcher,
	}
}

// shouldSendNotification determines if a notification should be sent based on template ID, notification type and user preferences
func (m *NotificationManager) shouldSendNotification(templateID int, notificationType structs.NotificationPreference, preferences structs.NotificationPreferences) bool {
	// For telegram, we use the same preferences as email for now
	// This could be extended in the future to have separate telegram preferences
	if notificationType == structs.TGNotificationPreference || notificationType == structs.EmailNotificationPreference {
		if notificationType == structs.EmailNotificationPreference && !preferences.EmailEnabled {
			return false
		}

		switch templateID {
		case emails.TemplateGenericNotification, emails.TemplateSpanishGenericNotification:
			return preferences.TransactionalEnabled
		case emails.TemplateWelcomeEmail, emails.TemplateSpanishWelcomeEmail:
			return preferences.TransactionalEnabled
		case emails.TemplatePasswordReset, emails.TemplateSpanishPasswordReset:
			return preferences.TransactionalEnabled
		case emails.TemplateEmailVerification, emails.TemplateSpanishEmailVerification:
			return preferences.TransactionalEnabled
		case emails.TemplateAdminNotification, emails.TemplateDailyReport:
			return preferences.ReportsEnabled
		default:
			return preferences.TransactionalEnabled // Default to transactional for unknown types
		}
	}
	return false
}

// SendNotification sends a notification to a user based on their preference
func (m *NotificationManager) SendNotification(notification structs.Notification, user structs.User) {
	// Store notification in user's notifications list regardless of delivery method
	user.AddNotification(notification)

	// Route to appropriate dispatcher based on user preference
	switch user.NotificationPreference {
	case structs.TGNotificationPreference:
		if user.TGChatID != 0 && m.shouldSendNotification(notification.TemplateID, structs.TGNotificationPreference, user.NotificationPreferences) {
			m.telegramDispatcher.DispatchNotification(notification, user, "")
			return
		} else if m.shouldSendNotification(notification.TemplateID, structs.EmailNotificationPreference, user.NotificationPreferences) {
			// Fallback to email if Telegram chat ID is not set and email is enabled for this type
			m.emailDispatcher.DispatchNotification(notification, user, "")
			return
		}
	case structs.EmailNotificationPreference:
		if m.shouldSendNotification(notification.TemplateID, structs.EmailNotificationPreference, user.NotificationPreferences) {
			m.emailDispatcher.DispatchNotification(notification, user, "")
			return
		}
	}

	// Always send to admin via both channels if user is admin
	if user.Role == structs.RoleAdmin {
		m.telegramDispatcher.DispatchNotification(notification, user, "")
		m.emailDispatcher.DispatchNotification(notification, user, "")
	}
}
