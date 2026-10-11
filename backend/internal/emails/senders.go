package emails

import (
	"fmt"
	"html/template"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/config"
)

// Language constants
const (
	LanguageEnglish = "en"
	LanguageSpanish = "es"
)

// AdminNewSignupInfo carries the fields rendered in the internal
// admin_new_signup template. It intentionally carries no dollar amounts.
type AdminNewSignupInfo struct {
	BusinessName string
	OwnerName    string
	OwnerEmail   string
	Country      string
	BusinessType string
	DashboardURL string
}

// SendAdminNewSignupEmail alerts internal admins (AdminsEmails) whenever a new
// business signs up. It always renders in English — admin emails don't
// localize per owner, they go to the instance admins.
func (e *EmailServer) SendAdminNewSignupEmail(info AdminNewSignupInfo) error {
	templateBody := map[string]interface{}{
		"business_name": info.BusinessName,
		"owner_name":    info.OwnerName,
		"owner_email":   info.OwnerEmail,
		"country":       info.Country,
		"business_type": info.BusinessType,
		"dashboard_url": info.DashboardURL,
	}
	return e.sendAdminAlert("admin_new_signup", templateBody)
}

// Staff Email Senders

// SendAccessRemovedEmail sends an email when staff access is removed
func (e *EmailServer) SendAccessRemovedEmail(to []string, businessName, staffName, language string) error {
	templateBody := map[string]interface{}{
		"business_name": businessName,
		"staff_name":    staffName,
	}
	return e.SendTransactionalEmail(to, "access_removed", templateBody, language)
}

// SendRoleUpdateEmail sends an email when a staff member's role is updated
func (e *EmailServer) SendRoleUpdateEmail(to []string, businessName, staffName, role, language string) error {
	templateBody := map[string]interface{}{
		"business_name": businessName,
		"staff_name":    staffName,
		"role":          role,
	}
	return e.SendTransactionalEmail(to, "role_update", templateBody, language)
}

// SendStaffInvitationEmail sends an invitation email to a new staff member
func (e *EmailServer) SendStaffInvitationEmail(to []string, businessName, staffName, role, invitationURL, language string, expiresDays int) error {
	templateBody := map[string]interface{}{
		"business_name":  businessName,
		"staff_name":     staffName,
		"role":           role,
		"invitation_url": invitationURL,
		"expires_days":   expiresDays,
	}
	return e.SendTransactionalEmail(to, "staff_invitation", templateBody, language)
}

// SendStaffLoginCodeEmail sends a login code to a staff member
func (e *EmailServer) SendStaffLoginCodeEmail(to []string, businessName, staffName, loginCode, language string, expiresMinutes int) error {
	templateBody := map[string]interface{}{
		"business_name":   businessName,
		"staff_name":      staffName,
		"login_code":      loginCode,
		"expires_minutes": expiresMinutes,
	}
	return e.SendTransactionalEmail(to, "staff_login_code", templateBody, language)
}

// SendStaffAddedEmail sends an email to the owner when staff is added
func (e *EmailServer) SendStaffAddedEmail(to []string, ownerName, staffRole, dashboardURL, language string) error {
	templateBody := map[string]interface{}{
		"owner_name":    ownerName,
		"staff_role":    staffRole,
		"dashboard_url": dashboardURL,
	}
	return e.SendTransactionalEmail(to, "staff_added", templateBody, language)
}

// SendStaffRemovedEmail sends an email to the owner when staff is removed
func (e *EmailServer) SendStaffRemovedEmail(to []string, ownerName, dashboardURL, language string) error {
	templateBody := map[string]interface{}{
		"owner_name":    ownerName,
		"dashboard_url": dashboardURL,
	}
	return e.SendTransactionalEmail(to, "staff_removed", templateBody, language)
}

// Owner Onboarding & Lifecycle Email Senders

// aiProviderConfiguredForEmail is swappable in tests.
var aiProviderConfiguredForEmail = config.AIProviderConfigured

