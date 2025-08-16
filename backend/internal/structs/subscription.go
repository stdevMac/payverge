package structs

import (
	"context"
	"fmt"
	"github.com/dstotijn/go-notion"
	"time"
)

// Notion API parameters
const (
	notionToken      = "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" // Replace with your actual token
	notionDatabaseID = "88d8be92b3114178a946195e1c8460a2"                   // Replace with your database/page ID
)

var notionClient *notion.Client

func init() {
	notionClient = notion.NewClient(notionToken)
}

// SubscriptionRequest represents a newsletter subscription request
type SubscriptionRequest struct {
	// Personal Information
	Name        string `json:"name"`
	Email       string `json:"email"`
	PhoneNumber string `json:"phoneNumber"`
	Username    string `json:"username"`

	// Company Information
	CompanyName string `json:"companyName,omitempty"`

	// Contact Preferences
	SocialMedia string `json:"socialMedia"`
	ContactWay  string `json:"contactWay"`
	Message     string `json:"message"`
}

// Subscriber represents a newsletter subscriber
type Subscriber struct {
	// Basic Information
	Name             string    `json:"name" bson:"name"`
	Email            string    `json:"email" bson:"email"`
	SubscriptionDate time.Time `json:"subscription_date" bson:"subscription_date"`

	// Contact Information
	PhoneNumber      string `json:"phone_number,omitempty" bson:"phone_number,omitempty"`
	TwitterHandle    string `json:"twitter_handle,omitempty" bson:"twitter_handle,omitempty"`
	TelegramHandle   string `json:"telegram_handle,omitempty" bson:"telegram_handle,omitempty"`
	PreferredContact string `json:"preferred_contact,omitempty" bson:"preferred_contact,omitempty"`

	// Additional Information
	CompanyName string `json:"company_name,omitempty" bson:"company_name,omitempty"`
	Message     string `json:"message,omitempty" bson:"message,omitempty"`
}

// SendToNotion sends subscriber information to Notion database
func SendToNotion(subscriber Subscriber) error {
	properties := map[string]notion.DatabasePageProperty{
		"Name": notion.DatabasePageProperty{
			Title: []notion.RichText{
				{
					Text: &notion.Text{
						Content: subscriber.Name,
					},
				},
			},
		},
		"Email": notion.DatabasePageProperty{
			Email: &subscriber.Email,
		},
	}

	// Conditionally add Phone Number if it is not empty
	if subscriber.PhoneNumber != "" {
		properties["Phone Number"] = notion.DatabasePageProperty{
			PhoneNumber: &subscriber.PhoneNumber,
		}
	}

	// Conditionally add Twitter Handle if it is not empty
	if subscriber.TwitterHandle != "" {
		properties["Twitter Handle"] = notion.DatabasePageProperty{
			RichText: []notion.RichText{
				{
					Text: &notion.Text{
						Content: subscriber.TwitterHandle,
					},
				},
			},
		}
	}

	// Conditionally add Telegram Handle if it is not empty
	if subscriber.TelegramHandle != "" {
		properties["Telegram Handle"] = notion.DatabasePageProperty{
			RichText: []notion.RichText{
				{
					Text: &notion.Text{
						Content: subscriber.TelegramHandle,
					},
				},
			},
		}
	}
	// Conditionally add Company Name if it is not empty
	if subscriber.CompanyName != "" {
		properties["Company Name"] = notion.DatabasePageProperty{
			RichText: []notion.RichText{
				{
					Text: &notion.Text{
						Content: subscriber.CompanyName,
					},
				},
			},
		}
	}

	// Conditionally add Preferred Contact if it is not empty
	if subscriber.PreferredContact != "" {
		properties["Preferred Contact"] = notion.DatabasePageProperty{
			RichText: []notion.RichText{
				{
					Text: &notion.Text{
						Content: subscriber.PreferredContact,
					},
				},
			},
		}
	}

	// Conditionally add Message if it is not empty
	if subscriber.Message != "" {
		properties["Message"] = notion.DatabasePageProperty{
			RichText: []notion.RichText{
				{
					Text: &notion.Text{
						Content: subscriber.Message,
					},
				},
			},
		}
	}

	// Always add Subscription Date
	properties["Subscription Date"] = notion.DatabasePageProperty{
		Date: &notion.Date{
			Start: notion.DateTime{
				Time: subscriber.SubscriptionDate,
			},
		},
	}

	databaseProperty := notion.DatabasePageProperties(properties)

	// Send the subscriber data to Notion
	_, err := notionClient.CreatePage(context.Background(), notion.CreatePageParams{
		ParentType:             notion.ParentTypeDatabase,
		ParentID:               notionDatabaseID,
		DatabasePageProperties: &databaseProperty,
	})
	if err != nil {
		return fmt.Errorf("failed to create Notion page: %v", err)
	}

	return nil
}
