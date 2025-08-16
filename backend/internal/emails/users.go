package emails

import (
	"web3-boilerplate/internal/structs"
	"strings"
)

func (e *EmailServer) SendWelcomeEmail(users []structs.User) error {
	for _, user := range users {
		name := user.Username
		if name == "" {
			name = strings.Split(user.Email, "@")[0]
		}

		// Define necessary fields
		actionURL := "https://app.tokenfleet.io/" // Adjust according to your platform
		supportEmail := "info@tokenfleet.io"
		liveChatURL := "https://tawk.to/chat/6714c45a4304e3196ad4afd3/1iakhu0s1"
		helpURL := "https://docs.tokenfleet.io/"
		senderName := "Marcos Maceo"

		// Prepare the data to send to Postmark template
		data := map[string]interface{}{
			"Name":          name,
			"action_url":    actionURL,
			"username":      user.Username,
			"support_email": supportEmail,
			"live_chat_url": liveChatURL,
			"sender_name":   senderName,
			"product_name":  "Token Fleet",
			"help_url":      helpURL,
			"name":          name,
		}

		to := []string{user.Email}
		// Send the email using the Postmark template
		if user.LanguageSelected == "es" {
			err := e.SendTransactionalEmail(to, TemplateSpanishWelcomeEmail, data)
			if err != nil {
				return err
			}
		} else {
			err := e.SendTransactionalEmail(to, TemplateWelcomeEmail, data)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