// welcomeTemplate picks the AI welcome only when this server can actually
// serve AI: without an LLM provider every AI route answers 503
// ai_not_configured, so the email must not advertise those features.
func welcomeTemplate() string {
	if aiProviderConfiguredForEmail() {
		return "welcome_ai"
	}
	return "welcome"
}

// SendBusinessOnboardingEmail sends the canonical registration/onboarding
// email with capabilities and setup guidance.
func (e *EmailServer) SendBusinessOnboardingEmail(to []string, ownerName, dashboardURL, language string) error {
	templateBody := map[string]interface{}{
		"owner_name":    ownerName,
		"dashboard_url": dashboardURL,
	}
	return e.SendTransactionalEmail(to, welcomeTemplate(), templateBody, language)
}

// SendGettingStartedEmail sends a getting started guide email
func (e *EmailServer) SendGettingStartedEmail(to []string, ownerName, dashboardURL, language string) error {
	templateBody := map[string]interface{}{
		"owner_name":    ownerName,
		"dashboard_url": dashboardURL,
	}
	return e.SendTransactionalEmail(to, "getting_started", templateBody, language)
}

// SendSetupNudgeEmail nudges a business (~day 3) that has not finished its
// required setup, segmented by the first missing step (menu | tables | profile).
// verifyEmail adds a "verify your email" reminder for owners whose email-auth
// record is still unverified (they'd otherwise hit the login lockout later).
func (e *EmailServer) SendSetupNudgeEmail(to []string, ownerName, missingStep, dashboardURL string, verifyEmail bool, language string) error {
	templateBody := map[string]interface{}{
		"owner_name":    ownerName,
		"missing_step":  missingStep,
		"dashboard_url": dashboardURL,
		"verify_email":  verifyEmail,
	}
	return e.SendTransactionalEmail(to, "setup_nudge", templateBody, language)
}

// Auth Email Senders

// SendPasswordResetEmail sends the localized password-reset email.
func (e *EmailServer) SendPasswordResetEmail(to []string, resetLink, language string) error {
	return e.SendTransactionalEmail(to, "password_reset", map[string]interface{}{
		"reset_link":    resetLink,
		"expires_hours": "1",
	}, language)
}

// SendEmailVerificationEmail sends the localized email-verification email.
func (e *EmailServer) SendEmailVerificationEmail(to []string, displayName, verifyLink, language string) error {
	return e.SendTransactionalEmail(to, "email_verification", map[string]interface{}{
		"display_name":  displayName,
		"verify_link":   verifyLink,
		"expires_hours": "24",
	}, language)
}

// SendAccountClosureEmail sends an email when admin closes an account
func (e *EmailServer) SendAccountClosureEmail(
	to []string,
	ownerName string,
	businessName string,
	reason string,
	language string,
) error {
	templateBody := map[string]interface{}{
		"owner_name":    ownerName,
		"business_name": businessName,
		"reason":        reason,
	}
	return e.SendTransactionalEmail(to, "account_closure", templateBody, language)
}

// Security Email Senders

// SendWalletChangeEmail sends an email when wallet address is changed
func (e *EmailServer) SendWalletChangeEmail(to []string, ownerName, dashboardURL, language string) error {
	templateBody := map[string]interface{}{
		"owner_name":    ownerName,
		"dashboard_url": dashboardURL,
	}
	return e.SendTransactionalEmail(to, "wallet_change", templateBody, language)
}

// Analytics Email Senders

// SendLowStockAlertEmail sends a low-stock alert email listing items at or below their reorder threshold.
// items is a slice of maps with keys: name, unit, current_quantity, reorder_threshold, alert_type ("low_stock" or "out_of_stock").
func (e *EmailServer) SendLowStockAlertEmail(to []string, ownerName, dashboardURL, language string, items []map[string]interface{}) error {
	templateBody := map[string]interface{}{
		"owner_name":    ownerName,
		"dashboard_url": dashboardURL,
		"items":         items,
	}
	return e.SendTransactionalEmail(to, "low_stock_alert", templateBody, language)
}

