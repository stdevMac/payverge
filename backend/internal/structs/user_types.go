package structs

import "time"

type NotificationPreference string

var (
	EmailNotificationPreference NotificationPreference = "email"
	TGNotificationPreference    NotificationPreference = "telegram"
)

// User represents a registered user in the system
type User struct {
	// Basic Information
	Username                string                  `json:"username" bson:"username"`
	JoinedAt                time.Time               `json:"joined_at" bson:"joined_at"`
	Email                   string                  `json:"email" bson:"email"`
	Address                 string                  `json:"address" bson:"address"`
	Role                    Role                    `json:"role" bson:"role"`
	TGChatID                int64                   `json:"telegram_chat_id" bson:"telegram_chat_id"`
	NotificationPreference  NotificationPreference  `json:"notification_preference" bson:"notification_preference"`
	NotificationPreferences NotificationPreferences `json:"notification_preferences" bson:"notification_preferences"`
	LanguageSelected        string                  `json:"language_selected" bson:"language_selected"`

	// Referral System
	ReferralCode  string           `json:"referral_code" bson:"referral_code"`
	Referrer      string           `json:"referrer" bson:"referrer"`
	RefereePoints map[string]int64 `json:"referees" bson:"referees"`

	// Communication
	Notifications []Notification `json:"notifications" bson:"notifications"`
}

type NotificationType string

const (
	NotificationNews          NotificationType = "news"
	NotificationUpdates       NotificationType = "updates"
	NotificationGeneral       NotificationType = "general"
	NotificationRewards       NotificationType = "rewards"
	NotificationTransactional NotificationType = "transactional"
	NotificationSecurity      NotificationType = "security"
	NotificationReports       NotificationType = "reports"
	NotificationStatistics    NotificationType = "statistics"
)

// PointsExpense represents points spent by a user
type PointsExpense struct {
	Amount float64 `json:"amount" bson:"amount"`
	Date   string  `json:"date" bson:"date"`
	Item   string  `json:"item" bson:"item"`
}

// Email represents a simple email address
type Email struct {
	Email string `json:"email" bson:"email"`
}

// NotificationPreferences represents a user's notification settings for both email and telegram
type NotificationPreferences struct {
	// General settings
	EmailEnabled bool `json:"email_enabled" bson:"email_enabled"` // Master switch for all email notifications

	// News and Updates
	NewsEnabled    bool `json:"news_enabled" bson:"news_enabled"`       // Platform news and announcements
	UpdatesEnabled bool `json:"updates_enabled" bson:"updates_enabled"` // Platform updates and changes

	// Account notifications
	TransactionalEnabled bool `json:"transactional_enabled" bson:"transactional_enabled"` // Essential account-related notifications
	SecurityEnabled      bool `json:"security_enabled" bson:"security_enabled"`           // Security-related notifications
	ReportsEnabled       bool `json:"reports_enabled" bson:"reports_enabled"`             // Daily and periodic reports
	StatisticsEnabled    bool `json:"statistics_enabled" bson:"statistics_enabled"`       // Platform statistics and analytics
}

// NewDefaultNotificationPreferences creates a new NotificationPreferences instance with all preferences enabled
func NewDefaultNotificationPreferences() NotificationPreferences {
	return NotificationPreferences{
		EmailEnabled:         true,
		NewsEnabled:          true,
		UpdatesEnabled:       true,
		TransactionalEnabled: true,
		SecurityEnabled:      true,
		ReportsEnabled:       true,
		StatisticsEnabled:    true,
	}
}
