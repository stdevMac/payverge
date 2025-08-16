package server

import (
	"log"

	"github.com/stdevMac/shares/internal/database"
	"github.com/stdevMac/shares/internal/emails"
	"github.com/stdevMac/shares/internal/structs"
)

func getTemplateLanguage(templateId int, language string) int {
	if language != "es" {
		return templateId
	}
	switch templateId {
	case emails.TemplateFleetFunded:
		return emails.TemplateSpanishFleetFunded
	case emails.TemplateFleetAlmostFunded:
		return emails.TemplateSpanishFleetAlmostFunded
	case emails.TemplateFleetReadyToInvest:
		return emails.TemplateSpanishFleetReadyToInvest
	case emails.TemplateFleetNotRealized:
		return emails.TemplateSpanishFleetNotRealized
	case emails.TemplateCarSold:
		return emails.TemplateSpanishCarSold
	case emails.TemplateCarBought:
		return emails.TemplateSpanishCarBought
	case emails.TemplateCarRented:
		return emails.TemplateSpanishCarRented
	case emails.TemplateCarMaintenance:
		return emails.TemplateSpanishCarMaintenance
	case emails.TemplateCarAvailable:
		return emails.TemplateSpanishCarAvailable
	default:
		return templateId
	}
}

func sendGlobalNotification(title, description string, templateID int, templateData map[string]interface{}) {
	go func() {
		users, err := database.GetAllUsers()
		if err != nil {
			log.Printf("Failed to get all users: %v", err)
			return
		}

		// TODO: ON Release, uncomment this code
		// Get subscribers and create users from them
		//subscribers, err := database.GetSubscribers()
		//if err != nil {
		//	log.Printf("Failed to get subscribers: %v", err)
		//	return
		//}
		//
		//for _, subscriber := range subscribers {
		//	users = append(users, structs.User{
		//		TGChatID:               0,
		//		NotificationPreference: structs.EmailNotificationPreference,
		//		Email:                  subscriber.Email,
		//	})
		//
		//}

		for _, user := range users {
			notification := structs.NewTemplateNotification(title, description, uint(user.TGChatID), getTemplateLanguage(templateID, user.LanguageSelected), templateData)
			SendNotification(notification, user)
		}
	}()
}

// sendAdminNotification sends notifications to the admin
func sendAdminNotification(title, description string, templateID int, templateData map[string]interface{}) {
	go func() {
		adminUser := structs.User{
			TGChatID:               677517973,
			NotificationPreference: structs.TGNotificationPreference,
			Role:                   structs.RoleAdmin,
			Email:                  "marcos@tokenfleet.io",
		}
		adminNotification := structs.NewTemplateNotification(title, description, uint(adminUser.TGChatID), templateID, templateData)
		SendNotification(adminNotification, adminUser)
	}()
}