// SendDirectorDigestEmail sends a daily AI briefing email to a business owner.
// insightsSummary must be a template.HTML value so that html/template renders the
// pre-assembled markup without double-escaping. The caller (director_digest_scheduler)
// is responsible for HTML-escaping any user-controlled data embedded in the markup
// before passing it here.
func (e *EmailServer) SendDirectorDigestEmail(to []string, ownerName, businessName, digestDate, directorConsoleURL string, insightsSummary template.HTML, language string) error {
	templateBody := map[string]interface{}{
		"owner_name":           ownerName,
		"business_name":        businessName,
		"digest_date":          digestDate,
		"director_console_url": directorConsoleURL,
		"insights_summary":     insightsSummary,
	}
	return e.SendTransactionalEmail(to, "director_digest", templateBody, language)
}

// SendDailySummaryEmailIdempotent is the scheduled-worker variant. Replays of
// the same report window retain one provider idempotency key.
func (e *EmailServer) SendDailySummaryEmailIdempotent(to []string, ownerName, totalOrders, totalRevenue, averageBill, dashboardURL, language, idempotencyKey string) error {
	templateBody := map[string]interface{}{
		"owner_name": ownerName, "total_orders": totalOrders, "total_revenue": totalRevenue,
		"average_bill": averageBill, "dashboard_url": dashboardURL,
	}
	return e.SendTransactionalEmailIdempotent(to, "daily_summary", templateBody, language, idempotencyKey)
}

// SendWeeklyAnalyticsEmailIdempotent is the scheduled-worker variant.
func (e *EmailServer) SendWeeklyAnalyticsEmailIdempotent(to []string, ownerName, totalOrders, totalRevenue, averageBill, topItem, dashboardURL, language, idempotencyKey string) error {
	templateBody := map[string]interface{}{
		"owner_name": ownerName, "total_orders": totalOrders, "total_revenue": totalRevenue,
		"average_bill": averageBill, "top_item": topItem, "dashboard_url": dashboardURL,
	}
	return e.SendTransactionalEmailIdempotent(to, "weekly_analytics", templateBody, language, idempotencyKey)
}

// Milestone Email Senders

// SendMilestoneFirstOrderEmail sends an email celebrating the first order
func (e *EmailServer) SendMilestoneFirstOrderEmail(to []string, ownerName, dashboardURL, language string) error {
	templateBody := map[string]interface{}{
		"owner_name":    ownerName,
		"dashboard_url": dashboardURL,
	}
	return e.SendTransactionalEmail(to, "milestone_first_order", templateBody, language)
}

// SendMilestonesRevenueThresholdsEmail sends an email celebrating revenue milestones
func (e *EmailServer) SendMilestonesRevenueThresholdsEmail(to []string, ownerName, revenueAmount, dashboardURL, language string) error {
	templateBody := map[string]interface{}{
		"owner_name":     ownerName,
		"revenue_amount": revenueAmount,
		"dashboard_url":  dashboardURL,
	}
	return e.SendTransactionalEmail(to, "milestone_revenue_threshold", templateBody, language)
}

// Platform & Operations Email Senders

// SendOperationalUpdatesEmail sends operational updates to users
func (e *EmailServer) SendOperationalUpdatesEmail(to []string, recipientName, updateTitle, updateIntro, updateBody, dashboardURL, language string) error {
	templateBody := map[string]interface{}{
		"recipient_name": recipientName,
		"update_title":   updateTitle,
		"update_intro":   updateIntro,
		"update_body":    updateBody,
		"dashboard_url":  dashboardURL,
	}
	return e.SendTransactionalEmail(to, "operational_update", templateBody, language)
}

