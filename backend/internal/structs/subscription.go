package structs

import (
	"time"
)

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
