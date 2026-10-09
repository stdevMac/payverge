package structs

import (
	"time"

	"github.com/stdevmac/payverge/backend/internal/utils"
)

type Notification struct {
	ID           string                 `json:"id" bson:"id"`
	Title        string                 `json:"title" bson:"title"`
	Description  string                 `json:"description" bson:"description"`
	IsRead       bool                   `json:"is_read" bson:"is_read"`
	UserID       uint                   `json:"user_id" bson:"user_id"`
	Date         time.Time              `json:"date" bson:"date"`
	TemplateName string                 `json:"template_name" bson:"template_name"`
	TemplateData map[string]interface{} `json:"template_data" bson:"template_data"`
	// MailOrigin attributes the email leg to a business so the tenant
	// outbound mail budget counts it (see emails.EmailOrigin). Zero means
	// system mail. Never serialized.
	MailOrigin NotificationMailOrigin `json:"-" bson:"-"`
}

// NotificationMailOrigin mirrors the subset of emails.EmailOrigin a
// notification can carry. It lives here because the emails package imports
// structs, not the other way round.
type NotificationMailOrigin struct {
	BusinessID      uint
	DeliveryOrderID uint
	// Purpose selects extra tenant-budget caps (an emails.MailPurpose* value).
	Purpose string
}

// NewNotification creates a new notification
func NewNotification(title, description string, userID uint) Notification {
	return Notification{
		ID:           utils.StringToRandomUint(title),
		Title:        title,
		Description:  description,
		IsRead:       false,
		UserID:       userID,
		Date:         time.Now(),
		TemplateData: make(map[string]interface{}),
	}
}

type NotificationDispatcher interface {
	DispatchNotification(notification Notification, user User, carId string)
}