// SendPayvergeUpdateEmail sends platform updates to users
func (e *EmailServer) SendPayvergeUpdateEmail(to []string, recipientName, updateTitle, updateIntro, updateBody, dashboardURL, language string) error {
	templateBody := map[string]interface{}{
		"recipient_name": recipientName,
		"update_title":   updateTitle,
		"update_intro":   updateIntro,
		"update_body":    updateBody,
		"dashboard_url":  dashboardURL,
	}
	return e.SendUpdatesEmail(to, "payverge_update", templateBody, language)
}

// Guest Email Senders (Privacy-First)

// SendPaymentReceiptEmail sends a payment receipt to a guest. items is a slice of
// maps with keys: name (string), quantity (int), line_total (pre-formatted dollar
// string, e.g. "$24.00"). The template escapes each field natively.
func (e *EmailServer) SendPaymentReceiptEmail(to []string, businessName, paymentDate, paymentMethod, transactionID string, items []map[string]interface{}, totalAmount, language string) error {
	templateBody := map[string]interface{}{
		"business_name":  businessName,
		"payment_date":   paymentDate,
		"payment_method": paymentMethod,
		"transaction_id": transactionID,
		"items":          items,
		"total_amount":   totalAmount,
	}
	return e.SendTransactionalEmail(to, "payment_receipt", templateBody, language)
}

// SendThankYouGuestEmail sends a thank you email to a guest
func (e *EmailServer) SendThankYouGuestEmail(to []string, businessName, language string) error {
	templateBody := map[string]interface{}{
		"business_name": businessName,
	}
	return e.SendTransactionalEmail(to, "thank_you_guest", templateBody, language)
}

// SendFiscalReceiptEmail emails an authorized fiscal receipt (AFIP factura /
// nota de crédito) to the guest with the rendered PDF attached. receiptTypeLabel
// is the human-facing document title (e.g. "Factura C"); receiptNumber is the
// AFIP comprobante number; totalDisplay is the pre-formatted total (e.g.
// "ARS 1.210,00"). The locale (es/es_ar/en) selects the template family. It is
// a guest template (no account footer). The PDF is delivered as an attachment so
// the guest keeps a self-contained copy.
func (e *EmailServer) SendFiscalReceiptEmail(to []string, businessName, receiptTypeLabel, receiptNumber, totalDisplay, language string, pdf []byte) error {
	templateBody := map[string]interface{}{
		"business_name":  businessName,
		"receipt_type":   receiptTypeLabel,
		"receipt_number": receiptNumber,
		"total_amount":   totalDisplay,
	}
	var attachments []EmailAttachment
	if len(pdf) > 0 {
		filename := "receipt.pdf"
		if strings.TrimSpace(receiptNumber) != "" {
			filename = fmt.Sprintf("receipt-%s.pdf", strings.ReplaceAll(strings.TrimSpace(receiptNumber), "/", "-"))
		}
		attachments = append(attachments, EmailAttachment{
			Filename:    filename,
			Content:     pdf,
			ContentType: "application/pdf",
		})
	}
	return e.SendTransactionalEmailWithAttachments(to, "fiscal_receipt", templateBody, language, attachments)
}

// SendGuestFeedbackEmail sends a feedback request to a guest
func (e *EmailServer) SendGuestFeedbackEmail(to []string, businessName, feedbackURL, language string) error {
	templateBody := map[string]interface{}{
		"business_name": businessName,
		"feedback_url":  feedbackURL,
	}
	return e.SendTransactionalEmail(to, "guest_feedback", templateBody, language)
}

// Validation helpers

// Reservation Email Senders
//
// Guest-facing reservation mail goes to an address the guest typed, so it
// must not echo the guest's own free text back: customer name, special
// requests and the guest's phone are accepted for signature stability but are
// NOT put in the template body (an attacker could otherwise book with a
// victim's address and a phishing sentence as the "name"). The venue sees
// those fields on the dashboard and in SendReservationApprovalReminderEmail,
// which goes to the operator. phone_number in the cancelled / no-show /
// declined / reminder mail is the venue's phone and stays.

