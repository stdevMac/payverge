package structs

import "time"

// User represents a registered user in the system
type User struct {
	// Basic Information
	Username                string                  `json:"username" bson:"username"`
	JoinedAt                time.Time               `json:"joined_at" bson:"joined_at"`
	Email                   string                  `json:"email" bson:"email"`
	Address                 string                  `json:"address" bson:"address"`
	Role                    Role                    `json:"role" bson:"role"`
	NotificationPreferences NotificationPreferences `json:"notification_preferences" bson:"notification_preferences"`
	LanguageSelected        string                  `json:"language_selected" bson:"language_selected"`

	AuthMethod string `json:"auth_method" bson:"auth_method"` // "social", "email", "wallet"
	Name       string `json:"name" bson:"name"`               // User's display name from social login

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

// NotificationPreferences represents a user's email notification settings.
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
