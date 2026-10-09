package emails

import (
	"log"

	"github.com/stdevmac/payverge/backend/internal/locales"
	"github.com/stdevmac/payverge/backend/internal/structs"
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

// DispatchNotification sends the notification through the configured email provider.
func (d *EmailServerDispatcher) DispatchNotification(notification structs.Notification, user structs.User, carID string) {
	// If no template name is specified, use the default template
	templateName := notification.TemplateName
	if templateName == "" {
		templateName = "generic_notification" // Default template for generic notifications
	}

	// Use template data directly, no need to add title/description
	// If title/description are present in notification but not in data, add them?
	data := notification.TemplateData
	if data == nil {
		data = make(map[string]interface{})
	}
	// Ensure title/desc from notification are passed if not already
	if _, ok := data["title"]; !ok && notification.Title != "" {
		data["title"] = notification.Title
	}
	if _, ok := data["description"]; !ok && notification.Description != "" {
		data["description"] = notification.Description
	}

	lang := locales.EmailFamily(user.LanguageSelected)

	// A notification a business triggered carries its origin, so the send is
	// claimed against that business's tenant mail budget like any other
	// stamped send. Unstamped notifications stay system mail.
	server := d.server
	if origin := notification.MailOrigin; origin.BusinessID != 0 {
		server = server.WithOrigin(EmailOrigin{
			BusinessID:      origin.BusinessID,
			DeliveryOrderID: origin.DeliveryOrderID,
			Purpose:         origin.Purpose,
		})
	}

	// Send transactional email using local template
	err := server.SendTransactionalEmail([]string{user.Email}, templateName, data, lang)
	if err != nil {
		log.Printf("Failed to send notification email: %v", err)
	}
}