// SendReservationConfirmationEmail sends a confirmation email when a reservation is created.
// customerName, specialRequests and phoneNumber (the guest's) are not rendered.
func (e *EmailServer) SendReservationConfirmationEmail(
	to []string,
	customerName string,
	businessName string,
	businessAddress string,
	reservationDate string,
	reservationTime string,
	partySize int,
	tableName string,
	specialRequests string,
	confirmationURL string,
	cancellationURL string,
	phoneNumber string,
	language string,
) error {
	templateBody := guestReservationTemplateBody(businessName, businessAddress, reservationDate, reservationTime, partySize, tableName)
	if confirmationURL != "" {
		templateBody["confirmation_url"] = confirmationURL
	}
	if cancellationURL != "" {
		templateBody["cancellation_url"] = cancellationURL
	}

	return e.SendTransactionalEmail(to, "reservation_confirmation", templateBody, language)
}

// SendReservationPendingEmail tells a guest their reservation request was
// received and is awaiting the business's approval. There is no guest confirm
// link anymore — approval belongs to the business; respondBy is the
// human-readable moment by which the guest will hear back. customerName,
// specialRequests and phoneNumber (the guest's) are not rendered.
func (e *EmailServer) SendReservationPendingEmail(
	to []string,
	customerName string,
	businessName string,
	businessAddress string,
	reservationDate string,
	reservationTime string,
	partySize int,
	tableName string,
	specialRequests string,
	respondBy string,
	cancellationURL string,
	phoneNumber string,
	language string,
) error {
	templateBody := guestReservationTemplateBody(businessName, businessAddress, reservationDate, reservationTime, partySize, tableName)
	if respondBy != "" {
		templateBody["respond_by"] = respondBy
	}
	if cancellationURL != "" {
		templateBody["cancellation_url"] = cancellationURL
	}

	return e.SendTransactionalEmail(to, "reservation_pending", templateBody, language)
}

// SendReservationApprovalReminderEmail nudges the OPERATOR (business contact
// email) that a manual-approval reservation request is still unactioned and
// will auto-decline at respondBy. Sent once per request, at the halfway point
// of its approval window.
func (e *EmailServer) SendReservationApprovalReminderEmail(
	to []string,
	ownerName string,
	businessName string,
	customerName string,
	partySize int,
	reservationDate string,
	reservationTime string,
	respondBy string,
	specialRequests string,
	dashboardURL string,
	language string,
) error {
	if ownerName == "" {
		ownerName = businessName
	}
	templateBody := map[string]interface{}{
		"owner_name":       ownerName,
		"business_name":    businessName,
		"customer_name":    customerName,
		"party_size":       partySize,
		"reservation_date": reservationDate,
		"reservation_time": reservationTime,
		"respond_by":       respondBy,
		"dashboard_url":    dashboardURL,
	}
	if specialRequests != "" {
		templateBody["special_requests"] = specialRequests
	}
	return e.SendTransactionalEmail(to, "reservation_approval_reminder", templateBody, language)
}

// SendReservationReminderEmail sends a reminder email 24 hours before the reservation.
// customerName and specialRequests are not rendered; phoneNumber is the venue's.
func (e *EmailServer) SendReservationReminderEmail(
	to []string,
	customerName string,
	businessName string,
	businessAddress string,
	reservationDate string,
	reservationTime string,
	partySize int,
	tableName string,
	specialRequests string,
	confirmationURL string,
	cancellationURL string,
	phoneNumber string,
	language string,
) error {
	templateBody := guestReservationTemplateBody(businessName, businessAddress, reservationDate, reservationTime, partySize, tableName)
	if confirmationURL != "" {
		templateBody["confirmation_url"] = confirmationURL
	}
	if cancellationURL != "" {
		templateBody["cancellation_url"] = cancellationURL
	}
	if phoneNumber != "" {
		templateBody["phone_number"] = phoneNumber
	}

	return e.SendTransactionalEmail(to, "reservation_reminder", templateBody, language)
}

