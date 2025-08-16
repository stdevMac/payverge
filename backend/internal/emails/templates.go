package emails

// Email template IDs for different notification types
// These should be updated with your own Postmark template IDs
const (
	// Basic templates
	TemplateWelcomeEmail        = 37695165 // Template for welcome email notifications
	TemplateGenericNotification = 37695166 // Template for generic notifications
	TemplatePasswordReset       = 37695167 // Template for password reset notifications
	TemplateEmailVerification   = 37695168 // Template for email verification

	// Spanish templates
	TemplateSpanishWelcomeEmail        = 39700604 // Template for Spanish welcome email
	TemplateSpanishGenericNotification = 39700605 // Template for Spanish generic notifications
	TemplateSpanishPasswordReset       = 39700606 // Template for Spanish password reset
	TemplateSpanishEmailVerification   = 39700607 // Template for Spanish email verification

	// Admin templates
	TemplateAdminNotification = 38670643 // Template for admin notifications
	TemplateDailyReport       = 38670644 // Template for daily reports
)
