package emails

import (
	"log"

	"web3-boilerplate/internal/structs"
)

// EmailServerDispatcher wraps the EmailServer to implement NotificationDispatcher
type EmailServerDispatcher struct {
	server *EmailServer
}

// NewEmailServerDispatcher creates a new email server dispatcher
func NewEmailServerDispatcher(server *EmailServer) *EmailServerDispatcher {
	return &EmailServerDispatcher{
		server: server,
	}
}

// DispatchNotification sends the notification via Postmark email
func (d *EmailServerDispatcher) DispatchNotification(notification structs.Notification, user structs.User, carID string) {
	// If no template ID is specified, use the default template
	templateID := notification.TemplateID
	if templateID == 0 {
		templateID = 37695165 // Default template for generic notifications
	}

	// Use template data directly, no need to add title/description
	data := notification.TemplateData
	if data == nil {
		data = make(map[string]interface{})
	}

	// Send transactional email using Postmark template
	err := d.server.SendTransactionalEmail([]string{user.Email}, templateID, data)
	if err != nil {
		log.Printf("Failed to send notification email: %v", err)
	}
}