// SendReservationCancelledEmail sends an email when a reservation is cancelled.
// customerName is not rendered; phoneNumber is the venue's.
func (e *EmailServer) SendReservationCancelledEmail(
	to []string,
	customerName string,
	businessName string,
	reservationDate string,
	reservationTime string,
	phoneNumber string,
	language string,
) error {
	templateBody := map[string]interface{}{
		"business_name":    businessName,
		"reservation_date": reservationDate,
		"reservation_time": reservationTime,
	}

	// Add optional phone number if provided
	if phoneNumber != "" {
		templateBody["phone_number"] = phoneNumber
	}

	return e.SendTransactionalEmail(to, "reservation_cancelled", templateBody, language)
}

// SendReservationNoShowEmail notifies a guest they were marked no-show.
// Uses a dedicated template (not cancellation reuse) so inbox copy is accurate.
// customerName is not rendered; phoneNumber is the venue's.
func (e *EmailServer) SendReservationNoShowEmail(
	to []string,
	customerName string,
	businessName string,
	reservationDate string,
	reservationTime string,
	phoneNumber string,
	language string,
) error {
	templateBody := map[string]interface{}{
		"business_name":    businessName,
		"reservation_date": reservationDate,
		"reservation_time": reservationTime,
	}
	if phoneNumber != "" {
		templateBody["phone_number"] = phoneNumber
	}
	return e.SendTransactionalEmail(to, "reservation_noshow", templateBody, language)
}

// SendReservationDeclinedEmail tells a guest the business (or the approval
// timeout) declined their pending reservation request. customerName is not
// rendered; phoneNumber is the venue's.
func (e *EmailServer) SendReservationDeclinedEmail(
	to []string,
	customerName string,
	businessName string,
	reservationDate string,
	reservationTime string,
	reason string,
	bookingURL string,
	phoneNumber string,
	language string,
) error {
	templateBody := map[string]interface{}{
		"business_name":    businessName,
		"reservation_date": reservationDate,
		"reservation_time": reservationTime,
	}

	// Optional fields: omit entirely so the template conditionals skip them
	if reason != "" {
		templateBody["reason"] = reason
	}
	if bookingURL != "" {
		templateBody["booking_url"] = bookingURL
	}
	if phoneNumber != "" {
		templateBody["phone_number"] = phoneNumber
	}

	return e.SendTransactionalEmail(to, "reservation_declined", templateBody, language)
}

// SendReservationUpdatedEmail sends an email when a reservation is modified.
// customerName, specialRequests and phoneNumber (the guest's) are not rendered.
func (e *EmailServer) SendReservationUpdatedEmail(
	to []string,
	customerName string,
	businessName string,
	businessAddress string,
	reservationDate string,
	reservationTime string,
	partySize int,
	tableName string,
	specialRequests string,
	confirmationURL string,
	cancellationURL string,
	phoneNumber string,
	language string,
) error {
	templateBody := guestReservationTemplateBody(businessName, businessAddress, reservationDate, reservationTime, partySize, tableName)
	if confirmationURL != "" {
		templateBody["confirmation_url"] = confirmationURL
	}
	if cancellationURL != "" {
		templateBody["cancellation_url"] = cancellationURL
	}

	return e.SendTransactionalEmail(to, "reservation_updated", templateBody, language)
}

// guestReservationTemplateBody is the venue-and-slot core every guest-facing
// reservation email shares. It deliberately has no slot for guest-typed text.
func guestReservationTemplateBody(businessName, businessAddress, reservationDate, reservationTime string, partySize int, tableName string) map[string]interface{} {
	return map[string]interface{}{
		"business_name":    businessName,
		"business_address": businessAddress,
		"reservation_date": reservationDate,
		"reservation_time": reservationTime,
		"party_size":       partySize,
		"table_name":       tableName,
	}
}
