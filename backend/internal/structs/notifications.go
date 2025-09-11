package structs

import (
	"time"

	"payverge/internal/utils"
)

type Notification struct {
	ID           string                 `json:"id" bson:"id"`
	Title        string                 `json:"title" bson:"title"`
	Description  string                 `json:"description" bson:"description"`
	IsRead       bool                   `json:"is_read" bson:"is_read"`
	UserID       uint                   `json:"user_id" bson:"user_id"`
	Date         time.Time              `json:"date" bson:"date"`
	TemplateID   int                    `json:"template_id" bson:"template_id"`
	TemplateData map[string]interface{} `json:"template_data" bson:"template_data"`
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

// NewTemplateNotification creates a new notification with template information
func NewTemplateNotification(title, description string, userID uint, templateID int, templateData map[string]interface{}) Notification {
	notification := NewNotification(title, description, userID)
	notification.TemplateID = templateID
	notification.TemplateData = templateData
	return notification
}

type NotificationDispatcher interface {
	DispatchNotification(notification Notification, user User, carId string)
}
