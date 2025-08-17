package emails

import (
	"net/http"
	"strings"

	"github.com/ethereum/go-ethereum/log"
	"github.com/mattevans/postmark-go"
)

var EmailServerInstance *EmailServer

// TODO: Change AdminsEmails to be configurable
var AdminsEmails = []string{"admin@yourapp.com", "admin@yourcompany.com", "admin@example.com"}

type EmailServer struct {
	FromTransactional string
	FromNews          string
	FromUpdates       string
	client            *postmark.Client
}

func NewEmailServer(fromTransactional, fromNews, fromUpdates, token string) *EmailServer {

	EmailServerInstance = &EmailServer{
		FromTransactional: fromTransactional,
		FromNews:          fromNews,
		FromUpdates:       fromUpdates,
		client: postmark.NewClient(
			postmark.WithClient(&http.Client{
				Transport: &postmark.AuthTransport{Token: token},
			}),
		),
	}
	return EmailServerInstance
}

func (e *EmailServer) addUnsubscribeURL(templateBody map[string]interface{}, emails []string) {
	if len(emails) > 0 {
		templateBody["unsubscribe_url"] = "https://yourapp.com/profile?tab=notifications"
		templateBody["unsubscribe_message"] = "To unsubscribe from these emails, please log in to your account and visit your notification settings."
	}
}

func (e *EmailServer) SendTransactionalEmail(to []string, templateID int, templateBody map[string]interface{}) error {
	templateBody["company_name"] = "Web3 Boilerplate, from Your Company LLC"
	templateBody["company_address"] = "Rasis Business Center, Office 75, 4th Floor, Al Barsha, Dubai, UAE"
	templateBody["product_name"] = "Web3 Boilerplate"

	e.addUnsubscribeURL(templateBody, to)

	email := &postmark.Email{
		From:          e.FromTransactional,
		To:            strings.Join(to, ","),
		Tag:           "transactional",
		TemplateID:    templateID,
		TemplateModel: templateBody,
		ReplyTo:       "info@yourapp.com",
		MessageStream: "outbound",
	}

	emailResponse, resp, err := e.client.Email.Send(email)
	if err != nil {
		log.Error("Failed to send email", "error", err, "to", to)
		return err
	}
	if resp.StatusCode != http.StatusOK {
		log.Error("Failed to send email", "status", resp.Status, "to", to)
		return err
	}

	log.Info("Email sent", "to", to, "MessageID", emailResponse.MessageID)
	return nil
}

func (e *EmailServer) SendUpdatesEmail(to []string, templateID int, templateBody map[string]interface{}) error {
	templateBody["company_name"] = "Web3 Boilerplate, from Your Company LLC"
	templateBody["company_address"] = "Rasis Business Center, Office 75, 4th Floor, Al Barsha, Dubai, UAE"
	templateBody["product_name"] = "Web3 Boilerplate"

	e.addUnsubscribeURL(templateBody, to)

	email := &postmark.Email{
		From:          e.FromUpdates,
		To:            strings.Join(to, ","),
		Tag:           "updates",
		TemplateID:    templateID,
		TemplateModel: templateBody,
		ReplyTo:       "info@yourapp.com",
		MessageStream: "broadcast",
	}

	emailResponse, resp, err := e.client.Email.Send(email)
	if err != nil {
		log.Error("Failed to send email", "error", err, "to", to)
		return err
	}
	if resp.StatusCode != http.StatusOK {
		log.Error("Failed to send email", "status", resp.Status, "to", to)
		return err
	}

	log.Info("Email sent", "to", to, "MessageID", emailResponse.MessageID)
	return nil
}

func (e *EmailServer) SendNewsEmail(to []string, templateID int, templateBody map[string]interface{}) error {
	templateBody["company_name"] = "Web3 Boilerplate, from Your Company LLC"
	templateBody["company_address"] = "Rasis Business Center, Office 75, 4th Floor, Al Barsha, Dubai, UAE"
	templateBody["product_name"] = "Web3 Boilerplate"

	e.addUnsubscribeURL(templateBody, to)

	email := &postmark.Email{
		From:          e.FromTransactional,
		To:            strings.Join(to, ","),
		Tag:           "news",
		TemplateID:    templateID,
		TemplateModel: templateBody,
		ReplyTo:       "info@yourapp.com",
		MessageStream: "news-broadcast-stream",
	}

	emailResponse, resp, err := e.client.Email.Send(email)
	if err != nil {
		log.Error("Failed to send email", "error", err, "to", to)
		return err
	}
	if resp.StatusCode != http.StatusOK {
		log.Error("Failed to send email", "status", resp.Status, "to", to)
		return err
	}

	log.Info("Email sent", "to", to, "MessageID", emailResponse.MessageID)
	return nil
}

func (e *EmailServer) SendAdminEmail(templateID int, templateBody map[string]interface{}) error {

	templateBody["company_name"] = "Web3 Boilerplate, from Your Company LLC"
	templateBody["company_address"] = "Rasis Business Center, Office 75, 4th Floor, Al Barsha, Dubai, UAE"
	templateBody["product_name"] = "Web3 Boilerplate"

	e.addUnsubscribeURL(templateBody, AdminsEmails)

	email := &postmark.Email{
		From:          e.FromTransactional,
		To:            strings.Join(AdminsEmails, ","),
		Tag:           "admin",
		TemplateID:    templateID,
		TemplateModel: templateBody,
		ReplyTo:       "info@yourapp.com",
		MessageStream: "admin-notifications",
	}

	emailResponse, resp, err := e.client.Email.Send(email)
	if err != nil {
		log.Error("Failed to send email", "error", err, "to", AdminsEmails)
		return err
	}
	if resp.StatusCode != http.StatusOK {
		log.Error("Failed to send email", "status", resp.Status, "to", AdminsEmails)
		return err
	}

	log.Info("Email sent", "to", AdminsEmails, "MessageID", emailResponse.MessageID)
	return nil
}
